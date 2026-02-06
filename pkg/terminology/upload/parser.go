// Package upload provides CSV parsing and validation for terminology mapping uploads.
package upload

import (
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

// ParsedRow represents a single validated row from a CSV upload.
type ParsedRow struct {
	RowNumber     int
	SourceSystem  string
	SourceCode    string
	SourceDisplay string
	TargetSystem  string
	TargetCode    string
	TargetDisplay string
	Equivalence   db.MappingEquivalence
	Confidence    *float64
	Comment       string
}

// ParseError describes an error in a specific row/column.
type ParseError struct {
	Row     int    `json:"row"`
	Column  string `json:"column,omitempty"`
	Message string `json:"message"`
}

func (e ParseError) Error() string {
	if e.Column != "" {
		return fmt.Sprintf("row %d, column %s: %s", e.Row, e.Column, e.Message)
	}
	return fmt.Sprintf("row %d: %s", e.Row, e.Message)
}

// ParseResult contains the result of parsing a CSV file.
type ParseResult struct {
	Rows           []ParsedRow
	Errors         []ParseError
	TotalRows      int
	ValidRows      int
	ErrorRows      int
	HeaderColumns  []string
	DetectedFormat string // "standard", "simple", "custom"
}

// ParseOptions configures CSV parsing behavior.
type ParseOptions struct {
	// DefaultSourceSystem is used when source_system column is missing or empty
	DefaultSourceSystem string
	// DefaultTargetSystem is used when target_system column is missing or empty
	DefaultTargetSystem string
	// StrictMode fails on any validation error; otherwise continues with valid rows
	StrictMode bool
	// MaxRows limits the number of rows to parse (0 = unlimited)
	MaxRows int
	// SkipHeaderValidation allows parsing files without standard headers
	SkipHeaderValidation bool
}

// Parser parses CSV files for terminology mappings.
type Parser struct {
	opts ParseOptions
}

// NewParser creates a new CSV parser with the given options.
func NewParser(opts ParseOptions) *Parser {
	return &Parser{opts: opts}
}

// Parse reads and validates a CSV from the given reader.
func (p *Parser) Parse(r io.Reader) (*ParseResult, error) {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1 // Allow variable fields
	reader.LazyQuotes = true    // Handle slightly malformed CSV

	result := &ParseResult{}

	// Read header
	header, err := reader.Read()
	if err != nil {
		if err == io.EOF {
			return nil, fmt.Errorf("empty CSV file")
		}
		return nil, fmt.Errorf("reading header: %w", err)
	}
	result.HeaderColumns = header

	// Map column names to indices
	colIdx := p.mapColumns(header)
	result.DetectedFormat = p.detectFormat(colIdx)

	// Validate required columns
	if err := p.validateColumns(colIdx); err != nil {
		return nil, err
	}

	// Parse rows
	rowNum := 1 // 1-indexed (header is row 0)
	for p.opts.MaxRows == 0 || result.TotalRows < p.opts.MaxRows {
		rowNum++
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			result.Errors = append(result.Errors, ParseError{
				Row:     rowNum,
				Message: fmt.Sprintf("CSV parse error: %v", err),
			})
			result.ErrorRows++
			result.TotalRows++
			continue
		}

		result.TotalRows++

		// Parse and validate row
		parsed, errs := p.parseRow(rowNum, record, colIdx)
		if len(errs) > 0 {
			result.Errors = append(result.Errors, errs...)
			result.ErrorRows++
			// In strict mode, skip the row entirely
			// In non-strict mode, also skip - we don't want partial/invalid data
			continue
		}

		// Add valid row (no errors)
		if parsed != nil {
			result.Rows = append(result.Rows, *parsed)
			result.ValidRows++
		}
	}

	return result, nil
}

// columnMap tracks column indices for standard mapping columns.
type columnMap struct {
	sourceSystem  int
	sourceCode    int
	sourceDisplay int
	targetSystem  int
	targetCode    int
	targetDisplay int
	equivalence   int
	confidence    int
	comment       int
}

