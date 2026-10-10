package processor

import (
	"context"
	"testing"
)

func TestProcessorObservationPropagatesContextAndCompletes(t *testing.T) {
	type contextKey struct{}
	fixture := newMessageProcessorFixture(t, strictExecutableProfileJSON(false), processorPublishedWorkflow, processorA01Message(true))
	var starts, finishes []Phase
	fixture.processor.SetObserver(func(ctx context.Context, initial Observation) (context.Context, func(Observation)) {
		starts = append(starts, initial.Phase)
		if initial.Phase == PhaseWorkflowPlan && ctx.Value(contextKey{}) != PhaseAdmission {
			t.Error("plan did not inherit admission context")
		}
		return context.WithValue(ctx, contextKey{}, initial.Phase), func(result Observation) {
			finishes = append(finishes, result.Phase)
			if result.Failed {
				t.Error("successful operation reported as failed")
			}
			if result.CorrelationID != fixture.request.CorrelationID {
				t.Error("missing correlation")
			}
		}
	})
	if _, err := fixture.processor.Process(context.Background(), fixture.request); err != nil {
		t.Fatal(err)
	}
	if len(starts) != 2 || len(finishes) != 2 || finishes[0] != PhaseWorkflowPlan || finishes[1] != PhaseAdmission {
		t.Fatalf("starts %v finishes %v", starts, finishes)
	}
}

func TestProcessorObservationReportsRejectionOnce(t *testing.T) {
	fixture := newMessageProcessorFixture(t, strictExecutableProfileJSON(false), processorPublishedWorkflow, []byte("INVALID_PHI_SENTINEL"))
	completed := 0
	fixture.processor.SetObserver(func(ctx context.Context, initial Observation) (context.Context, func(Observation)) {
		return ctx, func(result Observation) {
			completed++
			if !result.Failed || result.ReceiptID != "" {
				t.Errorf("unexpected failed observation: %+v", result)
			}
		}
	})
	if _, err := fixture.processor.Process(context.Background(), fixture.request); err == nil {
		t.Fatal("expected parse rejection")
	}
	if completed != 1 {
		t.Fatalf("completed %d observations", completed)
	}
}
