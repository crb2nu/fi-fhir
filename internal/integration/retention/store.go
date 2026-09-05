package retention

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrStoreUnavailable is returned when the store is used without the
// collaborators it needs.
var ErrStoreUnavailable = errors.New("retention store unavailable")

// policyWriteLockKey serialises retention policy writes across replicas.
//
// Without it, two replicas booting simultaneously against a fresh policy table
// both see no row, both compute version 1, and both insert into
// integration_retention_policy_audit — where UNIQUE (tenant_id, policy_version)
// fails one of them and takes a replica down at startup over a policy neither
// of them was changing. The migrators take an advisory lock at startup for the
// same reason (`postgres_submission.go:130`, `session/postgres.go:72`).
//
// This is the one place in the package that needs a lock. The purge itself does
// not: its claim is the guarded statement (see PurgeExpired).
const policyWriteLockKey = int64(6193446288274593311)

// defaultBatchSize bounds one purge pass. A purge holds row locks and writes an
// audit row per record, so an unbounded pass would be one long transaction
// against the busiest table in the system.
const defaultBatchSize = 200

// PostgresConfig configures the durable retention store.
type PostgresConfig struct {
	// TenantID is the one deployment security domain this process owns. The
	// destination registry enforces the same single-tenant rule.
	TenantID string

	// Clock supplies the server-owned instants written to purged_at and to the
	// audit. Never a client-supplied time.
	Clock func() time.Time
}

// PostgresStore reads the retention policy and performs the purge.
//
// It owns no migration of its own: the expiry columns and the exemption live in
// the processor's 0005 and the session workspace's 0006, applied by the stores
// that own those tables.
type PostgresStore struct {
	db       *sql.DB
	tenantID string
	clock    func() time.Time
}

// NewPostgresStore builds the durable retention store.
func NewPostgresStore(db *sql.DB, config PostgresConfig) (*PostgresStore, error) {
	if db == nil {
		return nil, fmt.Errorf("%w: no database", ErrStoreUnavailable)
	}
	if strings.TrimSpace(config.TenantID) == "" {
		return nil, fmt.Errorf("%w: deployment tenant is required", ErrStoreUnavailable)
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}
	return &PostgresStore{db: db, tenantID: config.TenantID, clock: clock}, nil
}

// TenantID reports the deployment security domain this store purges.
func (s *PostgresStore) TenantID() string {
	if s == nil {
		return ""
	}
	return s.tenantID
}

func (s *PostgresStore) now() time.Time {
	return s.clock().UTC()
}

