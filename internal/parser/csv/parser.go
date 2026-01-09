// Package csv provides parsing of CSV/flatfile healthcare data into canonical semantic events.
// Unlike HL7v2 which has standard message structures, CSV parsing requires schema inference
// or explicit column mapping to extract meaningful data.
package csv

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/cblevins/fi-fhir/pkg/events"
	"github.com/cblevins/fi-fhir/pkg/profile"
)

// Parser parses CSV/flatfile data into semantic events.
type Parser struct {
	source   string
	config   ParserConfig
	profile  *profile.SourceProfile
	warnings []events.ParseWarning
}

// ParserConfig contains configuration for the CSV parser.
type ParserConfig struct {
	// Delimiter is the field separator (default: comma)
	Delimiter rune

	// HasHeader indicates whether the first row contains column names
	HasHeader bool

	// ColumnMapping maps column indices or names to semantic fields
	ColumnMapping map[string]string

	// DateFormat is the expected date format (default: "2006-01-02")
	DateFormat string

	// DateTimeFormat is the expected datetime format (default: "2006-01-02 15:04:05")
	DateTimeFormat string

	// EventType specifies what type of events to produce
	EventType events.EventType

	// InferSchema enables automatic schema inference
	InferSchema bool
}

// NewParser creates a new CSV parser with the given source identifier.
func NewParser(source string, config ParserConfig) *Parser {
	if config.Delimiter == 0 {
		config.Delimiter = ','
	}
	if config.DateFormat == "" {
		config.DateFormat = "2006-01-02"
	}
	if config.DateTimeFormat == "" {
		config.DateTimeFormat = "2006-01-02 15:04:05"
	}
	return &Parser{
		source: source,
		config: config,
	}
}

// SetProfile sets the Source Profile for profile-driven parsing.
func (p *Parser) SetProfile(prof *profile.SourceProfile) {
	if prof != nil {
		p.profile = prof
	}
}

// ParseResult contains parsed events along with any warnings.
type ParseResult struct {
	Events    []interface{}         `json:"events"`
	Warnings  []events.ParseWarning `json:"warnings,omitempty"`
	Schema    *InferredSchema       `json:"schema,omitempty"`
	ProfileID string                `json:"profile_id,omitempty"`
}

// InferredSchema represents the automatically inferred column types.
type InferredSchema struct {
	Columns []ColumnInfo `json:"columns"`
}

// ColumnInfo describes a single CSV column.
type ColumnInfo struct {
	Index        int        `json:"index"`
	Name         string     `json:"name"`
	InferredType ColumnType `json:"inferred_type"`
	SampleValues []string   `json:"sample_values,omitempty"`
	SemanticHint string     `json:"semantic_hint,omitempty"` // e.g., "patient_mrn", "lab_value"
}

// ColumnType represents the detected data type of a column.
type ColumnType string

const (
	TypeString   ColumnType = "string"
	TypeInteger  ColumnType = "integer"
	TypeFloat    ColumnType = "float"
	TypeDate     ColumnType = "date"
	TypeDateTime ColumnType = "datetime"
	TypeBoolean  ColumnType = "boolean"
	TypeCode     ColumnType = "code"   // Looks like a medical code
	TypeMRN      ColumnType = "mrn"    // Medical record number pattern
	TypeSSN      ColumnType = "ssn"    // Social security number pattern
	TypePhone    ColumnType = "phone"  // Phone number pattern
	TypeEmail    ColumnType = "email"  // Email address
	TypeGender   ColumnType = "gender" // M/F/U/O pattern
)

// Parse parses CSV data from a reader and returns semantic events.
func (p *Parser) Parse(r io.Reader) (*ParseResult, error) {
	p.warnings = nil

	reader := csv.NewReader(r)
	reader.Comma = p.config.Delimiter
	reader.LazyQuotes = true // Handle messy CSV
	reader.TrimLeadingSpace = true

	// Read all records
	records, err := reader.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read CSV: %w", err)
	}

	if len(records) == 0 {
		return nil, errors.New("empty CSV file")
	}

	// Extract headers if present
	var headers []string
	startRow := 0
	if p.config.HasHeader {
		headers = records[0]
		startRow = 1
	}

	// Infer schema if requested
	var schema *InferredSchema
	if p.config.InferSchema {
		schema = p.inferSchema(headers, records[startRow:])
	}

	// Parse records into events
	var parsedEvents []interface{}
	for i, record := range records[startRow:] {
		event, err := p.parseRecord(record, headers, i+startRow)
		if err != nil {
			p.addWarning("parse", "RECORD_ERROR", err.Error(), fmt.Sprintf("row:%d", i+startRow))
			continue
		}
		if event != nil {
			parsedEvents = append(parsedEvents, event)
		}
	}

	profileID := ""
	if p.profile != nil {
		profileID = p.profile.ID
	}

	return &ParseResult{
		Events:    parsedEvents,
		Warnings:  p.warnings,
		Schema:    schema,
		ProfileID: profileID,
	}, nil
}

