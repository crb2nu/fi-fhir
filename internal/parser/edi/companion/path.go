package companion

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/crb2nu/fi-fhir/internal/parser/edi"
)

// PathResolver resolves X12 dot-notation paths to segment element values.
//
// Path syntax:
//   - "CLM.01" - Element 1 of first CLM segment
//   - "CLM.05-1" - Component 1 of element 5 in CLM
//   - "2010AA.NM1.09" - Element 9 of NM1 in loop 2010AA
//   - "2400[*].SV1.01" - Element 1 of all SV1 segments in 2400 loops
//   - "NM1[2].01" - Element 1 of the second NM1 segment
type PathResolver struct {
	transaction *edi.Transaction
	delimiters  edi.Delimiters
	loopStruct  interface{} // Parsed loop structure (Loop837Structure, Loop835Structure, etc.)
}

// PathComponent represents a parsed component of an X12 path.
type PathComponent struct {
	Loop         string // Loop identifier (e.g., "2010AA") - optional
	Segment      string // Segment ID (e.g., "NM1")
	SegmentIndex int    // Segment occurrence index (0-based), -1 for all
	Element      int    // Element number (1-based)
	Component    int    // Component number (1-based), 0 if not specified
	ComponentSep byte   // Component separator
}

// NewPathResolver creates a new path resolver for a transaction.
func NewPathResolver(tx *edi.Transaction, delimiters edi.Delimiters) *PathResolver {
	return &PathResolver{
		transaction: tx,
		delimiters:  delimiters,
	}
}

// NewPathResolverWithLoops creates a path resolver with a pre-parsed loop structure.
func NewPathResolverWithLoops(tx *edi.Transaction, delimiters edi.Delimiters, loopStruct interface{}) *PathResolver {
	return &PathResolver{
		transaction: tx,
		delimiters:  delimiters,
		loopStruct:  loopStruct,
	}
}

// Resolve returns the value at the specified path.
// Returns empty string if the path doesn't exist.
func (r *PathResolver) Resolve(path string) string {
	values := r.ResolveAll(path)
	if len(values) > 0 {
		return values[0]
	}
	return ""
}

// ResolveAll returns all values matching the path.
// This is useful for paths that match multiple segments (e.g., "2400[*].SV1.01").
func (r *PathResolver) ResolveAll(path string) []string {
	parsed, err := ParsePath(path)
	if err != nil {
		return nil
	}

	segments := r.findSegments(parsed)
	if len(segments) == 0 {
		return nil
	}

	var values []string
	for _, seg := range segments {
		value := r.extractValue(seg, parsed)
		if value != "" {
			values = append(values, value)
		}
	}
	return values
}

// Exists returns true if the path exists and has a non-empty value.
func (r *PathResolver) Exists(path string) bool {
	return r.Resolve(path) != ""
}

// GetSegments returns all segments matching the path's loop and segment criteria.
func (r *PathResolver) GetSegments(path string) []*edi.Segment {
	parsed, err := ParsePath(path)
	if err != nil {
		return nil
	}
	return r.findSegments(parsed)
}

// ParsePath parses an X12 path string into components.
func ParsePath(path string) (*PathComponent, error) {
	if path == "" {
		return nil, fmt.Errorf("empty path")
	}

	pc := &PathComponent{
		SegmentIndex: -1,  // Default: first occurrence
		ComponentSep: ':', // Default X12 component separator
	}

	// Split by dots
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid path format: %s (expected at least segment.element)", path)
	}

	var segmentPart string
	var elementPart string

	// Determine if first part is a loop or segment
	if len(parts) == 2 {
		// Format: SEGMENT.ELEMENT or LOOP.SEGMENT (need to check)
		// If first part looks like a loop (numeric prefix), treat as loop.segment
		if looksLikeLoop(parts[0]) {
			return nil, fmt.Errorf("invalid path format: %s (loop paths require segment.element)", path)
		}
		segmentPart = parts[0]
		elementPart = parts[1]
	} else if len(parts) == 3 {
		// Format: LOOP.SEGMENT.ELEMENT
		pc.Loop = parts[0]
		segmentPart = parts[1]
		elementPart = parts[2]
	} else {
		return nil, fmt.Errorf("invalid path format: %s (too many parts)", path)
	}

	// Parse segment part (may include index like NM1[2] or NM1[*])
	segment, index, err := parseSegmentWithIndex(segmentPart)
	if err != nil {
		return nil, err
	}
	pc.Segment = segment
	pc.SegmentIndex = index

	// Parse element part (may include component like 05-1)
	element, component, err := parseElementWithComponent(elementPart)
	if err != nil {
		return nil, err
	}
	pc.Element = element
	pc.Component = component

	return pc, nil
}

