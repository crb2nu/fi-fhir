// Package hl7v2 provides parsing of HL7 v2.x messages into canonical semantic events.
package hl7v2

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/profile"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/validate"
)

// Common HL7v2 message types
const (
	// ADT - Admit, Discharge, Transfer
	MsgADT_A01 = "ADT^A01" // Admit
	MsgADT_A02 = "ADT^A02" // Transfer
	MsgADT_A03 = "ADT^A03" // Discharge
	MsgADT_A04 = "ADT^A04" // Register (outpatient)
	MsgADT_A08 = "ADT^A08" // Update patient info
	MsgADT_A11 = "ADT^A11" // Cancel admit
	MsgADT_A13 = "ADT^A13" // Cancel discharge
	MsgADT_A40 = "ADT^A40" // Merge patient

	// ORU - Observation Result
	MsgORU_R01 = "ORU^R01" // Unsolicited observation result

	// Pharmacy
	MsgRDE_O11 = "RDE^O11" // Pharmacy/treatment encoded order

	// Immunizations
	MsgVXU_V04 = "VXU^V04" // Unsolicited vaccination record update

	// ORM - Order Message
	MsgORM_O01 = "ORM^O01" // Order message

	// SIU - Scheduling Information Unsolicited
	MsgSIU_S12 = "SIU^S12" // Notification of new appointment booking
	MsgSIU_S13 = "SIU^S13" // Notification of appointment rescheduling
	MsgSIU_S14 = "SIU^S14" // Notification of appointment modification
	MsgSIU_S15 = "SIU^S15" // Notification of appointment cancellation
	MsgSIU_S26 = "SIU^S26" // Notification of patient no-show

	// MDM - Medical Document Management
	MsgMDM_T01 = "MDM^T01" // Original document notification
	MsgMDM_T02 = "MDM^T02" // Original document notification and content
	MsgMDM_T03 = "MDM^T03" // Document status change notification
	MsgMDM_T04 = "MDM^T04" // Document status change notification and content
	MsgMDM_T05 = "MDM^T05" // Document addendum notification
	MsgMDM_T06 = "MDM^T06" // Document addendum notification and content
	MsgMDM_T08 = "MDM^T08" // Document edit notification
	MsgMDM_T09 = "MDM^T09" // Document edit notification and content
	MsgMDM_T10 = "MDM^T10" // Document replacement notification
	MsgMDM_T11 = "MDM^T11" // Document replacement notification and content

	// DFT - Detail Financial Transaction
	MsgDFT_P03 = "DFT^P03" // Post detail financial transaction
	MsgDFT_P11 = "DFT^P11" // Post detail financial transaction (new)
)

// Parser parses HL7v2 messages into semantic events.
type Parser struct {
	source   string
	config   ParserConfig
	profile  *profile.SourceProfile
	warnings []events.ParseWarning
}

// ParseResult contains the parsed event along with any warnings.
type ParseResult struct {
	// Event is the parsed semantic event
	Event interface{} `json:"event"`

	// Warnings contains non-fatal issues encountered during parsing
	Warnings []events.ParseWarning `json:"warnings,omitempty"`

	// ProfileID identifies which Source Profile was used
	ProfileID string `json:"profile_id,omitempty"`
}

// ParserConfig contains configuration for the HL7v2 parser.
type ParserConfig struct {
	// DefaultTimezone for parsing dates without timezone info
	DefaultTimezone *time.Location

	// ExtractZSegments controls whether Z-segments are extracted
	ExtractZSegments bool

	// ZSegmentMapping maps Z-segment fields to canonical fields
	ZSegmentMapping map[string]string

	// StrictValidation enables strict message validation
	StrictValidation bool
}

// NewParser creates a new HL7v2 parser with the given source identifier.
func NewParser(source string, config ParserConfig) *Parser {
	if config.DefaultTimezone == nil {
		config.DefaultTimezone = time.UTC
	}
	return &Parser{
		source:  source,
		config:  config,
		profile: profile.Default(),
	}
}

// SetProfile sets the Source Profile for profile-driven parsing.
// The profile controls tolerance, event classification, and identifier handling.
func (p *Parser) SetProfile(prof *profile.SourceProfile) {
	if prof != nil {
		p.profile = prof
	}
}

// addWarning records a parse warning.
func (p *Parser) addWarning(phase, code, message, path string) {
	p.warnings = append(p.warnings, events.ParseWarning{
		Phase:    phase,
		Code:     code,
		Message:  message,
		Path:     path,
		Severity: "warning",
	})
}

// resetWarnings clears warnings for a new parse operation.
func (p *Parser) resetWarnings() {
	p.warnings = nil
}

// Message represents a parsed HL7v2 message.
type Message struct {
	Raw        string
	Segments   []Segment
	Delimiters Delimiters // Parsed delimiters from MSH-1 and MSH-2
	Type       string     // MSH-9 (e.g., "ADT^A01")
	ControlID  string     // MSH-10
	Version    string     // MSH-12
}

// Segment represents an HL7v2 segment.
type Segment struct {
	ID     string   // Segment identifier (e.g., "MSH", "PID", "PV1")
	Fields []string // Fields (0-indexed, field 0 is segment ID)
}

// Parse parses an HL7v2 message string and returns a semantic event.
// For access to parse warnings, use ParseWithResult instead.
func (p *Parser) Parse(raw string) (interface{}, error) {
	result, err := p.ParseWithResult(raw)
	if err != nil {
		return nil, err
	}
	return result.Event, nil
}

// ParseWithResult parses an HL7v2 message and returns the event with warnings.
// This is the preferred method when you need access to parse warnings.
func (p *Parser) ParseWithResult(raw string) (*ParseResult, error) {
	p.resetWarnings()

	msg, err := p.parseRaw(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse HL7v2 message: %w", err)
	}

	event, err := p.toSemanticEvent(msg)
	if err != nil {
		return nil, err
	}

	// Copy warnings into the event's metadata
	p.setEventWarnings(event)

	profileID := ""
	if p.profile != nil {
		profileID = p.profile.ID
	}

	return &ParseResult{
		Event:     event,
		Warnings:  p.warnings,
		ProfileID: profileID,
	}, nil
}

// setEventWarnings copies accumulated warnings and profile ID into the event's EventMeta.
func (p *Parser) setEventWarnings(event interface{}) {
	// Get the EventMeta from the event and set warnings + profile ID
	switch e := event.(type) {
	case *events.PatientAdmitEvent:
		if len(p.warnings) > 0 {
			e.ParseWarnings = p.warnings
		}
		if p.profile != nil {
			e.SourceProfileID = p.profile.ID
		}
	case *events.PatientDischargeEvent:
		if len(p.warnings) > 0 {
			e.ParseWarnings = p.warnings
		}
		if p.profile != nil {
			e.SourceProfileID = p.profile.ID
		}
	case *events.LabResultEvent:
		if len(p.warnings) > 0 {
			e.ParseWarnings = p.warnings
		}
		if p.profile != nil {
			e.SourceProfileID = p.profile.ID
		}
	case *events.AppointmentEvent:
		if len(p.warnings) > 0 {
			e.ParseWarnings = p.warnings
		}
		if p.profile != nil {
			e.SourceProfileID = p.profile.ID
		}
	case *events.ImmunizationEvent:
		if len(p.warnings) > 0 {
			e.ParseWarnings = p.warnings
		}
		if p.profile != nil {
			e.SourceProfileID = p.profile.ID
		}
	case *events.MedicationRequestEvent:
		if len(p.warnings) > 0 {
			e.ParseWarnings = p.warnings
		}
		if p.profile != nil {
			e.SourceProfileID = p.profile.ID
		}
	case *events.DocumentEvent:
		if len(p.warnings) > 0 {
			e.ParseWarnings = p.warnings
		}
		if p.profile != nil {
			e.SourceProfileID = p.profile.ID
		}
	case *events.FinancialTransactionEvent:
		if len(p.warnings) > 0 {
			e.ParseWarnings = p.warnings
		}
		if p.profile != nil {
			e.SourceProfileID = p.profile.ID
		}
	}
}

// parseRaw parses the raw HL7v2 string into a structured Message.
func (p *Parser) parseRaw(raw string) (*Message, error) {
	// Normalize line endings
	raw = strings.ReplaceAll(raw, "\r\n", "\r")
	raw = strings.ReplaceAll(raw, "\n", "\r")

	lines := strings.Split(strings.TrimSpace(raw), "\r")
	if len(lines) == 0 {
		return nil, errors.New("empty message")
	}

	// Parse MSH first to get delimiters
	if !strings.HasPrefix(lines[0], "MSH") {
		return nil, errors.New("message must start with MSH segment")
	}

	// Default HL7v2 delimiters
	fieldSep := "|"
	if len(lines[0]) > 3 {
		fieldSep = string(lines[0][3])
	}

	// Parse MSH-2 encoding characters (default: ^~\&)
	delimiters := DefaultDelimiters()
	if len(lines[0]) >= 8 {
		encodingChars := lines[0][4:8] // MSH-2 is positions 4-7 (after "MSH|")
		delimiters = Delimiters{
			Field:        fieldSep[0],
			Component:    safeByte(encodingChars, 0, '^'),
			Repetition:   safeByte(encodingChars, 1, '~'),
			Escape:       safeByte(encodingChars, 2, '\\'),
			Subcomponent: safeByte(encodingChars, 3, '&'),
		}
	}

	msg := &Message{
		Raw:        raw,
		Segments:   make([]Segment, 0, len(lines)),
		Delimiters: delimiters,
	}

	for _, line := range lines {
		if line == "" {
			continue
		}

		// Special handling for MSH segment (field separator is at position 3)
		var fields []string
		if strings.HasPrefix(line, "MSH") {
			// MSH-1 is the field separator itself
			fields = append([]string{"MSH", fieldSep}, strings.Split(line[4:], fieldSep)...)
		} else {
			fields = strings.Split(line, fieldSep)
		}

		seg := Segment{
			ID:     fields[0],
			Fields: fields,
		}
		msg.Segments = append(msg.Segments, seg)

		// Extract message metadata from MSH
		// MSH field numbers match array indices (MSH-N -> fields[N])
		// because fields[0]="MSH", fields[1]="|", fields[2]="^~\&", etc.
		if seg.ID == "MSH" {
			if len(fields) > 9 {
				msg.Type = fields[9] // MSH-9 (message type)
			}
			if len(fields) > 10 {
				msg.ControlID = fields[10] // MSH-10 (message control ID)
			}
			if len(fields) > 12 {
				msg.Version = fields[12] // MSH-12 (version ID)
			}
		}
	}

	return msg, nil
}

