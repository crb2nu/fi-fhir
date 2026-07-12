package workflow

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// BenchmarkResult holds the results of a single benchmark run.
type BenchmarkResult struct {
	Name          string
	Iterations    int64
	NsPerOp       float64
	BytesPerOp    int64
	AllocsPerOp   int64
	EventsPerSec  float64
	CustomMetrics map[string]float64
	parsedMetrics map[string]struct{}
}

// BenchmarkSuite holds results from multiple benchmark runs for comparison.
type BenchmarkSuite struct {
	Name      string
	Timestamp time.Time
	Results   []BenchmarkResult
}

// NewBenchmarkSuite creates a new benchmark suite.
func NewBenchmarkSuite(name string) *BenchmarkSuite {
	return &BenchmarkSuite{
		Name:      name,
		Timestamp: time.Now(),
		Results:   make([]BenchmarkResult, 0),
	}
}

// AddResult adds a benchmark result to the suite.
func (s *BenchmarkSuite) AddResult(result BenchmarkResult) {
	s.Results = append(s.Results, result)
}

// GetResult retrieves a result by name.
func (s *BenchmarkSuite) GetResult(name string) *BenchmarkResult {
	for i := range s.Results {
		if s.Results[i].Name == name {
			return &s.Results[i]
		}
	}
	return nil
}

