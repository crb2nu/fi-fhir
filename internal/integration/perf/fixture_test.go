//go:build integration

package perf

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/ingress"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/mllp"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/registry"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// The workflow is log-only on purpose. The reference profile requires
// "destinations decoupled from durable acceptance for latency measurements"
// (docs/operations/SUPPORTED-1.0.md), and a log action produces zero
// DeliveryResults, so the accept transaction writes no delivery_attempts and no
// outbox rows. The definition below still declares a destination — a revision
// with none is invalid — it is simply referenced by no action.
//
// This matters for attribution as much as for speed: each accepted message
// writes exactly one receipt, one canonical event and one lineage row, so the
// ledger count is an exact multiple of the iteration count.
const perfWorkflowYAML = `dsl_version: "1"
name: adt-perf
version: "1"
routes:
  - name: admit
    filter:
      event_type: patient_admit
    actions:
      - id: audit
        type: log
`

const perfProfileJSON = `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]},"identifiers":{"assigning_authorities":[{"code":"HOSP","system":"urn:oid:1.2.3"}],"normalization":{"ssn_strip_dashes":true,"phone_normalize":false}}}`

const (
	perfTenantID      = "tenant-a"
	perfIntegrationID = "adt-perf"
	perfDefinitionID  = "integration-adt-perf"
	perfSourceID      = "adt-perf-east"
)

// fixture is one fully wired durable accept path over a private schema.
type fixture struct {
	db       *sql.DB
	ingress  *ingress.Service
	mllp     *mllp.Service
	revision integration.IntegrationDefinitionRevision
}

// newFixture builds the durable accept path against a real PostgreSQL.
//
// It takes testing.TB rather than *testing.T so the benchmarks and the
// kill-test share exactly one setup path. Every helper in the repository's
// other integration packages is *testing.T-typed and therefore unusable from a
// Benchmark; that is the only reason this is not a copy of one of them.
func newFixture(tb testing.TB) *fixture {
	tb.Helper()

	ctx := context.Background()
	db := openPerfDB(tb)

	entry, revision := perfEntry(tb)
	reg, err := registry.NewStaticRegistry(perfTenantID, []registry.Entry{entry})
	if err != nil {
		tb.Fatalf("NewStaticRegistry: %v", err)
	}

	definitions, err := processor.NewDefinitionRevisionResolver(perfTenantID, reg)
	if err != nil {
		tb.Fatalf("NewDefinitionRevisionResolver: %v", err)
	}
	artifacts, err := processor.NewRevisionResolver(perfTenantID, reg)
	if err != nil {
		tb.Fatalf("NewRevisionResolver: %v", err)
	}

	clock := func() time.Time { return time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC) }
	store, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{Clock: clock})
	if err != nil {
		tb.Fatalf("NewPostgresSubmissionStore: %v", err)
	}
	if err := store.Migrate(ctx); err != nil {
		tb.Fatalf("Migrate: %v", err)
	}

	durable, err := processor.NewDurableMessageProcessor(definitions, artifacts, store)
	if err != nil {
		tb.Fatalf("NewDurableMessageProcessor: %v", err)
	}

	ingressService, err := ingress.NewService(ingress.ServiceConfig{
		TenantID:  perfTenantID,
		Registry:  reg,
		Processor: durable,
		Clock:     clock,
	})
	if err != nil {
		tb.Fatalf("ingress.NewService: %v", err)
	}

	mllpSource := perfMLLPSource(tb)
	mllpService, err := mllp.NewService(mllp.ServiceConfig{
		TenantID:     perfTenantID,
		DefinitionID: perfDefinitionID,
		PrincipalID:  "perf-mllp-principal",
		Source:       mllpSource,
		Resolver:     staticRunnableResolver{revision: revision, source: mllpSource},
		Processor:    durable,
		Clock:        clock,
	})
	if err != nil {
		tb.Fatalf("mllp.NewService: %v", err)
	}

	return &fixture{db: db, ingress: ingressService, mllp: mllpService, revision: revision}
}

// ledgerCount returns the number of rows in one durable ledger table.
//
// A benchmark that never reaches PostgreSQL still produces a plausible ns/op,
// so the durable benchmarks assert on this rather than trusting that they wired
// the right processor. The legacy engine writes no SQL at all, which is exactly
// what makes a non-zero receipt count proof of subject.
func (f *fixture) ledgerCount(tb testing.TB, table string) int {
	tb.Helper()

	var count int
	query := `SELECT COUNT(*) FROM ` + pq.QuoteIdentifier(table)
	if err := f.db.QueryRow(query).Scan(&count); err != nil {
		tb.Fatalf("counting %s: %v", table, err)
	}
	return count
}

