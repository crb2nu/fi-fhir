package edi

import (
	"os"
	"strings"
	"testing"
)

func TestExtractDelimiters(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name        string
		content     string
		wantElement byte
		wantSubelem byte
		wantSegment byte
		wantErr     bool
	}{
		{
			name:        "standard delimiters",
			content:     "ISA*00*          *00*          *ZZ*SENDER         *ZZ*RECEIVER       *240115*0800*^*00501*000000001*0*P*:~GS*",
			wantElement: '*',
			wantSubelem: ':',
			wantSegment: '~',
			wantErr:     false,
		},
		{
			name:        "pipe element separator",
			content:     "ISA|00|          |00|          |ZZ|SENDER         |ZZ|RECEIVER       |240115|0800|^|00501|000000001|0|P|:~GS|",
			wantElement: '|',
			wantSubelem: ':',
			wantSegment: '~',
			wantErr:     false,
		},
		{
			name:    "too short",
			content: "ISA*00*",
			wantErr: true,
		},
		{
			name:    "no ISA prefix",
			content: "GS*HC*SENDER*RECEIVER~",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			delims, err := p.extractDelimiters(tt.content)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if delims.Element != tt.wantElement {
				t.Errorf("element separator = %c, want %c", delims.Element, tt.wantElement)
			}
			if delims.Subelement != tt.wantSubelem {
				t.Errorf("subelement separator = %c, want %c", delims.Subelement, tt.wantSubelem)
			}
			if delims.Segment != tt.wantSegment {
				t.Errorf("segment terminator = %c, want %c", delims.Segment, tt.wantSegment)
			}
		})
	}
}

func TestParseSegment(t *testing.T) {
	p := NewParser()
	delims := DefaultDelimiters()

	tests := []struct {
		name        string
		raw         string
		wantID      string
		wantElemCnt int
		wantElem1   string
	}{
		{
			name:        "CLM segment",
			raw:         "CLM*CLAIM001*150.00***11:B:1*Y*A*Y*Y",
			wantID:      "CLM",
			wantElemCnt: 9,
			wantElem1:   "CLAIM001",
		},
		{
			name:        "NM1 segment",
			raw:         "NM1*85*2*SMITH MEDICAL*****XX*1234567890",
			wantID:      "NM1",
			wantElemCnt: 9,
			wantElem1:   "85",
		},
		{
			name:        "simple segment",
			raw:         "SE*23*0001",
			wantID:      "SE",
			wantElemCnt: 2,
			wantElem1:   "23",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			seg := p.parseSegment(tt.raw, delims)
			if seg.ID != tt.wantID {
				t.Errorf("ID = %s, want %s", seg.ID, tt.wantID)
			}
			if len(seg.Elements) != tt.wantElemCnt {
				t.Errorf("element count = %d, want %d", len(seg.Elements), tt.wantElemCnt)
			}
			if seg.GetElement(1) != tt.wantElem1 {
				t.Errorf("element 1 = %s, want %s", seg.GetElement(1), tt.wantElem1)
			}
		})
	}
}

func TestSegmentGetComponent(t *testing.T) {
	p := NewParser()
	delims := DefaultDelimiters()

	// CLM segment with composite element: 11:B:1
	seg := p.parseSegment("CLM*CLAIM001*150.00***11:B:1*Y*A*Y*Y", delims)

	tests := []struct {
		elemIdx int
		compIdx int
		want    string
	}{
		{5, 1, "11"},       // Place of service
		{5, 2, "B"},        // Frequency type
		{5, 3, "1"},        // Type
		{5, 4, ""},         // Out of bounds
		{1, 1, "CLAIM001"}, // Non-composite
	}

	for _, tt := range tests {
		got := seg.GetComponent(tt.elemIdx, tt.compIdx, delims.Subelement)
		if got != tt.want {
			t.Errorf("GetComponent(%d, %d) = %s, want %s", tt.elemIdx, tt.compIdx, got, tt.want)
		}
	}
}