// toSemanticEvent converts a parsed Message to the appropriate semantic event.
func (p *Parser) toSemanticEvent(msg *Message) (interface{}, error) {
	switch {
	case strings.HasPrefix(msg.Type, "ADT^A01"):
		return p.parseADT_A01(msg)
	case strings.HasPrefix(msg.Type, "ADT^A02"):
		return p.parseADT_A02(msg)
	case strings.HasPrefix(msg.Type, "ADT^A03"):
		return p.parseADT_A03(msg)
	case strings.HasPrefix(msg.Type, "ADT^A04"):
		return p.parseADT_A04(msg)
	case strings.HasPrefix(msg.Type, "ADT^A08"):
		return p.parseADT_A08(msg)
	case strings.HasPrefix(msg.Type, "ORU^R01"):
		return p.parseORU_R01(msg)
	case strings.HasPrefix(msg.Type, "RDE^O11"):
		return p.parseRDE_O11(msg)
	case strings.HasPrefix(msg.Type, "VXU^V04"):
		return p.parseVXU_V04(msg)
	case strings.HasPrefix(msg.Type, "SIU^S12"):
		return p.parseSIU_S12(msg)
	case strings.HasPrefix(msg.Type, "SIU^S13"):
		return p.parseSIU_S13(msg)
	case strings.HasPrefix(msg.Type, "SIU^S14"):
		return p.parseSIU_S14(msg)
	case strings.HasPrefix(msg.Type, "SIU^S15"):
		return p.parseSIU_S15(msg)
	case strings.HasPrefix(msg.Type, "SIU^S26"):
		return p.parseSIU_S26(msg)
	// MDM - Medical Document Management
	case strings.HasPrefix(msg.Type, "MDM^T01"), strings.HasPrefix(msg.Type, "MDM^T02"):
		return p.parseMDM_Original(msg)
	case strings.HasPrefix(msg.Type, "MDM^T03"), strings.HasPrefix(msg.Type, "MDM^T04"):
		return p.parseMDM_StatusChange(msg)
	case strings.HasPrefix(msg.Type, "MDM^T05"), strings.HasPrefix(msg.Type, "MDM^T06"):
		return p.parseMDM_Addendum(msg)
	case strings.HasPrefix(msg.Type, "MDM^T08"), strings.HasPrefix(msg.Type, "MDM^T09"):
		return p.parseMDM_Edit(msg)
	case strings.HasPrefix(msg.Type, "MDM^T10"), strings.HasPrefix(msg.Type, "MDM^T11"):
		return p.parseMDM_Replacement(msg)
	// DFT - Detail Financial Transaction
	case strings.HasPrefix(msg.Type, "DFT^P03"), strings.HasPrefix(msg.Type, "DFT^P11"):
		return p.parseDFT_P03(msg)
	default:
		return nil, fmt.Errorf("unsupported message type: %s", msg.Type)
	}
}

// parseVXU_V04 parses an immunization update message.
func (p *Parser) parseVXU_V04(msg *Message) (*events.ImmunizationEvent, error) {
	patient, err := p.extractPatient(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract patient: %w", err)
	}

	rxa := p.getSegment(msg, "RXA")
	if rxa == nil {
		return nil, fmt.Errorf("RXA segment not found")
	}

	// RXA-3: Date/Time Start of Administration
	adminDate := ""
	if v := p.getField(rxa, 3); v != "" {
		if t, perr := p.parseHL7DateTime(v); perr == nil {
			adminDate = t.Format(time.RFC3339)
		}
	}

	// RXA-5: Administered Code (CE): id^text^coding system
	rxa5 := p.getField(rxa, 5)
	vaccineCode := p.getComponent(rxa5, 0)
	vaccineName := p.getComponentUnescaped(rxa5, 1, msg.Delimiters)

	// RXA-6/7: Administered Amount + Units
	dose := ""
	if amt := p.getField(rxa, 6); amt != "" {
		if unit := p.getField(rxa, 7); unit != "" {
			dose = amt + " " + unit
		} else {
			dose = amt
		}
	}

	// RXA-15: Lot Number
	lotNumber := p.getField(rxa, 15)

	// RXR: Route/Site (optional)
	route := ""
	site := ""
	if rxr := p.getSegment(msg, "RXR"); rxr != nil {
		rxr1 := p.getField(rxr, 1) // route
		route = p.getComponentUnescaped(rxr1, 1, msg.Delimiters)
		if route == "" {
			route = p.getComponentUnescaped(rxr1, 0, msg.Delimiters)
		}

		rxr2 := p.getField(rxr, 2) // site
		site = p.getComponentUnescaped(rxr2, 1, msg.Delimiters)
		if site == "" {
			site = p.getComponentUnescaped(rxr2, 0, msg.Delimiters)
		}
	}

	meta := events.NewEventMeta(events.EventImmunization, p.source, events.FormatHL7v2)
	meta.SourceMessageID = msg.ControlID

	return &events.ImmunizationEvent{
		EventMeta: meta,
		Patient:   &patient,
		Immunization: events.Immunization{
			VaccineCode:  vaccineCode,
			VaccineName:  vaccineName,
			Status:       "completed",
			LotNumber:    lotNumber,
			Site:         site,
			Route:        route,
			DoseQuantity: dose,
		},
		AdministeredDate: adminDate,
	}, nil
}

// parseRDE_O11 parses a medication order message.
func (p *Parser) parseRDE_O11(msg *Message) (*events.MedicationRequestEvent, error) {
	patient, err := p.extractPatient(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract patient: %w", err)
	}

	rxe := p.getSegment(msg, "RXE")
	if rxe == nil {
		return nil, fmt.Errorf("RXE segment not found")
	}

	// RXE-2: Give Code (CE): id^text^coding system
	rxe2 := p.getField(rxe, 2)
	medCode := p.getComponent(rxe2, 0)
	medName := p.getComponentUnescaped(rxe2, 1, msg.Delimiters)
	medSystem := p.getComponent(rxe2, 2)

	// ORC (optional): authored-on and prescriber
	authoredOn := ""
	var prescriber *events.Provider
	if orc := p.getSegment(msg, "ORC"); orc != nil {
		if v := p.getField(orc, 9); v != "" { // ORC-9 Date/Time of Transaction
			if t, perr := p.parseHL7DateTime(v); perr == nil {
				authoredOn = t.Format(time.RFC3339)
			}
		}

		orc12 := p.getField(orc, 12) // ORC-12 Ordering Provider (XCN)
		if orc12 != "" {
			id := p.getComponent(orc12, 0)
			family := p.getComponentUnescaped(orc12, 1, msg.Delimiters)
			given := p.getComponentUnescaped(orc12, 2, msg.Delimiters)
			if id != "" || family != "" || given != "" {
				prescriber = &events.Provider{
					ID:         id,
					FamilyName: family,
					GivenName:  given,
					MiddleName: p.getComponentUnescaped(orc12, 3, msg.Delimiters),
					Suffix:     p.getComponentUnescaped(orc12, 4, msg.Delimiters),
					Prefix:     p.getComponentUnescaped(orc12, 5, msg.Delimiters),
				}
			}
		}
	}

	meta := events.NewEventMeta(events.EventMedicationRequest, p.source, events.FormatHL7v2)
	meta.SourceMessageID = msg.ControlID

	return &events.MedicationRequestEvent{
		EventMeta: meta,
		Patient:   &patient,
		MedicationRequest: events.MedicationRequest{
			Medication: events.Medication{
				Code:       medCode,
				CodeSystem: medSystem,
				Name:       medName,
			},
			Status:     "active",
			Intent:     "order",
			AuthoredOn: authoredOn,
		},
		Prescriber: prescriber,
	}, nil
}

// parseADT_A01 parses an admit message.
func (p *Parser) parseADT_A01(msg *Message) (*events.PatientAdmitEvent, error) {
	patient, err := p.extractPatient(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract patient: %w", err)
	}

	encounter, err := p.extractEncounterTolerant(msg, "PV1")
	if err != nil {
		return nil, fmt.Errorf("failed to extract encounter: %w", err)
	}

	// Use profile-driven event classification based on PV1-2 (Patient Class)
	classifiedEventType := p.profile.GetEventClassification(msg.Type, encounter.Class)
	encounter.ClassifiedEventType = classifiedEventType

	meta := events.NewEventMeta(events.EventPatientAdmit, p.source, events.FormatHL7v2)
	meta.SourceMessageID = msg.ControlID

	return &events.PatientAdmitEvent{
		EventMeta: meta,
		Patient:   patient,
		Encounter: encounter,
	}, nil
}

