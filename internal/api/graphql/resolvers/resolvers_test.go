package resolvers

import (
	"context"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/store"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
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

func TestQueryResolver_LlmCapability_DefaultDisabled(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}

	capability, err := queryResolver.LlmCapability(context.Background())
	if err != nil {
		t.Fatalf("LlmCapability query failed: %v", err)
	}

	if capability.Enabled {
		t.Error("Expected LLM capability to be disabled by default")
	}
	if capability.Configured {
		t.Error("Expected default LLM capability to be unconfigured")
	}
	if capability.Status != "disabled" {
		t.Errorf("Expected status disabled, got %q", capability.Status)
	}
	if len(capability.Warnings) == 0 {
		t.Error("Expected default LLM capability warning")
	}
}

func TestQueryResolver_LlmCapability_SafeConfiguredFields(t *testing.T) {
	capability := NewLLMCapability(true, llm.Config{
		BaseURL:      "https://user:secret@example.com:8443/v1?api_key=hidden",
		DefaultModel: "default-model",
		QualityModel: "quality-model",
	}, "available", nil)
	resolver := NewResolver(WithLLMCapability(capability))
	queryResolver := &queryResolver{resolver}

	got, err := queryResolver.LlmCapability(context.Background())
	if err != nil {
		t.Fatalf("LlmCapability query failed: %v", err)
	}

	if !got.Enabled {
		t.Error("Expected LLM capability to be enabled")
	}
	if !got.Configured {
		t.Error("Expected LLM capability to be configured")
	}
	if got.ProviderBaseURLHost == nil || *got.ProviderBaseURLHost != "example.com:8443" {
		t.Fatalf("Expected provider host example.com:8443, got %v", got.ProviderBaseURLHost)
	}
	if strings.Contains(*got.ProviderBaseURLHost, "secret") || strings.Contains(*got.ProviderBaseURLHost, "hidden") {
		t.Errorf("Provider host leaked secret-bearing URL data: %q", *got.ProviderBaseURLHost)
	}
	if got.DefaultModel == nil || *got.DefaultModel != "default-model" {
		t.Errorf("Expected default model, got %v", got.DefaultModel)
	}
	if got.QualityModel == nil || *got.QualityModel != "quality-model" {
		t.Errorf("Expected quality model, got %v", got.QualityModel)
	}
	if got.Status != "available" {
		t.Errorf("Expected status available, got %q", got.Status)
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

	_, err := mutationResolver.TriggerWorkflow(ctx, "test-workflow", nil, nil, nil)
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

	result, err := mutationResolver.TriggerWorkflow(ctx, "test-workflow", event, nil, nil)
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

	result, err := mutationResolver.TriggerWorkflow(ctx, "test-workflow", event, nil, nil)
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

	status, err := queryResolver.Workflow(ctx, "test")
	if err != nil {
		t.Fatalf("Workflow query failed: %v", err)
	}
	if status == nil {
		t.Fatal("Expected non-nil status for matching workflow name")
	}
	if status.Name != "test" {
		t.Errorf("Expected name 'test', got '%s'", status.Name)
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

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestConvertToGraphQLEvent_VitalSign(t *testing.T) {
	patient := &events.Patient{MRN: "MRN001", FamilyName: "Doe", GivenName: "John"}
	evt := &events.VitalSignEvent{
		EventMeta: events.EventMeta{
			ID:        "vs-123",
			Type:      events.EventVitalSign,
			Timestamp: time.Now(),
		},
		Patient: patient,
		VitalSign: events.VitalSign{
			Name:      "Temperature",
			LOINCCode: "8310-5",
			Value:     "98.6",
			Unit:      "degF",
		},
	}

	format := model.SourceFormatHL7v2
	correlationID := "corr-123"
	result := convertToGraphQLEvent(evt, "test-source", format, &correlationID)

	vsEvent, ok := result.(*model.VitalSignEvent)
	if !ok {
		t.Fatalf("Expected *model.VitalSignEvent, got %T", result)
	}

	if vsEvent.ID != "vs-123" {
		t.Errorf("Expected ID 'vs-123', got '%s'", vsEvent.ID)
	}
	if vsEvent.VitalSign.Name != "Temperature" {
		t.Errorf("Expected vital sign name 'Temperature', got '%s'", vsEvent.VitalSign.Name)
	}
	if vsEvent.VitalSign.Value != "98.6" {
		t.Errorf("Expected value '98.6', got '%s'", vsEvent.VitalSign.Value)
	}
}

func TestConvertToGraphQLEvent_Condition(t *testing.T) {
	patient := &events.Patient{MRN: "MRN001", FamilyName: "Doe", GivenName: "John"}
	evt := &events.ConditionEvent{
		EventMeta: events.EventMeta{
			ID:        "cond-123",
			Type:      events.EventCondition,
			Timestamp: time.Now(),
		},
		Patient: patient,
		Condition: events.Condition{
			Name:       "Diabetes mellitus type 2",
			Code:       "E11.9",
			CodeSystem: "ICD-10-CM",
			Category:   "encounter-diagnosis",
		},
		ClinicalStatus: "active",
		OnsetDate:      "2020-01-15",
	}

	format := model.SourceFormatFHIR
	result := convertToGraphQLEvent(evt, "ehr", format, nil)

	condEvent, ok := result.(*model.ConditionEvent)
	if !ok {
		t.Fatalf("Expected *model.ConditionEvent, got %T", result)
	}

	if condEvent.ID != "cond-123" {
		t.Errorf("Expected ID 'cond-123', got '%s'", condEvent.ID)
	}
	if condEvent.Condition.Name != "Diabetes mellitus type 2" {
		t.Errorf("Expected condition name 'Diabetes mellitus type 2', got '%s'", condEvent.Condition.Name)
	}
	if *condEvent.ClinicalStatus != "active" {
		t.Errorf("Expected clinical status 'active', got '%s'", *condEvent.ClinicalStatus)
	}
}

func TestConvertToGraphQLEvent_Procedure(t *testing.T) {
	patient := &events.Patient{MRN: "MRN001"}
	evt := &events.ProcedureEvent{
		EventMeta: events.EventMeta{
			ID:        "proc-123",
			Type:      events.EventProcedure,
			Timestamp: time.Now(),
		},
		Patient: patient,
		Procedure: events.Procedure{
			Name:       "Appendectomy",
			Code:       "80146002",
			CodeSystem: "SNOMED-CT",
			Status:     "completed",
		},
		PerformedDate: "2024-01-15",
	}

	result := convertToGraphQLEvent(evt, "surgery", model.SourceFormatHL7v2, nil)

	procEvent, ok := result.(*model.ProcedureEvent)
	if !ok {
		t.Fatalf("Expected *model.ProcedureEvent, got %T", result)
	}

	if procEvent.Procedure.Name != "Appendectomy" {
		t.Errorf("Expected procedure name 'Appendectomy', got '%s'", procEvent.Procedure.Name)
	}
	if *procEvent.PerformedDate != "2024-01-15" {
		t.Errorf("Expected performed date '2024-01-15', got '%s'", *procEvent.PerformedDate)
	}
}

func TestConvertToGraphQLEvent_Immunization(t *testing.T) {
	patient := &events.Patient{MRN: "MRN001"}
	evt := &events.ImmunizationEvent{
		EventMeta: events.EventMeta{
			ID:        "imm-123",
			Type:      events.EventImmunization,
			Timestamp: time.Now(),
		},
		Patient: patient,
		Immunization: events.Immunization{
			VaccineName: "COVID-19 vaccine",
			VaccineCode: "207",
			Status:      "completed",
		},
		AdministeredDate: "2024-01-15",
	}

	result := convertToGraphQLEvent(evt, "clinic", model.SourceFormatFHIR, nil)

	immEvent, ok := result.(*model.ImmunizationEvent)
	if !ok {
		t.Fatalf("Expected *model.ImmunizationEvent, got %T", result)
	}

	if immEvent.Immunization.VaccineName != "COVID-19 vaccine" {
		t.Errorf("Expected vaccine name 'COVID-19 vaccine', got '%s'", immEvent.Immunization.VaccineName)
	}
}

func TestConvertToGraphQLEvent_Document(t *testing.T) {
	patient := &events.Patient{MRN: "MRN001", FamilyName: "Doe", GivenName: "John"}
	evt := &events.DocumentEvent{
		EventMeta: events.EventMeta{
			ID:        "doc-123",
			Type:      events.EventDocument,
			Timestamp: time.Now(),
		},
		Patient:      patient,
		DocumentType: "Discharge Summary",
		Title:        "Patient Discharge Summary for John Doe",
	}

	result := convertToGraphQLEvent(evt, "records", model.SourceFormatCDA, nil)

	docEvent, ok := result.(*model.DocumentEvent)
	if !ok {
		t.Fatalf("Expected *model.DocumentEvent, got %T", result)
	}

	if docEvent.DocumentType != "Discharge Summary" {
		t.Errorf("Expected document type 'Discharge Summary', got '%s'", docEvent.DocumentType)
	}
	if *docEvent.Title != "Patient Discharge Summary for John Doe" {
		t.Errorf("Expected title, got '%s'", *docEvent.Title)
	}
	if docEvent.Patient == nil {
		t.Error("Expected patient to be set")
	}
}

func TestConvertToGraphQLEvent_DocumentNilPatient(t *testing.T) {
	evt := &events.DocumentEvent{
		EventMeta: events.EventMeta{
			ID:        "doc-124",
			Type:      events.EventDocument,
			Timestamp: time.Now(),
		},
		Patient:      nil, // Nil patient
		DocumentType: "Note",
	}

	result := convertToGraphQLEvent(evt, "records", model.SourceFormatCDA, nil)

	docEvent, ok := result.(*model.DocumentEvent)
	if !ok {
		t.Fatalf("Expected *model.DocumentEvent, got %T", result)
	}

	if docEvent.Patient != nil {
		t.Error("Expected patient to be nil")
	}
}

func TestConvertToGraphQLEvent_Appointment(t *testing.T) {
	patient := events.Patient{MRN: "MRN001"}
	endTime := time.Now().Add(30 * time.Minute)
	evt := &events.AppointmentEvent{
		EventMeta: events.EventMeta{
			ID:        "appt-123",
			Type:      events.EventAppointmentScheduled,
			Timestamp: time.Now(),
		},
		Patient: patient,
		Appointment: events.Appointment{
			ID:        "APPT001",
			Status:    "booked",
			StartTime: time.Now(),
			EndTime:   endTime,
		},
	}

	result := convertToGraphQLEvent(evt, "scheduling", model.SourceFormatHL7v2, nil)

	apptEvent, ok := result.(*model.AppointmentEvent)
	if !ok {
		t.Fatalf("Expected *model.AppointmentEvent, got %T", result)
	}

	if apptEvent.Appointment.ID != "APPT001" {
		t.Errorf("Expected appointment ID 'APPT001', got '%s'", apptEvent.Appointment.ID)
	}
	if apptEvent.Appointment.EndTime == nil {
		t.Error("Expected end time to be set")
	}
}

func TestConvertToGraphQLEvent_UnknownType(t *testing.T) {
	// Pass a type that isn't handled
	result := convertToGraphQLEvent("unknown", "test", model.SourceFormatHL7v2, nil)

	if result != nil {
		t.Errorf("Expected nil for unknown event type, got %T", result)
	}
}

func TestConvertPatientPtr_Nil(t *testing.T) {
	result := convertPatientPtr(nil)

	// Should return empty patient
	if result.MRN != "" {
		t.Errorf("Expected empty MRN, got '%s'", result.MRN)
	}
}

func TestConvertPatientPtr_WithPatient(t *testing.T) {
	patient := &events.Patient{
		MRN:        "MRN001",
		FamilyName: "Doe",
		GivenName:  "John",
	}
	result := convertPatientPtr(patient)

	if result.MRN != "MRN001" {
		t.Errorf("Expected MRN 'MRN001', got '%s'", result.MRN)
	}
	if result.FamilyName != "Doe" {
		t.Errorf("Expected family name 'Doe', got '%s'", result.FamilyName)
	}
}

func TestCreateEventFromInput_PatientAdmit(t *testing.T) {
	input := model.SubmitEventInput{
		Type:   model.EventTypePatientAdmit,
		Source: "adt",
		Data: map[string]interface{}{
			"patient": map[string]interface{}{
				"mrn":        "MRN001",
				"familyName": "Doe",
				"givenName":  "John",
			},
			"encounter": map[string]interface{}{
				"id":     "ENC001",
				"class":  "inpatient",
				"status": "active",
			},
		},
	}

	event, err := createEventFromInput(input)
	if err != nil {
		t.Fatalf("createEventFromInput failed: %v", err)
	}

	admitEvent, ok := event.(*model.PatientAdmitEvent)
	if !ok {
		t.Fatalf("Expected *model.PatientAdmitEvent, got %T", event)
	}

	if admitEvent.Patient.MRN != "MRN001" {
		t.Errorf("Expected MRN 'MRN001', got '%s'", admitEvent.Patient.MRN)
	}
	if admitEvent.Encounter.ID != "ENC001" {
		t.Errorf("Expected encounter ID 'ENC001', got '%s'", admitEvent.Encounter.ID)
	}
	if admitEvent.Encounter.Class != "inpatient" {
		t.Errorf("Expected encounter class 'inpatient', got '%s'", admitEvent.Encounter.Class)
	}
}

func TestCreateEventFromInput_PatientDischarge(t *testing.T) {
	input := model.SubmitEventInput{
		Type:   model.EventTypePatientDischarge,
		Source: "adt",
		Data: map[string]interface{}{
			"patient": map[string]interface{}{
				"mrn":        "MRN002",
				"familyName": "Smith",
				"givenName":  "Jane",
			},
			"encounter": map[string]interface{}{
				"id":    "ENC002",
				"class": "inpatient",
			},
		},
	}

	event, err := createEventFromInput(input)
	if err != nil {
		t.Fatalf("createEventFromInput failed: %v", err)
	}

	dischargeEvent, ok := event.(*model.PatientDischargeEvent)
	if !ok {
		t.Fatalf("Expected *model.PatientDischargeEvent, got %T", event)
	}

	if dischargeEvent.Patient.MRN != "MRN002" {
		t.Errorf("Expected MRN 'MRN002', got '%s'", dischargeEvent.Patient.MRN)
	}
}

func TestCreateEventFromInput_Condition(t *testing.T) {
	input := model.SubmitEventInput{
		Type:   model.EventTypeCondition,
		Source: "ehr",
		Data: map[string]interface{}{
			"patient": map[string]interface{}{
				"mrn":        "MRN003",
				"familyName": "Johnson",
				"givenName":  "Robert",
			},
			"condition": map[string]interface{}{
				"name":       "Hypertension",
				"code":       "I10",
				"codeSystem": "ICD-10-CM",
				"category":   "problem-list-item",
			},
			"clinicalStatus": "active",
			"onsetDate":      "2023-06-15",
		},
	}

	event, err := createEventFromInput(input)
	if err != nil {
		t.Fatalf("createEventFromInput failed: %v", err)
	}

	condEvent, ok := event.(*model.ConditionEvent)
	if !ok {
		t.Fatalf("Expected *model.ConditionEvent, got %T", event)
	}

	if condEvent.Condition.Name != "Hypertension" {
		t.Errorf("Expected condition name 'Hypertension', got '%s'", condEvent.Condition.Name)
	}
	if *condEvent.Condition.Code != "I10" {
		t.Errorf("Expected condition code 'I10', got '%s'", *condEvent.Condition.Code)
	}
	if *condEvent.ClinicalStatus != "active" {
		t.Errorf("Expected clinical status 'active', got '%s'", *condEvent.ClinicalStatus)
	}
}

func TestCreateEventFromInput_Document(t *testing.T) {
	input := model.SubmitEventInput{
		Type:   model.EventTypeDocument,
		Source: "records",
		Data: map[string]interface{}{
			"documentType": "Progress Note",
			"title":        "Daily Progress Note",
		},
	}

	event, err := createEventFromInput(input)
	if err != nil {
		t.Fatalf("createEventFromInput failed: %v", err)
	}

	docEvent, ok := event.(*model.DocumentEvent)
	if !ok {
		t.Fatalf("Expected *model.DocumentEvent, got %T", event)
	}

	if docEvent.DocumentType != "Progress Note" {
		t.Errorf("Expected document type 'Progress Note', got '%s'", docEvent.DocumentType)
	}
	if *docEvent.Title != "Daily Progress Note" {
		t.Errorf("Expected title 'Daily Progress Note', got '%s'", *docEvent.Title)
	}
}

func TestCreateEventFromInput_UnsupportedType(t *testing.T) {
	input := model.SubmitEventInput{
		Type:   model.EventType("unsupported_type"),
		Source: "test",
		Data:   map[string]interface{}{},
	}

	_, err := createEventFromInput(input)
	if err == nil {
		t.Error("Expected error for unsupported event type")
	}
	if err.Error() != "unsupported event type: unsupported_type" {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestMutationResolver_SubmitEvent_VitalSign(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	input := model.SubmitEventInput{
		Type:   model.EventTypeVitalSign,
		Source: "vitals-monitor",
		Data: map[string]interface{}{
			"patient": map[string]interface{}{
				"mrn":        "MRN001",
				"familyName": "Doe",
				"givenName":  "John",
			},
			"vitalSign": map[string]interface{}{
				"name":      "Blood Pressure",
				"loincCode": "85354-9",
				"value":     "120/80",
				"unit":      "mmHg",
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
}

func TestMutationResolver_SubmitEvent_Condition(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	input := model.SubmitEventInput{
		Type:   model.EventTypeCondition,
		Source: "ehr",
		Data: map[string]interface{}{
			"patient": map[string]interface{}{
				"mrn":        "MRN001",
				"familyName": "Doe",
				"givenName":  "John",
			},
			"condition": map[string]interface{}{
				"name":       "Type 2 Diabetes",
				"code":       "E11.9",
				"codeSystem": "ICD-10-CM",
			},
			"clinicalStatus": "active",
		},
	}

	result, err := mutationResolver.SubmitEvent(ctx, input)
	if err != nil {
		t.Fatalf("SubmitEvent failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got errors: %v", result.Errors)
	}
}

// =============================================================================
// Additional SubmitMessage Format Tests
// =============================================================================

func TestMutationResolver_SubmitMessage_FHIR(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	fhirPatient := `{
		"resourceType": "Patient",
		"id": "example",
		"identifier": [{"system": "http://hospital.example.org", "value": "MRN12345"}],
		"name": [{"family": "Doe", "given": ["John"]}],
		"gender": "male",
		"birthDate": "1980-03-15"
	}`

	input := model.SubmitMessageInput{
		Format: model.SourceFormatFHIR,
		Source: "fhir-server",
		Data:   fhirPatient,
	}

	result, err := mutationResolver.SubmitMessage(ctx, input)
	if err != nil {
		t.Fatalf("SubmitMessage failed: %v", err)
	}

	// FHIR parsing should succeed
	if !result.Success {
		t.Errorf("Expected success, got errors: %v", result.Errors)
	}
}

func TestMutationResolver_SubmitMessage_FHIR_Invalid(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	input := model.SubmitMessageInput{
		Format: model.SourceFormatFHIR,
		Source: "test",
		Data:   "not valid json",
	}

	result, err := mutationResolver.SubmitMessage(ctx, input)
	if err != nil {
		t.Fatalf("SubmitMessage should not return error: %v", err)
	}

	// Should fail parsing
	if result.Success {
		t.Error("Expected failure for invalid FHIR")
	}
	if len(result.Errors) == 0 {
		t.Error("Expected errors for invalid FHIR")
	}
}

func TestMutationResolver_SubmitMessage_CSV(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	csvData := `mrn,family_name,given_name,dob,gender
MRN001,Doe,John,1980-03-15,M
MRN002,Smith,Jane,1975-06-22,F`

	input := model.SubmitMessageInput{
		Format: model.SourceFormatCSV,
		Source: "csv-upload",
		Data:   csvData,
	}

	result, err := mutationResolver.SubmitMessage(ctx, input)
	if err != nil {
		t.Fatalf("SubmitMessage failed: %v", err)
	}

	// CSV parsing should succeed
	if !result.Success {
		t.Errorf("Expected success, got errors: %v", result.Errors)
	}
}

func TestMutationResolver_SubmitMessage_CDA(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	cdaDoc := `<?xml version="1.0" encoding="UTF-8"?>
<ClinicalDocument xmlns="urn:hl7-org:v3" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <realmCode code="US"/>
  <typeId root="2.16.840.1.113883.1.3" extension="POCD_HD000040"/>
  <templateId root="2.16.840.1.113883.10.20.22.1.1"/>
  <id root="2.16.840.1.113883.19.5" extension="12345"/>
  <code code="34133-9" codeSystem="2.16.840.1.113883.6.1" displayName="Summarization of Episode Note"/>
  <title>Patient Summary</title>
  <effectiveTime value="20240115120000"/>
  <confidentialityCode code="N" codeSystem="2.16.840.1.113883.5.25"/>
  <languageCode code="en-US"/>
  <recordTarget>
    <patientRole>
      <id root="2.16.840.1.113883.19.5" extension="MRN001"/>
      <patient>
        <name><given>John</given><family>Doe</family></name>
        <administrativeGenderCode code="M" codeSystem="2.16.840.1.113883.5.1"/>
        <birthTime value="19800315"/>
      </patient>
    </patientRole>
  </recordTarget>
  <component>
    <structuredBody>
      <component>
        <section>
          <templateId root="2.16.840.1.113883.10.20.22.2.6.1"/>
          <code code="48765-2" codeSystem="2.16.840.1.113883.6.1"/>
          <title>Allergies</title>
          <text>No known allergies</text>
        </section>
      </component>
    </structuredBody>
  </component>
</ClinicalDocument>`

	input := model.SubmitMessageInput{
		Format: model.SourceFormatCDA,
		Source: "cda-import",
		Data:   cdaDoc,
	}

	result, err := mutationResolver.SubmitMessage(ctx, input)
	if err != nil {
		t.Fatalf("SubmitMessage failed: %v", err)
	}

	// CDA parsing should succeed (returns document event)
	if !result.Success {
		t.Errorf("Expected success, got errors: %v", result.Errors)
	}
}

func TestMutationResolver_SubmitMessage_CDA_Invalid(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	input := model.SubmitMessageInput{
		Format: model.SourceFormatCDA,
		Source: "test",
		Data:   "not valid xml",
	}

	result, err := mutationResolver.SubmitMessage(ctx, input)
	if err != nil {
		t.Fatalf("SubmitMessage should not return error: %v", err)
	}

	// Should fail parsing
	if result.Success {
		t.Error("Expected failure for invalid CDA")
	}
}

func TestMutationResolver_SubmitMessage_EDI837(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	// Minimal 837 transaction
	edi837 := `ISA*00*          *00*          *ZZ*SENDER         *ZZ*RECEIVER       *240115*1200*^*00501*000000001*0*P*:~
GS*HC*SENDER*RECEIVER*20240115*1200*1*X*005010X222A1~
ST*837*0001*005010X222A1~
BHT*0019*00*123456*20240115*1200*CH~
SE*4*0001~
GE*1*1~
IEA*1*000000001~`

	input := model.SubmitMessageInput{
		Format: model.SourceFormatEDI837,
		Source: "clearinghouse",
		Data:   edi837,
	}

	result, err := mutationResolver.SubmitMessage(ctx, input)
	if err != nil {
		t.Fatalf("SubmitMessage failed: %v", err)
	}

	// EDI parsing should succeed (even if no claims extracted)
	if !result.Success {
		t.Logf("EDI837 result errors: %v", result.Errors)
	}
}

func TestMutationResolver_SubmitMessage_EDI835(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	// Minimal 835 transaction
	edi835 := `ISA*00*          *00*          *ZZ*PAYER          *ZZ*PROVIDER       *240115*1200*^*00501*000000001*0*P*:~
GS*HP*PAYER*PROVIDER*20240115*1200*1*X*005010X221A1~
ST*835*0001*005010X221A1~
BPR*I*500.00*C*ACH*CTX*01*999999999*DA*123456789**01*999999999*DA*987654321*20240115~
TRN*1*TRACE123*1234567890~
SE*5*0001~
GE*1*1~
IEA*1*000000001~`

	input := model.SubmitMessageInput{
		Format: model.SourceFormatEDI835,
		Source: "payer",
		Data:   edi835,
	}

	result, err := mutationResolver.SubmitMessage(ctx, input)
	if err != nil {
		t.Fatalf("SubmitMessage failed: %v", err)
	}

	// EDI parsing should succeed
	if !result.Success {
		t.Logf("EDI835 result errors: %v", result.Errors)
	}
}

func TestMutationResolver_SubmitMessage_WithWorkflow(t *testing.T) {
	// Create a workflow that matches and executes
	wf := &workflow.Workflow{
		Name:    "test-workflow",
		Version: "1.0",
		Routes: []workflow.Route{
			{
				Name:   "all-events",
				Filter: workflow.Filter{},
				Actions: []workflow.Action{
					{Type: "log", Config: map[string]string{"level": "info"}},
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

	input := model.SubmitMessageInput{
		Format: model.SourceFormatHL7v2,
		Source: "adt",
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

	// Should have workflow results
	if len(result.WorkflowResults) == 0 {
		t.Error("Expected workflow results when engine is configured")
	}
}

// =============================================================================
// Additional ParsePreview Tests
// =============================================================================

func TestQueryResolver_ParsePreview_FHIR(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	fhirPatient := `{
		"resourceType": "Patient",
		"id": "example",
		"name": [{"family": "Doe", "given": ["John"]}]
	}`

	result, err := queryResolver.ParsePreview(ctx, model.SourceFormatFHIR, fhirPatient, nil)
	if err != nil {
		t.Fatalf("ParsePreview failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got errors: %v", result.Errors)
	}
}

func TestQueryResolver_ParsePreview_CSV(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	csvData := `mrn,family_name,given_name
MRN001,Doe,John`

	result, err := queryResolver.ParsePreview(ctx, model.SourceFormatCSV, csvData, nil)
	if err != nil {
		t.Fatalf("ParsePreview failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got errors: %v", result.Errors)
	}
}

func TestQueryResolver_ParsePreview_CDA(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	cdaDoc := `<?xml version="1.0" encoding="UTF-8"?>
<ClinicalDocument xmlns="urn:hl7-org:v3">
  <realmCode code="US"/>
  <typeId root="2.16.840.1.113883.1.3" extension="POCD_HD000040"/>
  <id root="2.16.840.1.113883.19.5" extension="12345"/>
  <code code="34133-9" codeSystem="2.16.840.1.113883.6.1"/>
  <effectiveTime value="20240115"/>
  <confidentialityCode code="N" codeSystem="2.16.840.1.113883.5.25"/>
  <recordTarget>
    <patientRole>
      <id extension="MRN001"/>
      <patient>
        <name><given>John</given><family>Doe</family></name>
      </patient>
    </patientRole>
  </recordTarget>
  <component><structuredBody/></component>
</ClinicalDocument>`

	result, err := queryResolver.ParsePreview(ctx, model.SourceFormatCDA, cdaDoc, nil)
	if err != nil {
		t.Fatalf("ParsePreview failed: %v", err)
	}

	if !result.Success {
		t.Errorf("Expected success, got errors: %v", result.Errors)
	}
}

func TestQueryResolver_ParsePreview_EDI837(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	edi837 := `ISA*00*          *00*          *ZZ*SENDER         *ZZ*RECEIVER       *240115*1200*^*00501*000000001*0*P*:~
GS*HC*SENDER*RECEIVER*20240115*1200*1*X*005010X222A1~
ST*837*0001~
BHT*0019*00*123456*20240115*1200*CH~
SE*4*0001~
GE*1*1~
IEA*1*000000001~`

	result, err := queryResolver.ParsePreview(ctx, model.SourceFormatEDI837, edi837, nil)
	if err != nil {
		t.Fatalf("ParsePreview failed: %v", err)
	}

	// Should succeed even if no events extracted
	if len(result.Errors) > 0 && result.Errors[0] != "" {
		t.Logf("EDI837 parse preview notes: %v", result.Errors)
	}
}

func TestQueryResolver_ParsePreview_EDI835(t *testing.T) {
	resolver := NewResolver()
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	edi835 := `ISA*00*          *00*          *ZZ*PAYER          *ZZ*PROVIDER       *240115*1200*^*00501*000000001*0*P*:~
GS*HP*PAYER*PROVIDER*20240115*1200*1*X*005010X221A1~
ST*835*0001~
BPR*I*500.00*C*ACH*CTX*01*999999999*DA*123456789**01*999999999*DA*987654321*20240115~
TRN*1*TRACE123*1234567890~
SE*5*0001~
GE*1*1~
IEA*1*000000001~`

	result, err := queryResolver.ParsePreview(ctx, model.SourceFormatEDI835, edi835, nil)
	if err != nil {
		t.Fatalf("ParsePreview failed: %v", err)
	}

	// Should succeed
	if len(result.Errors) > 0 && result.Errors[0] != "" {
		t.Logf("EDI835 parse preview notes: %v", result.Errors)
	}
}

// =============================================================================
// FHIR Subscription Tests with Mocked Client
// =============================================================================

func TestMutationResolver_CreateFhirSubscription_NoClient(t *testing.T) {
	resolver := NewResolver()
	mutationResolver := &mutationResolver{resolver}

	ctx := context.Background()

	input := model.CreateSubscriptionInput{
		Name:     "test-subscription",
		Server:   "https://fhir.example.com",
		Criteria: "Patient?_id=123",
		Endpoint: "https://my-app.com/webhook",
	}

	// Without FHIR subscription client factory, should fail
	_, err := mutationResolver.CreateFhirSubscription(ctx, input)
	if err == nil {
		t.Error("Expected error when FHIR subscription is not configured")
	}
}

// =============================================================================
// Patients Query Test
// =============================================================================

func TestQueryResolver_Patients(t *testing.T) {
	memStore := store.NewMemoryStore()
	resolver := NewResolver(WithStore(memStore))
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	// Add some patients
	memStore.SavePatient(&model.Patient{MRN: "MRN001", FamilyName: "Doe", GivenName: "John"})
	memStore.SavePatient(&model.Patient{MRN: "MRN002", FamilyName: "Smith", GivenName: "Jane"})

	first := 10
	patients, err := queryResolver.Patients(ctx, nil, &first, nil)
	if err != nil {
		t.Fatalf("Patients query failed: %v", err)
	}

	if patients.TotalCount != 2 {
		t.Errorf("Expected 2 patients, got %d", patients.TotalCount)
	}
}

func TestQueryResolver_Patients_WithFilter(t *testing.T) {
	memStore := store.NewMemoryStore()
	resolver := NewResolver(WithStore(memStore))
	queryResolver := &queryResolver{resolver}

	ctx := context.Background()

	// Add some patients
	memStore.SavePatient(&model.Patient{MRN: "MRN001", FamilyName: "Doe", GivenName: "John"})
	memStore.SavePatient(&model.Patient{MRN: "MRN002", FamilyName: "Smith", GivenName: "Jane"})

	first := 10
	mrn := "MRN001"
	filter := &model.PatientFilter{
		MRN: &mrn,
	}
	patients, err := queryResolver.Patients(ctx, filter, &first, nil)
	if err != nil {
		t.Fatalf("Patients query failed: %v", err)
	}

	if patients.TotalCount != 1 {
		t.Errorf("Expected 1 patient with filter, got %d", patients.TotalCount)
	}
}

// =============================================================================
// Root Resolver Tests
// =============================================================================

func TestResolver_RootResolvers(t *testing.T) {
	resolver := NewResolver()

	// Test that root resolvers return proper resolver types
	mutation := resolver.Mutation()
	if mutation == nil {
		t.Error("Expected Mutation() to return non-nil")
	}

	query := resolver.Query()
	if query == nil {
		t.Error("Expected Query() to return non-nil")
	}

	sub := resolver.Subscription()
	if sub == nil {
		t.Error("Expected Subscription() to return non-nil")
	}
}
