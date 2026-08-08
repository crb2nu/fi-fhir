package workflow

import (
	"strings"
	"testing"
	"time"
)

func TestBenchmarkSuite_AddResult(t *testing.T) {
	suite := NewBenchmarkSuite("test")

	suite.AddResult(BenchmarkResult{
		Name:        "BenchmarkTest",
		Iterations:  1000,
		NsPerOp:     100.5,
		BytesPerOp:  256,
		AllocsPerOp: 5,
	})

	if len(suite.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(suite.Results))
	}

	result := suite.GetResult("BenchmarkTest")
	if result == nil {
		t.Fatal("Expected to find result")
	}
	if result.NsPerOp != 100.5 {
		t.Errorf("Expected NsPerOp 100.5, got %f", result.NsPerOp)
	}
}

func TestBenchmarkSuite_GetResult_NotFound(t *testing.T) {
	suite := NewBenchmarkSuite("test")

	result := suite.GetResult("NonExistent")
	if result != nil {
		t.Error("Expected nil for non-existent result")
	}
}

func TestBenchmarkSuite_Summary(t *testing.T) {
	suite := NewBenchmarkSuite("test-suite")

	suite.AddResult(BenchmarkResult{
		Name:         "BenchmarkA",
		Iterations:   1000,
		NsPerOp:      100.0,
		BytesPerOp:   256,
		AllocsPerOp:  5,
		EventsPerSec: 10000000,
	})

	suite.AddResult(BenchmarkResult{
		Name:        "BenchmarkB",
		Iterations:  500,
		NsPerOp:     200.0,
		BytesPerOp:  512,
		AllocsPerOp: 10,
	})

	summary := suite.Summary()

	// Check that key elements are present
	if !strings.Contains(summary, "test-suite") {
		t.Error("Summary should contain suite name")
	}
	if !strings.Contains(summary, "BenchmarkA") {
		t.Error("Summary should contain BenchmarkA")
	}
	if !strings.Contains(summary, "BenchmarkB") {
		t.Error("Summary should contain BenchmarkB")
	}
	if !strings.Contains(summary, "10000000") || !strings.Contains(summary, "events/sec") {
		t.Error("Summary should contain throughput")
	}
}

func TestBenchmarkComparison(t *testing.T) {
	baseline := NewBenchmarkSuite("baseline")
	baseline.AddResult(BenchmarkResult{
		Name:        "BenchmarkTest",
		NsPerOp:     100.0,
		AllocsPerOp: 5,
	})

	current := NewBenchmarkSuite("current")
	current.AddResult(BenchmarkResult{
		Name:        "BenchmarkTest",
		NsPerOp:     80.0, // 20% faster
		AllocsPerOp: 5,
	})

	comparison := Compare(baseline, current)

	if len(comparison.Diffs) != 1 {
		t.Fatalf("Expected 1 diff, got %d", len(comparison.Diffs))
	}

	diff := comparison.Diffs[0]
	if !diff.Improved {
		t.Error("Expected benchmark to be marked as improved")
	}
	if diff.Regressed {
		t.Error("Expected benchmark NOT to be marked as regressed")
	}

	// 80 is 20% less than 100, so delta should be -20
	expectedDelta := -20.0
	if diff.NsPerOpDelta < expectedDelta-0.1 || diff.NsPerOpDelta > expectedDelta+0.1 {
		t.Errorf("Expected delta around -20%%, got %.1f%%", diff.NsPerOpDelta)
	}
}

func TestBenchmarkComparison_Regression(t *testing.T) {
	baseline := NewBenchmarkSuite("baseline")
	baseline.AddResult(BenchmarkResult{
		Name:    "BenchmarkTest",
		NsPerOp: 100.0,
	})

	current := NewBenchmarkSuite("current")
	current.AddResult(BenchmarkResult{
		Name:    "BenchmarkTest",
		NsPerOp: 120.0, // 20% slower
	})

	comparison := Compare(baseline, current)

	if !comparison.HasRegressions() {
		t.Error("Expected regression to be detected")
	}

	diff := comparison.Diffs[0]
	if !diff.Regressed {
		t.Error("Expected benchmark to be marked as regressed")
	}
}

