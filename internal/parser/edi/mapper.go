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

// Map270ToEvents converts a parsed 270 transaction to EligibilityInquiryEvent(s)
func Map270ToEvents(tx *Transaction, source string) ([]*events.EligibilityInquiryEvent, error) {
	loops := Parse270Loops(tx)
	var results []*events.EligibilityInquiryEvent

	// Process each information source (payer)
	for _, infoSource := range loops.InformationSources {
		payer := mapEntity270ToProvider(infoSource.SourceInfo)

		// Process each information receiver (provider)
		for _, receiver := range infoSource.Receivers {
			provider := mapEntity270ToProvider(receiver.ReceiverInfo)

			// Process each subscriber
			for _, sub := range receiver.Subscribers {
				subscriber := mapEntity270ToPatient(sub.SubscriberInfo)

				// Get trace number
				traceNumber := ""
				if len(sub.TRN) > 0 {
					traceNumber = sub.TRN[0].GetElement(2)
				}

				// Build inquiry from EQ segments
				inquiry := buildEligibilityInquiry(sub.EligibilityReq)

				// Check if there are dependents (patient != subscriber)
				if len(sub.Dependents) > 0 {
					for _, dep := range sub.Dependents {
						dependent := mapEntity270ToPatient(dep.DependentInfo)

						// Get dependent trace number if different
						depTrace := traceNumber
						if len(dep.TRN) > 0 {
							depTrace = dep.TRN[0].GetElement(2)
						}

						// Build inquiry from dependent's EQ segments
						depInquiry := inquiry
						if len(dep.EligibilityReq) > 0 {
							depInquiry = buildEligibilityInquiry(dep.EligibilityReq)
						}

						event := &events.EligibilityInquiryEvent{
							EventMeta: events.NewEventMeta(
								events.EventEligibilityInquiry,
								source,
								events.FormatEDI270,
							),
							InformationSource:   payer,
							InformationReceiver: provider,
							Subscriber:          subscriber,
							Dependent:           &dependent,
							Inquiry:             depInquiry,
							TraceNumber:         depTrace,
						}
						event.SourceMessageID = tx.ControlNumber
						results = append(results, event)
					}
				} else {
					// Subscriber is the patient
					event := &events.EligibilityInquiryEvent{
						EventMeta: events.NewEventMeta(
							events.EventEligibilityInquiry,
							source,
							events.FormatEDI270,
						),
						InformationSource:   payer,
						InformationReceiver: provider,
						Subscriber:          subscriber,
						Inquiry:             inquiry,
						TraceNumber:         traceNumber,
					}
					event.SourceMessageID = tx.ControlNumber
					results = append(results, event)
				}
			}
		}
	}

	return results, nil
}

// Map271ToEvents converts a parsed 271 transaction to EligibilityResponseEvent(s)
func Map271ToEvents(tx *Transaction, source string) ([]*events.EligibilityResponseEvent, error) {
	loops := Parse271Loops(tx)
	var results []*events.EligibilityResponseEvent

	// Process each information source (payer)
	for _, infoSource := range loops.InformationSources {
		payer := mapEntity271ToProvider(infoSource.SourceInfo)

		// Check for source-level errors
		sourceErrors := mapAAAToErrors(infoSource.AAA)

		// Process each information receiver (provider)
		for _, receiver := range infoSource.Receivers {
			provider := mapEntity271ToProvider(receiver.ReceiverInfo)

			// Process each subscriber
			for _, sub := range receiver.Subscribers {
				subscriber := mapEntity271ToPatient(sub.SubscriberInfo)

				// Get trace number
				traceNumber := ""
				if len(sub.TRN) > 0 {
					traceNumber = sub.TRN[0].GetElement(2)
				}

				// Check for subscriber-level AAA errors
				subErrors := sourceErrors
				if sub.SubscriberInfo != nil {
					subErrors = append(subErrors, mapAAAToErrors(sub.SubscriberInfo.AAA)...)
				}

				// Determine eligibility status and benefits
				benefits, status := mapBenefits(sub.Benefits)
				if len(subErrors) > 0 && status == events.EligibilityStatusUnknown {
					status = events.EligibilityStatusRejected
				}

				// Extract plan dates from subscriber entity
				planBegin, planEnd := extractPlanDates(sub.SubscriberInfo)

				// Check if there are dependents
				if len(sub.Dependents) > 0 {
					for _, dep := range sub.Dependents {
						dependent := mapEntity271ToPatient(dep.DependentInfo)

						// Get dependent trace number if different
						depTrace := traceNumber
						if len(dep.TRN) > 0 {
							depTrace = dep.TRN[0].GetElement(2)
						}

						// Check for dependent-level AAA errors
						depErrors := subErrors
						if dep.DependentInfo != nil {
							depErrors = append(depErrors, mapAAAToErrors(dep.DependentInfo.AAA)...)
						}

						// Get dependent benefits
						depBenefits, depStatus := benefits, status
						if len(dep.Benefits) > 0 {
							depBenefits, depStatus = mapBenefits(dep.Benefits)
						}
						if len(depErrors) > 0 && depStatus == events.EligibilityStatusUnknown {
							depStatus = events.EligibilityStatusRejected
						}

						// Extract plan dates from dependent entity
						depPlanBegin, depPlanEnd := extractPlanDates(dep.DependentInfo)
						if depPlanBegin.IsZero() {
							depPlanBegin = planBegin
						}
						if depPlanEnd.IsZero() {
							depPlanEnd = planEnd
						}

						event := &events.EligibilityResponseEvent{
							EventMeta: events.NewEventMeta(
								events.EventEligibilityResponse,
								source,
								events.FormatEDI271,
							),
							InformationSource:   payer,
							InformationReceiver: provider,
							Subscriber:          subscriber,
							Dependent:           &dependent,
							Status:              depStatus,
							Benefits:            depBenefits,
							Errors:              depErrors,
							TraceNumber:         depTrace,
							PlanBeginDate:       depPlanBegin,
							PlanEndDate:         depPlanEnd,
						}
						event.SourceMessageID = tx.ControlNumber
						results = append(results, event)
					}
				} else {
					// Subscriber is the patient
					event := &events.EligibilityResponseEvent{
						EventMeta: events.NewEventMeta(
							events.EventEligibilityResponse,
							source,
							events.FormatEDI271,
						),
						InformationSource:   payer,
						InformationReceiver: provider,
						Subscriber:          subscriber,
						Status:              status,
						Benefits:            benefits,
						Errors:              subErrors,
						TraceNumber:         traceNumber,
						PlanBeginDate:       planBegin,
						PlanEndDate:         planEnd,
					}
					event.SourceMessageID = tx.ControlNumber
					results = append(results, event)
				}
			}
		}
	}

	return results, nil
}

