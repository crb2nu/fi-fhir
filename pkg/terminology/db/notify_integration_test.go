//go:build integration

// Kill-test for the serve-time pending-autoroute review notifier (Lane C2).
//
// This lives in the external db_test package for the same reason as the expiry
// sweep kill-test: the notifier is in internal/terminology/autoroute, which
// imports this package, so an in-package test importing it would be an import
// cycle.
//
// Run with the same CI-compatible DSN path as the rest of the package:
//
//	POSTGRES_TEST_URL=postgres://user:pass@localhost:5432/testdb \
//	    go test -tags=integration -p 1 ./pkg/terminology/db/... -run ReviewNotif
package db_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/terminology/autoroute"
	termdb "gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

// newNotifyTestStore builds a migrated store against POSTGRES_TEST_URL.
func newNotifyTestStore(t *testing.T) (*termdb.MappingStore, context.Context) {
	t.Helper()

	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_URL not set; skipping pending autoroute notification integration test")
	}

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("connect to PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}

	ctx := context.Background()
	if _, err := sqlDB.ExecContext(ctx, "DROP SCHEMA IF EXISTS terminology CASCADE"); err != nil {
		t.Fatalf("drop schema: %v", err)
	}
	if _, err := termdb.NewMigrator(sqlDB).Initialize(ctx); err != nil {
		t.Fatalf("initialize schema: %v", err)
	}

	return termdb.NewMappingStore(sqlDB), ctx
}

