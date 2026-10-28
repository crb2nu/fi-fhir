package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/config"
)

// envTelemetryCorrelationKey names the deployment secret spanCorrelationID keys
// its digest under. It belongs with the other serve secrets, never beside the
// collector or log archive whose contents it protects.
const envTelemetryCorrelationKey = "FI_FHIR_TELEMETRY_CORRELATION_KEY"

// minTelemetryCorrelationKeyBytes is the shortest key accepted: 256 bits of
// input, the HMAC-SHA256 block's security level.
const minTelemetryCorrelationKeyBytes = 32

// loadTelemetryCorrelationKey reads the correlation-digest key. Unset returns a
// nil key, which keeps the unkeyed digest; a key that is set but short is an
// error, because a misconfigured key should stop serve rather than run weakly.
func loadTelemetryCorrelationKey() ([]byte, error) {
	value := strings.TrimSpace(os.Getenv(envTelemetryCorrelationKey))
	if value == "" {
		return nil, nil
	}
	if len(value) < minTelemetryCorrelationKeyBytes {
		return nil, fmt.Errorf("%s must be at least %d bytes", envTelemetryCorrelationKey, minTelemetryCorrelationKeyBytes)
	}
	return []byte(value), nil
}

func processorObservation(logger *slog.Logger, tracing *observability.Tracing, correlationKey []byte) processor.Observer {
	return func(ctx context.Context, initial processor.Observation) (context.Context, func(processor.Observation)) {
		started := time.Now()
		name := "integration.admission"
		if initial.Phase == processor.PhaseWorkflowPlan {
			name = "integration.workflow.plan"
		}
		fields := []slog.Attr{
			observability.F(observability.FieldComponent, "message-processor"),
			observability.F(observability.FieldOperation, string(initial.Phase)),
			observability.F(observability.FieldMode, string(initial.Mode)),
		}
		// X-Correlation-ID can contain arbitrary caller text. spanCorrelationID
		// digests it under the deployment key, so logs and spans join on the
		// header without carrying its text (or MSH-10) into telemetry.
		if initial.CorrelationID != "" {
			fields = append(fields, observability.F(observability.FieldCorrelationID, spanCorrelationID(correlationKey, initial.CorrelationID)))
		}
		ctx, finish := tracing.Start(ctx, name, fields...)
		return ctx, func(result processor.Observation) {
			outcome, level := "accepted", slog.LevelDebug
			if result.Failed {
				outcome, level = "rejected", slog.LevelWarn
			}
			attrs := append(fields, observability.F(observability.FieldOutcome, outcome), observability.F(observability.FieldDurationMs, time.Since(started).Milliseconds()))
			if result.ReceiptID != "" {
				attrs = append(attrs, observability.F(observability.FieldReceiptID, result.ReceiptID))
			}
			if result.TraceID != "" {
				attrs = append(attrs, observability.F(observability.FieldLineageTraceID, result.TraceID))
			}
			logger.LogAttrs(ctx, level, "integration operation completed", attrs...)
			finish(result.Failed, attrs...)
		}
	}
}

func deliveryLogFields(result delivery.DispatchResult) []slog.Attr {
	fields := []slog.Attr{observability.F(observability.FieldComponent, "delivery-worker")}
	if result.ReceiptID != "" {
		fields = append(fields, observability.F(observability.FieldReceiptID, result.ReceiptID))
	}
	if result.TraceID != "" {
		fields = append(fields, observability.F(observability.FieldLineageTraceID, result.TraceID))
	}
	if result.AttemptID != "" {
		fields = append(fields, observability.F(observability.FieldDeliveryAttemptID, result.AttemptID))
	}
	return fields
}

// newServeTracing keeps the serve configuration boundary shared with its proof.
func newServeTracing(ctx context.Context, cfg *config.Config, tenantID string, logger *slog.Logger) (*observability.Tracing, error) {
	return observability.NewTracing(ctx, observability.TraceConfig{
		Enabled:     cfg.Observability.TracingEnabled,
		Endpoint:    cfg.Observability.TracingEndpoint,
		SampleRatio: cfg.Observability.TracingSampler,
		Version:     version,
		Logger:      logger,
		TenantID:    tenantID,
	})
}