// parseRecord converts a single CSV record to a semantic event.
func (p *Parser) parseRecord(record []string, headers []string, rowNum int) (interface{}, error) {
	if len(record) == 0 {
		return nil, nil // Skip empty rows
	}

	// Build a map of column values
	values := make(map[string]string)
	for i, val := range record {
		// Use header name if available, otherwise column index
		key := fmt.Sprintf("col_%d", i)
		if i < len(headers) && headers[i] != "" {
			key = strings.ToLower(strings.TrimSpace(headers[i]))
		}
		values[key] = strings.TrimSpace(val)
	}

	// Apply column mapping if configured
	if p.config.ColumnMapping != nil {
		for from, to := range p.config.ColumnMapping {
			if val, ok := values[from]; ok {
				values[to] = val
			}
		}
	}

	// Create event based on configured type
	switch p.config.EventType {
	case events.EventPatientAdmit, events.EventPatientUpdate:
		return p.parsePatientRecord(values, rowNum)
	case events.EventLabResult:
		return p.parseLabRecord(values, rowNum)
	default:
		// Return a generic record if no specific type
		return p.parseGenericRecord(values, rowNum)
	}
}

// parsePatientRecord creates a patient event from CSV values.
func (p *Parser) parsePatientRecord(values map[string]string, rowNum int) (*events.PatientAdmitEvent, error) {
	patient := events.Patient{}

	// Try common column name patterns for patient fields
	patient.MRN = p.findValue(values, "mrn", "medical_record_number", "patient_id", "patientid", "id")
	patient.FamilyName = p.findValue(values, "last_name", "family_name", "lastname", "surname")
	patient.GivenName = p.findValue(values, "first_name", "given_name", "firstname", "forename")
	patient.MiddleName = p.findValue(values, "middle_name", "middlename", "middle_initial")
	patient.Gender = p.normalizeGender(p.findValue(values, "gender", "sex"))

	// Parse date of birth
	dobStr := p.findValue(values, "dob", "date_of_birth", "birthdate", "birth_date")
	if dobStr != "" {
		if dob, err := p.parseDate(dobStr); err == nil {
			patient.DateOfBirth = dob
		} else {
			p.addWarning("parse", "INVALID_DATE", fmt.Sprintf("invalid date of birth: %s", dobStr), fmt.Sprintf("row:%d", rowNum))
		}
	}

	// Address
	patient.Address = events.Address{
		Line1:      p.findValue(values, "address", "address1", "street", "address_line_1"),
		City:       p.findValue(values, "city"),
		State:      p.findValue(values, "state", "province"),
		PostalCode: p.findValue(values, "zip", "zipcode", "postal_code", "postcode"),
		Country:    p.findValue(values, "country"),
	}

	patient.Phone = p.findValue(values, "phone", "phone_number", "telephone")
	patient.Email = p.findValue(values, "email", "email_address")

	// SSN
	ssn := p.findValue(values, "ssn", "social_security", "social_security_number")
	if ssn != "" {
		patient.Identifiers.Identifiers = append(patient.Identifiers.Identifiers, events.Identifier{
			Value: ssn,
			Type:  "SS",
		})
	}

	meta := events.NewEventMeta(p.config.EventType, p.source, events.FormatCSV)
	meta.SourceMessageID = fmt.Sprintf("row:%d", rowNum)

	return &events.PatientAdmitEvent{
		EventMeta: meta,
		Patient:   patient,
	}, nil
}

