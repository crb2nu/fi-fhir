package mllp

import (
	"context"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const testDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func testSource(t *testing.T) SourceRevision {
	t.Helper()
	revision, err := NewSourceRevision(SourceRevisionInput{
		ArtifactID: "mllp-source", RevisionID: "source-v1", SourceID: "hospital-a",
		ListenAddress: "127.0.0.1:2575", Encoding: "utf-8",
		Framing:  FramingPolicy{StartByte: StandardStartByte, EndByte: StandardEndByte, TrailerByte: StandardTrailerByte},
		Timeouts: TimeoutPolicy{ReadSeconds: 2, WriteSeconds: 2, IdleSeconds: 3, ProcessSeconds: 2},
		TLS:      TLSPolicy{Mode: TLSModeDisabled}, Clients: ClientPolicy{AllowedCIDRs: []string{"127.0.0.0/8"}},
		Acknowledgements: AcknowledgementPolicy{Mode: AcknowledgementModeApplication, IncludeErrorSegment: true},
		MaxMessageBytes:  4096, MaxConnections: 4,
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	return revision
}

func testBinding(source SourceRevision) lifecycle.RunnableBinding {
	return lifecycle.RunnableBinding{
		ReleaseID: "release-1", SnapshotVersion: 5, Health: integration.DeploymentHealthHealthy,
		IntegrationRevision: integration.ArtifactRevisionRef{ArtifactID: "definition-a", RevisionID: "revision-v1", Digest: testDigest},
		SourceRevision:      source.Reference(), SourceID: source.SourceID, Format: events.FormatHL7v2,
		Classification: integration.DataClassificationPHI,
		Deployment: integration.IntegrationDeploymentPolicy{
			ConnectionValidation: integration.ConnectionValidationPolicy{TimeoutSeconds: 5, MaxAgeSeconds: 60},
			Schedule:             integration.SchedulePolicy{Mode: integration.ScheduleModeContinuous},
			Health:               integration.HealthPolicy{CheckIntervalSeconds: 10, TimeoutSeconds: 2, FailureThreshold: 3},
			Capacity:             integration.CapacityPolicy{MaxInFlight: 2, MaxQueued: 4, MaxMessagesPerSecond: 20},
		},
	}
}

func testHL7(controlID string) []byte {
	return []byte("MSH|^~\\&|SEND|FAC|RECV|RFAC|20260715120000||ADT^A01|" + controlID + "|P|2.5\rPID|1||12345^^^HOSP^MR\r")
}

type resolverFunc func(context.Context, string, string) (lifecycle.RunnableBinding, error)

func (f resolverFunc) ResolveRunnable(ctx context.Context, tenantID, definitionID string) (lifecycle.RunnableBinding, error) {
	return f(ctx, tenantID, definitionID)
}

type processorFunc func(context.Context, integration.ProcessRequest) (integration.ProcessResult, error)

func (f processorFunc) Process(ctx context.Context, request integration.ProcessRequest) (integration.ProcessResult, error) {
	return f(ctx, request)
}

func acceptedResult(request integration.ProcessRequest) integration.ProcessResult {
	return integration.ProcessResult{
		Mode: request.Mode, TenantID: request.Security.TenantID, IntegrationRevision: request.IntegrationRevision,
		Receipt: &integration.Receipt{ID: "receipt-1", Status: integration.ReceiptStatusAccepted, RecordedAt: time.Now().UTC()},
	}
}

func testServiceConfig(source SourceRevision, resolver RunnableResolver, messageProcessor MessageProcessor) ServiceConfig {
	return ServiceConfig{
		TenantID: "tenant-a", DefinitionID: "definition-a", PrincipalID: "mllp-listener",
		Source: source, Resolver: resolver, Processor: messageProcessor,
		Clock: time.Now, NewID: func() string { return "mllp-id" },
	}
}

func acknowledgementCodeFromPayload(t *testing.T, payload []byte) string {
	t.Helper()
	segments := strings.Split(string(payload), "\r")
	if len(segments) < 2 {
		t.Fatalf("invalid ACK payload %q", payload)
	}
	fields := strings.Split(segments[1], "|")
	if len(fields) < 2 || fields[0] != "MSA" {
		t.Fatalf("invalid MSA segment %q", segments[1])
	}
	return fields[1]
}
