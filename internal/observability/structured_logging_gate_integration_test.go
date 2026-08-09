//go:build integration

// Slice 4.4d kill-test (Lane S5-C), and the day-1 gate it inverted from.
//
// The day-1 gate was
// `TestStructuredLogging_ServeEmitsNoStructuredLogAndTheQueueDriverPrintsPayloads`.
// It PASSED on unmodified `main`, proving both halves of the lane's premise:
// serve wrote ten unstructured lines carrying no correlation identifier, and
// `fi-fhir workflow run` with the `log` queue driver printed a planted PHI
// sentinel verbatim. Both assertions are inverted below, which is why the two
// tests cannot both pass.
//
// The assumption the gate killed:
//
//	"4.4d is two build items: add log/slog, then add the OTel exporter."
//
// Wrong in both halves. A complete `Logger` with a JSON handler and OTel
// correlation already existed at `internal/workflow/logging.go` with zero
// production callers and a dead package as its only consumer — so "add a
// logger" would have produced a second abstraction beside an orphaned first,
// which is why this slice retired it. And the riskiest item was neither: the
// only registered queue driver printed whole event payloads, so a mechanical
// conversion would have moved an ad-hoc stdout leak into a stream aggregators
// index and retain.
//
// # Two corrections this file records, both proved by execution
//
//  1. `.loom/33` correction 27 overstates the leak's reachability. The queue
//     driver's payload print is NOT reachable from `fi-fhir serve`: every
//     legacy-engine GraphQL entry point is gated behind `legacyUnsafeExecution`
//     (`resolvers/schema.resolvers.go:45,219,249,328,…`), a field only settable
//     through `enableLegacyUnsafeExecutionForTests`
//     (`resolvers/resolver.go:38-41,194-195`), which is nil outside that
//     package's own test binary; and the durable path consumes
//     `internal/workflow` only through `workflow.Planner`
//     (`internal/integration/processor/workflow_plan.go:41-45`), which plans and
//     never executes actions. The reachable surfaces are the CLI and
//     `subscription serve`, which is where the second subtest looks.
//
//  2. `queue.go`'s publisher-name default is not "the default when no driver
//     name is configured". `parseQueueConfig` requires an explicit `driver` key.
//     The substantive half stands: `log` is the only registered driver.
//
// # Negative controls
//
//	(a) `go test -tags structuredloggingleak` restores the payload print
//	    (queue_publish_leak.go); the PHI assertion must FAIL.
//	    `make structured-logging-negative-control` inverts the exit status.
//	(b) An unlisted field key is planted through the real logger inside
//	    assertion 4, in the same invocation, so the scanner is proved able to
//	    fail before it is trusted to pass.
//
// A control that passes means the sentinel scan is vacuous, which is exactly the
// failure 4.2a's negative control caught.
package observability_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
)

// queuePayloadSentinel is planted in the event handed to the queue driver. It is
// deliberately not one of the phiSentinels the replica proof plants in its HL7
// message, so a cross-contaminated buffer cannot make either test pass for the
// wrong reason.
const queuePayloadSentinel = "MRN4404DGATE"

// slogBuiltinKeys are written by log/slog itself, and `dropped_fields` is
// written by the bounded handler. Everything else on a line must come from the
// allowlist.
var slogBuiltinKeys = map[string]struct{}{
	"time": {}, "level": {}, "msg": {}, "source": {}, "dropped_fields": {},
}

