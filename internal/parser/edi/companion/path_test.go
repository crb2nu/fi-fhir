package companion

import (
	"testing"

	"github.com/crb2nu/fi-fhir/internal/parser/edi"
)

func TestParsePath(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantLoop   string
		wantSeg    string
		wantSegIdx int
		wantElem   int
		wantComp   int
		wantErr    bool
	}{
		{
			name:       "simple segment.element",
			path:       "CLM.01",
			wantSeg:    "CLM",
			wantSegIdx: -1, // first occurrence
			wantElem:   1,
			wantComp:   0,
		},
		{
			name:       "segment with two-digit element",
			path:       "NM1.09",
			wantSeg:    "NM1",
			wantSegIdx: -1,
			wantElem:   9,
			wantComp:   0,
		},
		{
			name:       "element with component",
			path:       "CLM.05-1",
			wantSeg:    "CLM",
			wantSegIdx: -1,
			wantElem:   5,
			wantComp:   1,
		},
		{
			name:       "loop.segment.element",
			path:       "2010AA.NM1.09",
			wantLoop:   "2010AA",
			wantSeg:    "NM1",
			wantSegIdx: -1,
			wantElem:   9,
			wantComp:   0,
		},
		{
			name:       "loop.segment.element with component",
			path:       "2300.CLM.05-1",
			wantLoop:   "2300",
			wantSeg:    "CLM",
			wantSegIdx: -1,
			wantElem:   5,
			wantComp:   1,
		},
		{
			name:       "segment with specific index",
			path:       "REF[2].01",
			wantSeg:    "REF",
			wantSegIdx: 1, // 0-based
			wantElem:   1,
			wantComp:   0,
		},
		{
			name:       "segment with all index",
			path:       "NM1[*].01",
			wantSeg:    "NM1",
			wantSegIdx: -2, // all occurrences
			wantElem:   1,
			wantComp:   0,
		},
		{
			name:       "loop with indexed segment",
			path:       "2400.SV1[*].01",
			wantLoop:   "2400",
			wantSeg:    "SV1",
			wantSegIdx: -2,
			wantElem:   1,
			wantComp:   0,
		},
		{
			name:    "empty path",
			path:    "",
			wantErr: true,
		},
		{
			name:    "single part",
			path:    "CLM",
			wantErr: true,
		},
		{
			name:    "invalid element number",
			path:    "CLM.abc",
			wantErr: true,
		},
		{
			name:    "invalid component number",
			path:    "CLM.05-abc",
			wantErr: true,
		},
		{
			name:    "invalid segment index",
			path:    "REF[abc].01",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc, err := ParsePath(tt.path)

			if tt.wantErr {
				if err == nil {
					t.Errorf("ParsePath(%q) expected error, got nil", tt.path)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParsePath(%q) unexpected error: %v", tt.path, err)
			}

			if pc.Loop != tt.wantLoop {
				t.Errorf("Loop = %q, want %q", pc.Loop, tt.wantLoop)
			}
			if pc.Segment != tt.wantSeg {
				t.Errorf("Segment = %q, want %q", pc.Segment, tt.wantSeg)
			}
			if pc.SegmentIndex != tt.wantSegIdx {
				t.Errorf("SegmentIndex = %d, want %d", pc.SegmentIndex, tt.wantSegIdx)
			}
			if pc.Element != tt.wantElem {
				t.Errorf("Element = %d, want %d", pc.Element, tt.wantElem)
			}
			if pc.Component != tt.wantComp {
				t.Errorf("Component = %d, want %d", pc.Component, tt.wantComp)
			}
		})
	}
}