func TestBenchmarkComparison_Summary(t *testing.T) {
	baseline := NewBenchmarkSuite("v1.0")
	baseline.AddResult(BenchmarkResult{Name: "BenchmarkA", NsPerOp: 100.0})
	baseline.AddResult(BenchmarkResult{Name: "BenchmarkB", NsPerOp: 200.0})

	current := NewBenchmarkSuite("v1.1")
	current.AddResult(BenchmarkResult{Name: "BenchmarkA", NsPerOp: 80.0})  // Improved
	current.AddResult(BenchmarkResult{Name: "BenchmarkB", NsPerOp: 250.0}) // Regressed

	comparison := Compare(baseline, current)
	summary := comparison.Summary()

	if !strings.Contains(summary, "FASTER") {
		t.Error("Summary should indicate improved benchmark")
	}
	if !strings.Contains(summary, "SLOWER") {
		t.Error("Summary should indicate regressed benchmark")
	}
	if !strings.Contains(summary, "1 improved") {
		t.Error("Summary should show 1 improved")
	}
	if !strings.Contains(summary, "1 regressed") {
		t.Error("Summary should show 1 regressed")
	}
}

func TestPerformanceThresholds_Validate(t *testing.T) {
	// Test passing case
	t.Run("passing", func(t *testing.T) {
		thresholds := &PerformanceThresholds{
			MaxNsPerOp: map[string]float64{
				"BenchmarkFast": 100,
			},
			MaxAllocsPerOp: map[string]int64{
				"BenchmarkFast": 5,
			},
			MinThroughput: map[string]float64{
				"BenchmarkFast": 1000000,
			},
		}

		suite := NewBenchmarkSuite("test")
		suite.AddResult(BenchmarkResult{
			Name:         "BenchmarkFast",
			NsPerOp:      50,
			AllocsPerOp:  3,
			EventsPerSec: 2000000,
		})

		violations := thresholds.Validate(suite)
		if len(violations) != 0 {
			t.Errorf("Expected no violations, got: %v", violations)
		}
	})

	// Test ns/op violation (only checking ns/op threshold)
	t.Run("ns_violation", func(t *testing.T) {
		thresholds := &PerformanceThresholds{
			MaxNsPerOp: map[string]float64{
				"BenchmarkSlow": 100,
			},
		}

		suite := NewBenchmarkSuite("test")
		suite.AddResult(BenchmarkResult{
			Name:    "BenchmarkSlow",
			NsPerOp: 200, // Exceeds 100
		})

		violations := thresholds.Validate(suite)
		if len(violations) != 1 {
			t.Fatalf("Expected 1 violation, got %d: %v", len(violations), violations)
		}
		if !strings.Contains(violations[0], "exceeds threshold") {
			t.Errorf("Expected threshold violation message, got: %s", violations[0])
		}
	})

	// Test allocs violation (only checking allocs threshold)
	t.Run("allocs_violation", func(t *testing.T) {
		thresholds := &PerformanceThresholds{
			MaxAllocsPerOp: map[string]int64{
				"BenchmarkAllocy": 5,
			},
		}

		suite := NewBenchmarkSuite("test")
		suite.AddResult(BenchmarkResult{
			Name:        "BenchmarkAllocy",
			AllocsPerOp: 10, // Exceeds 5
		})

		violations := thresholds.Validate(suite)
		if len(violations) != 1 {
			t.Fatalf("Expected 1 violation, got %d: %v", len(violations), violations)
		}
		if !strings.Contains(violations[0], "allocs/op exceeds") {
			t.Errorf("Expected allocs violation message, got: %s", violations[0])
		}
	})

	// Test throughput violation (only checking throughput threshold)
	t.Run("throughput_violation", func(t *testing.T) {
		thresholds := &PerformanceThresholds{
			MinThroughput: map[string]float64{
				"BenchmarkThroughput": 1000000,
			},
		}

		suite := NewBenchmarkSuite("test")
		suite.AddResult(BenchmarkResult{
			Name:         "BenchmarkThroughput",
			EventsPerSec: 500000, // Below 1000000
		})

		violations := thresholds.Validate(suite)
		if len(violations) != 1 {
			t.Fatalf("Expected 1 violation, got %d: %v", len(violations), violations)
		}
		if !strings.Contains(violations[0], "below threshold") {
			t.Errorf("Expected throughput violation message, got: %s", violations[0])
		}
	})

	t.Run("missing_required_benchmark", func(t *testing.T) {
		thresholds := &PerformanceThresholds{
			MaxNsPerOp: map[string]float64{
				"BenchmarkRequired": 100,
			},
		}

		violations := thresholds.Validate(NewBenchmarkSuite("test"))
		if len(violations) != 1 {
			t.Fatalf("Expected 1 violation, got %d: %v", len(violations), violations)
		}
		if !strings.Contains(violations[0], "required benchmark result missing") {
			t.Errorf("Expected missing benchmark violation, got: %s", violations[0])
		}
	})

	t.Run("parsed_result_missing_required_metric", func(t *testing.T) {
		result, err := parseBenchLine("BenchmarkRequired-8 1000 50 ns/op")
		if err != nil {
			t.Fatalf("parseBenchLine returned error: %v", err)
		}

		suite := NewBenchmarkSuite("test")
		suite.AddResult(result)
		thresholds := &PerformanceThresholds{
			MaxNsPerOp:     map[string]float64{"BenchmarkRequired": 100},
			MaxAllocsPerOp: map[string]int64{"BenchmarkRequired": 1},
		}

		violations := thresholds.Validate(suite)
		if len(violations) != 1 {
			t.Fatalf("Expected 1 violation, got %d: %v", len(violations), violations)
		}
		if !strings.Contains(violations[0], "required allocs/op metric missing") {
			t.Errorf("Expected missing metric violation, got: %s", violations[0])
		}
	})
}

