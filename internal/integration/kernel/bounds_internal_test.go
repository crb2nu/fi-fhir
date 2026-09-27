package kernel

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBoundDiagnostics(t *testing.T) {
	many := make([]Diagnostic, MaxDiagnostics+7)
	for i := range many {
		many[i] = Diagnostic{Severity: "warning", Code: "W", Message: "m"}
	}
	bounded := boundDiagnostics(many)
	if len(bounded) != MaxDiagnostics+1 {
		t.Fatalf("len = %d, want %d", len(bounded), MaxDiagnostics+1)
	}
	last := bounded[len(bounded)-1]
	if last.Code != codeDiagnosticsTruncated || !strings.HasPrefix(last.Message, "7 more") {
		t.Fatalf("truncation notice = %+v", last)
	}
	if few := boundDiagnostics(many[:3]); len(few) != 3 {
		t.Fatalf("a short list was changed: %d", len(few))
	}
}

func TestPreviewBoundsSegments(t *testing.T) {
	var builder strings.Builder
	builder.WriteString("MSH|^~\\&|LAB|FAC|APP|FAC|20260115160532||ORU^R01^ORU_R01|CTRL1|P|2.5.1\r")
	builder.WriteString("PID|1||MRN1^^^HOSP^MR||DOE^JANE||19850315|F\r")
	builder.WriteString("OBR|1|ORD1|FIL1|CBC^Complete Blood Count^L\r")
	builder.WriteString("OBX|1|NM|WBC^White Blood Cell Count^L||7.2|10*3/uL|4.5-11.0|N|||F\r")
	for i := 0; i < MaxSegments; i++ {
		builder.WriteString("NTE|1||synthetic note\r")
	}
	input, err := json.Marshal(PreviewRequest{Message: builder.String(), Format: FormatHL7v2})
	if err != nil {
		t.Fatal(err)
	}
	response := Preview(input)
	if !response.OK {
		t.Fatalf("problems = %+v", response.Problems)
	}
	if len(response.Segments) != MaxSegments {
		t.Fatalf("segments = %d, want %d", len(response.Segments), MaxSegments)
	}
	last := response.Diagnostics[len(response.Diagnostics)-1]
	if last.Code != codeSegmentsTruncated || !strings.HasPrefix(last.Message, "4 more") {
		t.Fatalf("truncation notice = %+v", last)
	}
}

func TestWithCorrelationIDLeavesTheParsedEventAlone(t *testing.T) {
	if _, err := withCorrelationID(struct{}{}); err == nil {
		t.Fatal("a non-pointer was accepted")
	}
	if _, err := withCorrelationID(&struct{ Name string }{}); err == nil {
		t.Fatal("a struct without EventMeta was accepted")
	}
}
