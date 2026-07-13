package integration_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestRawEnvelopePayloadIsNonSerializable(t *testing.T) {
	payload := []byte("MSH|^~\\&|PHI-SENTINEL|patient-123")
	envelope, err := integration.NewRawEnvelope(integration.RawEnvelopeMetadata{
		TenantID:       "acme-health",
		SourceID:       "adt-east",
		Format:         events.FormatHL7v2,
		ContentType:    "x-application/hl7-v2+er7",
		ReceivedAt:     time.Date(2026, 7, 13, 1, 0, 0, 0, time.UTC),
		Classification: integration.DataClassificationPHI,
	}, payload)
	if err != nil {
		t.Fatalf("construct envelope: %v", err)
	}
	payload[0] = 'X'

	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	if bytes.Contains(encoded, []byte("PHI-SENTINEL")) || bytes.Contains(encoded, []byte("MSH|")) {
		t.Fatalf("raw payload leaked into JSON: %s", encoded)
	}
	for _, required := range []string{"payload_digest", "size_bytes", "source_id", "classification"} {
		if !bytes.Contains(encoded, []byte(required)) {
			t.Fatalf("envelope JSON missing %q: %s", required, encoded)
		}
	}

	copyOne := envelope.Bytes()
	copyOne[0] = 'X'
	copyTwo := envelope.Bytes()
	if copyTwo[0] != 'M' {
		t.Fatal("constructor or Bytes returned aliased payload storage")
	}
}

func TestRawRetentionDefaultsEphemeral(t *testing.T) {
	var policy integration.RawRetentionPolicy
	if got := policy.EffectiveMode(); got != integration.RawRetentionModeEphemeral {
		t.Fatalf("effective mode = %q, want ephemeral", got)
	}
	if err := policy.Validate(); err != nil {
		t.Fatalf("default ephemeral policy should be valid: %v", err)
	}
}

func TestEncryptedRawRetentionRequirements(t *testing.T) {
	valid := integration.RawRetentionPolicy{
		Mode:            integration.RawRetentionModeEncrypted,
		TTLSeconds:      3600,
		Purpose:         "incident replay",
		StorageRevision: ptr(testArtifact("encrypted-raw-store", "storage-rev-001", '7')),
		EncryptionKey: &integration.SecretReference{
			Provider: integration.SecretProviderKubernetes,
			Key:      "fi-fhir/raw-retention-key",
		},
		AuthorizedBy:        testHumanPrincipal(),
		AccessAuditRequired: true,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid encrypted policy: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*integration.RawRetentionPolicy)
	}{
		{name: "ttl", mutate: func(policy *integration.RawRetentionPolicy) { policy.TTLSeconds = 0 }},
		{name: "purpose", mutate: func(policy *integration.RawRetentionPolicy) { policy.Purpose = "" }},
		{name: "storage", mutate: func(policy *integration.RawRetentionPolicy) { policy.StorageRevision = nil }},
		{name: "key", mutate: func(policy *integration.RawRetentionPolicy) { policy.EncryptionKey = nil }},
		{name: "actor", mutate: func(policy *integration.RawRetentionPolicy) { policy.AuthorizedBy = integration.Principal{} }},
		{name: "access audit", mutate: func(policy *integration.RawRetentionPolicy) { policy.AccessAuditRequired = false }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := valid
			tt.mutate(&policy)
			if err := policy.Validate(); err == nil {
				t.Fatalf("expected missing %s to fail", tt.name)
			}
		})
	}
}

func TestEphemeralRawRetentionRejectsStorageConfiguration(t *testing.T) {
	policy := integration.RawRetentionPolicy{
		Mode:       integration.RawRetentionModeEphemeral,
		TTLSeconds: 60,
	}
	if err := policy.Validate(); err == nil {
		t.Fatal("ephemeral policy accepted retention configuration")
	}
}