// looksLikeLoop returns true if the string looks like a loop identifier.
// Loop identifiers typically start with a digit (e.g., "2010AA", "2300", "1000A").
func looksLikeLoop(s string) bool {
	if len(s) == 0 {
		return false
	}
	// Loops typically start with a digit
	return s[0] >= '0' && s[0] <= '9'
}

// parseSegmentWithIndex parses a segment specifier like "NM1", "NM1[2]", or "NM1[*]".
func parseSegmentWithIndex(s string) (segment string, index int, err error) {
	// Check for index notation
	if !strings.Contains(s, "[") {
		return s, -1, nil // -1 means "first occurrence"
	}

	// Parse segment[index] format
	re := regexp.MustCompile(`^([A-Z0-9]+)\[(\*|\d+)\]$`)
	matches := re.FindStringSubmatch(s)
	if matches == nil {
		return "", 0, fmt.Errorf("invalid segment format: %s", s)
	}

	segment = matches[1]
	indexStr := matches[2]

	if indexStr == "*" {
		return segment, -2, nil // -2 means "all occurrences"
	}

	idx, err := strconv.Atoi(indexStr)
	if err != nil {
		return "", 0, fmt.Errorf("invalid segment index: %s", indexStr)
	}
	return segment, idx - 1, nil // Convert to 0-based
}

// parseElementWithComponent parses an element specifier like "01", "05-1".
func parseElementWithComponent(s string) (element int, component int, err error) {
	// Check for component notation
	if strings.Contains(s, "-") {
		parts := strings.SplitN(s, "-", 2)
		element, err = strconv.Atoi(parts[0])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid element number: %s", parts[0])
		}
		component, err = strconv.Atoi(parts[1])
		if err != nil {
			return 0, 0, fmt.Errorf("invalid component number: %s", parts[1])
		}
		return element, component, nil
	}

	// Just element number
	element, err = strconv.Atoi(s)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid element number: %s", s)
	}
	return element, 0, nil
}

// findSegments finds all segments matching the path criteria.
func (r *PathResolver) findSegments(pc *PathComponent) []*edi.Segment {
	if pc.Loop != "" {
		return r.findSegmentsInLoop(pc)
	}
	return r.findSegmentsInTransaction(pc)
}

// findSegmentsInTransaction finds segments directly in the transaction.
func (r *PathResolver) findSegmentsInTransaction(pc *PathComponent) []*edi.Segment {
	allSegs := r.transaction.GetSegments(pc.Segment)
	if len(allSegs) == 0 {
		return nil
	}

	switch pc.SegmentIndex {
	case -2: // All occurrences
		return allSegs
	case -1: // First occurrence
		return []*edi.Segment{allSegs[0]}
	default: // Specific index
		if pc.SegmentIndex >= len(allSegs) {
			return nil
		}
		return []*edi.Segment{allSegs[pc.SegmentIndex]}
	}
}

// findSegmentsInLoop finds segments within a specific loop using the loop structure.
func (r *PathResolver) findSegmentsInLoop(pc *PathComponent) []*edi.Segment {
	// Use pre-parsed loop structure if available
	if r.loopStruct != nil {
		return r.findInParsedLoops(pc)
	}

	// Fall back to manual loop identification based on state machine
	return r.findInTransactionByLoop(pc)
}

