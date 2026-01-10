package projections

import (
	"context"
	"testing"
	"time"

	"github.com/crb2nu/fi-fhir/pkg/eventsourcing"
)

func TestPatientTimelineProjection(t *testing.T) {
	store := eventsourcing.NewMemoryStore()
	checkpointStore := eventsourcing.NewMemoryCheckpointStore()
	ctx := context.Background()

	// Create test events
	store.Append(ctx, "patient:MRN001", eventsourcing.VersionNone, []eventsourcing.EventData{
		{
			EventType: "patient_admit",
			Data:      []byte(`{"mrn":"MRN001","patient":{"mrn":"MRN001"}}`),
			Metadata:  map[string]string{"source": "adt"},
		},
	})
	store.Append(ctx, "patient:MRN001", 0, []eventsourcing.EventData{
		{
			EventType: "lab_result",
			Data:      []byte(`{"mrn":"MRN001","test":"CBC"}`),
			Metadata:  map[string]string{"source": "lab"},
		},
	})
	store.Append(ctx, "patient:MRN002", eventsourcing.VersionNone, []eventsourcing.EventData{
		{
			EventType: "patient_admit",
			Data:      []byte(`{"mrn":"MRN002"}`),
			Metadata:  map[string]string{"source": "adt"},
		},
	})

	// Run projection
	projection := NewPatientTimelineProjection()
	runner := eventsourcing.NewProjectionRunner(store, checkpointStore, eventsourcing.DefaultProjectionRunnerConfig())
	runner.RegisterProjection(projection)

	err := runner.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	// Check timelines
	timeline, ok := projection.GetTimeline("MRN001")
	if !ok {
		t.Fatal("Expected timeline for MRN001")
	}
	if len(timeline.Events) != 2 {
		t.Errorf("Expected 2 events in timeline, got %d", len(timeline.Events))
	}

	timeline2, ok := projection.GetTimeline("MRN002")
	if !ok {
		t.Fatal("Expected timeline for MRN002")
	}
	if len(timeline2.Events) != 1 {
		t.Errorf("Expected 1 event in timeline, got %d", len(timeline2.Events))
	}

	// Check patient MRNs
	mrns := projection.GetPatientMRNs()
	if len(mrns) != 2 {
		t.Errorf("Expected 2 patients, got %d", len(mrns))
	}
}

func TestPatientTimelineProjection_TimeRange(t *testing.T) {
	projection := NewPatientTimelineProjection()
	ctx := context.Background()

	now := time.Now()

	// Add events directly
	events := []eventsourcing.StoredEvent{
		{Position: 0, StreamID: "patient:MRN001", EventType: "event1", Timestamp: now.Add(-2 * time.Hour), Data: []byte(`{"mrn":"MRN001"}`)},
		{Position: 1, StreamID: "patient:MRN001", EventType: "event2", Timestamp: now.Add(-1 * time.Hour), Data: []byte(`{"mrn":"MRN001"}`)},
		{Position: 2, StreamID: "patient:MRN001", EventType: "event3", Timestamp: now, Data: []byte(`{"mrn":"MRN001"}`)},
	}

	for _, e := range events {
		projection.Handle(ctx, e)
	}

	// Query time range
	from := now.Add(-90 * time.Minute)
	to := now.Add(-30 * time.Minute)
	rangeEvents, ok := projection.GetTimelineRange("MRN001", from, to)
	if !ok {
		t.Fatal("Expected timeline for MRN001")
	}
	if len(rangeEvents) != 1 {
		t.Errorf("Expected 1 event in range, got %d", len(rangeEvents))
	}
}

