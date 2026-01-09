package edi

import (
	"strconv"
	"strings"
	"time"

	"github.com/cblevins/fi-fhir/pkg/events"
)

// Map837ToEvents converts a parsed 837 transaction to ClaimSubmittedEvent(s)
func Map837ToEvents(tx *Transaction, source string) ([]*events.ClaimSubmittedEvent, error) {
	loops := Parse837Loops(tx)
	var results []*events.ClaimSubmittedEvent

	// Process each billing provider hierarchy
	for _, bp := range loops.BillingProviders {
		billingProvider := mapNM1ToProvider(bp.BillingName)

		// Process each subscriber under this billing provider
		for _, sub := range bp.Subscribers {
			subscriber := mapNM1ToPatient(sub.SubscriberInfo)
			payer := mapNM1ToProvider(sub.PayerInfo)

			// Determine patient - could be subscriber or dependent
			patient := subscriber
			if sub.Patient != nil && sub.Patient.PatientInfo != nil {
				patient = mapNM1ToPatient(sub.Patient.PatientInfo)
			}

			// Process each claim under this subscriber
			for _, clm := range sub.Claims {
				event := &events.ClaimSubmittedEvent{
					EventMeta: events.NewEventMeta(
						events.EventClaimSubmitted,
						source,
						events.FormatEDI837,
					),
					Patient:         patient,
					BillingProvider: billingProvider,
					Subscriber:      subscriber,
					Payer:           payer,
					Claim:           mapCLMToClaim(clm),
				}

				event.SourceMessageID = tx.ControlNumber

				results = append(results, event)
			}
		}
	}

	return results, nil
}

// Map835ToEvents converts a parsed 835 transaction to ClaimAdjudicatedEvent(s)
func Map835ToEvents(tx *Transaction, source string) ([]*events.ClaimAdjudicatedEvent, error) {
	loops := Parse835Loops(tx)
	var results []*events.ClaimAdjudicatedEvent

	payer := mapN1ToProvider(loops.Payer)
	payee := mapN1ToProvider(loops.Payee)

	// Extract check info from BPR/TRN
	checkNumber := ""
	var checkDate time.Time
	totalPaid := 0.0

	if loops.BPR != nil {
		// BPR01 = Transaction Handling Code
		// BPR02 = Monetary Amount
		if amount := loops.BPR.GetElement(2); amount != "" {
			totalPaid, _ = strconv.ParseFloat(amount, 64)
		}
		// BPR16 = Check Issue Date
		if date := loops.BPR.GetElement(16); date != "" {
			checkDate = parseEDIDate(date)
		}
	}

	if loops.TRN != nil {
		// TRN02 = Reference Identification (check/EFT number)
		checkNumber = loops.TRN.GetElement(2)
	}

	// Process each header (can have multiple check stubs)
	for _, header := range loops.Headers {
		// Process each claim in this header
		for _, clp := range header.Claims {
			payment := mapCLPToPayment(clp)

			event := &events.ClaimAdjudicatedEvent{
				EventMeta: events.NewEventMeta(
					events.EventClaimAdjudicated,
					source,
					events.FormatEDI835,
				),
				Payer:       payer,
				Payee:       payee,
				CheckNumber: checkNumber,
				CheckDate:   checkDate,
				TotalPaid:   totalPaid,
				Payment:     payment,
			}

			event.SourceMessageID = tx.ControlNumber

			results = append(results, event)
		}
	}

	return results, nil
}

