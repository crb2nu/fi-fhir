package workflow

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ParseBenchmarkOutput parses standard Go benchmark output (from go test -bench)
// into a BenchmarkSuite. It expects lines in the format:
//
//	BenchmarkName-8    1000000    1234 ns/op    256 B/op    5 allocs/op
func ParseBenchmarkOutput(r io.Reader, suiteName string) (*BenchmarkSuite, error) {
	suite := NewBenchmarkSuite(suiteName)
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "Benchmark") {
			continue
		}

		result, err := parseBenchLine(line)
		if err != nil {
			continue // Skip unparseable lines (headers, sub-test labels, etc.)
		}

		suite.AddResult(result)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading benchmark output: %w", err)
	}

	return suite, nil
}

// parseBenchLine parses a single Go benchmark output line.
// Format: BenchmarkName-8    1000000    1234 ns/op    256 B/op    5 allocs/op
func parseBenchLine(line string) (BenchmarkResult, error) {
	fields := strings.Fields(line)
	if len(fields) < 4 {
		return BenchmarkResult{}, fmt.Errorf("too few fields: %q", line)
	}

	// First field is the benchmark name (strip -N CPU suffix)
	name := fields[0]
	if idx := strings.LastIndex(name, "-"); idx > 0 {
		// Verify the suffix after "-" is a number (CPU count)
		if _, err := strconv.Atoi(name[idx+1:]); err == nil {
			name = name[:idx]
		}
	}

	// Second field is iteration count
	iterations, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return BenchmarkResult{}, fmt.Errorf("parsing iterations: %w", err)
	}

	result := BenchmarkResult{
		Name:          name,
		Iterations:    iterations,
		parsedMetrics: make(map[string]struct{}),
	}

	// Parse remaining key-value pairs: "1234 ns/op", "256 B/op", "5 allocs/op"
	for i := 2; i < len(fields)-1; i += 2 {
		val, err := strconv.ParseFloat(fields[i], 64)
		if err != nil {
			continue
		}

		unit := fields[i+1]
		switch unit {
		case "ns/op":
			result.NsPerOp = val
		case "B/op":
			result.BytesPerOp = int64(val)
		case "allocs/op":
			result.AllocsPerOp = int64(val)
		case "events/sec":
			result.EventsPerSec = val
		default:
			continue
		}
		result.parsedMetrics[unit] = struct{}{}
	}

	if len(result.parsedMetrics) == 0 {
		return BenchmarkResult{}, fmt.Errorf("no recognized metrics: %q", line)
	}

	return result, nil
}
