package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

const testCPU = "Intel Core Processor (Broadwell, IBRS)"

const testPkg = "gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"

// benchOutput renders one package's worth of "go test -bench" output.
func benchOutput(cpu string, lines ...string) string {
	var b strings.Builder
	b.WriteString("goos: linux\ngoarch: amd64\n")
	fmt.Fprintf(&b, "pkg: %s\ncpu: %s\n", testPkg, cpu)
	for _, l := range lines {
		b.WriteString(l + "\n")
	}
	b.WriteString("PASS\nok  \t" + testPkg + "\t1.0s\n")
	return b.String()
}

// slowSuite is a run where BenchmarkEngineProcess breached its Broadwell
// ceiling of 12600 ns/op.
func slowSuite(t *testing.T) (*workflow.BenchmarkSuite, *workflow.PerformanceThresholds, []workflow.ThresholdViolation) {
	t.Helper()

	suite, err := workflow.ParseBenchmarkOutput(strings.NewReader(benchOutput(testCPU,
		"BenchmarkEngineProcess-2    50000    30000 ns/op    888 B/op    24 allocs/op",
	)), "current")
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}

	thresholds, _, matched := workflow.ResolveWorkflowThresholds(suite.CPU)
	if !matched {
		t.Fatalf("fixture CPU %q is not calibrated", suite.CPU)
	}
	thresholds = thresholds.Subset([]string{"BenchmarkEngineProcess"})

	violations := thresholds.Check(suite)
	if len(violations) != 1 {
		t.Fatalf("fixture should breach exactly one threshold, got %v", violations)
	}
	return suite, thresholds, violations
}