func TestExecutionModeDeliveryBoundary(t *testing.T) {
	tests := []struct {
		mode    integration.ExecutionMode
		class   integration.DestinationClass
		allowed bool
	}{
		{mode: integration.ExecutionModeProduction, class: integration.DestinationClassProduction, allowed: true},
		{mode: integration.ExecutionModeProduction, class: integration.DestinationClassSandbox, allowed: true},
		{mode: integration.ExecutionModePreview, class: integration.DestinationClassProduction, allowed: false},
		{mode: integration.ExecutionModePreview, class: integration.DestinationClassSandbox, allowed: false},
	}
	for _, tt := range tests {
		if got := tt.mode.AllowsDelivery(tt.class); got != tt.allowed {
			t.Errorf("%s allows %s = %t, want %t", tt.mode, tt.class, got, tt.allowed)
		}
	}
}

func TestProcessRequestMatchesRevision(t *testing.T) {
	revision, err := integration.NewIntegrationDefinitionRevision(validRevisionInput())
	if err != nil {
		t.Fatalf("construct revision: %v", err)
	}
	envelope, err := integration.NewRawEnvelope(integration.RawEnvelopeMetadata{
		TenantID:       revision.TenantID,
		SourceID:       revision.Source.SourceID,
		Format:         revision.Format,
		ReceivedAt:     time.Now().UTC(),
		Classification: integration.DataClassificationPHI,
	}, []byte("MSH|^~\\&|ADT"))
	if err != nil {
		t.Fatalf("construct envelope: %v", err)
	}
	request := integration.ProcessRequest{
		Mode:                integration.ExecutionModeProduction,
		IntegrationRevision: revision.Reference(),
		Security: integration.SecurityContext{
			TenantID:  revision.TenantID,
			Principal: testServicePrincipal(),
		},
		Envelope:       envelope,
		IdempotencyKey: "source:adt-east:message:123",
		CorrelationID:  "corr-123",
	}
	if err := request.ValidateAgainst(revision); err != nil {
		t.Fatalf("valid request: %v", err)
	}
	derivedIdempotency := request
	derivedIdempotency.IdempotencyKey = ""
	if err := derivedIdempotency.ValidateAgainst(revision); err != nil {
		t.Fatalf("request using derived idempotency should remain valid: %v", err)
	}

	humanRequest := derivedIdempotency
	humanRequest.Mode = integration.ExecutionModePreview
	humanRequest.Security = integration.SecurityContext{
		TenantID:  revision.TenantID,
		Principal: testHumanPrincipal(),
		Reason:    "inspect profile behavior",
	}
	if err := humanRequest.ValidateAgainst(revision); err != nil {
		t.Fatalf("valid human request: %v", err)
	}
	humanRequest.Security.Reason = ""
	if err := humanRequest.ValidateAgainst(revision); err == nil {
		t.Fatal("human request without a reason was accepted")
	}

	tests := []struct {
		name   string
		mutate func(*integration.ProcessRequest)
	}{
		{name: "security tenant", mutate: func(request *integration.ProcessRequest) { request.Security.TenantID = "other" }},
		{name: "envelope tenant", mutate: func(request *integration.ProcessRequest) { request.Envelope.TenantID = "other" }},
		{name: "source", mutate: func(request *integration.ProcessRequest) { request.Envelope.SourceID = "other" }},
		{name: "format", mutate: func(request *integration.ProcessRequest) { request.Envelope.Format = events.FormatCSV }},
		{name: "revision", mutate: func(request *integration.ProcessRequest) { request.IntegrationRevision.RevisionID = "other" }},
		{name: "anonymous", mutate: func(request *integration.ProcessRequest) { request.Security.Principal = integration.Principal{} }},
		{name: "principal source", mutate: func(request *integration.ProcessRequest) { request.Security.Principal.SourceID = "other" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := request
			tt.mutate(&candidate)
			if err := candidate.ValidateAgainst(revision); err == nil {
				t.Fatalf("expected %s mismatch to fail", tt.name)
			}
		})
	}
}