// mapNM1ToProvider maps an NM1 loop to a Provider
func mapNM1ToProvider(loop *Loop2010) events.Provider {
	if loop == nil || loop.NM1 == nil {
		return events.Provider{}
	}

	nm1 := loop.NM1
	provider := events.Provider{}

	// NM102: Entity Type Qualifier (1=Person, 2=Non-Person Entity)
	entityType := nm1.GetElement(2)

	if entityType == "2" {
		// Organization
		provider.OrganizationName = nm1.GetElement(3)
		provider.ProviderType = "organization"
	} else {
		// Individual
		provider.FamilyName = nm1.GetElement(3)
		provider.GivenName = nm1.GetElement(4)
		provider.MiddleName = nm1.GetElement(5)
		provider.Prefix = nm1.GetElement(6)
		provider.Suffix = nm1.GetElement(7)
		provider.ProviderType = "individual"
	}

	// NM108: ID Code Qualifier, NM109: ID Code
	idQual := nm1.GetElement(8)
	idCode := nm1.GetElement(9)

	if idCode != "" {
		var idType string
		switch idQual {
		case "XX": // NPI
			provider.NPI = idCode
			idType = "NPI"
		case "24": // EIN
			idType = "EIN"
		case "34": // SSN
			idType = "SSN"
		case "PI": // Payer ID
			idType = "PI"
		case "MI": // Member ID
			idType = "MI"
		default:
			idType = idQual
		}

		provider.Identifiers.Identifiers = append(provider.Identifiers.Identifiers, events.Identifier{
			Type:  idType,
			Value: idCode,
		})
	}

	// Extract REF segments for additional IDs
	for _, ref := range loop.REF {
		refQual := ref.GetElement(1)
		refValue := ref.GetElement(2)

		switch refQual {
		case "EI": // Tax ID
			provider.Identifiers.Identifiers = append(provider.Identifiers.Identifiers, events.Identifier{
				Type:  "EIN",
				Value: refValue,
			})
		case "SY": // SSN
			provider.Identifiers.Identifiers = append(provider.Identifiers.Identifiers, events.Identifier{
				Type:  "SSN",
				Value: refValue,
			})
		}
	}

	return provider
}

// mapNM1ToPatient maps an NM1 loop to a Patient
func mapNM1ToPatient(loop *Loop2010) events.Patient {
	if loop == nil || loop.NM1 == nil {
		return events.Patient{}
	}

	nm1 := loop.NM1
	patient := events.Patient{}

	patient.FamilyName = nm1.GetElement(3)
	patient.GivenName = nm1.GetElement(4)
	patient.MiddleName = nm1.GetElement(5)
	patient.Prefix = nm1.GetElement(6)
	patient.Suffix = nm1.GetElement(7)

	// ID from NM1
	idQual := nm1.GetElement(8)
	idCode := nm1.GetElement(9)

	if idCode != "" {
		var idType string
		switch idQual {
		case "MI": // Member ID
			idType = "MI"
			patient.MRN = idCode
		case "II": // Standard Unique Health Identifier
			idType = "SUHI"
		default:
			idType = idQual
		}

		patient.Identifiers.Identifiers = append(patient.Identifiers.Identifiers, events.Identifier{
			Type:  idType,
			Value: idCode,
		})
	}

	// Address from N3/N4
	if loop.N3 != nil {
		patient.Address.Line1 = loop.N3.GetElement(1)
		patient.Address.Line2 = loop.N3.GetElement(2)
	}

	if loop.N4 != nil {
		patient.Address.City = loop.N4.GetElement(1)
		patient.Address.State = loop.N4.GetElement(2)
		patient.Address.PostalCode = loop.N4.GetElement(3)
		patient.Address.Country = loop.N4.GetElement(4)
	}

	// Demographics from DMG
	if loop.DMG != nil {
		// DMG01: Date Time Period Format Qualifier (D8=CCYYMMDD)
		// DMG02: Date of Birth
		if dob := loop.DMG.GetElement(2); dob != "" {
			patient.DateOfBirth = parseEDIDate(dob)
		}

		// DMG03: Gender Code (F, M, U)
		gender := loop.DMG.GetElement(3)
		switch gender {
		case "F":
			patient.Gender = "female"
		case "M":
			patient.Gender = "male"
		case "U":
			patient.Gender = "unknown"
		}
	}

	return patient
}

// mapN1ToProvider maps an N1 segment (used in 835) to a Provider
func mapN1ToProvider(loop *Loop1000) events.Provider {
	if loop == nil || loop.NM1 == nil {
		return events.Provider{}
	}

	// In 835, N1 is used but we store in NM1 field
	seg := loop.NM1
	provider := events.Provider{}

	// N102: Name
	provider.OrganizationName = seg.GetElement(2)

	// N103: ID Code Qualifier, N104: ID Code
	idQual := seg.GetElement(3)
	idCode := seg.GetElement(4)

	if idCode != "" {
		var idType string
		switch idQual {
		case "XX":
			provider.NPI = idCode
			idType = "NPI"
		case "PI":
			idType = "PI"
		default:
			idType = idQual
		}

		provider.Identifiers.Identifiers = append(provider.Identifiers.Identifiers, events.Identifier{
			Type:  idType,
			Value: idCode,
		})
	}

	return provider
}