// TestReviewNotifier_HighConfidenceRowsReachWebhook is the Lane C2 kill-test.
//
// Real store rows on both sides of the threshold; a real HTTP receiver. It
// asserts that exactly the above-threshold rows are delivered and that the
// payload carries no free-text or LLM-authored content from the row.
func TestReviewNotifier_HighConfidenceRowsReachWebhook(t *testing.T) {
	store, ctx := newNotifyTestStore(t)

	future := time.Now().Add(24 * time.Hour)
	// Free text that must never leave the process. If the projection ever widens
	// to the whole row, these strings show up on the wire and fail the test.
	const poisonReasoning = "the note for the patient at 12 Oak Street mentions glucose"
	const poisonDisplay = "JANE DOE 1954-03-02 SERUM GLUCOSE"

	rows := []*termdb.PendingAutoroute{
		{
			SourceSystem: "epic_labs", SourceCode: "NOTIFY_HIGH_1", TargetSystem: "http://loinc.org",
			SuggestedCode: "2345-7", Confidence: 0.97, ExpiresAt: &future,
			SourceDisplay: poisonDisplay, Reasoning: poisonReasoning,
			DecisionTrace: json.RawMessage(`{"raw":"` + poisonReasoning + `"}`),
		},
		{
			SourceSystem: "epic_labs", SourceCode: "NOTIFY_HIGH_2", TargetSystem: "http://loinc.org",
			SuggestedCode: "2339-0", Confidence: 0.93, ExpiresAt: &future,
		},
		{
			SourceSystem: "epic_labs", SourceCode: "NOTIFY_LOW_1", TargetSystem: "http://loinc.org",
			SuggestedCode: "1558-6", Confidence: 0.55, ExpiresAt: &future,
		},
		{
			SourceSystem: "epic_labs", SourceCode: "NOTIFY_LOW_2", TargetSystem: "http://loinc.org",
			SuggestedCode: "4548-4", Confidence: 0.89, ExpiresAt: &future,
		},
	}
	for _, row := range rows {
		if err := store.CreatePendingAutoroute(ctx, row); err != nil {
			t.Fatalf("create %s: %v", row.SourceCode, err)
		}
	}

	received := make(chan []byte, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		received <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sink, err := autoroute.NewWebhookSink(server.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("NewWebhookSink: %v", err)
	}
	notifier, err := autoroute.NewReviewNotifier(autoroute.ReviewNotifierConfig{
		Store:         store,
		Sink:          sink,
		Interval:      50 * time.Millisecond,
		MinConfidence: 0.90,
	})
	if err != nil {
		t.Fatalf("NewReviewNotifier: %v", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- notifier.Run(runCtx) }()

	var body []byte
	select {
	case body = <-received:
	case <-time.After(15 * time.Second):
		t.Fatal("webhook never received a review notification")
	}

	var notification autoroute.ReviewNotification
	if err := json.Unmarshal(body, &notification); err != nil {
		t.Fatalf("decode notification: %v", err)
	}
	if notification.Event != autoroute.ReviewEventName {
		t.Errorf("event = %q, want %q", notification.Event, autoroute.ReviewEventName)
	}

	delivered := map[string]bool{}
	for _, item := range notification.Items {
		delivered[item.SourceCode] = true
	}
	for _, code := range []string{"NOTIFY_HIGH_1", "NOTIFY_HIGH_2"} {
		if !delivered[code] {
			t.Errorf("%s (above threshold) was not delivered; got %v", code, delivered)
		}
	}
	for _, code := range []string{"NOTIFY_LOW_1", "NOTIFY_LOW_2"} {
		if delivered[code] {
			t.Errorf("%s (below threshold) was delivered", code)
		}
	}
	if notification.EligibleCount != 2 {
		t.Errorf("eligible_count = %d, want 2", notification.EligibleCount)
	}

	// PHI minimality against a row that really carries free text in Postgres.
	payload := string(body)
	for _, forbidden := range []string{poisonReasoning, poisonDisplay, "reasoning", "decision_trace", "source_display"} {
		if strings.Contains(payload, forbidden) {
			t.Errorf("notification payload leaked %q: %s", forbidden, payload)
		}
	}

	// Nothing new on the next tick means no repeat page.
	select {
	case extra := <-received:
		var repeat autoroute.ReviewNotification
		if err := json.Unmarshal(extra, &repeat); err == nil && repeat.NewCount > 0 {
			t.Errorf("received a repeat notification with %d new items; de-duplication failed", repeat.NewCount)
		}
	case <-time.After(500 * time.Millisecond):
		// Expected: silence once every eligible row has been notified.
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run after cancel = %v, want nil", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("notifier did not stop after cancellation")
	}
}

// TestReviewNotifier_HangingWebhookDoesNotSlowCreation proves the isolation
// requirement against the real store: with the notifier's webhook wedged, the
// pending-autoroute creation path keeps succeeding at normal latency.
func TestReviewNotifier_HangingWebhookDoesNotSlowCreation(t *testing.T) {
	store, ctx := newNotifyTestStore(t)

	release := make(chan struct{})
	var mu sync.Mutex
	hits := 0
	hung := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		select {
		case hung <- struct{}{}:
		default:
		}
		// The handler must always be able to exit: httptest.Server.Close waits
		// for outstanding requests.
		select {
		case <-release:
		case <-r.Context().Done():
		case <-time.After(30 * time.Second):
		}
		w.WriteHeader(http.StatusOK)
	}))
	// Registered after Close so it runs first: release the handler, then close.
	defer server.Close()
	defer close(release)

	sink, err := autoroute.NewWebhookSink(server.URL, 30*time.Second)
	if err != nil {
		t.Fatalf("NewWebhookSink: %v", err)
	}
	notifier, err := autoroute.NewReviewNotifier(autoroute.ReviewNotifierConfig{
		Store:         store,
		Sink:          sink,
		Interval:      20 * time.Millisecond,
		MinConfidence: 0.90,
		QueueSize:     1,
	})
	if err != nil {
		t.Fatalf("NewReviewNotifier: %v", err)
	}

	future := time.Now().Add(24 * time.Hour)
	seed := &termdb.PendingAutoroute{
		SourceSystem: "epic_labs", SourceCode: "HANG_SEED", TargetSystem: "http://loinc.org",
		SuggestedCode: "2345-7", Confidence: 0.99, ExpiresAt: &future,
	}
	if err := store.CreatePendingAutoroute(ctx, seed); err != nil {
		t.Fatalf("create seed row: %v", err)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- notifier.Run(runCtx) }()

	// Wait until the webhook is actually wedged before measuring.
	select {
	case <-hung:
	case <-time.After(15 * time.Second):
		t.Fatal("webhook was never called; the hang scenario never started")
	}

	// Creation must keep succeeding promptly while the webhook hangs.
	var worst time.Duration
	for i := 0; i < 20; i++ {
		row := &termdb.PendingAutoroute{
			SourceSystem: "epic_labs",
			SourceCode:   "HANG_CREATE_" + time.Now().Format("150405.000000000"),
			TargetSystem: "http://loinc.org", SuggestedCode: "2345-7",
			Confidence: 0.99, ExpiresAt: &future,
		}
		started := time.Now()
		if err := store.CreatePendingAutoroute(ctx, row); err != nil {
			t.Fatalf("create while webhook hung (iteration %d): %v", i, err)
		}
		if elapsed := time.Since(started); elapsed > worst {
			worst = elapsed
		}
	}
	// A blocked webhook has a 30s client timeout; anything anywhere near that
	// would mean creation is coupled to dispatch.
	if worst > 2*time.Second {
		t.Errorf("slowest CreatePendingAutoroute while the webhook hung = %s, want well under the webhook timeout", worst)
	}

	// The notifier must still shut down cleanly out from under the hung request.
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run after cancel = %v, want nil", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("notifier did not stop while its webhook was hung")
	}

	mu.Lock()
	defer mu.Unlock()
	if hits == 0 {
		t.Error("webhook was never called; the isolation assertion proved nothing")
	}
}
