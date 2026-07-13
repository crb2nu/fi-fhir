package hl7v2

import (
	"errors"
	"strings"
	"time"
)

const minimumEventTimePrecision = 8

var errInvalidHL7DTM = errors.New("invalid HL7 date/time")

// parsedHL7DTM preserves the source precision and whether the value supplied
// its own offset. time.Time alone cannot retain either distinction.
type parsedHL7DTM struct {
	Time      time.Time
	Precision int
	// ExplicitZone records +/-HHMM syntax. time.Time intentionally collapses
	// the HL7 v2.9 +0000/-0000 distinction to the same UTC instant.
	ExplicitZone bool
}

// parseHL7DTM parses the complete HL7 DTM grammar and the legacy TS precision
// component. It deliberately returns a fixed error that cannot echo PHI.
func parseHL7DTM(raw string, componentSeparator byte, defaultLocation *time.Location) (parsedHL7DTM, error) {
	if raw == "" {
		return parsedHL7DTM{}, errInvalidHL7DTM
	}
	if componentSeparator == 0 {
		componentSeparator = DefaultDelimiters().Component
	}
	if defaultLocation == nil {
		defaultLocation = time.UTC
	}

	value := raw
	legacyPrecision := 0
	components := strings.Split(value, string(componentSeparator))
	if len(components) > 2 {
		return parsedHL7DTM{}, errInvalidHL7DTM
	}
	if len(components) == 2 {
		var ok bool
		legacyPrecision, ok = hl7TSPrecision(components[1])
		if !ok {
			return parsedHL7DTM{}, errInvalidHL7DTM
		}
		value = components[0]
	}

	offset := ""
	explicitZone := false
	if len(value) >= 5 {
		offsetStart := len(value) - 5
		if value[offsetStart] == '+' || value[offsetStart] == '-' {
			offset = value[offsetStart:]
			value = value[:offsetStart]
			explicitZone = true
			if !validHL7Offset(offset) {
				return parsedHL7DTM{}, errInvalidHL7DTM
			}
		}
	}

	base := value
	fraction := ""
	if dot := strings.IndexByte(value, '.'); dot >= 0 {
		if dot != strings.LastIndexByte(value, '.') {
			return parsedHL7DTM{}, errInvalidHL7DTM
		}
		base = value[:dot]
		fraction = value[dot+1:]
		if len(base) != 14 || len(fraction) < 1 || len(fraction) > 4 || !asciiDigits(fraction) {
			return parsedHL7DTM{}, errInvalidHL7DTM
		}
	}

	layout, ok := hl7DTMLayout(len(base))
	if !ok || !asciiDigits(base) {
		return parsedHL7DTM{}, errInvalidHL7DTM
	}

	basePrecision := len(base)
	precision := basePrecision
	if fraction != "" {
		layout += "." + strings.Repeat("0", len(fraction))
		base += "." + fraction
		precision += 1 + len(fraction)
	}
	if legacyPrecision != 0 {
		if legacyPrecision > basePrecision {
			return parsedHL7DTM{}, errInvalidHL7DTM
		}
		precision = legacyPrecision
	}

	var parsed time.Time
	var err error
	if explicitZone {
		parsed, err = time.ParseInLocation(layout, base, time.FixedZone("", hl7OffsetSeconds(offset)))
	} else {
		parsed, err = time.ParseInLocation(layout, base, defaultLocation)
	}
	if err != nil {
		return parsedHL7DTM{}, errInvalidHL7DTM
	}
	if legacyPrecision != 0 {
		parsed = normalizeHL7DTMPrecision(parsed, legacyPrecision)
	}

	return parsedHL7DTM{
		Time:         parsed,
		Precision:    precision,
		ExplicitZone: explicitZone,
	}, nil
}

func hl7OffsetSeconds(value string) int {
	hours := int(value[1]-'0')*10 + int(value[2]-'0')
	minutes := int(value[3]-'0')*10 + int(value[4]-'0')
	seconds := (hours*60 + minutes) * 60
	if value[0] == '-' {
		return -seconds
	}
	return seconds
}

func normalizeHL7DTMPrecision(value time.Time, precision int) time.Time {
	year, month, day := value.Date()
	hour, minute, second := value.Clock()
	location := value.Location()
	switch precision {
	case 4:
		return time.Date(year, time.January, 1, 0, 0, 0, 0, location)
	case 6:
		return time.Date(year, month, 1, 0, 0, 0, 0, location)
	case 8:
		return time.Date(year, month, day, 0, 0, 0, 0, location)
	case 10:
		return time.Date(year, month, day, hour, 0, 0, 0, location)
	case 12:
		return time.Date(year, month, day, hour, minute, 0, 0, location)
	case 14:
		return time.Date(year, month, day, hour, minute, second, 0, location)
	default:
		return value
	}
}

