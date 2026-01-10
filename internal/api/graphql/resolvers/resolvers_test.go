package resolvers

import (
	"context"
	"testing"
	"time"

	"github.com/cblevins/fi-fhir/internal/api/graphql/model"
	"github.com/cblevins/fi-fhir/internal/api/graphql/store"
	"github.com/cblevins/fi-fhir/internal/workflow"
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

// =============================================================================
// Batch Submission Tests
// =============================================================================

func TestMutationResolver_SubmitBatch_Empty(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}
	ctx := context.Background()

	input := model.SubmitBatchInput{}

	result, err := mutationResolver.SubmitBatch(ctx, input)
	if err != nil {
		t.Fatalf("SubmitBatch failed: %v", err)
	}

	if result.TotalItems != 0 {
		t.Errorf("Expected 0 total items, got %d", result.TotalItems)
	}
	if result.SuccessCount != 0 {
		t.Errorf("Expected 0 success count, got %d", result.SuccessCount)
	}
	if result.FailureCount != 0 {
		t.Errorf("Expected 0 failure count, got %d", result.FailureCount)
	}
}

func TestMutationResolver_SubmitBatch_SingleMessage(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}
	ctx := context.Background()

	input := model.SubmitBatchInput{
		Messages: []model.BatchMessageItem{
			{
				Format: model.SourceFormatHL7v2,
				Source: "test-source",
				Data: `MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240115120000||ADT^A01|MSG00001|P|2.5
EVN|A01|20240115120000
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN||19800315|M
PV1|1|I|ICU^101`,
			},
		},
	}

	result, err := mutationResolver.SubmitBatch(ctx, input)
	if err != nil {
		t.Fatalf("SubmitBatch failed: %v", err)
	}

	if result.TotalItems != 1 {
		t.Errorf("Expected 1 total item, got %d", result.TotalItems)
	}
	if result.SuccessCount != 1 {
		t.Errorf("Expected 1 success, got %d", result.SuccessCount)
	}
	if len(result.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(result.Results))
	}
	if !result.Results[0].Success {
		t.Errorf("Expected first result to succeed, got errors: %v", result.Results[0].Errors)
	}
	if result.Results[0].EventID == nil {
		t.Error("Expected event ID to be set")
	}
}

func TestMutationResolver_SubmitBatch_MultipleMessages(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}
	ctx := context.Background()

	hl7Msg := `MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240115120000||ADT^A01|MSG00001|P|2.5
EVN|A01|20240115120000
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN||19800315|M
PV1|1|I|ICU^101`

	input := model.SubmitBatchInput{
		Messages: []model.BatchMessageItem{
			{Format: model.SourceFormatHL7v2, Source: "test-1", Data: hl7Msg},
			{Format: model.SourceFormatHL7v2, Source: "test-2", Data: hl7Msg},
			{Format: model.SourceFormatHL7v2, Source: "test-3", Data: hl7Msg},
		},
	}

	result, err := mutationResolver.SubmitBatch(ctx, input)
	if err != nil {
		t.Fatalf("SubmitBatch failed: %v", err)
	}

	if result.TotalItems != 3 {
		t.Errorf("Expected 3 total items, got %d", result.TotalItems)
	}
	if result.SuccessCount != 3 {
		t.Errorf("Expected 3 successes, got %d", result.SuccessCount)
	}
	if result.FailureCount != 0 {
		t.Errorf("Expected 0 failures, got %d", result.FailureCount)
	}

	// Verify each result has correct index
	for i, r := range result.Results {
		if r.Index != i {
			t.Errorf("Expected result index %d, got %d", i, r.Index)
		}
	}
}

func TestMutationResolver_SubmitBatch_SingleEvent(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}
	ctx := context.Background()

	input := model.SubmitBatchInput{
		Events: []model.BatchEventItem{
			{
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
					},
					"result": map[string]interface{}{
						"value": "12.5",
					},
				},
			},
		},
	}

	result, err := mutationResolver.SubmitBatch(ctx, input)
	if err != nil {
		t.Fatalf("SubmitBatch failed: %v", err)
	}

	if result.TotalItems != 1 {
		t.Errorf("Expected 1 total item, got %d", result.TotalItems)
	}
	if result.SuccessCount != 1 {
		t.Errorf("Expected 1 success, got %d", result.SuccessCount)
	}
	if !result.Results[0].Success {
		t.Errorf("Expected event to succeed, got errors: %v", result.Results[0].Errors)
	}
}

