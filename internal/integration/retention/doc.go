// Package retention owns PHI retention policy and the durable purge runtime for
// the integration engine (Slice 4.1e).
//
// At the time this package was created it contained nothing but its day-1 gate,
// TestPhiRetention_PurgeIsStructurallyBlockedToday, which proves against an
// unmodified database that a purge cannot be built the obvious way: Slice 4.1d
// C1's blanket BEFORE UPDATE OR DELETE triggers block the DELETE *and* the
// redaction UPDATE, and the ON DELETE RESTRICT chains terminating in undeletable
// state tables make row deletion structurally impossible regardless of what a
// purge component does.
//
// See .loom/32-sprint4-execution-specs.md, "Lane S4-B", corrections 11-16.
package retention