func TestDefaultWorkflowThresholds(t *testing.T) {
	thresholds := DefaultWorkflowThresholds()

	for _, name := range []string{
		"BenchmarkEngineProcess",
		"BenchmarkCELEvaluate_Simple",
		"BenchmarkFilterMatch_EventType",
		"BenchmarkTransform_SetField",
	} {
		if _, ok := thresholds.MaxNsPerOp[name]; !ok {
			t.Errorf("Expected a latency ceiling for %s", name)
		}
		if _, ok := thresholds.MaxAllocsPerOp[name]; !ok {
			t.Errorf("Expected an allocation ceiling for %s", name)
		}
	}
	for _, name := range []string{"BenchmarkThroughput_Simple", "BenchmarkThroughput_Complex"} {
		if _, ok := thresholds.MinThroughput[name]; !ok {
			t.Errorf("Expected a throughput floor for %s", name)
		}
	}
	if _, ok := thresholds.MaxNsPerOp["BenchmarkFilterMatch"]; ok {
		t.Error("Unexpected threshold for nonexistent BenchmarkFilterMatch")
	}

	// An unknown CPU must resolve to the slowest profile, so that new CI
	// hardware cannot turn a green pipeline red before it has been calibrated.
	fallback, matched := lookupCPUProfile("some CPU nobody has calibrated")
	if matched {
		t.Error("Unknown CPU must not report a profile match")
	}
	if fallback.ID != FallbackCPUProfileID {
		t.Errorf("Unknown CPU resolved to %q, want the fallback %q", fallback.ID, FallbackCPUProfileID)
	}
	for name, ceiling := range thresholds.MaxNsPerOp {
		for _, p := range WorkflowCPUProfiles() {
			if p.MaxNsPerOp[name] > ceiling {
				t.Errorf("%s: fallback ceiling %.0f is below profile %s (%.0f); the fallback must be the most permissive",
					name, ceiling, p.ID, p.MaxNsPerOp[name])
			}
		}
	}
}

