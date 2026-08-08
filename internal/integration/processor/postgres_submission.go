package processor

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	_ "embed"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"sort"
	"strings"
	"time"
	"unicode"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	submissionMigrationLockKey = int64(5064657639792058881)
	derivedIdempotencyDomain   = "fi-fhir/idempotency/v1\x00"
	requestFingerprintDomain   = "fi-fhir/submission-fingerprint/v1\x00"
	receiptIDDomain            = "fi-fhir/receipt-id/v1\x00"
	traceIDDomain              = "fi-fhir/trace-id/v1\x00"
	lineageIDDomain            = "fi-fhir/lineage-id/v1\x00"
	attemptIDDomain            = "fi-fhir/attempt-id/v1\x00"
	outboxIDDomain             = "fi-fhir/outbox-id/v1\x00"
	deliveryOutboxTopic        = "integration.delivery.v1"
)

var (
	// ErrPostgresSubmissionUnavailable means the durable store is not configured.
	ErrPostgresSubmissionUnavailable = errors.New("postgres submission store unavailable")
	// ErrUnsupportedRawRetention keeps production fail-closed until encrypted raw storage exists.
	ErrUnsupportedRawRetention = errors.New("production raw retention policy is unsupported")
	// ErrCommitOutcomeUnknown means COMMIT may have succeeded and the caller must retry by idempotency key.
	ErrCommitOutcomeUnknown = errors.New("submission commit outcome unknown")
)

//go:embed migrations/0001_atomic_submission.sql
var atomicSubmissionMigration string

//go:embed migrations/0002_delivery_reliability.sql
var deliveryReliabilityMigration string

//go:embed migrations/0003_operator_control_plane.sql
var operatorControlPlaneMigration string

var submissionMigrations = []struct {
	version int64
	name    string
	sql     string
}{
	{version: 1, name: "0001_atomic_submission", sql: atomicSubmissionMigration},
	{version: 2, name: "0002_delivery_reliability", sql: deliveryReliabilityMigration},
	{version: 3, name: "0003_operator_control_plane", sql: operatorControlPlaneMigration},
}

// AdmissionAuthorizer runs inside the durable submission transaction before
// any receipt rows are written. Implementations may take database locks that
// must remain held through commit.
type AdmissionAuthorizer func(
	context.Context,
	*sql.Tx,
	integration.ProcessRequest,
	integration.IntegrationDefinitionRevision,
) error

// PostgresSubmissionConfig configures deterministic timestamps and an optional
// transaction-scoped deployment authorization gate.
type PostgresSubmissionConfig struct {
	Clock     func() time.Time
	Authorize AdmissionAuthorizer
}

// PostgresSubmissionStore owns the fixed PostgreSQL schema and transaction for
// production admission. It never receives or persists raw envelope bytes.
type PostgresSubmissionStore struct {
	db        *sql.DB
	clock     func() time.Time
	authorize AdmissionAuthorizer
	faultHook submissionFaultHook
}

type submissionCheckpoint string

const (
	checkpointAfterReceipt submissionCheckpoint = "after_receipt"
	checkpointAfterEvent   submissionCheckpoint = "after_event"
	checkpointAfterLineage submissionCheckpoint = "after_lineage"
	checkpointAfterAttempt submissionCheckpoint = "after_attempt"
	checkpointAfterOutbox  submissionCheckpoint = "after_outbox"
	checkpointBeforeCommit submissionCheckpoint = "before_commit"
	checkpointAfterCommit  submissionCheckpoint = "after_commit"
)

type submissionFaultHook func(submissionCheckpoint) error

