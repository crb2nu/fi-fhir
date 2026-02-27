// bench-check validates Go benchmark output against performance thresholds.
//
// Usage:
//
//	go test -bench=. -benchmem -run=^$ ./internal/workflow/... | tee benchmark.txt
//	go run ./cmd/bench-check < benchmark.txt
//	go run ./cmd/bench-check benchmark.txt
//	go run ./cmd/bench-check -baseline=benchmark-baseline.txt benchmark.txt
package main

import (
	"fmt"
	"os"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

func main() {
	os.Exit(run())
}

func run() int {
	var baselinePath string
	var inputPath string

	// Simple arg parsing: -baseline=file and positional input file
	args := os.Args[1:]
	for _, arg := range args {
		if len(arg) > 10 && arg[:10] == "-baseline=" {
			baselinePath = arg[10:]
		} else if len(arg) > 0 && arg[0] != '-' {
			inputPath = arg
		}
	}

	// Open input (file arg or stdin)
	var input *os.File
	if inputPath != "" {
		f, err := os.Open(inputPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: opening %s: %v\n", inputPath, err)
			return 1
		}
		defer func() { _ = f.Close() }()
		input = f
	} else {
		input = os.Stdin
	}

	// Parse current benchmark results
	suite, err := workflow.ParseBenchmarkOutput(input, "current")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: parsing benchmark output: %v\n", err)
		return 1
	}

	if len(suite.Results) == 0 {
		fmt.Fprintf(os.Stderr, "error: no benchmark results found in input\n")
		return 1
	}

	fmt.Printf("Parsed %d benchmark results\n\n", len(suite.Results))

	exitCode := 0

	// Check absolute thresholds
	thresholds := workflow.DefaultWorkflowThresholds()
	violations := thresholds.Validate(suite)
	if len(violations) > 0 {
		fmt.Println("THRESHOLD VIOLATIONS:")
		for _, v := range violations {
			fmt.Printf("  FAIL: %s\n", v)
		}
		fmt.Println()
		exitCode = 1
	} else {
		fmt.Println("Threshold check: PASS (all benchmarks within bounds)")
		fmt.Println()
	}

	// Compare against baseline if provided
	if baselinePath != "" {
		f, err := os.Open(baselinePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not open baseline %s: %v (skipping comparison)\n", baselinePath, err)
		} else {
			defer func() { _ = f.Close() }()
			baseline, err := workflow.ParseBenchmarkOutput(f, "baseline")
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: could not parse baseline: %v (skipping comparison)\n", err)
			} else if len(baseline.Results) > 0 {
				comparison := workflow.Compare(baseline, suite)
				fmt.Println(comparison.Summary())

				if comparison.HasRegressions() {
					fmt.Println("REGRESSION DETECTED: one or more benchmarks regressed >5%")
					exitCode = 1
				}
			}
		}
	}

	return exitCode
}
