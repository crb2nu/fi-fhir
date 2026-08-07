package processor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestMessageProcessorPreviewIsDeterministicExactAndSideEffectFree(t *testing.T) {
	t.Parallel()

	trapPath := filepath.Join(t.TempDir(), "must-not-exist")
	workflowYAML := strings.Replace(
		processorPublishedWorkflow,
		"      - id: send-fhir\n        type: fhir\n        destination: fhir-primary",
		"      - id: send-fhir\n        type: fhir\n        destination: fhir-primary\n      - id: trap-file\n        type: file\n        destination: file-trap\n      - id: trap-exec\n        type: exec\n        destination: exec-trap\n      - id: trap-store\n        type: event_store\n        destination: store-trap",
		1,
	)
	fixture := newMessageProcessorFixture(t, strictExecutableProfileJSON(false), workflowYAML, processorA01Message(true))

	first, err := fixture.processor.Process(context.Background(), fixture.request)
	if err != nil {
		t.Fatalf("Process(first): %v", err)
	}
	second, err := fixture.processor.Process(context.Background(), fixture.request)
	if err != nil {
		t.Fatalf("Process(second): %v", err)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Fatalf("preview is nondeterministic:\nfirst:  %s\nsecond: %s", firstJSON, secondJSON)
	}
	if err := first.ValidatePreviewFor(fixture.request, fixture.revision); err != nil {
		t.Fatalf("strict preview contract: %v", err)
	}
	if first.ArtifactRevisions == nil || first.ArtifactRevisions.Source != fixture.revision.Source.ArtifactRevisionRef || first.ArtifactRevisions.Profile != fixture.revision.Profile || first.ArtifactRevisions.Workflow != fixture.revision.Workflow {
		t.Fatalf("exact artifact provenance missing: %#v", first.ArtifactRevisions)
	}
	if first.Receipt != nil || first.Correlations.ReceiptID != "" || len(first.Correlations.DeliveryAttemptIDs) != 0 {
		t.Fatalf("preview contains durable/attempt state: %#v", first)
	}
	if len(first.Events) != 1 || len(first.Routes) != 3 || len(first.Deliveries) != 4 {
		t.Fatalf("unexpected preview shape: events=%d routes=%d deliveries=%d", len(first.Events), len(first.Routes), len(first.Deliveries))
	}
	for _, delivery := range first.Deliveries {
		if delivery.Status != integration.DeliveryStatusSuppressed || delivery.AttemptID != "" || delivery.AttemptCount != 0 {
			t.Fatalf("delivery was not suppressed: %#v", delivery)
		}
	}
	for _, forbidden := range []string{"RAW-ONLY-SENTINEL", "raw_payload", "event.???", "must-not-exist"} {
		if bytes.Contains(firstJSON, []byte(forbidden)) {
			t.Fatalf("preview leaked source/workflow content %q: %s", forbidden, firstJSON)
		}
	}
	if _, err := os.Stat(trapPath); !os.IsNotExist(err) {
		t.Fatalf("preview created a side-effect trap: %v", err)
	}
}

func TestMessageProcessorRejectsProductionAndWrongTenantBeforeLoading(t *testing.T) {
	t.Parallel()

	fixture := newMessageProcessorFixture(t, strictExecutableProfileJSON(false), processorPublishedWorkflow, processorA01Message(true))
	production := fixture.request
	production.Mode = integration.ExecutionModeProduction
	if _, err := fixture.processor.Process(context.Background(), production); !errors.Is(err, ErrProductionCommitterRequired) {
		t.Fatalf("production error = %v", err)
	}
	if fixture.definitionLoader.callCount() != 0 || fixture.artifactLoader.callCount() != 0 {
		t.Fatalf("production mode reached loaders: definition=%d artifact=%d", fixture.definitionLoader.callCount(), fixture.artifactLoader.callCount())
	}

	wrongTenant := fixture.request
	wrongTenant.Security.TenantID = "tenant-b"
	wrongTenant.Envelope = messageProcessorEnvelope(t, fixture.revision, processorA01Message(true), "tenant-b")
	if _, err := fixture.processor.Process(context.Background(), wrongTenant); !errors.Is(err, ErrTenantMismatch) {
		t.Fatalf("wrong tenant error = %v", err)
	}
	if fixture.definitionLoader.callCount() != 0 || fixture.artifactLoader.callCount() != 0 {
		t.Fatalf("wrong tenant reached loaders: definition=%d artifact=%d", fixture.definitionLoader.callCount(), fixture.artifactLoader.callCount())
	}
}

