//nolint:gosec // Test file - G104 errors intentionally ignored in test setup
package eventsourcing

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

func TestSagaOrchestrator_ExecuteSaga_Success(t *testing.T) {
	store := NewMemorySagaStore()
	orchestrator := NewSagaOrchestrator(store)

	// Track step execution
	var step1Called, step2Called, step3Called bool
	var completeCalled bool

	// Define a simple 3-step saga
	saga := NewSaga("test_saga").
		Step("step1", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			step1Called = true
			return map[string]interface{}{"step1_output": "value1"}, nil
		}).
		Step("step2", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			step2Called = true
			// Verify we received step1's output
			if data["step1_output"] != "value1" {
				t.Error("step2 didn't receive step1 output")
			}
			return map[string]interface{}{"step2_output": "value2"}, nil
		}).
		Step("step3", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			step3Called = true
			return nil, nil
		}).
		OnComplete(func(ctx context.Context, sagaID string, data map[string]interface{}) {
			completeCalled = true
		}).
		Build()

	orchestrator.RegisterSaga(saga)

	ctx := context.Background()
	state, err := orchestrator.ExecuteSaga(ctx, "test_saga", "saga-001", map[string]interface{}{
		"initial": "data",
	})

	if err != nil {
		t.Fatalf("ExecuteSaga failed: %v", err)
	}

	// Verify all steps were called
	if !step1Called || !step2Called || !step3Called {
		t.Error("Not all steps were called")
	}

	// Verify completion callback
	if !completeCalled {
		t.Error("OnComplete callback not called")
	}

	// Verify final state
	if state.Status != SagaStatusCompleted {
		t.Errorf("Status = %s, want completed", state.Status)
	}

	// Verify data was accumulated
	if state.Data["step1_output"] != "value1" {
		t.Error("step1_output not in final data")
	}
	if state.Data["step2_output"] != "value2" {
		t.Error("step2_output not in final data")
	}

	// Verify all step states
	for i, ss := range state.StepStates {
		if ss.Status != StepStatusCompleted {
			t.Errorf("Step %d status = %s, want completed", i, ss.Status)
		}
	}
}

func TestSagaOrchestrator_ExecuteSaga_FailureAndCompensation(t *testing.T) {
	store := NewMemorySagaStore()
	orchestrator := NewSagaOrchestrator(store)

	var step1Compensated, step2Compensated bool
	var compensatedCalled bool
	var compensationError error

	saga := NewSaga("test_saga_fail").
		Step("step1", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{"step1": "done"}, nil
		}).
		WithCompensation(func(ctx context.Context, data map[string]interface{}) error {
			step1Compensated = true
			return nil
		}).
		Step("step2", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{"step2": "done"}, nil
		}).
		WithCompensation(func(ctx context.Context, data map[string]interface{}) error {
			step2Compensated = true
			return nil
		}).
		Step("step3", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			// This step fails
			return nil, errors.New("step3 intentional failure")
		}).
		OnCompensated(func(ctx context.Context, sagaID string, data map[string]interface{}, err error) {
			compensatedCalled = true
			compensationError = err
		}).
		Build()

	orchestrator.RegisterSaga(saga)

	ctx := context.Background()
	state, _ := orchestrator.ExecuteSaga(ctx, "test_saga_fail", "saga-002", nil)

	// Verify compensation was triggered
	if !step1Compensated || !step2Compensated {
		t.Error("Not all completed steps were compensated")
	}

	if !compensatedCalled {
		t.Error("OnCompensated callback not called")
	}

	if compensationError == nil || compensationError.Error() != "step step3 failed: step3 intentional failure" {
		t.Errorf("Unexpected compensation error: %v", compensationError)
	}

	// Verify final state
	if state.Status != SagaStatusCompensated {
		t.Errorf("Status = %s, want compensated", state.Status)
	}

	// Step 3 should be failed
	if state.StepStates[2].Status != StepStatusFailed {
		t.Errorf("Step 3 status = %s, want failed", state.StepStates[2].Status)
	}

	// Steps 1 and 2 should be compensated
	if state.StepStates[0].Status != StepStatusCompensated {
		t.Errorf("Step 1 status = %s, want compensated", state.StepStates[0].Status)
	}
	if state.StepStates[1].Status != StepStatusCompensated {
		t.Errorf("Step 2 status = %s, want compensated", state.StepStates[1].Status)
	}
}

