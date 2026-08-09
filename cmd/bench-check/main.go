// bench-check validates Go benchmark output against performance thresholds.
//
// Thresholds are selected from the CPU model recorded in the benchmark output,
// because CI schedules this job across heterogeneous hardware whose speeds
// differ by more than 5x. See internal/workflow/benchmark_util.go.
//
// Usage:
//
//	go test -bench=. -benchmem -run=^$ ./internal/workflow/... | tee benchmark.txt
//	go run ./cmd/bench-check < benchmark.txt
//	go run ./cmd/bench-check benchmark.txt
//	go run ./cmd/bench-check -confirm=3 benchmark.txt
//	go run ./cmd/bench-check -baseline=benchmark-baseline.txt benchmark.txt
//	go run ./cmd/bench-check -suggest artifacts/*.txt
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

// remeasureTimeout bounds the confirmation run. Only the benchmarks that
// violated are re-run, so this is seconds of work, not the ~6 minutes the full
// suite takes.
const remeasureTimeout = 5 * time.Minute

// modulePath is this repository's module. Benchmarks can only be re-run from
// within it; see remeasure.
const modulePath = "gitlab.flexinfer.ai/libs/fi-fhir"

// reporter writes the human-readable report. A failed write to the CI log is
// not something the gate can act on, so the errors are dropped in one place
// rather than at every call site.
type reporter struct{ w io.Writer }

func report(w io.Writer) reporter { return reporter{w} }

func (r reporter) printf(format string, args ...interface{}) {
	_, _ = fmt.Fprintf(r.w, format, args...)
}

func (r reporter) println(args ...interface{}) { _, _ = fmt.Fprintln(r.w, args...) }

func (r reporter) print(args ...interface{}) { _, _ = fmt.Fprint(r.w, args...) }

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Stdin))
}

func run(args []string, stdout, stderr io.Writer, stdin io.Reader) int {
	fs := flag.NewFlagSet("bench-check", flag.ContinueOnError)
	fs.SetOutput(stderr)
	baselinePath := fs.String("baseline", "", "compare against a baseline benchmark file")
	confirm := fs.Int("confirm", 0, "on violation, re-run only the offending benchmarks this many times and fail only if the violation reproduces")
	suggest := fs.Bool("suggest", false, "print calibrated CPUProfile entries derived from the input files instead of validating")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	inputs := fs.Args()

	if *suggest {
		return runSuggest(inputs, stdout, stderr)
	}

	var inputPath string
	if len(inputs) > 0 {
		inputPath = inputs[0]
	}

	suite, err := parseInput(inputPath, stdin)
	if err != nil {
		report(stderr).printf("error: %v\n", err)
		return 1
	}
	suite = suite.ReduceToBest()

	if len(suite.Results) == 0 {
		report(stderr).printf("error: no benchmark results found in input\n")
		return 1
	}

	report(stdout).printf("Parsed %d benchmark results\n", len(suite.Results))

	thresholds, profile, matched := workflow.ResolveWorkflowThresholds(suite.CPU)
	reportProfile(stdout, suite.CPU, profile, matched)

	exitCode := 0

	violations := thresholds.Check(suite)
	if len(violations) > 0 {
		report(stdout).println("THRESHOLD VIOLATIONS:")
		for _, v := range violations {
			report(stdout).printf("  FAIL: %s\n", v.Message)
		}
		report(stdout).println("")

		if *confirm > 0 {
			confirmed, err := confirmViolations(stdout, stderr, thresholds, suite, violations, *confirm, remeasure)
			switch {
			case err != nil:
				report(stderr).printf("warning: re-measurement failed: %v\n", err)
				report(stdout).println("Keeping the original verdict; a re-measurement that cannot run is not a pass.")
				exitCode = 1
			case len(confirmed) == 0:
				report(stdout).println("CONFIRMATION: no violation reproduced. Treating the first measurement as runner noise.")
			default:
				report(stdout).println("CONFIRMED VIOLATIONS (reproduced on re-measurement):")
				for _, v := range confirmed {
					report(stdout).printf("  FAIL: %s\n", v.Message)
				}
				exitCode = 1
			}
			report(stdout).println("")
		} else {
			exitCode = 1
		}
	} else {
		report(stdout).println("Threshold check: PASS (all benchmarks within bounds)")
		report(stdout).println("")
	}

	if *baselinePath != "" && compareBaseline(stdout, stderr, *baselinePath, suite) {
		exitCode = 1
	}

	return exitCode
}