func TestPreviewResultRejectsSideEffects(t *testing.T) {
	revision, err := integration.NewIntegrationDefinitionRevision(validRevisionInput())
	if err != nil {
		t.Fatalf("construct revision: %v", err)
	}
	base := integration.ProcessResult{
		Mode:                integration.ExecutionModePreview,
		TenantID:            revision.TenantID,
		IntegrationRevision: revision.Reference(),
		Security: integration.SecurityContext{
			TenantID:  revision.TenantID,
			Principal: testServicePrincipal(),
		},
		Routes: []integration.RouteResult{
			{
				TenantID:       revision.TenantID,
				Route:          "admit-to-fhir",
				Matched:        true,
				TransformCount: 1,
				PlannedActions: []string{"fhir-create"},
			},
		},
		Deliveries: []integration.DeliveryResult{
			{
				TenantID:    revision.TenantID,
				Destination: revision.Destinations[0],
				Route:       "admit-to-fhir",
				Action:      "fhir-create",
				Status:      integration.DeliveryStatusSuppressed,
			},
		},
		Correlations: integration.CorrelationIDs{
			TenantID:      revision.TenantID,
			CorrelationID: "corr-preview",
		},
	}
	if err := base.Validate(); err != nil {
		t.Fatalf("valid preview result: %v", err)
	}
	withoutLineage := base
	withoutLineage.Deliveries = append([]integration.DeliveryResult(nil), base.Deliveries...)
	withoutLineage.Deliveries[0].Route = ""
	withoutLineage.Deliveries[0].Action = ""
	if err := withoutLineage.Validate(); err == nil {
		t.Fatal("preview delivery without route/action lineage was accepted")
	}

	withReceipt := base
	withReceipt.Receipt = &integration.Receipt{
		ID:       "receipt-preview",
		TenantID: revision.TenantID,
		Status:   integration.ReceiptStatusAccepted,
	}
	if err := withReceipt.Validate(); err == nil {
		t.Fatal("preview result accepted a durable receipt")
	}

	for _, status := range []integration.DeliveryStatus{
		integration.DeliveryStatusQueued,
		integration.DeliveryStatusSucceeded,
		integration.DeliveryStatusFailed,
	} {
		t.Run(string(status), func(t *testing.T) {
			candidate := base
			candidate.Deliveries = append([]integration.DeliveryResult(nil), base.Deliveries...)
			candidate.Deliveries[0].Status = status
			if err := candidate.Validate(); err == nil {
				t.Fatalf("preview accepted delivery status %q", status)
			}
		})
	}
}