// parseADT_A02 parses a transfer message.
func (p *Parser) parseADT_A02(msg *Message) (*events.PatientAdmitEvent, error) {
	// Transfer uses similar structure to admit
	patient, err := p.extractPatient(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract patient: %w", err)
	}

	encounter, err := p.extractEncounterTolerant(msg, "PV1")
	if err != nil {
		return nil, fmt.Errorf("failed to extract encounter: %w", err)
	}

	meta := events.NewEventMeta(events.EventPatientTransfer, p.source, events.FormatHL7v2)
	meta.SourceMessageID = msg.ControlID

	return &events.PatientAdmitEvent{
		EventMeta: meta,
		Patient:   patient,
		Encounter: encounter,
	}, nil
}

// parseADT_A03 parses a discharge message.
func (p *Parser) parseADT_A03(msg *Message) (*events.PatientDischargeEvent, error) {
	patient, err := p.extractPatient(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract patient: %w", err)
	}

	encounter, err := p.extractEncounterTolerant(msg, "PV1")
	if err != nil {
		return nil, fmt.Errorf("failed to extract encounter: %w", err)
	}

	meta := events.NewEventMeta(events.EventPatientDischarge, p.source, events.FormatHL7v2)
	meta.SourceMessageID = msg.ControlID

	return &events.PatientDischargeEvent{
		EventMeta: meta,
		Patient:   patient,
		Encounter: encounter,
	}, nil
}

// parseADT_A04 parses a registration message (outpatient).
func (p *Parser) parseADT_A04(msg *Message) (*events.PatientAdmitEvent, error) {
	patient, err := p.extractPatient(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract patient: %w", err)
	}

	encounter, err := p.extractEncounterTolerant(msg, "PV1")
	if err != nil {
		return nil, fmt.Errorf("failed to extract encounter: %w", err)
	}

	// A04 is outpatient registration - set default if PV1-2 was missing
	if encounter.Class == "" {
		encounter.Class = "O" // Outpatient
	}

	// Use profile-driven event classification
	classifiedEventType := p.profile.GetEventClassification(msg.Type, encounter.Class)
	encounter.ClassifiedEventType = classifiedEventType

	meta := events.NewEventMeta(events.EventPatientAdmit, p.source, events.FormatHL7v2)
	meta.SourceMessageID = msg.ControlID

	return &events.PatientAdmitEvent{
		EventMeta: meta,
		Patient:   patient,
		Encounter: encounter,
	}, nil
}

// parseADT_A08 parses a patient update message.
func (p *Parser) parseADT_A08(msg *Message) (*events.PatientAdmitEvent, error) {
	patient, err := p.extractPatient(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract patient: %w", err)
	}

	// A08 updates may not have PV1 - always tolerate for this message type
	encounter, _ := p.extractEncounterTolerant(msg, "PV1")

	// Use profile-driven event classification if we have a patient class
	if encounter.Class != "" {
		classifiedEventType := p.profile.GetEventClassification(msg.Type, encounter.Class)
		encounter.ClassifiedEventType = classifiedEventType
	}

	meta := events.NewEventMeta(events.EventPatientUpdate, p.source, events.FormatHL7v2)
	meta.SourceMessageID = msg.ControlID

	return &events.PatientAdmitEvent{
		EventMeta: meta,
		Patient:   patient,
		Encounter: encounter,
	}, nil
}

// parseORU_R01 parses an observation result message.
// Supports multiple OBX segments (e.g., CBC panel with WBC, RBC, HGB, etc.).
func (p *Parser) parseORU_R01(msg *Message) (*events.LabResultEvent, error) {
	patient, err := p.extractPatient(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract patient: %w", err)
	}

	// Extract OBR data (order information)
	orderID, orderingProvider, panelName := p.extractOBR(msg)

	// Extract all OBX segments
	obxSegments := p.getAllSegments(msg, "OBX")
	if len(obxSegments) == 0 {
		return nil, errors.New("no OBX segments found in ORU message")
	}

	// Extract all observations
	var observations []events.LabObservation
	var isCritical bool

	for _, obx := range obxSegments {
		test, result, err := p.extractObservationFromSegment(obx, msg.Delimiters)
		if err != nil {
			// Add warning but continue with other OBX segments
			p.addWarning("semantic", "OBX_PARSE_ERROR", err.Error(), obx.ID)
			continue
		}

		// Set order ID and panel name from OBR if available
		if orderID != "" && test.OrderID == "" {
			test.OrderID = orderID
		}
		if panelName != "" && test.Panel == "" {
			test.Panel = panelName
		}

		observations = append(observations, events.LabObservation{
			Test:   test,
			Result: result,
		})

		// Check if any result is critical
		if strings.Contains(strings.ToUpper(result.Interpretation), "CRITICAL") ||
			strings.Contains(strings.ToUpper(result.Interpretation), "PANIC") {
			isCritical = true
		}
	}

	if len(observations) == 0 {
		return nil, errors.New("failed to extract any observations from OBX segments")
	}

	meta := events.NewEventMeta(events.EventLabResult, p.source, events.FormatHL7v2)
	meta.SourceMessageID = msg.ControlID

	// Set primary test/result from first observation for backwards compatibility
	return &events.LabResultEvent{
		EventMeta:        meta,
		Patient:          patient,
		OrderingProvider: orderingProvider,
		Test:             observations[0].Test,
		Result:           observations[0].Result,
		Results:          observations,
		IsCritical:       isCritical,
	}, nil
}

// extractOBR extracts order information from the OBR segment.
// Returns: orderID, orderingProvider, panelName
func (p *Parser) extractOBR(msg *Message) (string, *events.Provider, string) {
	obr := p.getSegment(msg, "OBR")
	if obr == nil {
		return "", nil, ""
	}

	// OBR-2: Placer Order Number (the ordering system's order ID)
	placerOrderNumber := p.getComponent(p.getField(obr, 2), 0)

	// OBR-3: Filler Order Number (the lab's order ID)
	fillerOrderNumber := p.getComponent(p.getField(obr, 3), 0)

	// Use filler order number if available, otherwise placer
	orderID := fillerOrderNumber
	if orderID == "" {
		orderID = placerOrderNumber
	}

	// OBR-4: Universal Service Identifier (test/panel code)
	// Format: code^text^coding system
	obrField4 := p.getField(obr, 4)
	panelName := p.getComponentUnescaped(obrField4, 1, msg.Delimiters)

	// OBR-16: Ordering Provider
	// Format: ID^family^given^middle^suffix^prefix^degree^source table^assigning authority
	obrField16 := p.getField(obr, 16)
	var orderingProvider *events.Provider
	if obrField16 != "" {
		providerID := p.getComponent(obrField16, 0)
		familyName := p.getComponentUnescaped(obrField16, 1, msg.Delimiters)
		givenName := p.getComponentUnescaped(obrField16, 2, msg.Delimiters)

		if providerID != "" || familyName != "" {
			orderingProvider = &events.Provider{
				ID:         providerID,
				FamilyName: familyName,
				GivenName:  givenName,
				MiddleName: p.getComponentUnescaped(obrField16, 3, msg.Delimiters),
				Suffix:     p.getComponentUnescaped(obrField16, 4, msg.Delimiters),
				Prefix:     p.getComponentUnescaped(obrField16, 5, msg.Delimiters),
				Degree:     p.getComponentUnescaped(obrField16, 6, msg.Delimiters),
			}

			// OBR-16.9 is often the assigning authority which may contain NPI
			assigningAuth := p.getComponent(obrField16, 8)
			if strings.Contains(strings.ToUpper(assigningAuth), "NPI") {
				orderingProvider.NPI = providerID
			}

			// Extract identifiers
			if providerID != "" {
				orderingProvider.Identifiers.Identifiers = append(
					orderingProvider.Identifiers.Identifiers,
					events.Identifier{
						Value:    providerID,
						Type:     "ID",
						Assigner: assigningAuth,
					},
				)
			}
		}
	}

	return orderID, orderingProvider, panelName
}

// getAllSegments returns all segments with the given ID.
func (p *Parser) getAllSegments(msg *Message, id string) []*Segment {
	var segments []*Segment
	for i := range msg.Segments {
		if msg.Segments[i].ID == id {
			segments = append(segments, &msg.Segments[i])
		}
	}
	return segments
}

// parseSIU_S12 parses a new appointment booking message.
func (p *Parser) parseSIU_S12(msg *Message) (*events.AppointmentEvent, error) {
	patient, err := p.extractPatient(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract patient: %w", err)
	}

	appt, err := p.extractAppointment(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract appointment: %w", err)
	}

	meta := events.NewEventMeta(events.EventAppointmentScheduled, p.source, events.FormatHL7v2)
	meta.SourceMessageID = msg.ControlID

	return &events.AppointmentEvent{
		EventMeta:   meta,
		Patient:     patient,
		Appointment: appt,
	}, nil
}

// parseSIU_S13 parses an appointment rescheduling message.
func (p *Parser) parseSIU_S13(msg *Message) (*events.AppointmentEvent, error) {
	patient, err := p.extractPatient(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract patient: %w", err)
	}

	appt, err := p.extractAppointment(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract appointment: %w", err)
	}

	// For rescheduling, extract the previous appointment time if available
	// SCH-11 contains the new timing, SCH-27 may contain previous timing
	sch := p.getSegment(msg, "SCH")
	if sch != nil {
		// SCH-27: Filler Status Code for previous appointment
		prevStatus := p.getField(sch, 27)
		if prevStatus != "" {
			appt.PreviousStatus = prevStatus
		}
	}

	meta := events.NewEventMeta(events.EventAppointmentRescheduled, p.source, events.FormatHL7v2)
	meta.SourceMessageID = msg.ControlID

	return &events.AppointmentEvent{
		EventMeta:   meta,
		Patient:     patient,
		Appointment: appt,
	}, nil
}

