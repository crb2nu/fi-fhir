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

// Claim reaps this deployment's expired claims, records the caller's, and
// rebalances every live holder's share — all inside one transaction that is
// serialised per deployment.
//
// The serialisation is the bound. Every replica writes its own row, so nothing
// in the statements themselves blocks a concurrent claimant, and under READ
// COMMITTED each transaction's count sees only committed rows plus its own.
// Ten replicas starting together therefore each computed a share against a
// count of one or two: CI recorded 190 granted against a declared 100
// (test:mllp-rate-quota, every run from pipeline 22968 on) where a fast local
// run had happened to interleave cleanly. A transaction-scoped advisory lock on
// the deployment key makes the count exact.
//
// Rewriting every holder's share, not only the caller's, is the second half. A
// holder whose last renewal predates a later arrival would otherwise keep its
// larger share until its next interval, and the table would sum to more than
// the declared rate in the meantime. With the rebalance the persisted shares
// sum to the declared rate after every commit; a holder whose share shrank
// learns it on its next claim, within one interval. Neither the lock nor the
// rebalance runs on the admission path.
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

	// Serialise claims for this deployment. Released at commit or rollback, and
	// keyed on the deployment rather than the digest for the same reason the
	// table is: a rolling redeploy is one pool, not two.
	if _, err := tx.ExecContext(ctx, `
		SELECT pg_advisory_xact_lock(hashtextextended($1 || '/' || $2, 0))
	`, key.TenantID, key.DefinitionID); err != nil {
		return QuotaClaim{}, fmt.Errorf("serialise rate claims: %w", err)
	}

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

	// Every live holder, in the order the split is defined over. Ordering by
	// holder id is what makes the remainder assignment stable: every replica
	// computing the split reaches the same answer, so the shares sum to the
	// declared rate rather than merely approximating it.
	//
	// The pool rate is the smallest declaration among the live holders. During a
	// rolling redeploy that changes max_messages_per_second the old and new
	// revisions are live at once; a share computed from the larger declaration
	// would breach the CHECK on the rows carrying the smaller one, and the bound
	// the older revision's operator still believes in. The deployment runs at the
	// lower rate until the older revision drains.
	live, err := listClaimHolders(ctx, tx, key)
	if err != nil {
		return QuotaClaim{}, err
	}
	var (
		holderIDs []string
		poolRate  = declaredRate
		expiresAt time.Time
		found     bool
	)
	for _, holder := range live {
		if holder.declaredRate < poolRate {
			poolRate = holder.declaredRate
		}
		if holder.holderID == holderID {
			expiresAt, found = holder.expiresAt, true
		}
		holderIDs = append(holderIDs, holder.holderID)
	}
	if !found {
		return QuotaClaim{}, fmt.Errorf("%w: claim for %q vanished inside its own transaction", ErrQuotaUnavailable, holderID)
	}

	holders := len(holderIDs)
	var share int
	for index, id := range holderIDs {
		granted := partitionShare(poolRate, holders, index)
		if id == holderID {
			share = granted
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE integration_mllp_rate_claims
			SET granted_share = $4, holders = $5
			WHERE tenant_id = $1 AND definition_id = $2 AND holder_id = $3
		`, key.TenantID, key.DefinitionID, id, granted, holders); err != nil {
			return QuotaClaim{}, fmt.Errorf("grant rate claim share: %w", err)
		}
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

// claimHolder is one live row of integration_mllp_rate_claims as the rebalance
// sees it: identity, the rate that row's revision declared, and its lease end.
type claimHolder struct {
	holderID     string
	declaredRate int
	expiresAt    time.Time
}

// listClaimHolders reads every live holder of a deployment in holder-id order,
// the order partitionShare's remainder assignment is defined over. It drains and
// closes the cursor before returning: lib/pq allows one active statement per
// connection, and the caller issues more on the same transaction.
func listClaimHolders(ctx context.Context, tx *sql.Tx, key QuotaKey) ([]claimHolder, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT holder_id, declared_rate, expires_at
		FROM integration_mllp_rate_claims
		WHERE tenant_id = $1 AND definition_id = $2
		ORDER BY holder_id
	`, key.TenantID, key.DefinitionID)
	if err != nil {
		return nil, fmt.Errorf("list rate claim holders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var live []claimHolder
	for rows.Next() {
		var holder claimHolder
		if err := rows.Scan(&holder.holderID, &holder.declaredRate, &holder.expiresAt); err != nil {
			return nil, fmt.Errorf("scan rate claim holder: %w", err)
		}
		live = append(live, holder)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list rate claim holders: %w", err)
	}
	return live, nil
}

// intervalArg renders a lease as a PostgreSQL interval literal. Sent as a
// parameter rather than interpolated, and expressed in milliseconds so a
// sub-second lease in a test survives the round trip.
func intervalArg(lease time.Duration) string {
	return fmt.Sprintf("%d milliseconds", lease.Milliseconds())
}