// mapCLMToClaim maps a CLM loop to a Claim
func mapCLMToClaim(loop *Loop2300) events.Claim {
	if loop == nil || loop.CLM == nil {
		return events.Claim{}
	}

	clm := loop.CLM
	claim := events.Claim{
		ID: clm.GetElement(1),
	}

	// CLM02: Total Claim Charge Amount
	if amount := clm.GetElement(2); amount != "" {
		claim.TotalAmount, _ = strconv.ParseFloat(amount, 64)
	}

	// CLM05: Place of Service (composite: Place:Frequency:ClaimType)
	if pos := clm.GetComponent(5, 1, ':'); pos != "" {
		claim.PlaceOfService = pos
	}

	// Extract diagnosis codes from HI segments
	for _, hi := range loop.HI {
		// HI segments contain diagnosis codes
		// Format: HI*ABK:J0290*ABF:E119~
		for i := 1; i <= 12; i++ {
			element := hi.GetElement(i)
			if element == "" {
				break
			}
			// Split composite: qualifier:code
			parts := strings.Split(element, ":")
			if len(parts) >= 2 {
				code := parts[1]
				claim.DiagnosisCodes = append(claim.DiagnosisCodes, code)
			}
		}
	}

	// Map service lines
	for i, sl := range loop.ServiceLines {
		serviceLine := mapSV1ToServiceLine(sl, i+1)
		claim.ServiceLines = append(claim.ServiceLines, serviceLine)
	}

	return claim
}

// mapSV1ToServiceLine maps an SV1 segment to a ServiceLine
func mapSV1ToServiceLine(loop *Loop2400, lineNum int) events.ServiceLine {
	line := events.ServiceLine{
		LineNumber: lineNum,
	}

	if loop.SV1 != nil {
		sv1 := loop.SV1

		// SV101: Composite Medical Procedure (HC:code:mod1:mod2:mod3:mod4)
		procElement := sv1.GetElement(1)
		if procElement != "" {
			parts := strings.Split(procElement, ":")
			if len(parts) >= 2 {
				line.ProcedureCode = parts[1]
			}
			// Modifiers are in positions 3-6
			for i := 2; i < len(parts) && i < 6; i++ {
				if parts[i] != "" {
					line.Modifiers = append(line.Modifiers, parts[i])
				}
			}
		}

		// SV102: Line Item Charge Amount
		if amount := sv1.GetElement(2); amount != "" {
			line.ChargeAmount, _ = strconv.ParseFloat(amount, 64)
		}

		// SV103: Unit or Basis for Measurement Code
		line.UnitType = sv1.GetElement(3)

		// SV104: Service Unit Count
		if units := sv1.GetElement(4); units != "" {
			line.Units, _ = strconv.ParseFloat(units, 64)
		}

		// SV107: Composite Diagnosis Code Pointer
		if pointers := sv1.GetElement(7); pointers != "" {
			parts := strings.Split(pointers, ":")
			for _, p := range parts {
				if idx, err := strconv.Atoi(p); err == nil {
					line.DiagnosisPointers = append(line.DiagnosisPointers, idx)
				}
			}
		}
	}

	// Extract service date from DTP
	for _, dtp := range loop.DTP {
		// DTP01=472 is Service Date
		if dtp.GetElement(1) == "472" {
			// DTP02: Date Time Period Format Qualifier (D8=single date)
			// DTP03: Date
			if date := dtp.GetElement(3); date != "" {
				line.ServiceDate = parseEDIDate(date)
			}
		}
	}

	return line
}

// mapCLPToPayment maps a CLP loop to a ClaimPayment
func mapCLPToPayment(loop *Loop835Claim) events.ClaimPayment {
	if loop == nil || loop.CLP == nil {
		return events.ClaimPayment{}
	}

	clp := loop.CLP
	payment := events.ClaimPayment{
		ClaimID: clp.GetElement(1),
	}

	// CLP02: Claim Status Code
	statusCode := clp.GetElement(2)
	payment.Status = mapClaimStatus(statusCode)

	// CLP03: Total Claim Charge Amount
	if amount := clp.GetElement(3); amount != "" {
		payment.ChargedAmount, _ = strconv.ParseFloat(amount, 64)
	}

	// CLP04: Claim Payment Amount
	if amount := clp.GetElement(4); amount != "" {
		payment.PaidAmount, _ = strconv.ParseFloat(amount, 64)
	}

	// CLP07: Payer Claim Control Number
	payment.PayerClaimID = clp.GetElement(7)

	// Map claim-level adjustments
	for _, cas := range loop.CAS {
		adj := mapCASToAdjustment(cas)
		payment.Adjustments = append(payment.Adjustments, adj...)

		// Calculate patient responsibility from PR adjustments
		if cas.GetElement(1) == "PR" {
			for i := 2; i <= 18; i += 3 {
				if amtStr := cas.GetElement(i + 1); amtStr != "" {
					amt, _ := strconv.ParseFloat(amtStr, 64)
					payment.PatientResponsibility += amt
				}
			}
		}
	}

	// Map service line payments
	for _, svc := range loop.ServiceLines {
		slp := mapSVCToServiceLinePayment(svc)
		payment.ServiceLinePayments = append(payment.ServiceLinePayments, slp)
	}

	return payment
}