func TestMutationResolver_SubmitBatch_MixedMessagesAndEvents(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}
	ctx := context.Background()

	hl7Msg := `MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240115120000||ADT^A01|MSG00001|P|2.5
EVN|A01|20240115120000
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN||19800315|M
PV1|1|I|ICU^101`

	input := model.SubmitBatchInput{
		Messages: []model.BatchMessageItem{
			{Format: model.SourceFormatHL7v2, Source: "msg-1", Data: hl7Msg},
		},
		Events: []model.BatchEventItem{
			{
				Type:   model.EventTypeLabResult,
				Source: "evt-1",
				Data: map[string]interface{}{
					"patient": map[string]interface{}{"mrn": "MRN001", "familyName": "Doe", "givenName": "John"},
					"test":    map[string]interface{}{"description": "CBC"},
					"result":  map[string]interface{}{"value": "12.5"},
				},
			},
		},
	}

	result, err := mutationResolver.SubmitBatch(ctx, input)
	if err != nil {
		t.Fatalf("SubmitBatch failed: %v", err)
	}

	if result.TotalItems != 2 {
		t.Errorf("Expected 2 total items, got %d", result.TotalItems)
	}
	if result.SuccessCount != 2 {
		t.Errorf("Expected 2 successes, got %d", result.SuccessCount)
	}
	if len(result.Results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(result.Results))
	}

	// First result should be message (index 0), second should be event (index 1)
	if result.Results[0].Index != 0 || result.Results[1].Index != 1 {
		t.Errorf("Unexpected result indices: %d, %d", result.Results[0].Index, result.Results[1].Index)
	}
}

func TestMutationResolver_SubmitBatch_ExceedsMaxSize(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}
	ctx := context.Background()

	// Create 1001 messages to exceed max batch size of 1000
	messages := make([]model.BatchMessageItem, 1001)
	for i := range messages {
		messages[i] = model.BatchMessageItem{
			Format: model.SourceFormatHL7v2,
			Source: "test",
			Data:   "MSH|^~\\&|TEST",
		}
	}

	input := model.SubmitBatchInput{Messages: messages}

	_, err := mutationResolver.SubmitBatch(ctx, input)
	if err == nil {
		t.Fatal("Expected error for batch exceeding max size")
	}

	if err.Error() != "batch size 1001 exceeds maximum of 1000" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestMutationResolver_SubmitBatch_StopOnError(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}
	ctx := context.Background()

	stopOnError := true
	input := model.SubmitBatchInput{
		StopOnError: &stopOnError,
		Messages: []model.BatchMessageItem{
			{Format: model.SourceFormatHL7v2, Source: "test", Data: "INVALID_MESSAGE"},
			{Format: model.SourceFormatHL7v2, Source: "test", Data: `MSH|^~\&|TEST|FAC|REC|FAC|20240115||ADT^A01|1|P|2.5
PID|1||123^^^H^MRN||DOE^JOHN||19800315|M`},
		},
	}

	result, err := mutationResolver.SubmitBatch(ctx, input)
	if err != nil {
		t.Fatalf("SubmitBatch failed: %v", err)
	}

	// First message should fail, and stopOnError should prevent processing second
	if result.FailureCount < 1 {
		t.Errorf("Expected at least 1 failure, got %d", result.FailureCount)
	}
	// Should only have 1 result due to stopOnError
	if len(result.Results) != 1 {
		t.Errorf("Expected 1 result with stopOnError, got %d", len(result.Results))
	}
}

