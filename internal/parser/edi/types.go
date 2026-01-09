// Package edi provides X12 EDI parsing for healthcare transaction sets (837, 835, etc.)
package edi

// Delimiters holds the separator characters extracted from the ISA segment
type Delimiters struct {
	Element    byte // Element separator (typically *)
	Subelement byte // Component separator (typically :)
	Segment    byte // Segment terminator (typically ~)
	Repetition byte // Repetition separator (typically ^)
}

// DefaultDelimiters returns standard X12 delimiters
func DefaultDelimiters() Delimiters {
	return Delimiters{
		Element:    '*',
		Subelement: ':',
		Segment:    '~',
		Repetition: '^',
	}
}

// Segment represents a single EDI segment with its elements
type Segment struct {
	ID       string   // Segment identifier (ISA, GS, ST, CLM, etc.)
	Elements []string // Element values (index 0 is first element after ID)
	Raw      string   // Original raw segment string
}

// GetElement safely retrieves an element by index (1-based to match X12 spec)
func (s *Segment) GetElement(index int) string {
	// X12 uses 1-based indexing; element 0 is the segment ID
	if index < 1 || index > len(s.Elements) {
		return ""
	}
	return s.Elements[index-1]
}

// GetComponent retrieves a component from a composite element (1-based)
func (s *Segment) GetComponent(elementIndex, componentIndex int, subSep byte) string {
	element := s.GetElement(elementIndex)
	if element == "" {
		return ""
	}
	parts := splitByByte(element, subSep)
	if componentIndex < 1 || componentIndex > len(parts) {
		return ""
	}
	return parts[componentIndex-1]
}

// Interchange represents the outermost X12 envelope (ISA/IEA)
type Interchange struct {
	ControlNumber       string     // ISA13 - Unique interchange control number
	SenderQualifier     string     // ISA05
	SenderID            string     // ISA06
	ReceiverQualifier   string     // ISA07
	ReceiverID          string     // ISA08
	Date                string     // ISA09 (YYMMDD)
	Time                string     // ISA10 (HHMM)
	ControlVersion      string     // ISA12 (e.g., "00501")
	UsageIndicator      string     // ISA15 (P=Production, T=Test)
	Delimiters          Delimiters // Extracted from ISA segment
	FunctionalGroups    []*FunctionalGroup
	TrailerSegmentCount int // IEA01
	RawISA              string
	RawIEA              string
}

// FunctionalGroup represents a GS/GE envelope containing transactions of the same type
type FunctionalGroup struct {
	ControlNumber     string // GS06 - Group control number
	FunctionalID      string // GS01 (HC=Healthcare Claim, HP=Health Plan, etc.)
	SenderCode        string // GS02
	ReceiverCode      string // GS03
	Date              string // GS04 (CCYYMMDD)
	Time              string // GS05 (HHMM or HHMMSS)
	ResponsibleAgency string // GS07 (X = X12)
	VersionCode       string // GS08 (e.g., "005010X222A1")
	Transactions      []*Transaction
	RawGS             string
	RawGE             string
}

// Transaction represents a single ST/SE transaction set (one claim, one remittance, etc.)
type Transaction struct {
	ControlNumber     string     // ST02 - Transaction control number
	SetIdentifier     string     // ST01 (837, 835, 270, etc.)
	ImplementationRef string     // ST03 (implementation convention reference)
	Segments          []*Segment // All segments between ST and SE
	SegmentCount      int        // SE01 - Number of segments including ST/SE
	RawST             string
	RawSE             string
}

// GetSegment finds the first segment with the given ID
func (t *Transaction) GetSegment(id string) *Segment {
	for _, seg := range t.Segments {
		if seg.ID == id {
			return seg
		}
	}
	return nil
}

// GetSegments returns all segments with the given ID
func (t *Transaction) GetSegments(id string) []*Segment {
	var result []*Segment
	for _, seg := range t.Segments {
		if seg.ID == id {
			result = append(result, seg)
		}
	}
	return result
}

// HLNode represents a hierarchical level in the HL loop structure
type HLNode struct {
	ID          string     // HL01 - Hierarchical ID
	ParentID    string     // HL02 - Parent hierarchical ID
	LevelCode   string     // HL03 - Level code (20=Provider, 22=Subscriber, 23=Dependent)
	HasChildren bool       // HL04 - 1 = has children, 0 = no children
	Segments    []*Segment // Segments belonging to this HL level
	Children    []*HLNode  // Child HL nodes
}

