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
	Package       string // Import path the benchmark belongs to, from the "pkg:" header.
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
	// CPU is the processor model reported by "go test" ("cpu:" header). It is
	// empty when the output predates the header or reports several models.
	CPU     string
	Results []BenchmarkResult
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

// ReduceToBest collapses repeated measurements of the same benchmark (as
// produced by "go test -count=N") into one result holding the best value seen
// for each metric, preserving the order benchmarks first appeared in.
//
// Best means fastest: minimum ns/op, bytes and allocs, maximum events/sec.
// Runner contention only ever adds time to a measurement, never removes it, so
// the minimum is the closest estimate of the code's true cost. A genuine
// regression slows every repetition, so taking the best of N cannot hide one.
func (s *BenchmarkSuite) ReduceToBest() *BenchmarkSuite {
	reduced := &BenchmarkSuite{
		Name:      s.Name,
		Timestamp: s.Timestamp,
		CPU:       s.CPU,
		Results:   make([]BenchmarkResult, 0, len(s.Results)),
	}

	index := make(map[string]int, len(s.Results))
	for _, r := range s.Results {
		pos, seen := index[r.Name]
		if !seen {
			index[r.Name] = len(reduced.Results)
			reduced.Results = append(reduced.Results, r)
			continue
		}

		best := &reduced.Results[pos]
		if r.Package != "" && best.Package == "" {
			best.Package = r.Package
		}
		if r.Iterations > best.Iterations {
			best.Iterations = r.Iterations
		}
		mergeMin(&best.NsPerOp, r.NsPerOp, best.hasParsedMetric("ns/op"), r.hasParsedMetric("ns/op"))
		mergeMinInt(&best.BytesPerOp, r.BytesPerOp, best.hasParsedMetric("B/op"), r.hasParsedMetric("B/op"))
		mergeMinInt(&best.AllocsPerOp, r.AllocsPerOp, best.hasParsedMetric("allocs/op"), r.hasParsedMetric("allocs/op"))
		if r.hasParsedMetric("events/sec") && (!best.hasParsedMetric("events/sec") || r.EventsPerSec > best.EventsPerSec) {
			best.EventsPerSec = r.EventsPerSec
		}
		if best.parsedMetrics != nil && r.parsedMetrics != nil {
			for unit := range r.parsedMetrics {
				best.parsedMetrics[unit] = struct{}{}
			}
		}
	}

	return reduced
}

func mergeMin(dst *float64, candidate float64, haveDst, haveCandidate bool) {
	if haveCandidate && (!haveDst || candidate < *dst) {
		*dst = candidate
	}
}

