package preview

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestServiceBuildsTrustedPreviewRequest(t *testing.T) {
	revision := previewRevision(t)
	registry := &registryStub{binding: Binding{
		IntegrationRevision: revision.Reference(),
		SourceID:            revision.Source.SourceID,
		Format:              revision.Format,
		Classification:      revision.Policy.Classification,
	}}
	processorSpy := &processorStub{}
	now := time.Date(2026, 7, 13, 16, 30, 0, 0, time.UTC)
	service, err := NewService(registry, processorSpy, func() time.Time { return now })
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	security := integration.SecurityContext{
		TenantID:  "tenant-a",
		Principal: integration.Principal{ID: "engineer-1", Kind: integration.PrincipalKindHuman, AuthMethod: "bearer", Roles: []string{PreviewRole}},
	}
	payload := []byte("MSH|^~\\&|SENDING|FAC|RECEIVING|FAC|20260713120000||ADT^A01|control-1|P|2.5.1")

	_, err = service.Preview(context.Background(), security, Input{
		IntegrationID: "adt-east",
		Payload:       payload,
		CorrelationID: "correlation-1",
		Reason:        "validate mapping",
	})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	request := processorSpy.request
	if request.Mode != integration.ExecutionModePreview || request.IntegrationRevision != revision.Reference() {
		t.Fatalf("processor request binding drifted: %#v", request)
	}
	if request.Security.TenantID != security.TenantID || request.Security.Principal.ID != security.Principal.ID {
		t.Fatalf("processor request did not preserve authenticated identity: %#v", request.Security)
	}
	if request.Security.Reason != "validate mapping" {
		t.Fatalf("reason = %q", request.Security.Reason)
	}
	if request.Envelope.SourceID != revision.Source.SourceID || request.Envelope.Format != revision.Format {
		t.Fatalf("caller influenced trusted envelope binding: %#v", request.Envelope)
	}
	if request.Envelope.ReceivedAt != now || request.CorrelationID != "correlation-1" {
		t.Fatalf("request time/correlation drifted: %#v", request)
	}
	if string(request.Envelope.Bytes()) != string(payload) {
		t.Fatal("processor did not receive exact payload bytes")
	}
	payload[0] = 'X'
	if request.Envelope.Bytes()[0] == 'X' {
		t.Fatal("service retained caller-owned payload storage")
	}
}

func TestServiceRejectsBeforeRegistryOrProcessor(t *testing.T) {
	validSecurity := integration.SecurityContext{
		TenantID:  "tenant-a",
		Principal: integration.Principal{ID: "engineer-1", Kind: integration.PrincipalKindHuman, AuthMethod: "bearer", Roles: []string{PreviewRole}},
	}
	validInput := Input{IntegrationID: "adt-east", Payload: []byte("message"), CorrelationID: "correlation-1", Reason: "preview"}

	tests := []struct {
		name     string
		security integration.SecurityContext
		input    Input
		want     error
	}{
		{name: "missing tenant", security: integration.SecurityContext{Principal: validSecurity.Principal}, input: validInput, want: ErrUnauthenticated},
		{name: "missing principal", security: integration.SecurityContext{TenantID: "tenant-a"}, input: validInput, want: ErrUnauthenticated},
		{name: "missing role", security: integration.SecurityContext{TenantID: "tenant-a", Principal: integration.Principal{ID: "engineer-1", Kind: integration.PrincipalKindHuman, AuthMethod: "bearer"}}, input: validInput, want: ErrForbidden},
		{name: "empty integration", security: validSecurity, input: Input{Payload: validInput.Payload, CorrelationID: validInput.CorrelationID, Reason: validInput.Reason}, want: ErrInvalidInput},
		{name: "empty correlation", security: validSecurity, input: Input{IntegrationID: validInput.IntegrationID, Payload: validInput.Payload, Reason: validInput.Reason}, want: ErrInvalidInput},
		{name: "human reason required", security: validSecurity, input: Input{IntegrationID: validInput.IntegrationID, Payload: validInput.Payload, CorrelationID: validInput.CorrelationID}, want: ErrInvalidInput},
		{name: "oversized payload", security: validSecurity, input: Input{IntegrationID: validInput.IntegrationID, Payload: []byte(strings.Repeat("x", int(processor.MaxPreviewSourceBytes)+1)), CorrelationID: validInput.CorrelationID, Reason: validInput.Reason}, want: ErrPayloadTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := &registryStub{}
			processorSpy := &processorStub{}
			service, err := NewService(registry, processorSpy, time.Now)
			if err != nil {
				t.Fatalf("NewService: %v", err)
			}
			if _, err := service.Preview(context.Background(), tt.security, tt.input); !errors.Is(err, tt.want) {
				t.Fatalf("Preview error = %v, want %v", err, tt.want)
			}
			if registry.calls != 0 || processorSpy.calls != 0 {
				t.Fatalf("rejected input reached registry=%d processor=%d", registry.calls, processorSpy.calls)
			}
		})
	}
}

type registryStub struct {
	binding Binding
	err     error
	calls   int
}

func (r *registryStub) LookupPreviewBinding(_ context.Context, _, _ string) (Binding, error) {
	r.calls++
	return r.binding, r.err
}

type processorStub struct {
	request integration.ProcessRequest
	result  integration.ProcessResult
	err     error
	calls   int
}

func (p *processorStub) Process(_ context.Context, request integration.ProcessRequest) (integration.ProcessResult, error) {
	p.calls++
	p.request = request
	return p.result, p.err
}

func previewRevision(t *testing.T) integration.IntegrationDefinitionRevision {
	t.Helper()
	digest := func(character string) string { return "sha256:" + strings.Repeat(character, 64) }
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "integration-adt",
		RevisionID:   "definition-revision-1",
		TenantID:     "tenant-a",
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "source-adt", RevisionID: "source-1", Digest: digest("a")},
			SourceID:            "adt-east",
		},
		Format:   events.FormatHL7v2,
		Profile:  integration.ArtifactRevisionRef{ArtifactID: "profile-adt", RevisionID: "1", Digest: digest("b")},
		Workflow: integration.ArtifactRevisionRef{ArtifactID: "workflow-adt", RevisionID: "workflow-version-1", Digest: digest("c")},
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "fhir-primary", RevisionID: "destination-1", Digest: digest("d")},
			Class:               integration.DestinationClassProduction,
		}},
		Policy: integration.IntegrationPolicy{Classification: integration.DataClassificationPHI, RawRetention: integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral}},
		Created: integration.AuditEnvelope{
			TenantID: "tenant-a", Principal: integration.Principal{ID: "publisher", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc", Roles: []string{"publisher"}}, Reason: "publish", OccurredAt: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC),
		},
	})
	if err != nil {
		t.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}
	return revision
}