func TestMessageProcessorRejectsProductionWithoutSubmitGrantBeforeArtifactsOrDurability(t *testing.T) {
	t.Parallel()

	fixture := newMessageProcessorFixture(t, strictExecutableProfileJSON(false), processorPublishedWorkflow, processorA01Message(true))
	durable, err := NewDurableMessageProcessor(
		fixture.processor.definitions,
		fixture.processor.artifacts,
		&PostgresSubmissionStore{},
	)
	if err != nil {
		t.Fatalf("NewDurableMessageProcessor: %v", err)
	}
	request := fixture.request
	request.Mode = integration.ExecutionModeProduction
	request.Security.Principal.Roles = []string{"integration:read"}

	if _, err := durable.Process(context.Background(), request); !errors.Is(err, ErrProcessForbidden) {
		t.Fatalf("Process() error = %v, want ErrProcessForbidden", err)
	}
	if fixture.definitionLoader.callCount() != 1 {
		t.Fatalf("definition loader calls = %d, want 1 exact resolution", fixture.definitionLoader.callCount())
	}
	if fixture.artifactLoader.callCount() != 0 {
		t.Fatalf("forbidden request reached artifact loader %d times", fixture.artifactLoader.callCount())
	}

	request.Security.Principal.Roles = []string{authorization.HTTPSubmitGrant}
	request.Security.Principal.SourceID = "adt-west"
	if _, err := durable.Process(context.Background(), request); !errors.Is(err, ErrInvalidProcessRequest) {
		t.Fatalf("source-spoof error = %v, want ErrInvalidProcessRequest", err)
	}
	if fixture.artifactLoader.callCount() != 0 {
		t.Fatalf("source-spoof request reached artifact loader %d times", fixture.artifactLoader.callCount())
	}
}

func TestDurableSubmissionErrorKeepsCatalogSafeMessageAndCause(t *testing.T) {
	t.Parallel()

	cause := errors.New("database detail must stay private")
	err := &durableSubmissionError{cause: cause}
	if err.Error() != ErrDurableSubmissionFailed.Error() {
		t.Fatalf("error text = %q", err.Error())
	}
	if !errors.Is(err, ErrDurableSubmissionFailed) || !errors.Is(err, cause) {
		t.Fatalf("error chain does not preserve catalog kind and cause: %v", err)
	}
	if durableSubmissionCause(err) != cause {
		t.Fatalf("durable submission cause was not recoverable")
	}
}

func TestMessageProcessorRejectsOversizedSourceBeforeLoading(t *testing.T) {
	t.Parallel()

	raw := bytes.Repeat([]byte{'X'}, int(MaxPreviewSourceBytes)+1)
	fixture := newMessageProcessorFixture(t, strictExecutableProfileJSON(false), processorPublishedWorkflow, raw)
	fixture.request.Envelope.SizeBytes = 1 // exported metadata cannot bypass the private payload-length cap
	if _, err := fixture.processor.Process(context.Background(), fixture.request); !errors.Is(err, ErrInvalidProcessRequest) {
		t.Fatalf("oversized source error = %v", err)
	}
	if fixture.definitionLoader.callCount() != 0 || fixture.artifactLoader.callCount() != 0 {
		t.Fatalf("oversized source reached loaders: definition=%d artifact=%d", fixture.definitionLoader.callCount(), fixture.artifactLoader.callCount())
	}
}

func TestMessageProcessorUsesExactMissingPV1Tolerance(t *testing.T) {
	t.Parallel()

	raw := processorA01Message(false)
	strict := newMessageProcessorFixture(t, strictExecutableProfileJSON(false), processorPublishedWorkflow, raw)
	if _, err := strict.processor.Process(context.Background(), strict.request); !errors.Is(err, ErrInvalidSourceMessage) {
		t.Fatalf("strict missing-PV1 error = %v", err)
	}

	tolerant := newMessageProcessorFixture(t, strictExecutableProfileJSON(true), processorPublishedWorkflow, raw)
	result, err := tolerant.processor.Process(context.Background(), tolerant.request)
	if err != nil {
		t.Fatalf("tolerant missing-PV1 Process: %v", err)
	}
	found := false
	for _, diagnostic := range result.Diagnostics {
		if diagnostic.Code == "MISSING_PV1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("tolerant preview lacks MISSING_PV1 diagnostic: %#v", result.Diagnostics)
	}
}