// mapCASToAdjustment maps a CAS segment to ClaimAdjustment(s)
func mapCASToAdjustment(cas *Segment) []events.ClaimAdjustment {
	if cas == nil {
		return nil
	}

	var adjustments []events.ClaimAdjustment
	group := cas.GetElement(1)

	// CAS can have up to 6 adjustment groups (elements 2-4, 5-7, 8-10, etc.)
	for i := 2; i <= 18; i += 3 {
		reasonCode := cas.GetElement(i)
		if reasonCode == "" {
			break
		}

		adj := events.ClaimAdjustment{
			Group:      group,
			ReasonCode: reasonCode,
		}

		if amtStr := cas.GetElement(i + 1); amtStr != "" {
			adj.Amount, _ = strconv.ParseFloat(amtStr, 64)
		}

		if qtyStr := cas.GetElement(i + 2); qtyStr != "" {
			adj.Quantity, _ = strconv.Atoi(qtyStr)
		}

		adjustments = append(adjustments, adj)
	}

	return adjustments
}

// mapSVCToServiceLinePayment maps an SVC loop to ServiceLinePayment
func mapSVCToServiceLinePayment(loop *Loop835Service) events.ServiceLinePayment {
	if loop == nil || loop.SVC == nil {
		return events.ServiceLinePayment{}
	}

	svc := loop.SVC
	slp := events.ServiceLinePayment{}

	// SVC01: Composite Medical Procedure
	procElement := svc.GetElement(1)
	if procElement != "" {
		parts := strings.Split(procElement, ":")
		if len(parts) >= 2 {
			slp.ProcedureCode = parts[1]
		}
	}

	// SVC02: Line Item Charge Amount
	if amount := svc.GetElement(2); amount != "" {
		slp.ChargedAmount, _ = strconv.ParseFloat(amount, 64)
	}

	// SVC03: Line Item Payment Amount
	if amount := svc.GetElement(3); amount != "" {
		slp.PaidAmount, _ = strconv.ParseFloat(amount, 64)
	}

	// SVC05: Units of Service Paid Count
	if units := svc.GetElement(5); units != "" {
		slp.Units, _ = strconv.ParseFloat(units, 64)
	}

	// Map line-level adjustments
	for _, cas := range loop.CAS {
		adj := mapCASToAdjustment(cas)
		slp.Adjustments = append(slp.Adjustments, adj...)
	}

	return slp
}

// mapClaimStatus converts CLP02 status code to human-readable status
func mapClaimStatus(code string) string {
	switch code {
	case "1":
		return "Processed as Primary"
	case "2":
		return "Processed as Secondary"
	case "3":
		return "Processed as Tertiary"
	case "4":
		return "Denied"
	case "19":
		return "Pended"
	case "20":
		return "Processed as Primary, Forwarded to Additional Payer"
	case "21":
		return "Processed as Secondary, Forwarded to Additional Payer"
	case "22":
		return "Reversal of Previous Payment"
	case "23":
		return "Not Our Claim, Forwarded to Additional Payer"
	default:
		return "Unknown Status: " + code
	}
}

// parseEDIDate parses X12 date formats (CCYYMMDD or YYMMDD)
func parseEDIDate(s string) time.Time {
	if s == "" {
		return time.Time{}
	}

	var t time.Time
	var err error

	switch len(s) {
	case 8:
		t, err = time.Parse("20060102", s)
	case 6:
		t, err = time.Parse("060102", s)
		// Adjust for century
		if t.Year() < 50 {
			t = t.AddDate(100, 0, 0)
		}
	}

	if err != nil {
		return time.Time{}
	}
	return t
}
