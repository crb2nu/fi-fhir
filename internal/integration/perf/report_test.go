package perf

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestPerformanceReportCertifiesOnlyWhatItMeasured pins the rule
// scripts/performance-report.sh applies before it may write "certified": true.
//
// Schema version 1 certified any run that had the FI_FHIR_PERF_RUNNER project
// variable set and evaluated no measurement at all, so a build that slept
// 300 ms in every accept certified budget 1. This runs the real script against
// synthetic benchmark output — no database, no pinned runner — so the rule is
// proven in the ordinary unit job, where a regression would otherwise go
// unnoticed until someone read an archived report closely.
func TestPerformanceReportCertifiesOnlyWhatItMeasured(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is not installed; the report script needs it")
	}
	if _, err := exec.LookPath("awk"); err != nil {
		t.Skip("awk is not installed; the report script needs it")
	}
	script := filepath.Join("..", "..", "..", "scripts", "performance-report.sh")

	pinned := []string{`CI_RUNNER_TAGS=["fi-fhir-perf"]`, "CI_RUNNER_ID=8"}

	tests := []struct {
		name          string
		bench         string
		env           []string
		wantExit      int
		wantCertified bool
		wantBudget1   string
		wantNC        bool
		wantFailed    *int
	}{
		{
			name:          "pinned runner, within budget: certified",
			bench:         benchOutput(false, 5.1, 7.2),
			env:           pinned,
			wantCertified: true,
			wantBudget1:   "certified",
		},
		{
			name:        "the version-1 proxy variable alone certifies nothing",
			bench:       benchOutput(false, 5.1, 7.2),
			env:         []string{"FI_FHIR_PERF_RUNNER=1"},
			wantBudget1: "harnessed",
		},
		{
			name:        "negative control over budget: failed on budget 1, exit 0",
			bench:       benchOutput(true, 305.3, 306.2),
			env:         append([]string{"FI_FHIR_PERF_BUILD_TAGS=perfregress"}, pinned...),
			wantBudget1: "failed",
			wantNC:      true,
			wantFailed:  intPtr(1),
		},
		{
			name:        "negative control the harness missed: exit 3",
			bench:       benchOutput(true, 5.1, 7.2),
			env:         pinned,
			wantExit:    3,
			wantBudget1: "harnessed",
			wantNC:      true,
		},
		{
			name:        "a regression fails budget 1 on any runner",
			bench:       benchOutput(false, 5.1, 612.0),
			env:         nil, // shared pool: failed still beats harnessed
			wantBudget1: "failed",
		},
		{
			name:        "mean-only output does not measure a percentile budget",
			bench:       stripMetric(benchOutput(false, 5.1, 7.2), "p95-ms"),
			env:         pinned,
			wantBudget1: "not_measured",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			in := filepath.Join(dir, "benchmark-profile.txt")
			out := filepath.Join(dir, "performance-report.json")
			if err := os.WriteFile(in, []byte(tt.bench), 0o600); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command(bash, script, in, out)
			// A clean environment: the CI job running this test has its own
			// CI_RUNNER_* values, and they must not leak into the case.
			cmd.Env = append([]string{"PATH=" + os.Getenv("PATH")}, tt.env...)
			output, err := cmd.CombinedOutput()

			exit := 0
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				exit = exitErr.ExitCode()
			} else if err != nil {
				t.Fatalf("running report script: %v", err)
			}
			if exit != tt.wantExit {
				t.Fatalf("exit = %d, want %d; output:\n%s", exit, tt.wantExit, output)
			}

			report := readReport(t, out)
			if report.SchemaVersion != 2 {
				t.Errorf("schema_version = %d, want 2", report.SchemaVersion)
			}
			if report.Certified != tt.wantCertified {
				t.Errorf("certified = %v, want %v (reason %q)", report.Certified, tt.wantCertified, report.Certification.Reason)
			}
			if !report.Certified && report.Certification.Reason == "" {
				t.Error("an uncertified report must say why")
			}
			if got := report.Budgets[0].Status; got != tt.wantBudget1 {
				t.Errorf("budget 1 status = %q, want %q (measured %q)", got, tt.wantBudget1, report.Budgets[0].Measured)
			}
			if report.NegativeControl.Ran != tt.wantNC {
				t.Errorf("negative_control.ran = %v, want %v", report.NegativeControl.Ran, tt.wantNC)
			}
			if fmt.Sprint(deref(report.NegativeControl.FailedBudget)) != fmt.Sprint(deref(tt.wantFailed)) {
				t.Errorf("negative_control.failed_budget = %v, want %v",
					deref(report.NegativeControl.FailedBudget), deref(tt.wantFailed))
			}
			if report.Budgets[2].Status == "certified" {
				t.Error("budget 3 has no workload in this job and must never read as certified")
			}
		})
	}
}

type perfReport struct {
	SchemaVersion int  `json:"schema_version"`
	Certified     bool `json:"certified"`
	Certification struct {
		RunnerTag string `json:"runner_tag"`
		RunnerID  string `json:"runner_id"`
		Reason    string `json:"reason"`
	} `json:"certification"`
	NegativeControl struct {
		Ran          bool `json:"ran"`
		FailedBudget *int `json:"failed_budget"`
	} `json:"negative_control"`
	Budgets []struct {
		ID       int    `json:"id"`
		Status   string `json:"status"`
		Measured string `json:"measured"`
	} `json:"budgets"`
}

func readReport(t *testing.T, path string) perfReport {
	t.Helper()

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading report: %v", err)
	}
	var report perfReport
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("report is not valid JSON: %v\n%s", err, raw)
	}
	if len(report.Budgets) != 3 {
		t.Fatalf("budgets = %d entries, want 3", len(report.Budgets))
	}
	return report
}

// benchOutput renders `go test -bench` output for the four durable-accept
// benchmarks in the shape Go prints it, every one reporting the given p95/p99.
func benchOutput(negativeControl bool, p95, p99 float64) string {
	var b strings.Builder
	b.WriteString("goos: linux\ngoarch: amd64\npkg: gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/perf\n")
	b.WriteString("cpu: Intel(R) Core(TM) i7-5930K CPU @ 3.50GHz\n")
	for _, name := range []string{"IngressSubmit", "IngressSubmitParallel", "MLLPSubmit", "MLLPSubmitParallel"} {
		fmt.Fprintf(&b, "BenchmarkDurableAccept_%s-4\t     300\t   4123456 ns/op\t     %.3f p50-ms\t     %.3f p95-ms\t     %.3f p99-ms\t  412345 B/op\t    4650 allocs/op\n",
			name, p95*0.8, p95, p99)
		if negativeControl {
			fmt.Fprintf(&b, "--- BENCH: BenchmarkDurableAccept_%s-4\n    accept_bench_test.go:1: NEGATIVE-CONTROL build: 300ms injected into every measured accept\n", name)
		}
	}
	b.WriteString("PASS\n")
	return b.String()
}

// stripMetric removes every value/unit pair with the given unit from the
// benchmark lines, leaving the rest intact — the shape of output from a
// benchmark that never reported the percentile.
func stripMetric(text, unit string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "Benchmark") {
			continue
		}
		fields := strings.Split(line, "\t")
		kept := fields[:0]
		for _, f := range fields {
			if !strings.HasSuffix(f, " "+unit) {
				kept = append(kept, f)
			}
		}
		lines[i] = strings.Join(kept, "\t")
	}
	return strings.Join(lines, "\n")
}

func intPtr(n int) *int { return &n }

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