func parseInput(path string, stdin io.Reader) (*workflow.BenchmarkSuite, error) {
	r := stdin
	if path != "" {
		f, err := os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("opening %s: %w", path, err)
		}
		defer func() { _ = f.Close() }()
		r = f
	}

	suite, err := workflow.ParseBenchmarkOutput(r, "current")
	if err != nil {
		return nil, fmt.Errorf("parsing benchmark output: %w", err)
	}
	return suite, nil
}

func reportProfile(w io.Writer, cpu string, profile workflow.CPUProfile, matched bool) {
	if cpu == "" {
		cpu = "(not reported)"
	}
	report(w).printf("CPU:     %s\n", cpu)
	report(w).printf("Profile: %s\n", profile.ID)
	if !matched {
		report(w).printf(`
WARNING: this CPU model has no calibrated profile, so the slowest known one
         (%s) is being used for latency and throughput. Allocation
         ceilings are unaffected because they do not depend on the machine.
         Calibrate it with: bench-check -suggest <recent benchmark.txt files>

`, profile.ID)
	}
	report(w).println("")
}

// benchRunner runs the named benchmarks in one package and returns raw
// "go test" output. It is a variable so tests can exercise the confirmation
// logic without shelling out.
type benchRunner func(pkg string, names []string, count int) ([]byte, error)

// confirmViolations re-runs only the benchmarks that failed and returns those
// that failed again. A benchmark can be several times slower than usual when a
// noisy neighbour lands on the same node mid-run, and that burst is transient:
// no threshold width can absorb it, but a second measurement clears it.
//
// The re-measurement must land on the same CPU model as the original run,
// otherwise it is comparing against a profile calibrated for other hardware —
// re-running a CI artifact's violation on a developer laptop would otherwise
// clear a genuine regression.
func confirmViolations(
	stdout, stderr io.Writer,
	thresholds *workflow.PerformanceThresholds,
	suite *workflow.BenchmarkSuite,
	violations []workflow.ThresholdViolation,
	count int,
	runner benchRunner,
) ([]workflow.ThresholdViolation, error) {
	byPackage := make(map[string]map[string]struct{})
	var names []string
	seen := make(map[string]struct{})

	for _, v := range violations {
		result := suite.GetResult(v.Benchmark)
		if result == nil || result.Package == "" {
			// A benchmark that produced no result, or output with no "pkg:"
			// header, cannot be targeted for a re-run.
			return violations, fmt.Errorf("cannot re-measure %q: no package recorded for it", v.Benchmark)
		}
		if _, ok := byPackage[result.Package]; !ok {
			byPackage[result.Package] = make(map[string]struct{})
		}
		byPackage[result.Package][v.Benchmark] = struct{}{}
		if _, ok := seen[v.Benchmark]; !ok {
			seen[v.Benchmark] = struct{}{}
			names = append(names, v.Benchmark)
		}
	}

	report(stdout).printf("Re-measuring %d benchmark(s) %d times to separate a regression from runner noise...\n",
		len(names), count)

	merged := &workflow.BenchmarkSuite{Name: "re-measure", CPU: suite.CPU}
	for _, pkg := range sortedKeys(byPackage) {
		out, err := runner(pkg, sortedSet(byPackage[pkg]), count)
		if len(out) > 0 {
			report(stdout).print(string(out))
		}
		if err != nil {
			return nil, fmt.Errorf("re-running benchmarks in %s: %w", pkg, err)
		}
		reparsed, err := workflow.ParseBenchmarkOutput(strings.NewReader(string(out)), "re-measure")
		if err != nil {
			return nil, fmt.Errorf("parsing re-measured output for %s: %w", pkg, err)
		}
		if reparsed.CPU != suite.CPU {
			return nil, fmt.Errorf(
				"re-measurement ran on %q but the original run used %q; a confirmation from different hardware cannot clear a violation",
				displayCPU(reparsed.CPU), displayCPU(suite.CPU))
		}
		merged.Results = append(merged.Results, reparsed.Results...)
	}

	best := merged.ReduceToBest()
	for _, name := range names {
		if best.GetResult(name) == nil {
			return nil, fmt.Errorf("re-measurement produced no result for %q", name)
		}
	}

	return thresholds.Subset(names).Check(best), nil
}

