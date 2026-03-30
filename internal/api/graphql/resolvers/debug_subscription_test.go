package resolvers

import (
	"context"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
)

func TestDebugStepEventSubscriptionReceivesPausedSteps(t *testing.T) {
	resolver := NewResolver()
	mutations := &mutationResolver{resolver}
	subscriptions := &subscriptionResolver{resolver}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	session, err := mutations.StartDebugSession(ctx, model.StartDebugSessionInput{
		WorkflowYaml: `name: debug_sub
version: "1.0"
routes:
  - name: first_route
    filter:
      event_type: TEST
    actions:
      - type: log
        message: "hello"
`,
		Event: map[string]interface{}{
			"type":   "TEST",
			"source": "subscription-test",
		},
	})
	if err != nil {
		t.Fatalf("StartDebugSession failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		current, queryErr := (&queryResolver{resolver}).DebugSession(ctx, session.ID)
		if queryErr != nil {
			t.Fatalf("DebugSession query failed: %v", queryErr)
		}
		if current != nil && len(current.Steps) > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected session to pause with at least one step")
		}
		time.Sleep(10 * time.Millisecond)
	}

	ch, err := subscriptions.DebugStepEvent(ctx, session.ID)
	if err != nil {
		t.Fatalf("DebugStepEvent subscription failed: %v", err)
	}

	step, err := mutations.DebugStep(ctx, session.ID)
	if err != nil {
		t.Fatalf("DebugStep failed: %v", err)
	}
	if step == nil {
		t.Fatalf("expected second paused step")
	}

	select {
	case eventStep := <-ch:
		if eventStep == nil {
			t.Fatalf("expected debug step event payload")
		}
		if eventStep.StepNumber != step.StepNumber {
			t.Fatalf("expected step number %d, got %d", step.StepNumber, eventStep.StepNumber)
		}
		if eventStep.Name != step.Name {
			t.Fatalf("expected step name %q, got %q", step.Name, eventStep.Name)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for debug step subscription event")
	}
}
