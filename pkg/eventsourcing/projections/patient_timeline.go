// Package projections provides built-in projections for healthcare data.
package projections

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/cblevins/fi-fhir/pkg/eventsourcing"
)

// TimelineEvent represents an event in a patient's timeline.
type TimelineEvent struct {
	Position  int64     `json:"position"`
	Timestamp time.Time `json:"timestamp"`
	EventType string    `json:"event_type"`
	Summary   string    `json:"summary"`
	StreamID  string    `json:"stream_id"`
	Source    string    `json:"source,omitempty"`
	RawData   []byte    `json:"-"` // Not included in JSON by default
}

// PatientTimeline is the read model for a patient's event timeline.
type PatientTimeline struct {
	MRN          string          `json:"mrn"`
	Events       []TimelineEvent `json:"events"`
	LastUpdated  time.Time       `json:"last_updated"`
	LastPosition int64           `json:"last_position"`
}

// PatientTimelineProjection builds chronological timelines for each patient.
type PatientTimelineProjection struct {
	timelines map[string]*PatientTimeline
	mu        sync.RWMutex
}

// NewPatientTimelineProjection creates a new patient timeline projection.
func NewPatientTimelineProjection() *PatientTimelineProjection {
	return &PatientTimelineProjection{
		timelines: make(map[string]*PatientTimeline),
	}
}

// Name returns the projection name.
func (p *PatientTimelineProjection) Name() string {
	return "patient_timeline"
}

// Handle processes an event and updates patient timelines.
func (p *PatientTimelineProjection) Handle(ctx context.Context, event eventsourcing.StoredEvent) error {
	// Extract patient MRN from the event
	mrn := p.extractMRN(event)
	if mrn == "" {
		return nil // Skip non-patient events
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Get or create timeline
	timeline, ok := p.timelines[mrn]
	if !ok {
		timeline = &PatientTimeline{
			MRN:    mrn,
			Events: make([]TimelineEvent, 0),
		}
		p.timelines[mrn] = timeline
	}

	// Check if we've already processed this event (idempotency)
	for _, existing := range timeline.Events {
		if existing.Position == event.Position {
			return nil // Already processed
		}
	}

	// Create timeline event
	timelineEvent := TimelineEvent{
		Position:  event.Position,
		Timestamp: event.Timestamp,
		EventType: event.EventType,
		Summary:   p.buildSummary(event),
		StreamID:  event.StreamID,
		Source:    event.Metadata["source"],
		RawData:   event.Data,
	}

	// Add to timeline (maintaining chronological order)
	timeline.Events = p.insertSorted(timeline.Events, timelineEvent)
	timeline.LastUpdated = time.Now()
	timeline.LastPosition = event.Position

	return nil
}

// GetTimeline returns the timeline for a patient.
func (p *PatientTimelineProjection) GetTimeline(mrn string) (*PatientTimeline, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	timeline, ok := p.timelines[mrn]
	if !ok {
		return nil, false
	}

	// Return a copy
	copy := &PatientTimeline{
		MRN:          timeline.MRN,
		Events:       make([]TimelineEvent, len(timeline.Events)),
		LastUpdated:  timeline.LastUpdated,
		LastPosition: timeline.LastPosition,
	}
	for i, e := range timeline.Events {
		copy.Events[i] = e
	}

	return copy, true
}

// GetTimelineRange returns events within a time range.
func (p *PatientTimelineProjection) GetTimelineRange(mrn string, from, to time.Time) ([]TimelineEvent, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	timeline, ok := p.timelines[mrn]
	if !ok {
		return nil, false
	}

	result := make([]TimelineEvent, 0)
	for _, event := range timeline.Events {
		if (from.IsZero() || !event.Timestamp.Before(from)) &&
			(to.IsZero() || !event.Timestamp.After(to)) {
			result = append(result, event)
		}
	}

	return result, true
}

// GetPatientMRNs returns all patient MRNs with timelines.
func (p *PatientTimelineProjection) GetPatientMRNs() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	mrns := make([]string, 0, len(p.timelines))
	for mrn := range p.timelines {
		mrns = append(mrns, mrn)
	}
	return mrns
}

