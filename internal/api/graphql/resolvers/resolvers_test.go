package resolvers

import (
	"context"
	"testing"
	"time"

	"github.com/cblevins/fi-fhir/internal/api/graphql/model"
	"github.com/cblevins/fi-fhir/internal/api/graphql/store"
)

func TestQueryResolver_Health(t *testing.T) {
	resolver := NewResolver(WithVersion("1.0.0"))
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()
	health, err := queryResolver.Health(ctx)
	if err != nil {
		t.Fatalf("Health query failed: %v", err)
	}

	if health.Status != "healthy" {
		t.Errorf("Expected status 'healthy', got '%s'", health.Status)
	}
	if health.Version != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", health.Version)
	}
	if len(health.Components) < 2 {
		t.Errorf("Expected at least 2 components, got %d", len(health.Components))
	}
}

func TestQueryResolver_Event(t *testing.T) {
	memStore := store.NewMemoryStore()
	resolver := NewResolver(WithStore(memStore))
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	// Save an event
	event := &model.LabResultEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "test-lab-1",
			Type:      model.EventTypeLabResult,
			Timestamp: time.Now(),
			Source:    "lab-system",
		},
		Patient: model.Patient{MRN: "MRN001"},
		Test:    model.LabTest{Description: "CBC"},
		Result:  model.LabResult{Value: "12.5"},
	}
	memStore.SaveEvent(ctx, event)

	// Query the event
	retrieved, err := queryResolver.Event(ctx, "test-lab-1")
	if err != nil {
		t.Fatalf("Event query failed: %v", err)
	}

	labEvent, ok := retrieved.(*model.LabResultEvent)
	if !ok {
		t.Fatalf("Expected *model.LabResultEvent, got %T", retrieved)
	}

	if labEvent.Test.Description != "CBC" {
		t.Errorf("Expected test description 'CBC', got '%s'", labEvent.Test.Description)
	}
}

func TestQueryResolver_Events(t *testing.T) {
	memStore := store.NewMemoryStore()
	resolver := NewResolver(WithStore(memStore))
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	// Add multiple events
	for i := 0; i < 5; i++ {
		event := &model.VitalSignEvent{
			BaseEventFields: model.BaseEventFields{
				ID:        "vs-" + string(rune('A'+i)),
				Type:      model.EventTypeVitalSign,
				Timestamp: time.Now().Add(time.Duration(i) * time.Hour),
				Source:    "vitals",
			},
			Patient: model.Patient{MRN: "MRN001"},
		}
		memStore.SaveEvent(ctx, event)
	}

	// Query events
	first := 10
	conn, err := queryResolver.Events(ctx, nil, &first, nil, nil)
	if err != nil {
		t.Fatalf("Events query failed: %v", err)
	}

	if conn.TotalCount != 5 {
		t.Errorf("Expected 5 events, got %d", conn.TotalCount)
	}
}

func TestQueryResolver_Events_WithFilter(t *testing.T) {
	memStore := store.NewMemoryStore()
	resolver := NewResolver(WithStore(memStore))
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	// Add different event types
	memStore.SaveEvent(ctx, &model.PatientAdmitEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "admit-1",
			Type:      model.EventTypePatientAdmit,
			Timestamp: time.Now(),
			Source:    "adt",
		},
	})
	memStore.SaveEvent(ctx, &model.LabResultEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "lab-1",
			Type:      model.EventTypeLabResult,
			Timestamp: time.Now(),
			Source:    "lab",
		},
	})
	memStore.SaveEvent(ctx, &model.VitalSignEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "vs-1",
			Type:      model.EventTypeVitalSign,
			Timestamp: time.Now(),
			Source:    "vitals",
		},
	})

	// Query with type filter
	filter := &model.EventFilter{
		Types: []model.EventType{model.EventTypeLabResult},
	}
	first := 10
	conn, err := queryResolver.Events(ctx, filter, &first, nil, nil)
	if err != nil {
		t.Fatalf("Events query failed: %v", err)
	}

	if conn.TotalCount != 1 {
		t.Errorf("Expected 1 event, got %d", conn.TotalCount)
	}
}

func TestQueryResolver_Patient(t *testing.T) {
	memStore := store.NewMemoryStore()
	resolver := NewResolver(WithStore(memStore))
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	// Save a patient
	patient := &model.Patient{
		MRN:        "MRN123",
		FamilyName: "Doe",
		GivenName:  "John",
	}
	memStore.SavePatient(patient)

	// Query the patient
	retrieved, err := queryResolver.Patient(ctx, "MRN123")
	if err != nil {
		t.Fatalf("Patient query failed: %v", err)
	}

	if retrieved.FamilyName != "Doe" {
		t.Errorf("Expected family name 'Doe', got '%s'", retrieved.FamilyName)
	}
}

