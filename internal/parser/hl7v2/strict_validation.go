package hl7v2

import (
	"errors"
	"strings"
	"unicode/utf8"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/profile"
)

var (
	errStrictInvalidUTF8       = errors.New("strict validation requires valid UTF-8")
	errStrictLineEndings       = errors.New("strict validation requires CR segment terminators")
	errStrictDelimiterSyntax   = errors.New("strict validation requires a valid MSH delimiter declaration")
	errStrictStandardDelimiter = errors.New("strict validation requires standard MSH delimiters")
	errStrictMessageType       = errors.New("strict validation supports only ADT A01")
	errStrictA01Structure      = errors.New("strict validation rejected the A01 segment structure")
	errStrictUnknownSegment    = errors.New("strict validation rejected a segment outside the A01 v1 subset")
	errStrictNTEPlacement      = errors.New("strict validation rejected NTE placement")
	errStrictExtraComponents   = errors.New("strict validation rejected extra field components")
)

const standardMSHDelimiterPrefix = `MSH|^~\&`

// These limits cover only the components interpreted by the executable A01 v1
// mapper. Higher components from broader HL7 data types are unsupported in this
// bounded path rather than being silently ignored.
var strictA01FieldComponentLimits = [...]strictFieldComponentLimit{
	{segmentID: "MSH", field: 7, max: 2, path: "MSH.7"},
	{segmentID: "MSH", field: 9, max: 3, path: "MSH.9"},
	{segmentID: "MSH", field: 10, max: 1, path: "MSH.10"},
	{segmentID: "MSH", field: 12, max: 1, path: "MSH.12"},
	{segmentID: "EVN", field: 2, max: 2, path: "EVN.2"},
	{segmentID: "EVN", field: 6, max: 2, path: "EVN.6"},
	{segmentID: "PID", field: 3, max: 6, path: "PID.3"},
	{segmentID: "PID", field: 5, max: 5, path: "PID.5"},
	{segmentID: "PID", field: 7, max: 1, path: "PID.7"},
	{segmentID: "PID", field: 8, max: 1, path: "PID.8"},
	{segmentID: "PID", field: 11, max: 6, path: "PID.11"},
	{segmentID: "PID", field: 13, max: 1, path: "PID.13"},
	{segmentID: "PV1", field: 2, max: 1, path: "PV1.2"},
	{segmentID: "PV1", field: 3, max: 4, path: "PV1.3"},
	{segmentID: "PV1", field: 7, max: 3, path: "PV1.7"},
	{segmentID: "PV1", field: 19, max: 1, path: "PV1.19"},
	{segmentID: "PV1", field: 44, max: 2, path: "PV1.44"},
	{segmentID: "PV1", field: 45, max: 2, path: "PV1.45"},
}

type strictFieldComponentLimit struct {
	segmentID string
	field     int
	max       int
	path      string
}

func (p *Parser) validateStrictRawA01(raw string) error {
	if !utf8.ValidString(raw) {
		return errStrictInvalidUTF8
	}

	if strings.Contains(raw, "\n") {
		if !p.strictLineEndingsTolerated() {
			return errStrictLineEndings
		}
		p.addWarning(
			"byte",
			"NON_STANDARD_LINE_ENDING",
			"non-CR segment terminators were accepted by the source profile",
			"message",
		)
	}

	if !validMSHDelimiterDeclaration(raw) {
		return errStrictDelimiterSyntax
	}
	if !strings.HasPrefix(raw, standardMSHDelimiterPrefix) {
		if !p.strictToleranceEnabled(func(t *profile.ToleranceConfig) bool {
			return t.NonStandardDelimiters
		}) {
			return errStrictStandardDelimiter
		}
		p.addWarning(
			"syntactic",
			"NON_STANDARD_DELIMITERS",
			"non-standard delimiters were accepted by the source profile",
			"MSH.1-2",
		)
	}
	return nil
}

func (p *Parser) validateStrictParsedA01(msg *Message) error {
	if msg.Type != "ADT^A01" && msg.Type != "ADT^A01^ADT_A01" {
		return errStrictMessageType
	}
	if err := p.validateStrictA01Structure(msg); err != nil {
		return err
	}
	return p.validateStrictA01Components(msg)
}

