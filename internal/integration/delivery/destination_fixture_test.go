//go:build integration

package delivery

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// rawIngressSentinel marks the exact source bytes so every durable record and
// broker payload can be proven raw-free.
const rawIngressSentinel = "RAW-INGRESS-SENTINEL"

// durableSubmissionFixture builds the complete production admission path used by
// the Slice 4.1c-a proofs: server-owned definition and artifact resolvers over a
// PostgreSQL submission store, driven by one authenticated service principal.
type durableSubmissionFixture struct {
	revision  integration.IntegrationDefinitionRevision
	processor *processor.MessageProcessor
	store     *processor.PostgresSubmissionStore
	request   integration.ProcessRequest
}

// newDurableSubmissionFixture composes the durable processor for one workflow
// and its exact destination set. destinationIDs must match every `destination:`
// named by workflowYAML, otherwise planning fails closed (correction 16).
//
// Each destination gets a synthetic digest, which is all the Slice 4.1c-a proofs
// need. A proof that must resolve the attempt against a real deployed
// destination revision uses newDurableSubmissionFixtureWithDestinations instead,
// because a revision's digest is computed from its own content and cannot be
// chosen.
func newDurableSubmissionFixture(
	t *testing.T,
	db *sql.DB,
	clock func() time.Time,
	workflowYAML string,
	destinationIDs []string,
) durableSubmissionFixture {
	t.Helper()
	destinations := make([]integration.DestinationRevisionRef, 0, len(destinationIDs))
	for index, artifactID := range destinationIDs {
		destinations = append(destinations, integration.DestinationRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: artifactID,
				RevisionID: "destination-1",
				Digest:     destinationFixtureDigest(index),
			},
			Class: integration.DestinationClassProduction,
		})
	}
	return newDurableSubmissionFixtureWithDestinations(t, db, clock, workflowYAML, destinations)
}

// newDurableSubmissionFixtureWithDestinations composes the durable processor for
// one workflow over exact destination references.
//
// Slice 4.1c-b needs this shape: the deployed destination registry holds content
// addressed revisions whose digests are derived from their own bytes, and the
// registry refuses any attempt whose reference does not match one byte for byte
// (internal/integration/destination/registry.go Resolve). A durable attempt must
// therefore carry the real revision's reference, not a fabricated digest.
func newDurableSubmissionFixtureWithDestinations(
	t *testing.T,
	db *sql.DB,
	clock func() time.Time,
	workflowYAML string,
	destinations []integration.DestinationRevisionRef,
) durableSubmissionFixture {
	t.Helper()
	profileJSON := destinationFixtureProfileJSON()
	profileRef, err := processor.NewProfileRevisionReference("profile-adt", 7, []byte(profileJSON))
	if err != nil {
		t.Fatalf("NewProfileRevisionReference: %v", err)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference("workflow-adt", "workflow-1", []byte(workflowYAML))
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference: %v", err)
	}
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "integration-adt",
		RevisionID:   "revision-1",
		TenantID:     "tenant-a",
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: "source-adt", RevisionID: "source-1",
				Digest: "sha256:" + strings.Repeat("a", 64),
			},
			SourceID: "adt-east",
		},
		Format:       events.FormatHL7v2,
		Profile:      profileRef,
		Workflow:     workflowRef,
		Destinations: destinations,
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
		},
		Created: integration.AuditEnvelope{
			TenantID: "tenant-a",
			Principal: integration.Principal{
				ID: "operator-1", Kind: integration.PrincipalKindHuman,
				AuthMethod: "oidc", Roles: []string{"publisher"},
			},
			Reason:     "publish",
			OccurredAt: time.Date(2026, 8, 8, 10, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}
	definitionJSON, err := json.Marshal(revision)
	if err != nil {
		t.Fatalf("marshal definition revision: %v", err)
	}
	definitionResolver, err := processor.NewDefinitionRevisionResolver(
		"tenant-a",
		&destinationFixtureDefinitionLoader{raw: definitionJSON},
	)
	if err != nil {
		t.Fatalf("NewDefinitionRevisionResolver: %v", err)
	}
	artifactResolver, err := processor.NewRevisionResolver("tenant-a", &destinationFixtureArtifactLoader{
		profile:  []byte(profileJSON),
		workflow: []byte(workflowYAML),
	})
	if err != nil {
		t.Fatalf("NewRevisionResolver: %v", err)
	}
	store, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{Clock: clock})
	if err != nil {
		t.Fatalf("NewPostgresSubmissionStore: %v", err)
	}
	if err := store.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	durableProcessor, err := processor.NewDurableMessageProcessor(definitionResolver, artifactResolver, store)
	if err != nil {
		t.Fatalf("NewDurableMessageProcessor: %v", err)
	}
	envelope, err := integration.NewRawEnvelope(integration.RawEnvelopeMetadata{
		TenantID:       revision.TenantID,
		SourceID:       revision.Source.SourceID,
		Format:         revision.Format,
		ContentType:    "x-application/hl7-v2+er7",
		ReceivedAt:     clock(),
		Classification: integration.DataClassificationPHI,
	}, destinationFixtureA01Message())
	if err != nil {
		t.Fatalf("NewRawEnvelope: %v", err)
	}
	return durableSubmissionFixture{
		revision:  revision,
		processor: durableProcessor,
		store:     store,
		request: integration.ProcessRequest{
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
			Envelope: envelope,
			// Unique per run so the derived receipt and attempt identifiers are
			// unique, and per-attempt Kafka assertions stay exact on a broker that
			// retains records from earlier runs.
			IdempotencyKey: fmt.Sprintf("destination-proof-%d", time.Now().UnixNano()),
			CorrelationID:  "correlation-4-1c-a",
		},
	}
}

