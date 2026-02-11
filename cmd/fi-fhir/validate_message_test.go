package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateMessage_HL7v2_WarningsBecomeIssues(t *testing.T) {
	// Create a message that triggers a semantic warning via identifier validation:
	// PID-3 includes an invalid NPI repetition (value "123").
	msg := "MSH|^~\\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240115120000||ADT^A01|MSG00001|P|2.5\r" +
		"EVN|A01|20240115120000\r" +
		"PID|1||123456789^^^HOSPITAL^MRN~123^^^HOSPITAL^NPI||DOE^JOHN^WILLIAM^^^MR||19800315|M|||123 MAIN ST^^ANYTOWN^VA^24101^USA||5551234567\r" +
		"PV1|1|I|ICU^101^A^HOSPITAL\r"

	dir := t.TempDir()
	msgPath := filepath.Join(dir, "msg.hl7")
	if err := os.WriteFile(msgPath, []byte(msg), 0o600); err != nil {
		t.Fatalf("write message: %v", err)
	}

	profilePath := filepath.Join("..", "..", "testdata", "profiles", "inferred", "adt_a01_inferred.yaml")

	errs := validateMessage(profilePath, msgPath, "hl7v2", false)
	if len(errs) == 0 {
		t.Fatalf("expected at least one issue from invalid NPI")
	}
}