// parseLabRecord creates a lab result event from CSV values.
func (p *Parser) parseLabRecord(values map[string]string, rowNum int) (*events.LabResultEvent, error) {
	// Extract patient info
	patient := events.Patient{
		MRN:        p.findValue(values, "mrn", "patient_id", "patientid"),
		FamilyName: p.findValue(values, "last_name", "patient_last_name"),
		GivenName:  p.findValue(values, "first_name", "patient_first_name"),
	}

	// Extract test info
	test := events.LabTest{
		LocalCode:   p.findValue(values, "test_code", "code", "lab_code"),
		Description: p.findValue(values, "test_name", "test_description", "description", "test"),
		LOINCCode:   p.findValue(values, "loinc", "loinc_code"),
	}

	if test.LocalCode != "" {
		test.Code = events.CodeableConcept{
			Text: test.Description,
			Coding: []events.Coding{{
				Code:    test.LocalCode,
				Display: test.Description,
				System:  "LOCAL",
			}},
		}
	}

	// Extract result
	result := events.LabValue{
		Value:          p.findValue(values, "result", "value", "result_value"),
		Unit:           p.findValue(values, "unit", "units", "uom"),
		ReferenceRange: p.findValue(values, "reference_range", "ref_range", "normal_range"),
		Interpretation: p.findValue(values, "interpretation", "flag", "abnormal_flag"),
		Status:         p.findValue(values, "status", "result_status"),
	}

	// Parse observation time
	obsTimeStr := p.findValue(values, "collection_date", "collected_date", "specimen_date", "date")
	if obsTimeStr != "" {
		if obsTime, err := p.parseDateTime(obsTimeStr); err == nil {
			result.ObservationTime = obsTime
		}
	}

	meta := events.NewEventMeta(events.EventLabResult, p.source, events.FormatCSV)
	meta.SourceMessageID = fmt.Sprintf("row:%d", rowNum)

	return &events.LabResultEvent{
		EventMeta: meta,
		Patient:   patient,
		Test:      test,
		Result:    result,
	}, nil
}

// parseGenericRecord creates a generic event from CSV values.
func (p *Parser) parseGenericRecord(values map[string]string, rowNum int) (*events.Event, error) {
	meta := events.NewEventMeta("csv_record", p.source, events.FormatCSV)
	meta.SourceMessageID = fmt.Sprintf("row:%d", rowNum)

	// Convert values to JSON-compatible data
	data := make(map[string]interface{})
	for k, v := range values {
		data[k] = v
	}

	return &events.Event{
		EventMeta: meta,
	}, nil
}

// findValue looks up a value using multiple possible column names.
func (p *Parser) findValue(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if val, ok := values[key]; ok && val != "" {
			return val
		}
	}
	return ""
}

// normalizeGender converts various gender representations to standard codes.
func (p *Parser) normalizeGender(gender string) string {
	g := strings.ToUpper(strings.TrimSpace(gender))
	switch g {
	case "M", "MALE", "1":
		return "M"
	case "F", "FEMALE", "2":
		return "F"
	case "O", "OTHER", "3":
		return "O"
	case "U", "UNKNOWN", "9", "":
		return "U"
	default:
		return g
	}
}