func destinationFixtureDigest(index int) string {
	characters := []string{"d", "e", "f", "0", "1", "2"}
	return "sha256:" + strings.Repeat(characters[index%len(characters)], 64)
}

func destinationFixtureProfileJSON() string {
	return `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","event_classifications":` +
		`[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]},` +
		`"identifiers":{"assigning_authorities":[{"code":"HOSP","system":"urn:oid:1.2.3"}],` +
		`"normalization":{"ssn_strip_dashes":true,"phone_normalize":false}}}`
}

func destinationFixtureA01Message() []byte {
	msh := `MSH|^~\&|` + rawIngressSentinel +
		`|FAC|APP|FAC|20260808120000-0400||ADT^A01^ADT_A01|control-4-1c|P|2.5.1`
	evn := destinationFixtureSegment("EVN", 6, map[int]string{
		1: "A01", 2: "20260808120000", 6: "20260808115900-0400",
	})
	pid := destinationFixtureSegment("PID", 8, map[int]string{
		1: "1", 3: "MRN-123^^^HOSP^MR", 5: "Patient^Test", 7: "19800101", 8: "F",
	})
	pv1 := destinationFixtureSegment("PV1", 44, map[int]string{
		1: "1", 2: "I", 3: "UNIT^101^A^FAC", 19: "visit-123", 44: "20260808120000",
	})
	return []byte(strings.Join([]string{msh, evn, pid, pv1}, "\r"))
}

func destinationFixtureSegment(id string, lastField int, values map[int]string) string {
	fields := make([]string, lastField+1)
	fields[0] = id
	for index, value := range values {
		fields[index] = value
	}
	return strings.Join(fields, "|")
}

type destinationFixtureDefinitionLoader struct {
	raw []byte
}

func (l *destinationFixtureDefinitionLoader) LoadDefinitionRevision(
	ctx context.Context, _, _, _ string,
) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), l.raw...), nil
}

type destinationFixtureArtifactLoader struct {
	profile  []byte
	workflow []byte
}

func (l *destinationFixtureArtifactLoader) LoadProfileRevision(ctx context.Context, _, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), l.profile...), nil
}

func (l *destinationFixtureArtifactLoader) LoadWorkflowRevision(ctx context.Context, _, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), l.workflow...), nil
}

// countingListener records every accepted TCP connection so a destination
// listener can prove it was never contacted rather than merely never logged.
type countingListener struct {
	net.Listener
	accepted atomic.Int64
}

func (l *countingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err == nil {
		l.accepted.Add(1)
	}
	return conn, err
}

// destinationListener is a live TLS endpoint standing at the address a webhook
// or FHIR destination would be reached on.
type destinationListener struct {
	server   *httptest.Server
	listener *countingListener
	requests atomic.Int64
}

