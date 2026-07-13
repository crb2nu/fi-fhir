package processor_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestDefinitionRevisionResolverLoadsExactServerOwnedRevision(t *testing.T) {
	t.Parallel()

	revision := definitionRevision(t, "adt-http", "rev-7", "tenant-a", '1')
	raw := marshalDefinitionRevision(t, revision)
	loader := &fakeDefinitionRevisionLoader{raw: raw}
	resolver, err := processor.NewDefinitionRevisionResolver("tenant-a", loader)
	if err != nil {
		t.Fatalf("NewDefinitionRevisionResolver: %v", err)
	}

	resolved, err := resolver.Resolve(context.Background(), "tenant-a", revision.Reference())
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if resolved.Reference() != revision.Reference() || resolved.TenantID != revision.TenantID {
		t.Fatalf("resolved wrong revision: %#v", resolved)
	}
	if len(loader.calls) != 1 {
		t.Fatalf("loader calls = %d, want 1", len(loader.calls))
	}
	wantCall := definitionRevisionLoad{
		tenantID:     "tenant-a",
		definitionID: revision.DefinitionID,
		revisionID:   revision.RevisionID,
	}
	if loader.calls[0] != wantCall {
		t.Fatalf("loader lookup = %#v, want %#v", loader.calls[0], wantCall)
	}

	for index := range raw {
		raw[index] = '!'
	}
	if err := resolved.Validate(); err != nil {
		t.Fatalf("resolved revision changed with loader-owned bytes: %v", err)
	}
	resolved.Destinations[0].ArtifactID = "caller-mutated"
	if resolved.Reference() != revision.Reference() {
		t.Fatal("caller mutation changed immutable revision identity")
	}
	if revision.Destinations[0].ArtifactID == "caller-mutated" {
		t.Fatal("resolved revision aliased the independently constructed revision")
	}
	if got := string(loader.raw); !strings.HasPrefix(got, "!!!") {
		t.Fatal("test did not mutate loader-owned bytes")
	}
}

func TestDefinitionRevisionResolverRejectsWrongTenantBeforeLoad(t *testing.T) {
	t.Parallel()

	revision := definitionRevision(t, "adt-http", "rev-7", "tenant-a", '1')
	loader := &fakeDefinitionRevisionLoader{raw: marshalDefinitionRevision(t, revision)}
	resolver, err := processor.NewDefinitionRevisionResolver("tenant-a", loader)
	if err != nil {
		t.Fatalf("NewDefinitionRevisionResolver: %v", err)
	}

	_, err = resolver.Resolve(context.Background(), "tenant-b", revision.Reference())
	if !errors.Is(err, processor.ErrTenantMismatch) {
		t.Fatalf("Resolve error = %v, want tenant mismatch", err)
	}
	if len(loader.calls) != 0 {
		t.Fatalf("wrong tenant reached loader: %#v", loader.calls)
	}
}

func TestDefinitionRevisionResolverRejectsCallerInventedUndeployedRevision(t *testing.T) {
	t.Parallel()

	invented := definitionRevision(t, "caller-invented", "rev-1", "tenant-a", '2')
	loader := &fakeDefinitionRevisionLoader{err: fmt.Errorf("%w: private storage detail", processor.ErrDefinitionRevisionNotFound)}
	resolver, err := processor.NewDefinitionRevisionResolver("tenant-a", loader)
	if err != nil {
		t.Fatalf("NewDefinitionRevisionResolver: %v", err)
	}

	_, err = resolver.Resolve(context.Background(), "tenant-a", invented.Reference())
	if !errors.Is(err, processor.ErrDefinitionRevisionNotFound) {
		t.Fatalf("Resolve error = %v, want not found", err)
	}
	if err.Error() != processor.ErrDefinitionRevisionNotFound.Error() {
		t.Fatalf("not-found error leaked loader details: %q", err)
	}
	if len(loader.calls) != 1 || loader.calls[0].definitionID != invented.DefinitionID || loader.calls[0].revisionID != invented.RevisionID {
		t.Fatalf("invented lookup was not exact: %#v", loader.calls)
	}
}

