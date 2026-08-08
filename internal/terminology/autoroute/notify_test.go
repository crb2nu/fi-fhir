package autoroute

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

// stubLister returns scripted pending autoroutes and records the filter used.
type stubLister struct {
	mu     sync.Mutex
	rows   []*db.PendingAutoroute
	total  int
	err    error
	calls  int
	filter db.ListPendingAutoroutesFilter
	notify chan struct{}
}

func (s *stubLister) ListPendingAutoroutes(_ context.Context, filter db.ListPendingAutoroutesFilter) ([]*db.PendingAutoroute, int, error) {
	s.mu.Lock()
	s.calls++
	s.filter = filter
	rows, total, err := s.rows, s.total, s.err
	notify := s.notify
	s.mu.Unlock()

	if notify != nil {
		select {
		case notify <- struct{}{}:
		default:
		}
	}
	if err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (s *stubLister) setRows(rows []*db.PendingAutoroute) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rows = rows
}

func (s *stubLister) lastFilter() db.ListPendingAutoroutesFilter {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.filter
}

// recordingSink captures published notifications.
type recordingSink struct {
	mu            sync.Mutex
	published     []ReviewNotification
	err           error
	block         chan struct{}
	notify        chan struct{}
	publishCalled int
}

func (r *recordingSink) Publish(ctx context.Context, notification ReviewNotification) error {
	r.mu.Lock()
	r.publishCalled++
	block, err := r.block, r.err
	notify := r.notify
	r.published = append(r.published, notification)
	r.mu.Unlock()

	if notify != nil {
		select {
		case notify <- struct{}{}:
		default:
		}
	}
	if block != nil {
		select {
		case <-block:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func (r *recordingSink) all() []ReviewNotification {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ReviewNotification, len(r.published))
	copy(out, r.published)
	return out
}

func pending(id int64, code string, confidence float64) *db.PendingAutoroute {
	return &db.PendingAutoroute{
		ID:            id,
		SourceSystem:  "epic_labs",
		SourceCode:    code,
		TargetSystem:  "http://loinc.org",
		SuggestedCode: "1234-5",
		Confidence:    confidence,
		Status:        db.StatusPending,
		CreatedAt:     time.Now().UTC(),
	}
}

// -----------------------------------------------------------------------------
// Construction
// -----------------------------------------------------------------------------

func TestNewReviewNotifier_RejectsInvalidConfig(t *testing.T) {
	valid := func() ReviewNotifierConfig {
		return ReviewNotifierConfig{
			Store:         &stubLister{},
			Sink:          &recordingSink{},
			Interval:      time.Minute,
			MinConfidence: 0.9,
		}
	}
	tests := []struct {
		name  string
		mutit func(*ReviewNotifierConfig)
	}{
		{"nil store", func(c *ReviewNotifierConfig) { c.Store = nil }},
		{"nil sink", func(c *ReviewNotifierConfig) { c.Sink = nil }},
		{"zero interval", func(c *ReviewNotifierConfig) { c.Interval = 0 }},
		{"negative interval", func(c *ReviewNotifierConfig) { c.Interval = -time.Second }},
		{"confidence below range", func(c *ReviewNotifierConfig) { c.MinConfidence = -0.1 }},
		{"confidence above range", func(c *ReviewNotifierConfig) { c.MinConfidence = 1.1 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid()
			tt.mutit(&cfg)
			notifier, err := NewReviewNotifier(cfg)
			if err == nil {
				t.Fatalf("NewReviewNotifier(%s) = nil error, want error", tt.name)
			}
			if !errors.Is(err, ErrNotifierUnavailable) {
				t.Errorf("error = %v, want ErrNotifierUnavailable", err)
			}
			if notifier != nil {
				t.Errorf("notifier = %v, want nil on error", notifier)
			}
		})
	}
}

func TestNewReviewNotifier_Defaults(t *testing.T) {
	notifier, err := NewReviewNotifier(ReviewNotifierConfig{
		Store:         &stubLister{},
		Sink:          &recordingSink{},
		Interval:      30 * time.Second,
		MinConfidence: 0.85,
	})
	if err != nil {
		t.Fatalf("NewReviewNotifier failed: %v", err)
	}
	if got := notifier.Interval(); got != 30*time.Second {
		t.Errorf("Interval() = %s, want 30s", got)
	}
	if got := notifier.MinConfidence(); got != 0.85 {
		t.Errorf("MinConfidence() = %v, want 0.85", got)
	}
	if notifier.scanLimit != DefaultNotifyScanLimit {
		t.Errorf("scanLimit = %d, want %d", notifier.scanLimit, DefaultNotifyScanLimit)
	}
	if cap(notifier.queue) != defaultNotifyQueueSize {
		t.Errorf("queue capacity = %d, want %d", cap(notifier.queue), defaultNotifyQueueSize)
	}
}

func TestReviewNotifier_NilReceiverAndContext(t *testing.T) {
	var notifier *ReviewNotifier
	if got := notifier.Interval(); got != 0 {
		t.Errorf("nil Interval() = %s, want 0", got)
	}
	if got := notifier.MinConfidence(); got != 0 {
		t.Errorf("nil MinConfidence() = %v, want 0", got)
	}
	if notifier.Notify(ReviewNotification{}) {
		t.Error("nil Notify() = true, want false")
	}
	if _, err := notifier.ScanOnce(context.Background()); !errors.Is(err, ErrNotifierUnavailable) {
		t.Errorf("nil ScanOnce = %v, want ErrNotifierUnavailable", err)
	}
	if err := notifier.Run(context.Background()); !errors.Is(err, ErrNotifierUnavailable) {
		t.Errorf("nil Run = %v, want ErrNotifierUnavailable", err)
	}

	valid, err := NewReviewNotifier(ReviewNotifierConfig{
		Store:    &stubLister{},
		Sink:     &recordingSink{},
		Interval: time.Minute,
	})
	if err != nil {
		t.Fatalf("NewReviewNotifier failed: %v", err)
	}
	// Deliberately passing a nil context to prove the guard.
	if err := valid.Run(nil); !errors.Is(err, ErrNotifierUnavailable) {
		t.Errorf("Run(nil ctx) = %v, want ErrNotifierUnavailable", err)
	}
}

// -----------------------------------------------------------------------------
// Threshold filtering
// -----------------------------------------------------------------------------

func TestReviewNotifier_ScanOnce_FiltersByConfidence(t *testing.T) {
	// The store filter is asserted separately; these rows prove the notifier
	// re-checks rather than trusting whatever the store returns.
	store := &stubLister{rows: []*db.PendingAutoroute{
		pending(1, "HIGH1", 0.95),
		pending(2, "AT_THRESHOLD", 0.90),
		pending(3, "LOW1", 0.55),
		pending(4, "JUST_BELOW", 0.8999),
	}}
	sink := &recordingSink{}
	notifier, err := NewReviewNotifier(ReviewNotifierConfig{
		Store:         store,
		Sink:          sink,
		Interval:      time.Minute,
		MinConfidence: 0.90,
	})
	if err != nil {
		t.Fatalf("NewReviewNotifier failed: %v", err)
	}

	result, err := notifier.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("ScanOnce failed: %v", err)
	}
	if result.Eligible != 2 {
		t.Errorf("Eligible = %d, want 2", result.Eligible)
	}
	if result.New != 2 {
		t.Errorf("New = %d, want 2", result.New)
	}
	if result.Queued != 1 {
		t.Errorf("Queued = %d, want 1", result.Queued)
	}

	// The scan pushes the threshold to the store too, so a large backlog does
	// not get read into the process just to be discarded.
	filter := store.lastFilter()
	if filter.Status != db.StatusPending {
		t.Errorf("filter.Status = %q, want pending", filter.Status)
	}
	if filter.MinConfidence == nil || *filter.MinConfidence != 0.90 {
		t.Errorf("filter.MinConfidence = %v, want 0.90", filter.MinConfidence)
	}
	if filter.Limit != DefaultNotifyScanLimit {
		t.Errorf("filter.Limit = %d, want %d", filter.Limit, DefaultNotifyScanLimit)
	}

	notification := <-notifier.queue
	codes := []string{}
	for _, item := range notification.Items {
		codes = append(codes, item.SourceCode)
	}
	want := []string{"HIGH1", "AT_THRESHOLD"}
	if len(codes) != len(want) {
		t.Fatalf("notified codes = %v, want %v", codes, want)
	}
	for i := range want {
		if codes[i] != want[i] {
			t.Errorf("notified codes = %v, want %v (confidence-descending)", codes, want)
			break
		}
	}
	if notification.EligibleCount != 2 {
		t.Errorf("EligibleCount = %d, want 2", notification.EligibleCount)
	}
	if notification.MinConfidence != 0.90 {
		t.Errorf("MinConfidence = %v, want 0.90", notification.MinConfidence)
	}
	if notification.Event != ReviewEventName {
		t.Errorf("Event = %q, want %q", notification.Event, ReviewEventName)
	}
}

func TestReviewNotifier_ScanOnce_SilentWhenNothingEligible(t *testing.T) {
	store := &stubLister{rows: []*db.PendingAutoroute{pending(1, "LOW", 0.2)}}
	notifier, err := NewReviewNotifier(ReviewNotifierConfig{
		Store:         store,
		Sink:          &recordingSink{},
		Interval:      time.Minute,
		MinConfidence: 0.9,
	})
	if err != nil {
		t.Fatalf("NewReviewNotifier failed: %v", err)
	}

	result, err := notifier.ScanOnce(context.Background())
	if err != nil {
		t.Fatalf("ScanOnce failed: %v", err)
	}
	if result.Eligible != 0 || result.Queued != 0 || result.Dropped != 0 {
		t.Errorf("result = %+v, want no eligible rows and no dispatch", result)
	}
	if len(notifier.queue) != 0 {
		t.Errorf("queue length = %d, want 0", len(notifier.queue))
	}
}

func TestReviewNotifier_ScanOnce_DeduplicatesAcrossScans(t *testing.T) {
	store := &stubLister{rows: []*db.PendingAutoroute{pending(1, "FIRST", 0.95)}}
	notifier, err := NewReviewNotifier(ReviewNotifierConfig{
		Store:         store,
		Sink:          &recordingSink{},
		Interval:      time.Minute,
		MinConfidence: 0.9,
	})
	if err != nil {
		t.Fatalf("NewReviewNotifier failed: %v", err)
	}
	ctx := context.Background()

	first, err := notifier.ScanOnce(ctx)
	if err != nil {
		t.Fatalf("first ScanOnce failed: %v", err)
	}
	if first.New != 1 || first.Queued != 1 {
		t.Fatalf("first scan = %+v, want New=1 Queued=1", first)
	}

	// Same row still pending: eligible, but not new, so no second page.
	second, err := notifier.ScanOnce(ctx)
	if err != nil {
		t.Fatalf("second ScanOnce failed: %v", err)
	}
	if second.Eligible != 1 {
		t.Errorf("second Eligible = %d, want 1", second.Eligible)
	}
	if second.New != 0 || second.Queued != 0 {
		t.Errorf("second scan = %+v, want New=0 Queued=0 (already notified)", second)
	}

	// A genuinely new row is notified, and only it.
	store.setRows([]*db.PendingAutoroute{pending(1, "FIRST", 0.95), pending(2, "SECOND", 0.93)})
	third, err := notifier.ScanOnce(ctx)
	if err != nil {
		t.Fatalf("third ScanOnce failed: %v", err)
	}
	if third.New != 1 || third.Queued != 1 {
		t.Fatalf("third scan = %+v, want New=1 Queued=1", third)
	}

	<-notifier.queue // drop the first digest
	notification := <-notifier.queue
	if len(notification.Items) != 1 || notification.Items[0].SourceCode != "SECOND" {
		t.Errorf("second digest items = %+v, want only SECOND", notification.Items)
	}
	if notification.EligibleCount != 2 {
		t.Errorf("EligibleCount = %d, want 2 (whole backlog)", notification.EligibleCount)
	}
}

func TestReviewNotifier_ScanOnce_WrapsStoreError(t *testing.T) {
	storeErr := errors.New("connection refused")
	notifier, err := NewReviewNotifier(ReviewNotifierConfig{
		Store:         &stubLister{err: storeErr},
		Sink:          &recordingSink{},
		Interval:      time.Minute,
		MinConfidence: 0.9,
	})
	if err != nil {
		t.Fatalf("NewReviewNotifier failed: %v", err)
	}

	result, err := notifier.ScanOnce(context.Background())
	if err == nil {
		t.Fatal("ScanOnce = nil error, want error")
	}
	if !errors.Is(err, storeErr) {
		t.Errorf("error = %v, want wrapped %v", err, storeErr)
	}
	if result.Queued != 0 {
		t.Errorf("Queued = %d, want 0 on failure", result.Queued)
	}
}

// -----------------------------------------------------------------------------
// Non-blocking dispatch
// -----------------------------------------------------------------------------

// Notify is the entry point any producer (including the pending-autoroute
// creation path) would use. It must return immediately even when the sink is
// wedged, and drop rather than block once the bounded queue fills.
func TestReviewNotifier_Notify_NeverBlocks(t *testing.T) {
	notifier, err := NewReviewNotifier(ReviewNotifierConfig{
		Store:     &stubLister{},
		Sink:      &recordingSink{},
		Interval:  time.Minute,
		QueueSize: 2,
	})
	if err != nil {
		t.Fatalf("NewReviewNotifier failed: %v", err)
	}

	started := time.Now()
	queued, dropped := 0, 0
	for i := 0; i < 50; i++ {
		if notifier.Notify(ReviewNotification{Event: ReviewEventName}) {
			queued++
		} else {
			dropped++
		}
	}
	elapsed := time.Since(started)

	if queued != 2 {
		t.Errorf("queued = %d, want 2 (bounded queue)", queued)
	}
	if dropped != 48 {
		t.Errorf("dropped = %d, want 48", dropped)
	}
	if elapsed > time.Second {
		t.Errorf("50 Notify calls took %s, want effectively instant", elapsed)
	}
}

// A hanging webhook must not stall the producer. The digest scan is the shipped
// producer; this proves that with the sink blocked and the queue full, the scan
// still returns promptly and reports a drop rather than an error.
func TestReviewNotifier_HangingSinkDoesNotBlockProducer(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	publishing := make(chan struct{}, 8)

	store := &stubLister{rows: []*db.PendingAutoroute{pending(1, "HANG1", 0.99)}}
	sink := &recordingSink{block: release, notify: publishing}
	notifier, err := NewReviewNotifier(ReviewNotifierConfig{
		Store:         store,
		Sink:          sink,
		Interval:      time.Hour,
		MinConfidence: 0.9,
		QueueSize:     1,
	})
	if err != nil {
		t.Fatalf("NewReviewNotifier failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runDone := make(chan error, 1)
	go func() { runDone <- notifier.Run(ctx) }()

	// Wait until the dispatcher is wedged inside Publish.
	select {
	case <-publishing:
	case <-time.After(5 * time.Second):
		t.Fatal("sink was never called")
	}

	// With the dispatcher stuck and the queue about to fill, further scans must
	// still complete quickly.
	for i := 2; i <= 12; i++ {
		store.setRows([]*db.PendingAutoroute{pending(int64(i), "HANG", 0.99)})
		scanStart := time.Now()
		result, scanErr := notifier.ScanOnce(ctx)
		if scanErr != nil {
			t.Fatalf("ScanOnce %d failed: %v", i, scanErr)
		}
		if elapsed := time.Since(scanStart); elapsed > 2*time.Second {
			t.Fatalf("ScanOnce %d took %s while the webhook hung; dispatch is blocking the producer", i, elapsed)
		}
		if result.Queued+result.Dropped != 1 {
			t.Fatalf("ScanOnce %d = %+v, want exactly one queued or dropped", i, result)
		}
	}

	// At least one scan must have been dropped: the queue is size 1 and the only
	// consumer is blocked.
	dropSeen := false
	store.setRows([]*db.PendingAutoroute{pending(99, "HANG_LAST", 0.99)})
	for i := 0; i < 5; i++ {
		result, scanErr := notifier.ScanOnce(ctx)
		if scanErr != nil {
			t.Fatalf("drop-probe ScanOnce failed: %v", scanErr)
		}
		if result.Dropped == 1 {
			dropSeen = true
			break
		}
		store.setRows([]*db.PendingAutoroute{pending(int64(100+i), "HANG_LAST", 0.99)})
	}
	if !dropSeen {
		t.Error("no notification was dropped while the webhook hung; the queue is not bounded")
	}

	cancel()
	select {
	case err := <-runDone:
		if err != nil {
			t.Errorf("Run after cancel = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation while the webhook hung")
	}
}

// -----------------------------------------------------------------------------
// Run loop
// -----------------------------------------------------------------------------

func TestReviewNotifier_Run_ScansImmediatelyAndDelivers(t *testing.T) {
	delivered := make(chan struct{}, 4)
	store := &stubLister{rows: []*db.PendingAutoroute{pending(1, "RUN1", 0.95)}}
	sink := &recordingSink{notify: delivered}

	var mu sync.Mutex
	var deliveries []DeliveryResult
	notifier, err := NewReviewNotifier(ReviewNotifierConfig{
		Store:         store,
		Sink:          sink,
		Interval:      5 * time.Millisecond,
		MinConfidence: 0.9,
		ObserveDelivery: func(result DeliveryResult, err error) {
			if err != nil {
				return
			}
			mu.Lock()
			deliveries = append(deliveries, result)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("NewReviewNotifier failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- notifier.Run(ctx) }()

	select {
	case <-delivered:
	case <-time.After(5 * time.Second):
		t.Fatal("no notification delivered")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run after cancel = %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	published := sink.all()
	if len(published) == 0 {
		t.Fatal("sink received nothing")
	}
	if published[0].Items[0].SourceCode != "RUN1" {
		t.Errorf("first published item = %q, want RUN1", published[0].Items[0].SourceCode)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(deliveries) == 0 {
		t.Fatal("no delivery observed")
	}
	if deliveries[0].Items != 1 {
		t.Errorf("delivery items = %d, want 1", deliveries[0].Items)
	}
}

// A failing store must not stop the loop: serve keeps running and the next tick
// retries.
func TestReviewNotifier_Run_ContinuesAfterStoreError(t *testing.T) {
	scanned := make(chan struct{}, 8)
	store := &stubLister{err: errors.New("transient failure"), notify: scanned}

	var mu sync.Mutex
	var observedErrs int
	notifier, err := NewReviewNotifier(ReviewNotifierConfig{
		Store:         store,
		Sink:          &recordingSink{},
		Interval:      5 * time.Millisecond,
		MinConfidence: 0.9,
		Observe: func(_ NotifyResult, err error) {
			if err != nil {
				mu.Lock()
				observedErrs++
				mu.Unlock()
			}
		},
	})
	if err != nil {
		t.Fatalf("NewReviewNotifier failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- notifier.Run(ctx) }()

	for i := 0; i < 3; i++ {
		select {
		case <-scanned:
		case <-time.After(5 * time.Second):
			t.Fatalf("scan iteration %d did not run after prior failure", i+1)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run = %v, want nil despite store failures", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	mu.Lock()
	defer mu.Unlock()
	if observedErrs < 3 {
		t.Errorf("observed errors = %d, want >= 3 (every failure reported)", observedErrs)
	}
}

// A failing webhook is a warning, not a component failure.
func TestReviewNotifier_Run_ContinuesAfterSinkError(t *testing.T) {
	attempted := make(chan struct{}, 8)
	store := &stubLister{notify: make(chan struct{}, 8)}
	sink := &recordingSink{err: errors.New("webhook 500"), notify: attempted}

	var mu sync.Mutex
	var deliveryErrs int
	notifier, err := NewReviewNotifier(ReviewNotifierConfig{
		Store:         store,
		Sink:          sink,
		Interval:      5 * time.Millisecond,
		MinConfidence: 0.9,
		ObserveDelivery: func(_ DeliveryResult, err error) {
			if err == nil {
				return
			}
			mu.Lock()
			deliveryErrs++
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("NewReviewNotifier failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- notifier.Run(ctx) }()

	// Feed a distinct new row per iteration so de-duplication does not silence
	// the loop while we watch the sink fail.
	for i := 1; i <= 3; i++ {
		store.setRows([]*db.PendingAutoroute{pending(int64(i), "ERR", 0.99)})
		select {
		case <-attempted:
		case <-time.After(5 * time.Second):
			t.Fatalf("delivery attempt %d never happened", i)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Run = %v, want nil despite webhook failures", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}

	mu.Lock()
	defer mu.Unlock()
	if deliveryErrs == 0 {
		t.Error("no delivery error observed; webhook failures are being swallowed")
	}
}

// -----------------------------------------------------------------------------
// Webhook sink
// -----------------------------------------------------------------------------

func TestNewWebhookSink_RejectsInvalidConfig(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		timeout time.Duration
	}{
		{"empty url", "", time.Second},
		{"no scheme", "example.com/hook", time.Second},
		{"unsupported scheme", "ftp://example.com/hook", time.Second},
		{"no host", "http:///hook", time.Second},
		{"zero timeout", "https://example.com/hook", 0},
		{"negative timeout", "https://example.com/hook", -time.Second},
		{"unparseable", "http://%zz", time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sink, err := NewWebhookSink(tt.url, tt.timeout)
			if err == nil {
				t.Fatalf("NewWebhookSink(%q, %s) = nil error, want error", tt.url, tt.timeout)
			}
			if !errors.Is(err, ErrNotifierUnavailable) {
				t.Errorf("error = %v, want ErrNotifierUnavailable", err)
			}
			if sink != nil {
				t.Errorf("sink = %v, want nil on error", sink)
			}
		})
	}
}

func TestWebhookSink_Publish_PostsJSON(t *testing.T) {
	type received struct {
		method      string
		contentType string
		body        []byte
	}
	got := make(chan received, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- received{method: r.Method, contentType: r.Header.Get("Content-Type"), body: body}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	sink, err := NewWebhookSink(server.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("NewWebhookSink failed: %v", err)
	}

	notification := ReviewNotification{
		Event:         ReviewEventName,
		GeneratedAt:   time.Now().UTC(),
		MinConfidence: 0.9,
		NewCount:      1,
		EligibleCount: 3,
		Items:         []ReviewItem{newReviewItem(pending(42, "A1234", 0.97))},
	}
	if err := sink.Publish(context.Background(), notification); err != nil {
		t.Fatalf("Publish failed: %v", err)
	}

	select {
	case r := <-got:
		if r.method != http.MethodPost {
			t.Errorf("method = %q, want POST", r.method)
		}
		if r.contentType != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.contentType)
		}
		var decoded ReviewNotification
		if err := json.Unmarshal(r.body, &decoded); err != nil {
			t.Fatalf("decoding webhook body: %v", err)
		}
		if decoded.Event != ReviewEventName {
			t.Errorf("event = %q, want %q", decoded.Event, ReviewEventName)
		}
		if decoded.EligibleCount != 3 {
			t.Errorf("eligible_count = %d, want 3", decoded.EligibleCount)
		}
		if len(decoded.Items) != 1 || decoded.Items[0].ID != 42 {
			t.Errorf("items = %+v, want one item with id 42", decoded.Items)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("webhook never received a request")
	}
}

func TestWebhookSink_Publish_RetriesOnceThenFails(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		attempts++
		mu.Unlock()
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	sink, err := NewWebhookSink(server.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("NewWebhookSink failed: %v", err)
	}
	if err := sink.Publish(context.Background(), ReviewNotification{Event: ReviewEventName}); err == nil {
		t.Fatal("Publish = nil error, want error on 500")
	}

	mu.Lock()
	defer mu.Unlock()
	if attempts != webhookMaxAttempts {
		t.Errorf("attempts = %d, want %d (one delivery plus one bounded retry)", attempts, webhookMaxAttempts)
	}
}

func TestWebhookSink_Publish_SucceedsOnRetry(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		attempts++
		current := attempts
		mu.Unlock()
		if current == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sink, err := NewWebhookSink(server.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("NewWebhookSink failed: %v", err)
	}
	if err := sink.Publish(context.Background(), ReviewNotification{Event: ReviewEventName}); err != nil {
		t.Fatalf("Publish = %v, want success on retry", err)
	}
}

// A hung receiver must fail on the client timeout, not hang forever.
func TestWebhookSink_Publish_BoundedByTimeout(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The handler must always be able to exit: httptest.Server.Close waits
		// for outstanding requests, and a client-side timeout does not reliably
		// cancel r.Context() before then.
		select {
		case <-release:
		case <-r.Context().Done():
		case <-time.After(10 * time.Second):
		}
		w.WriteHeader(http.StatusOK)
	}))
	// Registered after Close so it runs first: release the handler, then close.
	defer server.Close()
	defer close(release)

	sink, err := NewWebhookSink(server.URL, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("NewWebhookSink failed: %v", err)
	}

	started := time.Now()
	if err := sink.Publish(context.Background(), ReviewNotification{Event: ReviewEventName}); err == nil {
		t.Fatal("Publish = nil error, want timeout error")
	}
	// Two attempts plus the retry delay; generous ceiling for slow CI.
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Errorf("Publish took %s, want bounded by the client timeout", elapsed)
	}
}

func TestWebhookSink_NilReceiverAndContext(t *testing.T) {
	var sink *WebhookSink
	if err := sink.Publish(context.Background(), ReviewNotification{}); !errors.Is(err, ErrNotifierUnavailable) {
		t.Errorf("nil Publish = %v, want ErrNotifierUnavailable", err)
	}

	valid, err := NewWebhookSink("https://example.invalid/hook", time.Second)
	if err != nil {
		t.Fatalf("NewWebhookSink failed: %v", err)
	}
	// Deliberately passing a nil context to prove the guard.
	if err := valid.Publish(nil, ReviewNotification{}); !errors.Is(err, ErrNotifierUnavailable) {
		t.Errorf("Publish(nil ctx) = %v, want ErrNotifierUnavailable", err)
	}
}

// -----------------------------------------------------------------------------
// PHI minimality
// -----------------------------------------------------------------------------

// The webhook is an untrusted egress point. Every free-text or LLM-authored
// field on a pending autoroute can quote source message content, so none of
// them may appear on the wire.
func TestReviewNotification_PayloadIsPHIMinimal(t *testing.T) {
	poison := map[string]string{
		"source_display":    "PATIENT JANE DOE DOB 1954-03-02 GLUCOSE",
		"suggested_display": "MRN 00998877 serum glucose",
		"reasoning":         "The note says the patient at 12 Oak St has diabetes",
		"rejection_reason":  "reviewer said this is the wrong patient",
		"reviewed_by":       "dr.house@example.org",
		"decision_trace":    "raw-hl7-obx-segment-payload",
		"alternates":        "alternate-candidate-free-text",
	}

	row := pending(7, "GLU", 0.99)
	row.SourceDisplay = poison["source_display"]
	row.SuggestedDisplay = poison["suggested_display"]
	row.Reasoning = poison["reasoning"]
	row.RejectionReason = poison["rejection_reason"]
	row.ReviewedBy = poison["reviewed_by"]
	row.DecisionTrace = json.RawMessage(`{"note":"` + poison["decision_trace"] + `"}`)
	row.Alternates = json.RawMessage(`[{"note":"` + poison["alternates"] + `"}]`)
	row.Equivalence = "equivalent"

	notification := ReviewNotification{
		Event:         ReviewEventName,
		GeneratedAt:   time.Now().UTC(),
		MinConfidence: 0.9,
		NewCount:      1,
		EligibleCount: 1,
		Items:         []ReviewItem{newReviewItem(row)},
	}
	encoded, err := json.Marshal(notification)
	if err != nil {
		t.Fatalf("marshalling notification: %v", err)
	}
	payload := string(encoded)

	for field, value := range poison {
		if strings.Contains(payload, value) {
			t.Errorf("notification payload leaked %s: %s", field, payload)
		}
		if strings.Contains(payload, `"`+field+`"`) {
			t.Errorf("notification payload carries a %q key: %s", field, payload)
		}
	}

	// The coded identity a reviewer needs must survive.
	for _, want := range []string{`"id":7`, `"source_system":"epic_labs"`, `"source_code":"GLU"`,
		`"target_system":"http://loinc.org"`, `"suggested_code":"1234-5"`, `"confidence":0.99`,
		`"equivalence":"equivalent"`} {
		if !strings.Contains(payload, want) {
			t.Errorf("notification payload missing %s: %s", want, payload)
		}
	}
}

// -----------------------------------------------------------------------------
// End-to-end over HTTP
// -----------------------------------------------------------------------------

// Threshold filtering all the way to a real HTTP receiver: only the
// above-threshold rows may produce a delivery.
func TestReviewNotifier_EndToEnd_OnlyHighConfidenceReachesWebhook(t *testing.T) {
	bodies := make(chan []byte, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies <- body
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	sink, err := NewWebhookSink(server.URL, 5*time.Second)
	if err != nil {
		t.Fatalf("NewWebhookSink failed: %v", err)
	}

	store := &stubLister{rows: []*db.PendingAutoroute{
		pending(1, "ABOVE_A", 0.97),
		pending(2, "BELOW_A", 0.40),
		pending(3, "ABOVE_B", 0.91),
		pending(4, "BELOW_B", 0.89),
	}}
	notifier, err := NewReviewNotifier(ReviewNotifierConfig{
		Store:         store,
		Sink:          sink,
		Interval:      5 * time.Millisecond,
		MinConfidence: 0.90,
	})
	if err != nil {
		t.Fatalf("NewReviewNotifier failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- notifier.Run(ctx) }()

	var body []byte
	select {
	case body = <-bodies:
	case <-time.After(5 * time.Second):
		t.Fatal("webhook never received a notification")
	}
	cancel()
	<-done

	var decoded ReviewNotification
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatalf("decoding webhook body: %v", err)
	}
	if len(decoded.Items) != 2 {
		t.Fatalf("delivered %d items, want 2 above-threshold items: %+v", len(decoded.Items), decoded.Items)
	}
	delivered := map[string]bool{}
	for _, item := range decoded.Items {
		delivered[item.SourceCode] = true
	}
	for _, code := range []string{"ABOVE_A", "ABOVE_B"} {
		if !delivered[code] {
			t.Errorf("%s was not delivered", code)
		}
	}
	for _, code := range []string{"BELOW_A", "BELOW_B"} {
		if delivered[code] {
			t.Errorf("%s was delivered but is below the threshold", code)
		}
	}
}
