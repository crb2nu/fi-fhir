package destination

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"
)

const destinationMigrationLockKey = int64(5064657639792058897)

// SchemaVersion is the destination ledger version this binary expects. Slice
// 4.4a defines N-1 as the per-package ledger version; see
// `.loom/40-decisions.md` (2026-08-09, "What one version means"). Slice 4.1c-c
// claimed 0003 (the `fhir` transport's provenance columns).
const SchemaVersion = 3

//go:embed migrations/0001_delivery_identity.sql
var deliveryIdentityMigration string

//go:embed migrations/0002_https_delivery_provenance.sql
var httpsDeliveryProvenanceMigration string

//go:embed migrations/0003_fhir_delivery_provenance.sql
var fhirDeliveryProvenanceMigration string

// destinationMigration is one numbered step in this package's own forward-only
// ledger, integration_destination_schema_migrations.
type destinationMigration struct {
	version    int64
	name       string
	statements string
}

// destinationMigrations is the fixed, ordered migration set. The ledger is the
// authority on the next free number at every rebase, not a planning document.
func destinationMigrations() []destinationMigration {
	return []destinationMigration{
		{version: 1, name: "0001_delivery_identity", statements: deliveryIdentityMigration},
		{version: 2, name: "0002_https_delivery_provenance", statements: httpsDeliveryProvenanceMigration},
		{version: 3, name: "0003_fhir_delivery_provenance", statements: fhirDeliveryProvenanceMigration},
	}
}

// ErrProvenanceUnavailable means the decision recorder is not configured.
var ErrProvenanceUnavailable = errors.New("destination identity provenance store unavailable")

// PostgresProvenance records deliver decisions in the same database as the
// durable delivery state machine.
//
// It owns its own numbered migration set and its own version ledger, following
// the per-package go:embed idiom already used by processor, lifecycle, batch,
// and session. It therefore claims no migration number in any other package's
// sequence.
type PostgresProvenance struct {
	db *sql.DB
}

// NewPostgresProvenance constructs the decision recorder.
func NewPostgresProvenance(db *sql.DB) (*PostgresProvenance, error) {
	if db == nil {
		return nil, ErrProvenanceUnavailable
	}
	return &PostgresProvenance{db: db}, nil
}