func TestSagaOrchestrator_ExecuteSaga_CompensationFails(t *testing.T) {
	store := NewMemorySagaStore()
	orchestrator := NewSagaOrchestrator(store)

	var failedCalled bool

	saga := NewSaga("test_saga_comp_fail").
		Step("step1", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			return nil, nil
		}).
		WithCompensation(func(ctx context.Context, data map[string]interface{}) error {
			// Compensation also fails
			return errors.New("compensation failed")
		}).
		Step("step2", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			return nil, errors.New("step2 failed")
		}).
		OnFailed(func(ctx context.Context, sagaID string, data map[string]interface{}, err error) {
			failedCalled = true
		}).
		Build()

	orchestrator.RegisterSaga(saga)

	ctx := context.Background()
	state, err := orchestrator.ExecuteSaga(ctx, "test_saga_comp_fail", "saga-003", nil)

	// Should return error when compensation fails
	if err == nil {
		t.Error("Expected error when compensation fails")
	}

	if !failedCalled {
		t.Error("OnFailed callback not called")
	}

	if state.Status != SagaStatusFailed {
		t.Errorf("Status = %s, want failed", state.Status)
	}
}

func TestSagaOrchestrator_StepRetry(t *testing.T) {
	store := NewMemorySagaStore()
	orchestrator := NewSagaOrchestrator(store)

	var attemptCount int32

	saga := NewSaga("test_retry").
		Step("flaky_step", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			count := atomic.AddInt32(&attemptCount, 1)
			if count < 3 {
				return nil, errors.New("transient error")
			}
			return map[string]interface{}{"success": true}, nil
		}).
		WithRetry(5, 10*time.Millisecond).
		Build()

	orchestrator.RegisterSaga(saga)

	ctx := context.Background()
	state, err := orchestrator.ExecuteSaga(ctx, "test_retry", "saga-004", nil)

	if err != nil {
		t.Fatalf("ExecuteSaga failed: %v", err)
	}

	if state.Status != SagaStatusCompleted {
		t.Errorf("Status = %s, want completed", state.Status)
	}

	if attemptCount != 3 {
		t.Errorf("Attempt count = %d, want 3", attemptCount)
	}

	// Verify retry count was recorded
	if state.StepStates[0].RetryCount != 2 { // 0, 1, 2 = 3 attempts
		t.Errorf("RetryCount = %d, want 2", state.StepStates[0].RetryCount)
	}
}

func TestSagaOrchestrator_StepTimeout(t *testing.T) {
	store := NewMemorySagaStore()
	orchestrator := NewSagaOrchestrator(store)

	saga := NewSaga("test_timeout").
		Step("slow_step", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(5 * time.Second):
				return nil, nil
			}
		}).
		WithTimeout(100 * time.Millisecond).
		Build()

	orchestrator.RegisterSaga(saga)

	ctx := context.Background()
	state, _ := orchestrator.ExecuteSaga(ctx, "test_timeout", "saga-005", nil)

	// Should have failed due to timeout
	if state.Status != SagaStatusCompensated {
		t.Errorf("Status = %s, want compensated (timeout should trigger failure)", state.Status)
	}

	if state.StepStates[0].Status != StepStatusFailed {
		t.Errorf("Step status = %s, want failed", state.StepStates[0].Status)
	}
}

func TestSagaOrchestrator_Resume(t *testing.T) {
	store := NewMemorySagaStore()
	orchestrator := NewSagaOrchestrator(store)

	var step2Called bool

	saga := NewSaga("test_resume").
		Step("step1", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			return map[string]interface{}{"step1": "done"}, nil
		}).
		Step("step2", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			step2Called = true
			return nil, nil
		}).
		Build()

	orchestrator.RegisterSaga(saga)

	ctx := context.Background()

	// Start saga but don't execute
	state, _ := orchestrator.StartSaga(ctx, "test_resume", "saga-006", nil)

	if state.Status != SagaStatusPending {
		t.Errorf("Initial status = %s, want pending", state.Status)
	}

	// Now process it
	state, err := orchestrator.ProcessSaga(ctx, "saga-006")
	if err != nil {
		t.Fatalf("ProcessSaga failed: %v", err)
	}

	if !step2Called {
		t.Error("step2 not called during resume")
	}

	if state.Status != SagaStatusCompleted {
		t.Errorf("Final status = %s, want completed", state.Status)
	}
}