// parseDate parses a date string using configured format.
func (p *Parser) parseDate(s string) (time.Time, error) {
	// Try configured format first
	if t, err := time.Parse(p.config.DateFormat, s); err == nil {
		return t, nil
	}

	// Try common formats
	formats := []string{
		"2006-01-02",
		"01/02/2006",
		"01-02-2006",
		"1/2/2006",
		"2006/01/02",
		"20060102",
	}
	for _, fmt := range formats {
		if t, err := time.Parse(fmt, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse date: %s", s)
}

// parseDateTime parses a datetime string using configured format.
func (p *Parser) parseDateTime(s string) (time.Time, error) {
	// Try configured format first
	if t, err := time.Parse(p.config.DateTimeFormat, s); err == nil {
		return t, nil
	}

	// Try common formats
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
		"01/02/2006 15:04:05",
		"01/02/2006 3:04:05 PM",
		"2006-01-02",
	}
	for _, fmt := range formats {
		if t, err := time.Parse(fmt, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unable to parse datetime: %s", s)
}

// inferSchema analyzes CSV data to infer column types.
func (p *Parser) inferSchema(headers []string, records [][]string) *InferredSchema {
	if len(records) == 0 {
		return nil
	}

	numCols := len(records[0])
	schema := &InferredSchema{
		Columns: make([]ColumnInfo, numCols),
	}

	// Collect sample values for each column
	for i := 0; i < numCols; i++ {
		col := ColumnInfo{
			Index: i,
		}
		if i < len(headers) {
			col.Name = headers[i]
		}

		// Collect sample values (up to 10)
		for j := 0; j < len(records) && j < 10; j++ {
			if i < len(records[j]) {
				val := strings.TrimSpace(records[j][i])
				if val != "" {
					col.SampleValues = append(col.SampleValues, val)
				}
			}
		}

		// Infer type from values
		col.InferredType = p.inferColumnType(col.Name, col.SampleValues)
		col.SemanticHint = p.inferSemanticHint(col.Name, col.InferredType)

		schema.Columns[i] = col
	}

	return schema
}

// inferColumnType determines the data type of a column from sample values.
func (p *Parser) inferColumnType(name string, samples []string) ColumnType {
	if len(samples) == 0 {
		return TypeString
	}

	// Check column name hints first
	nameLower := strings.ToLower(name)
	if strings.Contains(nameLower, "date") || strings.Contains(nameLower, "dob") {
		return TypeDate
	}
	if strings.Contains(nameLower, "time") || strings.Contains(nameLower, "datetime") {
		return TypeDateTime
	}

	// Pattern matchers
	mrnPattern := regexp.MustCompile(`^[A-Z]?\d{6,10}$`)
	ssnPattern := regexp.MustCompile(`^\d{3}-?\d{2}-?\d{4}$`)
	phonePattern := regexp.MustCompile(`^[\d\-\(\)\s\.]+$`)
	emailPattern := regexp.MustCompile(`^[^@]+@[^@]+\.[^@]+$`)
	datePattern := regexp.MustCompile(`^\d{4}[-/]\d{1,2}[-/]\d{1,2}$`)
	intPattern := regexp.MustCompile(`^-?\d+$`)
	floatPattern := regexp.MustCompile(`^-?\d+\.?\d*$`)
	genderPattern := regexp.MustCompile(`^[MFOUmfou]$|^(male|female|other|unknown)$`)
	codePattern := regexp.MustCompile(`^[A-Z]\d{2,4}(\.\d+)?$`) // ICD-10, CPT-like

	// Score each type
	scores := make(map[ColumnType]int)
	for _, sample := range samples {
		if ssnPattern.MatchString(sample) {
			scores[TypeSSN]++
		} else if emailPattern.MatchString(sample) {
			scores[TypeEmail]++
		} else if phonePattern.MatchString(sample) && len(sample) >= 7 {
			scores[TypePhone]++
		} else if mrnPattern.MatchString(sample) {
			scores[TypeMRN]++
		} else if genderPattern.MatchString(sample) {
			scores[TypeGender]++
		} else if codePattern.MatchString(sample) {
			scores[TypeCode]++
		} else if datePattern.MatchString(sample) {
			scores[TypeDate]++
		} else if intPattern.MatchString(sample) {
			scores[TypeInteger]++
		} else if floatPattern.MatchString(sample) {
			scores[TypeFloat]++
		}
	}

	// Find highest scoring type
	maxScore := 0
	bestType := TypeString
	// Must match majority of samples (more than half, minimum 2)
	threshold := (len(samples) / 2) + 1
	if threshold < 2 {
		threshold = 2
	}
	for t, score := range scores {
		if score > maxScore && score >= threshold {
			maxScore = score
			bestType = t
		}
	}

	return bestType
}

// inferSemanticHint suggests what healthcare field a column might represent.
func (p *Parser) inferSemanticHint(name string, colType ColumnType) string {
	nameLower := strings.ToLower(name)

	// Direct type-based hints
	switch colType {
	case TypeSSN:
		return "patient_ssn"
	case TypeMRN:
		return "patient_mrn"
	case TypeGender:
		return "patient_gender"
	case TypeEmail:
		return "patient_email"
	case TypePhone:
		return "patient_phone"
	}

	// Name-based hints
	hints := map[string]string{
		"mrn":        "patient_mrn",
		"patient_id": "patient_mrn",
		"first_name": "patient_given_name",
		"last_name":  "patient_family_name",
		"dob":        "patient_dob",
		"birth":      "patient_dob",
		"gender":     "patient_gender",
		"sex":        "patient_gender",
		"address":    "patient_address",
		"city":       "patient_city",
		"state":      "patient_state",
		"zip":        "patient_postal_code",
		"test":       "lab_test_name",
		"result":     "lab_result_value",
		"unit":       "lab_result_unit",
		"loinc":      "lab_loinc_code",
		"diagnosis":  "diagnosis_code",
		"icd":        "diagnosis_icd_code",
		"procedure":  "procedure_code",
		"cpt":        "procedure_cpt_code",
		"provider":   "provider_name",
		"npi":        "provider_npi",
		"encounter":  "encounter_id",
		"visit":      "encounter_id",
		"admission":  "admission_date",
		"discharge":  "discharge_date",
	}

	for pattern, hint := range hints {
		if strings.Contains(nameLower, pattern) {
			return hint
		}
	}

	return ""
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

// ParseString is a convenience method to parse CSV from a string.
func (p *Parser) ParseString(data string) (*ParseResult, error) {
	return p.Parse(strings.NewReader(data))
}
