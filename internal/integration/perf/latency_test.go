package perf

import (
	"slices"
	"testing"
	"time"
)

// Budget 1 is written as percentiles — p95 <= 250 ms, p99 <= 500 ms — and a
// benchmark's ns/op is a mean. A mean cannot fail a tail budget: a path that
// answers most requests in 2 ms and one in twenty in 400 ms reports a
// comfortable mean while missing p95 by 150 ms. So the durable benchmarks
// record every measured accept and report the percentiles the budget names;
// scripts/performance-report.sh evaluates those, never ns/op.
//
// This file carries no build tag on purpose. The percentile arithmetic is the
// part of the harness that can be wrong without a database, so it is proven in
// the ordinary unit job rather than only on the pinned runner.

// latencyPercentiles returns the nearest-rank p50, p95 and p99 of samples.
//
// Nearest rank is the definition that never interpolates: every reported
// value is a latency some accept actually took. It sorts samples in place.
func latencyPercentiles(samples []time.Duration) (p50, p95, p99 time.Duration) {
	if len(samples) == 0 {
		return 0, 0, 0
	}
	slices.Sort(samples)
	return nearestRank(samples, 50), nearestRank(samples, 95), nearestRank(samples, 99)
}

// nearestRank returns the pct-th percentile of an ascending, non-empty slice:
// the smallest value with at least pct percent of samples at or below it.
// Integer arithmetic keeps the rank exact. A float ceiling is not safe in
// general: 7.0/100*100 is 7.000000000000001 in float64, so ceil reports rank 8.
func nearestRank(sorted []time.Duration, pct int) time.Duration {
	rank := (pct*len(sorted) + 99) / 100 // ceil(pct/100 * n)
	if rank < 1 {
		rank = 1
	}
	return sorted[rank-1]
}

func TestLatencyPercentilesAreNearestRank(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }

	tests := []struct {
		name                      string
		samples                   []time.Duration
		wantP50, wantP95, wantP99 time.Duration
	}{
		{
			name:    "empty reports zero rather than panicking",
			samples: nil,
		},
		{
			name:    "a single sample is every percentile",
			samples: []time.Duration{ms(7)},
			wantP50: ms(7), wantP95: ms(7), wantP99: ms(7),
		},
		{
			// 1..100 ms: the k-th percentile is exactly k ms.
			name:    "one hundred samples",
			samples: ramp(100),
			wantP50: ms(50), wantP95: ms(95), wantP99: ms(99),
		},
		{
			// The CI shape: -benchtime=300x. Ranks 150, 285 and 297.
			name:    "three hundred samples, the pinned iteration count",
			samples: ramp(300),
			wantP50: ms(150), wantP95: ms(285), wantP99: ms(297),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p50, p95, p99 := latencyPercentiles(tt.samples)
			if p50 != tt.wantP50 || p95 != tt.wantP95 || p99 != tt.wantP99 {
				t.Fatalf("percentiles = %v/%v/%v, want %v/%v/%v", p50, p95, p99, tt.wantP50, tt.wantP95, tt.wantP99)
			}
		})
	}

	t.Run("the tail decides p95 even when the mean is comfortable", func(t *testing.T) {
		// 285 accepts at 2 ms and 15 at 400 ms sit exactly on the p95
		// boundary; one more slow accept tips it past. The mean stays near
		// 23 ms either way — the shape a mean-only report would certify
		// against a 250 ms p95.
		samples := make([]time.Duration, 0, 300)
		for i := 0; i < 285; i++ {
			samples = append(samples, ms(2))
		}
		for i := 0; i < 15; i++ {
			samples = append(samples, ms(400))
		}
		// Unsorted input: the slow accepts arrive first.
		slices.Reverse(samples)

		_, p95, _ := latencyPercentiles(samples)
		if p95 != ms(2) {
			// 285 of 300 is exactly 95%, so nearest-rank p95 is still 2 ms …
			t.Fatalf("p95 = %v, want 2ms at exactly 95%% fast samples", p95)
		}

		samples = append(samples, ms(400)) // … and one more slow accept tips it.
		_, p95, _ = latencyPercentiles(samples)
		if p95 != ms(400) {
			t.Fatalf("p95 = %v, want 400ms once more than 5%% of accepts are slow", p95)
		}
	})
}

func ramp(n int) []time.Duration {
	out := make([]time.Duration, n)
	for i := range out {
		// Descending, so the test also proves the function sorts.
		out[i] = time.Duration(n-i) * time.Millisecond
	}
	return out
}