func TestQueryResolver_ParsePreview_HL7v2(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	msg := `MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240115120000||ADT^A01|MSG00001|P|2.5
EVN|A01|20240115120000
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN||19800315|M
PV1|1|I|ICU^101`

	result, err := queryResolver.ParsePreview(ctx, model.SourceFormatHL7v2, msg, nil)
	if err != nil {
		t.Fatalf("ParsePreview failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got errors: %v", result.Errors)
	}
	if len(result.Events) == 0 {
		t.Error("Expected at least one event")
	}
}

func TestMutationResolver_SubmitMessage_HL7v2(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	input := model.SubmitMessageInput{
		Format: model.SourceFormatHL7v2,
		Source: "test-source",
		Data: `MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240115120000||ADT^A01|MSG00001|P|2.5
EVN|A01|20240115120000
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN||19800315|M
PV1|1|I|ICU^101`,
	}

	result, err := mutationResolver.SubmitMessage(ctx, input)
	if err != nil {
		t.Fatalf("SubmitMessage failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got errors: %v", result.Errors)
	}
}

func TestMutationResolver_SubmitEvent(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	input := model.SubmitEventInput{
		Type:   model.EventTypeLabResult,
		Source: "test-source",
		Data: map[string]interface{}{
			"patient": map[string]interface{}{
				"mrn":        "MRN001",
				"familyName": "Doe",
				"givenName":  "John",
			},
			"test": map[string]interface{}{
				"description": "CBC",
				"loincCode":   "58410-2",
			},
			"result": map[string]interface{}{
				"value": "12.5",
				"unit":  "10*3/uL",
			},
		},
	}

	result, err := mutationResolver.SubmitEvent(ctx, input)
	if err != nil {
		t.Fatalf("SubmitEvent failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got errors: %v", result.Errors)
	}
	if result.EventID == nil {
		t.Error("Expected event ID to be set")
	}
}

func TestMutationResolver_TriggerWorkflow_NoEngine(t *testing.T) {
	resolver := NewResolver() // No workflow engine
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	_, err := mutationResolver.TriggerWorkflow(ctx, "test-workflow", nil)
	if err == nil {
		t.Error("Expected error when workflow engine is not configured")
	}
}

func TestSubscriptionResolver_EventStream(t *testing.T) {
	memStore := store.NewMemoryStore()
	resolver := NewResolver(WithStore(memStore))
	subscriptionResolver := &subscriptionResolver{resolver}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to all events
	eventCh, err := subscriptionResolver.EventStream(ctx, nil)
	if err != nil {
		t.Fatalf("EventStream failed: %v", err)
	}

	// Submit an event
	event := &model.LabResultEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "sub-test",
			Type:      model.EventTypeLabResult,
			Timestamp: time.Now(),
			Source:    "test",
		},
	}
	memStore.SaveEvent(ctx, event)

	// Wait for event
	select {
	case received := <-eventCh:
		if received.GetID() != "sub-test" {
			t.Errorf("Expected event ID 'sub-test', got '%s'", received.GetID())
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for subscribed event")
	}
}

func TestSubscriptionResolver_PatientEvents(t *testing.T) {
	memStore := store.NewMemoryStore()
	resolver := NewResolver(WithStore(memStore))
	subscriptionResolver := &subscriptionResolver{resolver}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to specific patient events
	eventCh, err := subscriptionResolver.PatientEvents(ctx, "MRN001")
	if err != nil {
		t.Fatalf("PatientEvents failed: %v", err)
	}

	// Submit an event for different patient (should not be received)
	memStore.SaveEvent(ctx, &model.LabResultEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "other-patient",
			Type:      model.EventTypeLabResult,
			Timestamp: time.Now(),
			Source:    "test",
		},
		Patient: model.Patient{MRN: "MRN999"},
	})

	// Submit an event for subscribed patient
	memStore.SaveEvent(ctx, &model.LabResultEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "target-patient",
			Type:      model.EventTypeLabResult,
			Timestamp: time.Now(),
			Source:    "test",
		},
		Patient: model.Patient{MRN: "MRN001"},
	})

	// Should receive only the target patient event
	select {
	case received := <-eventCh:
		if received.GetID() != "target-patient" {
			t.Errorf("Expected event ID 'target-patient', got '%s'", received.GetID())
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for patient event")
	}
}

