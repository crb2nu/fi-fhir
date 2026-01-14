package edi

import (
	"os"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

func TestMap837ToEvents(t *testing.T) {
	content, err := os.ReadFile("../../../testdata/edi/837p_minimal.edi")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	p := NewParser()
	result, err := p.Parse(string(content))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	tx := result.Interchange.FunctionalGroups[0].Transactions[0]
	claims, err := Map837ToEvents(tx, "test_source")
	if err != nil {
		t.Fatalf("Map837ToEvents failed: %v", err)
	}

	if len(claims) != 1 {
		t.Fatalf("expected 1 claim event, got %d", len(claims))
	}

	claim := claims[0]

	// Verify event metadata
	if claim.Type != events.EventClaimSubmitted {
		t.Errorf("event type = %s, want claim_submitted", claim.Type)
	}
	if claim.Source != "test_source" {
		t.Errorf("source = %s, want test_source", claim.Source)
	}
	if claim.SourceFormat != events.FormatEDI837 {
		t.Errorf("source format = %s, want edi_837", claim.SourceFormat)
	}

	// Verify billing provider
	if claim.BillingProvider.OrganizationName != "SMITH MEDICAL CLINIC" {
		t.Errorf("billing provider name = %s, want SMITH MEDICAL CLINIC", claim.BillingProvider.OrganizationName)
	}
	if claim.BillingProvider.NPI != "1234567890" {
		t.Errorf("billing provider NPI = %s, want 1234567890", claim.BillingProvider.NPI)
	}

	// Verify subscriber
	if claim.Subscriber.FamilyName != "DOE" {
		t.Errorf("subscriber family name = %s, want DOE", claim.Subscriber.FamilyName)
	}
	if claim.Subscriber.GivenName != "JOHN" {
		t.Errorf("subscriber given name = %s, want JOHN", claim.Subscriber.GivenName)
	}

	// Verify claim details
	if claim.Claim.ID != "CLAIM001" {
		t.Errorf("claim ID = %s, want CLAIM001", claim.Claim.ID)
	}
	if claim.Claim.TotalAmount != 150.00 {
		t.Errorf("claim total = %f, want 150.00", claim.Claim.TotalAmount)
	}

	// Verify diagnosis codes
	if len(claim.Claim.DiagnosisCodes) != 1 {
		t.Errorf("diagnosis code count = %d, want 1", len(claim.Claim.DiagnosisCodes))
	} else if claim.Claim.DiagnosisCodes[0] != "J0290" {
		t.Errorf("diagnosis code = %s, want J0290", claim.Claim.DiagnosisCodes[0])
	}

	// Verify service lines
	if len(claim.Claim.ServiceLines) != 1 {
		t.Errorf("service line count = %d, want 1", len(claim.Claim.ServiceLines))
	} else {
		sl := claim.Claim.ServiceLines[0]
		if sl.ProcedureCode != "99213" {
			t.Errorf("procedure code = %s, want 99213", sl.ProcedureCode)
		}
		if sl.ChargeAmount != 150.00 {
			t.Errorf("charge amount = %f, want 150.00", sl.ChargeAmount)
		}
		if sl.Units != 1.0 {
			t.Errorf("units = %f, want 1.0", sl.Units)
		}
	}
}

func TestMap835ToEvents(t *testing.T) {
	// Create a minimal 835 test file
	edi835 := `ISA*00*          *00*          *ZZ*PAYER          *ZZ*PROVIDER       *240115*0800*^*00501*000000002*0*P*:~
GS*HR*PAYER*PROVIDER*20240115*0800*2*X*005010X221A1~
ST*835*0002*005010X221A1~
BPR*I*100.00*C*CHK************20240115~
TRN*1*123456789*1234567890~
N1*PR*MEDICARE*PI*CMS~
N1*PE*SMITH MEDICAL CLINIC*XX*1234567890~
LX*1~
CLP*CLAIM001*1*150.00*100.00**12*PAYERID123*11~
CAS*CO*45*30.00~
CAS*PR*1*20.00~
SVC*HC:99213*150.00*100.00**1~
SE*13*0002~
GE*1*2~
IEA*1*000000002~`

	p := NewParser()
	result, err := p.Parse(edi835)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	tx := result.Interchange.FunctionalGroups[0].Transactions[0]
	remittances, err := Map835ToEvents(tx, "test_source")
	if err != nil {
		t.Fatalf("Map835ToEvents failed: %v", err)
	}

	if len(remittances) != 1 {
		t.Fatalf("expected 1 remittance event, got %d", len(remittances))
	}

	rem := remittances[0]

	// Verify event metadata
	if rem.Type != events.EventClaimAdjudicated {
		t.Errorf("event type = %s, want claim_adjudicated", rem.Type)
	}
	if rem.SourceFormat != events.FormatEDI835 {
		t.Errorf("source format = %s, want edi_835", rem.SourceFormat)
	}

	// Verify check info
	if rem.CheckNumber != "123456789" {
		t.Errorf("check number = %s, want 123456789", rem.CheckNumber)
	}
	if rem.TotalPaid != 100.00 {
		t.Errorf("total paid = %f, want 100.00", rem.TotalPaid)
	}

	// Verify payment details
	if rem.Payment.ClaimID != "CLAIM001" {
		t.Errorf("claim ID = %s, want CLAIM001", rem.Payment.ClaimID)
	}
	if rem.Payment.Status != "Processed as Primary" {
		t.Errorf("status = %s, want Processed as Primary", rem.Payment.Status)
	}
	if rem.Payment.ChargedAmount != 150.00 {
		t.Errorf("charged amount = %f, want 150.00", rem.Payment.ChargedAmount)
	}
	if rem.Payment.PaidAmount != 100.00 {
		t.Errorf("paid amount = %f, want 100.00", rem.Payment.PaidAmount)
	}

	// Verify adjustments
	if len(rem.Payment.Adjustments) != 2 {
		t.Errorf("adjustment count = %d, want 2", len(rem.Payment.Adjustments))
	}

	// Verify service line payment
	if len(rem.Payment.ServiceLinePayments) != 1 {
		t.Errorf("service line payment count = %d, want 1", len(rem.Payment.ServiceLinePayments))
	} else {
		slp := rem.Payment.ServiceLinePayments[0]
		if slp.ProcedureCode != "99213" {
			t.Errorf("procedure code = %s, want 99213", slp.ProcedureCode)
		}
		if slp.PaidAmount != 100.00 {
			t.Errorf("paid amount = %f, want 100.00", slp.PaidAmount)
		}
	}
}

func TestParseEDIDate(t *testing.T) {
	tests := []struct {
		input     string
		wantYear  int
		wantMonth int
		wantDay   int
	}{
		{"20240115", 2024, 1, 15},
		{"19901231", 1990, 12, 31},
		{"240115", 2024, 1, 15},
		{"", 1, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseEDIDate(tt.input)
			if tt.input == "" {
				if !got.IsZero() {
					t.Errorf("expected zero time for empty input")
				}
				return
			}
			if got.Year() != tt.wantYear {
				t.Errorf("year = %d, want %d", got.Year(), tt.wantYear)
			}
			if int(got.Month()) != tt.wantMonth {
				t.Errorf("month = %d, want %d", got.Month(), tt.wantMonth)
			}
			if got.Day() != tt.wantDay {
				t.Errorf("day = %d, want %d", got.Day(), tt.wantDay)
			}
		})
	}
}

