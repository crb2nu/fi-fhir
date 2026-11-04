// Generated corpus regeneration (edilint v0.5.0; run from repository root):
//
// go install github.com/crb2nu/edilint/cmd/edilint@v0.5.0
// $(go env GOPATH)/bin/edilint gen 837p --claims 25 --control 101 --date 2026-11-01 > testdata/edi/837p_gen_25.edi
// $(go env GOPATH)/bin/edilint gen 835 --claims 10 --control 102 --date 2026-11-01 > testdata/edi/835_gen_10.edi
// $(go env GOPATH)/bin/edilint gen 837p --claims 3 --control 201 --date 2026-11-01 --defect EL3006 > testdata/edi/defects/837p_se01.edi
// $(go env GOPATH)/bin/edilint gen 837p --claims 3 --control 202 --date 2026-11-01 --defect EL3007 > testdata/edi/defects/837p_ge01.edi
// $(go env GOPATH)/bin/edilint gen 837p --claims 3 --control 203 --date 2026-11-01 --defect EL3008 > testdata/edi/defects/837p_iea01.edi
// $(go env GOPATH)/bin/edilint gen 837p --claims 3 --control 204 --date 2026-11-01 --defect EL3005 > testdata/edi/defects/837p_control_mismatch.edi

package edi

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestFixtureTrailerCountsMatchTheirTransactionSets pins the SE01 repair
// df45f683 already landed on main. edilint found three fixtures declaring a
// segment count they did not contain: 271_rejected.edi declared 13 where it had
// 12, 271_response.edi declared 23 where it had 22, and 277_denied.edi declared
// 18 where it had 19. Those values are correct in the corpus today; this test
// is what keeps them correct.
//
// Nothing else catches this. Parse recomputes SegmentCount from the segments it
// actually read (parser.go) and never compares it with the declared SE01, so a
// wrong trailer round-trips through every assertion in mapper_test.go
// unnoticed — which is exactly how these three sat wrong for months. The other
// guards are the lint:edi and lint:edi-fixtures CI jobs, and both need network
// to install edilint; this one runs in `go test ./internal/parser/edi/...`.
//
// This deliberately asserts over the fixture bytes rather than over parser
// behaviour. Rejecting a bad SE01 inside Parse is a behaviour change with its
// own blast radius on real trading-partner traffic, and is tracked separately.
func TestFixtureTrailerCountsMatchTheirTransactionSets(t *testing.T) {
	paths, err := filepath.Glob("../../../testdata/edi/*.edi")
	if err != nil {
		t.Fatalf("glob fixtures: %v", err)
	}
	// An existence guard: a moved or renamed corpus would make the glob match
	// nothing and this test greener rather than redder.
	if len(paths) < 7 {
		t.Fatalf("found %d fixtures under testdata/edi, want at least 7", len(paths))
	}

	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			delims, err := NewParser().extractDelimiters(string(content))
			if err != nil {
				t.Fatalf("extract delimiters: %v", err)
			}
			elem := string(delims.Element)

			var (
				inSet   bool
				counted int
				sets    int
			)
			for _, raw := range strings.Split(string(content), string(delims.Segment)) {
				segment := strings.TrimSpace(raw)
				if segment == "" {
					continue
				}
				fields := strings.Split(segment, elem)

				switch fields[0] {
				case "ST":
					inSet = true
					counted = 0
				case "SE":
					if !inSet {
						t.Fatalf("SE segment %q without a preceding ST", segment)
					}
				}
				if inSet {
					counted++ // ST through SE inclusive
				}
				if fields[0] != "SE" {
					continue
				}

				if len(fields) < 2 {
					t.Fatalf("SE segment %q has no SE01 element", segment)
				}
				declared, err := strconv.Atoi(fields[1])
				if err != nil {
					t.Fatalf("SE01 %q is not a number: %v", fields[1], err)
				}
				if declared != counted {
					t.Errorf("SE01 declares %d segment(s) from ST through SE inclusive, but the transaction set contains %d", declared, counted)
				}
				inSet = false
				sets++
			}

			if inSet {
				t.Error("transaction set opened by ST is never closed by SE")
			}
			if sets == 0 {
				t.Error("fixture contains no ST/SE transaction set")
			}
		})
	}
}

func TestCleanFixturesParse(t *testing.T) {
	cases := []struct {
		file, claimSegment string
		claims             int
	}{
		{"270_inquiry.edi", "CLM", 0},
		{"271_rejected.edi", "CLM", 0},
		{"271_response.edi", "CLM", 0},
		{"276_request.edi", "CLM", 0},
		{"277_denied.edi", "CLM", 0},
		{"277_response.edi", "CLM", 0},
		{"837p_minimal.edi", "CLM", 1},
		{"837p_gen_25.edi", "CLM", 25},
		{"835_gen_10.edi", "CLP", 10},
	}
	paths, err := filepath.Glob("../../../testdata/edi/*.edi")
	if err != nil || len(paths) != len(cases) {
		t.Fatalf("clean corpus: %d files, want %d: %v", len(paths), len(cases), err)
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("../../../testdata/edi", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			result, err := NewParser().Parse(string(raw))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if len(result.Warnings) != 0 {
				t.Errorf("unexpected warnings: %+v", result.Warnings)
			}
			if result.Interchange == nil {
				t.Fatal("missing interchange")
			}
			groups := result.Interchange.FunctionalGroups
			if len(groups) != 1 {
				t.Fatalf("groups = %d, want 1", len(groups))
			}
			if len(groups[0].Transactions) != 1 {
				t.Fatalf("transactions = %d, want 1", len(groups[0].Transactions))
			}
			if got := len(groups[0].Transactions[0].GetSegments(tc.claimSegment)); got != tc.claims {
				t.Errorf("%s count = %d, want %d", tc.claimSegment, got, tc.claims)
			}
		})
	}
}

// This records current tolerance as a finding, not a fix. Both 837P and 835
// use the same envelope parser; format-specific defect copies add no coverage.
func TestDefectFixturesDiagnosticClass(t *testing.T) {
	cases := []struct{ file, want string }{
		{"837p_se01.edi", "silent acceptance"},
		{"837p_ge01.edi", "silent acceptance"},
		{"837p_iea01.edi", "silent acceptance"},
		{"837p_control_mismatch.edi", "silent acceptance"},
	}
	paths, err := filepath.Glob("../../../testdata/edi/defects/*.edi")
	if err != nil || len(paths) != len(cases) {
		t.Fatalf("defect corpus: %d files, want %d: %v", len(paths), len(cases), err)
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("../../../testdata/edi/defects", tc.file))
			if err != nil {
				t.Fatal(err)
			}
			result, err := NewParser().Parse(string(raw))
			got := "silent acceptance"
			if err != nil {
				got = "error: " + err.Error()
			} else {
				if len(result.Warnings) > 0 {
					codes := make([]string, 0, len(result.Warnings))
					for _, warning := range result.Warnings {
						codes = append(codes, warning.Code)
					}
					got = "warning: " + strings.Join(codes, ",")
				}
			}
			if got != tc.want {
				t.Errorf("diagnostic class = %q, want %q", got, tc.want)
			}
		})
	}
}
