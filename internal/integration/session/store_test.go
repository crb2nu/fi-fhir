package session

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
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

// testExportRequest builds the minimum attributed export a store will accept.
func testExportRequest(sessionID string) ExportRequest {
	return ExportRequest{
		SessionID: sessionID,
		Principal: integration.Principal{
			ID:         "integration-engineer-1",
			Kind:       integration.PrincipalKindHuman,
			AuthMethod: "oidc",
			Roles:      []string{"graphql:operator"},
		},
		Reason: "slice acceptance test export",
	}
}

func TestExportRequestValidateRefusesUnattributedDisclosure(t *testing.T) {
	valid := testExportRequest("sess-1")
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid export request rejected: %v", err)
	}
	cases := map[string]func(ExportRequest) ExportRequest{
		"missing session":     func(r ExportRequest) ExportRequest { r.SessionID = "  "; return r },
		"missing principal":   func(r ExportRequest) ExportRequest { r.Principal.ID = ""; return r },
		"unknown kind":        func(r ExportRequest) ExportRequest { r.Principal.Kind = "robot"; return r },
		"missing auth method": func(r ExportRequest) ExportRequest { r.Principal.AuthMethod = ""; return r },
		"missing reason":      func(r ExportRequest) ExportRequest { r.Reason = "   "; return r },
		"oversized reason": func(r ExportRequest) ExportRequest {
			r.Reason = strings.Repeat("r", maxExportReasonBytes+1)
			return r
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			if err := mutate(testExportRequest("sess-1")).Validate(); !errors.Is(err, ErrInvalid) {
				t.Fatalf("Validate() = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestMemoryStoreExportRefusedBeforeAnyRecordIsWritten(t *testing.T) {
	ctx := context.Background()
	store := NewMemoryStore()
	sess, err := store.CreateSession(ctx, CreateSessionRequest{Name: "export attribution"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	unattributed := testExportRequest(sess.ID)
	unattributed.Reason = ""
	if _, err := store.ExportBundle(ctx, unattributed); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ExportBundle(no reason) = %v, want ErrInvalid", err)
	}
	exports, err := store.ListExports(ctx, sess.ID)
	if err != nil || len(exports) != 0 {
		t.Fatalf("refused export still recorded a row: %#v, %v", exports, err)
	}

	ungranted := testExportRequest(sess.ID)
	ungranted.IncludeRawPayload = true
	if _, err := store.ExportBundle(ctx, ungranted); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ExportBundle(raw, no grant) = %v, want ErrForbidden", err)
	}

	attributed := testExportRequest(sess.ID)
	attributed.IncludeRawPayload = true
	attributed.Principal.Roles = append(attributed.Principal.Roles, PHIExportRole)
	bundle, err := store.ExportBundle(ctx, attributed)
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	if bundle.Principal.ID != attributed.Principal.ID || bundle.Reason != attributed.Reason {
		t.Fatalf("export not attributed: %#v", bundle)
	}
	if !bundle.IncludeRawPayload {
		t.Fatal("export did not record the raw-payload disclosure")
	}
}
