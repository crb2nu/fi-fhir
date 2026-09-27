//go:build integration

package connection

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/mllp"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// The .loom/38 Lane C-2 proofs (`make connection-capture`,
// ci/test-connection-capture.yml). The capture proofs run a real MLLP listener
// over a lifecycle-deployed integration and the durable PostgreSQL processor —
// the MLLP runtime harness's shape — with the capture tap wrapped around the
// processor exactly as serve wraps it. The peek proof runs the batch
// ingestion harness's shape over MinIO. Every proof skips without
// POSTGRES_TEST_URL (and the peek without BATCH_S3_*), and fails instead in CI.

const (
	proofTenant = "tenant-a"
	proofSource = "adt-east"

	proofProfileJSON = `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","event_classifications":[{"message_type":"ADT^A01","condition":"PV1.2 == 'I'","event_type":"inpatient_admit","priority":1},{"message_type":"ADT^A01","event_type":"patient_admit","priority":2}]},"identifiers":{"assigning_authorities":[{"code":"HOSP","system":"urn:oid:1.2.3"}],"normalization":{"ssn_strip_dashes":true,"phone_normalize":false}}}`

	proofWorkflowYAML = `dsl_version: "1"
name: capture-proof
version: "1"
routes:
  - name: matched
    filter:
      event_type: patient_admit
    actions:
      - id: send-fhir
        type: fhir
        destination: fhir-primary
`
)

// proofIdentifiers are the synthetic identifiers the proofs' messages carry —
// the patient's MRN, name, birth date, address, phone, account, and SSN, and
// (in proofKinADT) the next of kin's and the insured's names. None may survive
// into a stored sample.
var proofIdentifiers = []string{
	"MRN-", "Zyxwpat", "Quorbina", "19800101", "Zyxwton", "555-0199", "ACCT-", "000-00-0000",
	"Zyxwkin", "Morvath", "Zyxwins", "Tallowby",
}

// proofPID carries a synthetic value in every PID field the v1 kernel admits
// and the capture redactor masks, and in PID-18 and PID-19.
func proofPID(controlID string) string {
	return "PID|1||MRN-" + controlID + "^^^HOSP^MR||Zyxwpat^Quorbina||19800101|F|||1 Quorbina Way^^Zyxwton^ST^00000||555-0199|||||ACCT-" + controlID + "|000-00-0000"
}

// proofADT is one ADT^A01 the v1 production kernel admits: MSH, EVN, PID, PV1.
func proofADT(controlID string) []byte {
	segments := []string{
		"MSH|^~\\&|SENDER|FAC|FI-FHIR|FAC|20260926120000-0400||ADT^A01^ADT_A01|" + controlID + "|P|2.5.1",
		"EVN|A01|20260926120000||||20260926115900-0400",
		proofPID(controlID),
		"PV1|1|I|UNIT^101^A^FAC||||||||||||||||visit-" + controlID + "|||||||||||||||||||||||||20260926120000",
	}
	return []byte(strings.Join(segments, "\r") + "\r")
}

// proofKinADT adds an NK1 and an IN1 with a synthetic next of kin (NK1-2) and
// insured (IN1-16). Real feeds carry both; the v1 kernel refuses any segment
// outside MSH/EVN/PID/PV1 (hl7v2 strict validation, and the unknown-segments
// tolerance is unsupported in profile_compile.go), so a frame like this is
// never durably accepted and a stream capture never sees one. A peek reads it
// straight from the object, which is where these two fields are proved.
func proofKinADT(controlID string) []byte {
	segments := []string{
		"MSH|^~\\&|SENDER|FAC|FI-FHIR|FAC|20260926120000-0400||ADT^A01^ADT_A01|" + controlID + "|P|2.5.1",
		"EVN|A01|20260926120000||||20260926115900-0400",
		proofPID(controlID),
		"NK1|1|Zyxwkin^Morvath|SPO",
		"PV1|1|I|UNIT^101^A^FAC||||||||||||||||visit-" + controlID + "|||||||||||||||||||||||||20260926120000",
		"IN1|1|PLAN-A|PAYER-1|Synthetic Payer" + strings.Repeat("|", 12) + "Zyxwins^Tallowby",
	}
	return []byte(strings.Join(segments, "\r") + "\r")
}

func assertNoProofIdentifier(t *testing.T, where, text string) {
	t.Helper()
	for _, identifier := range proofIdentifiers {
		if strings.Contains(text, identifier) {
			t.Fatalf("%s kept the synthetic identifier %q", where, identifier)
		}
	}
}

// meteredObserver is cmd/fi-fhir's captureObserver over a real registry, plus
// a recorder the proofs read directly.
func meteredObserver(recording *recordingObserver, metrics *observability.Metrics) CaptureObserver {
	recorded := recording.observer()
	return CaptureObserver{
		Captured: func(mode CaptureMode, messages int) {
			recorded.Captured(mode, messages)
			metrics.RecordConnectionCaptureMessages(string(mode), messages)
		},
		TapFailed: func(reason TapFailure, err error) {
			recorded.TapFailed(reason, err)
			metrics.RecordConnectionCaptureTapError(string(reason))
		},
	}
}

// failureCount is every failure the tap reported, read under the recorder's
// lock: the tap runs on the listener's goroutines.
func (o *recordingObserver) failureCount() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	total := 0
	for _, count := range o.failures {
		total += count
	}
	return total
}

// counterValue reads one labelled series of a counter; an absent series is 0.
func counterValue(t *testing.T, metrics *observability.Metrics, name, label, value string) float64 {
	t.Helper()
	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			for _, pair := range metric.GetLabel() {
				if pair.GetName() == label && pair.GetValue() == value {
					return metric.GetCounter().GetValue()
				}
			}
		}
	}
	return 0
}

// proofArtifacts is the exact profile and workflow both proofs' definitions
// reference, and the resolver that serves their bytes.
func proofArtifacts(t *testing.T) (integration.ArtifactRevisionRef, integration.ArtifactRevisionRef, *processor.RevisionResolver) {
	t.Helper()
	profile, err := processor.NewProfileRevisionReference("profile-adt", 1, []byte(proofProfileJSON))
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := processor.NewWorkflowRevisionReference("workflow-adt", "workflow-1", []byte(proofWorkflowYAML))
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := processor.NewRevisionResolver(proofTenant, proofArtifactLoader{})
	if err != nil {
		t.Fatal(err)
	}
	return profile, workflow, resolver
}

