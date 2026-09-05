package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

// legacyGatedBenchmarks is the set of legacy workflow-engine benchmarks the
// per-CPU threshold set asserts on. Every one lives in internal/workflow and
// measures a code path no durable request executes at runtime: the durable path
// reaches internal/workflow only for parse and plan.
var legacyGatedBenchmarks = []string{
	"BenchmarkCELEvaluate_Simple",
	"BenchmarkEngineProcess",
	"BenchmarkFilterMatch_EventType",
	"BenchmarkThroughput_Complex",
	"BenchmarkThroughput_Simple",
	"BenchmarkTransform_SetField",
}

// durableBenchmarks is every benchmark in internal/integration/perf. Only the
// serial pair is gated; see durableAllocCeilings for the measured scheduler
// noise that keeps the parallel pair out of the threshold map.
var durableBenchmarks = []string{
	"BenchmarkDurableAccept_IngressSubmit",
	"BenchmarkDurableAccept_IngressSubmitParallel",
	"BenchmarkDurableAccept_MLLPSubmit",
	"BenchmarkDurableAccept_MLLPSubmitParallel",
}

// TestPerformanceHarness_TheDurablePathIsMeasuredAndGated is the day-1 gate,
// inverted.
//
// It began life as TestPerformanceHarness_NothingMeasuresAnyProductBudgetToday,
// which passed on main by asserting a zero: no benchmark anywhere under
// internal/integration, no durable package in test:benchmark's list, and a
// threshold map naming only legacy micro-benchmarks. That zero was the point.
// `test:benchmark` was green, blocking, and benched 32 functions, which read as
// partial credit toward the slice 4.4 performance budgets — while the durable
// accept path those budgets describe had never been measured at all.
//
// Slice 4.4b's task 3 moved the floor, so the gate is rewritten rather than
// deleted: the rewrite is the proof that the floor moved. What it now asserts is
// the shape of the new arrangement, and in particular the two things easiest to
// get wrong later — that the legacy and durable sets stay disjoint, and that a
// durable name never leaks into the shared map that the legacy job resolves.
func TestPerformanceHarness_TheDurablePathIsMeasuredAndGated(t *testing.T) {
	root := repoRoot(t)

	t.Run("the durable accept path has benchmarks", func(t *testing.T) {
		found := benchmarksUnder(t, filepath.Join(root, "internal", "integration"))
		if diff := diffStrings(durableBenchmarks, found); diff != "" {
			t.Fatalf("benchmarks under internal/integration are not the expected set:\n%s", diff)
		}
	})

	t.Run("the durable set gates the serial pair and nothing else", func(t *testing.T) {
		gated := sortedCeilingNames(workflow.DurableAllocCeilings())
		want := []string{
			"BenchmarkDurableAccept_IngressSubmit",
			"BenchmarkDurableAccept_MLLPSubmit",
		}
		if diff := diffStrings(want, gated); diff != "" {
			t.Fatalf("durable allocation ceilings are not the expected set:\n%s\n"+
				"The parallel variants are measured but not gated: their allocation count "+
				"depends on goroutine scheduling, not only on the accept path.", diff)
		}

		// A durable set that gates ns/op or events/sec would be a calibrated
		// wall-clock gate in the shared pool, which the lane's decision
		// prohibits outright.
		durable := workflow.ResolveDurableThresholds()
		if len(durable.MaxNsPerOp) != 0 {
			t.Errorf("durable set gates ns/op (%v); wall-clock belongs to the pinned-runner job", durable.MaxNsPerOp)
		}
		if len(durable.MinThroughput) != 0 {
			t.Errorf("durable set gates events/sec (%v); throughput belongs to the pinned-runner job", durable.MinThroughput)
		}
	})

	t.Run("the legacy set still gates exactly the six legacy benchmarks", func(t *testing.T) {
		gated := legacyGatedBenchmarkNames()
		if diff := diffStrings(legacyGatedBenchmarks, gated); diff != "" {
			t.Fatalf("the legacy threshold set changed:\n%s", diff)
		}

		legacy := benchmarksUnder(t, filepath.Join(root, "internal", "workflow"))
		have := make(map[string]struct{}, len(legacy))
		for _, name := range legacy {
			have[name] = struct{}{}
		}
		for _, name := range gated {
			if _, ok := have[name]; !ok {
				t.Errorf("gated benchmark %q is not declared in internal/workflow", name)
			}
		}
	})

	t.Run("no durable benchmark leaks into the legacy set", func(t *testing.T) {
		// This is the failure the sibling map exists to prevent, and it is
		// silent in the worst way: ResolveWorkflowThresholds copies the legacy
		// allocation ceilings into every CPU profile, and Check treats every
		// name in any map as a required result. A durable name in that map
		// makes test:benchmark fail with "required benchmark result missing",
		// which reads like a broken runner rather than a mistake in a map.
		legacy := legacyGatedBenchmarkNames()
		durable := make(map[string]struct{}, len(durableBenchmarks))
		for _, name := range durableBenchmarks {
			durable[name] = struct{}{}
		}
		for _, name := range legacy {
			if _, ok := durable[name]; ok {
				t.Errorf("%q is gated by the legacy set, but test:benchmark does not run "+
					"internal/integration; it would fail as a missing result", name)
			}
		}
	})

	t.Run("test:benchmark still runs only the legacy packages", func(t *testing.T) {
		// The durable benchmarks are behind //go:build integration and need a
		// PostgreSQL, so they run in their own job. If they ever appear in this
		// list, test:benchmark will fail on a database it does not have.
		for _, src := range []struct {
			file string
			path string
		}{
			{"gitlab-ci", filepath.Join(root, ".gitlab-ci.yml")},
			{"makefile", filepath.Join(root, "Makefile")},
		} {
			for _, list := range benchPackageLists(t, src.path, "bench-durable") {
				for _, pkg := range list {
					if strings.Contains(pkg, "internal/integration") {
						t.Errorf("%s: the legacy benchmark package list includes %q; "+
							"the durable benchmarks belong to their own job", src.file, pkg)
					}
				}
			}
		}
	})
}

