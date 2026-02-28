//go:build integration

package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
)

func setupPgEventStore(t *testing.T) *PostgresEventStore {
	t.Helper()

	// Allow manual DSN via env var.
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		dsn = startPgContainer(t)
	}

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("ping: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	s := NewPostgresEventStore(db)
	if err := s.InitSchema(context.Background()); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	return s
}

func startPgContainer(t *testing.T) string {
	t.Helper()

	defer func() {
		if r := recover(); r != nil {
			if os.Getenv("CI") != "" {
				t.Fatalf("Docker/testcontainers panic in CI: %v", r)
			}
			t.Skipf("Docker not available, skipping: %v", r)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	container, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("testuser"),
		postgres.WithPassword("testpass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Skipf("Could not start postgres container: %v", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("ConnectionString: %v", err)
	}
	return connStr
}

func TestPostgresEventStore_SaveAndGet(t *testing.T) {
	s := setupPgEventStore(t)
	ctx := context.Background()

	event := &model.PatientAdmitEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "test-evt-001",
			Type:      model.EventTypePatientAdmit,
			Timestamp: time.Now().UTC().Truncate(time.Microsecond),
			Source:    "test-source",
		},
		Patient: model.Patient{
			MRN:        "MRN-001",
			FamilyName: "Smith",
			GivenName:  "John",
		},
		Encounter: model.Encounter{
			ID:    "enc-001",
			Class: "inpatient",
		},
	}

	id, err := s.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("SaveEvent: %v", err)
	}
	if id != "test-evt-001" {
		t.Fatalf("expected id test-evt-001, got %s", id)
	}

	got, err := s.GetEvent(ctx, "test-evt-001")
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if got.GetType() != model.EventTypePatientAdmit {
		t.Fatalf("expected PATIENT_ADMIT, got %s", got.GetType())
	}
	if got.GetSource() != "test-source" {
		t.Fatalf("expected source test-source, got %s", got.GetSource())
	}
}

func TestPostgresEventStore_GetEvent_NotFound(t *testing.T) {
	s := setupPgEventStore(t)
	ctx := context.Background()

	_, err := s.GetEvent(ctx, "nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent event")
	}
}

func TestPostgresEventStore_PatientUpsert(t *testing.T) {
	s := setupPgEventStore(t)
	ctx := context.Background()

	// Save an event with patient data.
	event := &model.PatientAdmitEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "pat-evt-001",
			Type:      model.EventTypePatientAdmit,
			Timestamp: time.Now().UTC().Truncate(time.Microsecond),
			Source:    "adt",
		},
		Patient: model.Patient{
			MRN:        "MRN-100",
			FamilyName: "Doe",
			GivenName:  "Jane",
		},
		Encounter: model.Encounter{ID: "enc-100", Class: "inpatient"},
	}

	_, err := s.SaveEvent(ctx, event)
	if err != nil {
		t.Fatalf("SaveEvent: %v", err)
	}

	// Retrieve patient.
	patient, err := s.GetPatient(ctx, "MRN-100")
	if err != nil {
		t.Fatalf("GetPatient: %v", err)
	}
	if patient.FamilyName != "Doe" {
		t.Fatalf("expected FamilyName Doe, got %s", patient.FamilyName)
	}
	if patient.GivenName != "Jane" {
		t.Fatalf("expected GivenName Jane, got %s", patient.GivenName)
	}

	// Update patient via another event.
	event2 := &model.PatientAdmitEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "pat-evt-002",
			Type:      model.EventTypePatientAdmit,
			Timestamp: time.Now().UTC().Truncate(time.Microsecond),
			Source:    "adt",
		},
		Patient: model.Patient{
			MRN:        "MRN-100",
			FamilyName: "Doe-Updated",
			GivenName:  "Jane",
		},
		Encounter: model.Encounter{ID: "enc-101", Class: "outpatient"},
	}

	_, err = s.SaveEvent(ctx, event2)
	if err != nil {
		t.Fatalf("SaveEvent: %v", err)
	}

	patient, err = s.GetPatient(ctx, "MRN-100")
	if err != nil {
		t.Fatalf("GetPatient after upsert: %v", err)
	}
	if patient.FamilyName != "Doe-Updated" {
		t.Fatalf("expected FamilyName Doe-Updated, got %s", patient.FamilyName)
	}
}