type proofArtifactLoader struct{}

func (proofArtifactLoader) LoadProfileRevision(context.Context, string, string) ([]byte, error) {
	return []byte(proofProfileJSON), nil
}

func (proofArtifactLoader) LoadWorkflowRevision(context.Context, string, string) ([]byte, error) {
	return []byte(proofWorkflowYAML), nil
}

// passingValidation is the connection validator the lifecycle proofs use.
func passingValidation(context.Context, integration.IntegrationDefinitionRevision) (lifecycle.ConnectionValidationOutcome, error) {
	return lifecycle.ConnectionValidationOutcome{Passed: true, Codes: []string{"SOURCE_REACHABLE", "AUTH_OK"}}, nil
}

// deployProofRevision walks a definition revision to deployed.
func deployProofRevision(t *testing.T, catalog *lifecycle.PostgresCatalog, revision integration.IntegrationDefinitionRevision) {
	t.Helper()
	ctx := t.Context()
	snapshot, err := catalog.CreateDraft(ctx, revision)
	if err != nil {
		t.Fatalf("create definition draft: %v", err)
	}
	for _, step := range []struct {
		name string
		run  func(context.Context, lifecycle.Command) (lifecycle.Snapshot, error)
	}{
		{"validate", catalog.ValidateConnection}, {"approve", catalog.Approve},
		{"publish", catalog.Publish}, {"deploy", catalog.Deploy},
	} {
		snapshot, err = step.run(ctx, lifecycle.Command{
			TenantID: revision.TenantID, DefinitionID: revision.DefinitionID, RevisionID: revision.RevisionID,
			ExpectedVersion: snapshot.Version,
			Principal: integration.Principal{
				ID: "operator", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{"integration:operator"},
			},
			Reason: step.name + " the capture proof's integration",
		})
		if err != nil {
			t.Fatalf("%s definition: %v", step.name, err)
		}
	}
	if snapshot.State != integration.DeploymentStateDeployed {
		t.Fatalf("definition state = %s, want deployed", snapshot.State)
	}
}

func proofDeployment() *integration.IntegrationDeploymentPolicy {
	return &integration.IntegrationDeploymentPolicy{
		ConnectionValidation: integration.ConnectionValidationPolicy{TimeoutSeconds: 5, MaxAgeSeconds: 300},
		Schedule:             integration.SchedulePolicy{Mode: integration.ScheduleModeContinuous},
		Health:               integration.HealthPolicy{StartupGraceSeconds: 1, CheckIntervalSeconds: 30, TimeoutSeconds: 5, FailureThreshold: 3},
		Capacity:             integration.CapacityPolicy{MaxInFlight: 2, MaxQueued: 8, MaxMessagesPerSecond: 100},
	}
}

func proofAudit(now time.Time) integration.AuditEnvelope {
	return integration.AuditEnvelope{
		TenantID: proofTenant,
		Principal: integration.Principal{
			ID: "engineer", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{"integration:engineer"},
		},
		Reason: "connection capture proof fixture", OccurredAt: now,
	}
}

// mllpCaptureHarness is one migrated schema, a deployed MLLP integration for
// adt-east, the durable processor wrapped in a capture tap, and a listener.
type mllpCaptureHarness struct {
	db        *sql.DB
	address   string
	sessions  *session.PostgresStore
	store     *PostgresStore
	service   *Service
	registry  *CaptureRegistry
	recorded  *recordingObserver
	metrics   *observability.Metrics
	admission *admissionErrors
}

// admissionErrors keeps the durable processor's own errors, beneath the tap,
// so an ACK that is not AA explains itself.
type admissionErrors struct {
	inner Processor
	mu    sync.Mutex
	last  error
}

func (a *admissionErrors) Process(ctx context.Context, request integration.ProcessRequest) (integration.ProcessResult, error) {
	result, err := a.inner.Process(ctx, request)
	if err != nil {
		a.mu.Lock()
		a.last = err
		a.mu.Unlock()
	}
	return result, err
}

func (a *admissionErrors) lastError() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	chain := []string{}
	for err := a.last; err != nil; err = errors.Unwrap(err) {
		chain = append(chain, err.Error())
	}
	return strings.Join(chain, ": ")
}

