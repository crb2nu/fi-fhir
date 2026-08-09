// Package retention owns PHI retention policy and the durable purge runtime for
// the integration engine (Slice 4.1e).
//
// It began as a single test. TestPhiRetention_PurgeIsStructurallyBlockedToday
// proved against an unmodified database that a purge could not be built the
// obvious way: Slice 4.1d C1's blanket triggers blocked the DELETE *and* the
// redaction UPDATE, and the ON DELETE RESTRICT chains terminating in undeletable
// state tables made row deletion impossible regardless of what a purge component
// did. That result, not a policy document, decided the shape of everything here.
//
// So a purge in this package is a TOMBSTONE wherever the record is guarded, and
// an outright delete only where the table never carried a guarantee to begin
// with:
//
//   - integration_canonical_events — payload replaced by the canonical
//     tombstone, purged_at set, row and identity kept so an audit can still show
//     what existed.
//   - integration_session_exports — snapshot tombstoned, disclosure attribution
//     frozen. The row is evidence, and correction 13 makes it undeletable anyway.
//   - integration_session_samples — row deleted outright, ciphertext included.
//     No immutability trigger exists there, and a tombstone would invent a
//     guarantee the table never had.
//   - integration_session_stream_events — pruned past a schema floor. Envelopes,
//     no PHI: a growth control rather than a privacy control.
//
// A tombstone is not a backup-inclusive deletion. A backup taken before the
// purge still holds the payload; expiring backup copies is a storage-layer
// control this package cannot reach.
//
// The purge takes no lease and elects no leader. Every write is an idempotent
// guarded statement whose RETURNING clause is the claim, and the audit INSERT
// reads from that RETURNING in the same statement, so a purge without an audit
// row cannot be expressed and two replicas produce exactly one of each.
//
// See .loom/40-decisions.md (2026-08-08, "Slice 4.1e"),
// .loom/32-sprint4-execution-specs.md Lane S4-B corrections 11-20, and
// docs/operations/PHI-RETENTION.md.
package retention
