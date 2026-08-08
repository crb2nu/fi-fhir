package operator

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const testTenant = "tenant-a"

// countingRecovery records every delegated delivery write so an authorization
// test can prove a refused request never reached durable machinery.
type countingRecovery struct{ calls int }

func (r *countingRecovery) Replay(context.Context, string, string, delivery.Operation) (string, error) {
	r.calls++
	return "attempt-a", nil
}

func (r *countingRecovery) Resubmit(context.Context, string, string, delivery.Operation) (string, error) {
	r.calls++
	return "attempt-child", nil
}

func (r *countingRecovery) Discard(context.Context, string, string, delivery.Operation) (string, error) {
	r.calls++
	return "attempt-a", nil
}

type countingCatalog struct {
	calls    int
	snapshot lifecycle.Snapshot
	err      error
}

func (c *countingCatalog) command() (lifecycle.Snapshot, error) {
	c.calls++
	return c.snapshot, c.err
}

func (c *countingCatalog) Deploy(context.Context, lifecycle.Command) (lifecycle.Snapshot, error) {
	return c.command()
}
func (c *countingCatalog) Pause(context.Context, lifecycle.Command) (lifecycle.Snapshot, error) {
	return c.command()
}
func (c *countingCatalog) Resume(context.Context, lifecycle.Command) (lifecycle.Snapshot, error) {
	return c.command()
}
func (c *countingCatalog) Retire(context.Context, lifecycle.Command) (lifecycle.Snapshot, error) {
	return c.command()
}
func (c *countingCatalog) ListSnapshots(context.Context, string, int) ([]lifecycle.Snapshot, error) {
	c.calls++
	return []lifecycle.Snapshot{c.snapshot}, c.err
}
func (c *countingCatalog) ListEvents(context.Context, string, string, string) ([]lifecycle.EventRecord, error) {
	c.calls++
	return nil, c.err
}

func newTestService(t *testing.T) (*Service, *countingRecovery, *countingCatalog) {
	t.Helper()
	// A lazily-opened handle is enough: every assertion here refuses the
	// request before any statement is issued.
	db, err := sql.Open("postgres", "postgres://unused:unused@127.0.0.1:1/unused?sslmode=disable")
	if err != nil {
		t.Fatalf("open placeholder database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	reads, err := NewPostgresReadStore(db)
	if err != nil {
		t.Fatalf("NewPostgresReadStore: %v", err)
	}
	recovery := &countingRecovery{}
	catalog := &countingCatalog{snapshot: lifecycle.Snapshot{
		TenantID: testTenant,
		State:    integration.DeploymentStatePaused,
		Version:  3,
		Health:   integration.DeploymentHealthUnknown,
		Updated: integration.AuditEnvelope{
			TenantID:   testTenant,
			Principal:  operatorPrincipal(),
			Reason:     "destination outage",
			OccurredAt: time.Unix(0, 0),
		},
	}}
	service, err := NewService(reads, recovery, catalog, testTenant)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service, recovery, catalog
}

func operatorPrincipal(extraRoles ...string) integration.Principal {
	roles := append([]string{ReadRole, delivery.OperatorRole, DeploymentOperatorRole}, extraRoles...)
	return integration.Principal{
		ID:         "operator-a",
		Kind:       integration.PrincipalKindHuman,
		AuthMethod: "oidc",
		Roles:      roles,
	}
}

func securityContext(tenantID string, roles ...string) integration.SecurityContext {
	return integration.SecurityContext{
		TenantID: tenantID,
		Principal: integration.Principal{
			ID:         "operator-a",
			Kind:       integration.PrincipalKindHuman,
			AuthMethod: "oidc",
			Roles:      roles,
		},
	}
}

func TestControlActionsFailClosedBeforeDurableMachinery(t *testing.T) {
	request := ControlRequest{
		AttemptID:      "attempt-a",
		Reason:         "destination repaired",
		IdempotencyKey: "replay-1",
	}
	command := DeploymentCommand{
		DefinitionID:    "definition-a",
		RevisionID:      "revision-a",
		ExpectedVersion: 2,
		Reason:          "destination outage",
	}

	tests := []struct {
		name     string
		security *integration.SecurityContext
		wantErr  error
	}{
		{name: "unauthenticated", security: nil, wantErr: ErrUnauthenticated},
		{
			name:     "cross tenant identity",
			security: pointer(securityContext("tenant-b", ReadRole, delivery.OperatorRole, DeploymentOperatorRole)),
			wantErr:  ErrForbidden,
		},
		{
			name:     "read role only",
			security: pointer(securityContext(testTenant, ReadRole)),
			wantErr:  ErrForbidden,
		},
		{
			name:     "unprivileged role",
			security: pointer(securityContext(testTenant, "integration:preview")),
			wantErr:  ErrForbidden,
		},
		{
			name:     "delivery role without read role",
			security: pointer(securityContext(testTenant, delivery.OperatorRole)),
			wantErr:  ErrForbidden,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service, recovery, catalog := newTestService(t)
			ctx := context.Background()
			if tt.security != nil {
				ctx = requestsecurity.WithSecurityContext(ctx, *tt.security)
			}
			for action, run := range map[string]func() error{
				"replay":   func() error { _, err := service.ReplayDelivery(ctx, request); return err },
				"resubmit": func() error { _, err := service.ResubmitMessage(ctx, request); return err },
				"discard":  func() error { _, err := service.DiscardDeadLetter(ctx, request); return err },
				"pause":    func() error { _, err := service.PauseDeployment(ctx, command); return err },
				"resume":   func() error { _, err := service.ResumeDeployment(ctx, command); return err },
				"retire":   func() error { _, err := service.RetireDeployment(ctx, command); return err },
				"deploy":   func() error { _, err := service.DeployRelease(ctx, command); return err },
			} {
				if err := run(); !errors.Is(err, tt.wantErr) {
					t.Fatalf("%s error = %v, want %v", action, err, tt.wantErr)
				}
			}
			if recovery.calls != 0 || catalog.calls != 0 {
				t.Fatalf("refused request reached durable machinery: recovery=%d catalog=%d",
					recovery.calls, catalog.calls)
			}
		})
	}
}

func TestReadQueriesRequireTheReadRole(t *testing.T) {
	service, _, catalog := newTestService(t)
	ctx := requestsecurity.WithSecurityContext(context.Background(),
		securityContext(testTenant, delivery.OperatorRole, DeploymentOperatorRole))

	if _, err := service.ListDeployments(ctx); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListDeployments error = %v, want %v", err, ErrForbidden)
	}
	if _, err := service.ListDeploymentEvents(ctx, "definition-a", "revision-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListDeploymentEvents error = %v, want %v", err, ErrForbidden)
	}
	if catalog.calls != 0 {
		t.Fatalf("unauthorized read reached the catalog: %d calls", catalog.calls)
	}
}