// newMLLPCaptureHarness builds the harness. With closedTapSessions the taps
// write through a session store whose database handle is closed, while the
// service — which checks the session is active when a capture is armed —
// keeps the live one.
func newMLLPCaptureHarness(t *testing.T, closedTapSessions bool) *mllpCaptureHarness {
	t.Helper()
	ctx := t.Context()
	dsn := newConnectionSchema(t, requireConnectionDSN(t))
	db := openConnectionDB(t, dsn)

	lifecycleCatalog, err := lifecycle.NewPostgresCatalog(db, lifecycle.Config{ValidateConnection: passingValidation})
	if err != nil || lifecycleCatalog.Migrate(ctx) != nil {
		t.Fatalf("lifecycle catalog: %v", err)
	}
	submissions, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{
		Authorize: lifecycleCatalog.AuthorizeRunnableSubmission,
	})
	if err != nil || submissions.Migrate(ctx) != nil {
		t.Fatalf("submission store: %v", err)
	}
	sessions, err := session.NewPostgresStore(db, session.PostgresConfig{TenantID: proofTenant})
	if err != nil || sessions.Migrate(ctx) != nil {
		t.Fatalf("session store: %v", err)
	}
	store, err := NewPostgresStore(db, nil)
	if err != nil || store.Migrate(ctx) != nil {
		t.Fatalf("connection store: %v", err)
	}

	source, err := mllp.NewSourceRevision(mllp.SourceRevisionInput{
		ArtifactID: "adt-mllp", RevisionID: "1", SourceID: proofSource,
		ListenAddress: "127.0.0.1:2575", Encoding: "utf-8",
		Framing: mllp.FramingPolicy{
			StartByte: mllp.StandardStartByte, EndByte: mllp.StandardEndByte, TrailerByte: mllp.StandardTrailerByte,
		},
		Timeouts:         mllp.TimeoutPolicy{ReadSeconds: 5, WriteSeconds: 5, IdleSeconds: 10, ProcessSeconds: 10},
		TLS:              mllp.TLSPolicy{Mode: mllp.TLSModeDisabled},
		Clients:          mllp.ClientPolicy{AllowedCIDRs: []string{"127.0.0.0/8"}},
		Acknowledgements: mllp.AcknowledgementPolicy{Mode: mllp.AcknowledgementModeApplication, IncludeErrorSegment: true},
		MaxMessageBytes:  64 << 10, MaxConnections: 4,
	})
	if err != nil {
		t.Fatalf("MLLP source revision: %v", err)
	}
	profile, workflow, artifacts := proofArtifacts(t)
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "integration-adt-east", RevisionID: "definition-v1", TenantID: proofTenant,
		Source: integration.SourceRevisionRef{ArtifactRevisionRef: source.Reference(), SourceID: source.SourceID},
		Format: events.FormatHL7v2, Profile: profile, Workflow: workflow,
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: "fhir-primary", RevisionID: "destination-v1", Digest: "sha256:" + strings.Repeat("d", 64),
			},
			Class: integration.DestinationClassProduction,
		}},
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
		},
		Deployment: proofDeployment(),
		Created:    proofAudit(time.Now().UTC()),
	})
	if err != nil {
		t.Fatalf("definition revision: %v", err)
	}
	deployProofRevision(t, lifecycleCatalog, revision)

	definitions, err := processor.NewDefinitionRevisionResolver(proofTenant, lifecycleCatalog)
	if err != nil {
		t.Fatal(err)
	}
	durable, err := processor.NewDurableMessageProcessor(definitions, artifacts, submissions)
	if err != nil {
		t.Fatal(err)
	}

	harness := &mllpCaptureHarness{
		db: db, sessions: sessions, store: store, recorded: newRecordingObserver(),
		metrics: observability.NewMetrics("capture-proof"), admission: &admissionErrors{inner: durable},
	}
	observer := meteredObserver(harness.recorded, harness.metrics)
	harness.service, err = NewService(store, nil, nil, proofTenant)
	if err != nil {
		t.Fatal(err)
	}
	harness.registry, err = NewCaptureRegistry(CaptureRegistryConfig{Store: store, TenantID: proofTenant, Observer: observer})
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.service.EnableSampleIntake(IntakeConfig{Sessions: sessions, Registry: harness.registry, Observer: observer}); err != nil {
		t.Fatal(err)
	}
	var tapSessions SessionSink = sessions
	if closedTapSessions {
		closed, err := sql.Open("postgres", dsn)
		if err != nil {
			t.Fatal(err)
		}
		_ = closed.Close()
		tapSessions, err = session.NewPostgresStore(closed, session.PostgresConfig{TenantID: proofTenant})
		if err != nil {
			t.Fatal(err)
		}
	}
	taps := NewCaptureTaps()
	if err := taps.Bind(CaptureTapBinding{Registry: harness.registry, Sessions: tapSessions, Observer: observer}); err != nil {
		t.Fatal(err)
	}

	server, err := mllp.NewServer(mllp.ServerConfig{Service: mllp.ServiceConfig{
		TenantID: proofTenant, DefinitionID: revision.DefinitionID, PrincipalID: "mllp-listener",
		Source: source, Resolver: lifecycleCatalog, Processor: taps.Wrap(harness.admission),
	}})
	if err != nil {
		t.Fatalf("MLLP listener: %v", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	serveCtx, stop := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- server.Serve(serveCtx, listener) }()
	t.Cleanup(func() {
		stop()
		<-served
	})
	harness.address = listener.Addr().String()
	return harness
}

