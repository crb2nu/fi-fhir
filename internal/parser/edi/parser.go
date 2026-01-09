package edi

import (
	"fmt"
	"strings"
)

// Parser handles X12 EDI parsing
type Parser struct {
	warnings []ParseWarning
}

// NewParser creates a new EDI parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse parses a complete EDI interchange from raw content
func (p *Parser) Parse(content string) (*ParseResult, error) {
	p.warnings = nil

	// Normalize line endings and remove extra whitespace
	content = normalizeContent(content)

	if len(content) < 106 {
		return nil, &ParseError{
			Phase:   "envelope",
			Code:    "ISA_TOO_SHORT",
			Message: "content too short to contain valid ISA segment",
		}
	}

	// Extract delimiters from ISA segment
	delims, err := p.extractDelimiters(content)
	if err != nil {
		return nil, err
	}

	// Tokenize into segments
	segments, err := p.tokenizeSegments(content, delims)
	if err != nil {
		return nil, err
	}

	if len(segments) == 0 {
		return nil, &ParseError{
			Phase:   "envelope",
			Code:    "NO_SEGMENTS",
			Message: "no segments found in content",
		}
	}

	// Parse interchange envelope
	interchange, err := p.parseInterchange(segments, delims)
	if err != nil {
		return nil, err
	}

	return &ParseResult{
		Interchange: interchange,
		Warnings:    p.warnings,
	}, nil
}

// extractDelimiters reads delimiter characters from the ISA segment
// ISA is fixed-width: positions matter, not delimiters for parsing ISA itself
func (p *Parser) extractDelimiters(content string) (Delimiters, error) {
	// ISA segment is fixed-width with positions:
	// ISA*00*          *00*          *ZZ*...
	// Position 3 is element separator (after "ISA")
	// Position 104 is repetition separator
	// Position 105 is component separator
	// Position 106 is segment terminator

	if len(content) < 4 {
		return Delimiters{}, &ParseError{
			Phase:   "envelope",
			Code:    "ISA_TOO_SHORT",
			Message: "content too short for ISA segment",
		}
	}

	if !strings.HasPrefix(content, "ISA") {
		return Delimiters{}, &ParseError{
			Phase:   "envelope",
			Code:    "NO_ISA",
			Message: "content does not start with ISA segment",
		}
	}

	delims := Delimiters{
		Element: content[3], // Character right after "ISA"
	}

	// ISA11 position depends on element separator and fixed field widths
	// For now, extract repetition separator later after parsing elements

	// Find position 105 and 106 (component and segment separators)
	// ISA has exactly 16 elements, we need to count to find the end
	// The segment terminator follows the last character of ISA16 (component separator)

	// ISA is exactly 106 characters (including segment terminator for standard EDI)
	// But the actual positions depend on the element separator we found

	// Parse ISA by counting elements
	isaContent := content
	elementCount := 0
	pos := 3 // Start after "ISA"

	for pos < len(isaContent) && elementCount < 16 {
		if isaContent[pos] == delims.Element {
			elementCount++
		}
		pos++
	}

	// After 16 elements, we're at the end of ISA16
	// ISA16 is the component separator, followed by segment terminator
	if pos > len(isaContent)-1 {
		return Delimiters{}, &ParseError{
			Phase:   "envelope",
			Code:    "ISA_MALFORMED",
			Message: "unable to determine segment terminator from ISA",
		}
	}

	// Back up one position - pos points after the 16th element separator
	// The character at pos-1 is the last char of ISA16 (component separator is ISA16 content)
	// The character at pos is the segment terminator

	// Actually, let's recalculate: after finding 16 element separators,
	// we've consumed all of ISA. The next character is the segment terminator.
	if pos >= len(isaContent) {
		return Delimiters{}, &ParseError{
			Phase:   "envelope",
			Code:    "ISA_MALFORMED",
			Message: "ISA segment appears truncated",
		}
	}

	// ISA16 contains the component element separator
	// Find ISA16 value by parsing elements
	isaElements := p.parseISAElements(content, delims.Element)
	if len(isaElements) < 17 {
		return Delimiters{}, &ParseError{
			Phase:   "envelope",
			Code:    "ISA_ELEMENTS",
			Message: fmt.Sprintf("ISA has only %d elements, expected 17 (including ISA ID)", len(isaElements)),
		}
	}

	// ISA16 (index 16 since 0 is "ISA") is the component separator
	if len(isaElements[16]) > 0 {
		delims.Subelement = isaElements[16][0]
	} else {
		delims.Subelement = ':' // Default
	}

	// ISA11 (index 11) is the repetition separator
	if len(isaElements) > 11 && len(isaElements[11]) > 0 {
		delims.Repetition = isaElements[11][0]
	} else {
		delims.Repetition = '^' // Default
	}

	// The segment terminator follows ISA16
	// Find it by looking at character after the full ISA
	isaEnd := 0
	elementsSeen := 0
	for i := 0; i < len(content); i++ {
		if content[i] == delims.Element {
			elementsSeen++
			if elementsSeen == 16 {
				// Next character after this separator is ISA16 value
				// ISA16 is 1 char, then segment terminator
				isaEnd = i + 2 // Skip separator, skip ISA16 value
				break
			}
		}
	}

	if isaEnd >= len(content) {
		return Delimiters{}, &ParseError{
			Phase:   "envelope",
			Code:    "ISA_TERMINATOR",
			Message: "unable to find segment terminator after ISA",
		}
	}

	delims.Segment = content[isaEnd]

	return delims, nil
}