// findInParsedLoops finds segments using the parsed loop structure.
func (r *PathResolver) findInParsedLoops(pc *PathComponent) []*edi.Segment {
	// Handle different loop structure types
	switch ls := r.loopStruct.(type) {
	case *edi.Loop837Structure:
		return r.find837LoopSegments(ls, pc)
	case *edi.Loop835Structure:
		return r.find835LoopSegments(ls, pc)
	case *edi.Loop270Structure:
		return r.find270LoopSegments(ls, pc)
	case *edi.Loop271Structure:
		return r.find271LoopSegments(ls, pc)
	default:
		// Unknown structure, fall back to transaction search
		return r.findInTransactionByLoop(pc)
	}
}

// find837LoopSegments finds segments in an 837 loop structure.
func (r *PathResolver) find837LoopSegments(ls *edi.Loop837Structure, pc *PathComponent) []*edi.Segment {
	var segments []*edi.Segment

	loopID := pc.Loop
	segID := pc.Segment

	switch {
	case loopID == "1000A":
		if ls.Submitter != nil {
			segments = appendIfMatch(segments, ls.Submitter.NM1, segID, "NM1")
			segments = appendAllIfMatch(segments, ls.Submitter.PER, segID, "PER")
		}
	case loopID == "1000B":
		if ls.Receiver != nil {
			segments = appendIfMatch(segments, ls.Receiver.NM1, segID, "NM1")
			segments = appendAllIfMatch(segments, ls.Receiver.PER, segID, "PER")
		}
	case strings.HasPrefix(loopID, "2010AA"):
		for _, bp := range ls.BillingProviders {
			if bp.BillingName != nil {
				segments = append837Loop2010Segments(segments, bp.BillingName, segID)
			}
		}
	case strings.HasPrefix(loopID, "2010AB"):
		for _, bp := range ls.BillingProviders {
			if bp.PayToAddress != nil {
				segments = append837Loop2010Segments(segments, bp.PayToAddress, segID)
			}
		}
	case strings.HasPrefix(loopID, "2010BA"):
		for _, bp := range ls.BillingProviders {
			for _, sub := range bp.Subscribers {
				if sub.SubscriberInfo != nil {
					segments = append837Loop2010Segments(segments, sub.SubscriberInfo, segID)
				}
			}
		}
	case strings.HasPrefix(loopID, "2010BB"):
		for _, bp := range ls.BillingProviders {
			for _, sub := range bp.Subscribers {
				if sub.PayerInfo != nil {
					segments = append837Loop2010Segments(segments, sub.PayerInfo, segID)
				}
			}
		}
	case loopID == "2000A":
		for _, bp := range ls.BillingProviders {
			segments = appendIfMatch(segments, bp.HL, segID, "HL")
			segments = appendIfMatch(segments, bp.PRV, segID, "PRV")
		}
	case loopID == "2000B":
		for _, bp := range ls.BillingProviders {
			for _, sub := range bp.Subscribers {
				segments = appendIfMatch(segments, sub.HL, segID, "HL")
				segments = appendIfMatch(segments, sub.SBR, segID, "SBR")
			}
		}
	case loopID == "2300":
		for _, bp := range ls.BillingProviders {
			for _, sub := range bp.Subscribers {
				for _, claim := range sub.Claims {
					segments = append837ClaimSegments(segments, claim, segID)
				}
			}
		}
	case loopID == "2400":
		for _, bp := range ls.BillingProviders {
			for _, sub := range bp.Subscribers {
				for _, claim := range sub.Claims {
					for _, svc := range claim.ServiceLines {
						segments = append837ServiceSegments(segments, svc, segID)
					}
				}
			}
		}
	}

	return filterByIndex(segments, pc.SegmentIndex)
}