func (h *mllpCaptureHarness) newSession(t *testing.T) *session.Session {
	t.Helper()
	workspace, err := h.sessions.CreateSession(t.Context(), session.CreateSessionRequest{Name: "capture proof"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return workspace
}

func (h *mllpCaptureHarness) arm(t *testing.T, sourceID, sessionID string, maxMessages, ttlSeconds int) Capture {
	t.Helper()
	capture, err := h.service.StartCapture(callerContext(proofTenant, ReadRole), StartCaptureRequest{
		SourceID: sourceID, SessionID: sessionID, MaxMessages: maxMessages, TTLSeconds: ttlSeconds,
		Reason: "shape the east ADT profile from live traffic",
	})
	if err != nil {
		t.Fatalf("StartCapture: %v", err)
	}
	return capture
}

func (h *mllpCaptureHarness) captureRow(t *testing.T, sessionID string) Capture {
	t.Helper()
	captures, err := h.service.ListCaptures(callerContext(proofTenant, ReadRole), sessionID)
	if err != nil || len(captures) != 1 {
		t.Fatalf("ListCaptures = %d rows, %v; want 1", len(captures), err)
	}
	return captures[0]
}

func (h *mllpCaptureHarness) samples(t *testing.T, sessionID string) []session.Sample {
	t.Helper()
	samples, err := h.sessions.ListSamples(t.Context(), sessionID)
	if err != nil {
		t.Fatalf("ListSamples: %v", err)
	}
	return samples
}

func (h *mllpCaptureHarness) acceptedReceipts(t *testing.T) (int, int) {
	t.Helper()
	var total, accepted int
	if err := h.db.QueryRowContext(t.Context(),
		`SELECT count(*), count(*) FILTER (WHERE status = 'accepted') FROM integration_receipts`,
	).Scan(&total, &accepted); err != nil {
		t.Fatalf("count receipts: %v", err)
	}
	return total, accepted
}

// proofACK is one acknowledgement: MSA-1 and ERR-3's first component.
type proofACK struct {
	code      string
	errorCode string
}

// sendFrames sends one frame per control ID, in order, on one connection and
// returns each frame's ACK.
func (h *mllpCaptureHarness) sendFrames(t *testing.T, controlIDs ...string) []proofACK {
	t.Helper()
	connection, err := net.DialTimeout("tcp", h.address, 5*time.Second)
	if err != nil {
		t.Fatalf("dial MLLP: %v", err)
	}
	defer func() { _ = connection.Close() }()
	reader := bufio.NewReader(connection)
	acks := make([]proofACK, 0, len(controlIDs))
	for _, controlID := range controlIDs {
		framed := append(append([]byte{mllp.StandardStartByte}, proofADT(controlID)...), mllp.StandardEndByte, mllp.StandardTrailerByte)
		if _, err := connection.Write(framed); err != nil {
			t.Fatalf("write frame %s: %v", controlID, err)
		}
		_ = connection.SetReadDeadline(time.Now().Add(15 * time.Second))
		if start, err := reader.ReadByte(); err != nil || start != mllp.StandardStartByte {
			t.Fatalf("frame %s: ACK start byte %x, %v", controlID, start, err)
		}
		body, err := reader.ReadBytes(mllp.StandardEndByte)
		if err != nil {
			t.Fatalf("frame %s: read ACK: %v", controlID, err)
		}
		if trailer, err := reader.ReadByte(); err != nil || trailer != mllp.StandardTrailerByte {
			t.Fatalf("frame %s: ACK trailer %x, %v", controlID, trailer, err)
		}
		acks = append(acks, parseProofACK(t, string(body[:len(body)-1])))
	}
	return acks
}

func parseProofACK(t *testing.T, payload string) proofACK {
	t.Helper()
	var ack proofACK
	for _, segment := range strings.Split(payload, "\r") {
		fields := strings.Split(segment, "|")
		switch {
		case fields[0] == "MSA" && len(fields) > 1:
			ack.code = fields[1]
		case fields[0] == "ERR" && len(fields) > 3:
			ack.errorCode = strings.Split(fields[3], "^")[0]
		}
	}
	if ack.code == "" {
		t.Fatalf("ACK without MSA-1: %q", payload)
	}
	return ack
}

func (h *mllpCaptureHarness) assertAllAccepted(t *testing.T, label string, acks []proofACK) {
	t.Helper()
	for index, ack := range acks {
		if ack.code != "AA" || ack.errorCode != "" {
			t.Fatalf("%s frame %d ACK = %+v, want AA (admission error: %s)", label, index+1, ack, h.admission.lastError())
		}
	}
}

// TestConnectionCapture_TapCapturesRedactedSamplesWithoutTouchingAdmission is
// the kill-test: armed for adt-east at maxMessages 2, three frames carrying a
// synthetic patient (PID-3, -5, -7, -11, -13, -18, -19) produce exactly two
// session samples, each exactly the capture redactor's output of its frame
// with none of the identifiers, while all three frames are durably admitted
// and ACKed exactly as the same frames were before the capture was armed.
// NK1-2 and IN1-16 are proved by the peek: the v1 kernel admits no NK1 or IN1
// (proofKinADT), so no frame carrying one is ever captured.
func TestConnectionCapture_TapCapturesRedactedSamplesWithoutTouchingAdmission(t *testing.T) {
	h := newMLLPCaptureHarness(t, false)
	workspace := h.newSession(t)

	unarmed := h.sendFrames(t, "unarmed-1", "unarmed-2", "unarmed-3")
	h.assertAllAccepted(t, "unarmed", unarmed)
	totalBefore, acceptedBefore := h.acceptedReceipts(t)
	if totalBefore != 3 || acceptedBefore != 3 {
		t.Fatalf("unarmed receipts = %d (%d accepted), want 3", totalBefore, acceptedBefore)
	}

	capture := h.arm(t, proofSource, workspace.ID, 2, 0)
	armed := h.sendFrames(t, "armed-1", "armed-2", "armed-3")
	for index := range armed {
		if armed[index] != unarmed[index] {
			t.Fatalf("frame %d ACK = %+v while armed, %+v unarmed: the tap changed an ACK", index+1, armed[index], unarmed[index])
		}
	}
	total, accepted := h.acceptedReceipts(t)
	if total != 6 || accepted != 6 {
		t.Fatalf("receipts = %d (%d accepted), want all 3 armed frames durable beside the 3 unarmed", total, accepted)
	}

	samples := h.samples(t, workspace.ID)
	if len(samples) != 2 {
		t.Fatalf("session holds %d samples, want maxMessages 2", len(samples))
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i].Name < samples[j].Name })
	for index, sample := range samples {
		number := index + 1
		want := session.RedactCapturedHL7v2(string(proofADT(fmt.Sprintf("armed-%d", number))))
		if sample.Raw != want {
			t.Fatalf("sample %d is not the capture redactor's output of frame armed-%d:\n got %q\nwant %q", number, number, sample.Raw, want)
		}
		assertNoProofIdentifier(t, fmt.Sprintf("sample %d", number), sample.Raw)
		if sample.Name != fmt.Sprintf("capture %s #%d", capture.ID, number) || sample.Source != "capture:"+capture.ID ||
			sample.Redaction != session.SampleRedactionCapture || sample.PHIPolicy != session.PHIPolicyRedact || !sample.PHIRedacted {
			t.Fatalf("sample %d = name %q source %q redaction %q policy %q", number, sample.Name, sample.Source, sample.Redaction, sample.PHIPolicy)
		}
		if strings.Contains(sample.Raw, "armed-3") {
			t.Fatal("the third frame, past maxMessages, was captured")
		}
	}
	var leaked int
	if err := h.db.QueryRowContext(t.Context(),
		`SELECT count(*) FROM integration_session_samples WHERE record_json::text ~ 'Zyxw|Quorbina|Morvath|Tallowby'`,
	).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("stored sample records carrying a synthetic identifier = %d, %v", leaked, err)
	}

	row := h.captureRow(t, workspace.ID)
	if row.Status != CaptureStatusComplete || row.Captured != 2 || row.CompletedAt == nil || row.Mode != CaptureModeStream ||
		row.SourceID != proofSource || len(row.Problems) != 0 {
		t.Fatalf("capture row = %+v, want complete with 2 captured", row)
	}
	if got := counterValue(t, h.metrics, "fi_fhir_connection_capture_messages_total", "mode", "stream"); got != 2 {
		t.Fatalf("fi_fhir_connection_capture_messages_total{mode=stream} = %v, want 2", got)
	}
	if failures := h.recorded.failureCount(); failures != 0 {
		t.Fatalf("tap failures = %d, want none", failures)
	}
}