func displayCPU(cpu string) string {
	if cpu == "" {
		return "(not reported)"
	}
	return cpu
}

// benchmarkNamePattern matches the benchmark identifiers Go emits, including
// sub-benchmark paths such as BenchmarkEngine/routes=10.
var benchmarkNamePattern = regexp.MustCompile(`^Benchmark[A-Za-z0-9_/=.\-]*$`)

// remeasure re-runs the named benchmarks in one package.
//
// Both arguments originate in a benchmark.txt that CI treats as data, so they
// are validated before reaching the command line: the package must belong to
// this module, and names must look like benchmark identifiers. Without that, a
// crafted artifact could steer "go test" at an arbitrary path or smuggle a
// leading dash in as a flag.
func remeasure(pkg string, names []string, count int) ([]byte, error) {
	if pkg != modulePath && !strings.HasPrefix(pkg, modulePath+"/") {
		return nil, fmt.Errorf("refusing to re-run benchmarks outside %s: %q", modulePath, pkg)
	}
	quoted := make([]string, 0, len(names))
	for _, n := range names {
		if !benchmarkNamePattern.MatchString(n) {
			return nil, fmt.Errorf("refusing to re-run benchmark with unexpected name: %q", n)
		}
		quoted = append(quoted, regexp.QuoteMeta(n))
	}
	pattern := fmt.Sprintf("^(%s)$", strings.Join(quoted, "|"))

	// #nosec G204 -- pkg is constrained to this module and every name is
	// matched against benchmarkNamePattern above; both are passed as discrete
	// argv entries, never through a shell.
	cmd := exec.Command("go", "test",
		"-run=^$",
		"-bench="+pattern,
		"-benchmem",
		fmt.Sprintf("-count=%d", count),
		pkg,
	)
	// Inherit the environment unchanged: the re-measurement is only meaningful
	// if it runs under the same conditions as the measurement it is checking.

	done := make(chan struct{})
	var out []byte
	var err error
	go func() {
		out, err = cmd.CombinedOutput()
		close(done)
	}()

	select {
	case <-done:
		return out, err
	case <-time.After(remeasureTimeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-done
		return out, fmt.Errorf("re-measurement timed out after %s", remeasureTimeout)
	}
}

func compareBaseline(stdout, stderr io.Writer, path string, suite *workflow.BenchmarkSuite) bool {
	f, err := os.Open(path)
	if err != nil {
		report(stderr).printf("warning: could not open baseline %s: %v (skipping comparison)\n", path, err)
		return false
	}
	defer func() { _ = f.Close() }()

	baseline, err := workflow.ParseBenchmarkOutput(f, "baseline")
	if err != nil {
		report(stderr).printf("warning: could not parse baseline: %v (skipping comparison)\n", err)
		return false
	}
	if len(baseline.Results) == 0 {
		return false
	}

	baseline = baseline.ReduceToBest()
	if baseline.CPU != suite.CPU {
		report(stderr).printf("warning: baseline ran on %q but this run used %q; ns/op deltas below are dominated by the hardware difference\n",
			baseline.CPU, suite.CPU)
	}

	comparison := workflow.Compare(baseline, suite)
	report(stdout).println(comparison.Summary())

	if comparison.HasRegressions() {
		report(stdout).println("REGRESSION DETECTED: one or more benchmarks regressed >5%")
		return true
	}
	return false
}

func sortedKeys(m map[string]map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedSet(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