// parseSIU_S14 parses an appointment modification message.
// Modifications differ from rescheduling in that the time remains the same
// but other details (provider, location, reason) may change.
func (p *Parser) parseSIU_S14(msg *Message) (*events.AppointmentEvent, error) {
	patient, err := p.extractPatient(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract patient: %w", err)
	}

	appt, err := p.extractAppointment(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract appointment: %w", err)
	}

	meta := events.NewEventMeta(events.EventAppointmentModified, p.source, events.FormatHL7v2)
	meta.SourceMessageID = msg.ControlID

	return &events.AppointmentEvent{
		EventMeta:   meta,
		Patient:     patient,
		Appointment: appt,
	}, nil
}

// parseSIU_S15 parses an appointment cancellation message.
func (p *Parser) parseSIU_S15(msg *Message) (*events.AppointmentEvent, error) {
	patient, err := p.extractPatient(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract patient: %w", err)
	}

	appt, err := p.extractAppointment(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract appointment: %w", err)
	}

	// For cancellations, the status should reflect cancelled
	if appt.Status == "" {
		appt.Status = "cancelled"
	}

	// SCH-6 contains the event reason (cancellation reason)
	sch := p.getSegment(msg, "SCH")
	if sch != nil {
		schField6 := p.getField(sch, 6)
		cancelReason := p.getComponent(schField6, 1)
		if cancelReason != "" && appt.CancellationReason == "" {
			appt.CancellationReason = cancelReason
		}
	}

	meta := events.NewEventMeta(events.EventAppointmentCancelled, p.source, events.FormatHL7v2)
	meta.SourceMessageID = msg.ControlID

	return &events.AppointmentEvent{
		EventMeta:   meta,
		Patient:     patient,
		Appointment: appt,
	}, nil
}

// parseSIU_S26 parses a patient no-show notification message.
func (p *Parser) parseSIU_S26(msg *Message) (*events.AppointmentEvent, error) {
	patient, err := p.extractPatient(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract patient: %w", err)
	}

	appt, err := p.extractAppointment(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract appointment: %w", err)
	}

	// Mark as no-show
	if appt.Status == "" {
		appt.Status = "noshow"
	}
	appt.NoShow = true

	meta := events.NewEventMeta(events.EventAppointmentNoShow, p.source, events.FormatHL7v2)
	meta.SourceMessageID = msg.ControlID

	return &events.AppointmentEvent{
		EventMeta:   meta,
		Patient:     patient,
		Appointment: appt,
	}, nil
}

// getSegment retrieves the first segment with the given ID.
func (p *Parser) getSegment(msg *Message, id string) *Segment {
	for i := range msg.Segments {
		if msg.Segments[i].ID == id {
			return &msg.Segments[i]
		}
	}
	return nil
}

// getField safely retrieves a field from a segment.
func (p *Parser) getField(seg *Segment, index int) string {
	if seg == nil || index >= len(seg.Fields) {
		return ""
	}
	return seg.Fields[index]
}

// getComponent extracts a component from a field (separated by ^).
func (p *Parser) getComponent(field string, index int) string {
	parts := strings.Split(field, "^")
	if index >= len(parts) {
		return ""
	}
	return parts[index]
}

// getComponentUnescaped extracts a component and decodes HL7v2 escape sequences.
// Use this for text fields (names, addresses, notes) that may contain escapes.
func (p *Parser) getComponentUnescaped(field string, index int, delim Delimiters) string {
	raw := p.getComponent(field, index)
	if raw == "" {
		return ""
	}
	return UnescapeHL7(raw, delim)
}

// extractPatient extracts patient data from PID segment.
func (p *Parser) extractPatient(msg *Message) (events.Patient, error) {
	pid := p.getSegment(msg, "PID")
	if pid == nil {
		return events.Patient{}, errors.New("PID segment not found")
	}

	// PID-3: Patient identifier list (repeating CX field)
	// Format: ID^checkDigit^checkDigitScheme^assigningAuthority^typeCode^assigningFacility
	// Repetitions separated by ~
	pidField3 := p.getField(pid, 3)
	identifierSet := p.extractIdentifiers(pidField3, "PID.3")

	// Get MRN from identifier set (convenience field)
	mrn := identifierSet.GetMRN()

	// PID-5: Patient name (unescape for names with special characters)
	pidField5 := p.getField(pid, 5)
	familyName := p.getComponentUnescaped(pidField5, 0, msg.Delimiters)
	givenName := p.getComponentUnescaped(pidField5, 1, msg.Delimiters)
	middleName := p.getComponentUnescaped(pidField5, 2, msg.Delimiters)
	prefix := p.getComponentUnescaped(pidField5, 4, msg.Delimiters)
	suffix := p.getComponentUnescaped(pidField5, 3, msg.Delimiters)

	// PID-7: Date of birth
	var dob time.Time
	pidField7 := p.getField(pid, 7)
	if pidField7 != "" {
		dob, _ = p.parseHL7Date(pidField7)
	}

	// PID-8: Gender
	gender := p.getField(pid, 8)

	// PID-11: Address (unescape for addresses with special characters)
	pidField11 := p.getField(pid, 11)
	address := events.Address{
		Line1:      p.getComponentUnescaped(pidField11, 0, msg.Delimiters),
		City:       p.getComponentUnescaped(pidField11, 2, msg.Delimiters),
		State:      p.getComponentUnescaped(pidField11, 3, msg.Delimiters),
		PostalCode: p.getComponentUnescaped(pidField11, 4, msg.Delimiters),
		Country:    p.getComponentUnescaped(pidField11, 5, msg.Delimiters),
	}

	// PID-13: Phone
	phone := p.getField(pid, 13)

	patient := events.Patient{
		MRN:         mrn,
		Identifiers: identifierSet,
		FamilyName:  familyName,
		GivenName:   givenName,
		MiddleName:  middleName,
		Prefix:      prefix,
		Suffix:      suffix,
		DateOfBirth: dob,
		Gender:      gender,
		Address:     address,
		Phone:       phone,
	}

	// Extract Z-segment extensions
	patient.Extensions = p.extractZSegmentExtensions(msg)

	return patient, nil
}

// extractIdentifiers parses a repeating CX field (like PID-3) into an IdentifierSet.
// CX format: ID^checkDigit^checkDigitScheme^assigningAuthority^typeCode^assigningFacility
// Repetitions are separated by ~ (repetition separator).
func (p *Parser) extractIdentifiers(field string, fieldPath string) events.IdentifierSet {
	var idSet events.IdentifierSet

	if field == "" {
		return idSet
	}

	// Split by repetition separator
	repetitions := strings.Split(field, "~")

	for i, rep := range repetitions {
		if rep == "" {
			continue
		}

		// Parse CX components
		components := strings.Split(rep, "^")

		id := events.Identifier{
			OriginalValue: safeIndex(components, 0),
			Value:         safeIndex(components, 0), // Will be normalized below
		}

		// CX.4: Assigning authority (often contains subcomponents separated by &)
		assigningAuth := safeIndex(components, 3)
		if assigningAuth != "" {
			// Handle subcomponents: namespace^universalID^universalIDType
			authParts := strings.Split(assigningAuth, "&")
			id.Assigner = safeIndex(authParts, 0)
			if len(authParts) > 1 {
				// Universal ID is often the OID
				id.System = safeIndex(authParts, 1)
			}
		}

		// CX.5: Identifier type code (HL7 Table 0203)
		typeCode := safeIndex(components, 4)
		id.Type = p.mapIdentifierType(typeCode)

		// Map assigning authority to system URI using profile
		if id.Assigner != "" && id.System == "" {
			if mappedSystem := p.profile.GetAssigningAuthoritySystem(id.Assigner); mappedSystem != "" {
				id.System = mappedSystem
			}
		}

		// Validate and normalize based on type
		p.validateAndNormalizeIdentifier(&id, fmt.Sprintf("%s[%d]", fieldPath, i))

		idSet.Identifiers = append(idSet.Identifiers, id)

		// Set primary if this is an MRN and we don't have one yet
		if id.Type == "MR" && idSet.Primary == nil {
			idCopy := id
			idSet.Primary = &idCopy
		}
	}

	return idSet
}

// mapIdentifierType converts HL7 Table 0203 codes to standard type strings.
func (p *Parser) mapIdentifierType(code string) string {
	// HL7 Table 0203 - Identifier Type
	switch strings.ToUpper(code) {
	case "MR", "MRN":
		return "MR" // Medical Record Number
	case "SS", "SSN":
		return "SS" // Social Security Number
	case "DL":
		return "DL" // Driver's License
	case "PI":
		return "PI" // Patient Internal Identifier
	case "PT":
		return "PT" // Patient External Identifier
	case "NPI":
		return "NPI" // National Provider Identifier
	case "MB", "MBI":
		return "MB" // Medicare Beneficiary Identifier
	case "MA":
		return "MA" // Medicaid Number
	case "MCN":
		return "MCN" // Microchip Number
	case "AN":
		return "AN" // Account Number
	case "VN":
		return "VN" // Visit Number
	case "EN":
		return "EN" // Employer Number
	case "EI":
		return "EI" // Employee Number
	case "":
		return "MR" // Default to MRN if no type specified
	default:
		return code // Preserve unknown codes
	}
}

