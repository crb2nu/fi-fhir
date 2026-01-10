// Package terminology provides code system mapping for healthcare terminologies.
package terminology

import (
	"bufio"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

// LOINCCode represents a LOINC code with its attributes.
type LOINCCode struct {
	// Core identifiers
	Code      string `json:"loinc_num"`
	LongName  string `json:"long_common_name"`
	ShortName string `json:"shortname"`
	Status    string `json:"status"` // ACTIVE, TRIAL, DISCOURAGED, DEPRECATED

	// 6-axis model
	Component string `json:"component"`
	Property  string `json:"property"`
	TimeAspct string `json:"time_aspct"`
	System    string `json:"system"`
	ScaleTyp  string `json:"scale_typ"`
	MethodTyp string `json:"method_typ"`

	// Classification
	Class     string `json:"class"`
	ClassType string `json:"classtype"` // 1=Lab, 2=Clinical, 3=Claims, 4=Surveys

	// Additional attributes
	ExampleUnits   string `json:"example_units"`
	OrderObs       string `json:"order_obs"` // Order, Observation, Both
	UnitsRequired  string `json:"unitsrequired"`
	RelatedNames   string `json:"relatednames2"`
	Consumer       string `json:"consumer_name"`
	VersionChanged string `json:"versionlastchanged"`
}

// IsActive returns true if the code is active.
func (c *LOINCCode) IsActive() bool {
	return c.Status == "ACTIVE"
}

// IsLab returns true if this is a laboratory test.
func (c *LOINCCode) IsLab() bool {
	return c.ClassType == "1"
}

// DisplayName returns the best display name for the code.
func (c *LOINCCode) DisplayName() string {
	if c.Consumer != "" {
		return c.Consumer
	}
	if c.ShortName != "" {
		return c.ShortName
	}
	return c.LongName
}

// PanelMember represents a member of a LOINC panel.
type PanelMember struct {
	ParentCode  string `json:"parent_loinc"`
	Code        string `json:"loinc"`
	Sequence    int    `json:"sequence"`
	Cardinality string `json:"cardinality"` // R=Required, O=Optional
}

// LOINCLoader loads and indexes LOINC codes from official distribution files.
type LOINCLoader struct {
	codes        map[string]*LOINCCode    // LOINC_NUM -> code
	byComponent  map[string][]*LOINCCode  // Component -> codes
	byShortName  map[string][]*LOINCCode  // ShortName -> codes
	panels       map[string][]PanelMember // Panel code -> members
	panelParents map[string][]string      // Code -> parent panels
	mu           sync.RWMutex
	version      string
}

// NewLOINCLoader creates a new LOINC loader.
func NewLOINCLoader() *LOINCLoader {
	return &LOINCLoader{
		codes:        make(map[string]*LOINCCode),
		byComponent:  make(map[string][]*LOINCCode),
		byShortName:  make(map[string][]*LOINCCode),
		panels:       make(map[string][]PanelMember),
		panelParents: make(map[string][]string),
	}
}

// LoadLoincTable loads the main LOINC table from LoincTable.csv.
// This is the core file from the LOINC distribution.
func (l *LOINCLoader) LoadLoincTable(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open LOINC table: %w", err)
	}
	defer f.Close()

	return l.LoadLoincTableFromReader(f)
}

