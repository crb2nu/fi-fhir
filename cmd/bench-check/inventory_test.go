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

// legacyGatedBenchmarks is the set of benchmarks the performance gate asserts
// on today. Every one lives in internal/workflow and exercises the legacy
// workflow engine, which no durable path calls at runtime: the durable accept
// path reaches internal/workflow only for parse and plan
// (internal/integration/processor/workflow_plan.go, session/workflow_simulation.go),
// and neither entry point is benchmarked.
var legacyGatedBenchmarks = []string{
	"BenchmarkCELEvaluate_Simple",
	"BenchmarkEngineProcess",
	"BenchmarkFilterMatch_EventType",
	"BenchmarkThroughput_Complex",
	"BenchmarkThroughput_Simple",
	"BenchmarkTransform_SetField",
}

// TestPerformanceHarness_NothingMeasuresAnyProductBudgetToday is slice 4.4b's
// day-1 gate. It is an inventory assertion, not a benchmark, and it is expected
// to PASS on unmodified main.
//
// It exists to convert a claim into a measurement. `test:benchmark` is green,
// blocking, and benches 32 functions — which reads as partial credit toward the
// slice 4.4 performance budgets. It is not: every gated benchmark is a legacy
// workflow-engine micro-benchmark, and the durable ingress/MLLP/batch accept
// path that the product budgets are written about has never been measured at
// all. Passing here states that zero as a fact rather than as an impression.
//
// The gate inverts the moment the first durable benchmark lands. When
// internal/integration/perf exists, assertion 1 fails and this test must be
// rewritten to assert the new floor — that is the intended lifecycle, and the
// rewrite is the proof that the floor moved.
//
// On the scan mechanism: the spec sketched parsing `go test -list 'Benchmark.*'
// ./...`. This walks the source with go/ast instead, because internal/integration's
// tests are overwhelmingly behind `//go:build integration` and a plain
// `go test -list` cannot see a build-tagged benchmark. An AST walk ignores build
// tags, so it is the stricter check for "nothing exists" — and it does not
// compile the tree, so it runs in milliseconds inside an ordinary unit-test job.
func TestPerformanceHarness_NothingMeasuresAnyProductBudgetToday(t *testing.T) {
	root := repoRoot(t)

	t.Run("zero benchmarks exist under internal/integration", func(t *testing.T) {
		found := benchmarksUnder(t, filepath.Join(root, "internal", "integration"))
		if len(found) != 0 {
			t.Fatalf("expected zero benchmarks under internal/integration, found %d: %v\n"+
				"If slice 4.4b's task 3 has landed, this gate has served its purpose and must be\n"+
				"rewritten to assert the new floor rather than deleted.", len(found), found)
		}
	})

	t.Run("the test:benchmark package list contains no internal/integration package", func(t *testing.T) {
		for _, src := range []struct {
			file string
			path string
		}{
			{"gitlab-ci", filepath.Join(root, ".gitlab-ci.yml")},
			{"makefile", filepath.Join(root, "Makefile")},
		} {
			pkgs := benchPackageLists(t, src.path)
			if len(pkgs) == 0 {
				t.Fatalf("%s: found no `go test -bench` package list; the gate can no longer "+
					"see what test:benchmark measures", src.file)
			}
			for _, list := range pkgs {
				for _, pkg := range list {
					if strings.Contains(pkg, "internal/integration") {
						t.Errorf("%s: benchmark package list already includes %q; the durable "+
							"path is measured and this gate is stale", src.file, pkg)
					}
				}
			}
		}
	})

	t.Run("bench-check gates exactly the six legacy benchmarks and no durable one", func(t *testing.T) {
		gated := gatedBenchmarkNames()

		if diff := diffStrings(legacyGatedBenchmarks, gated); diff != "" {
			t.Fatalf("bench-check's threshold maps no longer name exactly the six legacy "+
				"benchmarks:\n%s", diff)
		}

		// Naming the six is not enough — prove they are legacy by locating each
		// one's declaration. A durable benchmark added under one of these names
		// would satisfy the set comparison above and defeat the gate.
		legacy := benchmarksUnder(t, filepath.Join(root, "internal", "workflow"))
		have := make(map[string]struct{}, len(legacy))
		for _, name := range legacy {
			have[name] = struct{}{}
		}
		for _, name := range gated {
			if _, ok := have[name]; !ok {
				t.Errorf("gated benchmark %q is not declared in internal/workflow; the gate "+
					"may now cover a non-legacy path", name)
			}
		}
	})
}

// gatedBenchmarkNames returns every benchmark named by any threshold map in any
// calibrated CPU profile, plus the CPU-independent allocation ceilings. It
// unions across all profiles so a name present in only one class is still seen.
func gatedBenchmarkNames() []string {
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
	// The unrecognized-hardware path, which falls back to the slowest profile.
	th, _, _ := workflow.ResolveWorkflowThresholds("no such cpu")
	collect(th)

	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// benchmarksUnder returns every `func BenchmarkXxx(*testing.B)` declared in a
// _test.go file under dir, regardless of build tag.
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
// is what `go test` requires before it will treat the name as a benchmark.
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

var benchInvocation = regexp.MustCompile(`go test\s+-bench=\.[^\n]*`)

// benchPackageLists extracts the `./...` package patterns from every
// `go test -bench=.` invocation in path.
func benchPackageLists(t *testing.T, path string) [][]string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	var lists [][]string
	for _, line := range benchInvocation.FindAllString(string(data), -1) {
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
	return lists
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

// diffStrings renders the symmetric difference of two sorted string slices, or
// "" when they are equal.
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