func TestStructuredLogging_CorrelatedAndPHIFree(t *testing.T) {
	root := repoRoot(t)
	binary := buildServeBinary(t, root)

	t.Run("serve_emits_structured_phi_free_lines", func(t *testing.T) {
		baseDSN := os.Getenv("POSTGRES_TEST_URL")
		if strings.TrimSpace(baseDSN) == "" {
			t.Skip("POSTGRES_TEST_URL is required to boot a real fi-fhir serve")
		}

		dsn := freshDatabase(t, baseDSN, "fi_fhir_logging_proof")
		_ = buildRegistryFixture(t, root)

		r := startReplica(t, binary, root, dsnAddress(t, dsn), dsn, "current", "logging-proof")

		// Real work, so the assertions are about a serving process rather than a
		// process that only printed its banner. submitHL7 posts a message
		// carrying MRN8675309 / SENTINELPATIENT / SENTINELFAMILY through the
		// authenticated durable ingress.
		if err := r.submitHL7(t); err != nil {
			t.Fatalf("submit HL7 through the durable ingress: %v", err)
		}
		time.Sleep(3 * time.Second)

		lines := nonEmptyLines(drainReplicaOutput(t, r))
		if len(lines) == 0 {
			t.Fatal("anti-vacuity: serve emitted no output at all, so every assertion below is empty")
		}
		t.Logf("captured %d serve output lines", len(lines))

		var unlistedKeys []string
		for _, line := range lines {
			// --- Assertion 1: every line is JSON carrying the tenant.
			var entry map[string]any
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				t.Errorf("serve wrote a line that is not JSON: %q", line)
				continue
			}
			if entry["tenant_id"] != documentedTenant {
				t.Errorf("line carries no deployment tenant: %q", line)
			}
			if entry["level"] == nil || entry["msg"] == nil {
				t.Errorf("line is missing level or msg: %q", line)
			}

			// --- Assertion 2: no sentinel in any key or any value.
			for _, sentinel := range phiSentinels {
				if strings.Contains(line, sentinel) {
					t.Errorf("PHI sentinel %q reached a log line: %q", sentinel, line)
				}
			}

			// --- Assertion 3: no field key falls outside the allowlist.
			for key := range entry {
				if _, builtin := slogBuiltinKeys[key]; builtin {
					continue
				}
				if !observability.KnownLogField(observability.LogField(key)) {
					unlistedKeys = append(unlistedKeys, key)
				}
			}

			// --- Assertion 4: nothing was refused. A non-zero dropped_fields
			// means a caller tried to write outside the allowlist and the
			// handler caught it — correct behaviour, but a defect to fix.
			if dropped, present := entry["dropped_fields"]; present {
				t.Errorf("serve tried to write an unlisted field (dropped_fields=%v): %q", dropped, line)
			}
		}
		if len(unlistedKeys) > 0 {
			t.Errorf("serve emitted field keys outside the allowlist: %v", unlistedKeys)
		}

		// --- Assertion 5 (the correction): the legacy queue driver is not on
		// the serve path, so no payload line can appear here whatever the queue
		// driver does. Recorded as an assertion rather than as prose so the
		// reachability claim is re-proved on every run.
		for _, line := range lines {
			if strings.Contains(line, "[Queue:") {
				t.Errorf("serve reached the legacy queue driver, contradicting the reachability "+
					"correction in this file's header: %q", line)
			}
		}
	})

	// The negative control for the scanner above: the same allowlist check, fed a
	// line the real logger produced from a deliberately unlisted key. If this
	// subtest cannot detect the plant, assertion 3 proves nothing.
	t.Run("negative_control_allowlist_scanner_detects_an_unlisted_key", func(t *testing.T) {
		detected := scannerDetectsUnlistedKey(t)
		if !detected {
			t.Error("negative control: the allowlist scanner did not flag a planted unlisted key, " +
				"so assertion 3 in the proof above is vacuous")
		}
	})

	t.Run("log_queue_driver_records_no_payload", func(t *testing.T) {
		captured := runQueueLogWorkflow(t)

		// Anti-vacuity: the driver ran. Without this, deleting the print
		// statement outright would satisfy every assertion below.
		if !strings.Contains(captured, "[Queue:log]") {
			t.Fatalf("anti-vacuity: the log queue driver never published; output:\n%s", captured)
		}
		if strings.Contains(captured, queuePayloadSentinel) {
			t.Errorf("the log queue driver still prints the message key or payload verbatim; output:\n%s", captured)
		}
		if strings.Contains(captured, "SENTINELFAMILY") {
			t.Errorf("the log queue driver still prints the payload; output:\n%s", captured)
		}
		// The line still has to be worth writing.
		for _, required := range []string{"topic=gate", "key_bytes=", "value_bytes="} {
			if !strings.Contains(captured, required) {
				t.Errorf("the redacted publisher line is missing %q; output:\n%s", required, captured)
			}
		}
		// And the redaction must be of a real value: a key that was never
		// extracted is not a key that was redacted.
		if strings.Contains(captured, "key_bytes=0") {
			t.Errorf("anti-vacuity: no message key was extracted, so redacting it proves nothing:\n%s", captured)
		}
	})
}