func TestConfirmViolations_TransientBurstIsCleared(t *testing.T) {
	suite, thresholds, violations := slowSuite(t)

	var gotPkg string
	var gotNames []string
	var gotCount int
	runner := func(pkg string, names []string, count int) ([]byte, error) {
		gotPkg, gotNames, gotCount = pkg, names, count
		return []byte(benchOutput(testCPU,
			"BenchmarkEngineProcess-2    100000    7900 ns/op    888 B/op    24 allocs/op",
			"BenchmarkEngineProcess-2    100000    8100 ns/op    888 B/op    24 allocs/op",
		)), nil
	}

	var out, errOut strings.Builder
	confirmed, err := confirmViolations(&out, &errOut, thresholds, suite, violations, 2, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(confirmed) != 0 {
		t.Errorf("expected the violation to clear, got %v", confirmed)
	}

	// Only the offending benchmark is re-run: the point of the mechanism is
	// that confirmation costs seconds, not another full suite.
	if gotPkg != testPkg {
		t.Errorf("re-ran package %q, want %q", gotPkg, testPkg)
	}
	if len(gotNames) != 1 || gotNames[0] != "BenchmarkEngineProcess" {
		t.Errorf("re-ran %v, want only BenchmarkEngineProcess", gotNames)
	}
	if gotCount != 2 {
		t.Errorf("re-ran %d times, want 2", gotCount)
	}
}

func TestConfirmViolations_RealRegressionIsConfirmed(t *testing.T) {
	suite, thresholds, violations := slowSuite(t)

	runner := func(pkg string, names []string, count int) ([]byte, error) {
		// A real regression reproduces on every repetition.
		return []byte(benchOutput(testCPU,
			"BenchmarkEngineProcess-2    40000    29500 ns/op    888 B/op    24 allocs/op",
			"BenchmarkEngineProcess-2    40000    30100 ns/op    888 B/op    24 allocs/op",
		)), nil
	}

	var out, errOut strings.Builder
	confirmed, err := confirmViolations(&out, &errOut, thresholds, suite, violations, 2, runner)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(confirmed) != 1 {
		t.Fatalf("expected the regression to be confirmed, got %v", confirmed)
	}
	if confirmed[0].Benchmark != "BenchmarkEngineProcess" || confirmed[0].Metric != "ns/op" {
		t.Errorf("confirmed %+v, want BenchmarkEngineProcess/ns/op", confirmed[0])
	}
}

// A confirmation is only meaningful on the hardware the thresholds were chosen
// for. Without this guard, re-running a CI artifact's violation on a faster
// machine would clear a genuine regression.
func TestConfirmViolations_RejectsDifferentHardware(t *testing.T) {
	suite, thresholds, violations := slowSuite(t)

	runner := func(pkg string, names []string, count int) ([]byte, error) {
		return []byte(benchOutput("Apple M4",
			"BenchmarkEngineProcess-10    2000000    480 ns/op    888 B/op    24 allocs/op",
		)), nil
	}

	var out, errOut strings.Builder
	_, err := confirmViolations(&out, &errOut, thresholds, suite, violations, 1, runner)
	if err == nil {
		t.Fatal("expected an error when the re-measurement runs on other hardware")
	}
	if !strings.Contains(err.Error(), "Apple M4") {
		t.Errorf("error should name the mismatched CPU, got: %v", err)
	}
}

func TestConfirmViolations_RunnerFailurePreservesVerdict(t *testing.T) {
	suite, thresholds, violations := slowSuite(t)

	runner := func(pkg string, names []string, count int) ([]byte, error) {
		return nil, fmt.Errorf("build failed")
	}

	var out, errOut strings.Builder
	if _, err := confirmViolations(&out, &errOut, thresholds, suite, violations, 1, runner); err == nil {
		t.Fatal("expected an error when the re-measurement cannot run")
	}
}

func TestConfirmViolations_MissingResultIsNotReMeasurable(t *testing.T) {
	// A required benchmark that produced no output has no package recorded,
	// so there is nothing to target for a re-run.
	suite := workflow.NewBenchmarkSuite("current")
	suite.CPU = testCPU
	thresholds, _, _ := workflow.ResolveWorkflowThresholds(testCPU)
	thresholds = thresholds.Subset([]string{"BenchmarkEngineProcess"})

	violations := thresholds.Check(suite)
	if len(violations) != 1 || violations[0].Metric != "" {
		t.Fatalf("expected one missing-result violation, got %v", violations)
	}

	called := false
	runner := func(pkg string, names []string, count int) ([]byte, error) {
		called = true
		return nil, nil
	}

	var out, errOut strings.Builder
	if _, err := confirmViolations(&out, &errOut, thresholds, suite, violations, 1, runner); err == nil {
		t.Fatal("expected an error for an unre-measurable violation")
	}
	if called {
		t.Error("runner should not be invoked when the package is unknown")
	}
}

func TestRun_ReportsProfileAndPasses(t *testing.T) {
	// 7842 ns/op is the Broadwell median: an ordinary result there, and one
	// that the previous flat 12000 ns/op ceiling also accepted.
	input := benchOutput(testCPU,
		"BenchmarkEngineProcess-2    100000    7842 ns/op    888 B/op    24 allocs/op",
	)

	var out, errOut strings.Builder
	code := run([]string{}, &out, &errOut, strings.NewReader(input))

	// The suite is missing most gated benchmarks, so it must not pass; what
	// this asserts is that the resolved profile is reported either way.
	if code == 0 {
		t.Error("a partial suite should not pass the gate")
	}
	if !strings.Contains(out.String(), "Profile: qemu-broadwell") {
		t.Errorf("output should name the resolved profile, got:\n%s", out.String())
	}
	if strings.Contains(out.String(), "WARNING: this CPU model") {
		t.Error("a calibrated CPU must not warn about missing calibration")
	}
}

func TestRun_WarnsOnUncalibratedCPU(t *testing.T) {
	input := benchOutput("Some Brand New CPU",
		"BenchmarkEngineProcess-2    100000    7842 ns/op    888 B/op    24 allocs/op",
	)

	var out, errOut strings.Builder
	run([]string{}, &out, &errOut, strings.NewReader(input))

	if !strings.Contains(out.String(), "WARNING: this CPU model") {
		t.Errorf("uncalibrated hardware must warn, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "Profile: "+workflow.FallbackCPUProfileID) {
		t.Errorf("uncalibrated hardware must fall back to %s, got:\n%s",
			workflow.FallbackCPUProfileID, out.String())
	}
}

func TestRun_NoResults(t *testing.T) {
	var out, errOut strings.Builder
	if code := run([]string{}, &out, &errOut, strings.NewReader("nothing here\n")); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "no benchmark results") {
		t.Errorf("stderr should explain the empty input, got: %s", errOut.String())
	}
}

func TestRunSuggest_EmitsProfileForEachCPU(t *testing.T) {
	dir := t.TempDir()
	paths := []string{
		writeTemp(t, dir, "a.txt", benchOutput(testCPU,
			"BenchmarkEngineProcess-2    100000    8000 ns/op    888 B/op    24 allocs/op",
			"BenchmarkThroughput_Simple-2    100000    4000 ns/op    250000 events/sec    840 B/op    21 allocs/op",
		)),
		writeTemp(t, dir, "b.txt", benchOutput(testCPU,
			"BenchmarkEngineProcess-2    100000    7000 ns/op    888 B/op    24 allocs/op",
			"BenchmarkThroughput_Simple-2    100000    4000 ns/op    270000 events/sec    840 B/op    21 allocs/op",
		)),
	}

	var out, errOut strings.Builder
	if code := runSuggest(paths, &out, &errOut); code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr: %s", code, errOut.String())
	}

	got := out.String()
	if !strings.Contains(got, testCPU) {
		t.Errorf("output should name the CPU, got:\n%s", got)
	}
	// median(7000, 8000) = 7500, x1.6 = 12000.
	if !strings.Contains(got, `"BenchmarkEngineProcess": 12000`) {
		t.Errorf("expected a ceiling of 12000 from the two samples, got:\n%s", got)
	}
	// median(250000, 270000) = 260000, /1.6 = 162500 -> floored to 162000.
	if !strings.Contains(got, `"BenchmarkThroughput_Simple": 162000`) {
		t.Errorf("expected a floor of 162000 from the two samples, got:\n%s", got)
	}
	if !strings.Contains(got, "too few for a stable median") {
		t.Error("two samples should carry a low-confidence warning")
	}
}

func TestRunSuggest_NoInput(t *testing.T) {
	var out, errOut strings.Builder
	if code := runSuggest(nil, &out, &errOut); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
	return path
}
