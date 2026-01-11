// Package terminology provides code system mapping for healthcare terminologies.
// It supports mapping local codes to standard systems like LOINC, SNOMED CT, ICD-10, and RxNorm.
package terminology

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// Standard code system URIs
const (
	SystemLOINC    = "http://loinc.org"
	SystemSNOMED   = "http://snomed.info/sct"
	SystemICD10CM  = "http://hl7.org/fhir/sid/icd-10-cm"
	SystemICD10PCS = "http://www.cms.gov/Medicare/Coding/ICD10"
	SystemCPT      = "http://www.ama-assn.org/go/cpt"
	SystemRxNorm   = "http://www.nlm.nih.gov/research/umls/rxnorm"
	SystemHCPCS    = "https://www.cms.gov/Medicare/Coding/HCPCSReleaseCodeSets"
	SystemNDC      = "http://hl7.org/fhir/sid/ndc"
	SystemCVX      = "http://hl7.org/fhir/sid/cvx"
)

// MappingEquivalence indicates the quality of a code mapping.
type MappingEquivalence string

const (
	EquivalenceEquivalent MappingEquivalence = "equivalent" // Exact match
	EquivalenceWider      MappingEquivalence = "wider"      // Target is more general
	EquivalenceNarrower   MappingEquivalence = "narrower"   // Target is more specific
	EquivalenceInexact    MappingEquivalence = "inexact"    // Approximate match
	EquivalenceUnmatched  MappingEquivalence = "unmatched"  // No mapping found
)

// CodeMapping represents a mapping from a source code to a target code.
type CodeMapping struct {
	SourceSystem  string             `json:"source_system"`
	SourceCode    string             `json:"source_code"`
	SourceDisplay string             `json:"source_display,omitempty"`
	TargetSystem  string             `json:"target_system"`
	TargetCode    string             `json:"target_code"`
	TargetDisplay string             `json:"target_display,omitempty"`
	Equivalence   MappingEquivalence `json:"equivalence"`
	Comment       string             `json:"comment,omitempty"`
}

// Mapper provides terminology mapping services.
type Mapper struct {
	// mappings indexed by source_system:source_code
	mappings map[string][]CodeMapping
	mu       sync.RWMutex
}

// NewMapper creates a new terminology mapper.
func NewMapper() *Mapper {
	return &Mapper{
		mappings: make(map[string][]CodeMapping),
	}
}

// LoadFromCSV loads mappings from a CSV file.
// Expected columns: source_system, source_code, source_display, target_system, target_code, target_display, equivalence, comment
// Header row is required.
func (m *Mapper) LoadFromCSV(path string) error {
	f, err := os.Open(path) //nolint:gosec // G304: path from trusted caller
	if err != nil {
		return fmt.Errorf("failed to open mapping file: %w", err)
	}
	defer func() { _ = f.Close() }()

	return m.LoadFromReader(f)
}

// LoadFromReader loads mappings from a CSV reader.
func (m *Mapper) LoadFromReader(r io.Reader) error {
	reader := csv.NewReader(r)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1 // Allow variable fields

	// Read header
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("failed to read CSV header: %w", err)
	}

	// Map column names to indices
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.ToLower(strings.TrimSpace(col))] = i
	}

	// Validate required columns
	required := []string{"source_system", "source_code", "target_system", "target_code"}
	for _, col := range required {
		if _, ok := colIdx[col]; !ok {
			return fmt.Errorf("missing required column: %s", col)
		}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Read mappings
	lineNum := 1
	for {
		lineNum++
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("error reading line %d: %w", lineNum, err)
		}

		mapping := CodeMapping{
			SourceSystem:  getCol(record, colIdx, "source_system"),
			SourceCode:    getCol(record, colIdx, "source_code"),
			SourceDisplay: getCol(record, colIdx, "source_display"),
			TargetSystem:  getCol(record, colIdx, "target_system"),
			TargetCode:    getCol(record, colIdx, "target_code"),
			TargetDisplay: getCol(record, colIdx, "target_display"),
			Equivalence:   parseEquivalence(getCol(record, colIdx, "equivalence")),
			Comment:       getCol(record, colIdx, "comment"),
		}

		if mapping.SourceCode == "" || mapping.TargetCode == "" {
			continue // Skip empty rows
		}

		key := m.makeKey(mapping.SourceSystem, mapping.SourceCode)
		m.mappings[key] = append(m.mappings[key], mapping)
	}

	return nil
}

