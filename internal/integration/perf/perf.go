// Package perf holds the durable integration path's performance harness.
//
// These are the first benchmarks under internal/integration in the repository's
// history. Everything that measured performance before this package measured
// internal/workflow — the legacy engine, which no durable path calls at runtime
// (the durable path reaches internal/workflow only for parse and plan). So the
// slice 4.4 product budgets, which are written about durable acceptance, had
// never been measured at all.
//
// # What is gated and what is not
//
// Only allocs/op is gated, and only in the ordinary shared CI pool. That is a
// deliberate limit, recorded as a dated decision in .loom/40-decisions.md
// (2026-08-09) and ratified by the sprint coordinator:
//
//   - Allocation counts are a property of the code, not the machine. This
//     repository measured that directly: across 78-87 CI artifacts spanning
//     three CPU classes, every gated benchmark reported a bit-identical
//     allocation count (internal/workflow/benchmark_util.go:321-327).
//
//   - Wall-clock is not gated here, in any calibrated form. The k3s pool spans
//     hardware differing by 5.3x, the existing latency margin was backtested on
//     micro-benchmarks, and a durable-accept benchmark runs against a
//     PostgreSQL service container in the same 1-CPU pod. A p95 gate calibrated
//     for that is either permanently red or too wide to detect a regression.
//     Wall-clock and throughput belong to the pinned-runner job, which archives
//     a report rather than asserting a threshold.
//
// So a green run here means "the accept path did not start allocating more per
// message". It does not mean any millisecond budget is met. Budgets 1, 2 and 3
// are harnessed but uncertified until a fi-fhir-perf runner exists; see
// docs/operations/SUPPORTED-1.0.md.
//
// # Running them
//
//	make bench-durable          # needs POSTGRES_TEST_URL
//
// The benchmarks carry //go:build integration, so a plain `go test -bench=.`
// does not see them and the existing test:benchmark job is unaffected.
package perf

import (
	"runtime"
	"time"
)

// HeapSampler measures peak heap use above an idle baseline.
//
// It exists for budget 3, which bounds the memory a 1-GiB batch import may add
// above idle. runtime.ReadMemStats appears nowhere in first-party code, so
// there was no sampler to reuse; this is the smallest one that answers the
// question. It deliberately lives in the perf package rather than in
// internal/integration/batch — measuring the product must not change it.
//
// It reports HeapAlloc rather than RSS. RSS is the number the budget is written
// in, but a Go process's RSS includes heap the collector has freed and not
// returned to the OS, so RSS is a property of GC timing as much as of the
// workload. HeapAlloc is reproducible and moves for the same reasons RSS moves.
// A report that needs true RSS must come from the pinned-runner job, which can
// read it from the cgroup; that gap is stated in the report schema rather than
// papered over here.
type HeapSampler struct {
	interval time.Duration
	stop     chan struct{}
	done     chan struct{}
	baseline uint64
	peak     uint64
}

// NewHeapSampler records an idle baseline and starts sampling.
//
// The caller is responsible for the process being genuinely idle at this point;
// a baseline taken mid-workload silently understates every later reading.
func NewHeapSampler(interval time.Duration) *HeapSampler {
	if interval <= 0 {
		interval = 10 * time.Millisecond
	}

	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)

	s := &HeapSampler{
		interval: interval,
		stop:     make(chan struct{}),
		done:     make(chan struct{}),
		baseline: stats.HeapAlloc,
		peak:     stats.HeapAlloc,
	}

	go s.run()
	return s
}

func (s *HeapSampler) run() {
	defer close(s.done)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	var stats runtime.MemStats
	for {
		select {
		case <-s.stop:
			return
		case <-ticker.C:
			runtime.ReadMemStats(&stats)
			if stats.HeapAlloc > s.peak {
				s.peak = stats.HeapAlloc
			}
		}
	}
}

// Stop halts sampling and returns peak heap above the idle baseline, in bytes.
//
// Stop is not safe to call twice.
func (s *HeapSampler) Stop() uint64 {
	close(s.stop)
	<-s.done

	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	if stats.HeapAlloc > s.peak {
		s.peak = stats.HeapAlloc
	}

	if s.peak < s.baseline {
		return 0
	}
	return s.peak - s.baseline
}

// Baseline returns the idle heap reading the sampler started from, in bytes.
func (s *HeapSampler) Baseline() uint64 { return s.baseline }