func TestEventStatisticsProjection(t *testing.T) {
	store := eventsourcing.NewMemoryStore()
	checkpointStore := eventsourcing.NewMemoryCheckpointStore()
	ctx := context.Background()

	// Create test events
	store.Append(ctx, "stream:1", eventsourcing.VersionNone, []eventsourcing.EventData{
		{EventType: "patient_admit", Data: []byte(`{}`), Metadata: map[string]string{"source": "adt"}},
	})
	store.Append(ctx, "stream:2", eventsourcing.VersionNone, []eventsourcing.EventData{
		{EventType: "patient_admit", Data: []byte(`{}`), Metadata: map[string]string{"source": "adt"}},
	})
	store.Append(ctx, "stream:3", eventsourcing.VersionNone, []eventsourcing.EventData{
		{EventType: "lab_result", Data: []byte(`{}`), Metadata: map[string]string{"source": "lab"}},
	})
	store.Append(ctx, "stream:4", eventsourcing.VersionNone, []eventsourcing.EventData{
		{EventType: "lab_result", Data: []byte(`{}`), Metadata: map[string]string{"source": "lab"}},
	})
	store.Append(ctx, "stream:5", eventsourcing.VersionNone, []eventsourcing.EventData{
		{EventType: "lab_result", Data: []byte(`{}`), Metadata: map[string]string{"source": "lab"}},
	})

	// Run projection
	projection := NewEventStatisticsProjection()
	runner := eventsourcing.NewProjectionRunner(store, checkpointStore, eventsourcing.DefaultProjectionRunnerConfig())
	runner.RegisterProjection(projection)

	err := runner.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce failed: %v", err)
	}

	// Check statistics
	stats := projection.GetStatistics()
	if stats.TotalEvents != 5 {
		t.Errorf("Expected 5 total events, got %d", stats.TotalEvents)
	}
	if stats.ByType["patient_admit"] != 2 {
		t.Errorf("Expected 2 patient_admit events, got %d", stats.ByType["patient_admit"])
	}
	if stats.ByType["lab_result"] != 3 {
		t.Errorf("Expected 3 lab_result events, got %d", stats.ByType["lab_result"])
	}
	if stats.BySource["adt"] != 2 {
		t.Errorf("Expected 2 adt events, got %d", stats.BySource["adt"])
	}
	if stats.BySource["lab"] != 3 {
		t.Errorf("Expected 3 lab events, got %d", stats.BySource["lab"])
	}

	// Test top types
	topTypes := projection.GetTopEventTypes(1)
	if len(topTypes) != 1 {
		t.Errorf("Expected 1 top type, got %d", len(topTypes))
	}
	if topTypes[0].Name != "lab_result" {
		t.Errorf("Expected top type 'lab_result', got '%s'", topTypes[0].Name)
	}
}

func TestActiveEncountersProjection(t *testing.T) {
	projection := NewActiveEncountersProjection()
	ctx := context.Background()

	// Admit patient
	admitEvent := eventsourcing.StoredEvent{
		Position:  0,
		StreamID:  "encounter:ENC001",
		EventType: "patient_admit",
		Timestamp: time.Now(),
		Data: []byte(`{
			"patient": {"mrn": "MRN001", "family_name": "Doe", "given_name": "John"},
			"encounter": {"id": "ENC001", "class": "inpatient"},
			"location": {"facility": "Hospital A", "unit": "ICU", "room": "101", "bed": "A"}
		}`),
	}
	projection.Handle(ctx, admitEvent)

	// Check encounter exists
	enc, ok := projection.GetEncounter("ENC001")
	if !ok {
		t.Fatal("Expected encounter ENC001")
	}
	if enc.PatientMRN != "MRN001" {
		t.Errorf("Expected MRN 'MRN001', got '%s'", enc.PatientMRN)
	}
	if enc.Class != "inpatient" {
		t.Errorf("Expected class 'inpatient', got '%s'", enc.Class)
	}
	if enc.Unit != "ICU" {
		t.Errorf("Expected unit 'ICU', got '%s'", enc.Unit)
	}

	// Check by patient lookup
	enc2, ok := projection.GetEncounterByPatient("MRN001")
	if !ok {
		t.Fatal("Expected encounter for MRN001")
	}
	if enc2.ID != "ENC001" {
		t.Errorf("Expected encounter ID 'ENC001', got '%s'", enc2.ID)
	}

	// Check count
	if projection.Count() != 1 {
		t.Errorf("Expected 1 active encounter, got %d", projection.Count())
	}
}

func TestActiveEncountersProjection_Transfer(t *testing.T) {
	projection := NewActiveEncountersProjection()
	ctx := context.Background()

	// Admit patient
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position:  0,
		StreamID:  "encounter:ENC001",
		EventType: "patient_admit",
		Timestamp: time.Now(),
		Data: []byte(`{
			"patient": {"mrn": "MRN001"},
			"encounter": {"id": "ENC001", "class": "inpatient"},
			"location": {"unit": "ICU"}
		}`),
	})

	// Transfer patient
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position:  1,
		StreamID:  "encounter:ENC001",
		EventType: "patient_transfer",
		Timestamp: time.Now(),
		Data: []byte(`{
			"encounter": {"id": "ENC001"},
			"new_location": {"unit": "MED-SURG", "room": "202", "bed": "B"}
		}`),
	})

	// Check updated location
	enc, ok := projection.GetEncounter("ENC001")
	if !ok {
		t.Fatal("Expected encounter ENC001")
	}
	if enc.Unit != "MED-SURG" {
		t.Errorf("Expected unit 'MED-SURG', got '%s'", enc.Unit)
	}
	if enc.Room != "202" {
		t.Errorf("Expected room '202', got '%s'", enc.Room)
	}
}