// TestWorkflowCPUProfiles_Calibration guards the shape of the profile table
// rather than its literal values, so a recalibration does not have to rewrite
// the test — only a mistake does.
func TestWorkflowCPUProfiles_Calibration(t *testing.T) {
	profiles := WorkflowCPUProfiles()
	if len(profiles) == 0 {
		t.Fatal("no CPU profiles defined")
	}

	reference := profiles[0]
	seenIDs := make(map[string]struct{})
	seenModels := make(map[string]string)

	for _, p := range profiles {
		if _, dup := seenIDs[p.ID]; dup {
			t.Errorf("duplicate profile ID %q", p.ID)
		}
		seenIDs[p.ID] = struct{}{}

		if len(p.CPUModels) == 0 {
			t.Errorf("profile %s matches no CPU model", p.ID)
		}
		for _, model := range p.CPUModels {
			if other, dup := seenModels[model]; dup {
				t.Errorf("CPU model %q claimed by both %s and %s", model, other, p.ID)
			}
			seenModels[model] = p.ID
		}

		// Every profile must gate the same benchmarks, or a job would be
		// silently ungated depending on which node it landed on.
		if len(p.MaxNsPerOp) != len(reference.MaxNsPerOp) {
			t.Errorf("profile %s gates %d latency benchmarks, reference %s gates %d",
				p.ID, len(p.MaxNsPerOp), reference.ID, len(reference.MaxNsPerOp))
		}
		for name := range reference.MaxNsPerOp {
			if _, ok := p.MaxNsPerOp[name]; !ok {
				t.Errorf("profile %s is missing a latency ceiling for %s", p.ID, name)
			}
		}
		for name := range reference.MinThroughput {
			if _, ok := p.MinThroughput[name]; !ok {
				t.Errorf("profile %s is missing a throughput floor for %s", p.ID, name)
			}
		}

		// Each ceiling must sit at the documented margin above its anchor.
		for name, ceiling := range p.MaxNsPerOp {
			median, ok := p.MedianNsPerOp[name]
			if !ok {
				t.Errorf("profile %s records no calibration median for %s", p.ID, name)
				continue
			}
			ratio := ceiling / median
			if ratio < LatencyMarginFactor || ratio > LatencyMarginFactor*1.25 {
				t.Errorf("profile %s: %s ceiling %.0f is %.2fx its median %.0f, want between %.2fx and %.2fx",
					p.ID, name, ceiling, ratio, median, LatencyMarginFactor, LatencyMarginFactor*1.25)
			}
		}
	}

	if _, ok := seenIDs[FallbackCPUProfileID]; !ok {
		t.Errorf("fallback profile %q is not defined", FallbackCPUProfileID)
	}
}

func TestResolveWorkflowThresholds(t *testing.T) {
	for _, p := range WorkflowCPUProfiles() {
		for _, model := range p.CPUModels {
			thresholds, got, matched := ResolveWorkflowThresholds(model)
			if !matched {
				t.Errorf("CPU %q did not match any profile", model)
			}
			if got.ID != p.ID {
				t.Errorf("CPU %q resolved to profile %s, want %s", model, got.ID, p.ID)
			}
			for name, want := range p.MaxNsPerOp {
				if thresholds.MaxNsPerOp[name] != want {
					t.Errorf("%s/%s: ceiling %.0f, want %.0f", p.ID, name, thresholds.MaxNsPerOp[name], want)
				}
			}
			// Allocation ceilings are a property of the code, so every
			// profile must enforce exactly the same ones.
			if len(thresholds.MaxAllocsPerOp) != len(workflowAllocCeilings) {
				t.Errorf("%s: got %d allocation ceilings, want %d",
					p.ID, len(thresholds.MaxAllocsPerOp), len(workflowAllocCeilings))
			}
		}
	}

	t.Run("mutating the result does not corrupt the table", func(t *testing.T) {
		model := WorkflowCPUProfiles()[0].CPUModels[0]
		first, _, _ := ResolveWorkflowThresholds(model)
		want := first.MaxNsPerOp["BenchmarkEngineProcess"]
		first.MaxNsPerOp["BenchmarkEngineProcess"] = 1
		first.MaxAllocsPerOp["BenchmarkEngineProcess"] = 1

		second, _, _ := ResolveWorkflowThresholds(model)
		if got := second.MaxNsPerOp["BenchmarkEngineProcess"]; got != want {
			t.Errorf("ceiling leaked between calls: got %.0f, want %.0f", got, want)
		}
		if second.MaxAllocsPerOp["BenchmarkEngineProcess"] == 1 {
			t.Error("allocation ceiling leaked between calls")
		}
	})
}