func validProductionResult(t *testing.T) (integration.IntegrationDefinitionRevision, integration.ProcessResult) {
	t.Helper()
	revision, err := integration.NewIntegrationDefinitionRevision(validRevisionInput())
	if err != nil {
		t.Fatalf("construct revision: %v", err)
	}
	processedEvent, err := integration.NewProcessedEvent(integration.ProcessedEventMetadata{
		TenantID:       revision.TenantID,
		Classification: integration.DataClassificationPHI,
	}, &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{
			ID:              "event-123",
			Type:            events.EventPatientAdmit,
			SourceMessageID: "message-123",
			CorrelationID:   "corr-123",
		},
		RawPayload: json.RawMessage([]byte("MSH|^~\\\\&|PHI-SENTINEL")),
	})
	if err != nil {
		t.Fatalf("construct processed event: %v", err)
	}
	diagnostic, err := integration.NewDiagnostic(integration.DiagnosticInput{
		TenantID:       revision.TenantID,
		Severity:       integration.DiagnosticSeverityWarning,
		Stage:          "semantic",
		Code:           "MISSING_PV1",
		Source:         "hl7v2.parser",
		Classification: integration.DataClassificationPHI,
	})
	if err != nil {
		t.Fatalf("construct diagnostic: %v", err)
	}
	result := integration.ProcessResult{
		Mode:                integration.ExecutionModeProduction,
		TenantID:            revision.TenantID,
		IntegrationRevision: revision.Reference(),
		Security: integration.SecurityContext{
			TenantID:  revision.TenantID,
			Principal: testServicePrincipal(),
		},
		Receipt: &integration.Receipt{
			ID:                  "receipt-123",
			TenantID:            revision.TenantID,
			IntegrationRevision: revision.Reference(),
			Status:              integration.ReceiptStatusAccepted,
			IdempotencyKey:      "idem-123",
			RecordedAt:          time.Date(2026, 7, 13, 1, 2, 3, 0, time.UTC),
			CorrelationID:       "corr-123",
			RawRetentionMode:    integration.RawRetentionModeEphemeral,
			Principal:           testServicePrincipal(),
		},
		Events: []integration.ProcessedEvent{processedEvent},
		Routes: []integration.RouteResult{
			{
				TenantID:       revision.TenantID,
				Route:          "admit-to-fhir",
				Matched:        true,
				TransformCount: 1,
				PlannedActions: []string{"fhir-create"},
			},
		},
		Diagnostics: []integration.Diagnostic{diagnostic},
		Deliveries: []integration.DeliveryResult{
			{
				TenantID:     revision.TenantID,
				Destination:  revision.Destinations[0],
				Route:        "admit-to-fhir",
				Action:       "fhir-create",
				Status:       integration.DeliveryStatusQueued,
				AttemptID:    "attempt-123",
				AttemptCount: 1,
			},
		},
		Correlations: integration.CorrelationIDs{
			TenantID:           revision.TenantID,
			CorrelationID:      "corr-123",
			TraceID:            "trace-123",
			SourceMessageID:    "message-123",
			ReceiptID:          "receipt-123",
			EventIDs:           []string{"event-123"},
			DeliveryAttemptIDs: []string{"attempt-123"},
		},
	}
	return revision, result
}

func TestProcessResultJSONContract(t *testing.T) {
	revision, result := validProductionResult(t)
	if err := result.ValidateAgainst(revision); err != nil {
		t.Fatalf("valid result: %v", err)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	for _, forbidden := range []string{"MSH|", "PHI-SENTINEL", `"raw_payload"`, `"original"`} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("result JSON contains forbidden field/value %q: %s", forbidden, encoded)
		}
	}

	var decoded integration.ProcessResult
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if err := decoded.ValidateAgainst(revision); err != nil {
		t.Fatalf("validate decoded result: %v", err)
	}
	if decoded.Correlations.ReceiptID != result.Correlations.ReceiptID {
		t.Fatalf("receipt correlation lost: %q", decoded.Correlations.ReceiptID)
	}
}

func TestProductionResultRequiresDurableAcceptance(t *testing.T) {
	_, base := validProductionResult(t)

	tests := []struct {
		name   string
		mutate func(*integration.ProcessResult)
	}{
		{name: "receipt", mutate: func(result *integration.ProcessResult) { result.Receipt = nil }},
		{name: "idempotency", mutate: func(result *integration.ProcessResult) { result.Receipt.IdempotencyKey = "" }},
		{name: "receipt correlation", mutate: func(result *integration.ProcessResult) { result.Correlations.ReceiptID = "" }},
		{
			name: "rejected with outputs",
			mutate: func(result *integration.ProcessResult) {
				result.Receipt.Status = integration.ReceiptStatusRejected
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := cloneProcessResult(base)
			tt.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("expected invalid production state %q to fail", tt.name)
			}
		})
	}
}

