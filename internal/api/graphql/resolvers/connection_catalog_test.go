package resolvers

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/vektah/gqlparser/v2/gqlerror"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestProjectConnectionNeverReturnsNullForANonNullField(t *testing.T) {
	projected := projectConnection(connection.Connection{Draft: connection.Draft{
		ID: "adt-mllp", Direction: connection.DirectionSource, Kind: connection.KindBatchSFTP, Version: 3,
	}})
	if projected.Spec == nil || projected.SecretBindings == nil || projected.References == nil ||
		projected.Runtime == nil || projected.CreatedBy == nil || projected.UpdatedBy == nil ||
		projected.CreatedBy.Roles == nil {
		t.Fatalf("a non-null field projected as nil: %+v", projected)
	}
	if projected.Runtime.Mounted || projected.Runtime.Role != nil || projected.Runtime.Detail != nil ||
		projected.Runtime.RevisionID != nil || projected.Runtime.Digest != nil {
		t.Fatalf("an unmounted connection reports a role: %+v", projected.Runtime)
	}
	if projected.Direction != model.ConnectionDirectionSource || projected.Kind != model.ConnectionKindBatchSftp ||
		projected.Version != 3 || projected.LatestRevision != nil || projected.Archived {
		t.Fatalf("projection = %+v", projected)
	}
}

func TestProjectConnectionCarriesTheDraft(t *testing.T) {
	archivedAt := time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC)
	principal := integration.Principal{ID: "engineer-1", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{connection.ReadRole}}
	document := []byte(`{"schema_version":"1","z_last":"kept in order","artifact_id":"dest-kafka"}`)
	projected := projectConnection(connection.Connection{
		Draft: connection.Draft{
			ID: "dest-kafka", Direction: connection.DirectionDestination, Kind: connection.KindKafka,
			Name: "Kafka", Spec: []byte(`{"kafka":{"topic":"t"},"class":"sandbox"}`), Version: 4, ArchivedAt: &archivedAt,
			SecretBindings: []integration.SecretBinding{{Name: "n", Reference: integration.SecretReference{Provider: "file", Key: "k"}}},
			Created:        integration.AuditEnvelope{Principal: principal, Reason: "create", OccurredAt: archivedAt.Add(-time.Hour)},
			Updated:        integration.AuditEnvelope{Principal: principal, Reason: "archive", OccurredAt: archivedAt},
		},
		LatestRevision: &connection.Revision{
			ArtifactID: "dest-kafka", RevisionID: "2", Digest: "sha256:abc", Direction: connection.DirectionDestination,
			Kind: connection.KindKafka, Document: document, CompiledFromVersion: 3,
			Created: integration.AuditEnvelope{Principal: principal, Reason: "compile", OccurredAt: archivedAt},
		},
		References: []connection.Reference{{DefinitionID: "d", RevisionID: "r", Digest: "sha256:abc", State: "deployed", Health: "healthy"}},
		Runtime: connection.RuntimeState{Mounted: true, Role: connection.RuntimeRoleDeliveryRegistry,
			Detail: "revision 2: registry", RevisionID: "2", Digest: "sha256:abc"},
	})
	if runtime := projected.Runtime; runtime.RevisionID == nil || *runtime.RevisionID != "2" ||
		runtime.Digest == nil || *runtime.Digest != "sha256:abc" {
		t.Fatalf("runtime = %+v, want revision 2 and its digest", runtime)
	}
	if !projected.Archived || projected.UpdatedReason != "archive" || projected.Spec["class"] != "sandbox" ||
		len(projected.SecretBindings) != 1 || projected.SecretBindings[0].Version != nil ||
		len(projected.References) != 1 || *projected.Runtime.Role != connection.RuntimeRoleDeliveryRegistry {
		t.Fatalf("projection = %+v", projected)
	}
	revision := projected.LatestRevision
	if revision == nil || revision.RevisionJSON != string(document) || revision.CompiledFromVersion != 3 ||
		revision.CreatedReason != "compile" || revision.Kind != model.ConnectionKindKafka {
		t.Fatalf("revision = %+v; revisionJson must be the stored bytes exactly", revision)
	}
}

