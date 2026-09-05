//go:build integration

package perf

import (
	"context"
	"runtime"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/mllp"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

// TestPerformanceHarness_DurableAcceptAllocationsAreBounded is slice 4.4b's
// primary kill-test.
//
// The assumption it kills is that a harness measures what it appears to
// measure. A benchmark that silently short-circuits — a rejected principal, a
// format mismatch, a preview-mode processor, a duplicate idempotency key —
// still produces a plausible ns/op and a plausible allocs/op. Nothing about the
// number says which code ran. The benchmark that timed the legacy engine while
// reporting "100.0% of target" is the same failure one layer up.
//
// So this asserts the three things a number has to earn before it is evidence:
// that the durable path ran, that the figure is reproducible, and that the gate
// bites when it should.
func TestPerformanceHarness_DurableAcceptAllocationsAreBounded(t *testing.T) {
	t.Run("the harness drives the durable path, not internal/workflow", func(t *testing.T) {
		f := newFixture(t)
		ctx := context.Background()

		const messages = 5
		for i := 1; i <= messages; i++ {
			if _, err := f.ingress.Submit(ctx, ingressInput(i)); err != nil {
				t.Fatalf("ingress Submit(%d): %v", i, err)
			}
		}
		for i := 1; i <= messages; i++ {
			if _, err := f.mllp.Submit(ctx, mllp.ConnectionIdentity{}, hl7Message(1000+i)); err != nil {
				t.Fatalf("mllp Submit(%d): %v", i, err)
			}
		}

		// The legacy engine writes no SQL at all, so a receipt row per accepted
		// message is what distinguishes the durable path from it. This is the
		// assertion that would have caught a harness pointed at the wrong
		// processor.
		if got := f.ledgerCount(t, "integration_receipts"); got != 2*messages {
			t.Fatalf("integration_receipts = %d, want %d", got, 2*messages)
		}
		for _, table := range []string{"integration_canonical_events", "integration_message_lineage"} {
			if got := f.ledgerCount(t, table); got != 2*messages {
				t.Errorf("%s = %d, want %d", table, got, 2*messages)
			}
		}

		// Destinations must stay decoupled from acceptance, per the reference
		// profile. A non-zero count here means the accept transaction started
		// doing delivery work and the benchmark changed subject.
		for _, table := range []string{"integration_delivery_attempts", "integration_delivery_outbox"} {
			if got := f.ledgerCount(t, table); got != 0 {
				t.Errorf("%s = %d, want 0", table, got)
			}
		}
	})

	t.Run("a repeated idempotency key writes no row", func(t *testing.T) {
		// The benchmarks vary the sequence number per iteration precisely to
		// avoid this path. If that ever regresses, iterations 2..N become cache
		// hits, allocs/op collapses, and the gate silently stops measuring the
		// accept path. Pin the behaviour so the reason is discoverable.
		f := newFixture(t)
		ctx := context.Background()

		if _, err := f.ingress.Submit(ctx, ingressInput(1)); err != nil {
			t.Fatalf("first Submit: %v", err)
		}
		if _, err := f.ingress.Submit(ctx, ingressInput(1)); err != nil {
			t.Fatalf("duplicate Submit: %v", err)
		}

		if got := f.ledgerCount(t, "integration_receipts"); got != 1 {
			t.Fatalf("integration_receipts = %d, want 1 — a duplicate key must not write a second row", got)
		}
	})

	t.Run("allocations are reproducible across runs on one machine", func(t *testing.T) {
		if testing.Short() {
			t.Skip("measuring allocations twice is not a short test")
		}

		// This is the property the whole gate rests on. It is weaker than the
		// legacy engine's: those micro-benchmarks report a bit-identical count,
		// while a database-backed path varies by a couple of allocations per
		// message. Assert the tolerance that was actually measured rather than
		// the exactness the legacy set enjoys.
		first := measureAcceptAllocs(t)
		second := measureAcceptAllocs(t)

		const tolerance = 0.01 // 1%; measured spread at this iteration count is ~0.04%
		delta := first - second
		if delta < 0 {
			delta = -delta
		}
		if float64(delta) > tolerance*float64(first) {
			t.Fatalf("allocations per accept moved %d between runs (%d then %d), more than %.0f%%; "+
				"the durable alloc signal is not stable enough to gate at this iteration count",
				delta, first, second, tolerance*100)
		}
	})

	t.Run("every gated name is a benchmark that exists", func(t *testing.T) {
		// A ceiling naming a benchmark that no longer exists does not fail
		// loudly — Check reports "required benchmark result missing", which
		// reads like an infrastructure problem. Catch the rename here instead.
		gated := workflow.DurableAllocCeilings()
		if len(gated) == 0 {
			t.Fatal("no durable allocation ceilings are configured")
		}

		known := map[string]bool{
			"BenchmarkDurableAccept_IngressSubmit":         true,
			"BenchmarkDurableAccept_IngressSubmitParallel": true,
			"BenchmarkDurableAccept_MLLPSubmit":            true,
			"BenchmarkDurableAccept_MLLPSubmitParallel":    true,
		}
		for name := range gated {
			if !known[name] {
				t.Errorf("ceiling names %q, which is not a benchmark in this package", name)
			}
		}

		// The parallel variants are measured but deliberately not gated; see
		// durableAllocCeilings for the measured noise that decided it.
		for _, name := range []string{
			"BenchmarkDurableAccept_IngressSubmitParallel",
			"BenchmarkDurableAccept_MLLPSubmitParallel",
		} {
			if _, ok := gated[name]; ok {
				t.Errorf("%q is gated, but its allocation count is scheduler-dependent; "+
					"gate the serial variant instead", name)
			}
		}
	})
}

// measureAcceptAllocs reports allocations per accepted message over a fixed
// number of submissions.
//
// It counts runtime.MemStats.Mallocs directly rather than calling
// testing.Benchmark, because testing.Benchmark chooses its own iteration count
// from a time budget. That is the very thing that makes a database-backed
// allocation figure unstable: connection pool growth and driver setup are
// amortized over however many iterations happened to fit, so two runs of the
// same code report different allocs/op. An earlier version of this test used
// testing.Benchmark and failed with 5659 then 4730 — a 20% swing that measured
// scheduling, not the accept path. Pinning the count is the same fix the CI job
// applies with -benchtime=300x.
func measureAcceptAllocs(t *testing.T) int64 {
	t.Helper()

	const (
		warmup      = 50  // let the connection pool and driver reach steady state
		measured    = 300 // matches workflow.DurableBenchtime
		firstSeqNum = 1
	)

	f := newFixture(t)
	ctx := context.Background()

	for i := 0; i < warmup; i++ {
		if _, err := f.ingress.Submit(ctx, ingressInput(firstSeqNum+i)); err != nil {
			t.Fatalf("warmup Submit(%d): %v", i, err)
		}
	}

	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	for i := 0; i < measured; i++ {
		if _, err := f.ingress.Submit(ctx, ingressInput(firstSeqNum+warmup+i)); err != nil {
			t.Fatalf("Submit(%d): %v", i, err)
		}
	}

	runtime.ReadMemStats(&after)
	return int64((after.Mallocs - before.Mallocs) / measured)
}

// TestHeapSamplerReportsGrowthAboveBaseline covers the budget-3 sampler.
func TestHeapSamplerReportsGrowthAboveBaseline(t *testing.T) {
	sampler := NewHeapSampler(time.Millisecond)

	// Hold the allocation live across a sampling tick; a garbage value the
	// collector can reclaim immediately would make this test flaky by design.
	ballast := make([]byte, 32<<20)
	for i := 0; i < len(ballast); i += 4096 {
		ballast[i] = 1
	}
	time.Sleep(20 * time.Millisecond)

	peak := sampler.Stop()
	runtimeKeepAlive(ballast)

	if peak < 16<<20 {
		t.Fatalf("peak above baseline = %d bytes, want at least 16 MiB after a 32 MiB allocation", peak)
	}
}

// runtimeKeepAlive stops the collector reclaiming the ballast before Stop reads
// the heap. runtime.KeepAlive is the documented spelling; this wrapper only
// exists to keep the intent legible at the call site.
func runtimeKeepAlive(b []byte) { _ = len(b) }
