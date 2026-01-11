//nolint:gosec,errcheck // Test file - G104 errors intentionally ignored in test setup
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

// =============================================================================
// Additional Coverage Tests
// =============================================================================

func TestActiveEncountersProjection_Name(t *testing.T) {
	projection := NewActiveEncountersProjection()
	if projection.Name() != "active_encounters" {
		t.Errorf("Expected name 'active_encounters', got '%s'", projection.Name())
	}
}

func TestActiveEncountersProjection_GetAllEncounters(t *testing.T) {
	projection := NewActiveEncountersProjection()
	ctx := context.Background()

	// Add multiple encounters
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position: 0, StreamID: "enc:1", EventType: "patient_admit",
		Data: []byte(`{"patient":{"mrn":"M1"},"encounter":{"id":"E1","class":"inpatient"}}`),
	})
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position: 1, StreamID: "enc:2", EventType: "patient_admit",
		Data: []byte(`{"patient":{"mrn":"M2"},"encounter":{"id":"E2","class":"outpatient"}}`),
	})

	all := projection.GetAllEncounters()
	if len(all) != 2 {
		t.Errorf("Expected 2 encounters, got %d", len(all))
	}
}

func TestActiveEncountersProjection_GetEncountersByLocation(t *testing.T) {
	projection := NewActiveEncountersProjection()
	ctx := context.Background()

	// Add encounters in different units
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position: 0, StreamID: "enc:1", EventType: "patient_admit",
		Data: []byte(`{"patient":{"mrn":"M1"},"encounter":{"id":"E1"},"location":{"unit":"ICU"}}`),
	})
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position: 1, StreamID: "enc:2", EventType: "patient_admit",
		Data: []byte(`{"patient":{"mrn":"M2"},"encounter":{"id":"E2"},"location":{"unit":"ICU"}}`),
	})
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position: 2, StreamID: "enc:3", EventType: "patient_admit",
		Data: []byte(`{"patient":{"mrn":"M3"},"encounter":{"id":"E3"},"location":{"unit":"MED-SURG"}}`),
	})

	icuEncounters := projection.GetEncountersByLocation("ICU")
	if len(icuEncounters) != 2 {
		t.Errorf("Expected 2 ICU encounters, got %d", len(icuEncounters))
	}

	medSurgEncounters := projection.GetEncountersByLocation("MED-SURG")
	if len(medSurgEncounters) != 1 {
		t.Errorf("Expected 1 MED-SURG encounter, got %d", len(medSurgEncounters))
	}
}

func TestActiveEncountersProjection_Clear(t *testing.T) {
	projection := NewActiveEncountersProjection()
	ctx := context.Background()

	// Add an encounter
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position: 0, StreamID: "enc:1", EventType: "patient_admit",
		Data: []byte(`{"patient":{"mrn":"M1"},"encounter":{"id":"E1"}}`),
	})

	if projection.Count() != 1 {
		t.Errorf("Expected 1 encounter, got %d", projection.Count())
	}

	// Clear
	projection.Clear()

	if projection.Count() != 0 {
		t.Errorf("Expected 0 encounters after clear, got %d", projection.Count())
	}
}

func TestActiveEncountersProjection_AdmitWithoutEncounterID(t *testing.T) {
	projection := NewActiveEncountersProjection()
	ctx := context.Background()

	// Admit without encounter.id (uses alternate path - generates ID from patient MRN)
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position:  0,
		StreamID:  "enc:1",
		EventType: "patient_admit",
		Timestamp: time.Now(),
		Data:      []byte(`{"patient":{"mrn":"MRN001"},"location":{"unit":"ICU"}}`),
	})

	// Should have created an encounter with generated ID
	if projection.Count() != 1 {
		t.Fatalf("Expected 1 encounter, got %d", projection.Count())
	}

	// Get by patient MRN
	enc, ok := projection.GetEncounterByPatient("MRN001")
	if !ok {
		t.Fatal("Expected encounter for patient MRN001")
	}
	if enc.PatientMRN != "MRN001" {
		t.Errorf("Expected MRN 'MRN001', got '%s'", enc.PatientMRN)
	}
}

func TestActiveEncountersProjection_DischargeNonExistent(t *testing.T) {
	projection := NewActiveEncountersProjection()
	ctx := context.Background()

	// Discharge without prior admit - should not panic
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position:  0,
		StreamID:  "enc:1",
		EventType: "patient_discharge",
		Timestamp: time.Now(),
		Data:      []byte(`{"patient":{"mrn":"MRN001"},"encounter":{"id":"ENC_NONEXISTENT"}}`),
	})

	if projection.Count() != 0 {
		t.Errorf("Expected 0 encounters, got %d", projection.Count())
	}
}