func TestSagaOrchestrator_ProcessPendingSagas(t *testing.T) {
	store := NewMemorySagaStore()
	orchestrator := NewSagaOrchestrator(store)

	var completedCount int32

	saga := NewSaga("test_pending").
		Step("step1", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			atomic.AddInt32(&completedCount, 1)
			return nil, nil
		}).
		Build()

	orchestrator.RegisterSaga(saga)

	ctx := context.Background()

	// Start multiple sagas without executing
	for i := 0; i < 5; i++ {
		orchestrator.StartSaga(ctx, "test_pending", fmt.Sprintf("saga-%d", i), nil)
	}

	// Process all pending
	results, err := orchestrator.ProcessPendingSagas(ctx, 100)
	if err != nil {
		t.Fatalf("ProcessPendingSagas failed: %v", err)
	}

	if len(results) != 5 {
		t.Errorf("Processed %d sagas, want 5", len(results))
	}

	if completedCount != 5 {
		t.Errorf("Completed count = %d, want 5", completedCount)
	}

	for _, result := range results {
		if result.Status != SagaStatusCompleted {
			t.Errorf("Saga %s status = %s, want completed", result.ID, result.Status)
		}
	}
}

func TestMemorySagaStore(t *testing.T) {
	store := NewMemorySagaStore()
	ctx := context.Background()

	// Test Save and Get
	state := &SagaState{
		ID:       "test-saga-1",
		SagaName: "test",
		Status:   SagaStatusRunning,
		Data:     map[string]interface{}{"key": "value"},
	}

	if err := store.SaveSaga(ctx, state); err != nil {
		t.Fatalf("SaveSaga failed: %v", err)
	}

	got, err := store.GetSaga(ctx, "test-saga-1")
	if err != nil {
		t.Fatalf("GetSaga failed: %v", err)
	}
	if got == nil {
		t.Fatal("Expected saga, got nil")
	}
	if got.Status != SagaStatusRunning {
		t.Errorf("Status = %s, want running", got.Status)
	}

	// Test ListByStatus
	state2 := &SagaState{
		ID:       "test-saga-2",
		SagaName: "test",
		Status:   SagaStatusCompleted,
	}
	store.SaveSaga(ctx, state2)

	running, _ := store.ListSagasByStatus(ctx, SagaStatusRunning, 10)
	if len(running) != 1 {
		t.Errorf("Expected 1 running saga, got %d", len(running))
	}

	// Test ListPending
	pending, _ := store.ListPendingSagas(ctx, 10)
	if len(pending) != 1 { // Only running saga should be pending
		t.Errorf("Expected 1 pending saga, got %d", len(pending))
	}

	// Test Delete
	if err := store.DeleteSaga(ctx, "test-saga-1"); err != nil {
		t.Fatalf("DeleteSaga failed: %v", err)
	}

	got, _ = store.GetSaga(ctx, "test-saga-1")
	if got != nil {
		t.Error("Expected nil after delete")
	}
}

func TestSagaBuilder(t *testing.T) {
	saga := NewSaga("builder_test").
		WithTimeout(5*time.Second).
		Step("step1", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			return nil, nil
		}).
		WithCompensation(func(ctx context.Context, data map[string]interface{}) error {
			return nil
		}).
		WithTimeout(10*time.Second).
		WithRetry(3, time.Second).
		Step("step2", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			return nil, nil
		}).
		Build()

	if saga.Name != "builder_test" {
		t.Errorf("Name = %s, want builder_test", saga.Name)
	}

	if saga.DefaultTimeout != 5*time.Second {
		t.Errorf("DefaultTimeout = %v, want 5s", saga.DefaultTimeout)
	}

	if len(saga.Steps) != 2 {
		t.Errorf("Steps count = %d, want 2", len(saga.Steps))
	}

	// Check step1 config
	step1 := saga.Steps[0]
	if step1.Name != "step1" {
		t.Errorf("Step1 name = %s, want step1", step1.Name)
	}
	if step1.Compensation == nil {
		t.Error("Step1 should have compensation")
	}
	if step1.Timeout != 10*time.Second {
		t.Errorf("Step1 timeout = %v, want 10s", step1.Timeout)
	}
	if step1.RetryCount != 3 {
		t.Errorf("Step1 retry count = %d, want 3", step1.RetryCount)
	}
}

