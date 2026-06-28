package session

import (
	"fmt"
	"strings"
)

type hl7Segment struct {
	id     string
	fields []string
	index  int
}

func BuildHL7v2Lineage(raw string, event any) []LineageLink {
	segments := parseHL7Segments(raw)
	links := make([]LineageLink, 0)
	add := func(segmentID string, occurrence int, field int, target string) {
		seg, ok := findSegment(segments, segmentID, occurrence)
		if !ok || field >= len(seg.fields) {
			return
		}
		value := seg.fields[field]
		if value == "" {
			return
		}
		sourcePath := fmt.Sprintf("%s.%d", segmentID, field)
		if occurrence > 0 {
			sourcePath = fmt.Sprintf("%s[%d].%d", segmentID, occurrence+1, field)
		}
		links = append(links, LineageLink{
			SourcePath:    sourcePath,
			SourceSegment: segmentID,
			SourceField:   field,
			TargetPath:    target,
			ValuePreview:  previewValue(sourcePath, value),
		})
	}

	add("MSH", 0, 9, "event.type")
	add("MSH", 0, 10, "event.source_message_id")
	add("MSH", 0, 7, "event.timestamp")
	add("PID", 0, 3, "event.patient.identifiers")
	add("PID", 0, 5, "event.patient.name")
	add("PID", 0, 7, "event.patient.birth_date")
	add("PID", 0, 8, "event.patient.gender")
	add("PV1", 0, 2, "event.encounter.class")
	add("PV1", 0, 3, "event.encounter.location")
	add("PV1", 0, 19, "event.encounter.id")
	add("OBR", 0, 4, "event.test.panel")

	obxOccurrence := 0
	for _, seg := range segments {
		if seg.id != "OBX" {
			continue
		}
		add("OBX", obxOccurrence, 3, fmt.Sprintf("event.results[%d].test", obxOccurrence))
		add("OBX", obxOccurrence, 5, fmt.Sprintf("event.results[%d].result", obxOccurrence))
		obxOccurrence++
	}

	return links
}

func parseHL7Segments(raw string) []hl7Segment {
	normalized := strings.ReplaceAll(raw, "\r\n", "\r")
	normalized = strings.ReplaceAll(normalized, "\n", "\r")
	lines := strings.Split(strings.TrimSpace(normalized), "\r")
	segments := make([]hl7Segment, 0, len(lines))

	fieldSep := "|"
	if len(lines) > 0 && strings.HasPrefix(lines[0], "MSH") && len(lines[0]) > 3 {
		fieldSep = string(lines[0][3])
	}
	for _, line := range lines {
		if line == "" {
			continue
		}
		var fields []string
		if strings.HasPrefix(line, "MSH") {
			fields = append([]string{"MSH", fieldSep}, strings.Split(line[4:], fieldSep)...)
		} else {
			fields = strings.Split(line, fieldSep)
		}
		if len(fields) == 0 || fields[0] == "" {
			continue
		}
		segments = append(segments, hl7Segment{id: fields[0], fields: fields, index: len(segments)})
	}
	return segments
}

func findSegment(segments []hl7Segment, segmentID string, occurrence int) (hl7Segment, bool) {
	seen := 0
	for _, seg := range segments {
		if seg.id != segmentID {
			continue
		}
		if seen == occurrence {
			return seg, true
		}
		seen++
	}
	return hl7Segment{}, false
}

func previewValue(path, value string) string {
	for _, sensitive := range []string{"PID.3", "PID.5", "PID.7", "PID.11", "PID.13", "PID.19"} {
		if path == sensitive {
			return "[redacted]"
		}
	}
	value = strings.TrimSpace(value)
	if len(value) <= 80 {
		return value
	}
	return value[:77] + "..."
}
