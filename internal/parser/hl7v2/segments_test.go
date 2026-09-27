package hl7v2

import "testing"

func TestSplitMessageMatchesTheParsersTokenization(t *testing.T) {
	raw := "MSH|^~\\&|APP|FAC|DEST|FAC|20260115143022||ADT^A01^ADT_A01|CTRL1|P|2.5.1\n" +
		"PID|1||MRN1^^^HOSP^MR||DOE^JANE\r\n" +
		"\n" +
		"PV1|1|I\r"
	msg, err := SplitMessage(raw)
	if err != nil {
		t.Fatalf("SplitMessage: %v", err)
	}
	if len(msg.Segments) != 3 {
		t.Fatalf("segments = %d, want 3 (blank lines dropped)", len(msg.Segments))
	}
	msh := msg.Segments[0]
	if msh.ID != "MSH" || msh.Fields[1] != "|" || msh.Fields[2] != "^~\\&" || msh.Fields[9] != "ADT^A01^ADT_A01" {
		t.Fatalf("MSH fields are not indexed by HL7 field number: %q", msh.Fields)
	}
	if pid := msg.Segments[1]; pid.ID != "PID" || pid.Fields[5] != "DOE^JANE" {
		t.Fatalf("PID = %q", pid.Fields)
	}
	if msg.Type != "ADT^A01^ADT_A01" || msg.ControlID != "CTRL1" || msg.Version != "2.5.1" {
		t.Fatalf("header = %q %q %q", msg.Type, msg.ControlID, msg.Version)
	}

	if _, err := SplitMessage("PID|1||MRN1"); err == nil {
		t.Fatal("a message without a leading MSH was tokenized")
	}
}
