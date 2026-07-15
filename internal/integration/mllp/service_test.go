package mllp

import (
	"context"
	"errors"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

func TestServiceSubmitsExactDeployedProductionRequest(t *testing.T) {
	source := testSource(t)
	binding := testBinding(source)
	var captured integration.ProcessRequest
	service, err := NewService(testServiceConfig(source,
		resolverFunc(func(_ context.Context, tenantID, definitionID string) (lifecycle.RunnableBinding, error) {
			if tenantID != "tenant-a" || definitionID != "definition-a" {
				t.Fatalf("unexpected lookup %s/%s", tenantID, definitionID)
			}
			return binding, nil
		}),
		processorFunc(func(_ context.Context, request integration.ProcessRequest) (integration.ProcessResult, error) {
			captured = request
			return acceptedResult(request), nil
		}),
	))
	if err != nil {
		t.Fatal(err)
	}

	result, err := service.Submit(context.Background(), testHL7("CTRL1"))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	if result.Receipt == nil || result.Receipt.Status != integration.ReceiptStatusAccepted {
		t.Fatalf("missing accepted receipt: %#v", result)
	}
	if captured.Mode != integration.ExecutionModeProduction || captured.IntegrationRevision != binding.IntegrationRevision {
		t.Fatalf("wrong execution identity: %#v", captured)
	}
	if captured.Envelope.SourceID != binding.SourceID || captured.Envelope.Format != events.FormatHL7v2 ||
		captured.Envelope.Classification != binding.Classification {
		t.Fatalf("wrong envelope binding: %#v", captured.Envelope)
	}
	if captured.Security.Principal.Kind != integration.PrincipalKindService ||
		captured.Security.Principal.AuthMethod != "mllp-allowlist" ||
		len(captured.Security.Principal.Roles) != 1 || captured.Security.Principal.Roles[0] != SubmitRole {
		t.Fatalf("wrong principal: %#v", captured.Security.Principal)
	}
}

func TestServiceFailsClosedForLifecycleAndProcessorErrors(t *testing.T) {
	source := testSource(t)
	binding := testBinding(source)
	cases := []struct {
		name       string
		resolveErr error
		processErr error
		want       error
		mutate     func(*lifecycle.RunnableBinding)
		payload    []byte
	}{
		{name: "paused", resolveErr: lifecycle.ErrNotFound, want: ErrUnavailable},
		{name: "source mismatch", want: ErrUnavailable, mutate: func(v *lifecycle.RunnableBinding) { v.SourceID = "other" }},
		{name: "invalid message", processErr: processor.ErrInvalidSourceMessage, want: ErrInvalidMessage},
		{name: "idempotency conflict", processErr: processor.ErrIdempotencyConflict, want: ErrIdempotencyConflict},
		{name: "storage failure", processErr: errors.New("database unavailable"), want: ErrRetryable},
		{name: "empty payload", want: ErrInvalidMessage, payload: []byte{}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			current := binding
			if test.mutate != nil {
				test.mutate(&current)
			}
			service, err := NewService(testServiceConfig(source,
				resolverFunc(func(context.Context, string, string) (lifecycle.RunnableBinding, error) {
					return current, test.resolveErr
				}),
				processorFunc(func(_ context.Context, request integration.ProcessRequest) (integration.ProcessResult, error) {
					if test.processErr != nil {
						return integration.ProcessResult{}, test.processErr
					}
					return acceptedResult(request), nil
				}),
			))
			if err != nil {
				t.Fatal(err)
			}
			payload := test.payload
			if payload == nil {
				payload = testHL7("CTRL1")
			}
			_, err = service.Submit(context.Background(), payload)
			if !errors.Is(err, test.want) {
				t.Fatalf("got %v, want %v", err, test.want)
			}
		})
	}
}

func TestServiceProcessTimeoutIsRetryable(t *testing.T) {
	source := testSource(t)
	source.Timeouts.ProcessSeconds = 1
	digest, _ := source.semanticDigest()
	source.Digest = digest
	binding := testBinding(source)
	service, err := NewService(testServiceConfig(source,
		resolverFunc(func(context.Context, string, string) (lifecycle.RunnableBinding, error) { return binding, nil }),
		processorFunc(func(ctx context.Context, _ integration.ProcessRequest) (integration.ProcessResult, error) {
			<-ctx.Done()
			return integration.ProcessResult{}, ctx.Err()
		}),
	))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := service.Submit(ctx, testHL7("CTRL1")); !errors.Is(err, ErrRetryable) {
		t.Fatalf("got %v", err)
	}
}