func mergeMinInt(dst *int64, candidate int64, haveDst, haveCandidate bool) {
	if haveCandidate && (!haveDst || candidate < *dst) {
		*dst = candidate
	}
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

// LatencyMarginFactor is the multiple of a profile's calibrated median that a
// latency ceiling is set to (and the divisor for a throughput floor).
//
// It is derived, not chosen by feel. Backtesting 87 benchmark.txt artifacts
// from CI (2026-05-22..2026-08-08) decomposes the observed spread into:
//
//   - hardware class: up to 5.3x, handled by selecting a CPUProfile;
//   - whole-run contention: p99 1.37x of the run's own class median;
//   - single-benchmark jitter bursts: up to 1.55x.
//
// 1.6 clears all three. Widening further buys nothing: from 1.6 through 2.5 the
// backtest failure count is flat at one run, an outlier whose spikes reach 5x
// and which no ceiling can absorb (see the re-measure path in cmd/bench-check).
// So every 0.1 above 1.6 is pure loss of regression sensitivity.
const LatencyMarginFactor = 1.6

// CPUProfile calibrates latency and throughput bounds for one class of CI
// hardware. GitLab schedules test:benchmark onto any node in the k3s pool and
// those nodes are not comparable: the same commit measures ~1.9µs/op on a Ryzen
// 7900X3D and ~7.8µs/op on an emulated Broadwell VM. A single absolute ceiling
// cannot be tight on the fast nodes and non-flaky on the slow ones, so the CPU
// model reported by "go test" selects the bounds.
type CPUProfile struct {
	// ID is a stable short name used in CI output.
	ID string
	// CPUModels lists exact "cpu:" strings that select this profile.
	CPUModels []string
	// MedianNsPerOp records the calibration anchors the ceilings derive from,
	// so a recalibration can be checked against the run that produced it.
	MedianNsPerOp map[string]float64
	MaxNsPerOp    map[string]float64
	MinThroughput map[string]float64
}

// workflowAllocCeilings bounds allocations per op. Allocation counts are a
// property of the code, not the machine: in every calibration artifact that
// measured them (78 to 87 runs depending on the benchmark), spanning all three
// CPU classes, each of these reported a bit-identical value. That makes this
// the sharpest and only flake-free part of the gate, so the ceilings sit just
// above the observed counts rather than at the 2x slack they previously
// carried.
var workflowAllocCeilings = map[string]int64{
	"BenchmarkEngineProcess":         28, // observed 24
	"BenchmarkCELEvaluate_Simple":    5,  // observed 3
	"BenchmarkFilterMatch_EventType": 25, // observed 21
	"BenchmarkTransform_SetField":    7,  // observed 5
	"BenchmarkThroughput_Simple":     25, // observed 21
	"BenchmarkThroughput_Complex":    58, // observed 50
}

// workflowCPUProfiles lists the calibrated CI hardware classes.
//
// Ceilings are LatencyMarginFactor x the median observed for that class, and
// throughput floors are the median divided by it, each rounded outward. To add
// or recalibrate a class, collect recent benchmark.txt artifacts from GitLab
// and paste what this emits:
//
//	go run ./cmd/bench-check -suggest artifacts/*.txt
//
// Calibrated 2026-08-08 from the 78 parseable test:benchmark artifacts
// available at the time (2026-05-22..2026-08-08): 13 Ryzen, 53 Xeon,
// 12 Broadwell runs.
var workflowCPUProfiles = []CPUProfile{
	{
		ID:        "ryzen-7900x3d",
		CPUModels: []string{"AMD Ryzen 9 7900X3D 12-Core Processor"},
		MedianNsPerOp: map[string]float64{
			"BenchmarkEngineProcess":         1869,
			"BenchmarkCELEvaluate_Simple":    252,
			"BenchmarkFilterMatch_EventType": 772,
			"BenchmarkTransform_SetField":    308,
		},
		MaxNsPerOp: map[string]float64{
			"BenchmarkEngineProcess":         3000,
			"BenchmarkCELEvaluate_Simple":    450,
			"BenchmarkFilterMatch_EventType": 1300,
			"BenchmarkTransform_SetField":    500,
		},
		MinThroughput: map[string]float64{
			"BenchmarkThroughput_Simple":  726000,
			"BenchmarkThroughput_Complex": 219000,
		},
	},
	{
		ID:        "xeon-e5-2680-v4",
		CPUModels: []string{"Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz"},
		MedianNsPerOp: map[string]float64{
			"BenchmarkEngineProcess":         6231,
			"BenchmarkCELEvaluate_Simple":    1020,
			"BenchmarkFilterMatch_EventType": 3174,
			"BenchmarkTransform_SetField":    1208,
		},
		MaxNsPerOp: map[string]float64{
			"BenchmarkEngineProcess":         10000,
			"BenchmarkCELEvaluate_Simple":    1700,
			"BenchmarkFilterMatch_EventType": 5100,
			"BenchmarkTransform_SetField":    2000,
		},
		MinThroughput: map[string]float64{
			"BenchmarkThroughput_Simple":  205000,
			"BenchmarkThroughput_Complex": 60000,
		},
	},
	{
		// Emulated QEMU CPU; the slowest class in the pool and therefore the
		// fallback for hardware that has not been calibrated yet.
		ID:        "qemu-broadwell",
		CPUModels: []string{"Intel Core Processor (Broadwell, IBRS)"},
		MedianNsPerOp: map[string]float64{
			"BenchmarkEngineProcess":         7842,
			"BenchmarkCELEvaluate_Simple":    1076,
			"BenchmarkFilterMatch_EventType": 3500,
			"BenchmarkTransform_SetField":    1497,
		},
		MaxNsPerOp: map[string]float64{
			"BenchmarkEngineProcess":         12600,
			"BenchmarkCELEvaluate_Simple":    1800,
			"BenchmarkFilterMatch_EventType": 5700,
			"BenchmarkTransform_SetField":    2400,
		},
		MinThroughput: map[string]float64{
			"BenchmarkThroughput_Simple":  163000,
			"BenchmarkThroughput_Complex": 49000,
		},
	},
}

// FallbackCPUProfileID names the profile used for unrecognized hardware.
const FallbackCPUProfileID = "qemu-broadwell"

// WorkflowCPUProfiles returns the calibrated CI hardware classes.
func WorkflowCPUProfiles() []CPUProfile {
	out := make([]CPUProfile, len(workflowCPUProfiles))
	copy(out, workflowCPUProfiles)
	return out
}

// ResolveWorkflowThresholds returns the thresholds calibrated for cpu, the
// profile they came from, and whether cpu was recognized.
//
// Unrecognized hardware falls back to the slowest calibrated profile rather
// than failing closed: a new node type appearing in the pool is an
// infrastructure change, and turning that into a red main pipeline would
// reintroduce exactly the flake this gate is meant to remove. Allocation
// ceilings still apply in full because they do not depend on the machine.
// Callers are expected to surface the miss so the profile gets calibrated.
func ResolveWorkflowThresholds(cpu string) (*PerformanceThresholds, CPUProfile, bool) {
	profile, matched := lookupCPUProfile(cpu)

	maxNs := make(map[string]float64, len(profile.MaxNsPerOp))
	for name, v := range profile.MaxNsPerOp {
		maxNs[name] = v
	}
	minThroughput := make(map[string]float64, len(profile.MinThroughput))
	for name, v := range profile.MinThroughput {
		minThroughput[name] = v
	}
	maxAllocs := make(map[string]int64, len(workflowAllocCeilings))
	for name, v := range workflowAllocCeilings {
		maxAllocs[name] = v
	}

	return &PerformanceThresholds{
		MaxNsPerOp:     maxNs,
		MaxAllocsPerOp: maxAllocs,
		MinThroughput:  minThroughput,
	}, profile, matched
}

func lookupCPUProfile(cpu string) (CPUProfile, bool) {
	cpu = strings.TrimSpace(cpu)
	var fallback CPUProfile
	for _, p := range workflowCPUProfiles {
		if p.ID == FallbackCPUProfileID {
			fallback = p
		}
		if cpu == "" {
			continue
		}
		for _, model := range p.CPUModels {
			if strings.EqualFold(cpu, model) {
				return p, true
			}
		}
	}
	return fallback, false
}

// DefaultWorkflowThresholds returns the thresholds applied when the CPU model
// is unknown. Prefer ResolveWorkflowThresholds, which picks the profile that
// matches the machine the benchmarks actually ran on.
func DefaultWorkflowThresholds() *PerformanceThresholds {
	t, _, _ := ResolveWorkflowThresholds("")
	return t
}

// Subset returns a copy of t restricted to the named benchmarks. It is used to
// re-validate a handful of benchmarks after re-measuring them, without the
// missing-result checks firing for every benchmark that was not re-run.
func (t *PerformanceThresholds) Subset(names []string) *PerformanceThresholds {
	keep := make(map[string]struct{}, len(names))
	for _, n := range names {
		keep[n] = struct{}{}
	}

	sub := &PerformanceThresholds{
		MaxNsPerOp:     make(map[string]float64),
		MaxAllocsPerOp: make(map[string]int64),
		MinThroughput:  make(map[string]float64),
	}
	for name, v := range t.MaxNsPerOp {
		if _, ok := keep[name]; ok {
			sub.MaxNsPerOp[name] = v
		}
	}
	for name, v := range t.MaxAllocsPerOp {
		if _, ok := keep[name]; ok {
			sub.MaxAllocsPerOp[name] = v
		}
	}
	for name, v := range t.MinThroughput {
		if _, ok := keep[name]; ok {
			sub.MinThroughput[name] = v
		}
	}
	return sub
}

// ThresholdViolation is a single failed threshold check.
type ThresholdViolation struct {
	// Benchmark is the benchmark name, with the -N CPU suffix already stripped.
	Benchmark string
	// Metric is the unit that failed ("ns/op", "allocs/op", "events/sec"), or
	// empty when the benchmark produced no result at all.
	Metric string
	// Message is the human-readable description used in CI output.
	Message string
}

// Validate checks if a benchmark suite meets the performance thresholds.
func (t *PerformanceThresholds) Validate(suite *BenchmarkSuite) []string {
	checks := t.Check(suite)
	messages := make([]string, 0, len(checks))
	for _, v := range checks {
		messages = append(messages, v.Message)
	}
	return messages
}

// Check is Validate with the offending benchmark and metric preserved, so a
// caller can re-measure exactly what failed instead of the whole suite.
func (t *PerformanceThresholds) Check(suite *BenchmarkSuite) []ThresholdViolation {
	var violations []ThresholdViolation
	add := func(name, metric, format string, args ...interface{}) {
		violations = append(violations, ThresholdViolation{
			Benchmark: name,
			Metric:    metric,
			Message:   fmt.Sprintf(format, args...),
		})
	}
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
			add(name, "", "%s: required benchmark result missing", name)
		}
	}

	for _, r := range suite.Results {
		// Check ns/op thresholds
		if maxNs, ok := t.MaxNsPerOp[r.Name]; ok {
			if !r.hasParsedMetric("ns/op") {
				add(r.Name, "ns/op", "%s: required ns/op metric missing", r.Name)
			} else if r.NsPerOp > maxNs {
				add(r.Name, "ns/op", "%s: %.0f ns/op exceeds threshold of %.0f ns/op",
					r.Name, r.NsPerOp, maxNs)
			}
		}

		// Check allocs/op thresholds
		if maxAllocs, ok := t.MaxAllocsPerOp[r.Name]; ok {
			if !r.hasParsedMetric("allocs/op") {
				add(r.Name, "allocs/op", "%s: required allocs/op metric missing", r.Name)
			} else if r.AllocsPerOp > maxAllocs {
				add(r.Name, "allocs/op", "%s: %d allocs/op exceeds threshold of %d allocs/op",
					r.Name, r.AllocsPerOp, maxAllocs)
			}
		}

		// Check throughput thresholds
		if minThroughput, ok := t.MinThroughput[r.Name]; ok {
			if !r.hasParsedMetric("events/sec") {
				add(r.Name, "events/sec", "%s: required events/sec metric missing", r.Name)
			} else if r.EventsPerSec < minThroughput {
				add(r.Name, "events/sec", "%s: %.0f events/sec below threshold of %.0f events/sec",
					r.Name, r.EventsPerSec, minThroughput)
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