// TestConnectionCapture_OtherSourceCapturesNothing is the negative control: a
// capture armed for another source id captures nothing while the same frames
// are admitted and ACKed.
func TestConnectionCapture_OtherSourceCapturesNothing(t *testing.T) {
	h := newMLLPCaptureHarness(t, false)
	workspace := h.newSession(t)
	h.arm(t, "adt-west", workspace.ID, 2, 0)

	acks := h.sendFrames(t, "other-1", "other-2", "other-3")
	h.assertAllAccepted(t, "adt-east while adt-west is armed", acks)
	if total, accepted := h.acceptedReceipts(t); total != 3 || accepted != 3 {
		t.Fatalf("receipts = %d (%d accepted), want 3", total, accepted)
	}
	if samples := h.samples(t, workspace.ID); len(samples) != 0 {
		t.Fatalf("a capture armed for adt-west captured %d samples of adt-east frames", len(samples))
	}
	if row := h.captureRow(t, workspace.ID); row.Status != CaptureStatusArmed || row.Captured != 0 {
		t.Fatalf("adt-west capture row = %+v, want still armed with nothing captured", row)
	}
	if got := counterValue(t, h.metrics, "fi_fhir_connection_capture_messages_total", "mode", "stream"); got != 0 {
		t.Fatalf("fi_fhir_connection_capture_messages_total{mode=stream} = %v, want 0", got)
	}
}

// TestConnectionCapture_TapFailureNeverChangesTheAck: with the tap's session
// store closed, the armed frames are admitted and ACKed exactly as unarmed
// ones, the tap error counter increments, and the capture finishes failed
// with nothing counted.
func TestConnectionCapture_TapFailureNeverChangesTheAck(t *testing.T) {
	h := newMLLPCaptureHarness(t, true)
	workspace := h.newSession(t)

	unarmed := h.sendFrames(t, "unarmed-1", "unarmed-2", "unarmed-3")
	h.assertAllAccepted(t, "unarmed", unarmed)
	h.arm(t, proofSource, workspace.ID, 2, 0)
	failing := h.sendFrames(t, "failing-1", "failing-2", "failing-3")
	for index := range failing {
		if failing[index] != unarmed[index] {
			t.Fatalf("frame %d ACK = %+v with the tap failing, %+v unarmed", index+1, failing[index], unarmed[index])
		}
	}
	if total, accepted := h.acceptedReceipts(t); total != 6 || accepted != 6 {
		t.Fatalf("receipts = %d (%d accepted), want 6: a tap failure must not cost a receipt", total, accepted)
	}
	if got := counterValue(t, h.metrics, "fi_fhir_connection_capture_tap_errors_total", "reason", string(TapFailureSessionStore)); got != 1 {
		t.Fatalf("fi_fhir_connection_capture_tap_errors_total{reason=session_store} = %v, want 1", got)
	}
	row := h.captureRow(t, workspace.ID)
	if row.Status != CaptureStatusFailed || row.Captured != 0 || len(row.Problems) != 1 || row.Problems[0].Code != CodeSampleWriteFailed {
		t.Fatalf("capture row = %+v, want failed with SAMPLE_WRITE_FAILED and nothing counted", row)
	}
	if samples := h.samples(t, workspace.ID); len(samples) != 0 {
		t.Fatalf("the session holds %d samples written through a closed store", len(samples))
	}
}

// TestConnectionCapture_ExpiresByTTL: a capture with a one-second TTL is inert
// once it expires — before any refresh, because the tap checks the TTL from
// memory — and the next refresh advances its row to expired.
func TestConnectionCapture_ExpiresByTTL(t *testing.T) {
	h := newMLLPCaptureHarness(t, false)
	workspace := h.newSession(t)
	capture := h.arm(t, proofSource, workspace.ID, 5, 1)
	time.Sleep(time.Until(capture.ExpiresAt) + 200*time.Millisecond)

	h.assertAllAccepted(t, "past the TTL", h.sendFrames(t, "late-1"))
	if row := h.captureRow(t, workspace.ID); row.Status != CaptureStatusArmed {
		t.Fatalf("before a refresh the row is %s; the refresh is what advances it", row.Status)
	}
	if err := h.registry.Refresh(t.Context()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	row := h.captureRow(t, workspace.ID)
	if row.Status != CaptureStatusExpired || row.CompletedAt == nil || row.Captured != 0 {
		t.Fatalf("after the refresh the row is %+v, want expired", row)
	}
	h.assertAllAccepted(t, "after expiry", h.sendFrames(t, "late-2"))
	if samples := h.samples(t, workspace.ID); len(samples) != 0 {
		t.Fatalf("an expired capture captured %d samples", len(samples))
	}
	if total, accepted := h.acceptedReceipts(t); total != 2 || accepted != 2 {
		t.Fatalf("receipts = %d (%d accepted), want 2", total, accepted)
	}
}

// requireProofMinIO reads the batch ingestion harness's MinIO settings.
func requireProofMinIO(t *testing.T) (string, string, string) {
	t.Helper()
	endpoint, access, secret := os.Getenv("BATCH_S3_ENDPOINT"), os.Getenv("BATCH_S3_ACCESS_KEY"), os.Getenv("BATCH_S3_SECRET_KEY")
	if endpoint == "" || access == "" || secret == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("BATCH_S3_ENDPOINT, BATCH_S3_ACCESS_KEY, and BATCH_S3_SECRET_KEY are required in CI")
		}
		t.Skip("BATCH_S3_ENDPOINT, BATCH_S3_ACCESS_KEY, and BATCH_S3_SECRET_KEY are required for the peek proof")
	}
	return endpoint, access, secret
}

// envSecrets is the env half of serve's env/file resolver.
type envSecrets struct{}

func (envSecrets) Resolve(_ context.Context, reference integration.SecretReference) ([]byte, error) {
	if reference.Provider != integration.SecretProviderEnvironment || reference.Version != "" {
		return nil, integration.ErrSecretUnresolvable
	}
	value := os.Getenv(reference.Key)
	if value == "" {
		return nil, integration.ErrSecretUnresolvable
	}
	return []byte(value), nil
}