// TestGate_RegressionForPipeline22521 pins the incident this profile table was
// written for. Job 220702 measured BenchmarkEngineProcess at 12194 ns/op and
// failed the flat 12000 ns/op ceiling; the retry passed only because it landed
// on a Xeon node roughly 2.5x faster. 12194 is an ordinary result for the
// emulated Broadwell node it actually ran on and must not fail there, while a
// genuine regression on the fast node must still fail.
func TestGate_RegressionForPipeline22521(t *testing.T) {
	const broadwell = "Intel Core Processor (Broadwell, IBRS)"
	const ryzen = "AMD Ryzen 9 7900X3D 12-Core Processor"

	tests := []struct {
		name      string
		cpu       string
		nsPerOp   float64
		wantFail  bool
		rationale string
	}{
		{
			name:      "reported failure is normal for its hardware",
			cpu:       broadwell,
			nsPerOp:   12194,
			wantFail:  false,
			rationale: "job 220702, within noise for the emulated Broadwell node",
		},
		{
			name:      "genuine regression on slow hardware still fails",
			cpu:       broadwell,
			nsPerOp:   20000,
			wantFail:  true,
			rationale: "2.5x the Broadwell median",
		},
		{
			name:      "same 12194 is a regression on fast hardware",
			cpu:       ryzen,
			nsPerOp:   12194,
			wantFail:  true,
			rationale: "6.5x the Ryzen median; the old flat ceiling let this pass",
		},
		{
			name:      "normal fast-hardware result passes",
			cpu:       ryzen,
			nsPerOp:   1869,
			wantFail:  false,
			rationale: "the Ryzen median itself",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			thresholds, profile, matched := ResolveWorkflowThresholds(tc.cpu)
			if !matched {
				t.Fatalf("CPU %q is not calibrated", tc.cpu)
			}

			suite := NewBenchmarkSuite("test")
			suite.CPU = tc.cpu
			suite.AddResult(BenchmarkResult{
				Name:        "BenchmarkEngineProcess",
				NsPerOp:     tc.nsPerOp,
				AllocsPerOp: 24,
			})

			violations := thresholds.Subset([]string{"BenchmarkEngineProcess"}).Validate(suite)
			if got := len(violations) > 0; got != tc.wantFail {
				t.Errorf("profile %s at %.0f ns/op: failed=%v, want %v (%s); violations=%v",
					profile.ID, tc.nsPerOp, got, tc.wantFail, tc.rationale, violations)
			}
		})
	}
}

func TestPerformanceThresholds_Subset(t *testing.T) {
	full := &PerformanceThresholds{
		MaxNsPerOp:     map[string]float64{"A": 1, "B": 2},
		MaxAllocsPerOp: map[string]int64{"A": 3, "B": 4},
		MinThroughput:  map[string]float64{"A": 5, "B": 6},
	}

	sub := full.Subset([]string{"A"})
	if len(sub.MaxNsPerOp) != 1 || sub.MaxNsPerOp["A"] != 1 {
		t.Errorf("MaxNsPerOp = %v, want only A", sub.MaxNsPerOp)
	}
	if len(sub.MaxAllocsPerOp) != 1 || sub.MaxAllocsPerOp["A"] != 3 {
		t.Errorf("MaxAllocsPerOp = %v, want only A", sub.MaxAllocsPerOp)
	}
	if len(sub.MinThroughput) != 1 || sub.MinThroughput["A"] != 5 {
		t.Errorf("MinThroughput = %v, want only A", sub.MinThroughput)
	}

	// A subset must not demand results for the benchmarks it excluded.
	suite := NewBenchmarkSuite("test")
	suite.AddResult(BenchmarkResult{Name: "A", NsPerOp: 1, AllocsPerOp: 3, EventsPerSec: 5})
	if violations := sub.Validate(suite); len(violations) != 0 {
		t.Errorf("Expected no violations from the subset, got %v", violations)
	}
	if violations := full.Validate(suite); len(violations) == 0 {
		t.Error("Expected the full thresholds to report B as missing")
	}
}

func TestPerformanceThresholds_Check_ReportsBenchmarkAndMetric(t *testing.T) {
	thresholds := &PerformanceThresholds{
		MaxNsPerOp:    map[string]float64{"BenchmarkSlow": 100},
		MinThroughput: map[string]float64{"BenchmarkThin": 1000},
	}

	suite := NewBenchmarkSuite("test")
	suite.AddResult(BenchmarkResult{Name: "BenchmarkSlow", NsPerOp: 200})
	suite.AddResult(BenchmarkResult{Name: "BenchmarkThin", EventsPerSec: 10})

	got := make(map[string]string)
	for _, v := range thresholds.Check(suite) {
		got[v.Benchmark] = v.Metric
	}

	if got["BenchmarkSlow"] != "ns/op" {
		t.Errorf("BenchmarkSlow metric = %q, want ns/op", got["BenchmarkSlow"])
	}
	if got["BenchmarkThin"] != "events/sec" {
		t.Errorf("BenchmarkThin metric = %q, want events/sec", got["BenchmarkThin"])
	}
}

