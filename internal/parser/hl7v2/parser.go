// Package hl7v2 provides parsing of HL7 v2.x messages into canonical semantic events.
package hl7v2

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/crb2nu/fi-fhir/pkg/events"
	"github.com/crb2nu/fi-fhir/pkg/profile"
	"github.com/crb2nu/fi-fhir/pkg/validate"
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

	// ORM - Order Message
	MsgORM_O01 = "ORM^O01" // Order message

	// SIU - Scheduling Information Unsolicited
	MsgSIU_S12 = "SIU^S12" // Notification of new appointment booking
	MsgSIU_S13 = "SIU^S13" // Notification of appointment rescheduling
	MsgSIU_S14 = "SIU^S14" // Notification of appointment modification
	MsgSIU_S15 = "SIU^S15" // Notification of appointment cancellation
	MsgSIU_S26 = "SIU^S26" // Notification of patient no-show
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
	default:
		return nil, fmt.Errorf("unsupported message type: %s", msg.Type)
	}
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