func TestMapClaimStatus(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{"1", "Processed as Primary"},
		{"2", "Processed as Secondary"},
		{"4", "Denied"},
		{"19", "Pended"},
		{"99", "Unknown Status: 99"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := mapClaimStatus(tt.code)
			if got != tt.want {
				t.Errorf("mapClaimStatus(%s) = %s, want %s", tt.code, got, tt.want)
			}
		})
	}
}

func TestMap270ToEvents(t *testing.T) {
	content, err := os.ReadFile("../../../testdata/edi/270_inquiry.edi")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	p := NewParser()
	result, err := p.Parse(string(content))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	tx := result.Interchange.FunctionalGroups[0].Transactions[0]
	inquiries, err := Map270ToEvents(tx, "test_source")
	if err != nil {
		t.Fatalf("Map270ToEvents failed: %v", err)
	}

	if len(inquiries) != 1 {
		t.Fatalf("expected 1 inquiry event, got %d", len(inquiries))
	}

	inquiry := inquiries[0]

	// Verify event metadata
	if inquiry.Type != events.EventEligibilityInquiry {
		t.Errorf("event type = %s, want eligibility_inquiry", inquiry.Type)
	}
	if inquiry.Source != "test_source" {
		t.Errorf("source = %s, want test_source", inquiry.Source)
	}
	if inquiry.SourceFormat != events.FormatEDI270 {
		t.Errorf("source format = %s, want edi_270", inquiry.SourceFormat)
	}

	// Verify information source (payer)
	if inquiry.InformationSource.OrganizationName != "ACME HEALTH INSURANCE" {
		t.Errorf("payer name = %s, want ACME HEALTH INSURANCE", inquiry.InformationSource.OrganizationName)
	}

	// Verify information receiver (provider)
	if inquiry.InformationReceiver.OrganizationName != "SMITH MEDICAL CLINIC" {
		t.Errorf("provider name = %s, want SMITH MEDICAL CLINIC", inquiry.InformationReceiver.OrganizationName)
	}
	if inquiry.InformationReceiver.NPI != "1234567890" {
		t.Errorf("provider NPI = %s, want 1234567890", inquiry.InformationReceiver.NPI)
	}

	// Verify subscriber
	if inquiry.Subscriber.FamilyName != "DOE" {
		t.Errorf("subscriber family name = %s, want DOE", inquiry.Subscriber.FamilyName)
	}
	if inquiry.Subscriber.GivenName != "JOHN" {
		t.Errorf("subscriber given name = %s, want JOHN", inquiry.Subscriber.GivenName)
	}
	if inquiry.Subscriber.Gender != "male" {
		t.Errorf("subscriber gender = %s, want male", inquiry.Subscriber.Gender)
	}

	// Verify trace number
	if inquiry.TraceNumber != "TRACE123456" {
		t.Errorf("trace number = %s, want TRACE123456", inquiry.TraceNumber)
	}

	// Verify inquiry service types
	if len(inquiry.Inquiry.ServiceTypes) != 1 {
		t.Errorf("service types count = %d, want 1", len(inquiry.Inquiry.ServiceTypes))
	} else if inquiry.Inquiry.ServiceTypes[0] != "30" {
		t.Errorf("service type = %s, want 30", inquiry.Inquiry.ServiceTypes[0])
	}

	// Verify no dependent (subscriber is patient)
	if inquiry.Dependent != nil {
		t.Errorf("expected no dependent, got %v", inquiry.Dependent)
	}
}