// find835LoopSegments finds segments in an 835 loop structure.
func (r *PathResolver) find835LoopSegments(ls *edi.Loop835Structure, pc *PathComponent) []*edi.Segment {
	var segments []*edi.Segment

	loopID := pc.Loop
	segID := pc.Segment

	switch loopID {
	case "1000A":
		if ls.Payer != nil {
			segments = appendIfMatch(segments, ls.Payer.NM1, segID, "NM1")
		}
	case "1000B":
		if ls.Payee != nil {
			segments = appendIfMatch(segments, ls.Payee.NM1, segID, "NM1")
		}
	case "2000":
		for _, hdr := range ls.Headers {
			segments = appendIfMatch(segments, hdr.LX, segID, "LX")
			segments = appendIfMatch(segments, hdr.TS3, segID, "TS3")
			segments = appendIfMatch(segments, hdr.TS2, segID, "TS2")
		}
	case "2100":
		for _, hdr := range ls.Headers {
			for _, claim := range hdr.Claims {
				segments = append835ClaimSegments(segments, claim, segID)
			}
		}
	case "2110":
		for _, hdr := range ls.Headers {
			for _, claim := range hdr.Claims {
				for _, svc := range claim.ServiceLines {
					segments = append835ServiceSegments(segments, svc, segID)
				}
			}
		}
	}

	return filterByIndex(segments, pc.SegmentIndex)
}

// find270LoopSegments finds segments in a 270 loop structure.
func (r *PathResolver) find270LoopSegments(ls *edi.Loop270Structure, pc *PathComponent) []*edi.Segment {
	var segments []*edi.Segment
	loopID := pc.Loop
	segID := pc.Segment

	switch loopID {
	case "2000A":
		for _, src := range ls.InformationSources {
			segments = appendIfMatch(segments, src.HL, segID, "HL")
		}
	case "2100A":
		for _, src := range ls.InformationSources {
			if src.SourceInfo != nil {
				segments = append270EntitySegments(segments, src.SourceInfo, segID)
			}
		}
	case "2000B":
		for _, src := range ls.InformationSources {
			for _, rcv := range src.Receivers {
				segments = appendIfMatch(segments, rcv.HL, segID, "HL")
			}
		}
	case "2100B":
		for _, src := range ls.InformationSources {
			for _, rcv := range src.Receivers {
				if rcv.ReceiverInfo != nil {
					segments = append270EntitySegments(segments, rcv.ReceiverInfo, segID)
				}
			}
		}
	case "2000C":
		for _, src := range ls.InformationSources {
			for _, rcv := range src.Receivers {
				for _, sub := range rcv.Subscribers {
					segments = appendIfMatch(segments, sub.HL, segID, "HL")
				}
			}
		}
	case "2100C":
		for _, src := range ls.InformationSources {
			for _, rcv := range src.Receivers {
				for _, sub := range rcv.Subscribers {
					if sub.SubscriberInfo != nil {
						segments = append270EntitySegments(segments, sub.SubscriberInfo, segID)
					}
				}
			}
		}
	}

	return filterByIndex(segments, pc.SegmentIndex)
}

// find271LoopSegments finds segments in a 271 loop structure.
func (r *PathResolver) find271LoopSegments(ls *edi.Loop271Structure, pc *PathComponent) []*edi.Segment {
	var segments []*edi.Segment
	loopID := pc.Loop
	segID := pc.Segment

	switch loopID {
	case "2000A":
		for _, src := range ls.InformationSources {
			segments = appendIfMatch(segments, src.HL, segID, "HL")
		}
	case "2100A":
		for _, src := range ls.InformationSources {
			if src.SourceInfo != nil {
				segments = append271EntitySegments(segments, src.SourceInfo, segID)
			}
		}
	case "2000C":
		for _, src := range ls.InformationSources {
			for _, rcv := range src.Receivers {
				for _, sub := range rcv.Subscribers {
					segments = appendIfMatch(segments, sub.HL, segID, "HL")
				}
			}
		}
	case "2100C":
		for _, src := range ls.InformationSources {
			for _, rcv := range src.Receivers {
				for _, sub := range rcv.Subscribers {
					if sub.SubscriberInfo != nil {
						segments = append271EntitySegments(segments, sub.SubscriberInfo, segID)
					}
				}
			}
		}
	}

	return filterByIndex(segments, pc.SegmentIndex)
}

