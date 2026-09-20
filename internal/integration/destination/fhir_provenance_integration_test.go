//go:build integration

package destination

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// TestFHIRDestination_ProvenanceLedgerRecordsFHIRDeliveries proves migration
// 0003 against PostgreSQL 16: a `fhir` delivery is recorded with its three
// facts, the N-1 binary's thirteen-column `https` INSERT — which names neither
// the transport 'fhir' nor the new columns — still lands and reads back with
// the DEFAULTs, and the widened CHECK still refuses a transport outside
// {https, fhir}.
func TestFHIRDestination_ProvenanceLedgerRecordsFHIRDeliveries(t *testing.T) {
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		t.Skip("POSTGRES_TEST_URL is not set")
	}
	ctx := context.Background()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.PingContext(ctx); err != nil {
		t.Skipf("PostgreSQL unreachable: %v", err)
	}

	provenance, err := NewPostgresProvenance(db)
	if err != nil {
		t.Fatalf("NewPostgresProvenance: %v", err)
	}
	if err := provenance.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	var head int
	if err := db.QueryRowContext(ctx,
		`SELECT coalesce(max(version), 0) FROM integration_destination_schema_migrations`).Scan(&head); err != nil {
		t.Fatalf("read ledger head: %v", err)
	}
	if head != SchemaVersion || SchemaVersion != 3 {
		t.Fatalf("destination ledger head = %d, want SchemaVersion %d = 3", head, SchemaVersion)
	}

	const tenant = "tenant-fhir-ledger"
	if _, err := db.ExecContext(ctx,
		`DELETE FROM integration_destination_deliveries WHERE tenant_id = $1`, tenant); err != nil {
		t.Fatalf("clean: %v", err)
	}
	completed := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
	digest := "sha256:" + strings.Repeat("c", 64)

	// A fhir delivery through the recorder.
	if err := provenance.RecordDelivery(ctx, DeliveryRecord{
		TenantID: tenant, AttemptID: "attempt-fhir-1", Transport: TransportFHIR,
		DestinationArtifactID: "dest-fhir", DestinationRevisionID: "destination-1",
		DestinationClass: "production", DestinationDigestVerified: digest,
		Outcome: outcomeDelivered, HTTPStatusClass: "2xx",
		EndpointAdvisory: "https://fhir.example.org/r4", CompletedAt: completed,
		FHIRResourceTypes: "Patient,Encounter", FHIREntryCount: 2,
	}); err != nil {
		t.Fatalf("RecordDelivery(fhir): %v", err)
	}
	if err := provenance.RecordDelivery(ctx, DeliveryRecord{
		TenantID: tenant, AttemptID: "attempt-fhir-2", Transport: TransportFHIR,
		DestinationArtifactID: "dest-fhir", DestinationRevisionID: "destination-1",
		DestinationClass: "production", DestinationDigestVerified: digest,
		Outcome: outcomeRefused, FailureCode: FailureRejected, HTTPStatusClass: "4xx",
		EndpointAdvisory: "https://fhir.example.org/r4", CompletedAt: completed,
		FHIRResourceTypes: "Patient,Encounter", FHIREntryCount: 2, FHIROutcomeCodesAdvisory: "invalid,not-found",
	}); err != nil {
		t.Fatalf("RecordDelivery(fhir refused): %v", err)
	}

	// The N-1 binary's exact 0002-shaped INSERT.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO integration_destination_deliveries (
			tenant_id, attempt_id, transport,
			destination_artifact_id, destination_revision_id, destination_class,
			destination_digest_verified, outcome, failure_code, http_status_class,
			destination_endpoint_advisory, served_certificate_subject_advisory,
			completed_at
		) VALUES ($1, 'attempt-https-n1', 'https', 'dest-https', 'destination-1', 'production',
			$2, 'delivered', '', '2xx', 'https://destination.example/ingest', '', $3)
	`, tenant, digest, completed); err != nil {
		t.Fatalf("N-1 https insert failed against the migrated schema: %v", err)
	}

	rows, err := db.QueryContext(ctx, `
		SELECT attempt_id, transport, outcome, fhir_resource_types, fhir_entry_count, fhir_outcome_codes_advisory
		FROM integration_destination_deliveries WHERE tenant_id = $1 ORDER BY attempt_id
	`, tenant)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	defer rows.Close()
	type row struct {
		attempt, transport, outcome, types, codes string
		entries                                   int
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.attempt, &r.transport, &r.outcome, &r.types, &r.entries, &r.codes); err != nil {
			t.Fatalf("scan: %v", err)
		}
		got = append(got, r)
	}
	want := []row{
		{"attempt-fhir-1", "fhir", "delivered", "Patient,Encounter", "", 2},
		{"attempt-fhir-2", "fhir", "refused", "Patient,Encounter", "invalid,not-found", 2},
		{"attempt-https-n1", "https", "delivered", "", "", 0},
	}
	if len(got) != len(want) {
		t.Fatalf("rows = %+v, want %+v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("row %d = %+v, want %+v", index, got[index], want[index])
		}
	}

	// The widened CHECK is still closed.
	if _, err := db.ExecContext(ctx, `
		INSERT INTO integration_destination_deliveries (
			tenant_id, attempt_id, transport, destination_artifact_id, destination_revision_id,
			destination_class, destination_digest_verified, outcome, http_status_class, completed_at
		) VALUES ($1, 'attempt-mllp', 'mllp', 'dest', 'destination-1', 'production', $2, 'delivered', '2xx', $3)
	`, tenant, digest, completed); err == nil {
		t.Fatal("the transport CHECK admitted 'mllp'")
	}
	if err := provenance.RecordDelivery(ctx, DeliveryRecord{
		TenantID: tenant, AttemptID: "attempt-kafka", Transport: TransportKafka,
		DestinationDigestVerified: digest, Outcome: outcomeDelivered, CompletedAt: completed,
	}); err == nil {
		t.Fatal("RecordDelivery accepted a kafka transport")
	}
}