func TestMutationResolver_SubmitBatch_ParallelProcessing(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}
	ctx := context.Background()

	hl7Msg := `MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240115120000||ADT^A01|MSG00001|P|2.5
EVN|A01|20240115120000
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN||19800315|M
PV1|1|I|ICU^101`

	parallel := true
	input := model.SubmitBatchInput{
		Parallel: &parallel,
		Messages: []model.BatchMessageItem{
			{Format: model.SourceFormatHL7v2, Source: "test-1", Data: hl7Msg},
			{Format: model.SourceFormatHL7v2, Source: "test-2", Data: hl7Msg},
			{Format: model.SourceFormatHL7v2, Source: "test-3", Data: hl7Msg},
			{Format: model.SourceFormatHL7v2, Source: "test-4", Data: hl7Msg},
		},
	}

	result, err := mutationResolver.SubmitBatch(ctx, input)
	if err != nil {
		t.Fatalf("SubmitBatch failed: %v", err)
	}

	if result.TotalItems != 4 {
		t.Errorf("Expected 4 total items, got %d", result.TotalItems)
	}
	if result.SuccessCount != 4 {
		t.Errorf("Expected 4 successes, got %d", result.SuccessCount)
	}

	// Verify all have event IDs (all succeeded)
	for i, r := range result.Results {
		if r.EventID == nil {
			t.Errorf("Expected event ID at index %d", i)
		}
	}
}

func TestMutationResolver_SubmitBatch_CustomIndex(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}
	ctx := context.Background()

	idx100 := 100
	idx200 := 200
	input := model.SubmitBatchInput{
		Events: []model.BatchEventItem{
			{
				Type:   model.EventTypeVitalSign,
				Source: "test",
				Index:  &idx100,
				Data: map[string]interface{}{
					"patient":   map[string]interface{}{"mrn": "MRN001", "familyName": "Doe", "givenName": "John"},
					"vitalSign": map[string]interface{}{"name": "Temperature", "value": "98.6"},
				},
			},
			{
				Type:   model.EventTypeVitalSign,
				Source: "test",
				Index:  &idx200,
				Data: map[string]interface{}{
					"patient":   map[string]interface{}{"mrn": "MRN002", "familyName": "Smith", "givenName": "Jane"},
					"vitalSign": map[string]interface{}{"name": "Blood Pressure", "value": "120/80"},
				},
			},
		},
	}

	result, err := mutationResolver.SubmitBatch(ctx, input)
	if err != nil {
		t.Fatalf("SubmitBatch failed: %v", err)
	}

	// Verify custom indices are preserved
	if result.Results[0].Index != 100 {
		t.Errorf("Expected custom index 100, got %d", result.Results[0].Index)
	}
	if result.Results[1].Index != 200 {
		t.Errorf("Expected custom index 200, got %d", result.Results[1].Index)
	}
}

func TestMutationResolver_SubmitBatch_PartialFailure(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}
	ctx := context.Background()

	hl7Msg := `MSH|^~\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240115120000||ADT^A01|MSG00001|P|2.5
EVN|A01|20240115120000
PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN||19800315|M
PV1|1|I|ICU^101`

	input := model.SubmitBatchInput{
		Messages: []model.BatchMessageItem{
			{Format: model.SourceFormatHL7v2, Source: "test-1", Data: hl7Msg},
			{Format: model.SourceFormatHL7v2, Source: "test-2", Data: "INVALID"},
			{Format: model.SourceFormatHL7v2, Source: "test-3", Data: hl7Msg},
		},
	}

	result, err := mutationResolver.SubmitBatch(ctx, input)
	if err != nil {
		t.Fatalf("SubmitBatch failed: %v", err)
	}

	// Should have 2 successes and 1 failure
	if result.SuccessCount != 2 {
		t.Errorf("Expected 2 successes, got %d", result.SuccessCount)
	}
	if result.FailureCount != 1 {
		t.Errorf("Expected 1 failure, got %d", result.FailureCount)
	}

	// Verify the failed result has errors
	if result.Results[1].Success {
		t.Error("Expected index 1 to fail")
	}
	if len(result.Results[1].Errors) == 0 {
		t.Error("Expected errors for failed item")
	}
}

// =============================================================================
// TriggerWorkflow Tests
// =============================================================================

