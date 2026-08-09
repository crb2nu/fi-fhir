package mllp

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrQuotaUnavailable means the durable quota could not be read or written.
// The coordinator treats it the same as any other store failure: keep the
// current grant until its lease expires, then degrade to the conservative
// share. It is never a reason to admit at the full declared rate.
var ErrQuotaUnavailable = errors.New("MLLP rate quota is unavailable")

// PostgresQuotaStore is the durable side of the lease-partitioned rate quota,
// backed by integration_mllp_rate_claims (lifecycle migration 0002).
//
// It is called from the claim loop on an interval measured in seconds, never
// from the admission path. Nothing here runs per frame.
type PostgresQuotaStore struct {
	db *sql.DB
}

// NewPostgresQuotaStore constructs the store. It performs no I/O and does not
// migrate: the lifecycle catalog owns the schema.
func NewPostgresQuotaStore(db *sql.DB) (*PostgresQuotaStore, error) {
	if db == nil {
		return nil, ErrQuotaUnavailable
	}
	return &PostgresQuotaStore{db: db}, nil
}

// Claim reaps this deployment's expired claims, records the caller's, counts
// the live holders, and returns the caller's share — all inside one
// transaction.
//
// The single transaction is the bound. A share computed against a holder count
// that changes before it is written is not a bound at all, which is the whole
// difference between this and a best-effort count. Two replicas claiming
// concurrently serialise on the same rows and each sees the other.
//
// Timestamps are server-owned: the caller supplies the lease length, never the
// instant. A replica with a skewed clock cannot extend its own lease.
func (s *PostgresQuotaStore) Claim(
	ctx context.Context,
	key QuotaKey,
	holderID string,
	revisionDigest string,
	declaredRate int,
	lease time.Duration,
) (QuotaClaim, error) {
	if s == nil || s.db == nil || ctx == nil {
		return QuotaClaim{}, ErrQuotaUnavailable
	}
	if !key.valid() || holderID == "" || declaredRate < 1 || lease <= 0 {
		return QuotaClaim{}, fmt.Errorf("%w: invalid claim", ErrQuotaUnavailable)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return QuotaClaim{}, fmt.Errorf("begin rate claim: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Reap first. An expired holder is gone — a replica that died, or one whose
	// renewals have been failing for longer than the lease — and counting it
	// would keep its share stranded.
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM integration_mllp_rate_claims
		WHERE tenant_id = $1 AND definition_id = $2 AND expires_at <= clock_timestamp()
	`, key.TenantID, key.DefinitionID); err != nil {
		return QuotaClaim{}, fmt.Errorf("reap expired rate claims: %w", err)
	}

	// Record the claim with a provisional share, so the caller is counted among
	// the live holders by the query that computes the real one. The share is
	// rewritten below; the provisional value only has to satisfy the CHECK.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO integration_mllp_rate_claims (
			tenant_id, definition_id, holder_id, revision_digest,
			declared_rate, granted_share, holders, claimed_at, expires_at
		) VALUES ($1, $2, $3, $4, $5, 1, 1, clock_timestamp(), clock_timestamp() + $6::interval)
		ON CONFLICT (tenant_id, definition_id, holder_id) DO UPDATE SET
			revision_digest = EXCLUDED.revision_digest,
			declared_rate = EXCLUDED.declared_rate,
			claimed_at = clock_timestamp(),
			expires_at = clock_timestamp() + $6::interval
	`, key.TenantID, key.DefinitionID, holderID, revisionDigest, declaredRate, intervalArg(lease)); err != nil {
		return QuotaClaim{}, fmt.Errorf("record rate claim: %w", err)
	}

	// Count the live holders and this holder's rank among them. Ordering by
	// holder id is what makes the remainder assignment stable: every replica
	// computing the split independently reaches the same answer, so the shares
	// sum to the declared rate rather than merely approximating it.
	var holders, index int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*), count(*) FILTER (WHERE holder_id < $3)
		FROM integration_mllp_rate_claims
		WHERE tenant_id = $1 AND definition_id = $2
	`, key.TenantID, key.DefinitionID, holderID).Scan(&holders, &index); err != nil {
		return QuotaClaim{}, fmt.Errorf("count rate claim holders: %w", err)
	}

	share := partitionShare(declaredRate, holders, index)
	var expiresAt time.Time
	if err := tx.QueryRowContext(ctx, `
		UPDATE integration_mllp_rate_claims
		SET granted_share = $4, holders = $5
		WHERE tenant_id = $1 AND definition_id = $2 AND holder_id = $3
		RETURNING expires_at
	`, key.TenantID, key.DefinitionID, holderID, share, holders).Scan(&expiresAt); err != nil {
		return QuotaClaim{}, fmt.Errorf("grant rate claim share: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return QuotaClaim{}, fmt.Errorf("commit rate claim: %w", err)
	}
	return QuotaClaim{Share: share, Holders: holders, ExpiresAt: expiresAt.UTC()}, nil
}

// Release drops this replica's claim so the share returns to the pool at once
// rather than after the lease expires. A graceful shutdown during a rolling
// redeploy is the case that matters: the replacement replica should not have to
// wait out a lease for capacity the departing one is no longer using.
func (s *PostgresQuotaStore) Release(ctx context.Context, key QuotaKey, holderID string) error {
	if s == nil || s.db == nil || ctx == nil {
		return ErrQuotaUnavailable
	}
	if !key.valid() || holderID == "" {
		return fmt.Errorf("%w: invalid release", ErrQuotaUnavailable)
	}
	if _, err := s.db.ExecContext(ctx, `
		DELETE FROM integration_mllp_rate_claims
		WHERE tenant_id = $1 AND definition_id = $2 AND holder_id = $3
	`, key.TenantID, key.DefinitionID, holderID); err != nil {
		return fmt.Errorf("release rate claim: %w", err)
	}
	return nil
}

// intervalArg renders a lease as a PostgreSQL interval literal. Sent as a
// parameter rather than interpolated, and expressed in milliseconds so a
// sub-second lease in a test survives the round trip.
func intervalArg(lease time.Duration) string {
	return fmt.Sprintf("%d milliseconds", lease.Milliseconds())
}