func TestMessageProcessorMapsParserAndCatalogErrorsWithoutRawPHI(t *testing.T) {
	t.Parallel()

	invalid := []byte("RAW-PHI-SENTINEL")
	fixture := newMessageProcessorFixture(t, strictExecutableProfileJSON(false), processorPublishedWorkflow, invalid)
	_, err := fixture.processor.Process(context.Background(), fixture.request)
	if !errors.Is(err, ErrInvalidSourceMessage) {
		t.Fatalf("invalid source error = %v", err)
	}
	if strings.Contains(err.Error(), "RAW-PHI-SENTINEL") {
		t.Fatalf("processor error leaked source PHI: %v", err)
	}

	unavailable := newMessageProcessorFixture(t, strictExecutableProfileJSON(false), processorPublishedWorkflow, processorA01Message(true))
	unavailable.artifactLoader.err = errors.New("database error for RAW-PHI-SENTINEL")
	_, err = unavailable.processor.Process(context.Background(), unavailable.request)
	if !errors.Is(err, ErrArtifactResolutionFailed) || strings.Contains(err.Error(), "RAW-PHI-SENTINEL") {
		t.Fatalf("artifact error was not catalog-safe: %v", err)
	}
}

func TestMessageProcessorIsConcurrentAndRaceSafe(t *testing.T) {
	fixture := newMessageProcessorFixture(t, strictExecutableProfileJSON(false), processorPublishedWorkflow, processorA01Message(true))
	want, err := fixture.processor.Process(context.Background(), fixture.request)
	if err != nil {
		t.Fatalf("baseline Process: %v", err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal baseline: %v", err)
	}

	const workers = 16
	errorsCh := make(chan error, workers)
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			result, err := fixture.processor.Process(context.Background(), fixture.request)
			if err != nil {
				errorsCh <- err
				return
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				errorsCh <- err
				return
			}
			if !bytes.Equal(encoded, wantJSON) {
				errorsCh <- errors.New("concurrent result differs")
			}
		}()
	}
	wait.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Errorf("concurrent Process: %v", err)
	}
}

type messageProcessorFixture struct {
	processor        *MessageProcessor
	revision         integration.IntegrationDefinitionRevision
	request          integration.ProcessRequest
	definitionLoader *messageProcessorDefinitionLoader
	artifactLoader   *messageProcessorArtifactLoader
}

func newMessageProcessorFixture(t *testing.T, profileJSON, workflowYAML string, raw []byte) messageProcessorFixture {
	t.Helper()
	profileRef, err := NewProfileRevisionReference("profile-adt", 7, []byte(profileJSON))
	if err != nil {
		t.Fatalf("NewProfileRevisionReference: %v", err)
	}
	workflowRef, err := NewWorkflowRevisionReference("workflow-adt", "workflow-1", []byte(workflowYAML))
	if err != nil {
		t.Fatalf("NewWorkflowRevisionReference: %v", err)
	}
	digest := func(character string) string { return "sha256:" + strings.Repeat(character, 64) }
	destinations := []integration.DestinationRevisionRef{}
	for index, artifactID := range []string{"fhir-primary", "file-trap", "exec-trap", "store-trap"} {
		digestCharacter := []string{"d", "e", "f", "0"}[index]
		destinations = append(destinations, integration.DestinationRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: artifactID, RevisionID: "destination-1", Digest: digest(digestCharacter)},
			Class:               integration.DestinationClassProduction,
		})
	}
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "integration-adt",
		RevisionID:   "revision-1",
		TenantID:     "tenant-a",
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "source-adt", RevisionID: "source-1", Digest: digest("a")},
			SourceID:            "adt-east",
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
			TenantID:   "tenant-a",
			Principal:  integration.Principal{ID: "operator-1", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{"publisher"}},
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
		Mode:                integration.ExecutionModePreview,
		IntegrationRevision: revision.Reference(),
		Security: integration.SecurityContext{
			TenantID:  "tenant-a",
			Principal: integration.Principal{ID: "source-service", Kind: integration.PrincipalKindService, AuthMethod: "mtls", SourceID: "adt-east"},
		},
		Envelope:      messageProcessorEnvelope(t, revision, raw, revision.TenantID),
		CorrelationID: "correlation-123",
	}
	return messageProcessorFixture{
		processor:        processor,
		revision:         revision,
		request:          request,
		definitionLoader: definitionLoader,
		artifactLoader:   artifactLoader,
	}
}