// tableSnapshot is every row of a table as JSON, in a stable order.
func tableSnapshot(t *testing.T, db *sql.DB, table string) string {
	t.Helper()
	var snapshot sql.NullString
	if err := db.QueryRowContext(t.Context(),
		`SELECT string_agg(row_to_json(t)::text, E'\n' ORDER BY row_to_json(t)::text) FROM `+table+` t`,
	).Scan(&snapshot); err != nil {
		t.Fatalf("snapshot %s: %v", table, err)
	}
	return snapshot.String
}

// bucketSnapshot is every version and delete marker of every key.
func bucketSnapshot(t *testing.T, client *minio.Client, bucket string) []string {
	t.Helper()
	var entries []string
	for info := range client.ListObjects(t.Context(), bucket, minio.ListObjectsOptions{Recursive: true, WithVersions: true}) {
		if info.Err != nil {
			t.Fatalf("list bucket: %v", info.Err)
		}
		entries = append(entries, fmt.Sprintf("%s|%s|%s|%d|%v", info.Key, info.VersionID, info.ETag, info.Size, info.IsDeleteMarker))
	}
	sort.Strings(entries)
	return entries
}

// proofBatch concatenates messages the way a batch file carries them.
func proofBatch(build func(string) []byte, prefix string, messages int) []byte {
	parts := make([][]byte, 0, messages)
	for index := 1; index <= messages; index++ {
		parts = append(parts, proofMessage(build, fmt.Sprintf("%s-%d", prefix, index)))
	}
	return bytes.Join(parts, []byte("\r"))
}

// proofMessage is one message as the batch reader hands it on: segments
// joined by CR, with no trailing separator.
func proofMessage(build func(string) []byte, controlID string) []byte {
	return bytes.TrimSuffix(build(controlID), []byte("\r"))
}

