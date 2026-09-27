package session

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// captureRedactionTable is this test's own copy of the published table in
// docs/operations/PHI-RETENTION.md ("Captured and peeked samples"). It is
// written out rather than read from CaptureRedactedFields so that shrinking the
// production table fails here instead of quietly shrinking the proof with it.
var captureRedactionTable = []struct {
	segment string
	fields  int // the segment's field count in HL7 v2.5.1
	masked  []int
}{
	{segment: "PID", fields: 39, masked: []int{2, 3, 4, 5, 6, 7, 9, 11, 12, 13, 14, 18, 19, 20, 21, 23, 29}},
	{segment: "NK1", fields: 39, masked: []int{2, 4, 5, 6, 8, 9, 12, 13, 16, 26, 30, 31, 32, 33, 37, 38}},
	{segment: "IN1", fields: 53, masked: []int{6, 8, 9, 10, 11, 12, 13, 14, 16, 18, 19, 24, 26, 28, 29, 30, 36, 44, 49, 51, 52}},
	{segment: "IN2", fields: 72, masked: []int{1, 2, 3, 6, 7, 8, 9, 10, 13, 17, 22, 26, 40, 44, 45, 49, 50, 52, 53, 55, 56, 61, 63, 64, 69, 70}},
	{segment: "GT1", fields: 57, masked: []int{2, 3, 4, 5, 6, 7, 8, 12, 13, 14, 16, 17, 18, 19, 21, 24, 29, 31, 32, 42, 45, 46, 51, 56}},
	{segment: "MRG", fields: 7, masked: []int{1, 2, 3, 4, 5, 6, 7}},
	{segment: "PV1", fields: 52, masked: []int{19, 50}},
}

// syntheticField is a unique, obviously synthetic value for one field. It has a
// repetition and components so a test can tell "the whole field was masked"
// from "the first component was masked".
func syntheticField(segment string, field int) string {
	stem := fmt.Sprintf("SYN-%s%02d", segment, field)
	return stem + "-A^" + stem + "-B~" + stem + "-C"
}

// syntheticSegment populates every field of one segment.
func syntheticSegment(segment string, fields int, separator string) string {
	values := make([]string, 0, fields+1)
	values = append(values, segment)
	for field := 1; field <= fields; field++ {
		values = append(values, syntheticField(segment, field))
	}
	return strings.Join(values, separator)
}

// syntheticMSH is an MSH segment declaring separator as its field separator.
func syntheticMSH(separator string) string {
	return "MSH" + separator + `^~\&` + separator + "SENDER" + separator + "FAC" + separator + "FI-FHIR" + separator + "FAC" +
		separator + "20260926120000" + separator + separator + "ADT^A01^ADT_A01" + separator + "control-1" +
		separator + "P" + separator + "2.5.1"
}

// syntheticCaptureMessage is one ADT message whose PID, NK1, IN1, IN2, GT1,
// MRG, and PV1 carry a synthetic value in every field.
func syntheticCaptureMessage(separator string) string {
	segments := []string{
		syntheticMSH(separator),
		"EVN" + separator + "A01" + separator + "20260926120000",
	}
	for _, row := range captureRedactionTable {
		segments = append(segments, syntheticSegment(row.segment, row.fields, separator))
	}
	return strings.Join(segments, "\r") + "\r"
}

// segmentFields splits the redacted output's line for one segment.
func segmentFields(t *testing.T, redacted, segment, separator string) []string {
	t.Helper()
	for _, line := range strings.Split(redacted, "\n") {
		if strings.HasPrefix(line, segment+separator) {
			return strings.Split(line, separator)
		}
	}
	t.Fatalf("segment %s is missing from the redacted output:\n%s", segment, redacted)
	return nil
}

func TestCaptureRedactedFieldsMatchesThePublishedTable(t *testing.T) {
	published := make(map[string][]int, len(captureRedactionTable))
	for _, row := range captureRedactionTable {
		published[row.segment] = append([]int(nil), row.masked...)
	}
	production := make(map[string][]int, len(CaptureRedactedFields))
	for segment, fields := range CaptureRedactedFields {
		sorted := append([]int(nil), fields...)
		sort.Ints(sorted)
		production[segment] = sorted
	}
	if !reflect.DeepEqual(production, published) {
		t.Fatalf("CaptureRedactedFields = %v\nthe published table (docs/operations/PHI-RETENTION.md) = %v", production, published)
	}
}