func TestMutationResolver_TriggerWorkflow_WithEngine(t *testing.T) {
	// Create a minimal workflow
	wf := &workflow.Workflow{
		Name:    "test-workflow",
		Version: "1.0",
		Routes: []workflow.Route{
			{
				Name:   "test-route",
				Filter: workflow.Filter{}, // Empty filter matches all events
				Actions: []workflow.Action{
					{
						Type: "log",
						Config: map[string]string{
							"level": "info",
						},
					},
				},
			},
		},
	}

	engine, err := workflow.NewEngine(wf)
	if err != nil {
		t.Fatalf("Failed to create workflow engine: %v", err)
	}

	resolver := NewResolver(WithWorkflowEngine(engine))
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()
	event := map[string]any{
		"type":   "lab_result",
		"source": "test",
		"patient": map[string]any{
			"mrn": "MRN001",
		},
	}

	result, err := mutationResolver.TriggerWorkflow(ctx, "test-workflow", event)
	if err != nil {
		t.Fatalf("TriggerWorkflow failed: %v", err)
	}

	if result.WorkflowName != "test-workflow" {
		t.Errorf("Expected workflow name 'test-workflow', got '%s'", result.WorkflowName)
	}
	if result.RoutesMatched != 1 {
		t.Errorf("Expected 1 matched route, got %d", result.RoutesMatched)
	}
	if result.Duration < 0 {
		t.Errorf("Expected non-negative duration, got %d", result.Duration)
	}
}

func TestMutationResolver_TriggerWorkflow_NoMatchingRoute(t *testing.T) {
	// Create a workflow with a filter that won't match
	wf := &workflow.Workflow{
		Name:    "selective-workflow",
		Version: "1.0",
		Routes: []workflow.Route{
			{
				Name: "specific-route",
				Filter: workflow.Filter{
					EventType: workflow.StringOrSlice{"claim_submitted"}, // Only matches claims
				},
				Actions: []workflow.Action{
					{Type: "log"},
				},
			},
		},
	}

	engine, err := workflow.NewEngine(wf)
	if err != nil {
		t.Fatalf("Failed to create workflow engine: %v", err)
	}

	resolver := NewResolver(WithWorkflowEngine(engine))
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()
	event := map[string]any{
		"type": "lab_result", // Won't match the claim_submitted filter
	}

	result, err := mutationResolver.TriggerWorkflow(ctx, "test-workflow", event)
	if err != nil {
		t.Fatalf("TriggerWorkflow failed: %v", err)
	}

	// Should not match any routes
	if result.RoutesMatched != 0 {
		t.Errorf("Expected 0 matched routes, got %d", result.RoutesMatched)
	}
}

// =============================================================================
// WorkflowEvents Subscription Tests
// =============================================================================

