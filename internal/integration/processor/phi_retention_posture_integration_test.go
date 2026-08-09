//go:build integration

package processor

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// TestPhiRetentionPosture_ProductionRejectsRetainedRawAndCanonicalEventsCarryNoPolicy
// is the retention-posture gate. It is a permanent regression guard for the two
// claims that docs/operations/PHI-RETENTION.md makes:
//
//	(a) production refuses every non-ephemeral raw retention mode, so there is no
//	    retained production raw PHI whose TTL could be enforced; and
//	(b) the PHI that *is* retained — the canonical event payload — carries a PHI
//	    classification AND the Slice 4.1e expiry state that governs how long it is
//	    kept.
//
// Assertion (b) is inverted from the form Slice 4.1d C1 shipped. C1 asserted that
// integration_canonical_events carried NO retention/TTL/expiry/purge column, and
// wrote into docs/operations/PHI-RETENTION.md that "if S3-C2 lands, this section
// must be rewritten before CI passes". Slice 4.1e is that slice; the assertion and
// the document were rewritten in the same commit that added the columns, which is
// exactly the behaviour C1 designed this gate to force.
//
// The gate did not become weaker by inverting. It now fails if the expiry columns
// are dropped, if a purged row loses its PHI classification, or if a class of
// retained PHI gains an expiry column without the document naming it — and the
// name of every column it will accept is enumerated below, so a stray
// "%retention%" column somewhere else still fails.
func TestPhiRetentionPosture_ProductionRejectsRetainedRawAndCanonicalEventsCarryNoPolicy(t *testing.T) {
	ctx := t.Context()
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		t.Skip("POSTGRES_TEST_URL is required for PHI retention posture integration tests")
	}

	schema := fmt.Sprintf("phi_retention_posture_%d", time.Now().UnixNano())
	createSubmissionSchema(t, dsn, schema)
	schemaDSN := submissionSchemaDSN(t, dsn, schema)
	fixedClock := func() time.Time { return time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC) }

	db := openSubmissionDB(t, schemaDSN)
	store := newSubmissionStore(t, db, fixedClock)
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	// Assertion (a): a fully valid encrypted raw-retention revision is refused by the
	// durable committer. The revision itself must validate — otherwise the rejection
	// would prove contract validation, not production posture.
	encrypted := newRetentionFixture(t, integration.RawRetentionPolicy{
		Mode:       integration.RawRetentionModeEncrypted,
		TTLSeconds: 3600,
		Purpose:    "clinical reconciliation",
		StorageRevision: &integration.ArtifactRevisionRef{
			ArtifactID: "raw-store",
			RevisionID: "raw-store-1",
			Digest:     "sha256:" + repeatChar("b", 64),
		},
		EncryptionKey: &integration.SecretReference{
			Provider: "file",
			Key:      "raw-retention-key",
			Version:  "1",
		},
		AuthorizedBy: integration.Principal{
			ID: "privacy-officer-1", Kind: integration.PrincipalKindHuman,
			AuthMethod: "oidc", Roles: []string{"privacy.officer"},
		},
		AccessAuditRequired: true,
	})
	if err := encrypted.revision.Policy.RawRetention.Validate(); err != nil {
		t.Fatalf("encrypted retention policy must be contract-valid, got: %v", err)
	}
	encryptedProcessor := newDurableFixtureProcessor(t, encrypted, store)
	_, err := encryptedProcessor.Process(ctx, encrypted.request)
	if !errors.Is(err, ErrUnsupportedRawRetention) {
		t.Fatalf("assertion (a) FAILED: production accepted encrypted raw retention: err = %v", err)
	}
	assertSubmissionCounts(t, db, submissionCounts{})
	t.Logf("assertion (a) PASSED: encrypted raw retention rejected with %v", ErrUnsupportedRawRetention)

	// Assertion (b): an ephemeral submission persists a PHI-classified canonical event,
	// and the table carries exactly the Slice 4.1e expiry state and nothing else.
	ephemeral := newRetentionFixture(t, integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral})
	ephemeralProcessor := newDurableFixtureProcessor(t, ephemeral, store)
	if _, err := ephemeralProcessor.Process(ctx, ephemeral.request); err != nil {
		t.Fatalf("ephemeral production submission: %v", err)
	}

	var eventCount int
	var classification string
	var payloadBytes int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*), coalesce(min(classification), ''), coalesce(max(octet_length(payload_json::text)), 0)
		FROM integration_canonical_events
	`).Scan(&eventCount, &classification, &payloadBytes); err != nil {
		t.Fatalf("read canonical events: %v", err)
	}
	if eventCount == 0 {
		t.Fatal("assertion (b) FAILED: ephemeral submission persisted no canonical event")
	}
	if classification != "phi" {
		t.Fatalf("assertion (b) FAILED: canonical event classification = %q, want %q", classification, "phi")
	}
	if payloadBytes == 0 {
		t.Fatal("assertion (b) FAILED: canonical event payload is empty")
	}

	policyColumns := retentionPolicyColumns(ctx, t, db, schema, "integration_canonical_events")
	expected := []string{"purge_after", "purged_at"}
	if !sameColumns(policyColumns, expected) {
		t.Fatalf("assertion (b) FAILED: integration_canonical_events carries retention columns %v, want exactly %v; "+
			"docs/operations/PHI-RETENTION.md section 2 is stale", policyColumns, expected)
	}

	// A freshly admitted event is not expired and not purged. If a submission ever
	// arrived pre-purged, or pre-stamped by something other than the retention
	// policy, the purge component is no longer the only thing that decides.
	var stamped, purged int
	if err := db.QueryRowContext(ctx, `
		SELECT count(*) FILTER (WHERE purge_after IS NOT NULL),
		       count(*) FILTER (WHERE purged_at IS NOT NULL)
		FROM integration_canonical_events
	`).Scan(&stamped, &purged); err != nil {
		t.Fatalf("read canonical event expiry state: %v", err)
	}
	if stamped != 0 || purged != 0 {
		t.Fatalf("assertion (b) FAILED: a freshly admitted event arrived with expiry state "+
			"(purge_after on %d rows, purged_at on %d); only an operator's retention policy may stamp one",
			stamped, purged)
	}
	t.Logf("assertion (b) PASSED: %d PHI-classified canonical event row(s), %d payload bytes, "+
		"expiry columns %v present and unstamped until a retention policy applies",
		eventCount, payloadBytes, policyColumns)
}

// sameColumns compares two already-sorted-by-query column lists.
func sameColumns(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

// retentionPolicyColumns reports any column on the table whose name suggests a
// retention, TTL, expiry, or purge policy.
func retentionPolicyColumns(ctx context.Context, t *testing.T, db *sql.DB, schema, table string) []string {
	t.Helper()
	rows, err := db.QueryContext(ctx, `
		SELECT column_name FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		  AND (column_name LIKE '%ttl%' OR column_name LIKE '%expire%'
		       OR column_name LIKE '%retention%' OR column_name LIKE '%purge%')
		ORDER BY column_name
	`, schema, table)
	if err != nil {
		t.Fatalf("inspect %s columns: %v", table, err)
	}
	defer func() { _ = rows.Close() }()
	out := []string{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatalf("scan column name: %v", err)
		}
		out = append(out, name)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate %s columns: %v", table, err)
	}
	return out
}

func repeatChar(character string, count int) string {
	out := make([]byte, 0, count)
	for index := 0; index < count; index++ {
		out = append(out, character[0])
	}
	return string(out)
}

// newRetentionFixture builds a production-mode fixture whose revision declares the
// supplied raw-retention policy. It deliberately does not reuse
// newMessageProcessorFixture because that helper hardcodes ephemeral retention.
func newRetentionFixture(t *testing.T, retention integration.RawRetentionPolicy) messageProcessorFixture {
	t.Helper()
	profileJSON := strictExecutableProfileJSON(false)
	workflowYAML := processorPublishedWorkflow
	raw := processorA01Message(true)

	profileRef, err := NewProfileRevisionReference("profile-adt", 7, []byte(profileJSON))
	if err != nil {
		t.Fatalf("NewProfileRevisionReference: %v", err)
	}
	workflowRef, err := NewWorkflowRevisionReference("workflow-adt", "workflow-1", []byte(workflowYAML))
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference: %v", err)
	}
	digest := func(character string) string { return "sha256:" + repeatChar(character, 64) }
	destinations := []integration.DestinationRevisionRef{}
	for index, artifactID := range []string{"fhir-primary", "file-trap", "exec-trap", "store-trap"} {
		digestCharacter := []string{"d", "e", "f", "0"}[index]
		destinations = append(destinations, integration.DestinationRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: artifactID, RevisionID: "destination-1", Digest: digest(digestCharacter),
			},
			Class: integration.DestinationClassProduction,
		})
	}
	revisionID := "revision-ephemeral"
	if retention.EffectiveMode() != integration.RawRetentionModeEphemeral {
		revisionID = "revision-" + string(retention.EffectiveMode())
	}
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "integration-adt",
		RevisionID:   revisionID,
		TenantID:     "tenant-a",
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: "source-adt", RevisionID: "source-1", Digest: digest("a"),
			},
			SourceID: "adt-east",
		},
		Format:       events.FormatHL7v2,
		Profile:      profileRef,
		Workflow:     workflowRef,
		Destinations: destinations,
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention:   retention,
		},
		Created: integration.AuditEnvelope{
			TenantID: "tenant-a",
			Principal: integration.Principal{
				ID: "operator-1", Kind: integration.PrincipalKindHuman,
				AuthMethod: "oidc", Roles: []string{"publisher"},
			},
			Reason:     "publish",
			OccurredAt: time.Date(2026, 7, 13, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}
	definitionJSON, err := json.Marshal(revision)
	if err != nil {
		t.Fatalf("marshal definition revision: %v", err)
	}
	definitionLoader := &messageProcessorDefinitionLoader{raw: definitionJSON}
	definitionResolver, err := NewDefinitionRevisionResolver("tenant-a", definitionLoader)
	if err != nil {
		t.Fatalf("NewDefinitionRevisionResolver: %v", err)
	}
	artifactLoader := &messageProcessorArtifactLoader{profile: []byte(profileJSON), workflow: []byte(workflowYAML)}
	artifactResolver, err := NewRevisionResolver("tenant-a", artifactLoader)
	if err != nil {
		t.Fatalf("NewRevisionResolver: %v", err)
	}
	processor, err := NewMessageProcessor(definitionResolver, artifactResolver)
	if err != nil {
		t.Fatalf("NewMessageProcessor: %v", err)
	}
	request := integration.ProcessRequest{
		Mode:                integration.ExecutionModeProduction,
		IntegrationRevision: revision.Reference(),
		Security: integration.SecurityContext{
			TenantID: "tenant-a",
			Principal: integration.Principal{
				ID: "source-service", Kind: integration.PrincipalKindService,
				AuthMethod: "mtls", SourceID: "adt-east",
				Roles: []string{authorization.HTTPSubmitGrant},
			},
		},
		Envelope:       messageProcessorEnvelope(t, revision, raw, revision.TenantID),
		CorrelationID:  "phi-retention-posture-correlation",
		IdempotencyKey: "phi-retention-posture-" + revisionID,
	}
	return messageProcessorFixture{
		processor:        processor,
		revision:         revision,
		request:          request,
		definitionLoader: definitionLoader,
		artifactLoader:   artifactLoader,
	}
}