func TestMap271ToEvents(t *testing.T) {
	content, err := os.ReadFile("../../../testdata/edi/271_response.edi")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	p := NewParser()
	result, err := p.Parse(string(content))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	tx := result.Interchange.FunctionalGroups[0].Transactions[0]
	responses, err := Map271ToEvents(tx, "test_source")
	if err != nil {
		t.Fatalf("Map271ToEvents failed: %v", err)
	}

	if len(responses) != 1 {
		t.Fatalf("expected 1 response event, got %d", len(responses))
	}

	resp := responses[0]

	// Verify event metadata
	if resp.Type != events.EventEligibilityResponse {
		t.Errorf("event type = %s, want eligibility_response", resp.Type)
	}
	if resp.SourceFormat != events.FormatEDI271 {
		t.Errorf("source format = %s, want edi_271", resp.SourceFormat)
	}

	// Verify eligibility status
	if resp.Status != events.EligibilityStatusActive {
		t.Errorf("status = %s, want active", resp.Status)
	}

	// Verify payer
	if resp.InformationSource.OrganizationName != "ACME HEALTH INSURANCE" {
		t.Errorf("payer name = %s, want ACME HEALTH INSURANCE", resp.InformationSource.OrganizationName)
	}

	// Verify subscriber
	if resp.Subscriber.FamilyName != "DOE" {
		t.Errorf("subscriber family name = %s, want DOE", resp.Subscriber.FamilyName)
	}

	// Verify trace number
	if resp.TraceNumber != "TRACE123456" {
		t.Errorf("trace number = %s, want TRACE123456", resp.TraceNumber)
	}

	// Verify plan dates
	if resp.PlanBeginDate.Year() != 2024 || resp.PlanBeginDate.Month() != 1 || resp.PlanBeginDate.Day() != 1 {
		t.Errorf("plan begin date = %v, want 2024-01-01", resp.PlanBeginDate)
	}
	if resp.PlanEndDate.Year() != 2024 || resp.PlanEndDate.Month() != 12 || resp.PlanEndDate.Day() != 31 {
		t.Errorf("plan end date = %v, want 2024-12-31", resp.PlanEndDate)
	}

	// Verify benefits
	if len(resp.Benefits) < 3 {
		t.Fatalf("expected at least 3 benefits, got %d", len(resp.Benefits))
	}

	// Check for active coverage benefit
	foundActive := false
	for _, b := range resp.Benefits {
		if b.InformationCode == "1" {
			foundActive = true
			if b.InformationCodeDescription != "Active Coverage" {
				t.Errorf("active coverage description = %s, want Active Coverage", b.InformationCodeDescription)
			}
		}
	}
	if !foundActive {
		t.Error("expected to find active coverage benefit (EB01=1)")
	}

	// Check for deductible benefit
	foundDeductible := false
	for _, b := range resp.Benefits {
		if b.InformationCode == "C" {
			foundDeductible = true
			if b.Amount != 500 {
				t.Errorf("deductible amount = %f, want 500", b.Amount)
			}
			if b.InNetworkIndicator != "Y" {
				t.Errorf("in-network indicator = %s, want Y", b.InNetworkIndicator)
			}
		}
	}
	if !foundDeductible {
		t.Error("expected to find deductible benefit (EB01=C)")
	}

	// Check for coinsurance benefit
	foundCoinsurance := false
	for _, b := range resp.Benefits {
		if b.InformationCode == "A" {
			foundCoinsurance = true
			if b.Percent != 20 {
				t.Errorf("coinsurance percent = %f, want 20", b.Percent)
			}
		}
	}
	if !foundCoinsurance {
		t.Error("expected to find coinsurance benefit (EB01=A)")
	}

	// Verify no errors
	if len(resp.Errors) != 0 {
		t.Errorf("expected no errors, got %d", len(resp.Errors))
	}
}