type messageProcessorDefinitionLoader struct {
	mu    sync.Mutex
	raw   []byte
	err   error
	calls int
}

func (l *messageProcessorDefinitionLoader) LoadDefinitionRevision(ctx context.Context, _, _, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	if l.err != nil {
		return nil, l.err
	}
	return append([]byte(nil), l.raw...), nil
}

func (l *messageProcessorDefinitionLoader) callCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

type messageProcessorArtifactLoader struct {
	mu       sync.Mutex
	profile  []byte
	workflow []byte
	err      error
	calls    int
}

func (l *messageProcessorArtifactLoader) LoadProfileRevision(ctx context.Context, _, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	if l.err != nil {
		return nil, l.err
	}
	return append([]byte(nil), l.profile...), nil
}

func (l *messageProcessorArtifactLoader) LoadWorkflowRevision(ctx context.Context, _, _ string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls++
	if l.err != nil {
		return nil, l.err
	}
	return append([]byte(nil), l.workflow...), nil
}

func (l *messageProcessorArtifactLoader) callCount() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

func strictExecutableProfileJSON(tolerateMissingPV1 bool) string {
	tolerance := ""
	if tolerateMissingPV1 {
		tolerance = `,"tolerance":{"missing_segments":["PV1"],"nte_anywhere":false,"extra_components":false,"unknown_segments":false,"non_standard_delimiters":false}`
	}
	return `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC"` + tolerance + `,"event_classifications":[{"message_type":"ADT^A01","condition":"PV1.2 == 'I'","event_type":"inpatient_admit","priority":1},{"message_type":"ADT^A01","event_type":"patient_admit","priority":2}]},"identifiers":{"assigning_authorities":[{"code":"HOSP","system":"urn:oid:1.2.3"}],"normalization":{"ssn_strip_dashes":true,"phone_normalize":false}}}`
}

func processorA01Message(includePV1 bool) []byte {
	msh := `MSH|^~\&|RAW-ONLY-SENTINEL|FAC|APP|FAC|20260713120000-0400||ADT^A01^ADT_A01|control-123|P|2.5.1`
	evn := processorSegment("EVN", 6, map[int]string{1: "A01", 2: "20260713120000", 6: "20260713115900-0400"})
	pid := processorSegment("PID", 8, map[int]string{1: "1", 3: "MRN-123^^^HOSP^MR", 5: "Patient^Test", 7: "19800101", 8: "F"})
	segments := []string{msh, evn, pid}
	if includePV1 {
		segments = append(segments, processorSegment("PV1", 44, map[int]string{1: "1", 2: "I", 3: "UNIT^101^A^FAC", 19: "visit-123", 44: "20260713120000"}))
	}
	return []byte(strings.Join(segments, "\r"))
}

func processorSegment(id string, lastField int, values map[int]string) string {
	fields := make([]string, lastField+1)
	fields[0] = id
	for index, value := range values {
		fields[index] = value
	}
	return strings.Join(fields, "|")
}

func messageProcessorEnvelope(t *testing.T, revision integration.IntegrationDefinitionRevision, raw []byte, tenantID string) integration.RawEnvelope {
	t.Helper()
	envelope, err := integration.NewRawEnvelope(integration.RawEnvelopeMetadata{
		TenantID:       tenantID,
		SourceID:       revision.Source.SourceID,
		Format:         revision.Format,
		ContentType:    "x-application/hl7-v2+er7",
		ReceivedAt:     time.Date(2026, 7, 13, 14, 0, 0, 0, time.UTC),
		Classification: integration.DataClassificationPHI,
	}, raw)
	if err != nil {
		t.Fatalf("NewRawEnvelope: %v", err)
	}
	return envelope
}