// Map looks up a code in the mapper and returns all mappings to the target system.
func (m *Mapper) Map(sourceSystem, sourceCode, targetSystem string) []CodeMapping {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.makeKey(sourceSystem, sourceCode)
	allMappings := m.mappings[key]

	if targetSystem == "" {
		return allMappings
	}

	// Filter by target system
	var result []CodeMapping
	for _, mapping := range allMappings {
		if mapping.TargetSystem == targetSystem {
			result = append(result, mapping)
		}
	}
	return result
}

// MapToLOINC maps a code to LOINC.
func (m *Mapper) MapToLOINC(sourceSystem, sourceCode string) *CodeMapping {
	mappings := m.Map(sourceSystem, sourceCode, SystemLOINC)
	if len(mappings) == 0 {
		return nil
	}
	return &mappings[0]
}

// MapToSNOMED maps a code to SNOMED CT.
func (m *Mapper) MapToSNOMED(sourceSystem, sourceCode string) *CodeMapping {
	mappings := m.Map(sourceSystem, sourceCode, SystemSNOMED)
	if len(mappings) == 0 {
		return nil
	}
	return &mappings[0]
}

// HasMapping checks if a mapping exists.
func (m *Mapper) HasMapping(sourceSystem, sourceCode string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := m.makeKey(sourceSystem, sourceCode)
	_, ok := m.mappings[key]
	return ok
}

// Count returns the number of unique source codes mapped.
func (m *Mapper) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.mappings)
}

// makeKey creates a lookup key from system and code.
func (m *Mapper) makeKey(system, code string) string {
	return strings.ToUpper(system) + ":" + strings.ToUpper(code)
}

// getCol safely gets a column value from a record.
func getCol(record []string, colIdx map[string]int, colName string) string {
	idx, ok := colIdx[colName]
	if !ok || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}

// parseEquivalence converts a string to MappingEquivalence.
func parseEquivalence(s string) MappingEquivalence {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "equivalent", "equal", "exact":
		return EquivalenceEquivalent
	case "wider", "broader":
		return EquivalenceWider
	case "narrower", "more specific":
		return EquivalenceNarrower
	case "inexact", "approximate":
		return EquivalenceInexact
	default:
		return EquivalenceEquivalent // Default to equivalent
	}
}

// Registry manages multiple terminology mappers.
type Registry struct {
	mappers map[string]*Mapper // keyed by source_system:target_system
	mu      sync.RWMutex
}

// NewRegistry creates a new terminology registry.
func NewRegistry() *Registry {
	return &Registry{
		mappers: make(map[string]*Mapper),
	}
}

// RegisterMapper adds a mapper for a specific source/target pair.
func (r *Registry) RegisterMapper(sourceSystem, targetSystem string, mapper *Mapper) {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := sourceSystem + ":" + targetSystem
	r.mappers[key] = mapper
}

// GetMapper retrieves a mapper for a source/target pair.
func (r *Registry) GetMapper(sourceSystem, targetSystem string) *Mapper {
	r.mu.RLock()
	defer r.mu.RUnlock()
	key := sourceSystem + ":" + targetSystem
	return r.mappers[key]
}

// Map looks up a code across all registered mappers.
func (r *Registry) Map(sourceSystem, sourceCode, targetSystem string) []CodeMapping {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []CodeMapping
	for _, mapper := range r.mappers {
		mappings := mapper.Map(sourceSystem, sourceCode, targetSystem)
		result = append(result, mappings...)
	}
	return result
}