// TestRedactCapturedHL7v2_MasksEveryTableField is the capture redactor's pin:
// with a synthetic value in every field of the table's segments, every table field
// is replaced whole and no fragment of its value survives anywhere in the
// output, while every field the table does not list survives byte for byte.
func TestRedactCapturedHL7v2_MasksEveryTableField(t *testing.T) {
	message := syntheticCaptureMessage("|")
	redacted := RedactCapturedHL7v2(message)
	for _, row := range captureRedactionTable {
		t.Run(row.segment, func(t *testing.T) {
			fields := segmentFields(t, redacted, row.segment, "|")
			if len(fields) != row.fields+1 {
				t.Fatalf("%s has %d fields after redaction, want %d", row.segment, len(fields)-1, row.fields)
			}
			masked := make(map[int]bool, len(row.masked))
			for _, field := range row.masked {
				masked[field] = true
			}
			for field := 1; field <= row.fields; field++ {
				got := fields[field]
				stem := fmt.Sprintf("SYN-%s%02d", row.segment, field)
				switch {
				case masked[field]:
					if got != redactedValue {
						t.Errorf("%s-%d = %q, want %q", row.segment, field, got, redactedValue)
					}
					if strings.Contains(redacted, stem) {
						t.Errorf("%s-%d: a fragment of the masked value (%s) survived", row.segment, field, stem)
					}
				case got != syntheticField(row.segment, field):
					t.Errorf("%s-%d = %q; a field outside the table must survive unchanged", row.segment, field, got)
				}
			}
		})
	}
	if strings.Contains(redacted, "\r") {
		t.Fatal("the redacted output kept a carriage return; segments are LF-separated like redactHL7v2's")
	}
	for _, kept := range []string{syntheticMSH("|"), "EVN|A01|20260926120000"} {
		if !strings.Contains(redacted, kept) {
			t.Fatalf("a segment outside the table changed: %q is missing", kept)
		}
	}
}

// TestRedactCapturedHL7v2_IsASupersetOfTheLegacyRedactor: every field the
// pasted-sample redactor masks, the capture redactor masks too — including on
// a literal "PID|" line of a message whose MSH-1 declares another separator,
// which redactHL7v2 masks because it never reads MSH-1.
func TestRedactCapturedHL7v2_IsASupersetOfTheLegacyRedactor(t *testing.T) {
	for name, message := range map[string]string{
		"every table segment populated": syntheticCaptureMessage("|"),
		"a PID| line under an MSH-1 of '#'": syntheticMSH("#") + "\r" +
			syntheticSegment("PID", 39, "|") + "\r" + syntheticSegment("NK1", 39, "#") + "\r",
	} {
		t.Run(name, func(t *testing.T) {
			legacy := strings.Split(redactHL7v2(message), "\n")
			captured := RedactCapturedHL7v2(message)
			capture := strings.Split(captured, "\n")
			if len(legacy) != len(capture) {
				t.Fatalf("line counts differ: legacy %d, capture %d", len(legacy), len(capture))
			}
			checked := 0
			for line := range legacy {
				legacyFields := strings.Split(legacy[line], "|")
				captureFields := strings.Split(capture[line], "|")
				for field, value := range legacyFields {
					if value != redactedValue {
						continue
					}
					checked++
					if field >= len(captureFields) || captureFields[field] != redactedValue {
						t.Errorf("%s-%d is masked by redactHL7v2 but not by RedactCapturedHL7v2", legacyFields[0], field)
					}
				}
			}
			if checked != 6 {
				t.Fatalf("the legacy redactor masked %d fields of the fixture, want its six PID fields", checked)
			}
			for _, field := range CaptureRedactedFields["PID"] {
				if stem := fmt.Sprintf("SYN-PID%02d", field); strings.Contains(captured, stem) {
					t.Errorf("PID-%d survived the capture redactor (%s)", field, stem)
				}
			}
		})
	}
}

func TestRedactCapturedHL7v2_ReadsTheFieldSeparatorFromMSH(t *testing.T) {
	message := syntheticCaptureMessage("#")
	redacted := RedactCapturedHL7v2(message)
	if !strings.Contains(redacted, syntheticMSH("#")) {
		t.Fatal("MSH changed under a non-default field separator")
	}
	fields := segmentFields(t, redacted, "PID", "#")
	if fields[5] != redactedValue || fields[3] != redactedValue || fields[8] != syntheticField("PID", 8) {
		t.Fatalf("a message declaring '#' as its field separator was not masked by field: %q", fields[:9])
	}
	for _, row := range captureRedactionTable {
		for _, field := range row.masked {
			if stem := fmt.Sprintf("SYN-%s%02d", row.segment, field); strings.Contains(redacted, stem) {
				t.Fatalf("%s survived under a non-default field separator", stem)
			}
		}
	}
}

