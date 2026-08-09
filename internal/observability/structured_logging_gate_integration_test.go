//go:build integration

// Slice 4.4d day-1 gate (Lane S5-C).
//
// The assumption this gate kills is:
//
//	"4.4d is two build items: add log/slog, then add the OTel exporter."
//
// Both halves of that sentence are wrong in the same way — the work is not
// additive. A complete structured `Logger` with a JSON handler and OTel
// trace/span correlation already sits at `internal/workflow/logging.go:17-286`
// with zero production callers, and OpenTelemetry is already a *direct*
// dependency (`go.mod:24-27`) whose global provider installer
// (`internal/workflow/tracing_otel.go:52,97`) has no caller either. So "add"
// produces a second abstraction beside an orphaned first.
//
// And the highest-risk item in the lane is neither of those: the only
// registered queue driver prints the entire serialized event payload to stdout
// (`internal/workflow/queue.go:320-323`). Converting that mechanically to a
// structured log line moves an ad-hoc stdout leak into a stream that log
// aggregators index and retain. Redaction has to precede conversion.
//
// This gate must PASS on unmodified `main`, proving both halves of the lane's
// premise in one invocation:
//
//  1. serve_emits_no_structured_log — a real `fi-fhir serve`, doing real work
//     against PostgreSQL, writes no line that parses as JSON and no line
//     carrying a correlation identifier.
//  2. log_queue_driver_prints_payloads — the shipped `fi-fhir workflow run`
//     surface, configured with the `log` queue driver, prints a planted PHI
//     sentinel verbatim on stdout.
//
// It inverts task by task and becomes this lane's negative control: after the
// lane lands, subtest 1's assertions must fail (serve emits JSON) and subtest
// 2's must fail (the payload is redacted).
//
// # Correction to `.loom/33-sprint5-execution-specs.md` correction 27
//
// The spec's day-1 gate describes assertion 2 as "one submission [to a running
// `fi-fhir serve`] carrying a PHI sentinel in the payload … appears verbatim on
// stdout". That is not reachable, and this file records why rather than
// constructing a configuration that hides it:
//
//   - Every legacy-engine GraphQL entry point is gated behind
//     `legacyUnsafeExecution` (`resolvers/schema.resolvers.go:45,219,249,328,
//     2215,2534,3441`), a field only settable through
//     `enableLegacyUnsafeExecutionForTests` (`resolvers/resolver.go:38-41,
//     194-195`), which is nil outside that package's own test binary. A real
//     `serve` therefore never calls `Engine.Process`, so it never runs a queue
//     action.
//   - The durable path consumes `internal/workflow` only through
//     `workflow.Planner` (`internal/integration/processor/workflow_plan.go:
//     41-45`), which *plans* and never executes actions.
//   - The reachable production surfaces for the leak are the CLI —
//     `fi-fhir workflow run` (`cmd/fi-fhir/main.go:2043`), `workflow record`
//     (`:2256`), `workflow simulate` (`:2512`) — and
//     `fi-fhir subscription serve` with a workflow router (`:4198-4202`).
//
// Subtest 1 asserts that absence directly, so the correction is proved by the
// gate rather than asserted in prose. Also corrected: the spec cites
// `queue.go:313-314` as "the default when no driver name is configured". Those
// lines default the publisher's *instance name*; `parseQueueConfig`
// (`queue.go:203-206`) requires an explicit `driver` key and errors without
// one. The substantive claim survives — `log` is the only registered driver
// (`queue.go:331-334`) — but it is a default nowhere.
package observability_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// queuePayloadSentinel is planted in the event handed to the workflow engine.
// It is deliberately not one of the phiSentinels used by the replica proof so a
// cross-contaminated buffer cannot make either test pass for the wrong reason.
const queuePayloadSentinel = "MRN4404DGATE"

// structuredLogKeys are the identifiers a correlated structured log line would
// have to carry. `main` carries none of them.
var structuredLogKeys = []string{"correlation_id", "trace_id", "span_id", "tenant_id"}