func TestPathResolver_Resolve(t *testing.T) {
	// Create a mock transaction with test segments
	tx := &edi.Transaction{
		SetIdentifier: "837",
		Segments: []*edi.Segment{
			{ID: "BHT", Elements: []string{"0019", "00", "12345", "20240115", "1200"}},
			{ID: "NM1", Elements: []string{"41", "2", "ACME BILLING", "", "", "", "", "46", "123456789"}},
			{ID: "PER", Elements: []string{"IC", "JANE DOE", "TE", "5551234567"}},
			{ID: "NM1", Elements: []string{"40", "2", "MEGA PAYER", "", "", "", "", "46", "987654321"}},
			{ID: "CLM", Elements: []string{"CLAIM001", "1500.00", "", "", "11:B:1", "Y", "A", "Y", "Y", "P"}},
			{ID: "HI", Elements: []string{"ABK:J0600", "ABF:Z87.89"}},
			{ID: "REF", Elements: []string{"D9", "123456"}},
			{ID: "REF", Elements: []string{"EA", "789012"}},
		},
	}

	delimiters := edi.DefaultDelimiters()
	resolver := NewPathResolver(tx, delimiters)

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "first element",
			path: "BHT.01",
			want: "0019",
		},
		{
			name: "fourth element",
			path: "BHT.04",
			want: "20240115",
		},
		{
			name: "NM1 entity identifier",
			path: "NM1.01",
			want: "41", // first NM1
		},
		{
			name: "second NM1 entity identifier",
			path: "NM1[2].01",
			want: "40", // second NM1
		},
		{
			name: "CLM ID",
			path: "CLM.01",
			want: "CLAIM001",
		},
		{
			name: "CLM composite with component",
			path: "CLM.05-1",
			want: "11",
		},
		{
			name: "CLM composite second component",
			path: "CLM.05-2",
			want: "B",
		},
		{
			name: "first REF value",
			path: "REF.02",
			want: "123456",
		},
		{
			name: "second REF value",
			path: "REF[2].02",
			want: "789012",
		},
		{
			name: "non-existent segment",
			path: "ZZZ.01",
			want: "",
		},
		{
			name: "element out of range",
			path: "BHT.99",
			want: "",
		},
		{
			name: "component out of range",
			path: "CLM.05-99",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolver.Resolve(tt.path)
			if got != tt.want {
				t.Errorf("Resolve(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestPathResolver_ResolveAll(t *testing.T) {
	tx := &edi.Transaction{
		SetIdentifier: "837",
		Segments: []*edi.Segment{
			{ID: "REF", Elements: []string{"D9", "AAA"}},
			{ID: "REF", Elements: []string{"EA", "BBB"}},
			{ID: "REF", Elements: []string{"F8", "CCC"}},
		},
	}

	delimiters := edi.DefaultDelimiters()
	resolver := NewPathResolver(tx, delimiters)

	t.Run("all REF qualifiers", func(t *testing.T) {
		values := resolver.ResolveAll("REF[*].01")
		if len(values) != 3 {
			t.Fatalf("ResolveAll returned %d values, want 3", len(values))
		}
		if values[0] != "D9" || values[1] != "EA" || values[2] != "F8" {
			t.Errorf("Values = %v, want [D9 EA F8]", values)
		}
	})

	t.Run("all REF values", func(t *testing.T) {
		values := resolver.ResolveAll("REF[*].02")
		if len(values) != 3 {
			t.Fatalf("ResolveAll returned %d values, want 3", len(values))
		}
		if values[0] != "AAA" || values[1] != "BBB" || values[2] != "CCC" {
			t.Errorf("Values = %v, want [AAA BBB CCC]", values)
		}
	})
}

func TestPathResolver_Exists(t *testing.T) {
	tx := &edi.Transaction{
		SetIdentifier: "837",
		Segments: []*edi.Segment{
			{ID: "CLM", Elements: []string{"CLAIM001", "1500.00", "", "", "11:B:1"}},
		},
	}

	delimiters := edi.DefaultDelimiters()
	resolver := NewPathResolver(tx, delimiters)

	tests := []struct {
		path string
		want bool
	}{
		{"CLM.01", true},
		{"CLM.02", true},
		{"CLM.03", false}, // empty element
		{"CLM.99", false}, // out of range
		{"ZZZ.01", false}, // non-existent segment
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := resolver.Exists(tt.path)
			if got != tt.want {
				t.Errorf("Exists(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestPathResolver_GetSegments(t *testing.T) {
	tx := &edi.Transaction{
		SetIdentifier: "837",
		Segments: []*edi.Segment{
			{ID: "NM1", Elements: []string{"41", "2", "BILLING"}},
			{ID: "NM1", Elements: []string{"40", "2", "PAYER"}},
			{ID: "CLM", Elements: []string{"123"}},
		},
	}

	delimiters := edi.DefaultDelimiters()
	resolver := NewPathResolver(tx, delimiters)

	t.Run("all NM1 segments", func(t *testing.T) {
		segments := resolver.GetSegments("NM1[*].01")
		if len(segments) != 2 {
			t.Fatalf("GetSegments returned %d segments, want 2", len(segments))
		}
	})

	t.Run("first NM1 segment", func(t *testing.T) {
		segments := resolver.GetSegments("NM1.01")
		if len(segments) != 1 {
			t.Fatalf("GetSegments returned %d segments, want 1", len(segments))
		}
		if segments[0].GetElement(1) != "41" {
			t.Errorf("First NM1 element 1 = %q, want 41", segments[0].GetElement(1))
		}
	})

	t.Run("second NM1 segment", func(t *testing.T) {
		segments := resolver.GetSegments("NM1[2].01")
		if len(segments) != 1 {
			t.Fatalf("GetSegments returned %d segments, want 1", len(segments))
		}
		if segments[0].GetElement(1) != "40" {
			t.Errorf("Second NM1 element 1 = %q, want 40", segments[0].GetElement(1))
		}
	})
}

func TestPathResolver_WithLoopStructure(t *testing.T) {
	// Create a minimal 837 transaction
	tx := &edi.Transaction{
		SetIdentifier: "837",
		Segments: []*edi.Segment{
			{ID: "BHT", Elements: []string{"0019", "00", "12345"}},
			{ID: "NM1", Elements: []string{"41", "2", "SUBMITTER NAME", "", "", "", "", "46", "111111111"}},
			{ID: "NM1", Elements: []string{"40", "2", "RECEIVER NAME", "", "", "", "", "46", "222222222"}},
			{ID: "HL", Elements: []string{"1", "", "20", "1"}},
			{ID: "NM1", Elements: []string{"85", "2", "BILLING PROVIDER", "", "", "", "", "XX", "1234567890"}},
		},
	}

	// Create a minimal loop structure for testing
	loopStruct := &edi.Loop837Structure{
		BHT: tx.Segments[0],
		Submitter: &edi.Loop1000{
			NM1: tx.Segments[1],
		},
		Receiver: &edi.Loop1000{
			NM1: tx.Segments[2],
		},
		BillingProviders: []*edi.Loop2000A{
			{
				HL: tx.Segments[3],
				BillingName: &edi.Loop2010{
					NM1: tx.Segments[4],
				},
			},
		},
	}

	delimiters := edi.DefaultDelimiters()
	resolver := NewPathResolverWithLoops(tx, delimiters, loopStruct)

	tests := []struct {
		name string
		path string
		want string
	}{
		{
			name: "1000A submitter name",
			path: "1000A.NM1.03",
			want: "SUBMITTER NAME",
		},
		{
			name: "1000B receiver name",
			path: "1000B.NM1.03",
			want: "RECEIVER NAME",
		},
		{
			name: "2010AA billing provider name",
			path: "2010AA.NM1.03",
			want: "BILLING PROVIDER",
		},
		{
			name: "2010AA billing provider NPI",
			path: "2010AA.NM1.09",
			want: "1234567890",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolver.Resolve(tt.path)
			if got != tt.want {
				t.Errorf("Resolve(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestLooksLikeLoop(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"2010AA", true},
		{"2300", true},
		{"1000A", true},
		{"NM1", false},
		{"CLM", false},
		{"", false},
		{"A2300", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := looksLikeLoop(tt.input)
			if got != tt.want {
				t.Errorf("looksLikeLoop(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