func (p *Parser) validateStrictA01Structure(msg *Message) error {
	seenCore := make(map[string]bool, 4)
	lastCoreOrder := -1
	nteAnchor := ""

	for i := range msg.Segments {
		segmentID := msg.Segments[i].ID
		order, core := strictA01CoreSegmentOrder(segmentID)
		if core {
			if seenCore[segmentID] || order < lastCoreOrder {
				return errStrictA01Structure
			}
			seenCore[segmentID] = true
			lastCoreOrder = order
			nteAnchor = segmentID
			continue
		}

		if segmentID == "NTE" {
			if nteAnchor == "PID" || nteAnchor == "PV1" {
				continue
			}
			if !p.strictToleranceEnabled(func(t *profile.ToleranceConfig) bool {
				return t.NTEAnywhere
			}) {
				return errStrictNTEPlacement
			}
			p.addWarning(
				"syntactic",
				"NTE_OUT_OF_ORDER",
				"out-of-order NTE was accepted by the source profile",
				"NTE",
			)
			continue
		}

		if !p.strictToleranceEnabled(func(t *profile.ToleranceConfig) bool {
			return t.UnknownSegments
		}) {
			return errStrictUnknownSegment
		}
		p.addWarning(
			"syntactic",
			"UNKNOWN_SEGMENT",
			"a segment outside the A01 v1 subset was accepted by the source profile",
			"segment",
		)
		nteAnchor = ""
	}

	if !seenCore["MSH"] || !seenCore["PID"] {
		return errStrictA01Structure
	}
	if !seenCore["EVN"] {
		if !p.profile.IsMissingSegmentTolerated("EVN") {
			return errStrictA01Structure
		}
		p.addWarning(
			"semantic",
			"MISSING_EVN",
			"EVN segment was omitted under the source profile",
			"EVN",
		)
	}
	if !seenCore["PV1"] && !p.profile.IsMissingSegmentTolerated("PV1") {
		return errStrictA01Structure
	}
	return nil
}

// The executable A01 v1 subset is intentionally narrower than the complete
// ADT_A01 standard structure. It supports MSH -> EVN -> PID -> [PV1], plus NTE
// runs attached directly to PID or PV1. Everything else requires the explicit
// UnknownSegments tolerance and remains semantically ignored by this parser.
func strictA01CoreSegmentOrder(segmentID string) (int, bool) {
	switch segmentID {
	case "MSH":
		return 0, true
	case "EVN":
		return 1, true
	case "PID":
		return 2, true
	case "PV1":
		return 3, true
	default:
		return 0, false
	}
}

func (p *Parser) validateStrictA01Components(msg *Message) error {
	componentSeparator := string(msg.Delimiters.Component)
	repetitionSeparator := string(msg.Delimiters.Repetition)
	for _, limit := range strictA01FieldComponentLimits {
		segment := p.getSegment(msg, limit.segmentID)
		value := p.getField(segment, limit.field)
		if value == "" {
			continue
		}

		exceeded := false
		for _, repetition := range strings.Split(value, repetitionSeparator) {
			if strings.Count(repetition, componentSeparator)+1 > limit.max {
				exceeded = true
				break
			}
		}
		if !exceeded {
			continue
		}

		if !p.strictToleranceEnabled(func(t *profile.ToleranceConfig) bool {
			return t.ExtraComponents
		}) {
			return errStrictExtraComponents
		}
		p.addWarning(
			"syntactic",
			"EXTRA_COMPONENTS",
			"extra field components were accepted by the source profile",
			limit.path,
		)
	}
	return nil
}

func (p *Parser) strictLineEndingsTolerated() bool {
	return p.profile != nil &&
		p.profile.HL7v2 != nil &&
		p.profile.HL7v2.Encoding != nil &&
		p.profile.HL7v2.Encoding.LineEndingMode == "tolerant"
}

func (p *Parser) strictToleranceEnabled(enabled func(*profile.ToleranceConfig) bool) bool {
	return p.profile != nil &&
		p.profile.HL7v2 != nil &&
		p.profile.HL7v2.Tolerate != nil &&
		enabled(p.profile.HL7v2.Tolerate)
}

func validMSHDelimiterDeclaration(raw string) bool {
	if len(raw) < 9 || !strings.HasPrefix(raw, "MSH") || raw[8] != raw[3] {
		return false
	}

	seen := make(map[byte]struct{}, 5)
	for _, delimiter := range []byte{raw[3], raw[4], raw[5], raw[6], raw[7]} {
		if delimiter <= ' ' || delimiter >= 0x7f || asciiLetterOrDigit(delimiter) {
			return false
		}
		if _, duplicate := seen[delimiter]; duplicate {
			return false
		}
		seen[delimiter] = struct{}{}
	}
	return true
}

func asciiLetterOrDigit(value byte) bool {
	return value >= '0' && value <= '9' ||
		value >= 'A' && value <= 'Z' ||
		value >= 'a' && value <= 'z'
}