// TestConnectionPeek_ReadsWithoutLeaseCheckpointOrArchive: a peek of a
// compiled batch_s3 connection lists the waiting objects and reads the first
// messages of one into the session, capture-redacted — including an object
// whose NK1-2 and IN1-16 carry synthetic names — while the batch checkpoint and
// audit tables stay byte-identical, the bucket gains and loses nothing (no
// archive object for a peeked one), and no receipt is written; afterwards the
// real batch runner processes both objects exactly as it would have. An
// unresolvable binding is a SECRET_UNRESOLVABLE problem on a failed audit row.
func TestConnectionPeek_ReadsWithoutLeaseCheckpointOrArchive(t *testing.T) {
	endpoint, accessKey, secretKey := requireProofMinIO(t)
	ctx := t.Context()
	dsn := newConnectionSchema(t, requireConnectionDSN(t))
	db := openConnectionDB(t, dsn)

	now := time.Now().UTC().Add(-time.Minute)
	lifecycleCatalog, err := lifecycle.NewPostgresCatalog(db, lifecycle.Config{
		Clock: func() time.Time { return now }, ValidateConnection: passingValidation,
	})
	if err != nil || lifecycleCatalog.Migrate(ctx) != nil {
		t.Fatalf("lifecycle catalog: %v", err)
	}
	submissions, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{
		Authorize: lifecycleCatalog.AuthorizeRunnableSubmission,
	})
	if err != nil || submissions.Migrate(ctx) != nil {
		t.Fatalf("submission store: %v", err)
	}
	checkpoints, err := batch.NewPostgresStore(db, nil)
	if err != nil || checkpoints.Migrate(ctx) != nil {
		t.Fatalf("batch checkpoint store: %v", err)
	}
	sessions, err := session.NewPostgresStore(db, session.PostgresConfig{TenantID: proofTenant})
	if err != nil || sessions.Migrate(ctx) != nil {
		t.Fatalf("session store: %v", err)
	}
	store, err := NewPostgresStore(db, nil)
	if err != nil || store.Migrate(ctx) != nil {
		t.Fatalf("connection store: %v", err)
	}

	client, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, "")})
	if err != nil {
		t.Fatal(err)
	}
	bucket := fmt.Sprintf("c2-peek-%d", time.Now().UnixNano())
	if err := client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatalf("make bucket: %v", err)
	}
	if err := client.SetBucketVersioning(ctx, bucket, minio.BucketVersioningConfiguration{Status: "Enabled"}); err != nil {
		t.Fatalf("enable versioning: %v", err)
	}
	t.Cleanup(func() {
		cleanup := context.Background()
		for info := range client.ListObjects(cleanup, bucket, minio.ListObjectsOptions{Recursive: true, WithVersions: true}) {
			_ = client.RemoveObject(cleanup, bucket, info.Key, minio.RemoveObjectOptions{VersionID: info.VersionID})
		}
		_ = client.RemoveBucket(cleanup, bucket)
	})
	t.Setenv("FI_FHIR_C2_PROOF_S3_ACCESS_KEY", accessKey)
	t.Setenv("FI_FHIR_C2_PROOF_S3_SECRET_KEY", secretKey)

	// The catalog connection: declared, compiled, and the compiled bytes are
	// what both the peek and the runner read.
	service, err := NewService(store, nil, nil, proofTenant)
	if err != nil {
		t.Fatal(err)
	}
	recorded := newRecordingObserver()
	metrics := observability.NewMetrics("peek-proof")
	if err := service.EnableSampleIntake(IntakeConfig{
		Sessions: sessions, Secrets: envSecrets{}, Observer: meteredObserver(recorded, metrics),
	}); err != nil {
		t.Fatal(err)
	}
	spec, err := json.Marshal(map[string]any{
		"source_id": proofSource,
		"s3": map[string]any{
			"endpoint": endpoint, "bucket": bucket, "input_prefix": "incoming", "archive_prefix": "archive",
			"use_tls": false, "access_key_binding": "peek-s3-access", "secret_access_key_binding": "peek-s3-secret",
		},
		"poll_seconds": 1, "lease_seconds": 60, "process_seconds": 30, "max_files_per_poll": 10, "max_message_bytes": 1 << 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	writer := callerContext(proofTenant, ReadRole, WriteRole)
	created, err := service.Create(writer, CreateRequest{
		ID: "adt-drop", Direction: DirectionSource, Kind: KindBatchS3, Name: "ADT east drop", Spec: spec,
		SecretBindings: []integration.SecretBinding{
			{Name: "peek-s3-access", Reference: integration.SecretReference{Provider: integration.SecretProviderEnvironment, Key: "FI_FHIR_C2_PROOF_S3_ACCESS_KEY"}},
			{Name: "peek-s3-secret", Reference: integration.SecretReference{Provider: integration.SecretProviderEnvironment, Key: "FI_FHIR_C2_PROOF_S3_SECRET_KEY"}},
		},
		Reason: "declare the east drop",
	})
	if err != nil {
		t.Fatalf("create batch connection: %v", err)
	}
	compiled, err := service.Compile(writer, CommandRequest{ID: created.ID, ExpectedVersion: created.Version, Reason: "compile the east drop"})
	if err != nil || compiled.Revision == nil {
		t.Fatalf("compile = %+v, %v", compiled.Problems, err)
	}
	source, err := batch.DecodeSourceRevision(bytes.NewReader(compiled.Revision.Document))
	if err != nil {
		t.Fatalf("decode compiled revision: %v", err)
	}

	// The integration the runner ingests for, over the same revision.
	profile, workflow, artifacts := proofArtifacts(t)
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "batch-adt-east", RevisionID: "v1", TenantID: proofTenant,
		Source: integration.SourceRevisionRef{ArtifactRevisionRef: source.Reference(), SourceID: source.SourceID},
		Format: events.FormatHL7v2, Profile: profile, Workflow: workflow,
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: "fhir-primary", RevisionID: "destination-1", Digest: "sha256:" + strings.Repeat("d", 64),
			},
			Class: integration.DestinationClassProduction,
		}},
		SecretBindings: []integration.SecretBinding{
			{Name: "peek-s3-access", Reference: integration.SecretReference{Provider: integration.SecretProviderFile, Key: "batch/peek-s3-access"}},
			{Name: "peek-s3-secret", Reference: integration.SecretReference{Provider: integration.SecretProviderFile, Key: "batch/peek-s3-secret"}},
		},
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
		},
		Deployment: proofDeployment(),
		Created:    proofAudit(now),
	})
	if err != nil {
		t.Fatalf("definition revision: %v", err)
	}
	deployProofRevision(t, lifecycleCatalog, revision)
	definitions, err := processor.NewDefinitionRevisionResolver(proofTenant, lifecycleCatalog)
	if err != nil {
		t.Fatal(err)
	}
	durable, err := processor.NewDurableMessageProcessor(definitions, artifacts, submissions)
	if err != nil {
		t.Fatal(err)
	}
	provider, err := batch.NewS3Provider(source, batch.S3Secrets{AccessKeyID: accessKey, SecretAccessKey: secretKey})
	if err != nil {
		t.Fatalf("S3 provider: %v", err)
	}
	runner, err := batch.NewRunner(batch.RunnerConfig{
		TenantID: proofTenant, DefinitionID: revision.DefinitionID, PrincipalID: "batch-adt-east-principal",
		WorkerID: "batch-adt-east-worker", Source: source, Resolver: lifecycleCatalog,
		Processor: durable, Store: checkpoints, Provider: provider,
	})
	if err != nil {
		t.Fatalf("batch runner: %v", err)
	}

	// A first object the runner ingests, archives, and deletes, so the tables
	// and the archive prefix the peek must not touch are not empty.
	put := func(key string, content []byte) {
		t.Helper()
		if _, err := client.PutObject(ctx, bucket, key, bytes.NewReader(content), int64(len(content)), minio.PutObjectOptions{}); err != nil {
			t.Fatalf("put %s: %v", key, err)
		}
	}
	put("incoming/first.hl7", proofBatch(proofADT, "first", 2))
	if processed, err := runner.PollOnce(ctx); err != nil || processed != 1 {
		t.Fatalf("first poll = %d, %v", processed, err)
	}
	// peek.hl7 is what the v1 kernel ingests; kin.hl7 carries the NK1 and IN1
	// a real feed carries, which only a peek can show an engineer.
	peekObject := proofBatch(proofADT, "peek", 3)
	kinObject := proofBatch(proofKinADT, "kin", 1)
	put("incoming/peek.hl7", peekObject)
	put("incoming/kin.hl7", kinObject)

	objectsBefore := tableSnapshot(t, db, "integration_batch_objects")
	auditBefore := tableSnapshot(t, db, "integration_batch_audit")
	bucketBefore := bucketSnapshot(t, client, bucket)
	receiptsBefore := countRows(t, db, "integration_receipts")
	if objectsBefore == "" || auditBefore == "" || receiptsBefore != 2 {
		t.Fatalf("the comparison would be vacuous: objects %q, audit %q, receipts %d", objectsBefore, auditBefore, receiptsBefore)
	}

	workspace, err := sessions.CreateSession(ctx, session.CreateSessionRequest{Name: "peek proof"})
	if err != nil {
		t.Fatal(err)
	}
	operator := callerContext(proofTenant, ReadRole)

	listed, err := service.PeekBatch(operator, PeekRequest{ConnectionID: "adt-drop", SessionID: workspace.ID, Reason: "see what is waiting"})
	if err != nil || len(listed.Problems) != 0 {
		t.Fatalf("list-only peek = %+v, %v", listed.Problems, err)
	}
	sizes := map[string]int64{}
	for _, object := range listed.Objects {
		if !strings.HasPrefix(object.Version, "version:") {
			t.Fatalf("listed object %s has version %q", object.Path, object.Version)
		}
		sizes[object.Path] = object.Size
	}
	if len(listed.Objects) != 2 || sizes["incoming/peek.hl7"] != int64(len(peekObject)) ||
		sizes["incoming/kin.hl7"] != int64(len(kinObject)) || len(listed.Samples) != 0 {
		t.Fatalf("list-only peek objects = %+v, samples %d", listed.Objects, len(listed.Samples))
	}
	if listed.Capture.Mode != CaptureModePeek || listed.Capture.Status != CaptureStatusComplete || listed.Capture.Captured != 0 ||
		listed.Capture.ConnectionArtifactID != "adt-drop" || listed.Capture.ConnectionDigest != compiled.Revision.Digest {
		t.Fatalf("list-only peek audit row = %+v", listed.Capture)
	}

	assertPeeked := func(result PeekResult, object string, build func(string) []byte, prefix string, want int) {
		t.Helper()
		if len(result.Problems) != 0 || len(result.Samples) != want {
			t.Fatalf("peek of %s = problems %+v, %d samples; want %d", object, result.Problems, len(result.Samples), want)
		}
		for index, sample := range result.Samples {
			number := index + 1
			expected := session.RedactCapturedHL7v2(string(proofMessage(build, fmt.Sprintf("%s-%d", prefix, number))))
			if sample.Raw != expected {
				t.Fatalf("%s sample %d is not the capture redactor's output of its message:\n got %q\nwant %q", object, number, sample.Raw, expected)
			}
			assertNoProofIdentifier(t, fmt.Sprintf("%s sample %d", object, number), sample.Raw)
			wantSource := fmt.Sprintf("peek:adt-drop@%s:%s#%d", compiled.Revision.Digest, object, number)
			if sample.Source != wantSource || sample.Redaction != session.SampleRedactionCapture {
				t.Fatalf("%s sample %d source %q redaction %q", object, number, sample.Source, sample.Redaction)
			}
		}
		if result.Capture.Status != CaptureStatusComplete || result.Capture.Captured != want {
			t.Fatalf("peek of %s audit row = %+v", object, result.Capture)
		}
	}
	peeked, err := service.PeekBatch(operator, PeekRequest{
		ConnectionID: "adt-drop", SessionID: workspace.ID, ObjectPath: "incoming/peek.hl7", MaxMessages: 2,
		Reason: "shape the profile from the waiting file",
	})
	if err != nil {
		t.Fatalf("peek: %v", err)
	}
	assertPeeked(peeked, "incoming/peek.hl7", proofADT, "peek", 2)
	kin, err := service.PeekBatch(operator, PeekRequest{
		ConnectionID: "adt-drop", SessionID: workspace.ID, ObjectPath: "incoming/kin.hl7",
		Reason: "see the next of kin and insurance segments",
	})
	if err != nil {
		t.Fatalf("peek kin: %v", err)
	}
	assertPeeked(kin, "incoming/kin.hl7", proofKinADT, "kin", 1)
	for _, segment := range strings.Split(kin.Samples[0].Raw, "\n") {
		fields := strings.Split(segment, "|")
		switch {
		case fields[0] == "NK1" && fields[2] != "REDACTED":
			t.Fatalf("NK1-2 = %q, want REDACTED", fields[2])
		case fields[0] == "IN1" && fields[16] != "REDACTED":
			t.Fatalf("IN1-16 = %q, want REDACTED", fields[16])
		}
	}

	t.Setenv("FI_FHIR_C2_PROOF_S3_SECRET_KEY", "")
	refused, err := service.PeekBatch(operator, PeekRequest{
		ConnectionID: "adt-drop", SessionID: workspace.ID, ObjectPath: "incoming/peek.hl7", Reason: "try without the secret",
	})
	if err != nil || len(refused.Problems) != 1 || refused.Problems[0].Code != CodeSecretUnresolvable ||
		refused.Problems[0].Path != "s3.secret_access_key_binding" || refused.Capture.Status != CaptureStatusFailed ||
		len(refused.Samples) != 0 || len(refused.Objects) != 0 {
		t.Fatalf("peek with an unresolvable binding = %+v, %v", refused, err)
	}
	t.Setenv("FI_FHIR_C2_PROOF_S3_SECRET_KEY", secretKey)

	if after := tableSnapshot(t, db, "integration_batch_objects"); after != objectsBefore {
		t.Fatalf("a peek changed integration_batch_objects:\nbefore %s\nafter  %s", objectsBefore, after)
	}
	if after := tableSnapshot(t, db, "integration_batch_audit"); after != auditBefore {
		t.Fatalf("a peek changed integration_batch_audit:\nbefore %s\nafter  %s", auditBefore, after)
	}
	bucketAfterPeek := bucketSnapshot(t, client, bucket)
	if strings.Join(bucketAfterPeek, "\n") != strings.Join(bucketBefore, "\n") {
		t.Fatalf("a peek changed the bucket:\nbefore %v\nafter  %v", bucketBefore, bucketAfterPeek)
	}
	for _, entry := range bucketAfterPeek {
		if strings.HasPrefix(entry, "archive/") && strings.Contains(entry, "/peek.hl7|") {
			t.Fatalf("a peek archived the object: %s", entry)
		}
	}
	if receipts := countRows(t, db, "integration_receipts"); receipts != receiptsBefore {
		t.Fatalf("a peek admitted %d messages", receipts-receiptsBefore)
	}
	if got := counterValue(t, metrics, "fi_fhir_connection_capture_messages_total", "mode", "peek"); got != 3 {
		t.Fatalf("fi_fhir_connection_capture_messages_total{mode=peek} = %v, want 3", got)
	}

	// Afterwards the real runner finds both objects unclaimed and treats them
	// exactly as it would have without the peeks: it ingests and archives
	// peek.hl7, and quarantines kin.hl7 as INVALID_MESSAGE, because the v1
	// kernel admits no NK1 or IN1.
	if processed, err := runner.PollOnce(ctx); err != nil || processed != 2 {
		t.Fatalf("poll after the peeks = %d, %v", processed, err)
	}
	if receipts := countRows(t, db, "integration_receipts"); receipts != receiptsBefore+3 {
		t.Fatalf("receipts after the runner = %d, want %d", receipts, receiptsBefore+3)
	}
	archived := false
	for _, entry := range bucketSnapshot(t, client, bucket) {
		archived = archived || (strings.HasPrefix(entry, "archive/") && strings.Contains(entry, "/peek.hl7|"))
	}
	if !archived {
		t.Fatal("the runner did not archive the peeked object")
	}
	remaining, err := provider.List(ctx, 10)
	if err != nil || len(remaining) != 1 || remaining[0].Path != "incoming/kin.hl7" {
		t.Fatalf("objects left under the input prefix = %+v, %v; want only the quarantined kin.hl7", remaining, err)
	}
	var quarantined int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM integration_batch_objects WHERE phase = 'failed' AND last_error_code = 'INVALID_MESSAGE'`,
	).Scan(&quarantined); err != nil || quarantined != 1 {
		t.Fatalf("quarantined objects = %d, %v; want kin.hl7", quarantined, err)
	}
}