func TestRedactCapturedHL7v2_KeepsEmptyFieldsEmpty(t *testing.T) {
	message := "MSH|^~\\&|S|F|R|F|20260926||ADT^A01|c|P|2.5.1\rPID|1||||Synthetic^Name\rNK1|1\r"
	redacted := RedactCapturedHL7v2(message)
	want := "MSH|^~\\&|S|F|R|F|20260926||ADT^A01|c|P|2.5.1\nPID|1||||REDACTED\nNK1|1\n"
	if redacted != want {
		t.Fatalf("RedactCapturedHL7v2 = %q, want %q", redacted, want)
	}
}

// TestRedactHL7v2_BehaviourPin pins the pasted-sample redactor exactly. The
// capture redactor is new; this one is unchanged by it, and this test is what
// says so.
func TestRedactHL7v2_BehaviourPin(t *testing.T) {
	message := "MSH|^~\\&|SENDER|FAC|FI-FHIR|FAC|20260926120000||ADT^A01^ADT_A01|control-1|P|2.5.1\r\n" +
		"PID|1|ALT-1|MRN-1^^^HOSP^MR|ALT-2|Synthetic^Pat|Maiden^M|19800101|F|Alias^A|2106-3|1 Main St^^Town^ST^00000|CNTY|555-0100|555-0101|EN|S|REL|ACCT-1|000-00-0000|DL-1\r" +
		"NK1|1|Synthetic^Kin|SPO|2 Side St^^Town|555-0102\n" +
		"IN1|1|PLAN|COMPANY|Payer|||||||||||||Synthetic^Insured\r" +
		"PV1|1|I"
	want := "MSH|^~\\&|SENDER|FAC|FI-FHIR|FAC|20260926120000||ADT^A01^ADT_A01|control-1|P|2.5.1\n" +
		"PID|1|ALT-1|REDACTED|ALT-2|REDACTED|Maiden^M|REDACTED|F|Alias^A|2106-3|REDACTED|CNTY|REDACTED|555-0101|EN|S|REL|ACCT-1|REDACTED|DL-1\n" +
		"NK1|1|Synthetic^Kin|SPO|2 Side St^^Town|555-0102\n" +
		"IN1|1|PLAN|COMPANY|Payer|||||||||||||Synthetic^Insured\n" +
		"PV1|1|I"
	if got := redactHL7v2(message); got != want {
		t.Fatalf("redactHL7v2 changed behaviour:\n got %q\nwant %q", got, want)
	}
}

func TestAddSample_CaptureRedaction(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	workspace, err := store.CreateSession(ctx, CreateSessionRequest{Name: "capture redaction"})
	if err != nil {
		t.Fatal(err)
	}
	message := syntheticCaptureMessage("|")

	captured, err := store.AddSample(ctx, workspace.ID, AddSampleRequest{
		Name: "capture #1", Format: events.FormatHL7v2, Source: "capture:c-1", Raw: message,
		PHIPolicy: PHIPolicyRedact, Redaction: SampleRedactionCapture,
	})
	if err != nil {
		t.Fatalf("AddSample with the capture redactor: %v", err)
	}
	if captured.Raw != RedactCapturedHL7v2(message) || !captured.PHIRedacted || captured.Redaction != SampleRedactionCapture {
		t.Fatalf("captured sample = redaction %q, redacted %v, raw matches capture redactor %v",
			captured.Redaction, captured.PHIRedacted, captured.Raw == RedactCapturedHL7v2(message))
	}

	pasted, err := store.AddSample(ctx, workspace.ID, AddSampleRequest{
		Name: "pasted", Format: events.FormatHL7v2, Raw: message,
	})
	if err != nil {
		t.Fatalf("AddSample on the default path: %v", err)
	}
	if pasted.Raw != redactHL7v2(message) || pasted.Redaction != SampleRedactionDefault {
		t.Fatal("the default path no longer applies redactHL7v2 exactly")
	}

	for name, request := range map[string]AddSampleRequest{
		"capture with retain": {Name: "x", Format: events.FormatHL7v2, Raw: message, PHIPolicy: PHIPolicyRetain, Redaction: SampleRedactionCapture},
		"unknown redaction":   {Name: "x", Format: events.FormatHL7v2, Raw: message, Redaction: SampleRedaction("partial")},
	} {
		if _, err := store.AddSample(ctx, workspace.ID, request); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: AddSample error = %v, want ErrInvalid", name, err)
		}
	}
}