// Summary returns a formatted summary of all benchmark results.
func (s *BenchmarkSuite) Summary() string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Benchmark Suite: %s\n", s.Name))
	sb.WriteString(fmt.Sprintf("Timestamp: %s\n", s.Timestamp.Format(time.RFC3339)))
	sb.WriteString(strings.Repeat("=", 80) + "\n\n")

	// Sort results by name for consistent output
	sorted := make([]BenchmarkResult, len(s.Results))
	copy(sorted, s.Results)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	for _, r := range sorted {
		sb.WriteString(fmt.Sprintf("%-50s\n", r.Name))
		sb.WriteString(fmt.Sprintf("  Iterations:     %d\n", r.Iterations))
		sb.WriteString(fmt.Sprintf("  Time/Op:        %.2f ns\n", r.NsPerOp))
		sb.WriteString(fmt.Sprintf("  Bytes/Op:       %d B\n", r.BytesPerOp))
		sb.WriteString(fmt.Sprintf("  Allocs/Op:      %d\n", r.AllocsPerOp))
		if r.EventsPerSec > 0 {
			sb.WriteString(fmt.Sprintf("  Throughput:     %.0f events/sec\n", r.EventsPerSec))
		}
		for k, v := range r.CustomMetrics {
			sb.WriteString(fmt.Sprintf("  %s: %.2f\n", k, v))
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// BenchmarkComparison compares two benchmark suites.
type BenchmarkComparison struct {
	Baseline BenchmarkSuite
	Current  BenchmarkSuite
	Diffs    []BenchmarkDiff
}

// BenchmarkDiff represents the difference between two benchmark results.
type BenchmarkDiff struct {
	Name            string
	BaselineNsPerOp float64
	CurrentNsPerOp  float64
	NsPerOpDelta    float64 // Percentage change (negative is improvement)
	BaselineAllocs  int64
	CurrentAllocs   int64
	AllocsDelta     float64 // Percentage change
	Improved        bool
	Regressed       bool
}

// Compare compares two benchmark suites and returns the differences.
func Compare(baseline, current *BenchmarkSuite) *BenchmarkComparison {
	comparison := &BenchmarkComparison{
		Baseline: *baseline,
		Current:  *current,
		Diffs:    make([]BenchmarkDiff, 0),
	}

	for _, curr := range current.Results {
		base := baseline.GetResult(curr.Name)
		if base == nil {
			continue // No baseline to compare against
		}

		diff := BenchmarkDiff{
			Name:            curr.Name,
			BaselineNsPerOp: base.NsPerOp,
			CurrentNsPerOp:  curr.NsPerOp,
			BaselineAllocs:  base.AllocsPerOp,
			CurrentAllocs:   curr.AllocsPerOp,
		}

		// Calculate percentage changes
		if base.NsPerOp > 0 {
			diff.NsPerOpDelta = ((curr.NsPerOp - base.NsPerOp) / base.NsPerOp) * 100
		}
		if base.AllocsPerOp > 0 {
			diff.AllocsDelta = ((float64(curr.AllocsPerOp) - float64(base.AllocsPerOp)) / float64(base.AllocsPerOp)) * 100
		}

		// Determine if improved or regressed (>5% threshold)
		if diff.NsPerOpDelta < -5 {
			diff.Improved = true
		} else if diff.NsPerOpDelta > 5 {
			diff.Regressed = true
		}

		comparison.Diffs = append(comparison.Diffs, diff)
	}

	return comparison
}

// Summary returns a formatted comparison summary.
func (c *BenchmarkComparison) Summary() string {
	var sb strings.Builder

	sb.WriteString("Benchmark Comparison\n")
	sb.WriteString(fmt.Sprintf("Baseline: %s (%s)\n", c.Baseline.Name, c.Baseline.Timestamp.Format("2006-01-02 15:04")))
	sb.WriteString(fmt.Sprintf("Current:  %s (%s)\n", c.Current.Name, c.Current.Timestamp.Format("2006-01-02 15:04")))
	sb.WriteString(strings.Repeat("=", 100) + "\n\n")

	// Sort by name
	sorted := make([]BenchmarkDiff, len(c.Diffs))
	copy(sorted, c.Diffs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Name < sorted[j].Name
	})

	// Header
	sb.WriteString(fmt.Sprintf("%-40s %15s %15s %10s %10s\n",
		"Benchmark", "Baseline", "Current", "Delta", "Status"))
	sb.WriteString(strings.Repeat("-", 100) + "\n")

	improved := 0
	regressed := 0

	for _, d := range sorted {
		status := "  "
		if d.Improved {
			status = "✓ FASTER"
			improved++
		} else if d.Regressed {
			status = "✗ SLOWER"
			regressed++
		}

		sb.WriteString(fmt.Sprintf("%-40s %12.0f ns %12.0f ns %+9.1f%% %10s\n",
			truncateName(d.Name, 40),
			d.BaselineNsPerOp,
			d.CurrentNsPerOp,
			d.NsPerOpDelta,
			status))
	}

	sb.WriteString(strings.Repeat("-", 100) + "\n")
	sb.WriteString(fmt.Sprintf("\nSummary: %d improved, %d regressed, %d unchanged\n",
		improved, regressed, len(sorted)-improved-regressed))

	return sb.String()
}

// HasRegressions returns true if any benchmarks regressed.
func (c *BenchmarkComparison) HasRegressions() bool {
	for _, d := range c.Diffs {
		if d.Regressed {
			return true
		}
	}
	return false
}

// truncateName truncates a name to fit within maxLen.
func truncateName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	return name[:maxLen-3] + "..."
}

// PerformanceThresholds defines acceptable performance bounds.
type PerformanceThresholds struct {
	MaxNsPerOp     map[string]float64 // Maximum ns/op per benchmark
	MaxAllocsPerOp map[string]int64   // Maximum allocs/op per benchmark
	MinThroughput  map[string]float64 // Minimum events/sec per benchmark
}