// Test helper function conversion
func TestConvertToGraphQLEvent_PatientAdmit(t *testing.T) {
	// Test internal conversion function behavior through public APIs
	memStore := store.NewMemoryStore()
	resolver := NewResolver(WithStore(memStore))
	mutationResolver := &mutationResolver{resolver}
	ctx := context.Background()

	input := model.SubmitMessageInput{
		Format: model.SourceFormatHL7v2,
		Source: "test-source",
		Data: `MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240115120000||ADT^A01|MSG00001|P|2.5
EVN|A01|20240115120000
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN||19800315|M
PV1|1|I|ICU^101`,
	}

	result, err := mutationResolver.SubmitMessage(ctx, input)
	if err != nil {
		t.Fatalf("SubmitMessage failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got errors: %v", result.Errors)
	}

	// Verify event was stored and can be retrieved
	if result.EventID != nil {
		event, err := memStore.GetEvent(ctx, *result.EventID)
		if err != nil {
			t.Fatalf("GetEvent failed: %v", err)
		}

		admitEvent, ok := event.(*model.PatientAdmitEvent)
		if !ok {
			t.Fatalf("Expected *model.PatientAdmitEvent, got %T", event)
		}

		if admitEvent.Source != "test-source" {
			t.Errorf("Expected source 'test-source', got '%s'", admitEvent.Source)
		}
	}
}

// =============================================================================
// Projection Resolver Tests
// =============================================================================

func TestQueryResolver_PatientTimeline(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	// Query timeline for non-existent patient (should return nil, not error)
	timeline, err := queryResolver.PatientTimeline(ctx, "MRN001", nil, nil, nil)
	if err != nil {
		t.Fatalf("PatientTimeline query failed: %v", err)
	}
	if timeline != nil {
		t.Errorf("Expected nil timeline for non-existent patient, got %v", timeline)
	}
}

func TestQueryResolver_EventStatistics(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	// Query statistics (should return empty stats for fresh projection)
	stats, err := queryResolver.EventStatistics(ctx)
	if err != nil {
		t.Fatalf("EventStatistics query failed: %v", err)
	}
	if stats == nil {
		t.Fatal("Expected non-nil statistics")
	}
	// Empty projection should have zero total events
	if stats.TotalEvents != 0 {
		t.Errorf("Expected 0 total events for fresh projection, got %d", stats.TotalEvents)
	}
}

func TestQueryResolver_ActiveEncounters(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	// Query active encounters (should return empty list for fresh projection)
	encounters, err := queryResolver.ActiveEncounters(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ActiveEncounters query failed: %v", err)
	}
	if encounters == nil {
		t.Fatal("Expected non-nil encounters list")
	}
	if len(encounters) != 0 {
		t.Errorf("Expected 0 encounters for fresh projection, got %d", len(encounters))
	}
}

func TestQueryResolver_ActiveEncounter(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	// Query specific encounter (should return nil for non-existent)
	encounter, err := queryResolver.ActiveEncounter(ctx, "ENC-001")
	if err != nil {
		t.Fatalf("ActiveEncounter query failed: %v", err)
	}
	if encounter != nil {
		t.Errorf("Expected nil encounter for non-existent ID, got %v", encounter)
	}
}

func TestQueryResolver_ActiveEncounterByPatient(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	// Query encounter by patient (should return nil for non-existent)
	encounter, err := queryResolver.ActiveEncounterByPatient(ctx, "MRN001")
	if err != nil {
		t.Fatalf("ActiveEncounterByPatient query failed: %v", err)
	}
	if encounter != nil {
		t.Errorf("Expected nil encounter for non-existent patient, got %v", encounter)
	}
}

func TestQueryResolver_ProjectionStatus(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	// Query projection status
	statuses, err := queryResolver.ProjectionStatus(ctx)
	if err != nil {
		t.Fatalf("ProjectionStatus query failed: %v", err)
	}
	if statuses == nil {
		t.Fatal("Expected non-nil projection statuses")
	}
	// Should have 3 projections: patient_timeline, event_statistics, active_encounters
	if len(statuses) != 3 {
		t.Errorf("Expected 3 projection statuses, got %d", len(statuses))
	}

	// Verify projection names
	names := make(map[string]bool)
	for _, s := range statuses {
		names[s.Name] = true
		if s.Status != "running" {
			t.Errorf("Expected status 'running' for projection '%s', got '%s'", s.Name, s.Status)
		}
	}
	expectedNames := []string{"patient_timeline", "event_statistics", "active_encounters"}
	for _, name := range expectedNames {
		if !names[name] {
			t.Errorf("Expected projection '%s' not found in statuses", name)
		}
	}
}