// LoadLoincTableFromReader loads the LOINC table from a reader.
func (l *LOINCLoader) LoadLoincTableFromReader(r io.Reader) error {
	// LOINC CSV files use comma delimiter and may have embedded commas in quoted fields
	reader := csv.NewReader(bufio.NewReader(r))
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1 // Variable fields

	// Read header
	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("failed to read LOINC header: %w", err)
	}

	// Map column names to indices (LOINC uses uppercase column names)
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.ToUpper(strings.TrimSpace(col))] = i
	}

	// Validate required columns
	required := []string{"LOINC_NUM", "LONG_COMMON_NAME", "STATUS"}
	for _, col := range required {
		if _, ok := colIdx[col]; !ok {
			return fmt.Errorf("missing required LOINC column: %s", col)
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Read all codes
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip malformed rows (LOINC files sometimes have issues)
			continue
		}

		code := &LOINCCode{
			Code:           getLoincCol(record, colIdx, "LOINC_NUM"),
			LongName:       getLoincCol(record, colIdx, "LONG_COMMON_NAME"),
			ShortName:      getLoincCol(record, colIdx, "SHORTNAME"),
			Status:         getLoincCol(record, colIdx, "STATUS"),
			Component:      getLoincCol(record, colIdx, "COMPONENT"),
			Property:       getLoincCol(record, colIdx, "PROPERTY"),
			TimeAspct:      getLoincCol(record, colIdx, "TIME_ASPCT"),
			System:         getLoincCol(record, colIdx, "SYSTEM"),
			ScaleTyp:       getLoincCol(record, colIdx, "SCALE_TYP"),
			MethodTyp:      getLoincCol(record, colIdx, "METHOD_TYP"),
			Class:          getLoincCol(record, colIdx, "CLASS"),
			ClassType:      getLoincCol(record, colIdx, "CLASSTYPE"),
			ExampleUnits:   getLoincCol(record, colIdx, "EXAMPLE_UNITS"),
			OrderObs:       getLoincCol(record, colIdx, "ORDER_OBS"),
			UnitsRequired:  getLoincCol(record, colIdx, "UNITSREQUIRED"),
			RelatedNames:   getLoincCol(record, colIdx, "RELATEDNAMES2"),
			Consumer:       getLoincCol(record, colIdx, "CONSUMER_NAME"),
			VersionChanged: getLoincCol(record, colIdx, "VERSIONLASTCHANGED"),
		}

		if code.Code == "" {
			continue
		}

		// Index by code
		l.codes[code.Code] = code

		// Index by component (for fuzzy matching)
		compKey := strings.ToUpper(code.Component)
		l.byComponent[compKey] = append(l.byComponent[compKey], code)

		// Index by short name
		if code.ShortName != "" {
			shortKey := strings.ToUpper(code.ShortName)
			l.byShortName[shortKey] = append(l.byShortName[shortKey], code)
		}
	}

	return nil
}

// LoadPanelHierarchy loads panel relationships from PanelHierarchy.csv.
func (l *LOINCLoader) LoadPanelHierarchy(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open panel hierarchy: %w", err)
	}
	defer f.Close()

	return l.LoadPanelHierarchyFromReader(f)
}

// LoadPanelHierarchyFromReader loads panel relationships from a reader.
func (l *LOINCLoader) LoadPanelHierarchyFromReader(r io.Reader) error {
	reader := csv.NewReader(bufio.NewReader(r))
	reader.LazyQuotes = true
	reader.FieldsPerRecord = -1

	header, err := reader.Read()
	if err != nil {
		return fmt.Errorf("failed to read panel header: %w", err)
	}

	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.ToUpper(strings.TrimSpace(col))] = i
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		parentCode := getLoincCol(record, colIdx, "PARENTLOINC")
		if parentCode == "" {
			parentCode = getLoincCol(record, colIdx, "PARENT_LOINC")
		}
		memberCode := getLoincCol(record, colIdx, "LOINC")

		if parentCode == "" || memberCode == "" {
			continue
		}

		member := PanelMember{
			ParentCode:  parentCode,
			Code:        memberCode,
			Cardinality: getLoincCol(record, colIdx, "CARDINALITY"),
		}

		l.panels[parentCode] = append(l.panels[parentCode], member)
		l.panelParents[memberCode] = append(l.panelParents[memberCode], parentCode)
	}

	return nil
}

// GetCode retrieves a LOINC code by its code number.
func (l *LOINCLoader) GetCode(loincNum string) *LOINCCode {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.codes[loincNum]
}

// LookupByCode is an alias for GetCode.
func (l *LOINCLoader) LookupByCode(loincNum string) *LOINCCode {
	return l.GetCode(loincNum)
}