// extractMRN extracts the patient MRN from an event.
func (p *PatientTimelineProjection) extractMRN(event eventsourcing.StoredEvent) string {
	// Try to get from metadata first
	if mrn, ok := event.Metadata["patient_mrn"]; ok && mrn != "" {
		return mrn
	}

	// Try to extract from stream ID (format: "patient:MRN")
	if len(event.StreamID) > 8 && event.StreamID[:8] == "patient:" {
		return event.StreamID[8:]
	}

	// Try to parse from event data
	var data map[string]interface{}
	if err := json.Unmarshal(event.Data, &data); err == nil {
		// Check common patterns
		if patient, ok := data["patient"].(map[string]interface{}); ok {
			if mrn, ok := patient["mrn"].(string); ok {
				return mrn
			}
		}
		if mrn, ok := data["mrn"].(string); ok {
			return mrn
		}
		if mrn, ok := data["patient_mrn"].(string); ok {
			return mrn
		}
	}

	return ""
}

// buildSummary creates a human-readable summary of the event.
func (p *PatientTimelineProjection) buildSummary(event eventsourcing.StoredEvent) string {
	switch event.EventType {
	case "patient_admit":
		return "Patient admitted"
	case "patient_discharge":
		return "Patient discharged"
	case "lab_result":
		return "Lab result received"
	case "vital_sign":
		return "Vital signs recorded"
	case "condition":
		return "Condition diagnosed"
	case "procedure":
		return "Procedure performed"
	case "immunization":
		return "Immunization administered"
	case "appointment_scheduled":
		return "Appointment scheduled"
	case "claim_submitted":
		return "Claim submitted"
	default:
		return event.EventType
	}
}

// insertSorted inserts an event maintaining chronological order.
func (p *PatientTimelineProjection) insertSorted(events []TimelineEvent, event TimelineEvent) []TimelineEvent {
	// Find insertion point (binary search)
	i := 0
	j := len(events)
	for i < j {
		m := (i + j) / 2
		if events[m].Timestamp.Before(event.Timestamp) {
			i = m + 1
		} else {
			j = m
		}
	}

	// Insert at position i
	events = append(events, TimelineEvent{})
	copy(events[i+1:], events[i:])
	events[i] = event

	return events
}

// Clear resets the projection (for testing/rebuilding).
func (p *PatientTimelineProjection) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.timelines = make(map[string]*PatientTimeline)
}

// Snapshot serializes the current state for persistence.
// Implements eventsourcing.Snapshotable interface.
func (p *PatientTimelineProjection) Snapshot() ([]byte, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Create a serializable copy
	data := make(map[string]*PatientTimeline, len(p.timelines))
	for mrn, timeline := range p.timelines {
		// Copy without RawData to reduce snapshot size
		t := &PatientTimeline{
			MRN:          timeline.MRN,
			Events:       make([]TimelineEvent, len(timeline.Events)),
			LastUpdated:  timeline.LastUpdated,
			LastPosition: timeline.LastPosition,
		}
		for i, e := range timeline.Events {
			t.Events[i] = TimelineEvent{
				Position:  e.Position,
				Timestamp: e.Timestamp,
				EventType: e.EventType,
				Summary:   e.Summary,
				StreamID:  e.StreamID,
				Source:    e.Source,
				// RawData omitted to save space
			}
		}
		data[mrn] = t
	}

	return json.Marshal(data)
}

// Restore loads projection state from a snapshot.
// Implements eventsourcing.Snapshotable interface.
func (p *PatientTimelineProjection) Restore(data []byte) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	timelines := make(map[string]*PatientTimeline)
	if err := json.Unmarshal(data, &timelines); err != nil {
		return err
	}

	p.timelines = timelines
	return nil
}
