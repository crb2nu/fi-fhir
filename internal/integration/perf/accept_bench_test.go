//go:build integration

package perf

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/mllp"
)

// The four benchmarks below are the repository's first measurement of the
// durable accept path. They cover the two entry points budget 1 names:
// authenticated HTTP (ingress.Service.Submit) and authenticated MLLP
// (mllp.Service.Submit), each serial and parallel.
//
// Only allocs/op is gated. See the package comment for why ns/op and
// events/sec are measured, reported, and deliberately not asserted on in the
// shared pool.

func BenchmarkDurableAccept_IngressSubmit(b *testing.B) {
	f := newFixture(b)
	ctx := context.Background()

	// Submit once before the timer so connection setup, prepared statements and
	// first-use lazy initialisation are not attributed to iteration 1.
	if _, err := f.ingress.Submit(ctx, ingressInput(0)); err != nil {
		b.Fatalf("warmup Submit: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := f.ingress.Submit(ctx, ingressInput(i+1)); err != nil {
			b.Fatalf("Submit(%d): %v", i+1, err)
		}
	}

	b.StopTimer()
	assertDurable(b, f, b.N+1)
}

func BenchmarkDurableAccept_IngressSubmitParallel(b *testing.B) {
	f := newFixture(b)
	ctx := context.Background()

	if _, err := f.ingress.Submit(ctx, ingressInput(0)); err != nil {
		b.Fatalf("warmup Submit: %v", err)
	}

	// Sequence numbers must stay unique across goroutines: a repeated
	// idempotency key returns the cached receipt without writing a row, which
	// would make the accept path look cheaper than it is and break the ledger
	// assertion below.
	var sequence atomic.Int64

	start := time.Now()
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := int(sequence.Add(1))
			if _, err := f.ingress.Submit(ctx, ingressInput(n)); err != nil {
				b.Errorf("Submit(%d): %v", n, err)
				return
			}
		}
	})

	b.StopTimer()
	reportThroughput(b, start)
	assertDurable(b, f, b.N+1)
}

func BenchmarkDurableAccept_MLLPSubmit(b *testing.B) {
	f := newFixture(b)
	ctx := context.Background()

	if _, err := f.mllp.Submit(ctx, mllp.ConnectionIdentity{}, hl7Message(0)); err != nil {
		b.Fatalf("warmup Submit: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		if _, err := f.mllp.Submit(ctx, mllp.ConnectionIdentity{}, hl7Message(i+1)); err != nil {
			b.Fatalf("Submit(%d): %v", i+1, err)
		}
	}

	b.StopTimer()
	assertDurable(b, f, b.N+1)
}

func BenchmarkDurableAccept_MLLPSubmitParallel(b *testing.B) {
	f := newFixture(b)
	ctx := context.Background()

	if _, err := f.mllp.Submit(ctx, mllp.ConnectionIdentity{}, hl7Message(0)); err != nil {
		b.Fatalf("warmup Submit: %v", err)
	}

	var sequence atomic.Int64

	start := time.Now()
	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			n := int(sequence.Add(1))
			if _, err := f.mllp.Submit(ctx, mllp.ConnectionIdentity{}, hl7Message(n)); err != nil {
				b.Errorf("Submit(%d): %v", n, err)
				return
			}
		}
	})

	b.StopTimer()
	reportThroughput(b, start)
	assertDurable(b, f, b.N+1)
}

// reportThroughput emits the events/sec metric bench-check's parser recognises.
// The unit string must be exactly "events/sec"; anything else is silently
// discarded by parseBenchLine. It is reported, never gated in the shared pool.
func reportThroughput(b *testing.B, start time.Time) {
	b.Helper()

	elapsed := time.Since(start)
	if elapsed <= 0 {
		return
	}
	b.ReportMetric(float64(b.N)/elapsed.Seconds(), "events/sec")
}

// assertDurable proves the benchmark drove the durable path.
//
// Without this, a misconfigured fixture that returned early — a rejected
// principal, a format mismatch, a preview-mode processor — would still produce
// a credible ns/op, and the number would be measuring nothing. The legacy
// engine writes no SQL, so one receipt row per accepted message is the
// discriminator. Delivery rows must stay at zero: the profile requires
// destinations decoupled from acceptance, and a non-zero count means the
// workflow started delivering inline and the measurement changed subject.
func assertDurable(b *testing.B, f *fixture, want int) {
	b.Helper()

	if got := f.ledgerCount(b, "integration_receipts"); got != want {
		b.Fatalf("integration_receipts = %d, want %d — the harness did not durably accept "+
			"every message, so its timings describe something other than the durable path", got, want)
	}
	for _, table := range []string{"integration_delivery_attempts", "integration_delivery_outbox"} {
		if got := f.ledgerCount(b, table); got != 0 {
			b.Fatalf("%s = %d, want 0 — destinations must stay decoupled from acceptance "+
				"for this measurement to match the reference profile", table, got)
		}
	}
}