// DefaultWorkflowThresholds returns default performance thresholds for workflow engine.
func DefaultWorkflowThresholds() *PerformanceThresholds {
	return &PerformanceThresholds{
		MaxNsPerOp: map[string]float64{
			"BenchmarkEngineProcess":         5000, // 5µs max
			"BenchmarkCELEvaluate_Simple":    2000, // 2µs max on shared x86 CI
			"BenchmarkFilterMatch_EventType": 3000, // 3µs max
			"BenchmarkTransform_SetField":    2000, // 2µs max on shared x86 CI
		},
		MaxAllocsPerOp: map[string]int64{
			"BenchmarkEngineProcess":         50,
			"BenchmarkCELEvaluate_Simple":    10,
			"BenchmarkFilterMatch_EventType": 50,
			"BenchmarkTransform_SetField":    10,
		},
		MinThroughput: map[string]float64{
			"BenchmarkThroughput_Simple":  100000, // 100k events/sec minimum
			"BenchmarkThroughput_Complex": 50000,  // 50k events/sec minimum
		},
	}
}

// Validate checks if a benchmark suite meets the performance thresholds.
func (t *PerformanceThresholds) Validate(suite *BenchmarkSuite) []string {
	var violations []string
	required := make(map[string]struct{}, len(t.MaxNsPerOp)+len(t.MaxAllocsPerOp)+len(t.MinThroughput))
	for name := range t.MaxNsPerOp {
		required[name] = struct{}{}
	}
	for name := range t.MaxAllocsPerOp {
		required[name] = struct{}{}
	}
	for name := range t.MinThroughput {
		required[name] = struct{}{}
	}

	requiredNames := make([]string, 0, len(required))
	for name := range required {
		requiredNames = append(requiredNames, name)
	}
	sort.Strings(requiredNames)
	for _, name := range requiredNames {
		if suite.GetResult(name) == nil {
			violations = append(violations, fmt.Sprintf("%s: required benchmark result missing", name))
		}
	}

	for _, r := range suite.Results {
		// Check ns/op thresholds
		if maxNs, ok := t.MaxNsPerOp[r.Name]; ok {
			if !r.hasParsedMetric("ns/op") {
				violations = append(violations,
					fmt.Sprintf("%s: required ns/op metric missing", r.Name))
			} else if r.NsPerOp > maxNs {
				violations = append(violations,
					fmt.Sprintf("%s: %.0f ns/op exceeds threshold of %.0f ns/op",
						r.Name, r.NsPerOp, maxNs))
			}
		}

		// Check allocs/op thresholds
		if maxAllocs, ok := t.MaxAllocsPerOp[r.Name]; ok {
			if !r.hasParsedMetric("allocs/op") {
				violations = append(violations,
					fmt.Sprintf("%s: required allocs/op metric missing", r.Name))
			} else if r.AllocsPerOp > maxAllocs {
				violations = append(violations,
					fmt.Sprintf("%s: %d allocs/op exceeds threshold of %d allocs/op",
						r.Name, r.AllocsPerOp, maxAllocs))
			}
		}

		// Check throughput thresholds
		if minThroughput, ok := t.MinThroughput[r.Name]; ok {
			if !r.hasParsedMetric("events/sec") {
				violations = append(violations,
					fmt.Sprintf("%s: required events/sec metric missing", r.Name))
			} else if r.EventsPerSec < minThroughput {
				violations = append(violations,
					fmt.Sprintf("%s: %.0f events/sec below threshold of %.0f events/sec",
						r.Name, r.EventsPerSec, minThroughput))
			}
		}
	}

	return violations
}

// hasParsedMetric distinguishes a parsed zero value from a metric that was not
// present in benchmark output. Programmatically constructed results predate
// parser metadata and retain their existing validation behavior.
func (r BenchmarkResult) hasParsedMetric(unit string) bool {
	if r.parsedMetrics == nil {
		return true
	}
	_, ok := r.parsedMetrics[unit]
	return ok
}

// QuickBenchmark runs a quick performance test and returns ops/sec.
// Useful for runtime performance monitoring.
func QuickBenchmark(engine *Engine, event interface{}, duration time.Duration) float64 {
	start := time.Now()
	count := 0

	for time.Since(start) < duration {
		engine.Process(event)
		count++
	}

	elapsed := time.Since(start)
	return float64(count) / elapsed.Seconds()
}