func TestStructuredLogging_ServeEmitsNoStructuredLogAndTheQueueDriverPrintsPayloads(t *testing.T) {
	root := repoRoot(t)
	binary := buildServeBinary(t, root)

	t.Run("serve_emits_no_structured_log", func(t *testing.T) {
		baseDSN := os.Getenv("POSTGRES_TEST_URL")
		if strings.TrimSpace(baseDSN) == "" {
			t.Skip("POSTGRES_TEST_URL is required to boot a real fi-fhir serve")
		}

		dsn := freshDatabase(t, baseDSN, "fi_fhir_logging_gate")
		_ = buildRegistryFixture(t, root)

		// The replica harness reaches PostgreSQL through an address it is told
		// to use; this gate needs no fault injection, so it is pointed straight
		// at the database.
		r := startReplica(t, binary, root, dsnAddress(t, dsn), dsn, "current", "logging-gate")

		// Do real work so the assertion is about a serving process, not about a
		// process that only printed its banner. submitHL7 posts a message
		// carrying the replica proof's own PHI sentinels through the
		// authenticated durable ingress.
		if err := r.submitHL7(t); err != nil {
			t.Fatalf("submit HL7 through the durable ingress: %v", err)
		}
		// Give the durable pipeline a moment to log whatever it logs.
		time.Sleep(3 * time.Second)

		lines := nonEmptyLines(drainReplicaOutput(t, r))
		if len(lines) == 0 {
			t.Fatal("anti-vacuity: serve emitted no output at all, so 'none of it is structured' proves nothing")
		}
		t.Logf("captured %d serve output lines", len(lines))

		var jsonLines []string
		var keyedLines []string
		var queueLines []string
		for _, line := range lines {
			var probe map[string]any
			if err := json.Unmarshal([]byte(line), &probe); err == nil {
				jsonLines = append(jsonLines, line)
			}
			for _, key := range structuredLogKeys {
				if strings.Contains(line, key) {
					keyedLines = append(keyedLines, line)
					break
				}
			}
			if strings.Contains(line, "[Queue:") {
				queueLines = append(queueLines, line)
			}
		}

		// (a) No structured logging exists on the serve path.
		if len(jsonLines) != 0 {
			t.Errorf("expected zero JSON log lines on main, got %d; first: %s", len(jsonLines), jsonLines[0])
		}
		// (b) Nothing serve writes is correlated.
		if len(keyedLines) != 0 {
			t.Errorf("expected zero lines carrying a correlation identifier on main, got %d; first: %s",
				len(keyedLines), keyedLines[0])
		}
		// (c) The correction: the queue driver's payload print is not on the
		// serve path at all, despite a real submission having been processed.
		if len(queueLines) != 0 {
			t.Errorf("serve reached the legacy queue driver, which contradicts the reachability "+
				"correction recorded in this file's header; got %d lines, first: %s",
				len(queueLines), queueLines[0])
		}
	})

	t.Run("log_queue_driver_prints_payloads", func(t *testing.T) {
		dir := t.TempDir()

		// Action config is flat: `Action.UnmarshalYAML`
		// (`internal/workflow/types.go:112`) folds every key except `type` into
		// Config, so a nested `config:` block is silently dropped and the
		// driver lookup fails. This is the shape a real workflow author uses.
		workflowPath := filepath.Join(dir, "queue-log.yaml")
		workflowYAML := strings.Join([]string{
			"name: structured-logging-day-1-gate",
			"version: \"1.0\"",
			"routes:",
			"  - name: everything",
			"    condition: \"true\"",
			"    actions:",
			"      - type: queue",
			"        driver: log",
			"        topic: gate",
			"",
		}, "\n")
		if err := os.WriteFile(workflowPath, []byte(workflowYAML), 0o600); err != nil {
			t.Fatalf("write workflow: %v", err)
		}

		eventsPath := filepath.Join(dir, "events.json")
		eventsJSON := `[{"id":"gate-1","type":"patient.admit","source":"day-1-gate",` +
			`"data":{"mrn":"` + queuePayloadSentinel + `","family":"SENTINELFAMILY"}}]`
		if err := os.WriteFile(eventsPath, []byte(eventsJSON), 0o600); err != nil {
			t.Fatalf("write events: %v", err)
		}

		cmd := exec.Command(binary, "workflow", "run", "--config", workflowPath, eventsPath)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("fi-fhir workflow run: %v\n%s", err, out)
		}
		captured := string(out)
		t.Logf("captured CLI output:\n%s", captured)

		// Anti-vacuity: the driver actually ran. Without this, a workflow that
		// silently matched no route would make the sentinel assertion pass for
		// the wrong reason.
		if !strings.Contains(captured, "[Queue:log]") {
			t.Fatalf("anti-vacuity: the log queue driver never published; output:\n%s", captured)
		}

		// The leak itself: the whole serialized payload reaches stdout.
		if !strings.Contains(captured, queuePayloadSentinel) {
			t.Errorf("expected the PHI sentinel %q verbatim on stdout from the log queue driver, "+
				"but it is absent; output:\n%s", queuePayloadSentinel, captured)
		}
	})
}

// drainReplicaOutput stops the replica and returns everything it wrote.
//
// The harness points the child process's stdout and stderr at a bytes.Buffer,
// and os/exec fills that buffer from a copier goroutine. Reading it while the
// process is alive is a data race the -race detector correctly flags — and this
// proof runs under -race, so the flag is a failure. The process is stopped and
// Wait is allowed to join the copier before the buffer is touched.
// startReplica's cleanup kills and waits again; both are no-ops the second time.
func drainReplicaOutput(t *testing.T, r *replica) string {
	t.Helper()
	_ = r.cmd.Process.Kill()
	_ = r.cmd.Wait()
	return r.logs.String()
}

func nonEmptyLines(blob string) []string {
	var out []string
	for _, line := range strings.Split(blob, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}