// LookupByDisplay finds codes matching a display name or component.
func (l *LOINCLoader) LookupByDisplay(display string) []*LOINCCode {
	l.mu.RLock()
	defer l.mu.RUnlock()

	displayUpper := strings.ToUpper(strings.TrimSpace(display))
	var results []*LOINCCode

	// Check exact short name match
	if codes, ok := l.byShortName[displayUpper]; ok {
		results = append(results, codes...)
	}

	// Check component match
	if codes, ok := l.byComponent[displayUpper]; ok {
		for _, c := range codes {
			// Avoid duplicates
			found := false
			for _, r := range results {
				if r.Code == c.Code {
					found = true
					break
				}
			}
			if !found {
				results = append(results, c)
			}
		}
	}

	return results
}

// SearchCodes searches for codes containing the given text in name or component.
func (l *LOINCLoader) SearchCodes(query string, limit int) []*LOINCCode {
	l.mu.RLock()
	defer l.mu.RUnlock()

	queryUpper := strings.ToUpper(query)
	var results []*LOINCCode

	for _, code := range l.codes {
		if !code.IsActive() {
			continue
		}

		// Check if query matches component, short name, or long name
		if strings.Contains(strings.ToUpper(code.Component), queryUpper) ||
			strings.Contains(strings.ToUpper(code.ShortName), queryUpper) ||
			strings.Contains(strings.ToUpper(code.LongName), queryUpper) {
			results = append(results, code)
			if limit > 0 && len(results) >= limit {
				break
			}
		}
	}

	return results
}

// ExpandPanel returns all member codes for a panel.
// For example, CBC (58410-2) expands to WBC, RBC, Hgb, etc.
func (l *LOINCLoader) ExpandPanel(panelCode string) []*LOINCCode {
	l.mu.RLock()
	defer l.mu.RUnlock()

	members, ok := l.panels[panelCode]
	if !ok {
		return nil
	}

	var results []*LOINCCode
	for _, member := range members {
		if code := l.codes[member.Code]; code != nil {
			results = append(results, code)
		}
	}

	return results
}

// GetPanelMembers returns panel member details (with cardinality).
func (l *LOINCLoader) GetPanelMembers(panelCode string) []PanelMember {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.panels[panelCode]
}

// GetParentPanels returns panels that contain this code.
func (l *LOINCLoader) GetParentPanels(code string) []string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.panelParents[code]
}

// IsPanel returns true if the code is a panel.
func (l *LOINCLoader) IsPanel(code string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, ok := l.panels[code]
	return ok
}

// Count returns the number of loaded LOINC codes.
func (l *LOINCLoader) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.codes)
}

// PanelCount returns the number of loaded panels.
func (l *LOINCLoader) PanelCount() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.panels)
}

// SetVersion sets the LOINC version for this loader.
func (l *LOINCLoader) SetVersion(version string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.version = version
}

// Version returns the LOINC version.
func (l *LOINCLoader) Version() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.version
}

// ToCodeMapping converts a LOINCCode to a CodeMapping for integration with Mapper.
func (c *LOINCCode) ToCodeMapping(sourceSystem, sourceCode string) CodeMapping {
	return CodeMapping{
		SourceSystem:  sourceSystem,
		SourceCode:    sourceCode,
		TargetSystem:  SystemLOINC,
		TargetCode:    c.Code,
		TargetDisplay: c.DisplayName(),
		Equivalence:   EquivalenceEquivalent,
	}
}

// getLoincCol safely gets a column value from a LOINC CSV record.
func getLoincCol(record []string, colIdx map[string]int, colName string) string {
	idx, ok := colIdx[colName]
	if !ok || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}