func TestDefinitionRevisionResolverStrictlyDecodesStoredJSON(t *testing.T) {
	t.Parallel()

	revision := definitionRevision(t, "adt-http", "rev-7", "tenant-a", '1')
	unknownField := append(
		[]byte(`{"server_secret":"PHI-SENTINEL",`),
		marshalDefinitionRevision(t, revision)[1:]...,
	)
	tests := []struct {
		name string
		raw  []byte
	}{
		{name: "malformed", raw: []byte(`{"definition_id":`)},
		{name: "duplicate key", raw: []byte(`{"tenant_id":"tenant-a","tenant_id":"tenant-a"}`)},
		{name: "unknown field", raw: unknownField},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loader := &fakeDefinitionRevisionLoader{raw: tt.raw}
			resolver, err := processor.NewDefinitionRevisionResolver("tenant-a", loader)
			if err != nil {
				t.Fatalf("NewDefinitionRevisionResolver: %v", err)
			}
			_, err = resolver.Resolve(context.Background(), "tenant-a", revision.Reference())
			if !errors.Is(err, processor.ErrInvalidDefinitionRevisionContent) {
				t.Fatalf("Resolve error = %v, want invalid content", err)
			}
			for _, secret := range []string{"PHI-SENTINEL", "duplicate", "server_secret"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("invalid-content error leaked catalog bytes/details %q: %q", secret, err)
				}
			}
		})
	}
}

func TestDefinitionRevisionResolverRejectsOversizedContent(t *testing.T) {
	t.Parallel()

	revision := definitionRevision(t, "adt-http", "rev-7", "tenant-a", '1')
	loader := &fakeDefinitionRevisionLoader{raw: bytes.Repeat([]byte{'X'}, (1<<20)+1)}
	resolver, err := processor.NewDefinitionRevisionResolver("tenant-a", loader)
	if err != nil {
		t.Fatalf("NewDefinitionRevisionResolver: %v", err)
	}
	if _, err := resolver.Resolve(context.Background(), "tenant-a", revision.Reference()); !errors.Is(err, processor.ErrInvalidDefinitionRevisionContent) {
		t.Fatalf("Resolve error = %v, want invalid content", err)
	}
}

func TestDefinitionRevisionResolverRequiresRequestedReferenceAndTenant(t *testing.T) {
	t.Parallel()

	requested := definitionRevision(t, "adt-http", "rev-7", "tenant-a", '1')
	tests := []struct {
		name string
		got  integration.IntegrationDefinitionRevision
		want error
	}{
		{name: "definition ID", got: definitionRevision(t, "other-definition", "rev-7", "tenant-a", '1'), want: processor.ErrDefinitionRevisionReferenceMismatch},
		{name: "revision ID", got: definitionRevision(t, "adt-http", "rev-8", "tenant-a", '1'), want: processor.ErrDefinitionRevisionReferenceMismatch},
		{name: "digest", got: definitionRevision(t, "adt-http", "rev-7", "tenant-a", '9'), want: processor.ErrDefinitionRevisionReferenceMismatch},
		{name: "tenant", got: definitionRevision(t, "adt-http", "rev-7", "tenant-b", '1'), want: processor.ErrTenantMismatch},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loader := &fakeDefinitionRevisionLoader{raw: marshalDefinitionRevision(t, tt.got)}
			resolver, err := processor.NewDefinitionRevisionResolver("tenant-a", loader)
			if err != nil {
				t.Fatalf("NewDefinitionRevisionResolver: %v", err)
			}
			_, err = resolver.Resolve(context.Background(), "tenant-a", requested.Reference())
			if !errors.Is(err, tt.want) {
				t.Fatalf("Resolve error = %v, want %v", err, tt.want)
			}
			if strings.Contains(err.Error(), tt.got.DefinitionID) || strings.Contains(err.Error(), tt.got.Digest) {
				t.Fatalf("mismatch error leaked catalog identity: %q", err)
			}
		})
	}
}