func TestProcessResultMatchesResolvedRevision(t *testing.T) {
	revision, base := validProductionResult(t)
	if err := base.ValidateAgainst(revision); err != nil {
		t.Fatalf("valid result: %v", err)
	}

	unbound := cloneProcessResult(base)
	unbound.Deliveries[0].Destination = integration.DestinationRevisionRef{
		ArtifactRevisionRef: testArtifact("unbound", "destination-rev-001", '8'),
		Class:               integration.DestinationClassProduction,
	}
	if err := unbound.ValidateAgainst(revision); err == nil {
		t.Fatal("result accepted an unbound destination")
	}

	retentionMismatch := cloneProcessResult(base)
	retentionMismatch.Receipt.RawRetentionMode = integration.RawRetentionModeEncrypted
	expiresAt := time.Now().UTC().Add(time.Hour)
	retentionMismatch.Receipt.RawExpiresAt = &expiresAt
	if err := retentionMismatch.ValidateAgainst(revision); err == nil {
		t.Fatal("result accepted a receipt that contradicted the integration retention policy")
	}
}

func TestEncryptedReceiptCannotOutlivePolicyTTL(t *testing.T) {
	input := validRevisionInput()
	input.Policy.RawRetention = integration.RawRetentionPolicy{
		Mode:            integration.RawRetentionModeEncrypted,
		TTLSeconds:      3600,
		Purpose:         "incident replay",
		StorageRevision: ptr(testArtifact("encrypted-raw-store", "storage-rev-001", '7')),
		EncryptionKey: &integration.SecretReference{
			Provider: integration.SecretProviderKubernetes,
			Key:      "fi-fhir/raw-retention-key",
		},
		AuthorizedBy:        testHumanPrincipal(),
		AccessAuditRequired: true,
	}
	revision, err := integration.NewIntegrationDefinitionRevision(input)
	if err != nil {
		t.Fatalf("construct encrypted revision: %v", err)
	}
	_, result := validProductionResult(t)
	result.IntegrationRevision = revision.Reference()
	result.Receipt.IntegrationRevision = revision.Reference()
	result.Receipt.RawRetentionMode = integration.RawRetentionModeEncrypted
	expiresAt := result.Receipt.RecordedAt.Add(time.Hour)
	result.Receipt.RawExpiresAt = &expiresAt
	if err := result.ValidateAgainst(revision); err != nil {
		t.Fatalf("receipt expiring at policy TTL: %v", err)
	}

	expiresAt = expiresAt.Add(time.Second)
	result.Receipt.RawExpiresAt = &expiresAt
	if err := result.ValidateAgainst(revision); err == nil {
		t.Fatal("receipt outliving the policy TTL was accepted")
	}
}

func processRequestForResult(t *testing.T, revision integration.IntegrationDefinitionRevision, result integration.ProcessResult) integration.ProcessRequest {
	t.Helper()
	envelope, err := integration.NewRawEnvelope(integration.RawEnvelopeMetadata{
		TenantID:       revision.TenantID,
		SourceID:       revision.Source.SourceID,
		Format:         revision.Format,
		ReceivedAt:     result.Receipt.RecordedAt.Add(-time.Second),
		Classification: revision.Policy.Classification,
	}, []byte("MSH|^~\\&|ADT"))
	if err != nil {
		t.Fatalf("construct request envelope: %v", err)
	}
	return integration.ProcessRequest{
		Mode:                result.Mode,
		IntegrationRevision: revision.Reference(),
		Security:            result.Security,
		Envelope:            envelope,
		IdempotencyKey:      result.Receipt.IdempotencyKey,
		CorrelationID:       result.Correlations.CorrelationID,
	}
}