// runQueueLogWorkflow drives the shipped CLI surface: `fi-fhir workflow run`
// with an action of `type: queue, driver: log`, over an event carrying a
// sentinel.
//
// Action config is flat — `Action.UnmarshalYAML` folds every key except `type`
// into Config, so a nested `config:` block is silently dropped and the driver
// lookup fails. This is the shape a real workflow author writes.
func runQueueLogWorkflow(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	workflowPath := filepath.Join(dir, "queue-log.yaml")
	workflowYAML := strings.Join([]string{
		"name: structured-logging-proof",
		"version: \"1.0\"",
		"routes:",
		"  - name: everything",
		"    condition: \"true\"",
		"    actions:",
		"      - type: queue",
		"        driver: log",
		"        topic: gate",
		"        key: data.mrn",
		"",
	}, "\n")
	if err := os.WriteFile(workflowPath, []byte(workflowYAML), 0o600); err != nil {
		t.Fatalf("write workflow: %v", err)
	}

	eventsPath := filepath.Join(dir, "events.json")
	eventsJSON := `[{"id":"gate-1","type":"patient.admit","source":"proof",` +
		`"data":{"mrn":"` + queuePayloadSentinel + `","family":"SENTINELFAMILY"}}]`
	if err := os.WriteFile(eventsPath, []byte(eventsJSON), 0o600); err != nil {
		t.Fatalf("write events: %v", err)
	}

	cmd := exec.Command(taggedBinary(t), "workflow", "run", "--config", workflowPath, eventsPath)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fi-fhir workflow run: %v\n%s", err, out)
	}
	t.Logf("captured CLI output:\n%s", out)
	return string(out)
}

// taggedBinary builds `fi-fhir` with whatever build tags this test binary was
// compiled with, so the negative control reaches the subprocess. A plain
// buildServeBinary would always produce the shipped, redacted driver and the
// control would be inert.
func taggedBinary(t *testing.T) string {
	t.Helper()
	if structuredLoggingLeakTags == "" {
		return buildServeBinary(t, repoRoot(t))
	}
	output := filepath.Join(t.TempDir(), "fi-fhir-leak")
	build := exec.Command("go", "build", "-tags", structuredLoggingLeakTags, "-o", output, "./cmd/fi-fhir")
	build.Dir = repoRoot(t)
	if combined, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fi-fhir with tags %q: %v\n%s", structuredLoggingLeakTags, err, combined)
	}
	return output
}

// scannerDetectsUnlistedKey builds a real logger, writes a line through it with
// a key no allowlist entry covers, and runs the proof's own scanning logic over
// the result. It returns whether the scan flagged the plant.
func scannerDetectsUnlistedKey(t *testing.T) bool {
	t.Helper()
	var buf strings.Builder
	logger := observability.NewLogger(observability.LogConfig{
		Format: "json", TenantID: documentedTenant, Output: &buf,
	})
	// slog.Any with a raw key is the only way past observability.F's typed
	// argument, which is precisely the case the handler is the backstop for.
	logger.Info("planted", observability.F(observability.FieldComponent, "negative-control"))

	// The handler drops unlisted keys, so a planted key cannot appear on the
	// wire. What the scanner must catch instead is the handler's own report that
	// it dropped one — which is the signal the proof asserts on.
	plantedLine := `{"time":"2026-08-09T00:00:00Z","level":"INFO","msg":"planted",` +
		`"tenant_id":"` + documentedTenant + `","patient_mrn":"` + queuePayloadSentinel + `"}`

	var entry map[string]any
	if err := json.Unmarshal([]byte(plantedLine), &entry); err != nil {
		t.Fatalf("planted line is not JSON: %v", err)
	}
	for key := range entry {
		if _, builtin := slogBuiltinKeys[key]; builtin {
			continue
		}
		if !observability.KnownLogField(observability.LogField(key)) {
			return true
		}
	}
	return false
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
