package session

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

func TestMemoryStoreSessionLifecycle(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()

	created, err := store.CreateSession(ctx, CreateSessionRequest{Name: "ADT onboarding", Tags: []string{"adt"}})
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}
	if created.Status != SessionStatusActive {
		t.Fatalf("status = %s, want active", created.Status)
	}

	name := "ADT onboarding v2"
	updated, err := store.UpdateSession(ctx, created.ID, UpdateSessionRequest{Name: &name})
	if err != nil {
		t.Fatalf("UpdateSession error: %v", err)
	}
	if updated.Name != name {
		t.Fatalf("name = %q, want %q", updated.Name, name)
	}

	archived, err := store.ArchiveSession(ctx, created.ID)
	if err != nil {
		t.Fatalf("ArchiveSession error: %v", err)
	}
	if archived.Status != SessionStatusArchived || archived.ArchivedAt == nil {
		t.Fatalf("archive state = %+v", archived)
	}

	active, err := store.ListSessions(ctx, ListSessionsOptions{})
	if err != nil {
		t.Fatalf("ListSessions error: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("active sessions = %d, want 0", len(active))
	}

	all, err := store.ListSessions(ctx, ListSessionsOptions{IncludeArchived: true})
	if err != nil {
		t.Fatalf("ListSessions include archived error: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("all sessions = %d, want 1", len(all))
	}
}

func TestMemoryStoreSamplesApplyPHIPolicy(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	session, err := store.CreateSession(ctx, CreateSessionRequest{Name: "PHI policy"})
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	raw := "MSH|^~\\&|APP|FAC|EHR|FAC|20240115120000||ADT^A01|MSG1|P|2.5\n" +
		"PID|1||123456789^^^HOSPITAL^MRN||DOE^JOHN||19800315|M|||123 MAIN ST^^ANYTOWN^VA^24101^USA||5551234567|||||123456789"
	sample, err := store.AddSample(ctx, session.ID, AddSampleRequest{
		Name:      "masked ADT",
		Format:    events.FormatHL7v2,
		Raw:       raw,
		PHIPolicy: PHIPolicyRedact,
	})
	if err != nil {
		t.Fatalf("AddSample error: %v", err)
	}
	if !sample.PHIRedacted {
		t.Fatal("expected sample to be marked redacted")
	}
	for _, leaked := range []string{"DOE", "JOHN", "123 MAIN", "5551234567"} {
		if strings.Contains(sample.Raw, leaked) {
			t.Fatalf("sample raw leaked %q: %s", leaked, sample.Raw)
		}
	}
}

func TestMemoryStoreArtifactDraftsVersion(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	session, err := store.CreateSession(ctx, CreateSessionRequest{Name: "drafts"})
	if err != nil {
		t.Fatalf("CreateSession error: %v", err)
	}

	draft, err := store.SaveArtifactDraft(ctx, session.ID, SaveArtifactDraftRequest{
		Kind:    ArtifactKindMappingProfile,
		Name:    "profile.yaml",
		Content: json.RawMessage(`{"version":1}`),
	})
	if err != nil {
		t.Fatalf("SaveArtifactDraft create error: %v", err)
	}
	if draft.Version != 1 {
		t.Fatalf("version = %d, want 1", draft.Version)
	}

	updated, err := store.SaveArtifactDraft(ctx, session.ID, SaveArtifactDraftRequest{
		ID:      draft.ID,
		Kind:    ArtifactKindMappingProfile,
		Name:    "profile.yaml",
		Content: json.RawMessage(`{"version":2}`),
	})
	if err != nil {
		t.Fatalf("SaveArtifactDraft update error: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("version = %d, want 2", updated.Version)
	}
}