func TestMap271ToEventsRejected(t *testing.T) {
	content, err := os.ReadFile("../../../testdata/edi/271_rejected.edi")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	p := NewParser()
	result, err := p.Parse(string(content))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	tx := result.Interchange.FunctionalGroups[0].Transactions[0]
	responses, err := Map271ToEvents(tx, "test_source")
	if err != nil {
		t.Fatalf("Map271ToEvents failed: %v", err)
	}

	if len(responses) != 1 {
		t.Fatalf("expected 1 response event, got %d", len(responses))
	}

	resp := responses[0]

	// Verify rejected status
	if resp.Status != events.EligibilityStatusRejected {
		t.Errorf("status = %s, want rejected", resp.Status)
	}

	// Verify errors are captured
	if len(resp.Errors) < 1 {
		t.Fatalf("expected at least 1 error, got %d", len(resp.Errors))
	}

	// Check for subscriber not found error
	foundError := false
	for _, e := range resp.Errors {
		if e.RejectReasonCode == "75" || e.RejectReasonCode == "67" {
			foundError = true
		}
	}
	if !foundError {
		t.Error("expected to find subscriber/patient not found error (AAA03=75 or 67)")
	}
}

func TestMapBenefitInfoCode(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{"1", "Active Coverage"},
		{"6", "Inactive"},
		{"C", "Deductible"},
		{"A", "Co-Insurance"},
		{"B", "Co-Payment"},
		{"99", "Unknown: 99"},
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := mapBenefitInfoCode(tt.code)
			if got != tt.want {
				t.Errorf("mapBenefitInfoCode(%s) = %s, want %s", tt.code, got, tt.want)
			}
		})
	}
}

func TestMapServiceTypeCode(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{"30", "Health Benefit Plan Coverage"},
		{"1", "Medical Care"},
		{"MH", "Mental Health"},
		{"99", ""}, // Unknown service type returns empty
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := mapServiceTypeCode(tt.code)
			if got != tt.want {
				t.Errorf("mapServiceTypeCode(%s) = %s, want %s", tt.code, got, tt.want)
			}
		})
	}
}

func TestMapAAARejectReason(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{"67", "Patient Not Found"},
		{"75", "Subscriber/Insured Not Found"},
		{"72", "Invalid/Missing Subscriber/Insured ID"},
		{"XX", "Reject Reason: XX"}, // Unknown reason
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := mapAAARejectReason(tt.code)
			if got != tt.want {
				t.Errorf("mapAAARejectReason(%s) = %s, want %s", tt.code, got, tt.want)
			}
		})
	}
}