func TestSubscriptionResolver_WorkflowEvents(t *testing.T) {
	resolver := NewResolver()
	subscriptionResolver := &subscriptionResolver{resolver}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to workflow events
	eventCh, err := subscriptionResolver.WorkflowEvents(ctx, "test-workflow")
	if err != nil {
		t.Fatalf("WorkflowEvents subscription failed: %v", err)
	}

	// Broadcast a workflow event notification
	notification := &model.WorkflowEventNotification{
		Workflow:        "test-workflow",
		RoutesMatched:   []string{"route-1"},
		ActionsExecuted: []string{"route-1:2"},
		Duration:        100,
	}
	resolver.broadcastWorkflowEvent(notification)

	// Should receive the notification
	select {
	case received := <-eventCh:
		if received.Workflow != "test-workflow" {
			t.Errorf("Expected workflow 'test-workflow', got '%s'", received.Workflow)
		}
		if len(received.RoutesMatched) != 1 || received.RoutesMatched[0] != "route-1" {
			t.Errorf("Unexpected routes matched: %v", received.RoutesMatched)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for workflow event notification")
	}
}

func TestSubscriptionResolver_WorkflowEvents_AllWorkflows(t *testing.T) {
	resolver := NewResolver()
	subscriptionResolver := &subscriptionResolver{resolver}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to all workflows (empty name)
	eventCh, err := subscriptionResolver.WorkflowEvents(ctx, "")
	if err != nil {
		t.Fatalf("WorkflowEvents subscription failed: %v", err)
	}

	// Broadcast events for different workflows
	notification1 := &model.WorkflowEventNotification{Workflow: "workflow-a", Duration: 10}
	notification2 := &model.WorkflowEventNotification{Workflow: "workflow-b", Duration: 20}

	resolver.broadcastWorkflowEvent(notification1)
	resolver.broadcastWorkflowEvent(notification2)

	// Should receive both notifications
	received := 0
	timeout := time.After(time.Second)
	for received < 2 {
		select {
		case <-eventCh:
			received++
		case <-timeout:
			t.Fatalf("Timeout: only received %d of 2 expected notifications", received)
		}
	}
}

func TestSubscriptionResolver_WorkflowEvents_Filtered(t *testing.T) {
	resolver := NewResolver()
	subscriptionResolver := &subscriptionResolver{resolver}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to specific workflow
	eventCh, err := subscriptionResolver.WorkflowEvents(ctx, "target-workflow")
	if err != nil {
		t.Fatalf("WorkflowEvents subscription failed: %v", err)
	}

	// Broadcast notification for different workflow (should NOT be received)
	resolver.broadcastWorkflowEvent(&model.WorkflowEventNotification{Workflow: "other-workflow"})

	// Broadcast notification for target workflow (should be received)
	resolver.broadcastWorkflowEvent(&model.WorkflowEventNotification{Workflow: "target-workflow"})

	select {
	case received := <-eventCh:
		if received.Workflow != "target-workflow" {
			t.Errorf("Expected 'target-workflow', got '%s'", received.Workflow)
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for target workflow notification")
	}
}

func TestSubscriptionResolver_WorkflowEvents_Unsubscribe(t *testing.T) {
	resolver := NewResolver()

	// Create subscription
	ch := resolver.subscribeToWorkflowEvents("")
	if len(resolver.workflowSubscribers) != 1 {
		t.Errorf("Expected 1 subscriber, got %d", len(resolver.workflowSubscribers))
	}

	// Unsubscribe
	resolver.unsubscribeFromWorkflowEvents(ch)
	if len(resolver.workflowSubscribers) != 0 {
		t.Errorf("Expected 0 subscribers after unsubscribe, got %d", len(resolver.workflowSubscribers))
	}
}

// =============================================================================
// FHIR Subscription Mutation Tests (Error Cases)
// =============================================================================

func TestMutationResolver_DeleteFhirSubscription_NotFound(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	// Try to delete non-existent subscription
	_, err := mutationResolver.DeleteFhirSubscription(ctx, "non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent subscription")
	}
	if err.Error() != "subscription not found: non-existent-id" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestMutationResolver_PauseFhirSubscription_NotFound(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	// Try to pause non-existent subscription
	_, err := mutationResolver.PauseFhirSubscription(ctx, "non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent subscription")
	}
	if err.Error() != "subscription not found: non-existent-id" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestMutationResolver_ResumeFhirSubscription_NotFound(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	// Try to resume non-existent subscription
	_, err := mutationResolver.ResumeFhirSubscription(ctx, "non-existent-id")
	if err == nil {
		t.Error("Expected error for non-existent subscription")
	}
	if err.Error() != "subscription not found: non-existent-id" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

// Test subscription record management helpers
func TestResolver_SubscriptionRecordManagement(t *testing.T) {
	resolver := NewResolver()

	// Store a record
	record := &SubscriptionRecord{
		ID:        "sub-123",
		Name:      "Test Subscription",
		Server:    "https://fhir.example.com",
		Criteria:  "Patient?_id=123",
		Endpoint:  "https://my-app.com/webhook",
		Status:    "active",
		CreatedAt: time.Now(),
	}
	resolver.storeSubscriptionRecord(record)

	// Retrieve the record
	retrieved, exists := resolver.getSubscriptionRecord("sub-123")
	if !exists {
		t.Fatal("Expected subscription record to exist")
	}
	if retrieved.Name != "Test Subscription" {
		t.Errorf("Expected name 'Test Subscription', got '%s'", retrieved.Name)
	}

	// Update status
	updated := resolver.updateSubscriptionStatus("sub-123", "off")
	if !updated {
		t.Error("Expected updateSubscriptionStatus to return true")
	}
	retrieved, _ = resolver.getSubscriptionRecord("sub-123")
	if retrieved.Status != "off" {
		t.Errorf("Expected status 'off', got '%s'", retrieved.Status)
	}

	// Update non-existent record
	updated = resolver.updateSubscriptionStatus("non-existent", "off")
	if updated {
		t.Error("Expected updateSubscriptionStatus to return false for non-existent record")
	}

	// Delete the record
	resolver.deleteSubscriptionRecord("sub-123")
	_, exists = resolver.getSubscriptionRecord("sub-123")
	if exists {
		t.Error("Expected subscription record to be deleted")
	}
}

// =============================================================================
// Query Resolver - Workflow Tests
// =============================================================================

func TestQueryResolver_Workflow_NoEngine(t *testing.T) {
	resolver := NewResolver() // No workflow engine
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	_, err := queryResolver.Workflow(ctx, "test-workflow")
	if err == nil {
		t.Error("Expected error when workflow engine is not configured")
	}
}

func TestQueryResolver_Workflow_WithEngine(t *testing.T) {
	wf := &workflow.Workflow{Name: "test", Version: "1.0"}
	engine, _ := workflow.NewEngine(wf)

	resolver := NewResolver(WithWorkflowEngine(engine))
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	status, err := queryResolver.Workflow(ctx, "test-workflow")
	if err != nil {
		t.Fatalf("Workflow query failed: %v", err)
	}
	if status.Name != "test-workflow" {
		t.Errorf("Expected name 'test-workflow', got '%s'", status.Name)
	}
	if !status.Enabled {
		t.Error("Expected workflow to be enabled")
	}
}

func TestQueryResolver_Workflows_NoEngine(t *testing.T) {
	resolver := NewResolver() // No workflow engine
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	workflows, err := queryResolver.Workflows(ctx)
	if err != nil {
		t.Fatalf("Workflows query failed: %v", err)
	}
	if len(workflows) != 0 {
		t.Errorf("Expected empty list without engine, got %d workflows", len(workflows))
	}
}

func TestQueryResolver_Workflows_WithEngine(t *testing.T) {
	wf := &workflow.Workflow{Name: "test", Version: "1.0"}
	engine, _ := workflow.NewEngine(wf)

	resolver := NewResolver(WithWorkflowEngine(engine))
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	workflows, err := queryResolver.Workflows(ctx)
	if err != nil {
		t.Fatalf("Workflows query failed: %v", err)
	}
	// Currently returns empty list (workflow listing not implemented in code)
	if workflows == nil {
		t.Error("Expected non-nil workflows list")
	}
}

// =============================================================================
// Additional Coverage Tests
// =============================================================================

func TestMutationResolver_SubmitMessage_UnsupportedFormat(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	// Use an invalid format by type assertion
	input := model.SubmitMessageInput{
		Format: model.SourceFormat("invalid"),
		Data:   "test data",
		Source: "test",
	}

	result, err := mutationResolver.SubmitMessage(ctx, input)
	if err != nil {
		t.Fatalf("SubmitMessage should not return error: %v", err)
	}

	if result.Success {
		t.Error("Expected failure for unsupported format")
	}
	if len(result.Errors) == 0 {
		t.Error("Expected errors for unsupported format")
	}
}

func TestQueryResolver_ParsePreview_UnsupportedFormat(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	result, err := queryResolver.ParsePreview(ctx, model.SourceFormat("invalid"), "data", nil)
	if err != nil {
		t.Fatalf("ParsePreview should not return error: %v", err)
	}

	if result.Success {
		t.Error("Expected failure for unsupported format")
	}
	if len(result.Errors) == 0 {
		t.Error("Expected errors for unsupported format")
	}
}