// HLLevelCode constants for healthcare transactions
const (
	HLLevelInformationSource   = "20" // Billing Provider in 837
	HLLevelInformationReceiver = "21" // Information Receiver
	HLLevelSubscriber          = "22" // Subscriber
	HLLevelDependent           = "23" // Patient/Dependent
)

// Loop represents a named loop in an X12 transaction (e.g., 2000A, 2010AA, 2300)
type Loop struct {
	ID       string     // Loop identifier (2000A, 2010AA, 2300, etc.)
	Segments []*Segment // Segments in this loop
	Loops    []*Loop    // Nested loops
}

// GetSegment finds the first segment with the given ID in this loop
func (l *Loop) GetSegment(id string) *Segment {
	for _, seg := range l.Segments {
		if seg.ID == id {
			return seg
		}
	}
	return nil
}

// GetLoop finds a nested loop by ID
func (l *Loop) GetLoop(id string) *Loop {
	for _, loop := range l.Loops {
		if loop.ID == id {
			return loop
		}
	}
	return nil
}

// TransactionType identifies the type of X12 transaction
type TransactionType string

const (
	Transaction837P    TransactionType = "837P" // Professional Claim
	Transaction837I    TransactionType = "837I" // Institutional Claim
	Transaction837D    TransactionType = "837D" // Dental Claim
	Transaction835     TransactionType = "835"  // Remittance Advice
	Transaction270     TransactionType = "270"  // Eligibility Inquiry
	Transaction271     TransactionType = "271"  // Eligibility Response
	Transaction276     TransactionType = "276"  // Claim Status Request
	Transaction277     TransactionType = "277"  // Claim Status Response
	Transaction278     TransactionType = "278"  // Authorization
	Transaction834     TransactionType = "834"  // Enrollment
	TransactionUnknown TransactionType = "UNKNOWN"
)

// FunctionalIDCode constants
const (
	FuncIDHealthcareClaim       = "HC" // 837
	FuncIDHealthcarePlanInfo    = "HP" // 271, 277
	FuncIDHealthcareEligibility = "HS" // 270
	FuncIDRemittanceAdvice      = "HR" // 835
	FuncIDAuthorization         = "HN" // 278
	FuncIDBenefitEnrollment     = "BE" // 834
)

// ClaimStatusCode from CLP02
type ClaimStatusCode string

const (
	ClaimStatusProcessedPrimary      ClaimStatusCode = "1"
	ClaimStatusProcessedSecondary    ClaimStatusCode = "2"
	ClaimStatusProcessedTertiary     ClaimStatusCode = "3"
	ClaimStatusDenied                ClaimStatusCode = "4"
	ClaimStatusPended                ClaimStatusCode = "19"
	ClaimStatusProcessedPrimaryFwd   ClaimStatusCode = "20"
	ClaimStatusProcessedSecondaryFwd ClaimStatusCode = "21"
	ClaimStatusReversed              ClaimStatusCode = "22"
	ClaimStatusNotOurClaim           ClaimStatusCode = "23"
)

// AdjustmentGroupCode from CAS01
type AdjustmentGroupCode string

const (
	AdjGroupContractual           AdjustmentGroupCode = "CO" // Contractual Obligations
	AdjGroupPatientResponsibility AdjustmentGroupCode = "PR" // Patient Responsibility
	AdjGroupOtherAdjustments      AdjustmentGroupCode = "OA" // Other Adjustments
	AdjGroupPayerInitiated        AdjustmentGroupCode = "PI" // Payer Initiated
	AdjGroupCorrection            AdjustmentGroupCode = "CR" // Correction
)

// ParseError represents an error during EDI parsing
type ParseError struct {
	Phase   string // "envelope", "segment", "loop", "semantic"
	Code    string // Error code
	Message string // Human-readable message
	Segment string // Segment ID where error occurred
	Element int    // Element index (if applicable)
	Line    int    // Approximate line/position
}

func (e *ParseError) Error() string {
	return e.Message
}

// ParseWarning represents a non-fatal issue found during parsing
type ParseWarning struct {
	Phase   string
	Code    string
	Message string
	Segment string
	Element int
}

// ParseResult holds the complete result of parsing an EDI file
type ParseResult struct {
	Interchange *Interchange
	Warnings    []ParseWarning
}

// Helper function to split by byte (avoiding strings.Split for single char)
func splitByByte(s string, sep byte) []string {
	if s == "" {
		return nil
	}
	n := 1
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			n++
		}
	}
	result := make([]string, n)
	idx := 0
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == sep {
			result[idx] = s[start:i]
			idx++
			start = i + 1
		}
	}
	result[idx] = s[start:]
	return result
}