// Policy reads the durable retention policy, or nil when none is recorded.
//
// A nil policy is the fail-closed default: an unconfigured deployment purges
// nothing, and says so rather than guessing a window.
func (s *PostgresStore) Policy(ctx context.Context) (*Policy, error) {
	if s == nil || s.db == nil || ctx == nil {
		return nil, ErrStoreUnavailable
	}
	var (
		policy        Policy
		canonical     sql.NullInt64
		sample        sql.NullInt64
		export        sql.NullInt64
		stream        sql.NullInt64
		principalJSON []byte
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT policy_version, canonical_event_retain_seconds, session_sample_retain_seconds,
		       session_export_retain_seconds, stream_event_retain_seconds,
		       principal_json, reason, document_digest, updated_at
		FROM integration_retention_policies
		WHERE tenant_id = $1
	`, s.tenantID).Scan(&policy.Version, &canonical, &sample, &export, &stream,
		&principalJSON, &policy.Reason, &policy.DocumentDigest, &policy.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read retention policy: %w", err)
	}
	policy.TenantID = s.tenantID
	policy.CanonicalEventRetain = secondsToDuration(canonical)
	policy.SessionSampleRetain = secondsToDuration(sample)
	policy.SessionExportRetain = secondsToDuration(export)
	policy.StreamEventRetain = secondsToDuration(stream)
	if err := json.Unmarshal(principalJSON, &policy.Principal); err != nil {
		return nil, fmt.Errorf("decode retention policy principal: %w", err)
	}
	return &policy, nil
}

// PutPolicy records a retention policy, bumping the version and writing an
// append-only audit row.
//
// A document whose digest matches the record in force is not a policy change:
// restarting a replica must not mint a version or forge an audit entry claiming
// an operator changed something. That idempotence is why the digest is stored.
func (s *PostgresStore) PutPolicy(ctx context.Context, policy Policy) (Policy, error) {
	if s == nil || s.db == nil || ctx == nil {
		return Policy{}, ErrStoreUnavailable
	}
	if policy.TenantID != s.tenantID {
		return Policy{}, fmt.Errorf("%w: policy tenant %q does not match store tenant %q",
			ErrInvalidPolicy, policy.TenantID, s.tenantID)
	}
	if strings.TrimSpace(policy.DocumentDigest) == "" {
		return Policy{}, fmt.Errorf("%w: document digest is required", ErrInvalidPolicy)
	}
	if err := validatePolicyPrincipal(policy.Principal); err != nil {
		return Policy{}, err
	}
	principalJSON, err := json.Marshal(clonePrincipal(policy.Principal))
	if err != nil {
		return Policy{}, fmt.Errorf("encode retention policy principal: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Policy{}, fmt.Errorf("begin retention policy write: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock($1)`, policyWriteLockKey); err != nil {
		return Policy{}, fmt.Errorf("lock retention policy write: %w", err)
	}

	var currentVersion int64
	var currentDigest string
	switch err := tx.QueryRowContext(ctx, `
		SELECT policy_version, document_digest FROM integration_retention_policies
		WHERE tenant_id = $1 FOR UPDATE
	`, s.tenantID).Scan(&currentVersion, &currentDigest); {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return Policy{}, fmt.Errorf("lock retention policy: %w", err)
	case currentDigest == policy.DocumentDigest:
		if err := tx.Commit(); err != nil {
			return Policy{}, fmt.Errorf("commit unchanged retention policy: %w", err)
		}
		existing, readErr := s.Policy(ctx)
		if readErr != nil {
			return Policy{}, readErr
		}
		if existing == nil {
			return Policy{}, fmt.Errorf("retention policy vanished after digest match")
		}
		return *existing, nil
	}

	recorded := s.now()
	next := policy
	next.Version = currentVersion + 1
	next.UpdatedAt = recorded
	args := []any{
		s.tenantID, next.Version,
		durationToSeconds(next.CanonicalEventRetain), durationToSeconds(next.SessionSampleRetain),
		durationToSeconds(next.SessionExportRetain), durationToSeconds(next.StreamEventRetain),
		principalJSON, next.Reason, next.DocumentDigest, recorded,
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO integration_retention_policies (
			tenant_id, policy_version, canonical_event_retain_seconds, session_sample_retain_seconds,
			session_export_retain_seconds, stream_event_retain_seconds,
			principal_json, reason, document_digest, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (tenant_id) DO UPDATE SET
			policy_version = EXCLUDED.policy_version,
			canonical_event_retain_seconds = EXCLUDED.canonical_event_retain_seconds,
			session_sample_retain_seconds = EXCLUDED.session_sample_retain_seconds,
			session_export_retain_seconds = EXCLUDED.session_export_retain_seconds,
			stream_event_retain_seconds = EXCLUDED.stream_event_retain_seconds,
			principal_json = EXCLUDED.principal_json,
			reason = EXCLUDED.reason,
			document_digest = EXCLUDED.document_digest,
			updated_at = EXCLUDED.updated_at
	`, args...); err != nil {
		return Policy{}, fmt.Errorf("record retention policy: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO integration_retention_policy_audit (
			tenant_id, policy_version, canonical_event_retain_seconds, session_sample_retain_seconds,
			session_export_retain_seconds, stream_event_retain_seconds,
			principal_json, reason, document_digest, recorded_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`, args...); err != nil {
		return Policy{}, fmt.Errorf("audit retention policy: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return Policy{}, fmt.Errorf("commit retention policy: %w", err)
	}
	return next, nil
}

// PurgeExpired reconciles expiry state and purges everything past its deadline,
// bounded by batchSize per class.
//
// It needs no lease. Every statement below is an idempotent guarded write whose
// RETURNING clause IS the claim: a row that another replica already tombstoned
// no longer matches, so only the replica that claims a row writes its audit
// entry — and the audit write is part of the same statement, so a purge without
// an audit row cannot be expressed. This follows S3-A's rejection of
// pg_advisory_lock for the autoroute notifier (.loom/40-decisions.md).
//
// EVERY CLASS IS ATTEMPTED, and the errors are joined. Before Sprint 5 this
// returned on the first class's error, so one poisoned class — a revoked grant,
// a lock timeout, a corrupt row — stopped every remaining class for that pass,
// and on an hourly cadence that is an hour of retention not enforced for
// classes that were healthy. The counts returned alongside a non-nil error are
// real: whatever did purge, purged, and its audit rows are committed.
//
// SATURATION. Counts.Saturated reports that at least one bounded statement came
// back full, which means backlog remains. The caller drains on it rather than
// waiting a whole tick per batch — the shape internal/integration/session's
// stream relay already uses (stream.go:174-179). Stamping counts toward
// saturation as well as purging: a canonical event is unpurgeable until
// purge_after is stamped (see stampCanonicalEvents), and the stamp carries the
// same LIMIT, so a stamp-bound backlog is exactly as invisible as a purge-bound
// one was.
func (s *PostgresStore) PurgeExpired(ctx context.Context, batchSize int) (PurgeCounts, error) {
	if s == nil || s.db == nil || ctx == nil {
		return PurgeCounts{}, ErrStoreUnavailable
	}
	if batchSize <= 0 {
		batchSize = defaultBatchSize
	}
	policy, err := s.Policy(ctx)
	if err != nil {
		return PurgeCounts{}, err
	}
	if policy == nil {
		// Fail-closed: no policy record, no purge, no error. An unconfigured
		// deployment must never destroy clinical data.
		return PurgeCounts{}, nil
	}

	var counts PurgeCounts
	var failures []error
	// saturated records that a bounded statement returned a full batch. It is
	// deliberately accumulated across classes: a healthy class with backlog must
	// keep draining even while another class is failing.
	saturate := func(affected int64) {
		if affected >= int64(batchSize) {
			counts.Saturated = true
		}
	}

	if policy.CanonicalEventRetain > 0 {
		stamped, err := s.stampCanonicalEvents(ctx, policy.CanonicalEventRetain, batchSize)
		saturate(stamped)
		if err != nil {
			failures = append(failures, err)
		} else {
			purged, err := s.purgeCanonicalEvents(ctx, policy.Version, batchSize)
			counts.CanonicalEvents = purged
			saturate(purged)
			if err != nil {
				failures = append(failures, err)
			}
		}
	}
	if policy.SessionSampleRetain > 0 {
		stamped, err := s.stampSessionSamples(ctx, policy.SessionSampleRetain, batchSize)
		saturate(stamped)
		if err != nil {
			failures = append(failures, err)
		} else {
			purged, err := s.purgeSessionSamples(ctx, policy.Version, batchSize)
			counts.SessionSamples = purged
			saturate(purged)
			if err != nil {
				failures = append(failures, err)
			}
		}
	}
	if policy.SessionExportRetain > 0 {
		stamped, err := s.stampSessionExports(ctx, policy.SessionExportRetain, batchSize)
		saturate(stamped)
		if err != nil {
			failures = append(failures, err)
		} else {
			purged, err := s.purgeSessionExports(ctx, policy.Version, batchSize)
			counts.SessionExports = purged
			saturate(purged)
			if err != nil {
				failures = append(failures, err)
			}
		}
	}
	if policy.StreamEventRetain > 0 {
		pruned, err := s.pruneStreamEvents(ctx, policy.StreamEventRetain, batchSize)
		counts.StreamEvents = pruned
		saturate(pruned)
		if err != nil {
			failures = append(failures, err)
		}
	}
	if len(failures) > 0 {
		// A failing class must not look like an exhausted one. Draining on a
		// failure would spin the loop against a broken class for the whole
		// budget; the next tick retries, which is the existing contract.
		counts.Saturated = false
		return counts, errors.Join(failures...)
	}
	return counts, nil
}

// BacklogCounts reports how many records are eligible for purge right now, per
// record class.
//
// "Eligible" means exactly what each purge statement means by it, interlocks
// included, so a count of zero and a purge that acts on nothing are the same
// statement. A canonical event withheld by the delivery interlock is not
// backlog — it is correctly retained — and counting it would leave the gauge
// permanently non-zero on a busy deployment, which is indistinguishable from
// the condition the gauge exists to reveal.
//
// It deliberately does NOT read purge_after. It recomputes the deadline from
// the record's own timestamp and the policy window in force, so a row that has
// never been stamped still counts. That is the half of D1 the counters could
// not have shown: stamping carries the same LIMIT as purging, so a stamp-bound
// backlog is real backlog.
type BacklogCounts struct {
	CanonicalEvents int64
	SessionSamples  int64
	SessionExports  int64
	StreamEvents    int64
}

// Total reports the whole backlog across every class.
func (c BacklogCounts) Total() int64 {
	return c.CanonicalEvents + c.SessionSamples + c.SessionExports + c.StreamEvents
}

// Backlog counts the records past their retention deadline that have not been
// purged.
//
// Retention observability was counters only before Sprint 5
// (internal/observability/metrics.go), so "falling behind" and "healthy and
// busy" produced identical exposition — which is why a purge ceiling of 200
// records per class per hour shipped and stayed invisible. This is the gauge
// that separates them.
//
// An unconfigured deployment has no policy and therefore no backlog, not an
// error: the same fail-closed posture PurgeExpired takes.
func (s *PostgresStore) Backlog(ctx context.Context) (BacklogCounts, error) {
	if s == nil || s.db == nil || ctx == nil {
		return BacklogCounts{}, ErrStoreUnavailable
	}
	policy, err := s.Policy(ctx)
	if err != nil {
		return BacklogCounts{}, err
	}
	if policy == nil {
		return BacklogCounts{}, nil
	}

	now := s.now()
	var counts BacklogCounts
	var failures []error

	if policy.CanonicalEventRetain > 0 {
		// The NOT EXISTS clause is purgeCanonicalEvents' delivery interlock,
		// character for character. If the two ever diverge, the gauge stops
		// meaning "what the purge would act on".
		if err := s.db.QueryRowContext(ctx, `
			SELECT count(*)
			FROM integration_canonical_events e
			WHERE e.tenant_id = $1
			  AND e.purged_at IS NULL
			  AND e.recorded_at + make_interval(secs => $2) <= $3
			  AND NOT EXISTS (
				SELECT 1
				FROM integration_delivery_attempts a
				WHERE a.tenant_id = e.tenant_id
				  AND a.event_id = e.event_id
				  AND (
					a.status = 'queued'
					OR EXISTS (
						SELECT 1 FROM integration_delivery_dlq d
						WHERE d.tenant_id = a.tenant_id AND d.attempt_id = a.attempt_id
						  AND d.active
					)
				  )
			  )
		`, s.tenantID, policy.CanonicalEventRetain.Seconds(), now).Scan(&counts.CanonicalEvents); err != nil {
			failures = append(failures, fmt.Errorf("count canonical event backlog: %w", err))
		}
	}
	if policy.SessionSampleRetain > 0 {
		if err := s.db.QueryRowContext(ctx, `
			SELECT count(*) FROM integration_session_samples
			WHERE tenant_id = $1 AND created_at + make_interval(secs => $2) <= $3
		`, s.tenantID, policy.SessionSampleRetain.Seconds(), now).Scan(&counts.SessionSamples); err != nil {
			failures = append(failures, fmt.Errorf("count session sample backlog: %w", err))
		}
	}
	if policy.SessionExportRetain > 0 {
		if err := s.db.QueryRowContext(ctx, `
			SELECT count(*) FROM integration_session_exports
			WHERE tenant_id = $1 AND purged_at IS NULL
			  AND exported_at + make_interval(secs => $2) <= $3
		`, s.tenantID, policy.SessionExportRetain.Seconds(), now).Scan(&counts.SessionExports); err != nil {
			failures = append(failures, fmt.Errorf("count session export backlog: %w", err))
		}
	}
	if policy.StreamEventRetain > 0 {
		window := policy.StreamEventRetain
		if window < StreamEventPruneFloor {
			window = StreamEventPruneFloor
		}
		if err := s.db.QueryRowContext(ctx, `
			SELECT count(*) FROM integration_session_stream_events
			WHERE tenant_id = $1 AND created_at <= $2
		`, s.tenantID, now.Add(-window)).Scan(&counts.StreamEvents); err != nil {
			failures = append(failures, fmt.Errorf("count stream event backlog: %w", err))
		}
	}
	if len(failures) > 0 {
		return counts, errors.Join(failures...)
	}
	return counts, nil
}

// stampCanonicalEvents reconciles purge_after with the window currently in
// force. It is a reconciler, not a one-shot backfill: when an operator shortens
// or lengthens the window, successive passes converge the stored deadlines onto
// the new policy, and in a steady state it matches zero rows and costs one
// indexed query.
//
// The migration deliberately backfills nothing. Stamping happens only under an
// operator's attributed policy record, which is a decision somebody made rather
// than one a migration invented.
func (s *PostgresStore) stampCanonicalEvents(ctx context.Context, window time.Duration, batchSize int) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		WITH stale AS (
			SELECT tenant_id, event_id
			FROM integration_canonical_events
			WHERE tenant_id = $1
			  AND purged_at IS NULL
			  AND purge_after IS DISTINCT FROM recorded_at + make_interval(secs => $2)
			ORDER BY recorded_at, event_id
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		UPDATE integration_canonical_events e
		SET purge_after = e.recorded_at + make_interval(secs => $2)
		FROM stale s
		WHERE e.tenant_id = s.tenant_id AND e.event_id = s.event_id
	`, s.tenantID, window.Seconds(), batchSize)
	if err != nil {
		return 0, fmt.Errorf("stamp canonical event expiry: %w", err)
	}
	return rowsAffected(result, "stamped canonical events")
}

// stampSessionSamples reconciles sample expiry against the window in force.
// A sample's clock starts when it was added to the workspace.
func (s *PostgresStore) stampSessionSamples(ctx context.Context, window time.Duration, batchSize int) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		WITH stale AS (
			SELECT tenant_id, sample_id
			FROM integration_session_samples
			WHERE tenant_id = $1
			  AND purge_after IS DISTINCT FROM created_at + make_interval(secs => $2)
			ORDER BY created_at, sample_id
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		UPDATE integration_session_samples t
		SET purge_after = t.created_at + make_interval(secs => $2)
		FROM stale s
		WHERE t.tenant_id = s.tenant_id AND t.sample_id = s.sample_id
	`, s.tenantID, window.Seconds(), batchSize)
	if err != nil {
		return 0, fmt.Errorf("stamp session sample expiry: %w", err)
	}
	return rowsAffected(result, "stamped session samples")
}

// stampSessionExports reconciles export-snapshot expiry against the window in
// force. The clock starts at the disclosure, not at the session's creation.
func (s *PostgresStore) stampSessionExports(ctx context.Context, window time.Duration, batchSize int) (int64, error) {
	result, err := s.db.ExecContext(ctx, `
		WITH stale AS (
			SELECT tenant_id, export_id
			FROM integration_session_exports
			WHERE tenant_id = $1
			  AND purged_at IS NULL
			  AND purge_after IS DISTINCT FROM exported_at + make_interval(secs => $2)
			ORDER BY exported_at, export_id
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		UPDATE integration_session_exports t
		SET purge_after = t.exported_at + make_interval(secs => $2)
		FROM stale s
		WHERE t.tenant_id = s.tenant_id AND t.export_id = s.export_id
	`, s.tenantID, window.Seconds(), batchSize)
	if err != nil {
		return 0, fmt.Errorf("stamp session export expiry: %w", err)
	}
	return rowsAffected(result, "stamped session exports")
}

// purgeCanonicalEvents tombstones expired payloads and writes their audit rows
// in one statement.
//
// The NOT EXISTS clause is the delivery interlock. An event whose delivery
// attempt is still claimable (`status = 'queued'`, exactly what
// internal/integration/delivery/store.go's Claim joins on) or still active in
// the dead-letter queue (a replay puts it straight back to 'queued') is never
// purged, so the Claim join can never observe a tombstone and publish one to a
// destination.
func (s *PostgresStore) purgeCanonicalEvents(ctx context.Context, policyVersion int64, batchSize int) (int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH expired AS (
			SELECT e.tenant_id, e.event_id
			FROM integration_canonical_events e
			WHERE e.tenant_id = $1
			  AND e.purged_at IS NULL
			  AND e.purge_after IS NOT NULL
			  AND e.purge_after <= $2
			  AND NOT EXISTS (
				SELECT 1
				FROM integration_delivery_attempts a
				WHERE a.tenant_id = e.tenant_id
				  AND a.event_id = e.event_id
				  AND (
					a.status = 'queued'
					OR EXISTS (
						SELECT 1 FROM integration_delivery_dlq d
						WHERE d.tenant_id = a.tenant_id AND d.attempt_id = a.attempt_id
						  AND d.active
					)
				  )
			  )
			ORDER BY e.purge_after, e.event_id
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		), purged AS (
			UPDATE integration_canonical_events e
			SET payload_json = integration_canonical_event_tombstone(), purged_at = $2
			FROM expired x
			WHERE e.tenant_id = x.tenant_id AND e.event_id = x.event_id AND e.purged_at IS NULL
			RETURNING e.event_id, e.purge_after
		)
		INSERT INTO integration_retention_purge_audit (
			tenant_id, record_class, record_id, policy_version, purge_mode, purge_after, purged_at
		)
		SELECT $1, 'canonical_event', p.event_id, $4, 'tombstone', p.purge_after, $2
		FROM purged p
		RETURNING record_id
	`, s.tenantID, s.now(), batchSize, policyVersion)
	if err != nil {
		return 0, fmt.Errorf("purge canonical events: %w", err)
	}
	return countReturnedRows(rows, "canonical event purge")
}

// purgeSessionSamples deletes expired samples outright, ciphertext included.
//
// integration_session_samples carries no immutability trigger, so the honest
// purge is removal. A tombstone here would invent a guarantee the table never
// had.
func (s *PostgresStore) purgeSessionSamples(ctx context.Context, policyVersion int64, batchSize int) (int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH expired AS (
			SELECT tenant_id, sample_id
			FROM integration_session_samples
			WHERE tenant_id = $1 AND purge_after IS NOT NULL AND purge_after <= $2
			ORDER BY purge_after, sample_id
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		), deleted AS (
			DELETE FROM integration_session_samples s
			USING expired x
			WHERE s.tenant_id = x.tenant_id AND s.sample_id = x.sample_id
			RETURNING s.sample_id, s.purge_after
		)
		INSERT INTO integration_retention_purge_audit (
			tenant_id, record_class, record_id, policy_version, purge_mode, purge_after, purged_at
		)
		SELECT $1, 'session_sample', d.sample_id, $4, 'deleted', d.purge_after, $2
		FROM deleted d
		RETURNING record_id
	`, s.tenantID, s.now(), batchSize, policyVersion)
	if err != nil {
		return 0, fmt.Errorf("purge session samples: %w", err)
	}
	return countReturnedRows(rows, "session sample purge")
}