// validateAndNormalizeIdentifier validates and normalizes an identifier based on its type.
func (p *Parser) validateAndNormalizeIdentifier(id *events.Identifier, path string) {
	switch id.Type {
	case "SS":
		// Validate and normalize SSN
		var rejectPatterns []string
		if p.profile.Identifiers != nil && p.profile.Identifiers.Normalization != nil &&
			p.profile.Identifiers.Normalization.SSN != nil {
			rejectPatterns = p.profile.Identifiers.Normalization.SSN.RejectPatterns
		}
		validator := validate.NewSSNValidator(rejectPatterns)
		normalizer := validate.NewSSNNormalizer()

		normalized := normalizer.Normalize(id.Value)
		result := validator.Validate(normalized)

		id.Value = normalized
		valid := result.Valid
		id.IsValid = &valid
		if !result.Valid {
			id.ValidationError = result.Message
			p.addWarning("semantic", result.Code, result.Message, path)
		}

	case "NPI":
		// Validate NPI
		validator := validate.NewNPIValidator()
		result := validator.Validate(id.Value)

		valid := result.Valid
		id.IsValid = &valid
		if !result.Valid {
			id.ValidationError = result.Message
			p.addWarning("semantic", result.Code, result.Message, path)
		}

	case "MB":
		// Validate MBI (Medicare Beneficiary Identifier)
		validator := validate.NewMBIValidator()
		result := validator.Validate(id.Value)

		// Normalize (remove dashes)
		id.Value = strings.ReplaceAll(strings.ReplaceAll(id.Value, "-", ""), " ", "")
		id.Value = strings.ToUpper(id.Value)

		valid := result.Valid
		id.IsValid = &valid
		if !result.Valid {
			id.ValidationError = result.Message
			p.addWarning("semantic", result.Code, result.Message, path)
		}
	}
}

// safeIndex safely gets an element from a slice, returning empty string if out of bounds.
func safeIndex(slice []string, index int) string {
	if index >= 0 && index < len(slice) {
		return slice[index]
	}
	return ""
}

// safeByte returns the byte at index in the string, or defaultVal if out of bounds.
func safeByte(s string, index int, defaultVal byte) byte {
	if index >= 0 && index < len(s) {
		return s[index]
	}
	return defaultVal
}

// extractEncounterTolerant extracts encounter data with profile-driven tolerance.
// If the segment is missing but tolerated by the profile, it returns an empty encounter
// with a warning instead of an error.
func (p *Parser) extractEncounterTolerant(msg *Message, segmentID string) (events.Encounter, error) {
	pv1 := p.getSegment(msg, segmentID)
	if pv1 == nil {
		// Check if this segment is tolerated as missing
		if p.profile.IsMissingSegmentTolerated(segmentID) {
			p.addWarning("semantic", "MISSING_"+segmentID,
				fmt.Sprintf("%s segment not found but tolerated by profile", segmentID),
				segmentID)
			return events.Encounter{}, nil
		}
		return events.Encounter{}, fmt.Errorf("%s segment not found", segmentID)
	}

	return p.extractEncounterFromSegment(pv1)
}

// extractEncounterFromSegment extracts encounter data from a given PV1 segment.
func (p *Parser) extractEncounterFromSegment(pv1 *Segment) (events.Encounter, error) {
	if pv1 == nil {
		return events.Encounter{}, errors.New("nil segment provided")
	}

	// PV1-2: Patient class
	class := p.getField(pv1, 2)

	// PV1-3: Assigned location
	pv1Field3 := p.getField(pv1, 3)
	location := events.Location{
		Unit:     p.getComponent(pv1Field3, 0),
		Room:     p.getComponent(pv1Field3, 1),
		Bed:      p.getComponent(pv1Field3, 2),
		Facility: p.getComponent(pv1Field3, 3),
	}

	// PV1-7: Attending doctor
	pv1Field7 := p.getField(pv1, 7)
	var attending *events.Provider
	if pv1Field7 != "" {
		attending = &events.Provider{
			ID:         p.getComponent(pv1Field7, 0),
			FamilyName: p.getComponent(pv1Field7, 1),
			GivenName:  p.getComponent(pv1Field7, 2),
		}
	}

	// PV1-19: Visit number
	visitNumber := p.getField(pv1, 19)

	// PV1-44: Admit date/time
	var admitTime time.Time
	pv1Field44 := p.getField(pv1, 44)
	if pv1Field44 != "" {
		admitTime, _ = p.parseHL7DateTime(pv1Field44)
	}

	// PV1-45: Discharge date/time
	var dischargeTime time.Time
	pv1Field45 := p.getField(pv1, 45)
	if pv1Field45 != "" {
		dischargeTime, _ = p.parseHL7DateTime(pv1Field45)
	}

	return events.Encounter{
		ID:                visitNumber,
		Class:             class,
		Location:          location,
		AttendingProvider: attending,
		AdmitDateTime:     admitTime,
		DischargeDateTime: dischargeTime,
	}, nil
}

// extractObservationFromSegment extracts lab test and result from a single OBX segment.
// The delimiters parameter is used for unescaping text fields.
func (p *Parser) extractObservationFromSegment(obx *Segment, delim Delimiters) (events.LabTest, events.LabValue, error) {
	if obx == nil {
		return events.LabTest{}, events.LabValue{}, errors.New("nil OBX segment")
	}

	// OBX-3: Observation identifier (CE/CWE data type)
	// Components: identifier^text^name of coding system^alternate identifier^alternate text^name of alternate coding system
	obxField3 := p.getField(obx, 3)
	localCode := p.getComponent(obxField3, 0)
	display := p.getComponentUnescaped(obxField3, 1, delim) // Unescape display text
	codingSystem := p.getComponent(obxField3, 2)
	altCode := p.getComponent(obxField3, 3)
	altDisplay := p.getComponentUnescaped(obxField3, 4, delim) // Unescape alt display
	altSystem := p.getComponent(obxField3, 5)

	// Build CodeableConcept with available codings
	code := events.CodeableConcept{
		Text: display,
	}

	// Primary coding (often LOCAL or first system)
	if localCode != "" {
		primaryCoding := events.Coding{
			Code:    localCode,
			Display: display,
			System:  codingSystem,
		}
		// If the system looks like LOINC, set it properly
		if codingSystem == "LN" || codingSystem == "LOINC" {
			primaryCoding.System = "http://loinc.org"
		}
		code.Coding = append(code.Coding, primaryCoding)
	}

	// Alternate coding (often LOINC when primary is LOCAL)
	if altCode != "" {
		altCoding := events.Coding{
			Code:    altCode,
			Display: altDisplay,
			System:  altSystem,
		}
		if altSystem == "LN" || altSystem == "LOINC" {
			altCoding.System = "http://loinc.org"
		}
		code.Coding = append(code.Coding, altCoding)
	}

	// Extract LOINC code for convenience field
	var loincCode string
	for _, c := range code.Coding {
		if c.System == "http://loinc.org" || c.System == "LN" || c.System == "LOINC" {
			loincCode = c.Code
			break
		}
	}

	test := events.LabTest{
		Code:        code,
		LocalCode:   localCode,
		Description: display,
		LOINCCode:   loincCode,
	}

	// OBX-5: Observation value (unescape for text results)
	value := UnescapeHL7(p.getField(obx, 5), delim)

	// OBX-6: Units (unescape for units with special chars like µ)
	units := UnescapeHL7(p.getField(obx, 6), delim)

	// OBX-7: Reference range (unescape for ranges with special chars)
	refRange := UnescapeHL7(p.getField(obx, 7), delim)

	// OBX-8: Abnormal flags
	interpretation := p.getField(obx, 8)

	// OBX-11: Observation result status
	status := p.getField(obx, 11)

	// OBX-14: Observation date/time
	var obsTime time.Time
	obxField14 := p.getField(obx, 14)
	if obxField14 != "" {
		obsTime, _ = p.parseHL7DateTime(obxField14)
	}

	result := events.LabValue{
		Value:           value,
		Unit:            units,
		ReferenceRange:  refRange,
		Interpretation:  interpretation,
		Status:          status,
		ObservationTime: obsTime,
	}

	return test, result, nil
}

// extractAppointment extracts appointment data from SCH and AIS segments.
func (p *Parser) extractAppointment(msg *Message) (events.Appointment, error) {
	sch := p.getSegment(msg, "SCH")
	if sch == nil {
		return events.Appointment{}, errors.New("SCH segment not found")
	}

	// SCH-1: Placer appointment ID
	apptID := p.getField(sch, 1)

	// SCH-7: Appointment reason
	schField7 := p.getField(sch, 7)
	reason := p.getComponent(schField7, 1)

	// SCH-11: Appointment timing quantity (start/end/duration)
	schField11 := p.getField(sch, 11)
	var startTime, endTime time.Time
	var duration int

	timingParts := strings.Split(schField11, "^")
	if len(timingParts) > 3 {
		startTime, _ = p.parseHL7DateTime(timingParts[3])
	}
	if len(timingParts) > 4 {
		endTime, _ = p.parseHL7DateTime(timingParts[4])
	}

	// SCH-25: Filler status code
	status := p.getField(sch, 25)

	// Try to get location from AIS segment
	ais := p.getSegment(msg, "AIS")
	var location events.Location
	if ais != nil {
		// AIS-3: Universal service identifier (can contain location info)
		aisField3 := p.getField(ais, 3)
		location.Description = p.getComponent(aisField3, 1)
	}

	return events.Appointment{
		ID:        apptID,
		StartTime: startTime,
		EndTime:   endTime,
		Duration:  duration,
		Status:    status,
		Reason:    reason,
		Location:  location,
	}, nil
}

// parseHL7Date parses an HL7 date (YYYYMMDD).
func (p *Parser) parseHL7Date(s string) (time.Time, error) {
	if len(s) < 8 {
		return time.Time{}, fmt.Errorf("invalid date: %s", s)
	}
	return time.ParseInLocation("20060102", s[:8], p.config.DefaultTimezone)
}