func TestDefinitionRevisionResolverReturnsCatalogSafeLoaderErrors(t *testing.T) {
	t.Parallel()

	revision := definitionRevision(t, "adt-http", "rev-7", "tenant-a", '1')
	loader := &fakeDefinitionRevisionLoader{err: errors.New("sql connection failed for patient PHI-SENTINEL")}
	resolver, err := processor.NewDefinitionRevisionResolver("tenant-a", loader)
	if err != nil {
		t.Fatalf("NewDefinitionRevisionResolver: %v", err)
	}

	_, err = resolver.Resolve(context.Background(), "tenant-a", revision.Reference())
	if !errors.Is(err, processor.ErrDefinitionRevisionUnavailable) {
		t.Fatalf("Resolve error = %v, want unavailable", err)
	}
	if err.Error() != processor.ErrDefinitionRevisionUnavailable.Error() || strings.Contains(err.Error(), "PHI-SENTINEL") {
		t.Fatalf("loader error was not catalog safe: %q", err)
	}
}

func TestDefinitionRevisionResolverRejectsMalformedReferenceBeforeLoad(t *testing.T) {
	t.Parallel()

	revision := definitionRevision(t, "adt-http", "rev-7", "tenant-a", '1')
	loader := &fakeDefinitionRevisionLoader{raw: marshalDefinitionRevision(t, revision)}
	resolver, err := processor.NewDefinitionRevisionResolver("tenant-a", loader)
	if err != nil {
		t.Fatalf("NewDefinitionRevisionResolver: %v", err)
	}
	bad := revision.Reference()
	bad.Digest = strings.ToUpper(bad.Digest)

	_, err = resolver.Resolve(context.Background(), "tenant-a", bad)
	if !errors.Is(err, processor.ErrInvalidDefinitionRevisionReference) {
		t.Fatalf("Resolve error = %v, want invalid reference", err)
	}
	if len(loader.calls) != 0 {
		t.Fatalf("malformed reference reached loader: %#v", loader.calls)
	}
}

type definitionRevisionLoad struct {
	tenantID     string
	definitionID string
	revisionID   string
}

type fakeDefinitionRevisionLoader struct {
	raw   []byte
	err   error
	calls []definitionRevisionLoad
}

func (l *fakeDefinitionRevisionLoader) LoadDefinitionRevision(_ context.Context, tenantID, definitionID, revisionID string) ([]byte, error) {
	l.calls = append(l.calls, definitionRevisionLoad{tenantID: tenantID, definitionID: definitionID, revisionID: revisionID})
	if l.err != nil {
		return nil, l.err
	}
	return l.raw, nil
}

func definitionRevision(t *testing.T, definitionID, revisionID, tenantID string, semanticByte byte) integration.IntegrationDefinitionRevision {
	t.Helper()
	digest := func(value byte) string { return "sha256:" + strings.Repeat(string(value), 64) }
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: definitionID,
		RevisionID:   revisionID,
		TenantID:     tenantID,
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "source-adt", RevisionID: "source-1", Digest: digest(semanticByte)},
			SourceID:            "adt-east",
		},
		Format:   events.FormatHL7v2,
		Profile:  integration.ArtifactRevisionRef{ArtifactID: "profile-adt", RevisionID: "profile-1", Digest: digest('2')},
		Workflow: integration.ArtifactRevisionRef{ArtifactID: "workflow-adt", RevisionID: "workflow-1", Digest: digest('3')},
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "destination-fhir", RevisionID: "destination-1", Digest: digest('4')},
			Class:               integration.DestinationClassProduction,
		}},
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
		},
		Created: integration.AuditEnvelope{
			TenantID: tenantID,
			Principal: integration.Principal{
				ID:         "operator-1",
				Kind:       integration.PrincipalKindHuman,
				AuthMethod: "oidc",
				Roles:      []string{"integration_operator"},
			},
			Reason:     "publish definition revision",
			OccurredAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}
	return revision
}

func marshalDefinitionRevision(t *testing.T, revision integration.IntegrationDefinitionRevision) []byte {
	t.Helper()
	raw, err := json.Marshal(revision)
	if err != nil {
		t.Fatalf("marshal definition revision: %v", err)
	}
	return bytes.Clone(raw)
}