// parseISAElements extracts ISA elements knowing only the element separator
func (p *Parser) parseISAElements(content string, elementSep byte) []string {
	// Find first segment terminator candidate
	// Start from position 100+ where we expect it
	end := len(content)
	for i := 100; i < len(content) && i < 110; i++ {
		// Segment terminator is typically ~ or newline
		if content[i] != elementSep && (content[i] == '~' || content[i] == '\n' || content[i] == '\r') {
			end = i
			break
		}
	}

	isa := content[:end]
	return splitByByte(isa, elementSep)
}

// tokenizeSegments splits content into segments
func (p *Parser) tokenizeSegments(content string, delims Delimiters) ([]*Segment, error) {
	var segments []*Segment

	// Split by segment terminator
	rawSegments := strings.Split(content, string(delims.Segment))

	for _, raw := range rawSegments {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}

		seg := p.parseSegment(raw, delims)
		segments = append(segments, seg)
	}

	return segments, nil
}

// parseSegment parses a single segment string into a Segment struct
func (p *Parser) parseSegment(raw string, delims Delimiters) *Segment {
	elements := splitByByte(raw, delims.Element)

	if len(elements) == 0 {
		return &Segment{Raw: raw}
	}

	return &Segment{
		ID:       elements[0],
		Elements: elements[1:],
		Raw:      raw,
	}
}

// parseInterchange builds the Interchange structure from segments
func (p *Parser) parseInterchange(segments []*Segment, delims Delimiters) (*Interchange, error) {
	if len(segments) == 0 || segments[0].ID != "ISA" {
		return nil, &ParseError{
			Phase:   "envelope",
			Code:    "NO_ISA",
			Message: "first segment must be ISA",
		}
	}

	isa := segments[0]
	interchange := &Interchange{
		Delimiters:        delims,
		RawISA:            isa.Raw,
		SenderQualifier:   isa.GetElement(5),
		SenderID:          strings.TrimSpace(isa.GetElement(6)),
		ReceiverQualifier: isa.GetElement(7),
		ReceiverID:        strings.TrimSpace(isa.GetElement(8)),
		Date:              isa.GetElement(9),
		Time:              isa.GetElement(10),
		ControlVersion:    isa.GetElement(12),
		ControlNumber:     isa.GetElement(13),
		UsageIndicator:    isa.GetElement(15),
	}

	// Parse functional groups and find IEA
	gsStart := -1
	var currentGroup *FunctionalGroup

	for i := 1; i < len(segments); i++ {
		seg := segments[i]

		switch seg.ID {
		case "GS":
			gsStart = i
			currentGroup = &FunctionalGroup{
				RawGS:             seg.Raw,
				FunctionalID:      seg.GetElement(1),
				SenderCode:        seg.GetElement(2),
				ReceiverCode:      seg.GetElement(3),
				Date:              seg.GetElement(4),
				Time:              seg.GetElement(5),
				ControlNumber:     seg.GetElement(6),
				ResponsibleAgency: seg.GetElement(7),
				VersionCode:       seg.GetElement(8),
			}

		case "GE":
			if currentGroup != nil {
				currentGroup.RawGE = seg.Raw
				interchange.FunctionalGroups = append(interchange.FunctionalGroups, currentGroup)
				currentGroup = nil
			}
			gsStart = -1

		case "ST":
			if currentGroup == nil {
				p.addWarning("envelope", "ST_NO_GS", "ST segment found outside GS envelope", seg.ID, 0)
				continue
			}
			tx, endIdx, err := p.parseTransaction(segments[i:], delims)
			if err != nil {
				return nil, err
			}
			currentGroup.Transactions = append(currentGroup.Transactions, tx)
			i += endIdx // Skip past SE

		case "IEA":
			interchange.RawIEA = seg.Raw
			// IEA01 is number of functional groups
			// IEA02 is interchange control number

		default:
			// Segments outside expected structure
			if gsStart == -1 && seg.ID != "IEA" && seg.ID != "ISA" {
				p.addWarning("envelope", "UNEXPECTED_SEGMENT",
					fmt.Sprintf("segment %s outside envelope structure", seg.ID), seg.ID, 0)
			}
		}
	}

	return interchange, nil
}