func hl7DTMLayout(length int) (string, bool) {
	switch length {
	case 4:
		return "2006", true
	case 6:
		return "200601", true
	case 8:
		return "20060102", true
	case 10:
		return "2006010215", true
	case 12:
		return "200601021504", true
	case 14:
		return "20060102150405", true
	default:
		return "", false
	}
}

func hl7TSPrecision(value string) (int, bool) {
	switch value {
	case "Y":
		return 4, true
	case "L":
		return 6, true
	case "D":
		return 8, true
	case "H":
		return 10, true
	case "M":
		return 12, true
	case "S":
		return 14, true
	default:
		return 0, false
	}
}

func validHL7Offset(value string) bool {
	if len(value) != 5 || (value[0] != '+' && value[0] != '-') || !asciiDigits(value[1:]) {
		return false
	}
	hours := int(value[1]-'0')*10 + int(value[2]-'0')
	minutes := int(value[3]-'0')*10 + int(value[4]-'0')
	return minutes <= 59 && (hours < 14 || (hours == 14 && minutes == 0))
}

func asciiDigits(value string) bool {
	if value == "" {
		return false
	}
	for i := range len(value) {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

type a01SourceTimes struct {
	OccurredAt        time.Time
	AdmitDateTime     time.Time
	DischargeDateTime time.Time
}

type a01DTMCandidate struct {
	parsed parsedHL7DTM
	valid  bool
	usable bool
}

func (p *Parser) extractADTA01SourceTimes(msg *Message) a01SourceTimes {
	messageLocation := p.config.DefaultTimezone
	if messageLocation == nil {
		messageLocation = time.UTC
	}

	msh := p.getSegment(msg, "MSH")
	msh7 := p.parseA01DTMCandidate(
		p.getField(msh, 7), "MSH.7", msg.Delimiters.Component, messageLocation,
	)
	if msh7.valid && msh7.parsed.ExplicitZone {
		_, offsetSeconds := msh7.parsed.Time.Zone()
		messageLocation = time.FixedZone("", offsetSeconds)
	}

	evn := p.getSegment(msg, "EVN")
	pv1 := p.getSegment(msg, "PV1")
	evn6 := p.parseA01DTMCandidate(
		p.getField(evn, 6), "EVN.6", msg.Delimiters.Component, messageLocation,
	)
	pv1Field44 := p.parseA01DTMCandidate(
		p.getField(pv1, 44), "PV1.44", msg.Delimiters.Component, messageLocation,
	)
	pv1Field45 := p.parseA01DTMCandidate(
		p.getField(pv1, 45), "PV1.45", msg.Delimiters.Component, messageLocation,
	)
	evn2 := p.parseA01DTMCandidate(
		p.getField(evn, 2), "EVN.2", msg.Delimiters.Component, messageLocation,
	)

	times := a01SourceTimes{}
	if pv1Field44.usable {
		times.AdmitDateTime = pv1Field44.parsed.Time
	}
	if pv1Field45.usable {
		times.DischargeDateTime = pv1Field45.parsed.Time
	}
	for _, candidate := range []a01DTMCandidate{evn6, pv1Field44, evn2, msh7} {
		if candidate.usable {
			times.OccurredAt = candidate.parsed.Time
			break
		}
	}
	return times
}

func (p *Parser) parseA01DTMCandidate(
	raw string,
	path string,
	componentSeparator byte,
	location *time.Location,
) a01DTMCandidate {
	if raw == "" {
		return a01DTMCandidate{}
	}
	parsed, err := parseHL7DTM(raw, componentSeparator, location)
	if err != nil {
		p.addWarning("semantic", "INVALID_DTM", "HL7 date/time value is invalid", path)
		return a01DTMCandidate{}
	}
	candidate := a01DTMCandidate{parsed: parsed, valid: true}
	if parsed.Precision < minimumEventTimePrecision {
		p.addWarning(
			"semantic",
			"IMPRECISE_EVENT_TIME",
			"HL7 date/time is too imprecise for an event timestamp",
			path,
		)
		return candidate
	}
	candidate.usable = true
	return candidate
}
