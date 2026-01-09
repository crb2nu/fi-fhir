package edi

import (
	"os"
	"testing"

	"github.com/cblevins/fi-fhir/pkg/events"
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