// findInTransactionByLoop is a fallback that finds segments by analyzing loop context.
// This is less accurate than using pre-parsed loop structures.
func (r *PathResolver) findInTransactionByLoop(pc *PathComponent) []*edi.Segment {
	// Simple fallback: find all segments with the given ID
	// In a full implementation, this would track loop state
	return r.findSegmentsInTransaction(pc)
}

// extractValue extracts the element/component value from a segment.
func (r *PathResolver) extractValue(seg *edi.Segment, pc *PathComponent) string {
	if pc.Component > 0 {
		return seg.GetComponent(pc.Element, pc.Component, r.delimiters.Subelement)
	}
	return seg.GetElement(pc.Element)
}

// Helper functions for building segment lists

func appendIfMatch(segments []*edi.Segment, seg *edi.Segment, target, actual string) []*edi.Segment {
	if seg != nil && target == actual {
		return append(segments, seg)
	}
	return segments
}

func appendAllIfMatch(segments []*edi.Segment, segs []*edi.Segment, target, actual string) []*edi.Segment {
	if target == actual {
		return append(segments, segs...)
	}
	return segments
}

func filterByIndex(segments []*edi.Segment, index int) []*edi.Segment {
	if len(segments) == 0 {
		return nil
	}

	switch index {
	case -2: // All
		return segments
	case -1: // First
		return []*edi.Segment{segments[0]}
	default:
		if index >= len(segments) {
			return nil
		}
		return []*edi.Segment{segments[index]}
	}
}

// 837-specific segment helpers

func append837Loop2010Segments(segments []*edi.Segment, l *edi.Loop2010, segID string) []*edi.Segment {
	segments = appendIfMatch(segments, l.NM1, segID, "NM1")
	segments = appendIfMatch(segments, l.N3, segID, "N3")
	segments = appendIfMatch(segments, l.N4, segID, "N4")
	segments = appendIfMatch(segments, l.DMG, segID, "DMG")
	segments = appendAllIfMatch(segments, l.REF, segID, "REF")
	segments = appendAllIfMatch(segments, l.PER, segID, "PER")
	return segments
}

func append837ClaimSegments(segments []*edi.Segment, c *edi.Loop2300, segID string) []*edi.Segment {
	segments = appendIfMatch(segments, c.CLM, segID, "CLM")
	segments = appendIfMatch(segments, c.CL1, segID, "CL1")
	segments = appendIfMatch(segments, c.CN1, segID, "CN1")
	segments = appendIfMatch(segments, c.CR1, segID, "CR1")
	segments = appendIfMatch(segments, c.CR2, segID, "CR2")
	segments = appendIfMatch(segments, c.HCP, segID, "HCP")
	segments = appendAllIfMatch(segments, c.DTP, segID, "DTP")
	segments = appendAllIfMatch(segments, c.PWK, segID, "PWK")
	segments = appendAllIfMatch(segments, c.AMT, segID, "AMT")
	segments = appendAllIfMatch(segments, c.REF, segID, "REF")
	segments = appendAllIfMatch(segments, c.K3, segID, "K3")
	segments = appendAllIfMatch(segments, c.NTE, segID, "NTE")
	segments = appendAllIfMatch(segments, c.CRC, segID, "CRC")
	segments = appendAllIfMatch(segments, c.HI, segID, "HI")
	return segments
}

