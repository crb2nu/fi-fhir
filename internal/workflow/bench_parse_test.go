package workflow

import (
	"strings"
	"testing"
)

func TestParseBenchmarkOutput(t *testing.T) {
	input := `goos: linux
goarch: amd64
pkg: gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow
cpu: AMD EPYC 7B13
BenchmarkEngineProcess-8             	  500000	      2345 ns/op	    1024 B/op	      15 allocs/op
BenchmarkCELEvaluate_Simple-8        	 5000000	       234 ns/op	      64 B/op	       2 allocs/op
BenchmarkFilterMatch_EventType-8     	 2000000	       567 ns/op	     128 B/op	       4 allocs/op
BenchmarkTransform_SetField-8        	10000000	       123 ns/op	      32 B/op	       1 allocs/op
PASS
ok  	gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow	12.345s
`

	suite, err := ParseBenchmarkOutput(strings.NewReader(input), "test-run")
	if err != nil {
		t.Fatalf("ParseBenchmarkOutput failed: %v", err)
	}

	if suite.Name != "test-run" {
		t.Errorf("suite.Name = %q, want %q", suite.Name, "test-run")
	}

	if len(suite.Results) != 4 {
		t.Fatalf("got %d results, want 4", len(suite.Results))
	}

	// Verify first result
	r := suite.GetResult("BenchmarkEngineProcess")
	if r == nil {
		t.Fatal("BenchmarkEngineProcess not found")
	}
	if r.Iterations != 500000 {
		t.Errorf("Iterations = %d, want 500000", r.Iterations)
	}
	if r.NsPerOp != 2345 {
		t.Errorf("NsPerOp = %f, want 2345", r.NsPerOp)
	}
	if r.BytesPerOp != 1024 {
		t.Errorf("BytesPerOp = %d, want 1024", r.BytesPerOp)
	}
	if r.AllocsPerOp != 15 {
		t.Errorf("AllocsPerOp = %d, want 15", r.AllocsPerOp)
	}
}

func TestParseBenchmarkOutput_StripsCPUSuffix(t *testing.T) {
	input := `BenchmarkTest-16    1000    5000 ns/op
`
	suite, err := ParseBenchmarkOutput(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suite.Results) != 1 {
		t.Fatalf("got %d results, want 1", len(suite.Results))
	}
	if suite.Results[0].Name != "BenchmarkTest" {
		t.Errorf("Name = %q, want %q", suite.Results[0].Name, "BenchmarkTest")
	}
}

func TestParseBenchmarkOutput_SkipsMalformed(t *testing.T) {
	input := `PASS
ok  	some/package	1.0s
BenchmarkValid-8    1000    100 ns/op
not a benchmark line
`
	suite, err := ParseBenchmarkOutput(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suite.Results) != 1 {
		t.Errorf("got %d results, want 1 (should skip malformed lines)", len(suite.Results))
	}
}

func TestParseBenchmarkOutput_CapturesCPUAndPackage(t *testing.T) {
	// Two packages, one machine: the shape of a real test:benchmark artifact.
	input := `goos: linux
goarch: amd64
pkg: gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow
cpu: Intel Core Processor (Broadwell, IBRS)
BenchmarkEngineProcess-2    101588    12194 ns/op    888 B/op    24 allocs/op
PASS
ok  	gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow	83.6s
goos: linux
goarch: amd64
pkg: gitlab.flexinfer.ai/libs/fi-fhir/pkg/validate
cpu: Intel Core Processor (Broadwell, IBRS)
BenchmarkNPIValidation-2    8475496    138.3 ns/op    0 B/op    0 allocs/op
`
	suite, err := ParseBenchmarkOutput(strings.NewReader(input), "test")
	if err != nil {
		t.Fatalf("ParseBenchmarkOutput failed: %v", err)
	}

	if suite.CPU != "Intel Core Processor (Broadwell, IBRS)" {
		t.Errorf("CPU = %q, want the Broadwell model", suite.CPU)
	}
	if got := suite.GetResult("BenchmarkEngineProcess"); got == nil {
		t.Fatal("BenchmarkEngineProcess not found")
	} else if got.Package != "gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow" {
		t.Errorf("Package = %q, want the workflow package", got.Package)
	}
	if got := suite.GetResult("BenchmarkNPIValidation"); got == nil {
		t.Fatal("BenchmarkNPIValidation not found")
	} else if got.Package != "gitlab.flexinfer.ai/libs/fi-fhir/pkg/validate" {
		t.Errorf("Package = %q, want the validate package", got.Package)
	}
}

