package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// parseEDIFixture runs `fi-fhir parse --format edi` on a testdata fixture and
// decodes stdout as the canonical event array. It fails the test if any
// transaction fell through to the unknown_transaction fallback.
func parseEDIFixture(t *testing.T, fixture string) []map[string]interface{} {
	t.Helper()

	inputPath := testdataPath(t, fixture)
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		t.Skipf("testdata file %s not found", fixture)
	}

	stdout, _, err := runCLI(t, "parse", "--format", "edi", inputPath)
	assertNoError(t, err)

	if strings.Contains(stdout, "unknown_transaction") {
		t.Fatalf("transaction fell through to unknown_transaction fallback:\n%s", stdout)
	}

	var evts []map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &evts); err != nil {
		t.Fatalf("failed to decode output as event array: %v\noutput:\n%s", err, stdout)
	}
	if len(evts) == 0 {
		t.Fatalf("expected at least one mapped event for %s", fixture)
	}
	return evts
}

func TestParse_EDI_271_MapsEligibilityResponse(t *testing.T) {
	evts := parseEDIFixture(t, "edi/271_response.edi")

	evt := evts[0]
	if evt["type"] != "eligibility_response" {
		t.Errorf("expected type eligibility_response, got %v", evt["type"])
	}
	if evt["status"] != "active" {
		t.Errorf("expected status active, got %v", evt["status"])
	}
	if evt["trace_number"] != "TRACE123456" {
		t.Errorf("expected trace_number TRACE123456, got %v", evt["trace_number"])
	}
	benefits, ok := evt["benefits"].([]interface{})
	if !ok || len(benefits) == 0 {
		t.Errorf("expected non-empty benefits, got %v", evt["benefits"])
	}
}

func TestParse_EDI_271_Rejected_MapsErrors(t *testing.T) {
	evts := parseEDIFixture(t, "edi/271_rejected.edi")

	evt := evts[0]
	if evt["type"] != "eligibility_response" {
		t.Errorf("expected type eligibility_response, got %v", evt["type"])
	}
	if evt["status"] != "rejected" {
		t.Errorf("expected status rejected, got %v", evt["status"])
	}
	errs, ok := evt["errors"].([]interface{})
	if !ok || len(errs) == 0 {
		t.Errorf("expected non-empty errors from AAA segments, got %v", evt["errors"])
	}
}

func TestParse_EDI_270_MapsEligibilityInquiry(t *testing.T) {
	evts := parseEDIFixture(t, "edi/270_inquiry.edi")

	evt := evts[0]
	if evt["type"] != "eligibility_inquiry" {
		t.Errorf("expected type eligibility_inquiry, got %v", evt["type"])
	}
	if _, ok := evt["inquiry"]; !ok {
		t.Error("expected inquiry field on eligibility_inquiry event")
	}
}

func TestParse_EDI_276_MapsClaimStatusRequest(t *testing.T) {
	evts := parseEDIFixture(t, "edi/276_request.edi")

	evt := evts[0]
	if evt["type"] != "claim_status_request" {
		t.Errorf("expected type claim_status_request, got %v", evt["type"])
	}
}

func TestParse_EDI_277_MapsClaimStatusResponse(t *testing.T) {
	evts := parseEDIFixture(t, "edi/277_response.edi")

	evt := evts[0]
	if evt["type"] != "claim_status_response" {
		t.Errorf("expected type claim_status_response, got %v", evt["type"])
	}
	if evt["trace_number"] != "TRACE276001" {
		t.Errorf("expected trace_number TRACE276001, got %v", evt["trace_number"])
	}
	statuses, ok := evt["statuses"].([]interface{})
	if !ok || len(statuses) == 0 {
		t.Fatalf("expected non-empty statuses from STC segments, got %v", evt["statuses"])
	}
	first, ok := statuses[0].(map[string]interface{})
	if !ok || first["status_category_code"] != "A2" {
		t.Errorf("expected first status_category_code A2, got %v", statuses[0])
	}
}

func TestParse_EDI_277_Denied(t *testing.T) {
	evts := parseEDIFixture(t, "edi/277_denied.edi")

	evt := evts[0]
	if evt["type"] != "claim_status_response" {
		t.Errorf("expected type claim_status_response, got %v", evt["type"])
	}
}