func TestMap276ToEvents(t *testing.T) {
	content, err := os.ReadFile("../../../testdata/edi/276_request.edi")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	p := NewParser()
	result, err := p.Parse(string(content))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	tx := result.Interchange.FunctionalGroups[0].Transactions[0]
	requests, err := Map276ToEvents(tx, "test_source")
	if err != nil {
		t.Fatalf("Map276ToEvents failed: %v", err)
	}

	if len(requests) != 1 {
		t.Fatalf("expected 1 request event, got %d", len(requests))
	}

	req := requests[0]

	// Verify event metadata
	if req.Type != events.EventClaimStatusRequest {
		t.Errorf("event type = %s, want claim_status_request", req.Type)
	}
	if req.Source != "test_source" {
		t.Errorf("source = %s, want test_source", req.Source)
	}
	if req.SourceFormat != events.FormatEDI276 {
		t.Errorf("source format = %s, want edi_276", req.SourceFormat)
	}

	// Verify payer (information source)
	if req.Payer.OrganizationName != "ACME HEALTH INSURANCE" {
		t.Errorf("payer name = %s, want ACME HEALTH INSURANCE", req.Payer.OrganizationName)
	}

	// Verify provider (information receiver)
	if req.Provider.OrganizationName != "SMITH MEDICAL CLINIC" {
		t.Errorf("provider name = %s, want SMITH MEDICAL CLINIC", req.Provider.OrganizationName)
	}
	if req.Provider.NPI != "1234567890" {
		t.Errorf("provider NPI = %s, want 1234567890", req.Provider.NPI)
	}

	// Verify subscriber
	if req.Subscriber.FamilyName != "DOE" {
		t.Errorf("subscriber family name = %s, want DOE", req.Subscriber.FamilyName)
	}
	if req.Subscriber.GivenName != "JOHN" {
		t.Errorf("subscriber given name = %s, want JOHN", req.Subscriber.GivenName)
	}
	if req.Subscriber.Gender != "male" {
		t.Errorf("subscriber gender = %s, want male", req.Subscriber.Gender)
	}

	// Verify trace number
	if req.TraceNumber != "TRACE276001" {
		t.Errorf("trace number = %s, want TRACE276001", req.TraceNumber)
	}

	// Verify inquiry - REF*D9 is ClaimSubmitterID, REF*1K is PayerClaimID
	if req.Inquiry.ClaimSubmitterID != "12345678" {
		t.Errorf("claim submitter ID = %s, want 12345678", req.Inquiry.ClaimSubmitterID)
	}
	if req.Inquiry.PayerClaimID != "CLM123456" {
		t.Errorf("payer claim ID = %s, want CLM123456", req.Inquiry.PayerClaimID)
	}
	if req.Inquiry.TotalClaimChargeAmount != 1500.00 {
		t.Errorf("claim amount = %f, want 1500.00", req.Inquiry.TotalClaimChargeAmount)
	}

	// Verify no dependent (subscriber is patient)
	if req.Dependent != nil {
		t.Errorf("expected no dependent, got %v", req.Dependent)
	}
}

func TestMap277ToEvents(t *testing.T) {
	content, err := os.ReadFile("../../../testdata/edi/277_response.edi")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	p := NewParser()
	result, err := p.Parse(string(content))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	tx := result.Interchange.FunctionalGroups[0].Transactions[0]
	responses, err := Map277ToEvents(tx, "test_source")
	if err != nil {
		t.Fatalf("Map277ToEvents failed: %v", err)
	}

	if len(responses) != 1 {
		t.Fatalf("expected 1 response event, got %d", len(responses))
	}

	resp := responses[0]

	// Verify event metadata
	if resp.Type != events.EventClaimStatusResponse {
		t.Errorf("event type = %s, want claim_status_response", resp.Type)
	}
	if resp.SourceFormat != events.FormatEDI277 {
		t.Errorf("source format = %s, want edi_277", resp.SourceFormat)
	}

	// Verify payer
	if resp.Payer.OrganizationName != "ACME HEALTH INSURANCE" {
		t.Errorf("payer name = %s, want ACME HEALTH INSURANCE", resp.Payer.OrganizationName)
	}

	// Verify provider
	if resp.Provider.OrganizationName != "SMITH MEDICAL CLINIC" {
		t.Errorf("provider name = %s, want SMITH MEDICAL CLINIC", resp.Provider.OrganizationName)
	}

	// Verify subscriber
	if resp.Subscriber.FamilyName != "DOE" {
		t.Errorf("subscriber family name = %s, want DOE", resp.Subscriber.FamilyName)
	}

	// Verify trace number
	if resp.TraceNumber != "TRACE276001" {
		t.Errorf("trace number = %s, want TRACE276001", resp.TraceNumber)
	}

	// Verify claim status - REF*D9 is ClaimSubmitterID, REF*1K is PayerClaimID
	if resp.ClaimSubmitterID != "12345678" {
		t.Errorf("claim submitter ID = %s, want 12345678", resp.ClaimSubmitterID)
	}
	if resp.PayerClaimID != "CLM123456" {
		t.Errorf("payer claim ID = %s, want CLM123456", resp.PayerClaimID)
	}

	// Verify status info - should have at least one status
	if len(resp.Statuses) < 1 {
		t.Fatalf("expected at least 1 status, got %d", len(resp.Statuses))
	}

	// Check for finalized status (A2 = Finalized)
	foundFinalized := false
	for _, s := range resp.Statuses {
		if s.StatusCategoryCode == events.ClaimStatusCategoryFinalized {
			foundFinalized = true
			if s.StatusCategoryDescription == "" {
				t.Errorf("expected category description for finalized status")
			}
		}
	}
	if !foundFinalized {
		t.Error("expected to find finalized status (A2)")
	}

	// Verify service line statuses
	if len(resp.ServiceLines) < 1 {
		t.Errorf("expected at least 1 service line status, got %d", len(resp.ServiceLines))
	}
}