func TestProcessResultBindsOriginalRequest(t *testing.T) {
	revision, result := validProductionResult(t)
	request := processRequestForResult(t, revision, result)
	if err := result.ValidateFor(request, revision); err != nil {
		t.Fatalf("valid request/result binding: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*integration.ProcessRequest)
	}{
		{name: "mode", mutate: func(candidate *integration.ProcessRequest) { candidate.Mode = integration.ExecutionModePreview }},
		{name: "actor", mutate: func(candidate *integration.ProcessRequest) { candidate.Security.Principal.ID = "other-adapter" }},
		{name: "correlation", mutate: func(candidate *integration.ProcessRequest) { candidate.CorrelationID = "other-correlation" }},
		{name: "explicit idempotency", mutate: func(candidate *integration.ProcessRequest) { candidate.IdempotencyKey = "other-idempotency" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := request
			tt.mutate(&candidate)
			if err := candidate.ValidateAgainst(revision); err != nil {
				t.Fatalf("mutated request should remain independently valid: %v", err)
			}
			if err := result.ValidateFor(candidate, revision); err == nil {
				t.Fatalf("result accepted mismatched request %s", tt.name)
			}
		})
	}
}

func TestExecutedDeliveryMatchesRoutePlan(t *testing.T) {
	_, base := validProductionResult(t)
	tests := []struct {
		name   string
		mutate func(*integration.ProcessResult)
	}{
		{name: "missing route", mutate: func(result *integration.ProcessResult) { result.Deliveries[0].Route = "" }},
		{name: "missing action", mutate: func(result *integration.ProcessResult) { result.Deliveries[0].Action = "" }},
		{name: "unplanned action", mutate: func(result *integration.ProcessResult) { result.Deliveries[0].Action = "unplanned" }},
		{name: "duplicate route", mutate: func(result *integration.ProcessResult) { result.Routes = append(result.Routes, result.Routes[0]) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := cloneProcessResult(base)
			tt.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("executed delivery accepted invalid route lineage: %s", tt.name)
			}
		})
	}
}

func TestProcessResultRejectsTenantAndActorDrift(t *testing.T) {
	_, base := validProductionResult(t)
	tests := []struct {
		name   string
		mutate func(*integration.ProcessResult)
	}{
		{name: "security tenant", mutate: func(result *integration.ProcessResult) { result.Security.TenantID = "other" }},
		{name: "receipt tenant", mutate: func(result *integration.ProcessResult) { result.Receipt.TenantID = "other" }},
		{name: "receipt principal", mutate: func(result *integration.ProcessResult) { result.Receipt.Principal.ID = "other" }},
		{name: "event tenant", mutate: func(result *integration.ProcessResult) { result.Events[0].TenantID = "other" }},
		{name: "event source message", mutate: func(result *integration.ProcessResult) { result.Events[0].SourceMessageID = "other" }},
		{name: "event correlation", mutate: func(result *integration.ProcessResult) { result.Events[0].CorrelationID = "other" }},
		{name: "diagnostic tenant", mutate: func(result *integration.ProcessResult) { result.Diagnostics[0].TenantID = "other" }},
		{name: "route tenant", mutate: func(result *integration.ProcessResult) { result.Routes[0].TenantID = "other" }},
		{name: "delivery tenant", mutate: func(result *integration.ProcessResult) { result.Deliveries[0].TenantID = "other" }},
		{name: "correlation tenant", mutate: func(result *integration.ProcessResult) { result.Correlations.TenantID = "other" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := cloneProcessResult(base)
			tt.mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("expected %s drift to fail", tt.name)
			}
		})
	}
}

func TestProcessResultPreservesHumanReason(t *testing.T) {
	revision, result := validProductionResult(t)
	result.Security.Principal = testHumanPrincipal()
	result.Security.Reason = "approved replay for incident 42"
	result.Receipt.Principal = testHumanPrincipal()
	result.Receipt.Reason = result.Security.Reason
	if err := result.ValidateAgainst(revision); err != nil {
		t.Fatalf("valid human result: %v", err)
	}

	result.Receipt.Reason = ""
	if err := result.Validate(); err == nil {
		t.Fatal("human receipt without its operation reason was accepted")
	}
}

