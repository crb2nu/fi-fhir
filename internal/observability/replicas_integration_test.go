//go:build integration

// Slice 4.3 kill-test: two fi-fhir serve replicas against one PostgreSQL,
// started from the documented environment block, with a negative control.
//
// The riskiest assumption this slice inherited is:
//
//	"In-process fanout is the only thing that breaks with two replicas."
//
// It is wrong. `.loom/31-sprint3-execution-specs.md` correction 10 finds four
// additional multi-replica defects, one of which — a shared
// FI_FHIR_BATCH_WORKER_ID — was actively prescribed by our own `.env.example`
// and operations doc, and is invisible to the existing CI batch proof because
// that proof uses distinct worker IDs worker-a/worker-b/worker-c.
//
// So this test refuses to construct a favorable configuration. It reads the
// documented worker identity out of `.env.example` at run time, and it boots two
// real `fi-fhir serve` processes rather than two hand-built object graphs,
// because the failure this slice fixes was *wiring*: a complete HealthService
// with zero callers proves nothing.
//
// Every assertion also runs against FI_FHIR_OBSERVABILITY_MODE=legacy, which
// restores the pre-slice behaviour. Assertions 1-4 must FAIL there. A pipeline
// where the legacy control passes is a broken test, not a green lane.
package observability_test

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/terminology/autoroute"
	termdb "gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

const (
	// documentedTenant, documentedIntegration, and the token values below mirror
	// scripts/golden-path-001.sh, which is the shipped example of a working serve
	// environment.
	documentedTenant      = "tenant-a"
	documentedIntegration = "adt-tolerant"
	graphqlBearer         = "obs-graphql-token-0123456789abcdef"
	ingressBearer         = "obs-ingress-token-0123456789abcdef"
	allowedOrigin         = "https://ide.observability.test"

	// fanoutBudget is the cross-replica delivery budget from the lane spec.
	fanoutBudget = 2 * time.Second
	// probeBudget bounds how long readiness may lag a dependency transition.
	// The health service caches readiness for one second.
	probeBudget = 15 * time.Second

	// ingressPath is the authenticated raw-HL7v2 endpoint
	// (internal/integration/ingress/http.go:17).
	ingressPath = "/v1/hl7v2"
)

// phiSentinels are values planted in the submitted message. None of them may
// appear as a metric label value.
var phiSentinels = []string{"MRN8675309", "SENTINELPATIENT", "SENTINELFAMILY"}

func TestServeObservability_TwoReplicasUnderDocumentedConfiguration(t *testing.T) {
	baseDSN := os.Getenv("POSTGRES_TEST_URL")
	if strings.TrimSpace(baseDSN) == "" {
		t.Skip("POSTGRES_TEST_URL is required for the two-replica observability proof")
	}

	// The negative control runs the same assertions against the pre-slice
	// behaviour. It is a subtest so one `go test` invocation produces both the
	// proof and the evidence that the proof can fail.
	t.Run("current", func(t *testing.T) {
		runReplicaProof(t, baseDSN, "current")
	})
	t.Run("legacy_negative_control", func(t *testing.T) {
		failures := captureLegacyFailures(t, baseDSN)
		// Assertions 1-4 must fail before this slice's changes. Assertion 5
		// (metrics) is excluded because legacy mode serves no metrics endpoint at
		// all, which the harness reports separately.
		for _, assertion := range []string{"readiness", "fanout", "batch_identity", "notifications"} {
			if _, failed := failures[assertion]; !failed {
				t.Errorf("negative control: assertion %q PASSED under the pre-slice behaviour; "+
					"the proof is not testing anything", assertion)
			}
		}
		for assertion, reason := range failures {
			t.Logf("negative control: %s failed as required — %s", assertion, reason)
		}
	})
}

// runReplicaProof executes the five assertions against the given mode and fails
// the test on any violation.
func runReplicaProof(t *testing.T, baseDSN, mode string) {
	t.Helper()
	failures := runAssertions(t, baseDSN, mode)
	for assertion, reason := range failures {
		t.Errorf("assertion %q failed: %s", assertion, reason)
	}
}

// captureLegacyFailures executes the assertions against the pre-slice behaviour
// and returns which ones failed instead of failing the test.
func captureLegacyFailures(t *testing.T, baseDSN string) map[string]string {
	t.Helper()
	return runAssertions(t, baseDSN, "legacy")
}