func TestBenchmarkSuite_ReduceToBest(t *testing.T) {
	input := `goos: linux
goarch: amd64
pkg: example.com/x
cpu: Test CPU
BenchmarkA-2   100   900 ns/op   500 B/op   9 allocs/op
BenchmarkA-2   300   400 ns/op   400 B/op   7 allocs/op
BenchmarkA-2   200   700 ns/op   450 B/op   8 allocs/op
BenchmarkB-2   100   50 ns/op   1000 events/sec   8 B/op   1 allocs/op
BenchmarkB-2   100   60 ns/op   3000 events/sec   8 B/op   1 allocs/op
`
	suite, err := ParseBenchmarkOutput(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("ParseBenchmarkOutput failed: %v", err)
	}
	if len(suite.Results) != 5 {
		t.Fatalf("got %d raw results, want 5", len(suite.Results))
	}

	best := suite.ReduceToBest()
	if len(best.Results) != 2 {
		t.Fatalf("got %d reduced results, want 2", len(best.Results))
	}
	if best.CPU != "Test CPU" {
		t.Errorf("CPU = %q, want %q", best.CPU, "Test CPU")
	}
	if best.Results[0].Name != "BenchmarkA" || best.Results[1].Name != "BenchmarkB" {
		t.Errorf("reduction changed benchmark order: %v", best.Results)
	}

	a := best.GetResult("BenchmarkA")
	if a.NsPerOp != 400 {
		t.Errorf("NsPerOp = %v, want the fastest 400", a.NsPerOp)
	}
	if a.BytesPerOp != 400 || a.AllocsPerOp != 7 {
		t.Errorf("B/op=%d allocs/op=%d, want the smallest 400/7", a.BytesPerOp, a.AllocsPerOp)
	}
	if a.Iterations != 300 {
		t.Errorf("Iterations = %d, want the highest 300", a.Iterations)
	}
	if a.Package != "example.com/x" {
		t.Errorf("Package = %q, want example.com/x", a.Package)
	}

	// Throughput is better when higher, unlike every other metric.
	if b := best.GetResult("BenchmarkB"); b.EventsPerSec != 3000 {
		t.Errorf("EventsPerSec = %v, want the highest 3000", b.EventsPerSec)
	}
}

func TestQuickBenchmark(t *testing.T) {
	workflow := &Workflow{
		Name: "test",
		Routes: []Route{
			{
				Name:    "route",
				Filter:  Filter{EventType: StringOrSlice{"test"}},
				Actions: []Action{{Type: "log"}},
			},
		},
	}

	engine, err := NewEngine(workflow)
	if err != nil {
		t.Fatalf("NewEngine failed: %v", err)
	}

	event := map[string]interface{}{"type": "test"}

	// Run a quick benchmark (10ms)
	opsPerSec := QuickBenchmark(engine, event, 10*time.Millisecond)

	// Should be able to process at least 1000 events/sec
	if opsPerSec < 1000 {
		t.Errorf("Expected at least 1000 ops/sec, got %.0f", opsPerSec)
	}
}

func TestTruncateName(t *testing.T) {
	tests := []struct {
		name     string
		maxLen   int
		expected string
	}{
		{"Short", 10, "Short"},
		{"ExactlyTen", 10, "ExactlyTen"},
		{"ThisNameIsWayTooLong", 10, "ThisNam..."},
		{"BenchmarkCELEvaluate_ConditionComplexity", 30, "BenchmarkCELEvaluate_Condit..."},
	}

	for _, tc := range tests {
		result := truncateName(tc.name, tc.maxLen)
		if result != tc.expected {
			t.Errorf("truncateName(%q, %d) = %q, want %q",
				tc.name, tc.maxLen, result, tc.expected)
		}
	}
}