func TestControlActionsRequireReasonAndIdempotencyKey(t *testing.T) {
	service, recovery, _ := newTestService(t)
	ctx := requestsecurity.WithSecurityContext(context.Background(),
		securityContext(testTenant, ReadRole, delivery.OperatorRole))

	invalid := []ControlRequest{
		{AttemptID: "attempt-a", Reason: "  ", IdempotencyKey: "replay-1"},
		{AttemptID: "attempt-a", Reason: "", IdempotencyKey: "replay-1"},
		{AttemptID: "attempt-a", Reason: "repaired", IdempotencyKey: ""},
		{AttemptID: "", Reason: "repaired", IdempotencyKey: "replay-1"},
		{AttemptID: "attempt-a", Reason: string(make([]byte, 1100)), IdempotencyKey: "replay-1"},
	}
	for index, request := range invalid {
		if _, err := service.ReplayDelivery(ctx, request); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("case %d error = %v, want %v", index, err, ErrInvalidRequest)
		}
	}
	if recovery.calls != 0 {
		t.Fatalf("invalid request reached durable machinery: %d calls", recovery.calls)
	}
}

func TestDeploymentCommandsRequireExpectedVersionAndReason(t *testing.T) {
	service, _, catalog := newTestService(t)
	ctx := requestsecurity.WithSecurityContext(context.Background(),
		securityContext(testTenant, ReadRole, DeploymentOperatorRole))

	invalid := []DeploymentCommand{
		{DefinitionID: "definition-a", RevisionID: "revision-a", ExpectedVersion: 0, Reason: "outage"},
		{DefinitionID: "definition-a", RevisionID: "revision-a", ExpectedVersion: -1, Reason: "outage"},
		{DefinitionID: "definition-a", RevisionID: "revision-a", ExpectedVersion: 2, Reason: "   "},
		{DefinitionID: "", RevisionID: "revision-a", ExpectedVersion: 2, Reason: "outage"},
	}
	for index, command := range invalid {
		if _, err := service.PauseDeployment(ctx, command); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("case %d error = %v, want %v", index, err, ErrInvalidRequest)
		}
	}
	if catalog.calls != 0 {
		t.Fatalf("invalid command reached the lifecycle catalog: %d calls", catalog.calls)
	}
}

func TestDeploymentCommandCarriesVerifiedActorAndSurfacesConflict(t *testing.T) {
	service, _, catalog := newTestService(t)
	catalog.err = lifecycle.ErrVersionConflict
	ctx := requestsecurity.WithSecurityContext(context.Background(),
		securityContext(testTenant, ReadRole, DeploymentOperatorRole))

	_, err := service.PauseDeployment(ctx, DeploymentCommand{
		DefinitionID:    "definition-a",
		RevisionID:      "revision-a",
		ExpectedVersion: 2,
		Reason:          "destination outage",
	})
	if !errors.Is(err, ErrVersionConflict) {
		t.Fatalf("stale expected version error = %v, want %v", err, ErrVersionConflict)
	}
	if catalog.calls != 1 {
		t.Fatalf("catalog calls = %d, want 1", catalog.calls)
	}
}

func TestSummarizeSnapshotProjectsActorAndReason(t *testing.T) {
	summary := summarizeSnapshot(lifecycle.Snapshot{
		TenantID: testTenant,
		State:    integration.DeploymentStatePaused,
		Version:  4,
		Health:   integration.DeploymentHealthDegraded,
		Updated: integration.AuditEnvelope{
			Principal:  operatorPrincipal(),
			Reason:     "destination outage",
			OccurredAt: time.Unix(1700000000, 0),
		},
	})
	if summary.State != "paused" || summary.Version != 4 || summary.Health != "degraded" {
		t.Fatalf("snapshot summary = %#v", summary)
	}
	if summary.UpdatedBy.ID != "operator-a" || summary.UpdatedReason != "destination outage" {
		t.Fatalf("snapshot summary actor = %#v", summary.UpdatedBy)
	}
	if summary.UpdatedAt.Location() != time.UTC {
		t.Fatalf("snapshot summary time is not UTC: %v", summary.UpdatedAt)
	}
}

func pointer[T any](value T) *T {
	return &value
}