// hl7Message renders a ~2 KiB ADT^A01, the size the reference profile names.
//
// The sequence number keeps each message distinct so the store's idempotency
// path does not turn iterations 2..N into cache hits that write no rows.
//
// The padding goes inside PID.11 (address) rather than into extra segments:
// the profile is strict, so an unknown segment is rejected with
// "invalid source message" and the benchmark would measure the reject path.
func hl7Message(sequence int) []byte {
	segments := []string{
		fmt.Sprintf(`MSH|^~\&|PERF-HARNESS|FAC|APP|FAC|20260809120000-0400||ADT^A01^ADT_A01|MSG%08d|P|2.5.1`, sequence),
		hl7Segment("EVN", 6, map[int]string{
			1: "A01", 2: "20260809120000", 6: "20260809115900-0400",
		}),
		hl7Segment("PID", 11, map[int]string{
			1: "1",
			3: fmt.Sprintf("MRN-%08d^^^HOSP^MR", sequence),
			5: "Patient^Test",
			7: "19800101",
			8: "F",
			// Pad to the profile's 2-KiB baseline inside a legal repeating field.
			11: strings.TrimSuffix(strings.Repeat("123 MAIN ST^APT 4B^SPRINGFIELD^IL^62701^USA~", 30), "~"),
		}),
		hl7Segment("PV1", 44, map[int]string{
			1: "1", 2: "I", 3: "UNIT^101^A^FAC",
			19: fmt.Sprintf("visit-%08d", sequence),
			44: "20260809120000",
		}),
	}
	return []byte(strings.Join(segments, "\r"))
}

// hl7Segment builds a fixed-arity segment, leaving unspecified fields empty.
func hl7Segment(id string, lastField int, values map[int]string) string {
	fields := make([]string, lastField+1)
	fields[0] = id
	for index, value := range values {
		fields[index] = value
	}
	return strings.Join(fields, "|")
}

func ingressInput(sequence int) ingress.Input {
	return ingress.Input{
		Security: integration.SecurityContext{
			TenantID: perfTenantID,
			Principal: integration.Principal{
				ID:         "perf-http-principal",
				Kind:       integration.PrincipalKindService,
				AuthMethod: "mtls",
				Roles:      []string{ingress.SubmitRole},
			},
		},
		IntegrationID:  perfIntegrationID,
		Payload:        hl7Message(sequence),
		IdempotencyKey: fmt.Sprintf("perf-%08d", sequence),
		CorrelationID:  fmt.Sprintf("corr-%08d", sequence),
	}
}

// staticRunnableResolver stands in for the deployment catalog.
//
// The MLLP service needs a RunnableBinding; building a real PostgresCatalog
// would add catalog writes to every iteration and measure the wrong thing. The
// capacity policy below is deliberately far above anything the benchmark can
// reach, because the rate gate is slice 4.4e's subject, not this harness's: a
// benchmark that trips it would be measuring rejection, and its allocation
// count would move when 4.4e replaces the in-memory bucket with a durable one.
type staticRunnableResolver struct {
	revision integration.IntegrationDefinitionRevision
	source   mllp.SourceRevision
}

func (r staticRunnableResolver) ResolveRunnable(_ context.Context, tenantID, definitionID string) (lifecycle.RunnableBinding, error) {
	if tenantID != perfTenantID || definitionID != perfDefinitionID {
		return lifecycle.RunnableBinding{}, fmt.Errorf("perf: unexpected runnable lookup %s/%s", tenantID, definitionID)
	}
	return lifecycle.RunnableBinding{
		ReleaseID:           "perf-release-1",
		SnapshotVersion:     1,
		Health:              integration.DeploymentHealthHealthy,
		IntegrationRevision: r.revision.Reference(),
		// The MLLP source is content-addressed and validates the binding
		// against its own Reference(), not against the definition's source ref.
		SourceRevision: r.source.Reference(),
		SourceID:       r.source.SourceID,
		Format:         r.revision.Format,
		Classification: r.revision.Policy.Classification,
		Deployment: integration.IntegrationDeploymentPolicy{
			ConnectionValidation: integration.ConnectionValidationPolicy{TimeoutSeconds: 10, MaxAgeSeconds: 3600},
			Schedule:             integration.SchedulePolicy{Mode: integration.ScheduleModeContinuous},
			Health: integration.HealthPolicy{
				StartupGraceSeconds:  0,
				CheckIntervalSeconds: 30,
				TimeoutSeconds:       5,
				FailureThreshold:     3,
			},
			Capacity: integration.CapacityPolicy{
				MaxInFlight:          10000,
				MaxQueued:            1000000,
				MaxMessagesPerSecond: 1000000,
			},
		},
	}, nil
}

