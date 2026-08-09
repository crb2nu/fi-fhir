package main

import (
	"io"
	"math"
	"os"
	"sort"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

// runSuggest derives calibrated CPUProfile entries from a set of benchmark.txt
// artifacts, grouped by the CPU model each one ran on.
//
// This exists so that adding a node type to the CI pool is a mechanical step
// rather than tribal knowledge: collect recent artifacts from the new hardware,
// run this, and paste the emitted block into workflowCPUProfiles. Medians need
// samples, so it refuses to guess from a handful of runs.
func runSuggest(paths []string, stdout, stderr io.Writer) int {
	if len(paths) == 0 {
		report(stderr).println("error: -suggest needs one or more benchmark output files")
		return 1
	}

	type sample struct {
		ns         map[string][]float64
		throughput map[string][]float64
		runs       int
	}
	byCPU := make(map[string]*sample)

	for _, path := range paths {
		f, err := os.Open(path)
		if err != nil {
			report(stderr).printf("warning: skipping %s: %v\n", path, err)
			continue
		}
		suite, err := workflow.ParseBenchmarkOutput(f, path)
		_ = f.Close()
		if err != nil {
			report(stderr).printf("warning: skipping %s: %v\n", path, err)
			continue
		}
		if suite.CPU == "" {
			report(stderr).printf("warning: skipping %s: no single cpu: header\n", path)
			continue
		}

		s := byCPU[suite.CPU]
		if s == nil {
			s = &sample{ns: map[string][]float64{}, throughput: map[string][]float64{}}
			byCPU[suite.CPU] = s
		}
		s.runs++
		for _, r := range suite.ReduceToBest().Results {
			if r.NsPerOp > 0 {
				s.ns[r.Name] = append(s.ns[r.Name], r.NsPerOp)
			}
			if r.EventsPerSec > 0 {
				s.throughput[r.Name] = append(s.throughput[r.Name], r.EventsPerSec)
			}
		}
	}

	if len(byCPU) == 0 {
		report(stderr).println("error: no usable benchmark output found")
		return 1
	}

	gatedNs, gatedThroughput := gatedBenchmarks()

	report(stdout).printf("// Calibrated from %d file(s) at margin %.2gx. Paste into workflowCPUProfiles.\n\n",
		len(paths), workflow.LatencyMarginFactor)

	for _, cpu := range sortedCPUs(byCPU) {
		s := byCPU[cpu]
		report(stdout).printf("// %s: %d run(s)\n", cpu, s.runs)
		if s.runs < 5 {
			report(stdout).printf("// WARNING: %d runs is too few for a stable median; collect more before trusting this.\n", s.runs)
		}
		report(stdout).println("{")
		report(stdout).printf("\tID:        %q,\n", "TODO-profile-id")
		report(stdout).printf("\tCPUModels: []string{%q},\n", cpu)

		report(stdout).println("\tMedianNsPerOp: map[string]float64{")
		for _, name := range gatedNs {
			if m, ok := median(s.ns[name]); ok {
				report(stdout).printf("\t\t%q: %.0f,\n", name, m)
			}
		}
		report(stdout).println("\t},")

		report(stdout).println("\tMaxNsPerOp: map[string]float64{")
		for _, name := range gatedNs {
			m, ok := median(s.ns[name])
			if !ok {
				report(stdout).printf("\t\t// %s: no samples\n", name)
				continue
			}
			report(stdout).printf("\t\t%q: %d,\n", name, roundUpNs(m*workflow.LatencyMarginFactor))
		}
		report(stdout).println("\t},")

		report(stdout).println("\tMinThroughput: map[string]float64{")
		for _, name := range gatedThroughput {
			m, ok := median(s.throughput[name])
			if !ok {
				report(stdout).printf("\t\t// %s: no samples\n", name)
				continue
			}
			report(stdout).printf("\t\t%q: %d,\n", name, roundDownThroughput(m/workflow.LatencyMarginFactor))
		}
		report(stdout).println("\t},")
		report(stdout).println("},")
		report(stdout).println("")
	}

	return 0
}

// gatedBenchmarks returns the benchmark names a profile must cover, taken from
// the existing calibration so the two cannot drift apart.
func gatedBenchmarks() (ns []string, throughput []string) {
	thresholds := workflow.DefaultWorkflowThresholds()
	for name := range thresholds.MaxNsPerOp {
		ns = append(ns, name)
	}
	for name := range thresholds.MinThroughput {
		throughput = append(throughput, name)
	}
	sort.Strings(ns)
	sort.Strings(throughput)
	return ns, throughput
}

func median(values []float64) (float64, bool) {
	if len(values) == 0 {
		return 0, false
	}
	sorted := make([]float64, len(values))
	copy(sorted, values)
	sort.Float64s(sorted)

	mid := len(sorted) / 2
	if len(sorted)%2 == 1 {
		return sorted[mid], true
	}
	return (sorted[mid-1] + sorted[mid]) / 2, true
}

// roundUpNs rounds a latency ceiling outward to a readable step so the table
// reads as a deliberate bound rather than a raw measurement.
func roundUpNs(v float64) int64 {
	step := 100.0
	if v < 1000 {
		step = 50.0
	}
	return int64(math.Ceil(v/step) * step)
}

func roundDownThroughput(v float64) int64 {
	return int64(math.Floor(v/1000) * 1000)
}

func sortedCPUs[T any](m map[string]T) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
