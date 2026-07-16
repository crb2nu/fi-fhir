package main

import "testing"

func TestParseDeliveryRecoveryArgs(t *testing.T) {
	parsed, err := parseDeliveryRecoveryArgs([]string{
		"--tenant", "tenant-a",
		"--attempt", "attempt-a",
		"--idempotency-key", "replay-a",
		"--reason", "Destination repaired",
	})
	if err != nil {
		t.Fatalf("parseDeliveryRecoveryArgs: %v", err)
	}
	if parsed.tenantID != "tenant-a" || parsed.attemptID != "attempt-a" ||
		parsed.idempotencyKey != "replay-a" || parsed.reason != "Destination repaired" {
		t.Fatalf("parsed = %#v", parsed)
	}
}

func TestParseDeliveryRecoveryArgsRejectsIncompleteInput(t *testing.T) {
	if _, err := parseDeliveryRecoveryArgs([]string{"--tenant", "tenant-a"}); err == nil {
		t.Fatal("incomplete recovery input accepted")
	}
	if _, err := parseDeliveryRecoveryArgs([]string{"--unknown", "value"}); err == nil {
		t.Fatal("unknown recovery option accepted")
	}
}
