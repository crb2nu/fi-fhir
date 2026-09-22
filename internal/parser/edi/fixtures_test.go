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
