package store

import (
	"context"
	"testing"
	"time"

	"github.com/cblevins/fi-fhir/internal/api/graphql/model"
)

func TestMemoryStore_SaveAndGetEvent(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	event := &model.PatientAdmitEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "test-event-1",
			Type:      model.EventTypePatientAdmit,
			Timestamp: time.Now(),
			Source:    "test-source",
		},
		Patient: model.Patient{
			MRN:        "MRN001",
			FamilyName: "Doe",
			GivenName:  "John",
		},
		Encounter: model.Encounter{
			ID:    "enc-1",
			Class: "inpatient",
		},
	}

	// Save event
	id, err := store.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("SaveEvent failed: %v", err)
	}
	if id != "test-event-1" {
		t.Errorf("Expected ID 'test-event-1', got '%s'", id)
	}

	// Retrieve event
	retrieved, err := store.GetEvent(ctx, "test-event-1")
	if err != nil {
		t.Fatalf("GetEvent failed: %v", err)
	}

	admitEvent, ok := retrieved.(*model.PatientAdmitEvent)
	if !ok {
		t.Fatalf("Expected *model.PatientAdmitEvent, got %T", retrieved)
	}

	if admitEvent.Patient.MRN != "MRN001" {
		t.Errorf("Expected MRN 'MRN001', got '%s'", admitEvent.Patient.MRN)
	}
}

func TestMemoryStore_GetEventNotFound(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	_, err := store.GetEvent(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent event")
	}
}

func TestMemoryStore_QueryEvents_NoFilter(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Add some events
	for i := 0; i < 5; i++ {
		event := &model.LabResultEvent{
			BaseEventFields: model.BaseEventFields{
				ID:        "lab-" + string(rune('A'+i)),
				Type:      model.EventTypeLabResult,
				Timestamp: time.Now().Add(time.Duration(i) * time.Hour),
				Source:    "lab-system",
			},
			Patient: model.Patient{MRN: "MRN001"},
		}
		store.SaveEvent(ctx, event)
	}

	// Query all events
	conn, err := store.QueryEvents(ctx, nil, 100, nil, nil)
	if err != nil {
		t.Fatalf("QueryEvents failed: %v", err)
	}

	if conn.TotalCount != 5 {
		t.Errorf("Expected 5 events, got %d", conn.TotalCount)
	}
	if len(conn.Edges) != 5 {
		t.Errorf("Expected 5 edges, got %d", len(conn.Edges))
	}
}

func TestMemoryStore_QueryEvents_TypeFilter(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Add mixed events
	store.SaveEvent(ctx, &model.PatientAdmitEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "admit-1",
			Type:      model.EventTypePatientAdmit,
			Timestamp: time.Now(),
			Source:    "adt",
		},
		Patient: model.Patient{MRN: "MRN001"},
	})
	store.SaveEvent(ctx, &model.LabResultEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "lab-1",
			Type:      model.EventTypeLabResult,
			Timestamp: time.Now(),
			Source:    "lab",
		},
		Patient: model.Patient{MRN: "MRN001"},
	})

	// Filter by type
	filter := &model.EventFilter{
		Types: []model.EventType{model.EventTypeLabResult},
	}
	conn, err := store.QueryEvents(ctx, filter, 100, nil, nil)
	if err != nil {
		t.Fatalf("QueryEvents failed: %v", err)
	}

	if conn.TotalCount != 1 {
		t.Errorf("Expected 1 event, got %d", conn.TotalCount)
	}
}

func TestMemoryStore_QueryEvents_SourceFilter(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Add events from different sources
	store.SaveEvent(ctx, &model.LabResultEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "lab-1",
			Type:      model.EventTypeLabResult,
			Timestamp: time.Now(),
			Source:    "lab-system-1",
		},
		Patient: model.Patient{MRN: "MRN001"},
	})
	store.SaveEvent(ctx, &model.LabResultEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "lab-2",
			Type:      model.EventTypeLabResult,
			Timestamp: time.Now(),
			Source:    "lab-system-2",
		},
		Patient: model.Patient{MRN: "MRN002"},
	})

	// Filter by source
	filter := &model.EventFilter{
		Sources: []string{"lab-system-1"},
	}
	conn, err := store.QueryEvents(ctx, filter, 100, nil, nil)
	if err != nil {
		t.Fatalf("QueryEvents failed: %v", err)
	}

	if conn.TotalCount != 1 {
		t.Errorf("Expected 1 event, got %d", conn.TotalCount)
	}
}

func TestMemoryStore_QueryEvents_Pagination(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	// Add 10 events
	for i := 0; i < 10; i++ {
		event := &model.VitalSignEvent{
			BaseEventFields: model.BaseEventFields{
				ID:        "vs-" + string(rune('A'+i)),
				Type:      model.EventTypeVitalSign,
				Timestamp: time.Now().Add(time.Duration(i) * time.Hour),
				Source:    "vitals",
			},
			Patient: model.Patient{MRN: "MRN001"},
		}
		store.SaveEvent(ctx, event)
	}

	// Get first page (3 items)
	conn, err := store.QueryEvents(ctx, nil, 3, nil, nil)
	if err != nil {
		t.Fatalf("QueryEvents failed: %v", err)
	}

	if len(conn.Edges) != 3 {
		t.Errorf("Expected 3 edges, got %d", len(conn.Edges))
	}
	if !conn.PageInfo.HasNextPage {
		t.Error("Expected HasNextPage to be true")
	}
	if conn.PageInfo.HasPreviousPage {
		t.Error("Expected HasPreviousPage to be false")
	}

	// Get second page
	conn2, err := store.QueryEvents(ctx, nil, 3, conn.PageInfo.EndCursor, nil)
	if err != nil {
		t.Fatalf("QueryEvents page 2 failed: %v", err)
	}

	if len(conn2.Edges) != 3 {
		t.Errorf("Expected 3 edges on page 2, got %d", len(conn2.Edges))
	}
	if !conn2.PageInfo.HasNextPage {
		t.Error("Expected HasNextPage to be true on page 2")
	}
	if !conn2.PageInfo.HasPreviousPage {
		t.Error("Expected HasPreviousPage to be true on page 2")
	}
}