// mapColumns creates a column index map from header names.
func (p *Parser) mapColumns(header []string) columnMap {
	cm := columnMap{
		sourceSystem:  -1,
		sourceCode:    -1,
		sourceDisplay: -1,
		targetSystem:  -1,
		targetCode:    -1,
		targetDisplay: -1,
		equivalence:   -1,
		confidence:    -1,
		comment:       -1,
	}

	for i, col := range header {
		// Normalize column name
		name := strings.ToLower(strings.TrimSpace(col))
		name = strings.ReplaceAll(name, " ", "_")
		name = strings.ReplaceAll(name, "-", "_")

		switch name {
		case "source_system", "sourcesystem", "src_system", "from_system":
			cm.sourceSystem = i
		case "source_code", "sourcecode", "src_code", "from_code", "local_code", "localcode":
			cm.sourceCode = i
		case "source_display", "sourcedisplay", "src_display", "from_display", "local_display", "localdisplay", "source_name", "local_name":
			cm.sourceDisplay = i
		case "target_system", "targetsystem", "tgt_system", "to_system", "dest_system":
			cm.targetSystem = i
		case "target_code", "targetcode", "tgt_code", "to_code", "dest_code", "standard_code", "standardcode":
			cm.targetCode = i
		case "target_display", "targetdisplay", "tgt_display", "to_display", "dest_display", "standard_display", "standard_name":
			cm.targetDisplay = i
		case "equivalence", "equiv", "mapping_type", "match_type":
			cm.equivalence = i
		case "confidence", "conf", "score", "match_score":
			cm.confidence = i
		case "comment", "comments", "note", "notes", "description":
			cm.comment = i
		}
	}

	return cm
}

// detectFormat returns the detected CSV format type.
func (p *Parser) detectFormat(cm columnMap) string {
	// Standard format has all columns
	if cm.sourceSystem >= 0 && cm.sourceCode >= 0 && cm.targetSystem >= 0 && cm.targetCode >= 0 {
		return "standard"
	}
	// Simple format has just codes
	if cm.sourceCode >= 0 && cm.targetCode >= 0 {
		return "simple"
	}
	return "custom"
}

// validateColumns checks that required columns are present or have defaults.
func (p *Parser) validateColumns(cm columnMap) error {
	var missing []string

	// source_code is always required
	if cm.sourceCode < 0 {
		missing = append(missing, "source_code")
	}

	// target_code is always required
	if cm.targetCode < 0 {
		missing = append(missing, "target_code")
	}

	// source_system is required unless default provided
	if cm.sourceSystem < 0 && p.opts.DefaultSourceSystem == "" {
		missing = append(missing, "source_system (or provide default)")
	}

	// target_system is required unless default provided
	if cm.targetSystem < 0 && p.opts.DefaultTargetSystem == "" {
		missing = append(missing, "target_system (or provide default)")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required columns: %s", strings.Join(missing, ", "))
	}
	return nil
}