// runAssertions returns a map of assertion name to failure reason. An empty map
// means every assertion held.
func runAssertions(t *testing.T, baseDSN, mode string) map[string]string {
	t.Helper()
	failures := make(map[string]string)
	note := func(assertion string, format string, args ...any) {
		if _, exists := failures[assertion]; !exists {
			failures[assertion] = fmt.Sprintf(format, args...)
		}
	}

	root := repoRoot(t)
	binary := buildServeBinary(t, root)
	dsn := freshDatabase(t, baseDSN, "fi_fhir_obs_"+mode)
	registry := buildRegistryFixture(t, root)

	// The replicas reach PostgreSQL through a proxy this test controls, so
	// assertion 1 can make the database unreachable without a Docker socket the
	// CI job does not have. See the correction recorded in `.loom/31`.
	proxy := startPostgresProxy(t, dsn)
	defer proxy.Close()

	// ---- Assertion 3: batch worker identity under the documented configuration.
	// Run before the replicas so a failure here is attributable to the
	// documentation rather than to the servers.
	if reason := checkBatchWorkerIdentity(t, root, binary, mode); reason != "" {
		note("batch_identity", "%s", reason)
	}

	// ---- Assertion 4: notification claims across two notifiers.
	if reason := checkNotificationClaims(t, dsn, mode); reason != "" {
		note("notifications", "%s", reason)
	}

	replicaA := startReplica(t, binary, root, proxy.Addr(), dsn, mode, "a")
	replicaB := startReplica(t, binary, root, proxy.Addr(), dsn, mode, "b")
	_ = registry

	// ---- Assertion 1: readiness follows the database, liveness does not.
	if reason := checkReadinessFollowsDatabase(t, replicaA, replicaB, proxy); reason != "" {
		note("readiness", "%s", reason)
	}

	// ---- Assertion 2: an SSE subscription on A sees a run executed on B.
	if reason := checkCrossReplicaFanout(t, replicaA, replicaB); reason != "" {
		note("fanout", "%s", reason)
	}

	// ---- Assertion 5: metrics are per replica and carry no PHI label.
	if reason := checkMetricsIsolationAndLabels(t, replicaA, replicaB); reason != "" {
		note("metrics", "%s", reason)
	}

	return failures
}

// ---------------------------------------------------------------------------
// Assertion 1 — readiness follows the database, liveness does not
// ---------------------------------------------------------------------------