func TestProcessedEventConstructorRemovesRawPayload(t *testing.T) {
	canonical := &events.PatientAdmitEvent{
		EventMeta: events.EventMeta{
			ID:              "event-raw-test",
			Type:            events.EventPatientAdmit,
			SourceMessageID: "message-raw-test",
			CorrelationID:   "corr-raw-test",
			ParseWarnings: []events.ParseWarning{
				{
					Phase:       "semantic",
					Code:        "RAW_WARNING",
					Message:     "WARNING-SENTINEL",
					Explanation: "MSH|^~\\&|WARNING-SENTINEL",
				},
			},
		},
		RawPayload: json.RawMessage([]byte("MSH|^~\\\\&|PHI-SENTINEL")),
	}
	processed, err := integration.NewProcessedEvent(integration.ProcessedEventMetadata{
		TenantID:       "acme-health",
		Classification: integration.DataClassificationPHI,
	}, canonical)
	if err != nil {
		t.Fatalf("construct processed event: %v", err)
	}
	if !bytes.Contains(canonical.RawPayload, []byte("PHI-SENTINEL")) {
		t.Fatal("processed event construction mutated the caller's canonical event")
	}
	encoded, err := json.Marshal(processed)
	if err != nil {
		t.Fatalf("marshal processed event: %v", err)
	}
	for _, forbidden := range []string{"PHI-SENTINEL", "WARNING-SENTINEL", "MSH|", `"raw_payload"`, `"parse_warnings"`} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("processed event leaked %q: %s", forbidden, encoded)
		}
	}
	if payload := processed.PayloadJSON(); !json.Valid(payload) {
		t.Fatalf("processed payload is not valid JSON: %s", payload)
	}

	_, err = integration.NewProcessedEvent(integration.ProcessedEventMetadata{
		TenantID:       "acme-health",
		Classification: integration.DataClassificationPHI,
	}, events.Event{
		EventMeta: events.EventMeta{ID: "generic", Type: events.EventPatientAdmit},
		Data:      json.RawMessage(`"MSH|^~\\&|PHI-SENTINEL"`),
	})
	if err == nil {
		t.Fatal("generic event payload bypassed canonical-event sanitization")
	}

	for _, key := range []string{"rawPayload", "raw-message", "source_payload"} {
		t.Run(key, func(t *testing.T) {
			wire := []byte(`{"tenant_id":"acme-health","id":"event-raw","type":"patient_admit","source_message_id":"message-raw","correlation_id":"corr-raw","classification":"phi","payload":{"id":"event-raw","type":"patient_admit","source_message_id":"message-raw","correlation_id":"corr-raw","` + key + `":"PHI-SENTINEL"}}`)
			if !json.Valid(wire) {
				t.Fatalf("adversarial wire payload is not valid JSON: %s", wire)
			}
			var candidate integration.ProcessedEvent
			if err := json.Unmarshal(wire, &candidate); err == nil {
				t.Fatalf("processed event accepted alternate raw key %q", key)
			}
		})
	}

	for name, forbiddenMember := range map[string]string{
		"canonical raw payload": `"raw_payload":"PHI-SENTINEL"`,
		"parser warnings":       `"parse_warnings":[{"phase":"semantic","code":"RAW_WARNING","message":"WARNING-SENTINEL"}]`,
	} {
		t.Run(name, func(t *testing.T) {
			wire := []byte(`{"tenant_id":"acme-health","id":"event-raw","type":"patient_admit","source_message_id":"message-raw","correlation_id":"corr-raw","classification":"phi","payload":{"id":"event-raw","type":"patient_admit","source_message_id":"message-raw","correlation_id":"corr-raw",` + forbiddenMember + `}}`)
			var candidate integration.ProcessedEvent
			if err := json.Unmarshal(wire, &candidate); err == nil {
				t.Fatalf("processed event wire accepted %s", name)
			}
		})
	}

	maliciousWire := []byte(`{"tenant_id":"acme-health","id":"event-wire","type":"patient_admit","source_message_id":"message-wire","correlation_id":"corr-wire","classification":"phi","payload":{"id":"event-wire","type":"patient_admit","source_message_id":"message-wire","correlation_id":"corr-wire","data":"MSH|PHI-SENTINEL"}}`)
	var malicious integration.ProcessedEvent
	if err := json.Unmarshal(maliciousWire, &malicious); err == nil {
		t.Fatal("wire decoding bypassed the concrete canonical-event schema")
	}

	typeMismatchWire := []byte(`{"tenant_id":"acme-health","id":"event-wire","type":"patient_transfer","source_message_id":"message-wire","correlation_id":"corr-wire","classification":"phi","payload":{"id":"event-wire","type":"patient_admit","source_message_id":"message-wire","correlation_id":"corr-wire"}}`)
	var typeMismatch integration.ProcessedEvent
	if err := json.Unmarshal(typeMismatchWire, &typeMismatch); err == nil {
		t.Fatal("wire wrapper accepted a different semantic type than its canonical payload")
	}

	mismatched := &events.PatientAdmitEvent{EventMeta: events.EventMeta{
		ID:              "event-mismatch",
		Type:            events.EventLabResult,
		SourceMessageID: "message-mismatch",
		CorrelationID:   "corr-mismatch",
	}}
	if _, err := integration.NewProcessedEvent(integration.ProcessedEventMetadata{
		TenantID:       "acme-health",
		Classification: integration.DataClassificationPHI,
	}, mismatched); err == nil {
		t.Fatal("patient-admit concrete type accepted a lab-result semantic type")
	}
}