func TestMemoryStore_QueryEvents_Sorting(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	now := time.Now()
	store.SaveEvent(ctx, &model.LabResultEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "old-event",
			Type:      model.EventTypeLabResult,
			Timestamp: now.Add(-24 * time.Hour),
			Source:    "lab",
		},
	})
	store.SaveEvent(ctx, &model.LabResultEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "new-event",
			Type:      model.EventTypeLabResult,
			Timestamp: now,
			Source:    "lab",
		},
	})

	// Sort ascending
	orderBy := &model.EventOrderBy{
		Field:     model.EventOrderFieldTimestamp,
		Direction: model.OrderDirectionAsc,
	}
	conn, err := store.QueryEvents(ctx, nil, 100, nil, orderBy)
	if err != nil {
		t.Fatalf("QueryEvents failed: %v", err)
	}

	if conn.Edges[0].Node.GetID() != "old-event" {
		t.Errorf("Expected first event to be 'old-event', got '%s'", conn.Edges[0].Node.GetID())
	}

	// Sort descending
	orderBy.Direction = model.OrderDirectionDesc
	conn, err = store.QueryEvents(ctx, nil, 100, nil, orderBy)
	if err != nil {
		t.Fatalf("QueryEvents failed: %v", err)
	}

	if conn.Edges[0].Node.GetID() != "new-event" {
		t.Errorf("Expected first event to be 'new-event', got '%s'", conn.Edges[0].Node.GetID())
	}
}

func TestMemoryStore_SaveAndGetPatient(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	dob := time.Date(1980, 5, 15, 0, 0, 0, 0, time.UTC)
	patient := &model.Patient{
		MRN:         "MRN123",
		FamilyName:  "Smith",
		GivenName:   "Jane",
		DateOfBirth: &dob,
	}

	store.SavePatient(patient)

	retrieved, err := store.GetPatient(ctx, "MRN123")
	if err != nil {
		t.Fatalf("GetPatient failed: %v", err)
	}

	if retrieved.FamilyName != "Smith" {
		t.Errorf("Expected family name 'Smith', got '%s'", retrieved.FamilyName)
	}
}

func TestMemoryStore_QueryPatients(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	store.SavePatient(&model.Patient{MRN: "MRN001", FamilyName: "Doe", GivenName: "John"})
	store.SavePatient(&model.Patient{MRN: "MRN002", FamilyName: "Doe", GivenName: "Jane"})
	store.SavePatient(&model.Patient{MRN: "MRN003", FamilyName: "Smith", GivenName: "Bob"})

	// Filter by family name
	familyName := "Doe"
	filter := &model.PatientFilter{FamilyName: &familyName}
	conn, err := store.QueryPatients(ctx, filter, 100, nil)
	if err != nil {
		t.Fatalf("QueryPatients failed: %v", err)
	}

	if conn.TotalCount != 2 {
		t.Errorf("Expected 2 patients, got %d", conn.TotalCount)
	}
}

func TestMemoryStore_Subscribe(t *testing.T) {
	store := NewMemoryStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to all events
	eventCh, err := store.Subscribe(ctx, nil)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Save an event
	event := &model.LabResultEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "sub-test",
			Type:      model.EventTypeLabResult,
			Timestamp: time.Now(),
			Source:    "test",
		},
	}
	store.SaveEvent(ctx, event)

	// Wait for event with timeout
	select {
	case received := <-eventCh:
		if received.GetID() != "sub-test" {
			t.Errorf("Expected event ID 'sub-test', got '%s'", received.GetID())
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for subscribed event")
	}
}

func TestMemoryStore_SubscribeWithFilter(t *testing.T) {
	store := NewMemoryStore()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Subscribe to lab results only
	filter := &model.EventFilter{
		Types: []model.EventType{model.EventTypeLabResult},
	}
	eventCh, err := store.Subscribe(ctx, filter)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	// Save a non-matching event (should not be received)
	store.SaveEvent(ctx, &model.PatientAdmitEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "admit-1",
			Type:      model.EventTypePatientAdmit,
			Timestamp: time.Now(),
			Source:    "adt",
		},
	})

	// Save a matching event
	store.SaveEvent(ctx, &model.LabResultEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "lab-1",
			Type:      model.EventTypeLabResult,
			Timestamp: time.Now(),
			Source:    "lab",
		},
	})

	// Should receive only the lab event
	select {
	case received := <-eventCh:
		if received.GetID() != "lab-1" {
			t.Errorf("Expected event ID 'lab-1', got '%s'", received.GetID())
		}
	case <-time.After(time.Second):
		t.Error("Timeout waiting for subscribed event")
	}
}