func TestParse837PMinimal(t *testing.T) {
	content, err := os.ReadFile("../../../testdata/edi/837p_minimal.edi")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	p := NewParser()
	result, err := p.Parse(string(content))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	// Verify interchange
	ix := result.Interchange
	if ix == nil {
		t.Fatal("interchange is nil")
	}

	if ix.ControlNumber != "000000001" {
		t.Errorf("interchange control number = %s, want 000000001", ix.ControlNumber)
	}
	if ix.SenderID != "SENDER" {
		t.Errorf("sender ID = %s, want SENDER", ix.SenderID)
	}
	if ix.ReceiverID != "RECEIVER" {
		t.Errorf("receiver ID = %s, want RECEIVER", ix.ReceiverID)
	}
	if ix.UsageIndicator != "P" {
		t.Errorf("usage indicator = %s, want P", ix.UsageIndicator)
	}
	if ix.ControlVersion != "00501" {
		t.Errorf("control version = %s, want 00501", ix.ControlVersion)
	}

	// Verify functional group
	if len(ix.FunctionalGroups) != 1 {
		t.Fatalf("functional groups count = %d, want 1", len(ix.FunctionalGroups))
	}

	gs := ix.FunctionalGroups[0]
	if gs.FunctionalID != "HC" {
		t.Errorf("functional ID = %s, want HC", gs.FunctionalID)
	}
	if gs.VersionCode != "005010X222A1" {
		t.Errorf("version code = %s, want 005010X222A1", gs.VersionCode)
	}

	// Verify transaction
	if len(gs.Transactions) != 1 {
		t.Fatalf("transactions count = %d, want 1", len(gs.Transactions))
	}

	tx := gs.Transactions[0]
	if tx.SetIdentifier != "837" {
		t.Errorf("set identifier = %s, want 837", tx.SetIdentifier)
	}
	if tx.ControlNumber != "0001" {
		t.Errorf("transaction control number = %s, want 0001", tx.ControlNumber)
	}

	// Verify transaction type detection
	txType := GetTransactionType(tx)
	if txType != Transaction837P {
		t.Errorf("transaction type = %s, want 837P", txType)
	}

	// Verify key segments exist
	clm := tx.GetSegment("CLM")
	if clm == nil {
		t.Fatal("CLM segment not found")
	}
	if clm.GetElement(1) != "CLAIM001" {
		t.Errorf("claim ID = %s, want CLAIM001", clm.GetElement(1))
	}
	if clm.GetElement(2) != "150.00" {
		t.Errorf("claim amount = %s, want 150.00", clm.GetElement(2))
	}

	// Verify HL segments
	hlSegs := tx.GetSegments("HL")
	if len(hlSegs) != 2 {
		t.Errorf("HL segment count = %d, want 2", len(hlSegs))
	}
}

func TestParseInlineEDI(t *testing.T) {
	// Simple inline test with minimal envelope
	edi := strings.Join([]string{
		"ISA*00*          *00*          *ZZ*SEND           *ZZ*RECV           *240115*0800*^*00501*000000001*0*T*:~",
		"GS*HC*SEND*RECV*20240115*0800*1*X*005010X222A1~",
		"ST*837*0001*005010X222A1~",
		"BHT*0019*00*BATCH001*20240115*0800*CH~",
		"SE*2*0001~",
		"GE*1*1~",
		"IEA*1*000000001~",
	}, "")

	p := NewParser()
	result, err := p.Parse(edi)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(result.Interchange.FunctionalGroups) != 1 {
		t.Errorf("functional groups = %d, want 1", len(result.Interchange.FunctionalGroups))
	}

	tx := result.Interchange.FunctionalGroups[0].Transactions[0]
	if tx.SetIdentifier != "837" {
		t.Errorf("set identifier = %s, want 837", tx.SetIdentifier)
	}

	// Should have only BHT segment (ST and SE not included in Segments)
	if len(tx.Segments) != 1 {
		t.Errorf("segment count = %d, want 1", len(tx.Segments))
	}

	bht := tx.GetSegment("BHT")
	if bht == nil {
		t.Fatal("BHT segment not found")
	}
}

func TestParseErrors(t *testing.T) {
	p := NewParser()

	tests := []struct {
		name    string
		content string
		errCode string
	}{
		{
			name:    "empty content",
			content: "",
			errCode: "ISA_TOO_SHORT",
		},
		{
			name:    "no ISA",
			content: "GS*HC*SENDER*RECEIVER~",
			errCode: "ISA_TOO_SHORT",
		},
		{
			name:    "missing SE (GE in transaction)",
			content: "ISA*00*          *00*          *ZZ*SEND           *ZZ*RECV           *240115*0800*^*00501*000000001*0*T*:~GS*HC*SEND*RECV*20240115*0800*1*X*005010X222A1~ST*837*0001~BHT*0019*00~GE*1*1~IEA*1*000000001~",
			errCode: "ENVELOPE_IN_TX",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := p.Parse(tt.content)
			if err == nil {
				t.Error("expected error but got none")
				return
			}
			parseErr, ok := err.(*ParseError)
			if !ok {
				t.Errorf("expected ParseError, got %T", err)
				return
			}
			if parseErr.Code != tt.errCode {
				t.Errorf("error code = %s, want %s", parseErr.Code, tt.errCode)
			}
		})
	}
}

func TestTransactionSegments(t *testing.T) {
	p := NewParser()
	delims := DefaultDelimiters()

	segments := []*Segment{
		p.parseSegment("ST*837*0001", delims),
		p.parseSegment("BHT*0019*00*BATCH", delims),
		p.parseSegment("NM1*41*2*BILLING", delims),
		p.parseSegment("NM1*85*2*PROVIDER", delims),
		p.parseSegment("SE*4*0001", delims),
	}

	tx, endIdx, err := p.parseTransaction(segments, delims)
	if err != nil {
		t.Fatalf("parseTransaction failed: %v", err)
	}

	if endIdx != 4 {
		t.Errorf("endIdx = %d, want 4", endIdx)
	}

	// BHT + 2 NM1 = 3 segments
	if len(tx.Segments) != 3 {
		t.Errorf("segment count = %d, want 3", len(tx.Segments))
	}

	// GetSegments should find both NM1
	nm1s := tx.GetSegments("NM1")
	if len(nm1s) != 2 {
		t.Errorf("NM1 count = %d, want 2", len(nm1s))
	}
}