func perfMLLPSource(tb testing.TB) mllp.SourceRevision {
	tb.Helper()

	source, err := mllp.NewSourceRevision(mllp.SourceRevisionInput{
		ArtifactID:      "source-adt-perf",
		RevisionID:      "source-1",
		SourceID:        perfSourceID,
		ListenAddress:   "127.0.0.1:2575",
		Encoding:        "utf-8",
		MaxMessageBytes: 1 << 20,
		MaxConnections:  64,
		Timeouts:        mllp.TimeoutPolicy{ReadSeconds: 30, WriteSeconds: 30, IdleSeconds: 60, ProcessSeconds: 30},
		Framing: mllp.FramingPolicy{
			StartByte:   mllp.StandardStartByte,
			EndByte:     mllp.StandardEndByte,
			TrailerByte: mllp.StandardTrailerByte,
		},
		TLS:              mllp.TLSPolicy{Mode: mllp.TLSModeDisabled},
		Clients:          mllp.ClientPolicy{AllowedCIDRs: []string{"127.0.0.0/8"}},
		Acknowledgements: mllp.AcknowledgementPolicy{Mode: mllp.AcknowledgementModeApplication, IncludeErrorSegment: true},
	})
	if err != nil {
		tb.Fatalf("mllp.NewSourceRevision: %v", err)
	}
	return source
}

func perfEntry(tb testing.TB) (registry.Entry, integration.IntegrationDefinitionRevision) {
	tb.Helper()

	profileRef, err := processor.NewProfileRevisionReference("profile-adt-perf", 1, []byte(perfProfileJSON))
	if err != nil {
		tb.Fatalf("NewProfileRevisionReference: %v", err)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference("workflow-adt-perf", "workflow-version-1", []byte(perfWorkflowYAML))
	if err != nil {
		tb.Fatalf("NewWorkflowRevisionReference: %v", err)
	}

	digest := func(c string) string { return "sha256:" + strings.Repeat(c, 64) }
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: perfDefinitionID,
		RevisionID:   "definition-revision-1",
		TenantID:     perfTenantID,
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: "source-adt-perf", RevisionID: "source-1", Digest: digest("a"),
			},
			SourceID: perfSourceID,
		},
		Format:   events.FormatHL7v2,
		Profile:  profileRef,
		Workflow: workflowRef,
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: "fhir-primary", RevisionID: "destination-1", Digest: digest("d"),
			},
			Class: integration.DestinationClassProduction,
		}},
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
		},
		Created: integration.AuditEnvelope{
			TenantID: perfTenantID,
			Principal: integration.Principal{
				ID: "publisher", Kind: integration.PrincipalKindHuman,
				AuthMethod: "oidc", Roles: []string{"publisher"},
			},
			Reason:     "publish perf harness fixture",
			OccurredAt: time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		tb.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}

	definitionJSON, err := json.Marshal(revision)
	if err != nil {
		tb.Fatalf("marshal definition: %v", err)
	}

	return registry.Entry{
		IntegrationID:  perfIntegrationID,
		DefinitionJSON: definitionJSON,
		ProfileJSON:    []byte(perfProfileJSON),
		WorkflowYAML:   []byte(perfWorkflowYAML),
	}, revision
}

// openPerfDB gives each run a private schema and drops it on cleanup.
func openPerfDB(tb testing.TB) *sql.DB {
	tb.Helper()

	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			tb.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		tb.Skip("POSTGRES_TEST_URL is required for the durable performance harness")
	}

	schema := fmt.Sprintf("perf_%d", time.Now().UnixNano())

	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		tb.Fatalf("open admin connection: %v", err)
	}
	defer func() { _ = admin.Close() }()
	if _, err := admin.Exec(`CREATE SCHEMA ` + pq.QuoteIdentifier(schema)); err != nil {
		tb.Fatalf("create schema %s: %v", schema, err)
	}

	db, err := sql.Open("postgres", schemaDSN(tb, dsn, schema))
	if err != nil {
		tb.Fatalf("open schema connection: %v", err)
	}
	if err := db.Ping(); err != nil {
		tb.Fatalf("ping: %v", err)
	}

	tb.Cleanup(func() {
		_ = db.Close()
		cleanup, err := sql.Open("postgres", dsn)
		if err != nil {
			return
		}
		defer func() { _ = cleanup.Close() }()
		_, _ = cleanup.Exec(`DROP SCHEMA ` + pq.QuoteIdentifier(schema) + ` CASCADE`)
	})

	return db
}

func schemaDSN(tb testing.TB, dsn, schema string) string {
	tb.Helper()

	parsed, err := url.Parse(dsn)
	if err != nil {
		tb.Fatalf("parse POSTGRES_TEST_URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}