// purgeSessionExports tombstones the export SNAPSHOT. The disclosure record —
// who exported, why, when, and whether raw payloads were included — is never
// purged, because it is the evidence Slice 4.1d C1 exists to preserve, and
// because correction 13 makes the row undeletable in any case.
func (s *PostgresStore) purgeSessionExports(ctx context.Context, policyVersion int64, batchSize int) (int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		WITH expired AS (
			SELECT tenant_id, export_id
			FROM integration_session_exports
			WHERE tenant_id = $1 AND purged_at IS NULL
			  AND purge_after IS NOT NULL AND purge_after <= $2
			ORDER BY purge_after, export_id
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		), purged AS (
			UPDATE integration_session_exports e
			SET record_json = integration_session_export_tombstone(), purged_at = $2
			FROM expired x
			WHERE e.tenant_id = x.tenant_id AND e.export_id = x.export_id AND e.purged_at IS NULL
			RETURNING e.export_id, e.purge_after
		)
		INSERT INTO integration_retention_purge_audit (
			tenant_id, record_class, record_id, policy_version, purge_mode, purge_after, purged_at
		)
		SELECT $1, 'session_export', p.export_id, $4, 'tombstone', p.purge_after, $2
		FROM purged p
		RETURNING record_id
	`, s.tenantID, s.now(), batchSize, policyVersion)
	if err != nil {
		return 0, fmt.Errorf("purge session exports: %w", err)
	}
	return countReturnedRows(rows, "session export purge")
}

// pruneStreamEvents trims the payload-free fanout log.
//
// This is a growth control, not a privacy control: the log carries envelopes,
// never clinical content. It therefore writes no purge-audit row — one row per
// pruned envelope would replace one unbounded table with another — and the
// schema keeps a 24 hour floor no deployment window can lower, so a subscriber
// reconnecting cannot have its cursor pruned out from under it.
func (s *PostgresStore) pruneStreamEvents(ctx context.Context, window time.Duration, batchSize int) (int64, error) {
	if window < StreamEventPruneFloor {
		window = StreamEventPruneFloor
	}
	result, err := s.db.ExecContext(ctx, `
		WITH stale AS (
			SELECT seq FROM integration_session_stream_events
			WHERE tenant_id = $1 AND created_at <= $2
			ORDER BY seq
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		DELETE FROM integration_session_stream_events e
		USING stale s
		WHERE e.seq = s.seq
	`, s.tenantID, s.now().Add(-window), batchSize)
	if err != nil {
		return 0, fmt.Errorf("prune session stream events: %w", err)
	}
	return rowsAffected(result, "pruned session stream events")
}

// rowsAffected turns a bounded statement's row count into the saturation signal
// PurgeExpired drains on. A driver that cannot report it is an error rather than
// a silent zero: a zero would read as "no backlog" and stop the drain.
func rowsAffected(result sql.Result, what string) (int64, error) {
	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count %s: %w", what, err)
	}
	return affected, nil
}

func countReturnedRows(rows *sql.Rows, what string) (int64, error) {
	defer func() { _ = rows.Close() }()
	var count int64
	for rows.Next() {
		var recordID string
		if err := rows.Scan(&recordID); err != nil {
			return count, fmt.Errorf("scan %s: %w", what, err)
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return count, fmt.Errorf("iterate %s: %w", what, err)
	}
	return count, nil
}

func durationToSeconds(window time.Duration) any {
	if window <= 0 {
		return nil
	}
	return int64(window / time.Second)
}

func secondsToDuration(value sql.NullInt64) time.Duration {
	if !value.Valid || value.Int64 <= 0 {
		return 0
	}
	return time.Duration(value.Int64) * time.Second
}