func TestActiveEncountersProjection_Discharge(t *testing.T) {
	projection := NewActiveEncountersProjection()
	ctx := context.Background()

	// Admit patient
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position:  0,
		StreamID:  "encounter:ENC001",
		EventType: "patient_admit",
		Timestamp: time.Now(),
		Data: []byte(`{
			"patient": {"mrn": "MRN001"},
			"encounter": {"id": "ENC001"}
		}`),
	})

	if projection.Count() != 1 {
		t.Errorf("Expected 1 active encounter after admit, got %d", projection.Count())
	}

	// Discharge patient
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position:  1,
		StreamID:  "encounter:ENC001",
		EventType: "patient_discharge",
		Timestamp: time.Now(),
		Data: []byte(`{
			"patient": {"mrn": "MRN001"},
			"encounter": {"id": "ENC001"}
		}`),
	})

	if projection.Count() != 0 {
		t.Errorf("Expected 0 active encounters after discharge, got %d", projection.Count())
	}

	// Verify encounter is removed
	_, ok := projection.GetEncounter("ENC001")
	if ok {
		t.Error("Expected encounter to be removed after discharge")
	}
}

func TestActiveEncountersProjection_ByClass(t *testing.T) {
	projection := NewActiveEncountersProjection()
	ctx := context.Background()

	// Add inpatient
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position: 0, StreamID: "enc:1", EventType: "patient_admit",
		Data: []byte(`{"patient":{"mrn":"M1"},"encounter":{"id":"E1","class":"inpatient"}}`),
	})
	// Add outpatient
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position: 1, StreamID: "enc:2", EventType: "patient_admit",
		Data: []byte(`{"patient":{"mrn":"M2"},"encounter":{"id":"E2","class":"outpatient"}}`),
	})
	// Add another inpatient
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position: 2, StreamID: "enc:3", EventType: "patient_admit",
		Data: []byte(`{"patient":{"mrn":"M3"},"encounter":{"id":"E3","class":"inpatient"}}`),
	})

	inpatients := projection.GetEncountersByClass("inpatient")
	if len(inpatients) != 2 {
		t.Errorf("Expected 2 inpatient encounters, got %d", len(inpatients))
	}

	outpatients := projection.GetEncountersByClass("outpatient")
	if len(outpatients) != 1 {
		t.Errorf("Expected 1 outpatient encounter, got %d", len(outpatients))
	}
}

func TestPatientTimelineProjection_Snapshot(t *testing.T) {
	projection := NewPatientTimelineProjection()
	ctx := context.Background()

	// Add events
	events := []eventsourcing.StoredEvent{
		{Position: 0, StreamID: "patient:MRN001", EventType: "patient_admit", Timestamp: time.Now(), Data: []byte(`{"mrn":"MRN001"}`)},
		{Position: 1, StreamID: "patient:MRN001", EventType: "lab_result", Timestamp: time.Now(), Data: []byte(`{"mrn":"MRN001","test":"CBC"}`)},
		{Position: 2, StreamID: "patient:MRN002", EventType: "patient_admit", Timestamp: time.Now(), Data: []byte(`{"mrn":"MRN002"}`)},
	}

	for _, e := range events {
		projection.Handle(ctx, e)
	}

	// Take snapshot
	data, err := projection.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}

	if len(data) == 0 {
		t.Fatal("Snapshot data is empty")
	}

	// Clear and restore
	projection.Clear()
	mrns := projection.GetPatientMRNs()
	if len(mrns) != 0 {
		t.Errorf("Expected 0 patients after clear, got %d", len(mrns))
	}

	err = projection.Restore(data)
	if err != nil {
		t.Fatalf("Restore failed: %v", err)
	}

	// Verify restored state
	mrns = projection.GetPatientMRNs()
	if len(mrns) != 2 {
		t.Errorf("Expected 2 patients after restore, got %d", len(mrns))
	}

	timeline1, ok := projection.GetTimeline("MRN001")
	if !ok {
		t.Fatal("Expected timeline for MRN001 after restore")
	}
	if len(timeline1.Events) != 2 {
		t.Errorf("Expected 2 events in MRN001 timeline, got %d", len(timeline1.Events))
	}

	timeline2, ok := projection.GetTimeline("MRN002")
	if !ok {
		t.Fatal("Expected timeline for MRN002 after restore")
	}
	if len(timeline2.Events) != 1 {
		t.Errorf("Expected 1 event in MRN002 timeline, got %d", len(timeline2.Events))
	}
}