// parseTransaction parses a single ST/SE transaction set
func (p *Parser) parseTransaction(segments []*Segment, delims Delimiters) (*Transaction, int, error) {
	if len(segments) == 0 || segments[0].ID != "ST" {
		return nil, 0, &ParseError{
			Phase:   "envelope",
			Code:    "NO_ST",
			Message: "expected ST segment",
		}
	}

	st := segments[0]
	tx := &Transaction{
		RawST:             st.Raw,
		SetIdentifier:     st.GetElement(1),
		ControlNumber:     st.GetElement(2),
		ImplementationRef: st.GetElement(3),
	}

	// Collect segments until SE
	var endIdx int
	for i := 1; i < len(segments); i++ {
		seg := segments[i]

		if seg.ID == "SE" {
			tx.RawSE = seg.Raw
			// SE01 is segment count, SE02 is control number
			endIdx = i
			break
		}

		// Check for unexpected envelope segments
		if seg.ID == "GE" || seg.ID == "GS" || seg.ID == "IEA" || seg.ID == "ISA" {
			return nil, 0, &ParseError{
				Phase:   "envelope",
				Code:    "ENVELOPE_IN_TX",
				Message: fmt.Sprintf("envelope segment %s found inside transaction", seg.ID),
				Segment: seg.ID,
			}
		}

		tx.Segments = append(tx.Segments, seg)
	}

	if endIdx == 0 {
		return nil, 0, &ParseError{
			Phase:   "envelope",
			Code:    "NO_SE",
			Message: "transaction missing SE segment",
		}
	}

	tx.SegmentCount = len(tx.Segments) + 2 // +2 for ST and SE

	return tx, endIdx, nil
}

// addWarning records a parse warning
func (p *Parser) addWarning(phase, code, message, segment string, element int) {
	p.warnings = append(p.warnings, ParseWarning{
		Phase:   phase,
		Code:    code,
		Message: message,
		Segment: segment,
		Element: element,
	})
}

// normalizeContent prepares EDI content for parsing
func normalizeContent(content string) string {
	// Remove BOM if present
	if strings.HasPrefix(content, "\xef\xbb\xbf") {
		content = content[3:]
	}

	// Normalize line endings - some EDI uses \n as segment terminator
	// but we should preserve the actual terminator from ISA
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")

	return strings.TrimSpace(content)
}

// GetTransactionType determines the type of transaction from ST01 and context
func GetTransactionType(tx *Transaction) TransactionType {
	switch tx.SetIdentifier {
	case "837":
		// Need to look at BHT or claims to determine P/I/D
		// For now, check implementation reference
		ref := tx.ImplementationRef
		switch {
		case strings.Contains(ref, "X222"):
			return Transaction837P
		case strings.Contains(ref, "X223"):
			return Transaction837I
		case strings.Contains(ref, "X224"):
			return Transaction837D
		default:
			// Default to Professional
			return Transaction837P
		}
	case "835":
		return Transaction835
	case "270":
		return Transaction270
	case "271":
		return Transaction271
	case "276":
		return Transaction276
	case "277":
		return Transaction277
	case "278":
		return Transaction278
	case "834":
		return Transaction834
	default:
		return TransactionUnknown
	}
}

// Warnings returns accumulated parse warnings
func (p *Parser) Warnings() []ParseWarning {
	return p.warnings
}
