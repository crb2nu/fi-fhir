//go:build retentionnodrain

package retention

// drainOnFullBatch restores the pre-Sprint-5 purge loop: exactly one
// PurgeExpired call per tick, then block on the ticker, whatever the batch came
// back holding. That is the shipped behaviour this lane repaired — a hard
// ceiling of one batch per class per tick, which at the shipped
// defaultBatchSize of 200 and defaultRetentionCadence of one hour is 200
// records per class per hour, on what store.go:31-33 calls "the busiest table
// in the system".
//
// This is the negative control for the D1 kill-test, required by
// .loom/33-sprint5-execution-specs.md, Lane S5-F ("restore the
// single-PurgeOnce-per-tick loop behind a build tag — assertion 1 must fail at
// exactly one batch per tick"). Building the tests with
//
//	go test -tags retentionnodrain ./internal/integration/retention/
//
// must make every drain assertion FAIL, and fail at the batch boundary. A
// kill-test that still passes with this tag is not measuring the drain, and the
// tag exists so that claim can be checked rather than asserted.
//
// This tag is never set in a shipped build or in any non-negative-control CI
// job. `make phi-retention-throughput-negative-control` is the only caller.
func drainOnFullBatch() bool { return false }