// Test a realistic healthcare scenario
func TestSagaOrchestrator_PatientAdmissionScenario(t *testing.T) {
	store := NewMemorySagaStore()
	orchestrator := NewSagaOrchestrator(store)

	// Track resources
	var patientCreated, bedAssigned, encounterCreated, billingNotified bool
	var assignedBed string

	saga := NewSaga("patient_admission").
		Step("create_patient", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			mrn := data["mrn"].(string)
			patientCreated = true
			return map[string]interface{}{
				"patient_id": "PAT-" + mrn,
			}, nil
		}).
		WithCompensation(func(ctx context.Context, data map[string]interface{}) error {
			patientCreated = false // "delete" patient
			return nil
		}).
		Step("assign_bed", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			bedAssigned = true
			assignedBed = "BED-101"
			return map[string]interface{}{
				"bed_id": assignedBed,
			}, nil
		}).
		WithCompensation(func(ctx context.Context, data map[string]interface{}) error {
			bedAssigned = false
			assignedBed = ""
			return nil
		}).
		Step("create_encounter", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			encounterCreated = true
			return map[string]interface{}{
				"encounter_id": "ENC-001",
			}, nil
		}).
		WithCompensation(func(ctx context.Context, data map[string]interface{}) error {
			encounterCreated = false
			return nil
		}).
		Step("notify_billing", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			billingNotified = true
			return nil, nil
		}).
		Build()

	orchestrator.RegisterSaga(saga)

	ctx := context.Background()
	state, err := orchestrator.ExecuteSaga(ctx, "patient_admission", "admission-001", map[string]interface{}{
		"mrn":  "12345",
		"name": "John Doe",
	})

	if err != nil {
		t.Fatalf("Patient admission saga failed: %v", err)
	}

	// Verify all steps completed
	if !patientCreated || !bedAssigned || !encounterCreated || !billingNotified {
		t.Error("Not all admission steps completed")
	}

	if state.Status != SagaStatusCompleted {
		t.Errorf("Status = %s, want completed", state.Status)
	}

	// Verify accumulated data
	if state.Data["patient_id"] != "PAT-12345" {
		t.Errorf("patient_id = %v, want PAT-12345", state.Data["patient_id"])
	}
	if state.Data["bed_id"] != "BED-101" {
		t.Errorf("bed_id = %v, want BED-101", state.Data["bed_id"])
	}
	if state.Data["encounter_id"] != "ENC-001" {
		t.Errorf("encounter_id = %v, want ENC-001", state.Data["encounter_id"])
	}
}

// Test admission with billing failure (should rollback)
func TestSagaOrchestrator_PatientAdmissionFailure(t *testing.T) {
	store := NewMemorySagaStore()
	orchestrator := NewSagaOrchestrator(store)

	var patientCreated, bedAssigned, encounterCreated bool

	saga := NewSaga("patient_admission_fail").
		Step("create_patient", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			patientCreated = true
			return nil, nil
		}).
		WithCompensation(func(ctx context.Context, data map[string]interface{}) error {
			patientCreated = false
			return nil
		}).
		Step("assign_bed", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			bedAssigned = true
			return nil, nil
		}).
		WithCompensation(func(ctx context.Context, data map[string]interface{}) error {
			bedAssigned = false
			return nil
		}).
		Step("create_encounter", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			encounterCreated = true
			return nil, nil
		}).
		WithCompensation(func(ctx context.Context, data map[string]interface{}) error {
			encounterCreated = false
			return nil
		}).
		Step("notify_billing", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			// Billing system unavailable
			return nil, errors.New("billing system unavailable")
		}).
		Build()

	orchestrator.RegisterSaga(saga)

	ctx := context.Background()
	state, _ := orchestrator.ExecuteSaga(ctx, "patient_admission_fail", "admission-002", nil)

	// All resources should be rolled back
	if patientCreated || bedAssigned || encounterCreated {
		t.Error("Resources should be cleaned up after failure")
	}

	if state.Status != SagaStatusCompensated {
		t.Errorf("Status = %s, want compensated", state.Status)
	}
}

func TestSagaNoCompensation(t *testing.T) {
	store := NewMemorySagaStore()
	orchestrator := NewSagaOrchestrator(store)

	saga := NewSaga("no_compensation").
		Step("step1", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			return nil, nil
		}).
		// No compensation for step1
		Step("step2", func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error) {
			return nil, errors.New("step2 failed")
		}).
		Build()

	orchestrator.RegisterSaga(saga)

	ctx := context.Background()
	state, _ := orchestrator.ExecuteSaga(ctx, "no_compensation", "saga-nc", nil)

	// Should complete compensation even without compensation handlers
	if state.Status != SagaStatusCompensated {
		t.Errorf("Status = %s, want compensated", state.Status)
	}
}