// parseRow parses and validates a single CSV row.
func (p *Parser) parseRow(rowNum int, record []string, cm columnMap) (*ParsedRow, []ParseError) {
	var errs []ParseError

	getField := func(idx int) string {
		if idx < 0 || idx >= len(record) {
			return ""
		}
		return strings.TrimSpace(record[idx])
	}

	row := &ParsedRow{
		RowNumber: rowNum,
	}

	// Source system
	row.SourceSystem = getField(cm.sourceSystem)
	if row.SourceSystem == "" {
		row.SourceSystem = p.opts.DefaultSourceSystem
	}
	if row.SourceSystem == "" {
		errs = append(errs, ParseError{Row: rowNum, Column: "source_system", Message: "missing source_system"})
	} else if !isValidSystemName(row.SourceSystem) {
		errs = append(errs, ParseError{Row: rowNum, Column: "source_system", Message: "invalid source_system format"})
	}

	// Source code
	row.SourceCode = getField(cm.sourceCode)
	if row.SourceCode == "" {
		errs = append(errs, ParseError{Row: rowNum, Column: "source_code", Message: "missing source_code"})
	} else if !isValidCode(row.SourceCode) {
		errs = append(errs, ParseError{Row: rowNum, Column: "source_code", Message: "invalid source_code format"})
	}

	// Source display (optional)
	row.SourceDisplay = getField(cm.sourceDisplay)
	if len(row.SourceDisplay) > 1000 {
		row.SourceDisplay = row.SourceDisplay[:1000] // Truncate long displays
	}

	// Target system
	row.TargetSystem = getField(cm.targetSystem)
	if row.TargetSystem == "" {
		row.TargetSystem = p.opts.DefaultTargetSystem
	}
	if row.TargetSystem == "" {
		errs = append(errs, ParseError{Row: rowNum, Column: "target_system", Message: "missing target_system"})
	} else if !isValidSystemURI(row.TargetSystem) {
		errs = append(errs, ParseError{Row: rowNum, Column: "target_system", Message: "invalid target_system URI"})
	}

	// Target code
	row.TargetCode = getField(cm.targetCode)
	if row.TargetCode == "" {
		errs = append(errs, ParseError{Row: rowNum, Column: "target_code", Message: "missing target_code"})
	} else if !isValidCode(row.TargetCode) {
		errs = append(errs, ParseError{Row: rowNum, Column: "target_code", Message: "invalid target_code format"})
	}

	// Target display (optional)
	row.TargetDisplay = getField(cm.targetDisplay)
	if len(row.TargetDisplay) > 1000 {
		row.TargetDisplay = row.TargetDisplay[:1000]
	}

	// Equivalence (optional, defaults to "equivalent")
	equivStr := getField(cm.equivalence)
	row.Equivalence = parseEquivalence(equivStr)

	// Confidence (optional)
	confStr := getField(cm.confidence)
	if confStr != "" {
		conf, err := strconv.ParseFloat(confStr, 64)
		if err != nil {
			errs = append(errs, ParseError{Row: rowNum, Column: "confidence", Message: "invalid confidence value"})
		} else if conf < 0 || conf > 1 {
			errs = append(errs, ParseError{Row: rowNum, Column: "confidence", Message: "confidence must be between 0 and 1"})
		} else {
			row.Confidence = &conf
		}
	}

	// Comment (optional)
	row.Comment = getField(cm.comment)
	if len(row.Comment) > 2000 {
		row.Comment = row.Comment[:2000]
	}

	if len(errs) > 0 {
		return row, errs
	}
	return row, nil
}

// parseEquivalence converts a string to MappingEquivalence.
func parseEquivalence(s string) db.MappingEquivalence {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "equivalent", "exact", "equal", "=", "==":
		return db.EquivalenceEquivalent
	case "wider", "broader", "more_general", "general":
		return db.EquivalenceWider
	case "narrower", "more_specific", "specific":
		return db.EquivalenceNarrower
	case "inexact", "approximate", "approx", "~":
		return db.EquivalenceInexact
	default:
		return db.EquivalenceEquivalent // Default
	}
}

// isValidCode checks if a code string is valid.
func isValidCode(s string) bool {
	if len(s) == 0 || len(s) > 100 {
		return false
	}
	if !utf8.ValidString(s) {
		return false
	}
	// Allow alphanumeric, dots, dashes, underscores
	for _, r := range s {
		if !isCodeChar(r) {
			return false
		}
	}
	return true
}

func isCodeChar(r rune) bool {
	return (r >= 'A' && r <= 'Z') ||
		(r >= 'a' && r <= 'z') ||
		(r >= '0' && r <= '9') ||
		r == '.' || r == '-' || r == '_' || r == ':' || r == '/'
}

// isValidSystemName checks if a source system name is valid.
func isValidSystemName(s string) bool {
	if len(s) == 0 || len(s) > 100 {
		return false
	}
	if !utf8.ValidString(s) {
		return false
	}
	// Allow alphanumeric, underscores, dashes
	for _, r := range s {
		isUpper := r >= 'A' && r <= 'Z'
		isLower := r >= 'a' && r <= 'z'
		isDigit := r >= '0' && r <= '9'
		isAllowed := isUpper || isLower || isDigit || r == '_' || r == '-'
		if !isAllowed {
			return false
		}
	}
	return true
}

// isValidSystemURI checks if a target system URI is valid.
func isValidSystemURI(s string) bool {
	if len(s) == 0 || len(s) > 255 {
		return false
	}
	// Accept http(s) URIs or short system names
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return true
	}
	// Also accept simple system identifiers (for internal use)
	return isValidSystemName(s)
}