// parseHL7DateTime parses an HL7 datetime (YYYYMMDDHHMMSS).
func (p *Parser) parseHL7DateTime(s string) (time.Time, error) {
	// Handle various HL7 datetime formats
	switch {
	case len(s) >= 14:
		return time.ParseInLocation("20060102150405", s[:14], p.config.DefaultTimezone)
	case len(s) >= 12:
		return time.ParseInLocation("200601021504", s[:12], p.config.DefaultTimezone)
	case len(s) >= 8:
		return time.ParseInLocation("20060102", s[:8], p.config.DefaultTimezone)
	default:
		return time.Time{}, fmt.Errorf("invalid datetime: %s", s)
	}
}

// extractZSegmentExtensions extracts Z-segment data based on profile mappings.
// Z-segments are vendor-specific segments (e.g., ZPI, ZPM, ZPD) that contain
// custom data not defined in the HL7v2 standard.
func (p *Parser) extractZSegmentExtensions(msg *Message) map[string]interface{} {
	if p.profile == nil || p.profile.ZSegments == nil {
		return nil
	}

	extensions := make(map[string]interface{})

	// Process each Z-segment found in the message
	for _, seg := range msg.Segments {
		if len(seg.ID) == 0 || seg.ID[0] != 'Z' {
			continue // Skip non-Z segments
		}

		// Check if we have mappings for this segment
		mappings, ok := p.profile.ZSegments.Mappings[seg.ID]
		if !ok {
			// If PreserveRaw is enabled, store the raw segment
			if p.profile.ZSegments.PreserveRaw {
				rawKey := fmt.Sprintf("raw_%s", seg.ID)
				if len(seg.Fields) > 1 {
					extensions[rawKey] = strings.Join(seg.Fields[1:], "|")
				}
			}
			continue
		}

		// Apply field mappings
		for _, mapping := range mappings {
			value := p.getField(&seg, mapping.Field)
			if value == "" {
				continue
			}

			// Convert value based on type
			var converted interface{}
			switch mapping.Type {
			case "boolean":
				converted = value == "Y" || value == "1" || value == "true" || value == "TRUE"
			case "integer":
				var intVal int
				if _, err := fmt.Sscanf(value, "%d", &intVal); err == nil {
					converted = intVal
				} else {
					converted = value // Keep as string if conversion fails
				}
			case "float":
				var floatVal float64
				if _, err := fmt.Sscanf(value, "%f", &floatVal); err == nil {
					converted = floatVal
				} else {
					converted = value
				}
			default: // "string" or unspecified
				converted = value
			}

			extensions[mapping.Target] = converted
		}
	}

	if len(extensions) == 0 {
		return nil
	}
	return extensions
}

// Delimiters holds the HL7v2 message delimiters extracted from MSH-2.
type Delimiters struct {
	Field        byte // | (from MSH-1)
	Component    byte // ^ (first char of MSH-2)
	Repetition   byte // ~ (second char of MSH-2)
	Escape       byte // \ (third char of MSH-2)
	Subcomponent byte // & (fourth char of MSH-2)
}

// DefaultDelimiters returns the standard HL7v2 delimiters.
func DefaultDelimiters() Delimiters {
	return Delimiters{
		Field:        '|',
		Component:    '^',
		Repetition:   '~',
		Escape:       '\\',
		Subcomponent: '&',
	}
}

// UnescapeHL7 decodes HL7v2 escape sequences in a field value.
// Standard escapes: \F\ (field), \S\ (component), \T\ (subcomponent),
// \R\ (repetition), \E\ (escape), \X..\ (hex), \H\ and \N\ (highlighting).
func UnescapeHL7(s string, delim Delimiters) string {
	if !strings.Contains(s, string(delim.Escape)) {
		return s // Fast path: no escapes
	}

	var result strings.Builder
	result.Grow(len(s))

	i := 0
	for i < len(s) {
		if s[i] == delim.Escape && i+2 < len(s) {
			// Look for closing escape
			closeIdx := strings.IndexByte(s[i+1:], delim.Escape)
			if closeIdx == -1 {
				// No closing escape, treat as literal
				result.WriteByte(s[i])
				i++
				continue
			}

			escapeContent := s[i+1 : i+1+closeIdx]
			replaced := true

			switch escapeContent {
			case "F":
				result.WriteByte(delim.Field)
			case "S":
				result.WriteByte(delim.Component)
			case "T":
				result.WriteByte(delim.Subcomponent)
			case "R":
				result.WriteByte(delim.Repetition)
			case "E":
				result.WriteByte(delim.Escape)
			case "H", "N":
				// Highlighting start/end - typically ignored or passed through
				// For now, we skip these (they have no output)
			case ".br":
				// Line break escape (common extension)
				result.WriteString("\n")
			default:
				if len(escapeContent) > 0 && escapeContent[0] == 'X' {
					// Hex escape: \Xnn\ or \Xnnnn\
					hexStr := escapeContent[1:]
					if decoded := decodeHexEscape(hexStr); decoded != "" {
						result.WriteString(decoded)
					} else {
						replaced = false
					}
				} else if len(escapeContent) > 0 && escapeContent[0] == 'C' {
					// Character set escape - skip for now (charset handling)
					// \Cxxyy\ where xx is ISO escape, yy is charset
				} else {
					replaced = false
				}
			}

			if replaced {
				i = i + 2 + closeIdx // Skip past \...\
			} else {
				// Unknown escape sequence, keep as-is
				result.WriteByte(s[i])
				i++
			}
		} else {
			result.WriteByte(s[i])
			i++
		}
	}

	return result.String()
}

// decodeHexEscape decodes a hex escape sequence (without the X prefix).
func decodeHexEscape(hex string) string {
	if len(hex) == 0 || len(hex)%2 != 0 {
		return ""
	}

	var result []byte
	for i := 0; i < len(hex); i += 2 {
		var b byte
		_, err := fmt.Sscanf(hex[i:i+2], "%02X", &b)
		if err != nil {
			return ""
		}
		result = append(result, b)
	}
	return string(result)
}

// ============================================================================
// MDM (Medical Document Management) Parsing
// ============================================================================

// TXAData holds extracted TXA segment data.
type TXAData struct {
	DocumentType           string
	DocumentTitle          string
	Author                 *events.Provider
	UniqueDocumentNumber   string
	ParentDocumentNumber   string
	DocumentStatus         string
	CompletionStatus       string
	OriginationDateTime    time.Time
	TranscriptionDateTime  time.Time
	EditDateTime           time.Time
	AuthenticationDateTime time.Time
}

// parseMDM_Original parses MDM^T01/T02 (original document notification).
func (p *Parser) parseMDM_Original(msg *Message) (*events.DocumentEvent, error) {
	return p.parseMDMCommon(msg, events.EventDocumentOriginal)
}

// parseMDM_StatusChange parses MDM^T03/T04 (document status change).
func (p *Parser) parseMDM_StatusChange(msg *Message) (*events.DocumentEvent, error) {
	return p.parseMDMCommon(msg, events.EventDocumentStatusChange)
}

// parseMDM_Addendum parses MDM^T05/T06 (document addendum).
func (p *Parser) parseMDM_Addendum(msg *Message) (*events.DocumentEvent, error) {
	return p.parseMDMCommon(msg, events.EventDocumentAddendum)
}

// parseMDM_Edit parses MDM^T08/T09 (document edit).
func (p *Parser) parseMDM_Edit(msg *Message) (*events.DocumentEvent, error) {
	return p.parseMDMCommon(msg, events.EventDocumentEdit)
}

// parseMDM_Replacement parses MDM^T10/T11 (document replacement).
func (p *Parser) parseMDM_Replacement(msg *Message) (*events.DocumentEvent, error) {
	return p.parseMDMCommon(msg, events.EventDocumentReplacement)
}

// parseMDMCommon contains shared MDM parsing logic.
func (p *Parser) parseMDMCommon(msg *Message, eventType events.EventType) (*events.DocumentEvent, error) {
	patient, err := p.extractPatient(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract patient: %w", err)
	}

	txa, err := p.extractTXA(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract TXA: %w", err)
	}

	// Extract document content from OBX if present (T02, T04, T06, T09, T11)
	content, contentType, encoding := p.extractDocumentContent(msg)

	// Extract encounter if PV1 present
	encounter, _ := p.extractEncounterTolerant(msg, "PV1")

	meta := events.NewEventMeta(eventType, p.source, events.FormatHL7v2)
	meta.SourceMessageID = msg.ControlID

	event := &events.DocumentEvent{
		EventMeta:                meta,
		Patient:                  &patient,
		DocumentType:             txa.DocumentType,
		Title:                    txa.DocumentTitle,
		Author:                   txa.Author,
		UniqueDocumentNumber:     txa.UniqueDocumentNumber,
		ParentDocumentNumber:     txa.ParentDocumentNumber,
		DocumentStatus:           txa.CompletionStatus, // Use human-readable status
		DocumentCompletionStatus: txa.DocumentStatus,   // Store raw code for reference
		OriginationDateTime:      txa.OriginationDateTime,
		TranscriptionDateTime:    txa.TranscriptionDateTime,
		EditDateTime:             txa.EditDateTime,
		AuthenticationDateTime:   txa.AuthenticationDateTime,
		ContentType:              contentType,
		Content:                  content,
		ContentEncoding:          encoding,
	}

	if encounter.ID != "" {
		event.Encounter = &encounter
	}

	return event, nil
}