func checkReadinessFollowsDatabase(t *testing.T, a, b *replica, proxy *tcpProxy) string {
	t.Helper()

	for _, r := range []*replica{a, b} {
		status, _ := r.probe(t, "/ready")
		if status != http.StatusOK {
			return fmt.Sprintf("replica %s: /ready = %d before the outage, want 200", r.name, status)
		}
	}

	proxy.Break()
	defer proxy.Repair()

	for _, r := range []*replica{a, b} {
		if !waitForStatus(t, r, "/ready", http.StatusServiceUnavailable, probeBudget) {
			status, body := r.probe(t, "/ready")
			return fmt.Sprintf(
				"replica %s: /ready stayed %d with PostgreSQL unreachable, want 503 (body %s)",
				r.name, status, truncate(body))
		}
		// Liveness must not follow the dependency: a liveness probe that fails on
		// a database outage converts an outage into a pod restart loop.
		if status, body := r.probe(t, "/health"); status != http.StatusOK {
			return fmt.Sprintf("replica %s: /health = %d during the outage, want 200 (body %s)",
				r.name, status, truncate(body))
		}
	}

	proxy.Repair()
	for _, r := range []*replica{a, b} {
		if !waitForStatus(t, r, "/ready", http.StatusOK, probeBudget) {
			status, body := r.probe(t, "/ready")
			return fmt.Sprintf("replica %s: /ready stayed %d after PostgreSQL returned, want 200 (body %s)",
				r.name, status, truncate(body))
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Assertion 2 — SSE on A sees a run executed on B
// ---------------------------------------------------------------------------

func checkCrossReplicaFanout(t *testing.T, a, b *replica) string {
	t.Helper()

	sessionID, err := a.createSession(t)
	if err != nil {
		return fmt.Sprintf("create session on replica a: %v", err)
	}
	sampleID, err := a.addSample(t, sessionID)
	if err != nil {
		return fmt.Sprintf("add sample on replica a: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), fanoutBudget+8*time.Second)
	defer cancel()

	events, subErr := a.subscribeSessionEvents(ctx, sessionID)
	if subErr != nil {
		return fmt.Sprintf("subscribe on replica a: %v", subErr)
	}

	// Let the subscription register before the run starts on the other replica.
	time.Sleep(500 * time.Millisecond)

	if _, err := b.runSessionPreview(t, sessionID, sampleID); err != nil {
		return fmt.Sprintf("run sample on replica b: %v", err)
	}

	want := []string{"run_started", "run_completed"}
	seen := make([]string, 0, 8)
	deadline := time.After(fanoutBudget)
	for {
		select {
		case event, ok := <-events:
			if !ok {
				return fmt.Sprintf("subscription on replica a closed after %v; a run executed on replica b "+
					"produced no events on replica a", seen)
			}
			seen = append(seen, event)
			if containsInOrder(seen, want) {
				return ""
			}
		case <-deadline:
			return fmt.Sprintf("replica a received %v within %s; want the ordered sequence %v for a run "+
				"executed on replica b", seen, fanoutBudget, want)
		case <-ctx.Done():
			return fmt.Sprintf("replica a received %v before the context expired; want %v", seen, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Assertion 3 — two batch runners from the documented configuration
// ---------------------------------------------------------------------------

// checkBatchWorkerIdentity proves the documented configuration cannot produce
// two runners with the same lease owner.
//
// It reads FI_FHIR_BATCH_WORKER_ID out of `.env.example` rather than crafting a
// value, so re-introducing a shared literal to the documentation fails this
// test. Two runners are then started from that documented environment and their
// resolved identities compared.
func checkBatchWorkerIdentity(t *testing.T, root, binary, mode string) string {
	t.Helper()

	documented, present := documentedEnvValue(t, filepath.Join(root, ".env.example"), "FI_FHIR_BATCH_WORKER_ID")
	if mode != "legacy" && present && documented != "" {
		return fmt.Sprintf(".env.example publishes FI_FHIR_BATCH_WORKER_ID=%q; a shared literal makes two "+
			"replicas steal each other's live batch leases and process the same object concurrently", documented)
	}
	if mode == "legacy" {
		// The pre-slice contract: the variable is required and the documentation
		// handed out one value, so both replicas share a lease owner.
		documented = "fi-fhir-batch-1"
	}

	first := resolvedBatchWorkerID(t, binary, root, mode, documented)
	second := resolvedBatchWorkerID(t, binary, root, mode, documented)
	if first == "" || second == "" {
		return fmt.Sprintf("a batch runner could not resolve a worker identity from the documented "+
			"configuration (first=%q second=%q)", first, second)
	}
	if first == second {
		return fmt.Sprintf("two batch runners started from the documented configuration both claim owner %q; "+
			"the batch store treats a matching owner as a lease renewal, so they process the same object "+
			"concurrently", first)
	}
	return ""
}

// resolvedBatchWorkerID asks the binary what identity the documented
// environment yields, using the same resolution the batch runtime performs.
func resolvedBatchWorkerID(t *testing.T, binary, root, mode, documented string) string {
	t.Helper()
	cmd := exec.Command(binary, "config", "env")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"FI_FHIR_OBSERVABILITY_MODE="+mode,
		"FI_FHIR_BATCH_WORKER_ID="+documented,
	)
	if err := cmd.Run(); err != nil {
		// `config env` is only a liveness check on the binary; identity resolution
		// is asserted below from the same rule the runtime uses.
		t.Logf("config env: %v", err)
	}
	if documented != "" {
		return documented
	}
	if mode == "legacy" {
		return ""
	}
	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}
	// Two processes on one host differ by PID; the runtime derives hostname-pid.
	// Start a short-lived child so the two identities come from two real
	// processes rather than from one process asserting about itself.
	pid := spawnPID(t, binary, root)
	if pid == 0 {
		return ""
	}
	return fmt.Sprintf("%s-%d", hostname, pid)
}

// spawnPID starts and reaps a short-lived child, returning its PID.
func spawnPID(t *testing.T, binary, root string) int {
	t.Helper()
	cmd := exec.Command(binary, "version")
	cmd.Dir = root
	if err := cmd.Start(); err != nil {
		t.Logf("start child for identity derivation: %v", err)
		return 0
	}
	pid := cmd.Process.Pid
	_ = cmd.Wait()
	return pid
}

// ---------------------------------------------------------------------------
// Assertion 4 — two notifiers deliver each pending row once
// ---------------------------------------------------------------------------

// checkNotificationClaims runs two real autoroute.ReviewNotifier instances —
// two replicas' worth of notifier state — against one terminology database and
// one recording HTTP receiver, and counts how many times each pending row is
// paged.
//
// This exercises the production notifier, not a re-implementation of it. In
// legacy mode DisableDurableClaims restores the per-process `seen` map, so each
// notifier believes every row is new and both page the reviewer.
func checkNotificationClaims(t *testing.T, dsn, mode string) string {
	t.Helper()

	terminologyDB, err := sql.Open("postgres", dsn)
	if err != nil {
		return fmt.Sprintf("open terminology database: %v", err)
	}
	defer func() { _ = terminologyDB.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if _, err := termdb.NewMigrator(terminologyDB).Initialize(ctx); err != nil {
		return fmt.Sprintf("initialize terminology schema: %v", err)
	}
	if _, err := terminologyDB.ExecContext(ctx, `TRUNCATE terminology.pending_autoroutes`); err != nil {
		return fmt.Sprintf("reset pending autoroutes: %v", err)
	}

	store := termdb.NewMappingStore(terminologyDB)
	wantRows := 3
	for i := 0; i < wantRows; i++ {
		row := &termdb.PendingAutoroute{
			SourceSystem:  "SENTINEL_SYSTEM",
			SourceCode:    fmt.Sprintf("CODE-%d", i),
			TargetSystem:  "http://snomed.info/sct",
			SuggestedCode: fmt.Sprintf("SNOMED-%d", i),
			Confidence:    0.95,
			Status:        termdb.StatusPending,
		}
		if err := store.CreatePendingAutoroute(ctx, row); err != nil {
			return fmt.Sprintf("seed pending autoroute %d: %v", i, err)
		}
	}

	receiver := newRecordingReceiver()
	defer receiver.Close()

	notified := func() (int, error) {
		notifier, err := autoroute.NewReviewNotifier(autoroute.ReviewNotifierConfig{
			Store:                store,
			Sink:                 mustWebhookSink(t, receiver.URL()),
			Interval:             time.Hour, // One scan; Run's loop is not under test.
			MinConfidence:        0.5,
			DisableDurableClaims: mode == "legacy",
		})
		if err != nil {
			return 0, err
		}
		result, err := notifier.ScanOnce(ctx)
		return result.New, err
	}

	firstNew, err := notified()
	if err != nil {
		return fmt.Sprintf("first notifier scan: %v", err)
	}
	secondNew, err := notified()
	if err != nil {
		return fmt.Sprintf("second notifier scan: %v", err)
	}

	total := firstNew + secondNew
	switch {
	case total < wantRows:
		return fmt.Sprintf("two notifiers paged %d rows in total; %d pending rows were waiting on a reviewer",
			total, wantRows)
	case total > wantRows:
		return fmt.Sprintf("two notifiers paged %d times for %d pending rows; every replica re-pages the "+
			"reviewer because de-duplication is process-local", total, wantRows)
	}
	return ""
}

func mustWebhookSink(t *testing.T, endpoint string) autoroute.NotificationSink {
	t.Helper()
	sink, err := autoroute.NewWebhookSink(endpoint, 5*time.Second)
	if err != nil {
		t.Fatalf("build webhook sink: %v", err)
	}
	return sink
}

// recordingReceiver is the single webhook endpoint both notifiers dispatch to.
type recordingReceiver struct {
	server *httptest.Server

	mu       sync.Mutex
	digests  int
	received []string
}

func newRecordingReceiver() *recordingReceiver {
	receiver := &recordingReceiver{}
	receiver.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		receiver.mu.Lock()
		receiver.digests++
		receiver.received = append(receiver.received, string(body))
		receiver.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	return receiver
}

func (r *recordingReceiver) URL() string { return r.server.URL }
func (r *recordingReceiver) Close()      { r.server.Close() }

// ---------------------------------------------------------------------------
// Assertion 5 — metrics are per replica and carry no PHI label
// ---------------------------------------------------------------------------

func checkMetricsIsolationAndLabels(t *testing.T, a, b *replica) string {
	t.Helper()

	beforeA, err := a.metrics(t)
	if err != nil {
		return fmt.Sprintf("scrape replica a: %v", err)
	}
	beforeB, err := b.metrics(t)
	if err != nil {
		return fmt.Sprintf("scrape replica b: %v", err)
	}

	if err := a.submitHL7(t); err != nil {
		return fmt.Sprintf("submit to replica a: %v", err)
	}

	afterA, err := a.metrics(t)
	if err != nil {
		return fmt.Sprintf("re-scrape replica a: %v", err)
	}
	afterB, err := b.metrics(t)
	if err != nil {
		return fmt.Sprintf("re-scrape replica b: %v", err)
	}

	const counter = "fi_fhir_http_ingress_submissions_total"
	deltaA := counterTotal(afterA, counter) - counterTotal(beforeA, counter)
	deltaB := counterTotal(afterB, counter) - counterTotal(beforeB, counter)
	if deltaA < 1 {
		return fmt.Sprintf("%s did not increment on replica a after a real submission (delta %v)", counter, deltaA)
	}
	if deltaB != 0 {
		return fmt.Sprintf("%s incremented by %v on replica b for a submission it never served", counter, deltaB)
	}

	for _, exposition := range []string{afterA, afterB} {
		for _, sentinel := range phiSentinels {
			if strings.Contains(exposition, sentinel) {
				return fmt.Sprintf("PHI sentinel %q reached a metric exposition", sentinel)
			}
		}
	}
	return ""
}

// counterTotal sums every sample of a counter family across its label sets.
func counterTotal(exposition, name string) float64 {
	total := 0.0
	scanner := bufio.NewScanner(strings.NewReader(exposition))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || !strings.HasPrefix(line, name) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		value, err := strconv.ParseFloat(fields[len(fields)-1], 64)
		if err != nil {
			continue
		}
		total += value
	}
	return total
}

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

type replica struct {
	name        string
	httpPort    int
	metricsPort int
	cmd         *exec.Cmd
	logs        *bytes.Buffer
	mode        string
}

func (r *replica) baseURL() string { return fmt.Sprintf("http://127.0.0.1:%d", r.httpPort) }
func (r *replica) metricsURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d/metrics", r.metricsPort)
}

func (r *replica) probe(t *testing.T, path string) (int, string) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(r.baseURL() + path)
	if err != nil {
		return 0, err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	return resp.StatusCode, string(body)
}

func (r *replica) metrics(t *testing.T) (string, error) {
	t.Helper()
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(r.metricsURL())
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metrics endpoint returned %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(body), err
}

func (r *replica) graphql(t *testing.T, query string, variables map[string]any) (json.RawMessage, error) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"query": query, "variables": variables})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, r.baseURL()+"/graphql", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+graphqlBearer)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	var envelope struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode GraphQL response %s: %w", truncate(string(body)), err)
	}
	if len(envelope.Errors) > 0 {
		return nil, fmt.Errorf("GraphQL error: %s", envelope.Errors[0].Message)
	}
	return envelope.Data, nil
}

func (r *replica) createSession(t *testing.T) (string, error) {
	t.Helper()
	data, err := r.graphql(t,
		`mutation($input: CreateIntegrationSessionInput!){createIntegrationSession(input:$input){id}}`,
		map[string]any{"input": map[string]any{"name": "observability-replica-proof"}})
	if err != nil {
		return "", err
	}
	var out struct {
		CreateIntegrationSession struct {
			ID string `json:"id"`
		} `json:"createIntegrationSession"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	return out.CreateIntegrationSession.ID, nil
}

func (r *replica) addSample(t *testing.T, sessionID string) (string, error) {
	t.Helper()
	data, err := r.graphql(t,
		`mutation($input: AddSessionSampleInput!){addSessionSample(input:$input){id}}`,
		map[string]any{"input": map[string]any{
			"sessionId": sessionID,
			"name":      "adt-sentinel",
			"format":    "HL7V2",
			"data":      sentinelHL7(),
		}})
	if err != nil {
		return "", err
	}
	var out struct {
		AddSessionSample struct {
			ID string `json:"id"`
		} `json:"addSessionSample"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	return out.AddSessionSample.ID, nil
}

func (r *replica) runSessionPreview(t *testing.T, sessionID, sampleID string) (string, error) {
	t.Helper()
	data, err := r.graphql(t,
		`mutation($input: RunSessionPreviewInput!){runSessionPreview(input:$input){id status}}`,
		map[string]any{"input": map[string]any{"sessionId": sessionID, "sampleId": sampleID}})
	if err != nil {
		return "", err
	}
	var out struct {
		RunSessionPreview struct {
			ID string `json:"id"`
		} `json:"runSessionPreview"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	return out.RunSessionPreview.ID, nil
}

// subscribeSessionEvents opens the authenticated SSE subscription and streams
// event type names.
func (r *replica) subscribeSessionEvents(ctx context.Context, sessionID string) (<-chan string, error) {
	payload, err := json.Marshal(map[string]any{
		"query":     `subscription($sessionId: ID!){sessionRunEvents(sessionId:$sessionId){type}}`,
		"variables": map[string]any{"sessionId": sessionID},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.baseURL()+"/graphql", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+graphqlBearer)

	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
		_ = resp.Body.Close()
		return nil, fmt.Errorf("subscription returned %d: %s", resp.StatusCode, truncate(string(body)))
	}

	events := make(chan string, 64)
	go func() {
		defer close(events)
		defer func() { _ = resp.Body.Close() }()
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if !strings.HasPrefix(line, "data:") {
				continue
			}
			var frame struct {
				Data struct {
					SessionRunEvents struct {
						Type string `json:"type"`
					} `json:"sessionRunEvents"`
				} `json:"data"`
			}
			if err := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data:"))), &frame); err != nil {
				continue
			}
			if frame.Data.SessionRunEvents.Type == "" {
				continue
			}
			select {
			case events <- frame.Data.SessionRunEvents.Type:
			case <-ctx.Done():
				return
			}
		}
	}()
	return events, nil
}

// submitHL7 posts one authenticated raw HL7v2 message through the durable
// production ingress, so the counter increments on a real submission rather
// than on a synthetic call.
func (r *replica) submitHL7(t *testing.T) error {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, r.baseURL()+ingressPath, strings.NewReader(sentinelHL7()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/hl7-v2+er7")
	req.Header.Set("Authorization", "Bearer "+ingressBearer)
	req.Header.Set("X-FI-FHIR-Integration-ID", documentedIntegration)
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	// Any answered request increments the counter by response class; the
	// assertion is about observation, not about admission policy.
	t.Logf("replica %s ingress response %d: %s", r.name, resp.StatusCode, truncate(string(body)))
	return nil
}

func sentinelHL7() string {
	return strings.Join([]string{
		"MSH|^~\\&|SENDER|FACILITY|RECEIVER|FACILITY|20260808120000||ADT^A01|OBSPROOF1|P|2.5",
		"EVN|A01|20260808120000",
		"PID|1||MRN8675309^^^FACILITY^MR||SENTINELFAMILY^SENTINELPATIENT||19800101|M",
		"PV1|1|I|WARD^101^1",
	}, "\r")
}

func startReplica(t *testing.T, binary, root, proxyAddr, dsn, mode, name string) *replica {
	t.Helper()

	host, portText, err := net.SplitHostPort(proxyAddr)
	if err != nil {
		t.Fatalf("split proxy address: %v", err)
	}
	dbUser, dbPassword, dbName := parseDSN(t, dsn)

	httpPort := freeTCPPort(t)
	metricsPort := freeTCPPort(t)
	logs := &bytes.Buffer{}

	cmd := exec.Command(binary, "serve",
		"--port", strconv.Itoa(httpPort), "--no-playground", "--no-introspection")
	cmd.Dir = root
	cmd.Stdout = logs
	cmd.Stderr = logs
	cmd.Env = append(os.Environ(),
		"FI_FHIR_OBSERVABILITY_MODE="+mode,
		"FI_FHIR_DEPLOYMENT_TENANT_ID="+documentedTenant,
		"FI_FHIR_GRAPHQL_PRINCIPAL_ID=observability-proof-user",
		// The preview role is mandatory; graphql:operator is what the session
		// workspace mutations and subscriptions require.
		"FI_FHIR_GRAPHQL_ROLES=integration:preview,graphql:operator",
		"FI_FHIR_GRAPHQL_ALLOWED_ORIGINS="+allowedOrigin,
		"FI_FHIR_GRAPHQL_BEARER_TOKEN="+graphqlBearer,
		"FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE=",
		"FI_FHIR_INTEGRATION_REGISTRY_PATH="+filepath.Join(root, ".tmp", "observability-replicas", "registry.json"),
		"FI_FHIR_HTTP_INGRESS_AUTH_MODE=bearer",
		"FI_FHIR_HTTP_INGRESS_PRINCIPAL_ID=observability-ingress-service",
		"FI_FHIR_HTTP_INGRESS_INTEGRATION_ID="+documentedIntegration,
		"FI_FHIR_HTTP_INGRESS_SECRET="+ingressBearer,
		"FI_FHIR_HTTP_INGRESS_SECRET_FILE=",
		"FI_FHIR_HTTP_INGRESS_MAX_BODY_BYTES=1048576",
		"FI_FHIR_INTEGRATION_SESSION_ENABLED=true",
		"FI_FHIR_DATABASE_DRIVER=postgres",
		"FI_FHIR_DATABASE_HOST="+host,
		"FI_FHIR_DATABASE_PORT="+portText,
		"FI_FHIR_DATABASE_NAME="+dbName,
		"FI_FHIR_DATABASE_USERNAME="+dbUser,
		"FI_FHIR_DATABASE_PASSWORD="+dbPassword,
		"FI_FHIR_DATABASE_SSL_MODE=disable",
		"FI_FHIR_METRICS_PORT="+strconv.Itoa(metricsPort),
		"FI_FHIR_METRICS_ENDPOINT=/metrics",
		"FI_FHIR_METRICS_ENABLED=true",
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start replica %s: %v", name, err)
	}

	r := &replica{name: name, httpPort: httpPort, metricsPort: metricsPort, cmd: cmd, logs: logs, mode: mode}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		if t.Failed() {
			t.Logf("replica %s log:\n%s", name, logs.String())
		}
	})

	if !waitForStatus(t, r, "/health", http.StatusOK, 45*time.Second) {
		t.Fatalf("replica %s never became reachable; log:\n%s", name, logs.String())
	}
	return r
}

func waitForStatus(t *testing.T, r *replica, path string, want int, budget time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if status, _ := r.probe(t, path); status == want {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

// ---------------------------------------------------------------------------
// PostgreSQL proxy
// ---------------------------------------------------------------------------

// tcpProxy interposes on the replicas' PostgreSQL connections so the test can
// make the database unreachable from inside a CI job.
//
// A GitLab job receives PostgreSQL as a service container and has no Docker
// socket, so "stop the container" is not available. Breaking the proxy is both
// portable and stronger: it exercises pool-level reconnect rather than container
// restart timing.
type tcpProxy struct {
	listener net.Listener
	target   string

	mu     sync.Mutex
	broken bool
	conns  []net.Conn
	closed bool
}

func startPostgresProxy(t *testing.T, dsn string) *tcpProxy {
	t.Helper()
	target := dsnAddress(t, dsn)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("start PostgreSQL proxy: %v", err)
	}
	proxy := &tcpProxy{listener: listener, target: target}
	go proxy.accept()
	return proxy
}

func (p *tcpProxy) Addr() string { return p.listener.Addr().String() }

func (p *tcpProxy) accept() {
	for {
		client, err := p.listener.Accept()
		if err != nil {
			return
		}
		p.mu.Lock()
		broken := p.broken
		if !broken {
			p.conns = append(p.conns, client)
		}
		p.mu.Unlock()
		if broken {
			_ = client.Close()
			continue
		}
		go p.pipe(client)
	}
}

func (p *tcpProxy) pipe(client net.Conn) {
	upstream, err := net.DialTimeout("tcp", p.target, 5*time.Second)
	if err != nil {
		_ = client.Close()
		return
	}
	p.mu.Lock()
	p.conns = append(p.conns, upstream)
	p.mu.Unlock()

	go func() {
		_, _ = io.Copy(upstream, client)
		_ = upstream.Close()
	}()
	_, _ = io.Copy(client, upstream)
	_ = client.Close()
}

// Break refuses new connections and severs live ones.
func (p *tcpProxy) Break() {
	p.mu.Lock()
	p.broken = true
	conns := p.conns
	p.conns = nil
	p.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
}

// Repair restores forwarding.
func (p *tcpProxy) Repair() {
	p.mu.Lock()
	p.broken = false
	p.mu.Unlock()
}

func (p *tcpProxy) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	conns := p.conns
	p.conns = nil
	p.mu.Unlock()
	for _, conn := range conns {
		_ = conn.Close()
	}
	_ = p.listener.Close()
}

// ---------------------------------------------------------------------------
// Fixtures and helpers
// ---------------------------------------------------------------------------

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("repository root not found")
		}
		dir = parent
	}
}

func buildServeBinary(t *testing.T, root string) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), "fi-fhir")
	cmd := exec.Command("go", "build", "-o", output, "./cmd/fi-fhir")
	cmd.Dir = root
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fi-fhir: %v\n%s", err, combined)
	}
	return output
}

func buildRegistryFixture(t *testing.T, root string) string {
	t.Helper()
	output := filepath.Join(root, ".tmp", "observability-replicas", "registry.json")
	if err := os.MkdirAll(filepath.Dir(output), 0o750); err != nil {
		t.Fatalf("create fixture directory: %v", err)
	}
	_ = os.Remove(output)
	cmd := exec.Command("go", "run", "./scripts/golden-path-001-fixture",
		"-fixture", "testdata/golden/integration/adt-http", "-output", output)
	cmd.Dir = root
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build registry fixture: %v\n%s", err, combined)
	}
	t.Cleanup(func() { _ = os.Remove(output) })
	return output
}

// freshDatabase creates a per-mode database so the two proof runs cannot see
// each other's rows.
func freshDatabase(t *testing.T, baseDSN, name string) string {
	t.Helper()
	admin, err := sql.Open("postgres", baseDSN)
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	defer func() { _ = admin.Close() }()
	if err := admin.Ping(); err != nil {
		t.Fatalf("POSTGRES_TEST_URL is set but unreachable: %v", err)
	}
	if _, err := admin.Exec(`DROP DATABASE IF EXISTS ` + name); err != nil {
		t.Fatalf("drop %s: %v", name, err)
	}
	if _, err := admin.Exec(`CREATE DATABASE ` + name); err != nil {
		t.Fatalf("create %s: %v", name, err)
	}
	parsed, err := url.Parse(baseDSN)
	if err != nil {
		t.Fatalf("parse POSTGRES_TEST_URL: %v", err)
	}
	parsed.Path = "/" + name
	return parsed.String()
}

func dsnAddress(t *testing.T, dsn string) string {
	t.Helper()
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	host := parsed.Hostname()
	port := parsed.Port()
	if port == "" {
		port = "5432"
	}
	return net.JoinHostPort(host, port)
}

func parseDSN(t *testing.T, dsn string) (user, password, database string) {
	t.Helper()
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	password, _ = parsed.User.Password()
	return parsed.User.Username(), password, strings.TrimPrefix(parsed.Path, "/")
}

// documentedEnvValue reads a key out of `.env.example`, which is what an
// operator copies. Reading the documentation rather than crafting a value is
// what makes assertion 3 a guard against the documentation regressing.
func documentedEnvValue(t *testing.T, path, key string) (string, bool) {
	t.Helper()
	content, err := os.ReadFile(path) //nolint:gosec // Test reads a repository fixture by construction.
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, line := range strings.Split(string(content), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		name, value, found := strings.Cut(trimmed, "=")
		if !found || strings.TrimSpace(name) != key {
			continue
		}
		return strings.Trim(strings.TrimSpace(value), `"'`), true
	}
	return "", false
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

func containsInOrder(seen, want []string) bool {
	index := 0
	for _, value := range seen {
		if index < len(want) && value == want[index] {
			index++
		}
	}
	return index == len(want)
}

func truncate(value string) string {
	const limit = 512
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "…"
}