func TestParseBenchmarkOutput_CPUAbsentOrAmbiguous(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "no cpu header",
			input: "BenchmarkA-8    1000    100 ns/op\n",
		},
		{
			// A run spanning two machines cannot be judged against either
			// profile, so the CPU is reported as unknown and the caller
			// falls back to the most permissive bounds.
			name: "two distinct cpu headers",
			input: `cpu: AMD Ryzen 9 7900X3D 12-Core Processor
BenchmarkA-8    1000    100 ns/op
cpu: Intel Core Processor (Broadwell, IBRS)
BenchmarkB-8    1000    100 ns/op
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			suite, err := ParseBenchmarkOutput(strings.NewReader(tt.input), "test")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if suite.CPU != "" {
				t.Errorf("CPU = %q, want empty", suite.CPU)
			}
			if _, _, matched := ResolveWorkflowThresholds(suite.CPU); matched {
				t.Error("an unknown CPU must not report a profile match")
			}
		})
	}
}

func TestParseBenchmarkOutput_Empty(t *testing.T) {
	suite, err := ParseBenchmarkOutput(strings.NewReader(""), "empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(suite.Results) != 0 {
		t.Errorf("got %d results, want 0", len(suite.Results))
	}
}

func TestParseBenchLine(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    BenchmarkResult
		wantErr bool
	}{
		{
			name: "full line",
			line: "BenchmarkFoo-8    1000    500 ns/op    64 B/op    3 allocs/op",
			want: BenchmarkResult{
				Name: "BenchmarkFoo", Iterations: 1000,
				NsPerOp: 500, BytesPerOp: 64, AllocsPerOp: 3,
			},
		},
		{
			name: "ns/op only",
			line: "BenchmarkBar-4    5000    1234 ns/op",
			want: BenchmarkResult{
				Name: "BenchmarkBar", Iterations: 5000, NsPerOp: 1234,
			},
		},
		{
			name: "sub-benchmark with slash",
			line: "BenchmarkEngine/route_count=10-8    2000    3456 ns/op    512 B/op    8 allocs/op",
			want: BenchmarkResult{
				Name: "BenchmarkEngine/route_count=10", Iterations: 2000,
				NsPerOp: 3456, BytesPerOp: 512, AllocsPerOp: 8,
			},
		},
		{
			name: "custom throughput metric",
			line: "BenchmarkThroughput-8    2000    3456 ns/op    125000 events/sec    512 B/op    8 allocs/op",
			want: BenchmarkResult{
				Name: "BenchmarkThroughput", Iterations: 2000,
				NsPerOp: 3456, EventsPerSec: 125000, BytesPerOp: 512, AllocsPerOp: 8,
			},
		},
		{
			name:    "too few fields",
			line:    "BenchmarkShort    1000",
			wantErr: true,
		},
		{
			name:    "no recognized metrics",
			line:    "BenchmarkRequired-8    1000    0 bogus",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseBenchLine(tt.line)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Name != tt.want.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.want.Name)
			}
			if got.Iterations != tt.want.Iterations {
				t.Errorf("Iterations = %d, want %d", got.Iterations, tt.want.Iterations)
			}
			if got.NsPerOp != tt.want.NsPerOp {
				t.Errorf("NsPerOp = %f, want %f", got.NsPerOp, tt.want.NsPerOp)
			}
			if got.BytesPerOp != tt.want.BytesPerOp {
				t.Errorf("BytesPerOp = %d, want %d", got.BytesPerOp, tt.want.BytesPerOp)
			}
			if got.AllocsPerOp != tt.want.AllocsPerOp {
				t.Errorf("AllocsPerOp = %d, want %d", got.AllocsPerOp, tt.want.AllocsPerOp)
			}
			if got.EventsPerSec != tt.want.EventsPerSec {
				t.Errorf("EventsPerSec = %f, want %f", got.EventsPerSec, tt.want.EventsPerSec)
			}
		})
	}
}