func TestMap277ToEventsDenied(t *testing.T) {
	content, err := os.ReadFile("../../../testdata/edi/277_denied.edi")
	if err != nil {
		t.Fatalf("failed to read test file: %v", err)
	}

	p := NewParser()
	result, err := p.Parse(string(content))
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	tx := result.Interchange.FunctionalGroups[0].Transactions[0]
	responses, err := Map277ToEvents(tx, "test_source")
	if err != nil {
		t.Fatalf("Map277ToEvents failed: %v", err)
	}

	if len(responses) != 1 {
		t.Fatalf("expected 1 response event, got %d", len(responses))
	}

	resp := responses[0]

	// Verify subscriber (different patient)
	if resp.Subscriber.FamilyName != "SMITH" {
		t.Errorf("subscriber family name = %s, want SMITH", resp.Subscriber.FamilyName)
	}
	if resp.Subscriber.GivenName != "JANE" {
		t.Errorf("subscriber given name = %s, want JANE", resp.Subscriber.GivenName)
	}
	if resp.Subscriber.Gender != "female" {
		t.Errorf("subscriber gender = %s, want female", resp.Subscriber.Gender)
	}

	// Verify claim ID - REF*D9 is ClaimSubmitterID, REF*1K is PayerClaimID
	if resp.ClaimSubmitterID != "87654321" {
		t.Errorf("claim submitter ID = %s, want 87654321", resp.ClaimSubmitterID)
	}
	if resp.PayerClaimID != "CLM789012" {
		t.Errorf("payer claim ID = %s, want CLM789012", resp.PayerClaimID)
	}

	// Verify denied status (A8 = Rejected/Denied)
	foundDenied := false
	for _, s := range resp.Statuses {
		if s.StatusCategoryCode == events.ClaimStatusCategoryRejected {
			foundDenied = true
		}
	}
	if !foundDenied {
		t.Error("expected to find rejected/denied status (A8)")
	}
}

func TestMapStatusCategoryCode(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{"A0", "Acknowledgement/Receipt - The claim/encounter has been received"},
		{"A1", "Pending - The claim/encounter is pending further review"},
		{"A2", "Finalized - The claim/encounter processing is complete"},
		{"A8", "Rejected - The claim/encounter has been rejected"},
		{"XX", "Status Category: XX"}, // Unknown category
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := mapStatusCategoryCode(tt.code)
			if got != tt.want {
				t.Errorf("mapStatusCategoryCode(%s) = %s, want %s", tt.code, got, tt.want)
			}
		})
	}
}

func TestMapStatusCode(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{"20", "Entity accepts responsibility for claim/encounter"},
		{"0", "Cannot provide further status electronically"},
		{"1", "For more detailed information, see remittance advice"},
		{"YY", "Status: YY"}, // Unknown code
	}

	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			got := mapStatusCode(tt.code)
			if got != tt.want {
				t.Errorf("mapStatusCode(%s) = %s, want %s", tt.code, got, tt.want)
			}
		})
	}
}