func TestEventStatisticsProjection_GetCountByTypeAndSource(t *testing.T) {
	projection := NewEventStatisticsProjection()
	ctx := context.Background()

	// Add events
	projection.Handle(ctx, eventsourcing.StoredEvent{
		EventType: "patient_admit", Metadata: map[string]string{"source": "adt"},
	})
	projection.Handle(ctx, eventsourcing.StoredEvent{
		EventType: "patient_admit", Metadata: map[string]string{"source": "adt"},
	})
	projection.Handle(ctx, eventsourcing.StoredEvent{
		EventType: "lab_result", Metadata: map[string]string{"source": "lab"},
	})

	// Test GetEventCountByType
	admitCount := projection.GetEventCountByType("patient_admit")
	if admitCount != 2 {
		t.Errorf("Expected 2 patient_admit events, got %d", admitCount)
	}

	// Test GetEventCountBySource
	adtCount := projection.GetEventCountBySource("adt")
	if adtCount != 2 {
		t.Errorf("Expected 2 adt events, got %d", adtCount)
	}

	// Test GetTopSources
	topSources := projection.GetTopSources(1)
	if len(topSources) != 1 {
		t.Errorf("Expected 1 top source, got %d", len(topSources))
	}
	if topSources[0].Name != "adt" {
		t.Errorf("Expected top source 'adt', got '%s'", topSources[0].Name)
	}
}

func TestEventStatisticsProjection_Clear(t *testing.T) {
	projection := NewEventStatisticsProjection()
	ctx := context.Background()

	// Add events
	projection.Handle(ctx, eventsourcing.StoredEvent{EventType: "test"})

	stats := projection.GetStatistics()
	if stats.TotalEvents != 1 {
		t.Errorf("Expected 1 event, got %d", stats.TotalEvents)
	}

	// Clear
	projection.Clear()

	stats = projection.GetStatistics()
	if stats.TotalEvents != 0 {
		t.Errorf("Expected 0 events after clear, got %d", stats.TotalEvents)
	}
}

func TestPatientTimelineProjection_ExtractMRN_EdgeCases(t *testing.T) {
	projection := NewPatientTimelineProjection()
	ctx := context.Background()

	// Test with MRN in root level
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position:  0,
		StreamID:  "patient:MRN_ROOT",
		EventType: "event1",
		Timestamp: time.Now(),
		Data:      []byte(`{"mrn":"MRN_ROOT"}`),
	})

	// Test with MRN in patient object
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position:  1,
		StreamID:  "patient:MRN_PATIENT",
		EventType: "event2",
		Timestamp: time.Now(),
		Data:      []byte(`{"patient":{"mrn":"MRN_PATIENT"}}`),
	})

	// Test with patient_mrn field
	projection.Handle(ctx, eventsourcing.StoredEvent{
		Position:  2,
		StreamID:  "patient:MRN_FIELD",
		EventType: "event3",
		Timestamp: time.Now(),
		Data:      []byte(`{"patient_mrn":"MRN_FIELD"}`),
	})

	// Verify all MRNs were extracted
	mrns := projection.GetPatientMRNs()
	if len(mrns) != 3 {
		t.Errorf("Expected 3 patients, got %d: %v", len(mrns), mrns)
	}
}

func TestPatientTimelineProjection_MultipleEventTypes(t *testing.T) {
	projection := NewPatientTimelineProjection()
	ctx := context.Background()

	// Add events with different types
	now := time.Now()
	events := []eventsourcing.StoredEvent{
		{Position: 0, StreamID: "patient:MRN001", EventType: "patient_admit", Timestamp: now.Add(-3 * time.Hour), Data: []byte(`{"mrn":"MRN001"}`)},
		{Position: 1, StreamID: "patient:MRN001", EventType: "lab_result", Timestamp: now.Add(-2 * time.Hour), Data: []byte(`{"mrn":"MRN001"}`)},
		{Position: 2, StreamID: "patient:MRN001", EventType: "lab_result", Timestamp: now.Add(-1 * time.Hour), Data: []byte(`{"mrn":"MRN001"}`)},
		{Position: 3, StreamID: "patient:MRN001", EventType: "vital_sign", Timestamp: now, Data: []byte(`{"mrn":"MRN001"}`)},
	}

	for _, e := range events {
		projection.Handle(ctx, e)
	}

	timeline, ok := projection.GetTimeline("MRN001")
	if !ok {
		t.Fatal("Expected timeline for MRN001")
	}

	// Check all events were captured
	if len(timeline.Events) != 4 {
		t.Errorf("Expected 4 events in timeline, got %d", len(timeline.Events))
	}

	// Verify last updated is set
	if timeline.LastUpdated.IsZero() {
		t.Error("Expected LastUpdated to be set")
	}

	// Verify events are in order by timestamp
	for i := 1; i < len(timeline.Events); i++ {
		if timeline.Events[i].Timestamp.Before(timeline.Events[i-1].Timestamp) {
			t.Error("Events should be sorted by timestamp")
		}
	}
}