// extractTXA extracts data from the TXA (Transcription Document Header) segment.
func (p *Parser) extractTXA(msg *Message) (*TXAData, error) {
	txa := p.getSegment(msg, "TXA")
	if txa == nil {
		return nil, errors.New("TXA segment not found")
	}

	data := &TXAData{}

	// TXA-2: Document Type (CE: code^text^coding system)
	txa2 := p.getField(txa, 2)
	data.DocumentType = p.getComponent(txa2, 0) // Use code (e.g., "HP", "AD")

	// TXA-4: Activity Date/Time (origination)
	if v := p.getField(txa, 4); v != "" {
		data.OriginationDateTime, _ = p.parseHL7DateTime(v)
	}

	// TXA-5: Primary Activity Provider Code (author)
	txa5 := p.getField(txa, 5)
	if txa5 != "" {
		data.Author = p.extractProviderFromXCN(txa5, msg.Delimiters)
	}

	// TXA-6: Transcription Date/Time
	if v := p.getField(txa, 6); v != "" {
		data.TranscriptionDateTime, _ = p.parseHL7DateTime(v)
	}

	// TXA-7: Edit Date/Time
	if v := p.getField(txa, 7); v != "" {
		data.EditDateTime, _ = p.parseHL7DateTime(v)
	}

	// TXA-12: Unique Document Number (required)
	txa12 := p.getField(txa, 12)
	data.UniqueDocumentNumber = p.getComponent(txa12, 0)

	// TXA-13: Parent Document Number (for addendum/replacement)
	txa13 := p.getField(txa, 13)
	data.ParentDocumentNumber = p.getComponent(txa13, 0)

	// TXA-16: Document Title (free text)
	data.DocumentTitle = UnescapeHL7(p.getField(txa, 16), msg.Delimiters)

	// TXA-17: Document Completion Status
	// Values: DI=Dictated, DO=Documented, IP=In Progress, AU=Authenticated, LA=Legally Authenticated
	data.DocumentStatus = p.getField(txa, 17)
	data.CompletionStatus = p.mapDocumentCompletionStatus(data.DocumentStatus)

	// TXA-22: Authentication Date/Time
	if v := p.getField(txa, 22); v != "" {
		data.AuthenticationDateTime, _ = p.parseHL7DateTime(v)
	}

	return data, nil
}

// extractDocumentContent extracts document content from OBX segments.
// Returns content, contentType, and encoding ("base64" or "text").
func (p *Parser) extractDocumentContent(msg *Message) (string, string, string) {
	obxSegments := p.getAllSegments(msg, "OBX")
	if len(obxSegments) == 0 {
		return "", "", ""
	}

	var contents []string
	var contentType string
	var encoding string

	for _, obx := range obxSegments {
		// OBX-2: Value Type (ED=Encapsulated Data, ST=String, TX=Text, FT=Formatted Text)
		valueType := p.getField(obx, 2)

		// OBX-5: Observation Value
		value := p.getField(obx, 5)

		switch valueType {
		case "ED":
			// Encapsulated Data: source^type^encoding^data
			// e.g., "Application^PDF^Base64^<base64 data>"
			parts := strings.Split(value, "^")
			if len(parts) >= 4 {
				contentType = parts[1] // e.g., "PDF", "RTF", "HTML"
				encoding = "base64"
				contents = append(contents, parts[3]) // Base64 encoded data
			}
		case "ST", "TX", "FT":
			// Text data - may contain ~ for line breaks
			contentType = valueType
			encoding = "text"
			// Replace ~ with newlines for text content
			textContent := strings.ReplaceAll(value, "~", "\n")
			contents = append(contents, UnescapeHL7(textContent, msg.Delimiters))
		}
	}

	return strings.Join(contents, "\n"), contentType, encoding
}

// mapDocumentCompletionStatus maps TXA-17 codes to human-readable status.
func (p *Parser) mapDocumentCompletionStatus(code string) string {
	switch code {
	case "DI":
		return "dictated"
	case "DO":
		return "documented"
	case "IP":
		return "in_progress"
	case "AU":
		return "authenticated"
	case "LA":
		return "legally_authenticated"
	case "PA":
		return "pre_authenticated"
	default:
		return code
	}
}

// extractProviderFromXCN extracts provider data from an XCN field.
func (p *Parser) extractProviderFromXCN(field string, delim Delimiters) *events.Provider {
	if field == "" {
		return nil
	}

	id := p.getComponent(field, 0)
	family := p.getComponentUnescaped(field, 1, delim)
	given := p.getComponentUnescaped(field, 2, delim)

	if id == "" && family == "" && given == "" {
		return nil
	}

	return &events.Provider{
		ID:         id,
		FamilyName: family,
		GivenName:  given,
		MiddleName: p.getComponentUnescaped(field, 3, delim),
		Suffix:     p.getComponentUnescaped(field, 4, delim),
		Prefix:     p.getComponentUnescaped(field, 5, delim),
		Degree:     p.getComponentUnescaped(field, 6, delim),
	}
}

// ============================================================================
// DFT (Detail Financial Transaction) Parsing
// ============================================================================

// parseDFT_P03 parses DFT^P03 and DFT^P11 (detail financial transaction).
func (p *Parser) parseDFT_P03(msg *Message) (*events.FinancialTransactionEvent, error) {
	patient, err := p.extractPatient(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to extract patient: %w", err)
	}

	// Extract all FT1 segments
	ft1Segments := p.getAllSegments(msg, "FT1")
	if len(ft1Segments) == 0 {
		return nil, errors.New("no FT1 segments found in DFT message")
	}

	var transactions []events.FinancialTransaction
	var totalAmount float64

	for _, ft1 := range ft1Segments {
		txn, err := p.extractFinancialTransaction(ft1, msg)
		if err != nil {
			p.addWarning("semantic", "FT1_PARSE_ERROR", err.Error(), "FT1")
			continue
		}
		transactions = append(transactions, txn)
		totalAmount += txn.Amount
	}

	if len(transactions) == 0 {
		return nil, errors.New("failed to extract any financial transactions")
	}

	// Extract diagnoses from DG1 segments
	diagnoses := p.extractDiagnoses(msg)

	// Extract procedures from PR1 segments
	procedures := p.extractProcedures(msg)

	// Associate diagnoses and procedures with transactions
	for i := range transactions {
		transactions[i].Diagnoses = diagnoses
		transactions[i].Procedures = procedures
	}

	// Extract encounter if PV1 present
	encounter, _ := p.extractEncounterTolerant(msg, "PV1")

	// Extract insurance info from IN1 segments
	insuranceInfo := p.extractInsuranceInfo(msg)

	// Account number from PID-18
	pid := p.getSegment(msg, "PID")
	accountNumber := ""
	if pid != nil {
		accountNumber = p.getComponent(p.getField(pid, 18), 0)
	}

	meta := events.NewEventMeta(events.EventFinancialTransaction, p.source, events.FormatHL7v2)
	meta.SourceMessageID = msg.ControlID

	event := &events.FinancialTransactionEvent{
		EventMeta:         meta,
		Patient:           patient,
		Transactions:      transactions,
		TotalChargeAmount: totalAmount,
		InsuranceInfo:     insuranceInfo,
		AccountNumber:     accountNumber,
	}

	if encounter.ID != "" {
		event.Encounter = &encounter
	}

	return event, nil
}

// extractFinancialTransaction extracts data from an FT1 segment.
func (p *Parser) extractFinancialTransaction(ft1 *Segment, msg *Message) (events.FinancialTransaction, error) {
	txn := events.FinancialTransaction{}

	// FT1-1: Set ID
	if v := p.getField(ft1, 1); v != "" {
		_, _ = fmt.Sscanf(v, "%d", &txn.SetID)
	}

	// FT1-2: Transaction ID
	txn.TransactionID = p.getField(ft1, 2)

	// FT1-3: Transaction Batch ID
	txn.BatchID = p.getField(ft1, 3)

	// FT1-4: Transaction Date
	if v := p.getField(ft1, 4); v != "" {
		txn.TransactionDate, _ = p.parseHL7DateTime(v)
	}

	// FT1-5: Transaction Posting Date
	if v := p.getField(ft1, 5); v != "" {
		txn.PostingDate, _ = p.parseHL7DateTime(v)
	}

	// FT1-6: Transaction Type (CG=Charge, CR=Credit, PA=Payment, etc.)
	txn.TransactionType = p.getField(ft1, 6)

	// FT1-7: Transaction Code (CE: code^text^system)
	ft1_7 := p.getField(ft1, 7)
	txn.TransactionCode = events.CodeableConcept{
		Coding: []events.Coding{{
			Code:    p.getComponent(ft1_7, 0),
			Display: p.getComponentUnescaped(ft1_7, 1, msg.Delimiters),
			System:  p.getComponent(ft1_7, 2),
		}},
		Text: p.getComponentUnescaped(ft1_7, 1, msg.Delimiters),
	}

	// FT1-10: Transaction Quantity
	if v := p.getField(ft1, 10); v != "" {
		_, _ = fmt.Sscanf(v, "%f", &txn.Quantity)
	}

	// FT1-11: Transaction Amount - Extended
	if v := p.getField(ft1, 11); v != "" {
		_, _ = fmt.Sscanf(v, "%f", &txn.Amount)
	}

	// FT1-12: Transaction Amount - Unit
	if v := p.getField(ft1, 12); v != "" {
		_, _ = fmt.Sscanf(v, "%f", &txn.UnitAmount)
	}

	// FT1-16: Patient Location (PL: unit^room^bed^facility)
	ft1_16 := p.getField(ft1, 16)
	if ft1_16 != "" {
		txn.PatientLocation = &events.Location{
			Unit:     p.getComponent(ft1_16, 0),
			Room:     p.getComponent(ft1_16, 1),
			Bed:      p.getComponent(ft1_16, 2),
			Facility: p.getComponent(ft1_16, 3),
		}
	}

	// FT1-19: Diagnosis Code - FT1 (CE: code^text^system)
	ft1_19 := p.getField(ft1, 19)
	if ft1_19 != "" {
		txn.DiagnosisCodes = append(txn.DiagnosisCodes, events.CodeableConcept{
			Coding: []events.Coding{{
				Code:    p.getComponent(ft1_19, 0),
				Display: p.getComponentUnescaped(ft1_19, 1, msg.Delimiters),
				System:  p.getComponent(ft1_19, 2),
			}},
			Text: p.getComponentUnescaped(ft1_19, 1, msg.Delimiters),
		})
	}

	// FT1-20: Performed By Code (XCN)
	if v := p.getField(ft1, 20); v != "" {
		txn.PerformedBy = p.extractProviderFromXCN(v, msg.Delimiters)
	}

	// FT1-21: Ordered By Code (XCN)
	if v := p.getField(ft1, 21); v != "" {
		txn.OrderedBy = p.extractProviderFromXCN(v, msg.Delimiters)
	}

	// FT1-23: Filler Order Number
	txn.FillerOrderNumber = p.getComponent(p.getField(ft1, 23), 0)

	// FT1-24: Entered By Code (XCN)
	if v := p.getField(ft1, 24); v != "" {
		txn.EnteredBy = p.extractProviderFromXCN(v, msg.Delimiters)
	}

	// FT1-25: Procedure Code (CNE: code^text^system)
	ft1_25 := p.getField(ft1, 25)
	if ft1_25 != "" {
		txn.ProcedureCode = &events.CodeableConcept{
			Coding: []events.Coding{{
				Code:    p.getComponent(ft1_25, 0),
				Display: p.getComponentUnescaped(ft1_25, 1, msg.Delimiters),
				System:  p.getComponent(ft1_25, 2),
			}},
			Text: p.getComponentUnescaped(ft1_25, 1, msg.Delimiters),
		}
	}

	// FT1-26: Procedure Code Modifier (repeating field)
	ft1_26 := p.getField(ft1, 26)
	if ft1_26 != "" {
		modifiers := strings.Split(ft1_26, "~")
		for _, mod := range modifiers {
			if code := p.getComponent(mod, 0); code != "" {
				txn.ProcedureModifiers = append(txn.ProcedureModifiers, code)
			}
		}
	}

	return txn, nil
}