// mapEntity270ToProvider maps a 270 entity to Provider
func mapEntity270ToProvider(entity *Loop270Entity) events.Provider {
	if entity == nil || entity.NM1 == nil {
		return events.Provider{}
	}

	nm1 := entity.NM1
	provider := events.Provider{}

	// NM102: Entity Type Qualifier (1=Person, 2=Non-Person Entity)
	entityType := nm1.GetElement(2)

	if entityType == "2" {
		provider.OrganizationName = nm1.GetElement(3)
		provider.ProviderType = "organization"
	} else {
		provider.FamilyName = nm1.GetElement(3)
		provider.GivenName = nm1.GetElement(4)
		provider.MiddleName = nm1.GetElement(5)
		provider.Prefix = nm1.GetElement(6)
		provider.Suffix = nm1.GetElement(7)
		provider.ProviderType = "individual"
	}

	// NM108/NM109: ID Code
	idQual := nm1.GetElement(8)
	idCode := nm1.GetElement(9)

	if idCode != "" {
		var idType string
		switch idQual {
		case "XX":
			provider.NPI = idCode
			idType = "NPI"
		case "PI":
			idType = "PI"
		case "FI":
			idType = "FI" // Federal Tax ID
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

// mapEntity270ToPatient maps a 270 entity to Patient
func mapEntity270ToPatient(entity *Loop270Entity) events.Patient {
	if entity == nil || entity.NM1 == nil {
		return events.Patient{}
	}

	nm1 := entity.NM1
	patient := events.Patient{}

	patient.FamilyName = nm1.GetElement(3)
	patient.GivenName = nm1.GetElement(4)
	patient.MiddleName = nm1.GetElement(5)
	patient.Prefix = nm1.GetElement(6)
	patient.Suffix = nm1.GetElement(7)

	// NM108/NM109: ID Code
	idQual := nm1.GetElement(8)
	idCode := nm1.GetElement(9)

	if idCode != "" {
		var idType string
		switch idQual {
		case "MI":
			idType = "MI"
			patient.MRN = idCode
		default:
			idType = idQual
		}

		patient.Identifiers.Identifiers = append(patient.Identifiers.Identifiers, events.Identifier{
			Type:  idType,
			Value: idCode,
		})
	}

	// Demographics from DMG
	if entity.DMG != nil {
		if dob := entity.DMG.GetElement(2); dob != "" {
			patient.DateOfBirth = parseEDIDate(dob)
		}
		gender := entity.DMG.GetElement(3)
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

// mapEntity271ToProvider maps a 271 entity to Provider
func mapEntity271ToProvider(entity *Loop271Entity) events.Provider {
	if entity == nil || entity.NM1 == nil {
		return events.Provider{}
	}

	nm1 := entity.NM1
	provider := events.Provider{}

	entityType := nm1.GetElement(2)

	if entityType == "2" {
		provider.OrganizationName = nm1.GetElement(3)
		provider.ProviderType = "organization"
	} else {
		provider.FamilyName = nm1.GetElement(3)
		provider.GivenName = nm1.GetElement(4)
		provider.MiddleName = nm1.GetElement(5)
		provider.Prefix = nm1.GetElement(6)
		provider.Suffix = nm1.GetElement(7)
		provider.ProviderType = "individual"
	}

	idQual := nm1.GetElement(8)
	idCode := nm1.GetElement(9)

	if idCode != "" {
		var idType string
		switch idQual {
		case "XX":
			provider.NPI = idCode
			idType = "NPI"
		case "PI":
			idType = "PI"
		case "FI":
			idType = "FI"
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

// mapEntity271ToPatient maps a 271 entity to Patient
func mapEntity271ToPatient(entity *Loop271Entity) events.Patient {
	if entity == nil || entity.NM1 == nil {
		return events.Patient{}
	}

	nm1 := entity.NM1
	patient := events.Patient{}

	patient.FamilyName = nm1.GetElement(3)
	patient.GivenName = nm1.GetElement(4)
	patient.MiddleName = nm1.GetElement(5)
	patient.Prefix = nm1.GetElement(6)
	patient.Suffix = nm1.GetElement(7)

	idQual := nm1.GetElement(8)
	idCode := nm1.GetElement(9)

	if idCode != "" {
		var idType string
		switch idQual {
		case "MI":
			idType = "MI"
			patient.MRN = idCode
		default:
			idType = idQual
		}

		patient.Identifiers.Identifiers = append(patient.Identifiers.Identifiers, events.Identifier{
			Type:  idType,
			Value: idCode,
		})
	}

	// Demographics from DMG
	if entity.DMG != nil {
		if dob := entity.DMG.GetElement(2); dob != "" {
			patient.DateOfBirth = parseEDIDate(dob)
		}
		gender := entity.DMG.GetElement(3)
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

// buildEligibilityInquiry builds an EligibilityInquiry from EQ segments
func buildEligibilityInquiry(eligReqs []*Loop270Eligibility) events.EligibilityInquiry {
	inquiry := events.EligibilityInquiry{}

	for _, req := range eligReqs {
		if req.EQ == nil {
			continue
		}

		// EQ01: Service Type Code
		serviceType := req.EQ.GetElement(1)
		if serviceType != "" {
			inquiry.ServiceTypes = append(inquiry.ServiceTypes, events.EligibilityServiceType(serviceType))
		}

		// Extract dates from DTP segments
		for _, dtp := range req.DTP {
			dtpQual := dtp.GetElement(1)
			dtpDate := dtp.GetElement(3)

			switch dtpQual {
			case "472": // Service date
				inquiry.ServiceDate = parseEDIDate(dtpDate)
			case "291": // Plan date
				// Date range format: CCYYMMDD-CCYYMMDD
				if strings.Contains(dtpDate, "-") {
					parts := strings.Split(dtpDate, "-")
					if len(parts) == 2 {
						inquiry.DateRangeStart = parseEDIDate(parts[0])
						inquiry.DateRangeEnd = parseEDIDate(parts[1])
					}
				} else {
					inquiry.ServiceDate = parseEDIDate(dtpDate)
				}
			}
		}
	}

	return inquiry
}

// mapBenefits converts Loop271Benefit to events.EligibilityBenefit and determines status
func mapBenefits(benefits []*Loop271Benefit) ([]events.EligibilityBenefit, events.EligibilityStatus) {
	var result []events.EligibilityBenefit
	status := events.EligibilityStatusUnknown

	for _, b := range benefits {
		if b.EB == nil {
			continue
		}

		eb := b.EB
		benefit := events.EligibilityBenefit{
			InformationCode:            eb.GetElement(1),
			InformationCodeDescription: mapBenefitInfoCode(eb.GetElement(1)),
			CoverageLevel:              eb.GetElement(2),
			ServiceType:                eb.GetElement(3),
			ServiceTypeDescription:     mapServiceTypeCode(eb.GetElement(3)),
			InsuranceType:              eb.GetElement(4),
			PlanDescription:            eb.GetElement(5),
			TimePeriodQualifier:        eb.GetElement(6),
		}

		// EB07: Monetary amount
		if amt := eb.GetElement(7); amt != "" {
			benefit.Amount, _ = strconv.ParseFloat(amt, 64)
		}

		// EB08: Percent
		if pct := eb.GetElement(8); pct != "" {
			benefit.Percent, _ = strconv.ParseFloat(pct, 64)
		}

		// EB09/EB10: Quantity
		benefit.QuantityQualifier = eb.GetElement(9)
		if qty := eb.GetElement(10); qty != "" {
			benefit.Quantity, _ = strconv.ParseFloat(qty, 64)
		}

		// EB11: Authorization required
		benefit.AuthorizationRequired = eb.GetElement(11) == "Y"

		// EB12: In-network indicator
		benefit.InNetworkIndicator = eb.GetElement(12)

		// Extract dates from DTP segments
		for _, dtp := range b.DTP {
			dtpQual := dtp.GetElement(1)
			dtpDate := dtp.GetElement(3)

			switch dtpQual {
			case "348": // Benefit begin
				benefit.EffectiveDate = parseEDIDate(dtpDate)
			case "349": // Benefit end
				benefit.TerminationDate = parseEDIDate(dtpDate)
			}
		}

		// Extract messages from MSG segments
		for _, msg := range b.MSG {
			msgText := msg.GetElement(1)
			if msgText != "" {
				benefit.Messages = append(benefit.Messages, msgText)
			}
		}

		// Determine overall status from EB01
		infoCode := eb.GetElement(1)
		switch infoCode {
		case "1": // Active Coverage
			if status != events.EligibilityStatusInactive {
				status = events.EligibilityStatusActive
			}
		case "6": // Inactive
			status = events.EligibilityStatusInactive
		case "7": // Inactive - Pending Eligibility Update
			status = events.EligibilityStatusInactive
		case "8": // Inactive - Pending Investigation
			status = events.EligibilityStatusInactive
		}

		result = append(result, benefit)
	}

	return result, status
}

// mapAAAToErrors converts AAA segments to validation errors
func mapAAAToErrors(aaaSegments []*Segment) []events.EligibilityValidationError {
	var errors []events.EligibilityValidationError

	for _, aaa := range aaaSegments {
		if aaa == nil {
			continue
		}

		validError := events.EligibilityValidationError{
			Code:               aaa.GetElement(1),
			RejectReasonCode:   aaa.GetElement(3),
			FollowUpActionCode: aaa.GetElement(4),
			Message:            mapAAARejectReason(aaa.GetElement(3)),
		}

		errors = append(errors, validError)
	}

	return errors
}

// extractPlanDates extracts plan begin/end dates from entity DTP segments
func extractPlanDates(entity *Loop271Entity) (time.Time, time.Time) {
	var planBegin, planEnd time.Time

	if entity == nil {
		return planBegin, planEnd
	}

	for _, dtp := range entity.DTP {
		dtpQual := dtp.GetElement(1)
		dtpDate := dtp.GetElement(3)

		switch dtpQual {
		case "346": // Plan begin
			planBegin = parseEDIDate(dtpDate)
		case "347": // Plan end
			planEnd = parseEDIDate(dtpDate)
		}
	}

	return planBegin, planEnd
}

// mapBenefitInfoCode returns description for EB01 information code
func mapBenefitInfoCode(code string) string {
	descriptions := map[string]string{
		"1":  "Active Coverage",
		"2":  "Active - Full Risk Capitation",
		"3":  "Active - Services Capitated",
		"4":  "Active - Services Capitated to Primary Care Physician",
		"5":  "Active - Pending Investigation",
		"6":  "Inactive",
		"7":  "Inactive - Pending Eligibility Update",
		"8":  "Inactive - Pending Investigation",
		"A":  "Co-Insurance",
		"B":  "Co-Payment",
		"C":  "Deductible",
		"CB": "Coverage Basis",
		"D":  "Benefit Description",
		"E":  "Exclusions",
		"F":  "Limitations",
		"G":  "Out of Pocket (Stop Loss)",
		"H":  "Unlimited",
		"I":  "Non-Covered",
		"J":  "Cost Containment",
		"K":  "Reserve",
		"L":  "Primary Care Provider",
		"M":  "Pre-existing Condition",
		"MC": "Managed Care Coordinator",
		"N":  "Services Restricted to Following Provider",
		"O":  "Not Deemed a Medical Necessity",
		"P":  "Benefit Disclaimer",
		"Q":  "Second Surgical Opinion Required",
		"R":  "Other or Additional Payor",
		"S":  "Prior Year(s) History",
		"T":  "Card(s) Reported Lost/Stolen",
		"U":  "Contact Following Entity for Information",
		"V":  "Cannot Process",
		"W":  "Other Source of Data",
		"X":  "Health Care Facility",
		"Y":  "Spend Down",
	}

	if desc, ok := descriptions[code]; ok {
		return desc
	}
	return "Unknown: " + code
}

// mapServiceTypeCode returns description for service type code
func mapServiceTypeCode(code string) string {
	descriptions := map[string]string{
		"1":  "Medical Care",
		"2":  "Surgical",
		"3":  "Consultation",
		"4":  "Diagnostic X-Ray",
		"5":  "Diagnostic Lab",
		"6":  "Radiation Therapy",
		"7":  "Anesthesia",
		"8":  "Surgical Assistance",
		"12": "Durable Medical Equipment Purchase",
		"30": "Health Benefit Plan Coverage",
		"33": "Chiropractic",
		"35": "Dental Care",
		"37": "Hospice",
		"45": "Immunizations",
		"47": "Hospital - Inpatient",
		"48": "Hospital - Outpatient",
		"50": "Hospital - Emergency Medical",
		"51": "Hospital - Emergency Room",
		"52": "Hospital - Ambulatory Surgical",
		"53": "Hospital - Emergency Accident",
		"54": "Hospice",
		"60": "Home Health Care",
		"62": "Skilled Nursing Care",
		"73": "Well Baby Care",
		"76": "Physician Visit - Sick",
		"86": "Emergency Services",
		"88": "Pharmacy",
		"89": "Free Standing Prescription Drug",
		"90": "Mail Order Prescription Drug",
		"92": "Physician Visit - Sick",
		"93": "Podiatry",
		"96": "Professional (Physician)",
		"98": "Professional (Physician) Visit - Office",
		"AL": "Alcoholism",
		"MH": "Mental Health",
		"UC": "Urgent Care",
	}

	if desc, ok := descriptions[code]; ok {
		return desc
	}
	return ""
}

// mapAAARejectReason returns description for AAA reject reason code
func mapAAARejectReason(code string) string {
	descriptions := map[string]string{
		"04": "Authorized Quantity Exceeded",
		"15": "Required Application Data Missing",
		"33": "Input Errors",
		"35": "Out of Network",
		"41": "Authorization/Access Restrictions",
		"42": "Unable to Respond at Current Time",
		"43": "Invalid/Missing Provider Identification",
		"44": "Invalid/Missing Provider Name",
		"45": "Invalid/Missing Provider Specialty",
		"46": "Invalid/Missing Provider Phone Number",
		"47": "Invalid/Missing Provider State",
		"48": "Invalid/Missing Referring Provider Identification Number",
		"49": "Provider is Not Primary Care Physician",
		"51": "Provider Not on File",
		"52": "Service Dates Not Within Provider Plan Enrollment",
		"56": "Inappropriate Date",
		"57": "Invalid/Missing Date(s) of Service",
		"58": "Invalid/Missing Date-of-Birth",
		"60": "Date of Birth Follows Date(s) of Service",
		"61": "Date of Death Precedes Date(s) of Service",
		"62": "Date of Service Not Within Allowable Inquiry Period",
		"63": "Date of Service in Future",
		"64": "Invalid/Missing Patient ID",
		"65": "Invalid/Missing Patient Name",
		"66": "Invalid/Missing Patient Gender Code",
		"67": "Patient Not Found",
		"68": "Duplicate Patient ID Number",
		"69": "Inconsistent with Patient's Age",
		"70": "Inconsistent with Patient's Gender",
		"71": "Patient Birth Date Does Not Match That for the Patient on the Database",
		"72": "Invalid/Missing Subscriber/Insured ID",
		"73": "Invalid/Missing Subscriber/Insured Name",
		"74": "Invalid/Missing Subscriber/Insured Gender Code",
		"75": "Subscriber/Insured Not Found",
		"76": "Duplicate Subscriber/Insured ID Number",
		"77": "Subscriber Found, Patient Not Found",
		"78": "Subscriber/Insured Not in Group/Plan Identified",
		"79": "Invalid Participant Identification",
		"80": "No Response Received - Transaction Terminated",
		"97": "Invalid or Missing Provider Address",
		"T4": "Payer Name or Identifier Missing",
	}

	if desc, ok := descriptions[code]; ok {
		return desc
	}
	return "Reject Reason: " + code
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

// --- 276/277 Claim Status Mappers ---

// Map276ToEvents converts a parsed 276 transaction to ClaimStatusRequestEvents
func Map276ToEvents(tx *Transaction, source string) ([]*events.ClaimStatusRequestEvent, error) {
	loops := Parse276Loops(tx)
	var results []*events.ClaimStatusRequestEvent

	// Process each information source (payer)
	for _, infoSource := range loops.InformationSources {
		payer := mapEntity276ToProvider(infoSource.SourceInfo)

		// Process each information receiver (provider)
		for _, receiver := range infoSource.Receivers {
			provider := mapEntity276ToProvider(receiver.ReceiverInfo)

			// Process each subscriber
			for _, sub := range receiver.Subscribers {
				subscriber := mapEntity276ToPatient(sub.SubscriberInfo)

				// Process each claim inquiry
				for _, inquiry := range sub.ClaimInquiries {
					// Get trace number
					traceNumber := ""
					if inquiry.TRN != nil {
						traceNumber = inquiry.TRN.GetElement(2)
					}

					// Build inquiry details from REF segments
					claimInquiry := buildClaimStatusInquiry(inquiry)

					// Check if there are dependents
					if len(sub.Dependents) > 0 {
						for _, dep := range sub.Dependents {
							dependent := mapEntity276ToPatient(dep.DependentInfo)

							// Process dependent's claim inquiries
							for _, depInq := range dep.ClaimInquiries {
								depTrace := traceNumber
								if depInq.TRN != nil {
									depTrace = depInq.TRN.GetElement(2)
								}

								depClaimInquiry := buildClaimStatusInquiry(depInq)

								event := &events.ClaimStatusRequestEvent{
									EventMeta: events.NewEventMeta(
										events.EventClaimStatusRequest,
										source,
										events.FormatEDI276,
									),
									Payer:       payer,
									Provider:    provider,
									Subscriber:  subscriber,
									Dependent:   &dependent,
									Inquiry:     depClaimInquiry,
									TraceNumber: depTrace,
								}
								results = append(results, event)
							}
						}
					} else {
						// Subscriber is the patient
						event := &events.ClaimStatusRequestEvent{
							EventMeta: events.NewEventMeta(
								events.EventClaimStatusRequest,
								source,
								events.FormatEDI276,
							),
							Payer:       payer,
							Provider:    provider,
							Subscriber:  subscriber,
							Inquiry:     claimInquiry,
							TraceNumber: traceNumber,
						}
						results = append(results, event)
					}
				}
			}
		}
	}

	return results, nil
}

// Map277ToEvents converts a parsed 277 transaction to ClaimStatusResponseEvents
func Map277ToEvents(tx *Transaction, source string) ([]*events.ClaimStatusResponseEvent, error) {
	loops := Parse277Loops(tx)
	var results []*events.ClaimStatusResponseEvent

	// Process each information source (payer)
	for _, infoSource := range loops.InformationSources {
		payer := mapEntity277ToProvider(infoSource.SourceInfo)

		// Process each information receiver (provider)
		for _, receiver := range infoSource.Receivers {
			provider := mapEntity277ToProvider(receiver.ReceiverInfo)

			// Process each subscriber
			for _, sub := range receiver.Subscribers {
				subscriber := mapEntity277ToPatient(sub.SubscriberInfo)

				// Process each claim status
				for _, status := range sub.ClaimStatuses {
					// Get trace number
					traceNumber := ""
					if status.TRN != nil {
						traceNumber = status.TRN.GetElement(2)
					}

					// Extract claim IDs from REF segments
					claimSubmitterID, payerClaimID, patientControlNum := extractClaimIDs(status.REF)

					// Map STC segments to statuses
					statuses := mapSTCToStatuses(status.STC)

					// Map service lines
					serviceLines := mapServiceLineStatuses(status.ServiceLines)

					// Get total charge from AMT if present
					totalCharge := extractClaimAmount(status.AMT)

					// Check if there are dependents
					if len(sub.Dependents) > 0 {
						for _, dep := range sub.Dependents {
							dependent := mapEntity277ToPatient(dep.DependentInfo)

							// Process dependent's claim statuses
							for _, depStatus := range dep.ClaimStatuses {
								depTrace := traceNumber
								if depStatus.TRN != nil {
									depTrace = depStatus.TRN.GetElement(2)
								}

								depSubmitterID, depPayerID, depPatientNum := extractClaimIDs(depStatus.REF)
								depStatuses := mapSTCToStatuses(depStatus.STC)
								depServiceLines := mapServiceLineStatuses(depStatus.ServiceLines)
								depTotalCharge := extractClaimAmount(depStatus.AMT)

								event := &events.ClaimStatusResponseEvent{
									EventMeta: events.NewEventMeta(
										events.EventClaimStatusResponse,
										source,
										events.FormatEDI277,
									),
									Payer:                  payer,
									Provider:               provider,
									Subscriber:             subscriber,
									Dependent:              &dependent,
									ClaimSubmitterID:       depSubmitterID,
									PayerClaimID:           depPayerID,
									PatientControlNumber:   depPatientNum,
									Statuses:               depStatuses,
									ServiceLines:           depServiceLines,
									TraceNumber:            depTrace,
									TotalClaimChargeAmount: depTotalCharge,
								}
								results = append(results, event)
							}
						}
					} else {
						// Subscriber is the patient
						event := &events.ClaimStatusResponseEvent{
							EventMeta: events.NewEventMeta(
								events.EventClaimStatusResponse,
								source,
								events.FormatEDI277,
							),
							Payer:                  payer,
							Provider:               provider,
							Subscriber:             subscriber,
							ClaimSubmitterID:       claimSubmitterID,
							PayerClaimID:           payerClaimID,
							PatientControlNumber:   patientControlNum,
							Statuses:               statuses,
							ServiceLines:           serviceLines,
							TraceNumber:            traceNumber,
							TotalClaimChargeAmount: totalCharge,
						}
						results = append(results, event)
					}
				}
			}
		}
	}

	return results, nil
}

// mapEntity276ToProvider converts a 276 Loop entity to a Provider
func mapEntity276ToProvider(entity *Loop276Entity) events.Provider {
	if entity == nil || entity.NM1 == nil {
		return events.Provider{}
	}

	nm1 := entity.NM1
	provider := events.Provider{}

	// NM102: Entity Type Qualifier (1=Person, 2=Non-Person Entity)
	entityType := nm1.GetElement(2)

	if entityType == "2" {
		provider.OrganizationName = nm1.GetElement(3)
		provider.ProviderType = "organization"
	} else {
		provider.FamilyName = nm1.GetElement(3)
		provider.GivenName = nm1.GetElement(4)
		provider.MiddleName = nm1.GetElement(5)
		provider.Prefix = nm1.GetElement(6)
		provider.Suffix = nm1.GetElement(7)
		provider.ProviderType = "individual"
	}

	// NM108/NM109: ID Code
	idQual := nm1.GetElement(8)
	idCode := nm1.GetElement(9)

	if idCode != "" {
		var idType string
		switch idQual {
		case "XX":
			provider.NPI = idCode
			idType = "NPI"
		case "PI":
			idType = "PI"
		case "FI":
			idType = "FI"
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

// mapEntity276ToPatient converts a 276 Loop entity to a Patient
func mapEntity276ToPatient(entity *Loop276Entity) events.Patient {
	if entity == nil || entity.NM1 == nil {
		return events.Patient{}
	}

	nm1 := entity.NM1
	patient := events.Patient{}

	patient.FamilyName = nm1.GetElement(3)
	patient.GivenName = nm1.GetElement(4)
	patient.MiddleName = nm1.GetElement(5)
	patient.Prefix = nm1.GetElement(6)
	patient.Suffix = nm1.GetElement(7)

	// NM108/NM109: ID Code
	idQual := nm1.GetElement(8)
	idCode := nm1.GetElement(9)

	if idCode != "" {
		var idType string
		switch idQual {
		case "MI":
			idType = "MI"
			patient.MRN = idCode
		default:
			idType = idQual
		}

		patient.Identifiers.Identifiers = append(patient.Identifiers.Identifiers, events.Identifier{
			Type:  idType,
			Value: idCode,
		})
	}

	// Demographics from DMG
	if entity.DMG != nil {
		if dob := entity.DMG.GetElement(2); dob != "" {
			patient.DateOfBirth = parseEDIDate(dob)
		}
		gender := entity.DMG.GetElement(3)
		switch gender {
		case "M":
			patient.Gender = "male"
		case "F":
			patient.Gender = "female"
		case "U":
			patient.Gender = "unknown"
		default:
			patient.Gender = gender
		}
	}

	return patient
}

// mapEntity277ToProvider converts a 277 Loop entity to a Provider
func mapEntity277ToProvider(entity *Loop277Entity) events.Provider {
	if entity == nil || entity.NM1 == nil {
		return events.Provider{}
	}

	nm1 := entity.NM1
	provider := events.Provider{}

	// NM102: Entity Type Qualifier (1=Person, 2=Non-Person Entity)
	entityType := nm1.GetElement(2)

	if entityType == "2" {
		provider.OrganizationName = nm1.GetElement(3)
		provider.ProviderType = "organization"
	} else {
		provider.FamilyName = nm1.GetElement(3)
		provider.GivenName = nm1.GetElement(4)
		provider.MiddleName = nm1.GetElement(5)
		provider.Prefix = nm1.GetElement(6)
		provider.Suffix = nm1.GetElement(7)
		provider.ProviderType = "individual"
	}

	// NM108/NM109: ID Code
	idQual := nm1.GetElement(8)
	idCode := nm1.GetElement(9)

	if idCode != "" {
		var idType string
		switch idQual {
		case "XX":
			provider.NPI = idCode
			idType = "NPI"
		case "PI":
			idType = "PI"
		case "FI":
			idType = "FI"
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

// mapEntity277ToPatient converts a 277 Loop entity to a Patient
func mapEntity277ToPatient(entity *Loop277Entity) events.Patient {
	if entity == nil || entity.NM1 == nil {
		return events.Patient{}
	}

	nm1 := entity.NM1
	patient := events.Patient{}

	patient.FamilyName = nm1.GetElement(3)
	patient.GivenName = nm1.GetElement(4)
	patient.MiddleName = nm1.GetElement(5)
	patient.Prefix = nm1.GetElement(6)
	patient.Suffix = nm1.GetElement(7)

	// NM108/NM109: ID Code
	idQual := nm1.GetElement(8)
	idCode := nm1.GetElement(9)

	if idCode != "" {
		var idType string
		switch idQual {
		case "MI":
			idType = "MI"
			patient.MRN = idCode
		default:
			idType = idQual
		}

		patient.Identifiers.Identifiers = append(patient.Identifiers.Identifiers, events.Identifier{
			Type:  idType,
			Value: idCode,
		})
	}

	// Demographics from DMG
	if entity.DMG != nil {
		if dob := entity.DMG.GetElement(2); dob != "" {
			patient.DateOfBirth = parseEDIDate(dob)
		}
		gender := entity.DMG.GetElement(3)
		switch gender {
		case "M":
			patient.Gender = "male"
		case "F":
			patient.Gender = "female"
		case "U":
			patient.Gender = "unknown"
		default:
			patient.Gender = gender
		}
	}

	return patient
}

// buildClaimStatusInquiry builds a ClaimStatusInquiry from a 276 inquiry loop
func buildClaimStatusInquiry(inquiry *Loop276Inquiry) events.ClaimStatusInquiry {
	result := events.ClaimStatusInquiry{}

	if inquiry == nil {
		return result
	}

	// Extract claim IDs from REF segments
	for _, ref := range inquiry.REF {
		qual := ref.GetElement(1)
		value := ref.GetElement(2)
		switch qual {
		case "BLT": // Billing Type
			// Not directly mapped
		case "EJ": // Patient Control Number
			result.PatientControlNumber = value
		case "D9": // Claim Number
			result.ClaimSubmitterID = value
		case "1K": // Payer Claim Control Number
			result.PayerClaimID = value
		case "TJ": // Federal Taxpayer ID
			// Not directly mapped
		}
	}

	// Extract amount from AMT segment
	for _, amt := range inquiry.AMT {
		if amt.GetElement(1) == "T3" { // Total Claim Charge Amount
			result.TotalClaimChargeAmount = parseFloat(amt.GetElement(2))
		}
	}

	// Extract dates from DTP segments
	for _, dtp := range inquiry.DTP {
		qual := dtp.GetElement(1)
		dateStr := dtp.GetElement(3)
		switch qual {
		case "472": // Service Date
			// Could be range (RD8) or single date (D8)
			format := dtp.GetElement(2)
			if format == "RD8" && len(dateStr) >= 16 {
				result.ServiceDateStart = parseEDIDate(dateStr[0:8])
				result.ServiceDateEnd = parseEDIDate(dateStr[8:16])
			} else {
				result.ServiceDateStart = parseEDIDate(dateStr)
				result.ServiceDateEnd = result.ServiceDateStart
			}
		}
	}

	return result
}

// extractClaimIDs extracts claim identifiers from REF segments
func extractClaimIDs(refs []*Segment) (submitterID, payerID, patientControl string) {
	for _, ref := range refs {
		qual := ref.GetElement(1)
		value := ref.GetElement(2)
		switch qual {
		case "D9", "BLT": // Claim Number
			submitterID = value
		case "1K": // Payer Claim Control Number
			payerID = value
		case "EJ": // Patient Control Number
			patientControl = value
		}
	}
	return
}

// mapSTCToStatuses converts STC segments to ClaimStatusInfo slice
func mapSTCToStatuses(stcSegments []*Segment) []events.ClaimStatusInfo {
	var statuses []events.ClaimStatusInfo

	for _, stc := range stcSegments {
		// STC01 is a composite: Category:StatusCode:EntityID:CodeListQualifier
		composite := stc.GetElement(1)
		parts := splitComposite(composite)

		status := events.ClaimStatusInfo{}

		// Category code (first component)
		if len(parts) > 0 {
			status.StatusCategoryCode = events.ClaimStatusCategoryCode(parts[0])
			status.StatusCategoryDescription = mapStatusCategoryCode(parts[0])
		}

		// Status code (second component)
		if len(parts) > 1 {
			status.StatusCode = parts[1]
			status.StatusCodeDescription = mapStatusCode(parts[1])
		}

		// Entity identifier (third component)
		if len(parts) > 2 {
			status.EntityIdentifier = parts[2]
		}

		// STC02: Status Date
		if dateStr := stc.GetElement(2); dateStr != "" {
			status.EffectiveDate = parseEDIDate(dateStr)
		}

		// STC03: Action Code
		status.ActionCode = stc.GetElement(3)

		// STC04: Monetary Amount (Total Claim Charge)
		if amt := stc.GetElement(4); amt != "" {
			status.TotalClaimChargeAmount = parseFloat(amt)
		}

		// STC05: Monetary Amount (Payment)
		if amt := stc.GetElement(5); amt != "" {
			status.PaymentAmount = parseFloat(amt)
		}

		// STC06: Payment Date
		if dateStr := stc.GetElement(6); dateStr != "" {
			status.PaymentDate = parseEDIDate(dateStr)
		}

		// STC09: Check Number
		status.CheckNumber = stc.GetElement(9)

		statuses = append(statuses, status)
	}

	return statuses
}

// mapServiceLineStatuses converts service line loops to ClaimServiceLineStatus slice
func mapServiceLineStatuses(lines []*Loop277ServiceLineStatus) []events.ClaimServiceLineStatus {
	var results []events.ClaimServiceLineStatus

	for i, line := range lines {
		if line.SVC == nil {
			continue
		}

		svcLine := events.ClaimServiceLineStatus{
			LineNumber: i + 1,
		}

		// SVC01 is composite: Qualifier:ProcedureCode:Modifiers
		svcComposite := line.SVC.GetElement(1)
		svcParts := splitComposite(svcComposite)

		if len(svcParts) > 0 {
			svcLine.ServiceIDQualifier = svcParts[0]
		}
		if len(svcParts) > 1 {
			svcLine.ProcedureCode = svcParts[1]
		}
		// Modifiers are in positions 2-5 of composite
		for j := 2; j < len(svcParts) && j < 6; j++ {
			if svcParts[j] != "" {
				svcLine.Modifiers = append(svcLine.Modifiers, svcParts[j])
			}
		}

		// SVC02: Line Item Charge Amount
		svcLine.ChargeAmount = parseFloat(line.SVC.GetElement(2))

		// SVC03: Line Item Paid Amount
		svcLine.PaidAmount = parseFloat(line.SVC.GetElement(3))

		// SVC05: Units
		svcLine.Units = parseFloat(line.SVC.GetElement(5))

		// Extract service date from DTP
		for _, dtp := range line.DTP {
			if dtp.GetElement(1) == "472" { // Service Date
				svcLine.ServiceDate = parseEDIDate(dtp.GetElement(3))
				break
			}
		}

		// Map STC segments for this service line
		svcLine.Statuses = mapSTCToStatuses(line.STC)

		results = append(results, svcLine)
	}

	return results
}

// extractClaimAmount extracts the total claim amount from AMT segments
func extractClaimAmount(amts []*Segment) float64 {
	for _, amt := range amts {
		// T3 = Total Claim Charge Amount, T = Total Claim Payment Amount
		qual := amt.GetElement(1)
		if qual == "T3" || qual == "T" {
			return parseFloat(amt.GetElement(2))
		}
	}
	return 0
}

// splitComposite splits a composite element by the component separator (:)
func splitComposite(composite string) []string {
	if composite == "" {
		return nil
	}
	return strings.Split(composite, ":")
}

// mapStatusCategoryCode maps STC01-01 category codes to descriptions
func mapStatusCategoryCode(code string) string {
	descriptions := map[string]string{
		"A0": "Acknowledgement/Receipt - The claim/encounter has been received",
		"A1": "Pending - The claim/encounter is pending further review",
		"A2": "Finalized - The claim/encounter processing is complete",
		"A3": "Request for additional information",
		"A4": "Adjudicated - The claim/encounter has been adjudicated",
		"A5": "Denied - The claim/encounter has been denied",
		"A6": "Partial payment - Partial payment has been issued",
		"A7": "Paid in full - Full payment has been issued",
		"A8": "Rejected - The claim/encounter has been rejected",
		"DR": "Data Reporting Acknowledgement",
		"E0": "Response not possible - error on submitted request data",
		"F0": "Recovery - Recoupment or recovery in progress",
		"P0": "Prior Authorization/Referral",
		"R0": "Requests for additional information",
	}

	if desc, ok := descriptions[code]; ok {
		return desc
	}
	return "Status Category: " + code
}

// mapStatusCode maps common STC01-02 status codes to descriptions
// Note: There are hundreds of status codes in Code Source 508
func mapStatusCode(code string) string {
	// Common status codes - not exhaustive
	descriptions := map[string]string{
		"0":   "Cannot provide further status electronically",
		"1":   "For more detailed information, see remittance advice",
		"2":   "More detailed information in letter",
		"3":   "Claim has been adjudicated and is awaiting payment cycle",
		"4":   "Claim has been paid",
		"5":   "Claim has been denied",
		"6":   "Claim is pending review",
		"12":  "One or more service lines have been denied",
		"15":  "Claim/Service lacks information needed for adjudication",
		"16":  "Claim/service has been suspended",
		"17":  "Claim has been forwarded to entity",
		"18":  "Entity received claim/service but final status not available",
		"19":  "Entity acknowledges receipt of claim/encounter",
		"20":  "Entity accepts responsibility for claim/encounter",
		"21":  "Entity cannot process - not authorized",
		"22":  "Entity cannot process - health care provider not authorized",
		"23":  "Entity cannot process - patient not eligible",
		"24":  "Entity cannot process - service not covered",
		"25":  "Entity cannot process - data validity problem",
		"29":  "Charge/Units exceeded maximum allowed",
		"33":  "Input errors",
		"35":  "Out of network",
		"36":  "Balance due from patient",
		"37":  "Final payment on this claim/service",
		"41":  "Authorization/Access Restrictions",
		"42":  "Unable to respond at current time",
		"65":  "Claim has been denied based on review criteria",
		"100": "Claim is waiting for premium payment",
		"101": "Claim/service pended - additional information needed",
		"102": "Claim/service pended - missing information",
		"103": "Claim is missing required documentation",
		"200": "Duplicate claim/service",
		"201": "Primary claim not received",
		"202": "Payer/processor denied claim/service",
		"203": "Patient ineligible on date of service",
		"204": "Service not covered in plan",
		"277": "Awaiting response from COB payer",
		"278": "Prior authorization required",
		"500": "Claim/service accepted",
		"501": "Claim/service forwarded for review",
	}

	if desc, ok := descriptions[code]; ok {
		return desc
	}
	return "Status: " + code
}

// parseFloat parses a string to float64, returning 0 on error
func parseFloat(s string) float64 {
	if s == "" {
		return 0
	}
	f, _ := strconv.ParseFloat(s, 64)
	return f
}