func TestDiagnosticConstructorRejectsRawMessage(t *testing.T) {
	wire := []byte(`{"tenant_id":"acme-health","severity":"error","stage":"syntactic","code":"INVALID_MESSAGE","message":"patient_id,MSH|^~\\\\&|PHI-SENTINEL","source":"hl7v2.parser","classification":"phi"}`)
	var diagnostic integration.Diagnostic
	if err := json.Unmarshal(wire, &diagnostic); err == nil {
		t.Fatal("diagnostic wire accepted caller-controlled raw message text")
	}
}

func cloneProcessResult(result integration.ProcessResult) integration.ProcessResult {
	clone := result
	if result.ArtifactRevisions != nil {
		artifactRevisions := *result.ArtifactRevisions
		clone.ArtifactRevisions = &artifactRevisions
	}
	if result.Receipt != nil {
		receipt := *result.Receipt
		clone.Receipt = &receipt
	}
	clone.Events = append([]integration.ProcessedEvent(nil), result.Events...)
	clone.Diagnostics = append([]integration.Diagnostic(nil), result.Diagnostics...)
	clone.Routes = append([]integration.RouteResult(nil), result.Routes...)
	clone.Deliveries = append([]integration.DeliveryResult(nil), result.Deliveries...)
	clone.Correlations.EventIDs = append([]string(nil), result.Correlations.EventIDs...)
	clone.Correlations.DeliveryAttemptIDs = append([]string(nil), result.Correlations.DeliveryAttemptIDs...)
	return clone
}

func TestSecretReferenceRoundTripContainsReferenceOnly(t *testing.T) {
	reference := integration.SecretReference{
		Provider: integration.SecretProviderVault,
		Key:      "clinical/fhir/client-secret",
		Version:  "42",
	}
	encoded, err := json.Marshal(reference)
	if err != nil {
		t.Fatalf("marshal reference: %v", err)
	}
	for _, required := range []string{"provider", "key", "version"} {
		if !bytes.Contains(encoded, []byte(required)) {
			t.Fatalf("reference missing %q: %s", required, encoded)
		}
	}
	for _, forbidden := range []string{"value", "password", "token", "client_secret", "plaintext-secret-sentinel"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("reference exposed forbidden key/value %q: %s", forbidden, encoded)
		}
	}
}

func ptr[T any](value T) *T {
	return &value
}