// Migrate applies the fixed, numbered destination schema exactly once. An
// advisory transaction lock serializes startup across replicas.
func (p *PostgresProvenance) Migrate(ctx context.Context) error {
	if p == nil || p.db == nil || ctx == nil {
		return ErrProvenanceUnavailable
	}
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin destination migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, destinationMigrationLockKey); err != nil {
		return fmt.Errorf("lock destination migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS integration_destination_schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
		)
	`); err != nil {
		return fmt.Errorf("create destination migration ledger: %w", err)
	}
	for _, migration := range destinationMigrations() {
		var applied bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM integration_destination_schema_migrations WHERE version = $1)`,
			migration.version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("read destination migration ledger: %w", err)
		}
		if applied {
			continue
		}
		if _, err := tx.ExecContext(ctx, migration.statements); err != nil {
			return fmt.Errorf("apply destination migration %s: %w", migration.name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO integration_destination_schema_migrations (version, name) VALUES ($1, $2)`,
			migration.version, migration.name,
		); err != nil {
			return fmt.Errorf("record destination migration %s: %w", migration.name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit destination migration: %w", err)
	}
	return nil
}

// RecordDecision durably appends one decision. It writes identity and
// destination provenance only; it never receives secret material, raw bytes, or
// canonical event content.
func (p *PostgresProvenance) RecordDecision(ctx context.Context, decision Decision) error {
	if p == nil || p.db == nil || ctx == nil {
		return ErrProvenanceUnavailable
	}
	if !validIdentity(decision.TenantID) || !validIdentity(decision.AttemptID) ||
		(decision.Mode != ModeStrict && decision.Mode != ModeCompatibility) ||
		decision.DecidedAt.IsZero() {
		return ErrProvenanceUnavailable
	}
	label := "denied"
	if decision.Authorized {
		label = "authorized"
	}
	if _, err := p.db.ExecContext(ctx, `
		INSERT INTO integration_delivery_identity_decisions (
			tenant_id, attempt_id, decision, identity_mode,
			principal_subject, principal_auth_method, granted_role,
			destination_artifact_id, destination_revision_id, destination_class,
			destination_digest_verified, denial_code,
			destination_endpoint_advisory, decided_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
	`,
		decision.TenantID,
		decision.AttemptID,
		label,
		string(decision.Mode),
		decision.Subject,
		decision.AuthMethod,
		decision.GrantedRole,
		decision.DestinationArtifactID,
		decision.DestinationRevisionID,
		decision.DestinationClass,
		decision.DestinationDigestVerified,
		decision.DenialCode,
		decision.EndpointAdvisory,
		decision.DecidedAt.UTC(),
	); err != nil {
		return fmt.Errorf("record delivery identity decision: %w", err)
	}
	return nil
}

// RecordDelivery durably appends one executed destination delivery.
//
// It writes destination provenance and one closed-vocabulary status class only.
// It never receives secret material, raw bytes, canonical event content, a
// response body, or a response header. For a `fhir` delivery it additionally
// writes the projected resource types, the bundle entry count, and the
// sanitised OperationOutcome issue codes — never diagnostics text.
func (p *PostgresProvenance) RecordDelivery(ctx context.Context, record DeliveryRecord) error {
	if p == nil || p.db == nil || ctx == nil {
		return ErrProvenanceUnavailable
	}
	if !validIdentity(record.TenantID) || !validIdentity(record.AttemptID) ||
		(record.Transport != TransportHTTPS && record.Transport != TransportFHIR) ||
		record.CompletedAt.IsZero() ||
		!validIdentity(record.DestinationDigestVerified) {
		return ErrProvenanceUnavailable
	}
	switch record.Outcome {
	case outcomeDelivered, outcomeRetryable, outcomeRefused:
	default:
		return ErrProvenanceUnavailable
	}
	if len(record.FHIRResourceTypes) > maxFHIRLedgerBytes ||
		len(record.FHIROutcomeCodesAdvisory) > maxFHIRLedgerBytes ||
		record.FHIREntryCount < 0 {
		return ErrProvenanceUnavailable
	}
	if _, err := p.db.ExecContext(ctx, `
		INSERT INTO integration_destination_deliveries (
			tenant_id, attempt_id, transport,
			destination_artifact_id, destination_revision_id, destination_class,
			destination_digest_verified, outcome, failure_code, http_status_class,
			destination_endpoint_advisory, served_certificate_subject_advisory,
			completed_at,
			fhir_resource_types, fhir_entry_count, fhir_outcome_codes_advisory
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`,
		record.TenantID,
		record.AttemptID,
		string(record.Transport),
		record.DestinationArtifactID,
		record.DestinationRevisionID,
		record.DestinationClass,
		record.DestinationDigestVerified,
		record.Outcome,
		record.FailureCode,
		record.HTTPStatusClass,
		record.EndpointAdvisory,
		record.ServedCertificateSubjectAdvisory,
		record.CompletedAt.UTC(),
		record.FHIRResourceTypes,
		record.FHIREntryCount,
		record.FHIROutcomeCodesAdvisory,
	); err != nil {
		return fmt.Errorf("record destination delivery: %w", err)
	}
	return nil
}

// MaxDeliveryReadLimit is the hard ceiling on the rows one
// ListDeliveriesForAttempt call may return.
const MaxDeliveryReadLimit = 100

// DeliverySummary is one delivery-ledger row as the operator control plane
// reads it (Slice 4.2c).
//
// It is exactly the ledger's columns and nothing more. The ledger is
// clinical-content-free by construction (migrations 0002 and 0003): every
// field here is either server-owned provenance or one of the three
// `_advisory` values — a destination address the revision declares, a bounded
// printable certificate subject, and closed-vocabulary OperationOutcome issue
// codes. No response body, header, diagnostics text, or event content was ever
// written, so none can be read.
type DeliverySummary struct {
	Transport                        TransportKind
	DestinationArtifactID            string
	DestinationRevisionID            string
	DestinationClass                 string
	DestinationDigestVerified        string
	Outcome                          string
	FailureCode                      string
	HTTPStatusClass                  string
	EndpointAdvisory                 string
	ServedCertificateSubjectAdvisory string
	CompletedAt                      time.Time
	// FHIRResourceTypes is fhir_resource_types split on commas, in bundle
	// order. Empty — never nil — for an https delivery.
	FHIRResourceTypes []string
	FHIREntryCount    int
	// FHIROutcomeCodesAdvisory is fhir_outcome_codes_advisory split on commas.
	// Empty — never nil — when the destination answered without an
	// OperationOutcome.
	FHIROutcomeCodesAdvisory []string
}

// ListDeliveriesForAttempt returns at most limit delivery-ledger rows for one
// attempt of one tenant, newest first.
//
// The tenant is a WHERE predicate, not a post-filter, so another tenant's row
// under the same attempt id is never read. Order is (completed_at, delivery_id)
// descending — the identity column breaks a completion-time tie in insertion
// order — which the (tenant_id, attempt_id, completed_at, delivery_id) index
// serves as a backward scan.
func (p *PostgresProvenance) ListDeliveriesForAttempt(
	ctx context.Context,
	tenantID, attemptID string,
	limit int,
) ([]DeliverySummary, error) {
	if p == nil || p.db == nil || ctx == nil {
		return nil, ErrProvenanceUnavailable
	}
	if !validIdentity(tenantID) || limit < 1 || limit > MaxDeliveryReadLimit {
		return nil, ErrProvenanceUnavailable
	}
	if !validIdentity(attemptID) {
		// RecordDelivery refuses this attempt id, so no row can carry it.
		return []DeliverySummary{}, nil
	}
	rows, err := p.db.QueryContext(ctx, `
		SELECT transport, destination_artifact_id, destination_revision_id,
			destination_class, destination_digest_verified, outcome, failure_code,
			http_status_class, destination_endpoint_advisory,
			served_certificate_subject_advisory, completed_at,
			fhir_resource_types, fhir_entry_count, fhir_outcome_codes_advisory
		FROM integration_destination_deliveries
		WHERE tenant_id = $1 AND attempt_id = $2
		ORDER BY completed_at DESC, delivery_id DESC
		LIMIT $3
	`, tenantID, attemptID, limit)
	if err != nil {
		return nil, fmt.Errorf("list destination deliveries: %w", err)
	}
	defer func() { _ = rows.Close() }()
	deliveries := make([]DeliverySummary, 0)
	for rows.Next() {
		var summary DeliverySummary
		var transport, resourceTypes, outcomeCodes string
		if err := rows.Scan(
			&transport, &summary.DestinationArtifactID, &summary.DestinationRevisionID,
			&summary.DestinationClass, &summary.DestinationDigestVerified, &summary.Outcome,
			&summary.FailureCode, &summary.HTTPStatusClass, &summary.EndpointAdvisory,
			&summary.ServedCertificateSubjectAdvisory, &summary.CompletedAt,
			&resourceTypes, &summary.FHIREntryCount, &outcomeCodes,
		); err != nil {
			return nil, fmt.Errorf("scan destination delivery: %w", err)
		}
		summary.Transport = TransportKind(transport)
		summary.CompletedAt = summary.CompletedAt.UTC()
		summary.FHIRResourceTypes = splitLedgerList(resourceTypes)
		summary.FHIROutcomeCodesAdvisory = splitLedgerList(outcomeCodes)
		deliveries = append(deliveries, summary)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate destination deliveries: %w", err)
	}
	return deliveries, nil
}

// splitLedgerList splits one of the ledger's comma-joined FHIR columns. The
// writer joins without spaces and never emits an empty element, so an empty
// column is the only way to get an empty list.
func splitLedgerList(value string) []string {
	if value == "" {
		return []string{}
	}
	return strings.Split(value, ",")
}