// newDestinationListener starts a real TLS server on loopback. It counts
// accepted connections and served requests independently so a zero result
// cannot be an artifact of a broken handler.
func newDestinationListener(t *testing.T) *destinationListener {
	t.Helper()
	base, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for destination endpoint: %v", err)
	}
	endpoint := &destinationListener{listener: &countingListener{Listener: base}}
	endpoint.server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		endpoint.requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	_ = endpoint.server.Listener.Close()
	endpoint.server.Listener = endpoint.listener
	endpoint.server.StartTLS()
	t.Cleanup(endpoint.server.Close)
	return endpoint
}

func (l *destinationListener) URL() string { return l.server.URL }

func (l *destinationListener) HostPort() string { return strings.TrimPrefix(l.server.URL, "https://") }

func (l *destinationListener) Accepted() int64 { return l.listener.accepted.Load() }

func (l *destinationListener) Requests() int64 { return l.requests.Load() }

// proveReachable dials the endpoint from the test itself. It is the control on
// the counters: a "zero contacts" assertion is meaningless unless a real
// contact registers.
func (l *destinationListener) proveReachable(t *testing.T) {
	t.Helper()
	beforeAccepted, beforeRequests := l.Accepted(), l.Requests()
	response, err := l.server.Client().Get(l.server.URL + "/fhir/Encounter")
	if err != nil {
		t.Fatalf("destination endpoint is not reachable, so zero-contact proves nothing: %v", err)
	}
	_ = response.Body.Close()
	if l.Accepted() <= beforeAccepted || l.Requests() <= beforeRequests {
		t.Fatalf("destination endpoint counters did not register a real contact: accepted %d->%d requests %d->%d",
			beforeAccepted, l.Accepted(), beforeRequests, l.Requests())
	}
}

func submissionCount(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return count
}

// drainDeliveryRecords reads the delivery topic from the beginning and returns
// every record, stopping once the topic goes quiet. Assertions then filter by
// attempt key, so a broker retaining records from an earlier run cannot make a
// per-attempt count look right by accident.
func drainDeliveryRecords(
	t *testing.T,
	ctx context.Context,
	brokers []string,
	topic string,
) []*kgo.Record {
	t.Helper()
	consumer, err := kgo.NewClient(
		kgo.SeedBrokers(brokers...),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	if err != nil {
		t.Fatalf("create Kafka consumer: %v", err)
	}
	defer consumer.Close()
	records := make([]*kgo.Record, 0, 8)
	for {
		pollCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		fetches := consumer.PollFetches(pollCtx)
		cancel()
		if errs := fetches.Errors(); len(errs) > 0 {
			if errors.Is(errs[0].Err, context.DeadlineExceeded) {
				return records
			}
			t.Fatalf("consume Kafka records: %v", errs[0].Err)
		}
		empty := true
		fetches.EachRecord(func(record *kgo.Record) {
			records = append(records, record)
			empty = false
		})
		if empty {
			return records
		}
	}
}

func recordsByKey(records []*kgo.Record) map[string][]*kgo.Record {
	byKey := make(map[string][]*kgo.Record, len(records))
	for _, record := range records {
		byKey[string(record.Key)] = append(byKey[string(record.Key)], record)
	}
	return byKey
}

func fixedDestinationClock() func() time.Time {
	instant := time.Date(2026, 8, 8, 12, 0, 0, 0, time.UTC)
	return func() time.Time { return instant }
}

func destinationWorkflowYAML(actionType, destinationID string) string {
	return fmt.Sprintf(`dsl_version: "1"
name: adt-durable
version: "1"
routes:
  - name: matched
    filter:
      event_type: patient_admit
      source: adt-east
    actions:
      - id: send-%s
        type: %s
        destination: %s
`, actionType, actionType, destinationID)
}

// destinationWorkflowYAMLFor plans one delivery action per destination on a
// single matched route, so one production submission seeds one durable attempt
// per destination in the deployed set.
func destinationWorkflowYAMLFor(actionType string, destinationIDs ...string) string {
	var actions strings.Builder
	for index, destinationID := range destinationIDs {
		fmt.Fprintf(&actions, "      - id: send-%s-%d\n        type: %s\n        destination: %s\n",
			actionType, index, actionType, destinationID)
	}
	return `dsl_version: "1"
name: adt-durable
version: "1"
routes:
  - name: matched
    filter:
      event_type: patient_admit
      source: adt-east
    actions:
` + actions.String()
}