// NewPostgresSubmissionStore constructs a PostgreSQL-only durable admission store.
func NewPostgresSubmissionStore(db *sql.DB, config PostgresSubmissionConfig) (*PostgresSubmissionStore, error) {
	if db == nil {
		return nil, ErrPostgresSubmissionUnavailable
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	return &PostgresSubmissionStore{db: db, clock: clock, authorize: config.Authorize}, nil
}

// Migrate applies the fixed, numbered submission schema exactly once. An
// advisory transaction lock serializes startup across replicas.
func (s *PostgresSubmissionStore) Migrate(ctx context.Context) error {
	if s == nil || s.db == nil || ctx == nil {
		return ErrPostgresSubmissionUnavailable
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin submission migration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, submissionMigrationLockKey); err != nil {
		return fmt.Errorf("lock submission migration: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS integration_submission_schema_migrations (
			version BIGINT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp()
		)
	`); err != nil {
		return fmt.Errorf("create submission migration ledger: %w", err)
	}
	for _, migration := range submissionMigrations {
		var applied bool
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM integration_submission_schema_migrations WHERE version = $1)`,
			migration.version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("read submission migration ledger: %w", err)
		}
		if applied {
			continue
		}
		if _, err := tx.ExecContext(ctx, migration.sql); err != nil {
			return fmt.Errorf("apply submission migration %s: %w", migration.name, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO integration_submission_schema_migrations (version, name) VALUES ($1, $2)`,
			migration.version,
			migration.name,
		); err != nil {
			return fmt.Errorf("record submission migration %s: %w", migration.name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit submission migration: %w", err)
	}
	return nil
}

func (s *PostgresSubmissionStore) commit(
	ctx context.Context,
	request integration.ProcessRequest,
	revision integration.IntegrationDefinitionRevision,
	plan integration.ProcessResult,
) (integration.ProcessResult, error) {
	if s == nil || s.db == nil || s.clock == nil || ctx == nil {
		return integration.ProcessResult{}, ErrPostgresSubmissionUnavailable
	}
	if revision.Policy.RawRetention.EffectiveMode() != integration.RawRetentionModeEphemeral {
		return integration.ProcessResult{}, ErrUnsupportedRawRetention
	}
	if err := request.ValidateAgainst(revision); err != nil || request.Mode != integration.ExecutionModeProduction {
		return integration.ProcessResult{}, ErrInvalidProcessRequest
	}
	if plan.Mode != integration.ExecutionModeProduction || plan.Receipt != nil || len(plan.Events) == 0 {
		return integration.ProcessResult{}, ErrInvalidProcessResult
	}

	effectiveKey, err := effectiveIdempotencyKey(request, revision, plan.Events[0].SourceMessageID)
	if err != nil {
		return integration.ProcessResult{}, err
	}
	fingerprint, err := submissionRequestFingerprint(request, revision)
	if err != nil {
		return integration.ProcessResult{}, err
	}
	result, err := finalizeProductionResult(request, revision, plan, effectiveKey, s.clock().UTC())
	if err != nil {
		return integration.ProcessResult{}, err
	}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		return integration.ProcessResult{}, fmt.Errorf("marshal production result: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return integration.ProcessResult{}, fmt.Errorf("begin production submission: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if s.authorize != nil {
		if err := s.authorize(ctx, tx, request, revision); err != nil {
			return integration.ProcessResult{}, fmt.Errorf("authorize production submission: %w", err)
		}
	}

	principalJSON, err := json.Marshal(result.Security.Principal)
	if err != nil {
		return integration.ProcessResult{}, fmt.Errorf("marshal receipt principal: %w", err)
	}
	integrationRevisionJSON, err := json.Marshal(result.IntegrationRevision)
	if err != nil {
		return integration.ProcessResult{}, fmt.Errorf("marshal receipt revision: %w", err)
	}
	var insertedReceiptID string
	err = tx.QueryRowContext(ctx, `
		INSERT INTO integration_receipts (
			tenant_id, receipt_id, idempotency_key, request_fingerprint,
			integration_revision, status, recorded_at, correlation_id,
			raw_retention_mode, principal_json, reason, result_json
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		ON CONFLICT DO NOTHING
		RETURNING receipt_id
	`,
		result.TenantID,
		result.Receipt.ID,
		result.Receipt.IdempotencyKey,
		fingerprint,
		string(integrationRevisionJSON),
		result.Receipt.Status,
		result.Receipt.RecordedAt,
		result.Receipt.CorrelationID,
		result.Receipt.RawRetentionMode,
		string(principalJSON),
		result.Receipt.Reason,
		string(resultJSON),
	).Scan(&insertedReceiptID)
	if errors.Is(err, sql.ErrNoRows) {
		_ = tx.Rollback()
		return s.loadDuplicate(ctx, revision, result.TenantID, effectiveKey, fingerprint)
	}
	if err != nil {
		return integration.ProcessResult{}, fmt.Errorf("insert durable receipt: %w", err)
	}
	if insertedReceiptID != result.Receipt.ID {
		return integration.ProcessResult{}, ErrInvalidProcessResult
	}
	if err := s.checkpoint(checkpointAfterReceipt); err != nil {
		return integration.ProcessResult{}, err
	}

	for _, event := range result.Events {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO integration_canonical_events (
				tenant_id, event_id, receipt_id, event_type, source_message_id,
				correlation_id, classification, payload_json, recorded_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`,
			event.TenantID,
			event.ID,
			result.Receipt.ID,
			event.Type,
			event.SourceMessageID,
			event.CorrelationID,
			event.Classification,
			string(event.PayloadJSON()),
			result.Receipt.RecordedAt,
		); err != nil {
			return integration.ProcessResult{}, fmt.Errorf("insert canonical event: %w", err)
		}
		if err := s.checkpoint(checkpointAfterEvent); err != nil {
			return integration.ProcessResult{}, err
		}

		routesJSON, err := json.Marshal(routesForEvent(result.Routes, event.ID))
		if err != nil {
			return integration.ProcessResult{}, fmt.Errorf("marshal event routes: %w", err)
		}
		diagnosticsJSON, err := json.Marshal(result.Diagnostics)
		if err != nil {
			return integration.ProcessResult{}, fmt.Errorf("marshal event diagnostics: %w", err)
		}
		artifactRevisionsJSON, err := json.Marshal(result.ArtifactRevisions)
		if err != nil {
			return integration.ProcessResult{}, fmt.Errorf("marshal artifact lineage: %w", err)
		}
		lineageID := deterministicSubmissionID(
			lineageIDDomain,
			result.TenantID,
			result.Receipt.ID,
			event.ID,
		)
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO integration_message_lineage (
				tenant_id, lineage_id, receipt_id, event_id, trace_id,
				correlation_id, source_message_id, artifact_revisions_json,
				routes_json, diagnostics_json, recorded_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		`,
			result.TenantID,
			lineageID,
			result.Receipt.ID,
			event.ID,
			result.Correlations.TraceID,
			result.Correlations.CorrelationID,
			result.Correlations.SourceMessageID,
			string(artifactRevisionsJSON),
			string(routesJSON),
			string(diagnosticsJSON),
			result.Receipt.RecordedAt,
		); err != nil {
			return integration.ProcessResult{}, fmt.Errorf("insert message lineage: %w", err)
		}
		if err := s.checkpoint(checkpointAfterLineage); err != nil {
			return integration.ProcessResult{}, err
		}
	}

	for _, delivery := range result.Deliveries {
		destinationJSON, err := json.Marshal(delivery.Destination)
		if err != nil {
			return integration.ProcessResult{}, fmt.Errorf("marshal delivery destination: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO integration_delivery_attempts (
				tenant_id, attempt_id, receipt_id, event_id, trace_id,
				destination_revision_json, route_name, action_id, status,
				attempt_count, recorded_at, scheduled_at
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $11)
		`,
			delivery.TenantID,
			delivery.AttemptID,
			result.Receipt.ID,
			delivery.EventID,
			result.Correlations.TraceID,
			string(destinationJSON),
			delivery.Route,
			delivery.Action,
			delivery.Status,
			delivery.AttemptCount,
			result.Receipt.RecordedAt,
		); err != nil {
			return integration.ProcessResult{}, fmt.Errorf("insert delivery attempt: %w", err)
		}
		if err := s.checkpoint(checkpointAfterAttempt); err != nil {
			return integration.ProcessResult{}, err
		}

		outboxID := deterministicSubmissionID(outboxIDDomain, result.TenantID, delivery.AttemptID)
		outboxPayload, err := json.Marshal(map[string]any{
			"schema":        deliveryOutboxTopic,
			"tenant_id":     result.TenantID,
			"receipt_id":    result.Receipt.ID,
			"event_id":      delivery.EventID,
			"trace_id":      result.Correlations.TraceID,
			"attempt_id":    delivery.AttemptID,
			"destination":   delivery.Destination,
			"route":         delivery.Route,
			"action":        delivery.Action,
			"attempt_count": delivery.AttemptCount,
		})
		if err != nil {
			return integration.ProcessResult{}, fmt.Errorf("marshal delivery outbox: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO integration_delivery_outbox (
				tenant_id, outbox_id, attempt_id, topic, status,
				payload_json, created_at, scheduled_at, updated_at
			) VALUES ($1, $2, $3, $4, 'pending', $5, $6, $6, $6)
		`,
			result.TenantID,
			outboxID,
			delivery.AttemptID,
			deliveryOutboxTopic,
			string(outboxPayload),
			result.Receipt.RecordedAt,
		); err != nil {
			return integration.ProcessResult{}, fmt.Errorf("insert delivery outbox: %w", err)
		}
		if err := s.checkpoint(checkpointAfterOutbox); err != nil {
			return integration.ProcessResult{}, err
		}
	}

	if err := s.checkpoint(checkpointBeforeCommit); err != nil {
		return integration.ProcessResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return integration.ProcessResult{}, fmt.Errorf("%w: %w", ErrCommitOutcomeUnknown, err)
	}
	if err := s.checkpoint(checkpointAfterCommit); err != nil {
		return integration.ProcessResult{}, err
	}
	return result, nil
}

func (s *PostgresSubmissionStore) loadDuplicate(
	ctx context.Context,
	revision integration.IntegrationDefinitionRevision,
	tenantID string,
	effectiveKey string,
	fingerprint string,
) (integration.ProcessResult, error) {
	var storedFingerprint string
	var resultJSON []byte
	if err := s.db.QueryRowContext(ctx, `
		SELECT request_fingerprint, result_json
		FROM integration_receipts
		WHERE tenant_id = $1 AND idempotency_key = $2
	`, tenantID, effectiveKey).Scan(&storedFingerprint, &resultJSON); err != nil {
		return integration.ProcessResult{}, fmt.Errorf("load durable duplicate: %w", err)
	}
	if subtle.ConstantTimeCompare([]byte(storedFingerprint), []byte(fingerprint)) != 1 {
		return integration.ProcessResult{}, ErrIdempotencyConflict
	}
	var result integration.ProcessResult
	if err := json.Unmarshal(resultJSON, &result); err != nil {
		return integration.ProcessResult{}, fmt.Errorf("decode durable duplicate: %w", err)
	}
	if err := result.ValidateProductionAgainst(revision); err != nil {
		return integration.ProcessResult{}, fmt.Errorf("validate durable duplicate: %w", err)
	}
	if result.Receipt == nil || result.Receipt.IdempotencyKey != effectiveKey {
		return integration.ProcessResult{}, ErrInvalidProcessResult
	}
	return result, nil
}

func (s *PostgresSubmissionStore) checkpoint(checkpoint submissionCheckpoint) error {
	if s == nil || s.faultHook == nil {
		return nil
	}
	return s.faultHook(checkpoint)
}

func finalizeProductionResult(
	request integration.ProcessRequest,
	revision integration.IntegrationDefinitionRevision,
	plan integration.ProcessResult,
	effectiveKey string,
	recordedAt time.Time,
) (integration.ProcessResult, error) {
	if recordedAt.IsZero() || recordedAt != recordedAt.UTC() || plan.ArtifactRevisions == nil {
		return integration.ProcessResult{}, ErrInvalidProcessResult
	}
	result := plan
	result.Security.Principal.Roles = append([]string(nil), plan.Security.Principal.Roles...)
	result.Events = append([]integration.ProcessedEvent(nil), plan.Events...)
	result.Diagnostics = append([]integration.Diagnostic{}, plan.Diagnostics...)
	result.Routes = cloneRoutes(plan.Routes)
	result.Deliveries = append([]integration.DeliveryResult(nil), plan.Deliveries...)
	result.Correlations.EventIDs = append([]string(nil), plan.Correlations.EventIDs...)

	receiptID := deterministicSubmissionID(receiptIDDomain, revision.TenantID, effectiveKey)
	traceID := deterministicSubmissionID(traceIDDomain, revision.TenantID, receiptID)
	principal := request.Security.Principal
	principal.Roles = append([]string(nil), request.Security.Principal.Roles...)
	result.Receipt = &integration.Receipt{
		ID:                  receiptID,
		TenantID:            revision.TenantID,
		IntegrationRevision: revision.Reference(),
		Status:              integration.ReceiptStatusAccepted,
		IdempotencyKey:      effectiveKey,
		RecordedAt:          recordedAt,
		CorrelationID:       request.CorrelationID,
		RawRetentionMode:    integration.RawRetentionModeEphemeral,
		Principal:           principal,
		Reason:              request.Security.Reason,
	}
	result.Correlations.ReceiptID = receiptID
	result.Correlations.TraceID = traceID
	result.Correlations.DeliveryAttemptIDs = make([]string, 0, len(result.Deliveries))
	for index := range result.Deliveries {
		delivery := &result.Deliveries[index]
		if delivery.Status != integration.DeliveryStatusPlanned || !request.Mode.AllowsDelivery(delivery.Destination.Class) {
			return integration.ProcessResult{}, ErrInvalidProcessResult
		}
		delivery.Status = integration.DeliveryStatusQueued
		delivery.AttemptID = deterministicSubmissionID(
			attemptIDDomain,
			revision.TenantID,
			receiptID,
			delivery.EventID,
			delivery.Route,
			delivery.Action,
			delivery.Destination.ArtifactID,
			delivery.Destination.RevisionID,
			delivery.Destination.Digest,
		)
		delivery.AttemptCount = 1
		result.Correlations.DeliveryAttemptIDs = append(result.Correlations.DeliveryAttemptIDs, delivery.AttemptID)
	}
	if err := result.ValidateFor(request, revision); err != nil {
		return integration.ProcessResult{}, ErrInvalidProcessResult
	}
	if err := result.ValidateProductionAgainst(revision); err != nil {
		return integration.ProcessResult{}, ErrInvalidProcessResult
	}
	return result, nil
}

func effectiveIdempotencyKey(
	request integration.ProcessRequest,
	revision integration.IntegrationDefinitionRevision,
	sourceMessageID string,
) (string, error) {
	if request.IdempotencyKey != "" {
		if !validIdempotencyKey(request.IdempotencyKey) {
			return "", ErrInvalidProcessRequest
		}
		return request.IdempotencyKey, nil
	}
	if !validIdempotencyComponent(revision.TenantID) ||
		!validIdempotencyComponent(revision.Source.SourceID) ||
		!validIdempotencyComponent(sourceMessageID) {
		return "", ErrInvalidProcessRequest
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(derivedIdempotencyDomain))
	writeSubmissionIdentity(hasher,
		revision.TenantID,
		revision.Source.SourceID,
		sourceMessageID,
		revision.Reference().ArtifactID,
		revision.Reference().RevisionID,
		revision.Reference().Digest,
	)
	return "derived:v1:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func submissionRequestFingerprint(
	request integration.ProcessRequest,
	revision integration.IntegrationDefinitionRevision,
) (string, error) {
	if err := request.ValidateAgainst(revision); err != nil {
		return "", ErrInvalidProcessRequest
	}
	roles := append([]string(nil), request.Security.Principal.Roles...)
	sort.Strings(roles)
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(requestFingerprintDomain))
	writeSubmissionIdentity(hasher,
		revision.TenantID,
		revision.Reference().ArtifactID,
		revision.Reference().RevisionID,
		revision.Reference().Digest,
		revision.Source.SourceID,
		string(revision.Format),
		request.Envelope.PayloadDigest,
		fmt.Sprintf("%d", request.Envelope.SizeBytes),
		request.Security.Principal.ID,
		string(request.Security.Principal.Kind),
		request.Security.Principal.AuthMethod,
		request.Security.Principal.SourceID,
		strings.Join(roles, "\x00"),
		request.Security.Reason,
	)
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func deterministicSubmissionID(domain string, values ...string) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(domain))
	writeSubmissionIdentity(hasher, values...)
	sum := hasher.Sum(nil)
	var identifier [16]byte
	copy(identifier[:], sum[:16])
	identifier[6] = (identifier[6] & 0x0f) | 0x80
	identifier[8] = (identifier[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(identifier[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32])
}

func writeSubmissionIdentity(hasher hash.Hash, values ...string) {
	for _, value := range values {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(value)))
		_, _ = hasher.Write(length[:])
		_, _ = hasher.Write([]byte(value))
	}
}

func validIdempotencyKey(value string) bool {
	if len(value) == 0 || len(value) > 512 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return false
		}
	}
	return true
}

func validIdempotencyComponent(value string) bool {
	return value != "" && len(value) <= 256 && strings.TrimSpace(value) == value && !strings.ContainsRune(value, '\x00')
}

func routesForEvent(routes []integration.RouteResult, eventID string) []integration.RouteResult {
	filtered := make([]integration.RouteResult, 0, len(routes))
	for _, route := range routes {
		if route.EventID != eventID {
			continue
		}
		clone := route
		clone.PlannedActions = append([]string(nil), route.PlannedActions...)
		clone.DiagnosticCodes = append([]string(nil), route.DiagnosticCodes...)
		filtered = append(filtered, clone)
	}
	return filtered
}

func cloneRoutes(routes []integration.RouteResult) []integration.RouteResult {
	cloned := make([]integration.RouteResult, len(routes))
	for index, route := range routes {
		cloned[index] = route
		cloned[index].PlannedActions = append([]string(nil), route.PlannedActions...)
		cloned[index].DiagnosticCodes = append([]string(nil), route.DiagnosticCodes...)
	}
	return cloned
}