func TestPostgresEventStore_QueryEvents_Pagination(t *testing.T) {
	s := setupPgEventStore(t)
	ctx := context.Background()

	// Insert 5 events.
	for i := 0; i < 5; i++ {
		event := &model.LabResultEvent{
			BaseEventFields: model.BaseEventFields{
				ID:        fmt.Sprintf("page-evt-%03d", i),
				Type:      model.EventTypeLabResult,
				Timestamp: time.Now().UTC().Add(time.Duration(i) * time.Second).Truncate(time.Microsecond),
				Source:    "lab",
			},
			Patient: model.Patient{MRN: "MRN-PAGE", FamilyName: "Page", GivenName: "Test"},
			Test:    model.LabTest{Description: "CBC"},
			Result:  model.LabResult{Value: "normal"},
		}
		if _, err := s.SaveEvent(ctx, event); err != nil {
			t.Fatalf("SaveEvent[%d]: %v", i, err)
		}
	}

	// Query first 2.
	conn, err := s.QueryEvents(ctx, &model.EventFilter{
		Types: []model.EventType{model.EventTypeLabResult},
	}, 2, nil, nil)
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}
	if len(conn.Edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(conn.Edges))
	}
	if !conn.PageInfo.HasNextPage {
		t.Fatal("expected HasNextPage=true")
	}
	if conn.TotalCount < 5 {
		t.Fatalf("expected TotalCount>=5, got %d", conn.TotalCount)
	}

	// Query next page using cursor.
	conn2, err := s.QueryEvents(ctx, &model.EventFilter{
		Types: []model.EventType{model.EventTypeLabResult},
	}, 2, conn.PageInfo.EndCursor, nil)
	if err != nil {
		t.Fatalf("QueryEvents page 2: %v", err)
	}
	if len(conn2.Edges) != 2 {
		t.Fatalf("expected 2 edges on page 2, got %d", len(conn2.Edges))
	}
	if !conn2.PageInfo.HasPreviousPage {
		t.Fatal("expected HasPreviousPage=true on page 2")
	}
}

func TestPostgresEventStore_QueryPatients(t *testing.T) {
	s := setupPgEventStore(t)
	ctx := context.Background()

	// Insert events for two patients.
	for _, p := range []model.Patient{
		{MRN: "QP-001", FamilyName: "Anderson", GivenName: "Alice"},
		{MRN: "QP-002", FamilyName: "Brown", GivenName: "Bob"},
	} {
		event := &model.PatientAdmitEvent{
			BaseEventFields: model.BaseEventFields{
				ID:        "qp-" + p.MRN,
				Type:      model.EventTypePatientAdmit,
				Timestamp: time.Now().UTC().Truncate(time.Microsecond),
				Source:    "adt",
			},
			Patient:   p,
			Encounter: model.Encounter{ID: "qp-enc-" + p.MRN, Class: "inpatient"},
		}
		if _, err := s.SaveEvent(ctx, event); err != nil {
			t.Fatalf("SaveEvent: %v", err)
		}
	}

	// Query by family name.
	familyName := "ander"
	conn, err := s.QueryPatients(ctx, &model.PatientFilter{
		FamilyName: &familyName,
	}, 10, nil)
	if err != nil {
		t.Fatalf("QueryPatients: %v", err)
	}
	if conn.TotalCount < 1 {
		t.Fatalf("expected at least 1 patient, got %d", conn.TotalCount)
	}
}

func TestPostgresEventStore_Subscribe(t *testing.T) {
	s := setupPgEventStore(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := s.Subscribe(ctx, nil)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	event := &model.VitalSignEvent{
		BaseEventFields: model.BaseEventFields{
			ID:        "sub-evt-001",
			Type:      model.EventTypeVitalSign,
			Timestamp: time.Now().UTC().Truncate(time.Microsecond),
			Source:    "vitals",
		},
		Patient:   model.Patient{MRN: "SUB-MRN", FamilyName: "Sub", GivenName: "Test"},
		VitalSign: model.VitalSign{Name: "temperature", Value: "37.0"},
	}

	if _, err := s.SaveEvent(ctx, event); err != nil {
		t.Fatalf("SaveEvent: %v", err)
	}

	select {
	case got := <-ch:
		if got.GetID() != "sub-evt-001" {
			t.Fatalf("expected sub-evt-001, got %s", got.GetID())
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for subscription event")
	}
}
