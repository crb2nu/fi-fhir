package hl7v2

import (
	"testing"
)

func TestLiveParser_ADTMessage(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})
	lp := NewLiveParser(parser)

	msg := "MSH|^~\\&|EPIC|HOSP|FHIR|DEST|20240101120000||ADT^A01|MSG001|P|2.5.1\rPID|1||MRN123^^^HOSP^MR||DOE^JOHN||19800101|M\rPV1|1|I|ICU^101^A|||ATTENDING^DOCTOR"

	events := make(chan ParseEvent, 10)
	go lp.ParseStream(msg, events)

	var collected []ParseEvent
	for ev := range events {
		collected = append(collected, ev)
	}

	if len(collected) != 3 {
		t.Fatalf("Expected 3 segments, got %d", len(collected))
	}

	// Verify segment types
	expectedTypes := []string{"MSH", "PID", "PV1"}
	for i, ev := range collected {
		if ev.SegmentType != expectedTypes[i] {
			t.Errorf("Segment %d: expected type '%s', got '%s'", i, expectedTypes[i], ev.SegmentType)
		}
	}

	// Verify IsComplete flag
	for i, ev := range collected {
		if i < len(collected)-1 && ev.IsComplete {
			t.Errorf("Segment %d: expected IsComplete=false for non-last segment", i)
		}
	}
	if !collected[len(collected)-1].IsComplete {
		t.Error("Last segment should have IsComplete=true")
	}
}

func TestLiveParser_ORUMessage(t *testing.T) {
	parser := NewParser("lab_system", ParserConfig{})
	lp := NewLiveParser(parser)

	msg := "MSH|^~\\&|LAB|LAB_FAC|EHR|HOSPITAL|20240115143000||ORU^R01|LAB001|P|2.5\rPID|1||987654321^^^HOSPITAL^MR||SMITH^JANE|||F\rOBR|1||LAB001|CBC^COMPLETE BLOOD COUNT^L\rOBX|1|NM|WBC^WHITE BLOOD CELL COUNT^L||12.5|10*3/uL|4.5-11.0|H|||F\rOBX|2|NM|RBC^RED BLOOD CELL COUNT^L||4.2|10*6/uL|4.0-5.5|N|||F"

	events := make(chan ParseEvent, 10)
	go lp.ParseStream(msg, events)

	var collected []ParseEvent
	for ev := range events {
		collected = append(collected, ev)
	}

	if len(collected) != 5 {
		t.Fatalf("Expected 5 segments, got %d", len(collected))
	}

	expectedTypes := []string{"MSH", "PID", "OBR", "OBX", "OBX"}
	for i, ev := range collected {
		if ev.SegmentType != expectedTypes[i] {
			t.Errorf("Segment %d: expected type '%s', got '%s'", i, expectedTypes[i], ev.SegmentType)
		}
	}

	// Verify OBX fields are parsed
	obx1 := collected[3]
	if val, ok := obx1.Fields["OBX-5"]; !ok || val != "12.5" {
		t.Errorf("Expected OBX-5='12.5', got '%v'", obx1.Fields["OBX-5"])
	}
}

func TestLiveParser_EmptyMessage(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})
	lp := NewLiveParser(parser)

	events := make(chan ParseEvent, 10)
	go lp.ParseStream("", events)

	var collected []ParseEvent
	for ev := range events {
		collected = append(collected, ev)
	}

	if len(collected) != 0 {
		t.Errorf("Expected 0 segments for empty message, got %d", len(collected))
	}
}

