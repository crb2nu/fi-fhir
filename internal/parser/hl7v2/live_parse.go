package hl7v2

import (
	"fmt"
	"strings"
	"time"
)

// ParseEvent represents a single parse event emitted during live parsing.
type ParseEvent struct {
	SegmentIndex int                    `json:"segmentIndex"`
	SegmentType  string                 `json:"segmentType"`
	RawSegment   string                 `json:"rawSegment"`
	Fields       map[string]interface{} `json:"fields"`
	Warnings     []string               `json:"warnings"`
	Timestamp    time.Time              `json:"timestamp"`
	IsComplete   bool                   `json:"isComplete"`
}

// LiveParser wraps the existing parser for streaming segment-by-segment parse events.
type LiveParser struct {
	parser *Parser
}

// NewLiveParser creates a new LiveParser wrapping the given Parser.
func NewLiveParser(parser *Parser) *LiveParser {
	return &LiveParser{parser: parser}
}

// ParseStream splits an HL7v2 message into segments and parses each one,
// emitting ParseEvent objects to the provided channel.
func (lp *LiveParser) ParseStream(message string, events chan<- ParseEvent) {
	defer close(events)

	// Normalize line endings
	message = strings.ReplaceAll(message, "\r\n", "\r")
	message = strings.ReplaceAll(message, "\n", "\r")
	segments := strings.Split(message, "\r")

	// Count non-empty segments for IsComplete tracking
	nonEmpty := make([]string, 0, len(segments))
	for _, seg := range segments {
		if strings.TrimSpace(seg) != "" {
			nonEmpty = append(nonEmpty, seg)
		}
	}

	emitted := 0
	for i, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}

		emitted++
		pe := ParseEvent{
			SegmentIndex: i,
			Timestamp:    time.Now(),
			RawSegment:   seg,
			Fields:       make(map[string]interface{}),
			Warnings:     make([]string, 0),
		}

		// Extract segment type (first 3 characters)
		if len(seg) >= 3 {
			pe.SegmentType = seg[:3]
		}

		// Parse fields using pipe delimiter
		delimiter := "|"
		if pe.SegmentType == "MSH" && len(seg) > 3 {
			delimiter = string(seg[3])
		}

		fields := strings.Split(seg, delimiter)
		for j, field := range fields {
			if j == 0 {
				continue // Skip segment type
			}
			key := fmt.Sprintf("%s-%d", pe.SegmentType, j)
			pe.Fields[key] = field
		}

		// Check for common issues
		if pe.SegmentType == "MSH" && len(fields) < 12 {
			pe.Warnings = append(pe.Warnings, "MSH segment has fewer than 12 fields")
		}

		pe.IsComplete = emitted == len(nonEmpty)

		events <- pe
	}
}
