// Package migrationcompat holds Slice 4.4a's cross-package migration
// compatibility proofs.
//
// It deliberately contains no production code. Every other durable package
// owns exactly one migration ledger and can only prove statements about its
// own schema; the properties 4.4a has to establish — that two replicas
// starting simultaneously converge across all six ledgers, that a one-version
// binary rollback survives the migrated schema, and that a pg_dump/restore
// round-trip preserves receipts, revisions, and resumable delivery work —
// are statements about the ledgers *together*. They need a package that may
// import all of them without any of them importing it.
//
// The proofs are behind the `integration` build tag and run in the
// `test:migration-compatibility` CI job against a dedicated PostgreSQL 16
// database.
package migrationcompat
