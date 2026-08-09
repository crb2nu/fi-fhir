package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/destination"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
	termdb "gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

// schemaLedger names one forward-only migration ledger and the version this
// binary expects of it.
type schemaLedger struct {
	Name    string
	Version int
}

// schemaLedgers is the complete set of durable ledgers, and it is the operative
// definition of "one version" for this repository (slice 4.4a, task 1).
//
// The repository has zero git tags and `version` below is a build stamp, so
// neither can express compatibility: two binaries built from different commits
// may expect identical schemas, and two binaries reporting the same version
// string may not. What actually determines whether a process can run against a
// database is the set of ledger versions it expects. An N-1 pair is two
// binaries whose ledger versions differ by exactly one in exactly one ledger —
// which is what a rolling upgrade holds transiently and what a rollback holds
// indefinitely.
//
// Each constant lives in the package that owns the ledger, so adding a
// migration and forgetting to update the reported version is not possible
// without also editing the owning package. A migrationcompat proof asserts
// every entry here equals the ledger's actual maximum applied version after
// Migrate, so the reported number cannot drift from the schema.
//
// See `.loom/40-decisions.md` (2026-08-09, "What one version means").
func schemaLedgers() []schemaLedger {
	return []schemaLedger{
		{Name: observability.SchemaLedgerSubmission, Version: processor.SchemaVersion},
		{Name: observability.SchemaLedgerSession, Version: session.SchemaVersion},
		{Name: observability.SchemaLedgerLifecycle, Version: lifecycle.SchemaVersion},
		{Name: observability.SchemaLedgerBatch, Version: batch.SchemaVersion},
		{Name: observability.SchemaLedgerDestination, Version: destination.SchemaVersion},
		{Name: observability.SchemaLedgerTerminology, Version: termdb.SchemaVersion},
	}
}

// printVersion writes the build stamp and every ledger version. An operator
// diagnosing a rolling upgrade needs the second line, not the first: the build
// stamp tells them which artifact is running, the ledger versions tell them
// whether it can talk to the database in front of it.
func printVersion(w io.Writer, buildVersion string) {
	var b strings.Builder
	fmt.Fprintf(&b, "fi-fhir version %s\n", buildVersion)
	b.WriteString("schema ledger versions (the compatibility boundary; see docs/operations/SUPPORTED-1.0.md):\n")
	for _, ledger := range schemaLedgers() {
		fmt.Fprintf(&b, "  %-12s %d\n", ledger.Name, ledger.Version)
	}
	// Version output going nowhere is not worth failing a command over, but it
	// is worth not pretending the write succeeded.
	if _, err := io.WriteString(w, b.String()); err != nil {
		fmt.Fprintf(os.Stderr, "fi-fhir: could not write version output: %v\n", err)
	}
}

// recordSchemaLedgerVersions publishes the same numbers as a gauge so two
// replicas mid-rolling-upgrade are distinguishable in Prometheus rather than
// only over SSH.
//
// It is a separate metric rather than six more labels on `fi_fhir_build_info`
// deliberately: an info metric's labels all change together, whereas ledger
// versions move independently and are the thing an upgrade alert wants to
// compare across replicas.
func recordSchemaLedgerVersions(metrics *observability.Metrics) {
	for _, ledger := range schemaLedgers() {
		metrics.SetSchemaLedgerVersion(ledger.Name, ledger.Version)
	}
}