// legacyGatedBenchmarkNames returns every benchmark named by any threshold map
// in any calibrated CPU profile, unioned with the CPU-independent allocation
// ceilings and the unrecognized-hardware fallback.
func legacyGatedBenchmarkNames() []string {
	set := make(map[string]struct{})
	collect := func(th *workflow.PerformanceThresholds) {
		for name := range th.MaxNsPerOp {
			set[name] = struct{}{}
		}
		for name := range th.MaxAllocsPerOp {
			set[name] = struct{}{}
		}
		for name := range th.MinThroughput {
			set[name] = struct{}{}
		}
	}
	for _, profile := range workflow.WorkflowCPUProfiles() {
		for _, model := range profile.CPUModels {
			th, _, _ := workflow.ResolveWorkflowThresholds(model)
			collect(th)
		}
	}
	th, _, _ := workflow.ResolveWorkflowThresholds("no such cpu")
	collect(th)

	return sortedKeysOf(set)
}

// benchmarksUnder returns every `func BenchmarkXxx(*testing.B)` declared in a
// _test.go file under dir, regardless of build tag.
//
// It walks the source rather than parsing `go test -list` output because
// internal/integration's tests are overwhelmingly behind `//go:build
// integration`, and a plain `go test -list` cannot see a build-tagged
// benchmark. The AST walk ignores build tags and compiles nothing.
func benchmarksUnder(t *testing.T, dir string) []string {
	t.Helper()

	var found []string
	fset := token.NewFileSet()
	err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); name == "testdata" || name == "node_modules" || strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if perr != nil {
			return perr
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name == nil {
				continue
			}
			if strings.HasPrefix(fn.Name.Name, "Benchmark") && isBenchmarkSignature(fn) {
				found = append(found, fn.Name.Name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	sort.Strings(found)
	return found
}

// isBenchmarkSignature reports whether fn takes exactly one *testing.B, which
// is what `go test` requires before it treats the name as a benchmark.
func isBenchmarkSignature(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}
	star, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	sel, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "testing" && sel.Sel.Name == "B"
}

var benchInvocation = regexp.MustCompile(`go test[^\n]*-bench=\.[^\n]*`)

// benchPackageLists extracts the `./...` package patterns from every
// `go test -bench=.` invocation in path, skipping lines that belong to a target
// named by skipMarkers (the durable job legitimately names internal/integration).
func benchPackageLists(t *testing.T, path string, skipMarkers ...string) [][]string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	var lists [][]string
	for _, line := range benchInvocation.FindAllString(string(data), -1) {
		if matchesAnyMarker(line, skipMarkers) {
			continue
		}
		var pkgs []string
		for _, field := range strings.Fields(line) {
			if strings.HasPrefix(field, "./") {
				pkgs = append(pkgs, field)
			}
		}
		if len(pkgs) > 0 {
			lists = append(lists, pkgs)
		}
	}
	if len(lists) == 0 {
		t.Fatalf("%s: found no legacy `go test -bench` package list; the gate can no longer "+
			"see what test:benchmark measures", path)
	}
	return lists
}

func matchesAnyMarker(line string, markers []string) bool {
	for _, marker := range markers {
		if strings.Contains(line, marker) {
			return true
		}
	}
	// The durable invocation is identifiable by its build tag even when the
	// surrounding target name is not on the same line.
	return strings.Contains(line, "-tags=integration")
}

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod found above %s", dir)
		}
		dir = parent
	}
}

func sortedCeilingNames(m map[string]int64) []string {
	out := make([]string, 0, len(m))
	for name := range m {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func sortedKeysOf(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for name := range m {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// diffStrings renders the symmetric difference of two string slices, or "" when
// they hold the same elements.
func diffStrings(want, got []string) string {
	wantSet := make(map[string]struct{}, len(want))
	for _, s := range want {
		wantSet[s] = struct{}{}
	}
	gotSet := make(map[string]struct{}, len(got))
	for _, s := range got {
		gotSet[s] = struct{}{}
	}

	var b strings.Builder
	for _, s := range want {
		if _, ok := gotSet[s]; !ok {
			b.WriteString("  missing: " + s + "\n")
		}
	}
	for _, s := range got {
		if _, ok := wantSet[s]; !ok {
			b.WriteString("  unexpected: " + s + "\n")
		}
	}
	return b.String()
}