func TestLiveParser_Fields(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})
	lp := NewLiveParser(parser)

	msg := "MSH|^~\\&|EPIC|HOSP|FHIR|DEST|20240101120000||ADT^A01|MSG001|P|2.5.1\rPID|1||MRN123^^^HOSP^MR||DOE^JOHN||19800101|M"

	events := make(chan ParseEvent, 10)
	go lp.ParseStream(msg, events)

	var collected []ParseEvent
	for ev := range events {
		collected = append(collected, ev)
	}

	if len(collected) != 2 {
		t.Fatalf("Expected 2 segments, got %d", len(collected))
	}

	// Verify MSH fields
	msh := collected[0]
	if msh.SegmentType != "MSH" {
		t.Fatalf("Expected MSH segment, got '%s'", msh.SegmentType)
	}

	// MSH-1 is the field separator itself (^~\&)
	if val, ok := msh.Fields["MSH-1"]; !ok || val != "^~\\&" {
		t.Errorf("Expected MSH-1='^~\\&', got '%v'", msh.Fields["MSH-1"])
	}

	// MSH-9 should be the message type
	if val, ok := msh.Fields["MSH-8"]; !ok || val != "ADT^A01" {
		t.Errorf("Expected MSH-8='ADT^A01', got '%v'", msh.Fields["MSH-8"])
	}

	// MSH-10 should be the message control ID
	if val, ok := msh.Fields["MSH-9"]; !ok || val != "MSG001" {
		t.Errorf("Expected MSH-9='MSG001', got '%v'", msh.Fields["MSH-9"])
	}

	// Verify PID fields
	pid := collected[1]
	if pid.SegmentType != "PID" {
		t.Fatalf("Expected PID segment, got '%s'", pid.SegmentType)
	}

	// PID-3 should be MRN (patient ID)
	if val, ok := pid.Fields["PID-3"]; !ok || val != "MRN123^^^HOSP^MR" {
		t.Errorf("Expected PID-3='MRN123^^^HOSP^MR', got '%v'", pid.Fields["PID-3"])
	}

	// PID-5 should be patient name
	if val, ok := pid.Fields["PID-5"]; !ok || val != "DOE^JOHN" {
		t.Errorf("Expected PID-5='DOE^JOHN', got '%v'", pid.Fields["PID-5"])
	}

	// PID-8 should be gender
	if val, ok := pid.Fields["PID-8"]; !ok || val != "M" {
		t.Errorf("Expected PID-8='M', got '%v'", pid.Fields["PID-8"])
	}
}

func TestLiveParser_NewlineNormalization(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})
	lp := NewLiveParser(parser)

	// Test with \n line endings
	msg := "MSH|^~\\&|EPIC|HOSP|FHIR|DEST|20240101120000||ADT^A01|MSG001|P|2.5.1\nPID|1||MRN123|||DOE^JOHN"

	events := make(chan ParseEvent, 10)
	go lp.ParseStream(msg, events)

	var collected []ParseEvent
	for ev := range events {
		collected = append(collected, ev)
	}

	if len(collected) != 2 {
		t.Fatalf("Expected 2 segments with \\n line endings, got %d", len(collected))
	}

	// Test with \r\n line endings
	msg2 := "MSH|^~\\&|EPIC|HOSP|FHIR|DEST|20240101120000||ADT^A01|MSG001|P|2.5.1\r\nPID|1||MRN456|||SMITH^JANE"

	events2 := make(chan ParseEvent, 10)
	go lp.ParseStream(msg2, events2)

	var collected2 []ParseEvent
	for ev := range events2 {
		collected2 = append(collected2, ev)
	}

	if len(collected2) != 2 {
		t.Fatalf("Expected 2 segments with \\r\\n line endings, got %d", len(collected2))
	}
}

func TestLiveParser_MSHWarning(t *testing.T) {
	parser := NewParser("test_source", ParserConfig{})
	lp := NewLiveParser(parser)

	// Short MSH segment with fewer than 12 fields
	msg := "MSH|^~\\&|EPIC|HOSP"

	events := make(chan ParseEvent, 10)
	go lp.ParseStream(msg, events)

	var collected []ParseEvent
	for ev := range events {
		collected = append(collected, ev)
	}

	if len(collected) != 1 {
		t.Fatalf("Expected 1 segment, got %d", len(collected))
	}

	if len(collected[0].Warnings) == 0 {
		t.Error("Expected warning for short MSH segment")
	}
	if len(collected[0].Warnings) > 0 && collected[0].Warnings[0] != "MSH segment has fewer than 12 fields" {
		t.Errorf("Expected MSH warning, got '%s'", collected[0].Warnings[0])
	}
}
