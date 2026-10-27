package main

import (
	"context"
	"log/slog"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
)

func processorObservation(logger *slog.Logger, tracing *observability.Tracing) processor.Observer {
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
		// hashes it, so logs and spans join on the header without carrying its
		// text (or MSH-10) into telemetry.
		if initial.CorrelationID != "" {
			fields = append(fields, observability.F(observability.FieldCorrelationID, spanCorrelationID(initial.CorrelationID)))
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
