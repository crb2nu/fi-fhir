package operator

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
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

// recordingLedger stands in for the destination provenance ledger. It records
// every read so a test can prove which tenant, attempt, and bound reached it.
type recordingLedger struct {
	reads   []ledgerRead
	records map[string][]destination.DeliverySummary
	err     error
}

type ledgerRead struct {
	tenantID  string
	attemptID string
	limit     int
}

func (l *recordingLedger) ListDeliveriesForAttempt(
	_ context.Context,
	tenantID, attemptID string,
	limit int,
) ([]destination.DeliverySummary, error) {
	l.reads = append(l.reads, ledgerRead{tenantID: tenantID, attemptID: attemptID, limit: limit})
	if l.err != nil {
		return nil, l.err
	}
	return l.records[attemptID], nil
}

func newTestService(t *testing.T) (*Service, *countingRecovery, *countingCatalog) {
	t.Helper()
	service, recovery, catalog, _ := newTestServiceWithLedger(t)
	return service, recovery, catalog
}

func newTestServiceWithLedger(t *testing.T) (*Service, *countingRecovery, *countingCatalog, *recordingLedger) {
	t.Helper()
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
	ledger := &recordingLedger{}
	service, err := NewService(reads, ledger, recovery, catalog, testTenant)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service, recovery, catalog, ledger
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

func TestNewServiceRequiresTheDeliveryLedger(t *testing.T) {
	db, err := sql.Open("postgres", "postgres://unused:unused@127.0.0.1:1/unused?sslmode=disable")
	if err != nil {
		t.Fatalf("open placeholder database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	reads, err := NewPostgresReadStore(db)
	if err != nil {
		t.Fatalf("NewPostgresReadStore: %v", err)
	}
	if _, err := NewService(reads, nil, &countingRecovery{}, &countingCatalog{}, testTenant); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("NewService without a ledger reader error = %v, want %v", err, ErrUnavailable)
	}
}

func TestAttemptReadsRequireTheReadRoleBeforeTheLedger(t *testing.T) {
	service, _, _, ledger := newTestServiceWithLedger(t)
	ctx := requestsecurity.WithSecurityContext(context.Background(),
		securityContext(testTenant, delivery.OperatorRole, DeploymentOperatorRole))

	if _, err := service.GetAttempt(ctx, "attempt-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("GetAttempt error = %v, want %v", err, ErrForbidden)
	}
	if _, err := service.ListAttempts(ctx, AttemptFilter{}, PageRequest{}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("ListAttempts error = %v, want %v", err, ErrForbidden)
	}
	if _, err := service.GetMessageTrace(ctx, "receipt-a"); !errors.Is(err, ErrForbidden) {
		t.Fatalf("GetMessageTrace error = %v, want %v", err, ErrForbidden)
	}
	if len(ledger.reads) != 0 {
		t.Fatalf("unauthorized read reached the delivery ledger: %+v", ledger.reads)
	}
}

func TestAttachDeliveriesProjectsTheLedgerPerAttemptTenantScopedAndBounded(t *testing.T) {
	service, _, _, ledger := newTestServiceWithLedger(t)
	completed := time.Date(2026, 9, 24, 9, 3, 0, 0, time.FixedZone("EDT", -4*60*60))
	ledger.records = map[string][]destination.DeliverySummary{
		"attempt-a": {
			{
				Transport: destination.TransportFHIR, DestinationArtifactID: "destination-fhir",
				DestinationRevisionID: "destination-1", DestinationClass: "production",
				DestinationDigestVerified: "sha256:abc", Outcome: "delivered", HTTPStatusClass: "2xx",
				EndpointAdvisory: "https://fhir.example.test/r4", CompletedAt: completed,
				FHIRResourceTypes: []string{"Patient", "Encounter"}, FHIREntryCount: 2,
				FHIROutcomeCodesAdvisory: []string{},
			},
			{
				Transport: destination.TransportHTTPS, DestinationArtifactID: "destination-https",
				DestinationRevisionID: "destination-1", DestinationClass: "production",
				DestinationDigestVerified: "sha256:abc", Outcome: "refused",
				FailureCode: destination.FailureRejected, HTTPStatusClass: "4xx",
				EndpointAdvisory:                 "https://https.example.test/ingest",
				ServedCertificateSubjectAdvisory: "CN=https.example.test", CompletedAt: completed,
				FHIRResourceTypes: []string{}, FHIROutcomeCodesAdvisory: []string{},
			},
		},
	}
	attempts := []DeliveryAttemptSummary{{AttemptID: "attempt-a"}, {AttemptID: "attempt-b"}}

	if err := service.attachDeliveries(context.Background(), testTenant, attempts, MaxListedAttemptDeliveries); err != nil {
		t.Fatalf("attachDeliveries: %v", err)
	}
	wantReads := []ledgerRead{
		{tenantID: testTenant, attemptID: "attempt-a", limit: MaxListedAttemptDeliveries},
		{tenantID: testTenant, attemptID: "attempt-b", limit: MaxListedAttemptDeliveries},
	}
	if len(ledger.reads) != len(wantReads) {
		t.Fatalf("ledger reads = %+v, want %+v", ledger.reads, wantReads)
	}
	for index := range wantReads {
		if ledger.reads[index] != wantReads[index] {
			t.Fatalf("ledger read %d = %+v, want %+v", index, ledger.reads[index], wantReads[index])
		}
	}

	got := attempts[0].Deliveries
	if len(got) != 2 {
		t.Fatalf("attempt-a deliveries = %+v, want 2 in ledger order", got)
	}
	fhir := got[0]
	if fhir.Transport != "fhir" || fhir.Outcome != "delivered" || fhir.FHIREntryCount != 2 ||
		strings.Join(fhir.FHIRResourceTypes, ",") != "Patient,Encounter" ||
		fhir.FHIROutcomeCodesAdvisory == nil || len(fhir.FHIROutcomeCodesAdvisory) != 0 ||
		fhir.DigestVerified != "sha256:abc" || fhir.DestinationArtifactID != "destination-fhir" {
		t.Fatalf("fhir delivery = %+v", fhir)
	}
	if fhir.CompletedAt.Location() != time.UTC || !fhir.CompletedAt.Equal(completed) {
		t.Fatalf("fhir delivery completedAt = %v, want %v in UTC", fhir.CompletedAt, completed)
	}
	https := got[1]
	if https.Transport != "https" || https.HTTPStatusClass != "4xx" ||
		https.FailureCode != destination.FailureRejected ||
		https.ServedCertificateSubjectAdvisory != "CN=https.example.test" ||
		https.FHIRResourceTypes == nil || len(https.FHIRResourceTypes) != 0 || https.FHIREntryCount != 0 {
		t.Fatalf("https delivery = %+v", https)
	}
	if attempts[1].Deliveries == nil || len(attempts[1].Deliveries) != 0 {
		t.Fatalf("attempt-b deliveries = %#v, want an empty, non-nil list", attempts[1].Deliveries)
	}
}

func TestAttachDeliveriesFailsTheReadRatherThanClaimingNoDelivery(t *testing.T) {
	service, _, _, ledger := newTestServiceWithLedger(t)

	ledger.err = destination.ErrProvenanceUnavailable
	attempts := []DeliveryAttemptSummary{{AttemptID: "attempt-a"}}
	if err := service.attachDeliveries(context.Background(), testTenant, attempts, MaxAttemptDeliveries); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unavailable ledger error = %v, want %v", err, ErrUnavailable)
	}

	ledger.err = errors.New("connection reset")
	err := service.attachDeliveries(context.Background(), testTenant, attempts, MaxAttemptDeliveries)
	if err == nil || errors.Is(err, ErrUnavailable) {
		t.Fatalf("failed ledger read error = %v, want a wrapped read failure", err)
	}
	if attempts[0].Deliveries != nil {
		t.Fatalf("a failed ledger read still attached %#v", attempts[0].Deliveries)
	}
}

func pointer[T any](value T) *T {
	return &value
}