// extractDiagnoses extracts all DG1 segments from the message.
func (p *Parser) extractDiagnoses(msg *Message) []events.Diagnosis {
	dg1Segments := p.getAllSegments(msg, "DG1")
	var diagnoses []events.Diagnosis

	for _, dg1 := range dg1Segments {
		dx := events.Diagnosis{}

		// DG1-1: Set ID
		if v := p.getField(dg1, 1); v != "" {
			_, _ = fmt.Sscanf(v, "%d", &dx.SetID)
		}

		// DG1-2: Diagnosis Coding Method (I9=ICD-9, I10=ICD-10)
		dx.CodingMethod = p.getField(dg1, 2)

		// DG1-3: Diagnosis Code (CE: code^text^system)
		dg1_3 := p.getField(dg1, 3)
		dx.Code = events.CodeableConcept{
			Coding: []events.Coding{{
				Code:    p.getComponent(dg1_3, 0),
				Display: p.getComponentUnescaped(dg1_3, 1, msg.Delimiters),
				System:  p.mapDiagnosisCodingSystem(dx.CodingMethod),
			}},
			Text: p.getComponentUnescaped(dg1_3, 1, msg.Delimiters),
		}

		// DG1-4: Diagnosis Description
		dx.Description = UnescapeHL7(p.getField(dg1, 4), msg.Delimiters)

		// DG1-5: Diagnosis Date/Time
		if v := p.getField(dg1, 5); v != "" {
			dx.DiagnosisDate, _ = p.parseHL7DateTime(v)
		}

		// DG1-6: Diagnosis Type (A=Admitting, W=Working, F=Final)
		dx.DiagnosisType = p.getField(dg1, 6)

		// DG1-15: Diagnosis Priority (1=Primary)
		if v := p.getField(dg1, 15); v == "1" {
			dx.IsPrimary = true
		}

		// DG1-16: Diagnosing Clinician (XCN)
		if v := p.getField(dg1, 16); v != "" {
			dx.DiagnosingClinician = p.extractProviderFromXCN(v, msg.Delimiters)
		}

		diagnoses = append(diagnoses, dx)
	}

	return diagnoses
}

// extractProcedures extracts all PR1 segments from the message.
func (p *Parser) extractProcedures(msg *Message) []events.ProcedureInfo {
	pr1Segments := p.getAllSegments(msg, "PR1")
	var procedures []events.ProcedureInfo

	for _, pr1 := range pr1Segments {
		proc := events.ProcedureInfo{}

		// PR1-1: Set ID
		if v := p.getField(pr1, 1); v != "" {
			_, _ = fmt.Sscanf(v, "%d", &proc.SetID)
		}

		// PR1-2: Procedure Coding Method
		proc.CodingMethod = p.getField(pr1, 2)

		// PR1-3: Procedure Code (CE: code^text^system)
		pr1_3 := p.getField(pr1, 3)
		proc.Code = events.CodeableConcept{
			Coding: []events.Coding{{
				Code:    p.getComponent(pr1_3, 0),
				Display: p.getComponentUnescaped(pr1_3, 1, msg.Delimiters),
				System:  p.mapProcedureCodingSystem(proc.CodingMethod),
			}},
			Text: p.getComponentUnescaped(pr1_3, 1, msg.Delimiters),
		}

		// PR1-4: Procedure Description
		proc.Description = UnescapeHL7(p.getField(pr1, 4), msg.Delimiters)

		// PR1-5: Procedure Date/Time
		if v := p.getField(pr1, 5); v != "" {
			proc.ProcedureDate, _ = p.parseHL7DateTime(v)
		}

		// PR1-6: Procedure Functional Type (A=Anesthesia, P=Procedure, I=Incision)
		proc.FunctionalType = p.getField(pr1, 6)

		// PR1-7: Procedure Minutes
		if v := p.getField(pr1, 7); v != "" {
			_, _ = fmt.Sscanf(v, "%d", &proc.ProcedureMinutes)
		}

		// PR1-8: Anesthesiologist (XCN)
		if v := p.getField(pr1, 8); v != "" {
			proc.Practitioner = p.extractProviderFromXCN(v, msg.Delimiters)
		}

		// PR1-9: Anesthesia Code
		proc.AnesthesiaCode = p.getField(pr1, 9)

		procedures = append(procedures, proc)
	}

	return procedures
}

// extractInsuranceInfo extracts insurance data from IN1 segments.
func (p *Parser) extractInsuranceInfo(msg *Message) []events.InsuranceInfo {
	in1Segments := p.getAllSegments(msg, "IN1")
	var insuranceList []events.InsuranceInfo

	for _, in1 := range in1Segments {
		ins := events.InsuranceInfo{}

		// IN1-1: Set ID
		if v := p.getField(in1, 1); v != "" {
			_, _ = fmt.Sscanf(v, "%d", &ins.SetID)
		}
		ins.CoordinationOrder = ins.SetID

		// IN1-2: Insurance Plan ID (CE)
		ins.PlanID = p.getComponent(p.getField(in1, 2), 0)

		// IN1-3: Insurance Company ID (CX)
		ins.CompanyID = p.getComponent(p.getField(in1, 3), 0)

		// IN1-4: Insurance Company Name (XON)
		ins.CompanyName = p.getComponentUnescaped(p.getField(in1, 4), 0, msg.Delimiters)

		// IN1-8: Group Number
		ins.GroupNumber = p.getField(in1, 8)

		// IN1-9: Group Name (XON)
		ins.GroupName = p.getComponentUnescaped(p.getField(in1, 9), 0, msg.Delimiters)

		// IN1-12: Plan Effective Date
		if v := p.getField(in1, 12); v != "" {
			ins.EffectiveDate, _ = p.parseHL7DateTime(v)
		}

		// IN1-13: Plan Expiration Date
		if v := p.getField(in1, 13); v != "" {
			ins.ExpirationDate, _ = p.parseHL7DateTime(v)
		}

		// IN1-36: Policy Number
		ins.PolicyNumber = p.getField(in1, 36)

		// IN1-49: Insured's ID Number (CX)
		ins.SubscriberID = p.getComponent(p.getField(in1, 49), 0)

		insuranceList = append(insuranceList, ins)
	}

	return insuranceList
}

// mapDiagnosisCodingSystem maps DG1-2 codes to FHIR code system URIs.
func (p *Parser) mapDiagnosisCodingSystem(code string) string {
	switch code {
	case "I9", "ICD9":
		return "http://hl7.org/fhir/sid/icd-9-cm"
	case "I10", "ICD10":
		return "http://hl7.org/fhir/sid/icd-10-cm"
	case "SNM", "SNOMED":
		return "http://snomed.info/sct"
	default:
		return code
	}
}

// mapProcedureCodingSystem maps PR1-2 codes to FHIR code system URIs.
func (p *Parser) mapProcedureCodingSystem(code string) string {
	switch code {
	case "C4", "CPT4", "CPT":
		return "http://www.ama-assn.org/go/cpt"
	case "I9", "ICD9":
		return "http://hl7.org/fhir/sid/icd-9-cm"
	case "I10", "ICD10PCS":
		return "http://hl7.org/fhir/sid/icd-10-pcs"
	case "HCPCS":
		return "https://www.cms.gov/Medicare/Coding/HCPCSReleaseCodeSets"
	default:
		return code
	}
}