func TestCatalogConnectionErrorIsInventorySafe(t *testing.T) {
	cases := map[error]string{
		connection.ErrUnauthenticated:   "authentication required",
		connection.ErrForbidden:         "connection catalog action forbidden",
		connection.ErrInvalidRequest:    "invalid connection catalog request",
		connection.ErrNotFound:          "connection not found",
		connection.ErrAlreadyExists:     "connection already exists",
		connection.ErrVersionConflict:   "connection version conflict",
		connection.ErrArchived:          "connection is archived",
		connection.ErrUnavailable:       "connection catalog unavailable",
		ErrConnectionCatalogUnavailable: "connection catalog unavailable",
		fmt.Errorf("load draft: %w", errors.New("dial tcp 10.0.0.9:5432: refused")): "connection catalog request failed",
	}
	for err, want := range cases {
		if got := catalogConnectionError(err); got.Error() != want {
			t.Errorf("catalogConnectionError(%v) = %q, want %q", err, got, want)
		}
	}
	if got := catalogConnectionError(context.Canceled); !errors.Is(got, context.Canceled) {
		t.Fatalf("cancellation was rewritten: %v", got)
	}

	specErr := &connection.SpecError{Problems: []connection.Problem{{
		Code: connection.CodeSecretValueForbidden, Path: "https.token", Message: "never a value",
	}}}
	var presented *gqlerror.Error
	if !errors.As(catalogConnectionError(fmt.Errorf("create: %w", specErr)), &presented) {
		t.Fatal("a spec rejection lost its extensions")
	}
	problems, _ := presented.Extensions["problems"].([]map[string]any)
	if presented.Extensions["code"] != connection.CodeSecretValueForbidden || len(problems) != 1 || problems[0]["path"] != "https.token" {
		t.Fatalf("extensions = %v", presented.Extensions)
	}
}

func TestConnectionResolversFailClosedWithoutACatalog(t *testing.T) {
	resolver := NewResolver()
	query := &queryResolver{resolver}
	mutation := &mutationResolver{resolver}
	ctx := context.Background()
	checks := map[string]error{}
	_, checks["connections"] = query.Connections(ctx, nil, nil)
	_, checks["connection"] = query.Connection(ctx, "c")
	_, checks["connectionRevisions"] = query.ConnectionRevisions(ctx, "c")
	_, checks["connectionRevision"] = query.ConnectionRevision(ctx, "c", "1")
	_, checks["createConnection"] = mutation.CreateConnection(ctx, model.CreateConnectionInput{})
	_, checks["updateConnection"] = mutation.UpdateConnection(ctx, model.UpdateConnectionInput{})
	_, checks["archiveConnection"] = mutation.ArchiveConnection(ctx, model.ConnectionCommandInput{})
	_, checks["compileConnection"] = mutation.CompileConnection(ctx, model.ConnectionCommandInput{})
	_, checks["validateConnectionSpec"] = mutation.ValidateConnectionSpec(ctx, model.ValidateConnectionSpecInput{})
	for field, err := range checks {
		if !errors.Is(err, ErrConnectionCatalogUnavailable) {
			t.Errorf("%s without a catalog: %v", field, err)
		}
	}
	if _, err := query.EngineRuntime(ctx); !errors.Is(err, ErrEngineRuntimeUnavailable) {
		t.Fatalf("engineRuntime without a description: %v", err)
	}
}

func TestWithEngineRuntimeKeepsItsOwnCopy(t *testing.T) {
	description := &connection.RuntimeDescription{Version: "v1", TenantID: "tenant-a"}
	resolver := NewResolver(WithEngineRuntime(description))
	description.Version = "mutated"
	if resolver.EngineRuntimeDescription.Version != "v1" {
		t.Fatal("the resolver aliases the description serve composed")
	}
	if NewResolver(WithEngineRuntime(nil)).EngineRuntimeDescription != nil {
		t.Fatal("a nil description became a non-nil one")
	}
}