func append837ServiceSegments(segments []*edi.Segment, s *edi.Loop2400, segID string) []*edi.Segment {
	segments = appendIfMatch(segments, s.LX, segID, "LX")
	segments = appendIfMatch(segments, s.SV1, segID, "SV1")
	segments = appendIfMatch(segments, s.SV2, segID, "SV2")
	segments = appendIfMatch(segments, s.SV3, segID, "SV3")
	segments = appendIfMatch(segments, s.PS1, segID, "PS1")
	segments = appendIfMatch(segments, s.HCP, segID, "HCP")
	segments = appendAllIfMatch(segments, s.DTP, segID, "DTP")
	segments = appendAllIfMatch(segments, s.PWK, segID, "PWK")
	segments = appendAllIfMatch(segments, s.REF, segID, "REF")
	segments = appendAllIfMatch(segments, s.AMT, segID, "AMT")
	segments = appendAllIfMatch(segments, s.K3, segID, "K3")
	segments = appendAllIfMatch(segments, s.NTE, segID, "NTE")
	return segments
}

// 835-specific segment helpers

func append835ClaimSegments(segments []*edi.Segment, c *edi.Loop835Claim, segID string) []*edi.Segment {
	segments = appendIfMatch(segments, c.CLP, segID, "CLP")
	segments = appendIfMatch(segments, c.MIA, segID, "MIA")
	segments = appendIfMatch(segments, c.MOA, segID, "MOA")
	segments = appendAllIfMatch(segments, c.CAS, segID, "CAS")
	segments = appendAllIfMatch(segments, c.NM1, segID, "NM1")
	segments = appendAllIfMatch(segments, c.REF, segID, "REF")
	segments = appendAllIfMatch(segments, c.DTM, segID, "DTM")
	segments = appendAllIfMatch(segments, c.PER, segID, "PER")
	segments = appendAllIfMatch(segments, c.AMT, segID, "AMT")
	segments = appendAllIfMatch(segments, c.QTY, segID, "QTY")
	return segments
}

func append835ServiceSegments(segments []*edi.Segment, s *edi.Loop835Service, segID string) []*edi.Segment {
	segments = appendIfMatch(segments, s.SVC, segID, "SVC")
	segments = appendAllIfMatch(segments, s.DTM, segID, "DTM")
	segments = appendAllIfMatch(segments, s.CAS, segID, "CAS")
	segments = appendAllIfMatch(segments, s.REF, segID, "REF")
	segments = appendAllIfMatch(segments, s.AMT, segID, "AMT")
	segments = appendAllIfMatch(segments, s.QTY, segID, "QTY")
	segments = appendAllIfMatch(segments, s.LQ, segID, "LQ")
	return segments
}

// 270-specific segment helpers

func append270EntitySegments(segments []*edi.Segment, e *edi.Loop270Entity, segID string) []*edi.Segment {
	segments = appendIfMatch(segments, e.NM1, segID, "NM1")
	segments = appendIfMatch(segments, e.N3, segID, "N3")
	segments = appendIfMatch(segments, e.N4, segID, "N4")
	segments = appendIfMatch(segments, e.DMG, segID, "DMG")
	segments = appendIfMatch(segments, e.INS, segID, "INS")
	segments = appendAllIfMatch(segments, e.REF, segID, "REF")
	segments = appendAllIfMatch(segments, e.PER, segID, "PER")
	segments = appendAllIfMatch(segments, e.HI, segID, "HI")
	segments = appendAllIfMatch(segments, e.DTP, segID, "DTP")
	return segments
}

// 271-specific segment helpers

func append271EntitySegments(segments []*edi.Segment, e *edi.Loop271Entity, segID string) []*edi.Segment {
	segments = appendIfMatch(segments, e.NM1, segID, "NM1")
	segments = appendIfMatch(segments, e.N3, segID, "N3")
	segments = appendIfMatch(segments, e.N4, segID, "N4")
	segments = appendIfMatch(segments, e.DMG, segID, "DMG")
	segments = appendIfMatch(segments, e.INS, segID, "INS")
	segments = appendAllIfMatch(segments, e.REF, segID, "REF")
	segments = appendAllIfMatch(segments, e.PER, segID, "PER")
	segments = appendAllIfMatch(segments, e.AAA, segID, "AAA")
	segments = appendAllIfMatch(segments, e.HI, segID, "HI")
	segments = appendAllIfMatch(segments, e.DTP, segID, "DTP")
	return segments
}