// CommonPanels provides LOINC codes for commonly used lab panels.
var CommonPanels = map[string]string{
	"CBC":         "58410-2", // Complete Blood Count
	"BMP":         "51990-0", // Basic Metabolic Panel
	"CMP":         "24323-8", // Comprehensive Metabolic Panel
	"LIPID":       "57698-3", // Lipid Panel
	"LIPID_PANEL": "57698-3",
	"LFT":         "24325-3", // Liver Function Tests
	"HEPATIC":     "24325-3",
	"URINALYSIS":  "24356-8", // Urinalysis
	"UA":          "24356-8",
	"TSH":         "3016-3", // TSH alone (not a panel but commonly referenced)
	"HBA1C":       "4548-4", // Hemoglobin A1c
	"A1C":         "4548-4",
	"PT_INR":      "34714-6", // PT/INR Panel
	"COAG":        "34714-6",
}

// CommonLabCodes provides LOINC codes for commonly used individual tests.
var CommonLabCodes = map[string]string{
	// Hematology
	"WBC":  "6690-2", // White blood cells
	"RBC":  "789-8",  // Red blood cells
	"HGB":  "718-7",  // Hemoglobin
	"HCT":  "4544-3", // Hematocrit
	"PLT":  "777-3",  // Platelets
	"MCV":  "787-2",  // Mean corpuscular volume
	"MCH":  "785-6",  // Mean corpuscular hemoglobin
	"MCHC": "786-4",  // Mean corpuscular hemoglobin concentration

	// Chemistry
	"GLUCOSE":    "2345-7",  // Glucose
	"BUN":        "3094-0",  // Blood urea nitrogen
	"CREATININE": "2160-0",  // Creatinine
	"SODIUM":     "2951-2",  // Sodium
	"POTASSIUM":  "2823-3",  // Potassium
	"CHLORIDE":   "2075-0",  // Chloride
	"CO2":        "2028-9",  // Carbon dioxide
	"CALCIUM":    "17861-6", // Calcium
	"MAGNESIUM":  "19123-9", // Magnesium

	// Liver
	"ALT":       "1742-6", // Alanine aminotransferase
	"AST":       "1920-8", // Aspartate aminotransferase
	"ALP":       "6768-6", // Alkaline phosphatase
	"BILIRUBIN": "1975-2", // Total bilirubin
	"ALBUMIN":   "1751-7", // Albumin
	"PROTEIN":   "2885-2", // Total protein

	// Lipids
	"CHOLESTEROL":  "2093-3", // Total cholesterol
	"HDL":          "2085-9", // HDL cholesterol
	"LDL":          "2089-1", // LDL cholesterol (calculated)
	"TRIGLYCERIDE": "2571-8", // Triglycerides

	// Thyroid
	"TSH":     "3016-3", // Thyroid stimulating hormone
	"T3":      "3053-6", // Triiodothyronine
	"T4":      "3026-2", // Thyroxine
	"FREE_T4": "3024-7", // Free thyroxine

	// Cardiac
	"TROPONIN_I": "10839-9", // Troponin I
	"TROPONIN_T": "6598-7",  // Troponin T
	"BNP":        "30934-4", // Brain natriuretic peptide
	"PRBNP":      "33762-6", // NT-proBNP

	// Coagulation
	"PT":  "5902-2", // Prothrombin time
	"INR": "6301-6", // INR
	"PTT": "3173-2", // Partial thromboplastin time

	// Diabetes
	"HBA1C": "4548-4", // Hemoglobin A1c

	// Urinalysis
	"UA_GLUCOSE":          "2350-7", // Urine glucose
	"UA_PROTEIN":          "2888-6", // Urine protein
	"UA_BLOOD":            "5794-3", // Urine blood
	"UA_PH":               "2756-5", // Urine pH
	"UA_SPECIFIC_GRAVITY": "2965-2", // Urine specific gravity
}

// GetCommonPanelCode returns the LOINC code for a common panel name.
func GetCommonPanelCode(panelName string) string {
	return CommonPanels[strings.ToUpper(panelName)]
}

// GetCommonLabCode returns the LOINC code for a common lab test name.
func GetCommonLabCode(testName string) string {
	return CommonLabCodes[strings.ToUpper(testName)]
}
