package processor

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestEffectiveIdempotencyKeyPrecedenceAndDerivation(t *testing.T) {
	t.Parallel()

	fixture := newMessageProcessorFixture(t, strictExecutableProfileJSON(false), processorPublishedWorkflow, processorA01Message(true))
	request := fixture.request
	request.Mode = integration.ExecutionModeProduction

	derived, err := effectiveIdempotencyKey(request, fixture.revision, "control-123")
	if err != nil {
		t.Fatalf("derived idempotency key: %v", err)
	}
	if !strings.HasPrefix(derived, "derived:v1:") || len(derived) != len("derived:v1:")+64 {
		t.Fatalf("unexpected derived key %q", derived)
	}
	again, err := effectiveIdempotencyKey(request, fixture.revision, "control-123")
	if err != nil || again != derived {
		t.Fatalf("derived key changed: key=%q err=%v", again, err)
	}
	changedControlID, err := effectiveIdempotencyKey(request, fixture.revision, "control-456")
	if err != nil || changedControlID == derived {
		t.Fatalf("source message ID did not affect derived key: key=%q err=%v", changedControlID, err)
	}
	changedRevision := fixture.revision
	changedRevision.RevisionID = "revision-2"
	changedRevision.Digest = "sha256:" + strings.Repeat("9", 64)
	changedRevisionKey, err := effectiveIdempotencyKey(request, changedRevision, "control-123")
	if err != nil || changedRevisionKey == derived {
		t.Fatalf("integration revision did not affect derived key: key=%q err=%v", changedRevisionKey, err)
	}

	request.IdempotencyKey = "caller-key-123"
	explicit, err := effectiveIdempotencyKey(request, fixture.revision, "control-123")
	if err != nil {
		t.Fatalf("explicit idempotency key: %v", err)
	}
	if explicit != request.IdempotencyKey || explicit == derived {
		t.Fatalf("explicit key did not win: got %q derived %q", explicit, derived)
	}

	for _, invalid := range []string{" whitespace ", "control\nkey", strings.Repeat("x", 513)} {
		request.IdempotencyKey = invalid
		if _, err := effectiveIdempotencyKey(request, fixture.revision, "control-123"); !errors.Is(err, ErrInvalidProcessRequest) {
			t.Fatalf("invalid key %q error = %v", invalid, err)
		}
	}
}

func TestSubmissionFingerprintExcludesRetryCorrelationButBindsPayloadAndIdentity(t *testing.T) {
	t.Parallel()

	fixture := newMessageProcessorFixture(t, strictExecutableProfileJSON(false), processorPublishedWorkflow, processorA01Message(true))
	request := fixture.request
	request.Mode = integration.ExecutionModeProduction
	first, err := submissionRequestFingerprint(request, fixture.revision)
	if err != nil {
		t.Fatalf("submission fingerprint: %v", err)
	}

	retry := request
	retry.CorrelationID = "retry-correlation"
	retry.Envelope.ReceivedAt = retry.Envelope.ReceivedAt.Add(time.Minute)
	second, err := submissionRequestFingerprint(retry, fixture.revision)
	if err != nil || second != first {
		t.Fatalf("retry metadata changed fingerprint: first=%q second=%q err=%v", first, second, err)
	}

	changedPayload := request
	changedPayload.Envelope = messageProcessorEnvelope(t, fixture.revision, bytes.Replace(processorA01Message(true), []byte("Patient^Test"), []byte("Patient^Changed"), 1), fixture.revision.TenantID)
	third, err := submissionRequestFingerprint(changedPayload, fixture.revision)
	if err != nil || third == first {
		t.Fatalf("payload did not change fingerprint: fingerprint=%q err=%v", third, err)
	}

	changedPrincipal := request
	changedPrincipal.Security.Principal.ID = "other-service"
	fourth, err := submissionRequestFingerprint(changedPrincipal, fixture.revision)
	if err != nil || fourth == first {
		t.Fatalf("principal did not change fingerprint: fingerprint=%q err=%v", fourth, err)
	}
}

func TestFinalizeProductionResultCreatesDeterministicQueuedLineage(t *testing.T) {
	t.Parallel()

	fixture := newMessageProcessorFixture(t, strictExecutableProfileJSON(false), processorPublishedWorkflow, processorA01Message(true))
	preview, err := fixture.processor.Process(t.Context(), fixture.request)
	if err != nil {
		t.Fatalf("preview Process: %v", err)
	}
	request := fixture.request
	request.Mode = integration.ExecutionModeProduction
	request.IdempotencyKey = "caller-key-123"
	plan := preview
	plan.Mode = integration.ExecutionModeProduction
	plan.Diagnostics = nil
	for index := range plan.Routes {
		plan.Routes[index].DiagnosticCodes = nil
	}
	for index := range plan.Deliveries {
		plan.Deliveries[index].Status = integration.DeliveryStatusPlanned
		plan.Deliveries[index].DiagnosticCodes = nil
	}
	recordedAt := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	first, err := finalizeProductionResult(request, fixture.revision, plan, request.IdempotencyKey, recordedAt)
	if err != nil {
		t.Fatalf("finalize production result: %v", err)
	}
	second, err := finalizeProductionResult(request, fixture.revision, plan, request.IdempotencyKey, recordedAt)
	if err != nil {
		t.Fatalf("finalize production result again: %v", err)
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
		t.Fatalf("final production result is not deterministic:\nfirst: %s\nsecond: %s", firstJSON, secondJSON)
	}
	diagnosticsJSON, err := json.Marshal(first.Diagnostics)
	if err != nil || first.Diagnostics == nil || !bytes.Equal(diagnosticsJSON, []byte(`[]`)) {
		t.Fatalf("empty diagnostics were not normalized as a JSON array: %#v, %s, %v", first.Diagnostics, diagnosticsJSON, err)
	}
	if err := first.ValidateProductionAgainst(fixture.revision); err != nil {
		t.Fatalf("strict production validation: %v", err)
	}
	if first.Receipt == nil || first.Receipt.IdempotencyKey != request.IdempotencyKey {
		t.Fatalf("receipt did not preserve explicit key: %#v", first.Receipt)
	}
	if first.Correlations.TraceID == "" || len(first.Correlations.DeliveryAttemptIDs) != len(first.Deliveries) {
		t.Fatalf("durable lineage is incomplete: %#v", first.Correlations)
	}
	for _, delivery := range first.Deliveries {
		if delivery.Status != integration.DeliveryStatusQueued || delivery.AttemptID == "" || delivery.AttemptCount != 1 {
			t.Fatalf("delivery was not initialized exactly once: %#v", delivery)
		}
	}
}

func TestPostgresSubmissionStoreRequiresDatabase(t *testing.T) {
	t.Parallel()

	if _, err := NewPostgresSubmissionStore(nil, PostgresSubmissionConfig{}); !errors.Is(err, ErrPostgresSubmissionUnavailable) {
		t.Fatalf("nil database error = %v", err)
	}
}
