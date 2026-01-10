package eventsourcing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"
)

// =============================================================================
// Saga Types and Status
// =============================================================================

// SagaStatus represents the current state of a saga execution.
type SagaStatus string

const (
	// SagaStatusPending indicates the saga hasn't started yet
	SagaStatusPending SagaStatus = "pending"

	// SagaStatusRunning indicates the saga is executing steps
	SagaStatusRunning SagaStatus = "running"

	// SagaStatusCompleted indicates all steps completed successfully
	SagaStatusCompleted SagaStatus = "completed"

	// SagaStatusCompensating indicates the saga is rolling back
	SagaStatusCompensating SagaStatus = "compensating"

	// SagaStatusCompensated indicates rollback completed
	SagaStatusCompensated SagaStatus = "compensated"

	// SagaStatusFailed indicates the saga failed and couldn't compensate
	SagaStatusFailed SagaStatus = "failed"
)

// StepStatus represents the state of an individual saga step.
type StepStatus string

const (
	StepStatusPending      StepStatus = "pending"
	StepStatusRunning      StepStatus = "running"
	StepStatusCompleted    StepStatus = "completed"
	StepStatusFailed       StepStatus = "failed"
	StepStatusCompensating StepStatus = "compensating"
	StepStatusCompensated  StepStatus = "compensated"
)

// =============================================================================
// Saga Definition
// =============================================================================

// SagaStep defines a single step in a saga with its action and compensation.
type SagaStep struct {
	// Name identifies this step (e.g., "create_patient", "assign_bed")
	Name string

	// Action is the forward operation to execute
	// Receives saga data and returns updated data or error
	Action func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error)

	// Compensation is the rollback operation if a later step fails
	// Receives the data that was returned by Action
	Compensation func(ctx context.Context, data map[string]interface{}) error

	// Timeout for this step (0 = use saga default)
	Timeout time.Duration

	// RetryCount for transient failures (0 = no retry)
	RetryCount int

	// RetryDelay between retry attempts
	RetryDelay time.Duration
}

// SagaDefinition defines a complete saga with all its steps.
type SagaDefinition struct {
	// Name identifies this saga type (e.g., "patient_admission")
	Name string

	// Steps in execution order
	Steps []SagaStep

	// DefaultTimeout for steps without explicit timeout
	DefaultTimeout time.Duration

	// OnComplete callback when saga completes successfully
	OnComplete func(ctx context.Context, sagaID string, data map[string]interface{})

	// OnCompensated callback when saga is fully compensated
	OnCompensated func(ctx context.Context, sagaID string, data map[string]interface{}, originalError error)

	// OnFailed callback when saga fails and can't recover
	OnFailed func(ctx context.Context, sagaID string, data map[string]interface{}, err error)
}

// =============================================================================
// Saga State (Execution Instance)
// =============================================================================

// SagaState represents the current state of a saga execution.
type SagaState struct {
	// ID uniquely identifies this saga instance
	ID string `json:"id"`

	// SagaName is the name of the saga definition
	SagaName string `json:"saga_name"`

	// Status is the current saga status
	Status SagaStatus `json:"status"`

	// CurrentStep is the index of the step being executed (or last executed)
	CurrentStep int `json:"current_step"`

	// StepStates tracks the status of each step
	StepStates []StepState `json:"step_states"`

	// Data is the saga's working data, passed between steps
	Data map[string]interface{} `json:"data"`

	// Error message if the saga failed
	Error string `json:"error,omitempty"`

	// StartedAt is when the saga started
	StartedAt time.Time `json:"started_at"`

	// CompletedAt is when the saga finished (success or failure)
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// LastUpdatedAt is the last state change
	LastUpdatedAt time.Time `json:"last_updated_at"`
}

// StepState tracks the execution state of a single step.
type StepState struct {
	Name        string                 `json:"name"`
	Status      StepStatus             `json:"status"`
	StartedAt   *time.Time             `json:"started_at,omitempty"`
	CompletedAt *time.Time             `json:"completed_at,omitempty"`
	Error       string                 `json:"error,omitempty"`
	RetryCount  int                    `json:"retry_count"`
	OutputData  map[string]interface{} `json:"output_data,omitempty"`
}

// =============================================================================
// Saga Store Interface
// =============================================================================

// SagaStore provides persistence for saga state.
type SagaStore interface {
	// SaveSaga persists a saga state
	SaveSaga(ctx context.Context, state *SagaState) error

	// GetSaga retrieves a saga by ID
	GetSaga(ctx context.Context, sagaID string) (*SagaState, error)

	// ListSagasByStatus returns sagas with the given status
	ListSagasByStatus(ctx context.Context, status SagaStatus, limit int) ([]*SagaState, error)

	// ListPendingSagas returns sagas that need processing
	// (status = pending, running, or compensating)
	ListPendingSagas(ctx context.Context, limit int) ([]*SagaState, error)

	// DeleteSaga removes a saga (for cleanup of old completed sagas)
	DeleteSaga(ctx context.Context, sagaID string) error
}

// =============================================================================
// Saga Orchestrator
// =============================================================================

// SagaOrchestrator manages saga execution.
type SagaOrchestrator struct {
	store       SagaStore
	definitions map[string]*SagaDefinition
	mu          sync.RWMutex
}

// NewSagaOrchestrator creates a new saga orchestrator.
func NewSagaOrchestrator(store SagaStore) *SagaOrchestrator {
	return &SagaOrchestrator{
		store:       store,
		definitions: make(map[string]*SagaDefinition),
	}
}

// RegisterSaga registers a saga definition.
func (o *SagaOrchestrator) RegisterSaga(definition *SagaDefinition) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.definitions[definition.Name] = definition
}

// GetDefinition returns a saga definition by name.
func (o *SagaOrchestrator) GetDefinition(name string) (*SagaDefinition, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()
	def, ok := o.definitions[name]
	return def, ok
}

// StartSaga begins a new saga execution.
func (o *SagaOrchestrator) StartSaga(ctx context.Context, sagaName string, sagaID string, initialData map[string]interface{}) (*SagaState, error) {
	def, ok := o.GetDefinition(sagaName)
	if !ok {
		return nil, fmt.Errorf("saga definition not found: %s", sagaName)
	}

	if initialData == nil {
		initialData = make(map[string]interface{})
	}

	now := time.Now()
	state := &SagaState{
		ID:            sagaID,
		SagaName:      sagaName,
		Status:        SagaStatusPending,
		CurrentStep:   0,
		StepStates:    make([]StepState, len(def.Steps)),
		Data:          initialData,
		StartedAt:     now,
		LastUpdatedAt: now,
	}

	// Initialize step states
	for i, step := range def.Steps {
		state.StepStates[i] = StepState{
			Name:   step.Name,
			Status: StepStatusPending,
		}
	}

	if err := o.store.SaveSaga(ctx, state); err != nil {
		return nil, fmt.Errorf("failed to save initial saga state: %w", err)
	}

	return state, nil
}

// ExecuteSaga runs a saga to completion (or failure).
// This is a synchronous execution - for async, use StartSaga + ProcessSaga separately.
func (o *SagaOrchestrator) ExecuteSaga(ctx context.Context, sagaName string, sagaID string, initialData map[string]interface{}) (*SagaState, error) {
	state, err := o.StartSaga(ctx, sagaName, sagaID, initialData)
	if err != nil {
		return nil, err
	}

	return o.ProcessSaga(ctx, state.ID)
}

// ProcessSaga continues processing a saga from its current state.
// Call this to resume a saga after restart or to process pending sagas.
func (o *SagaOrchestrator) ProcessSaga(ctx context.Context, sagaID string) (*SagaState, error) {
	state, err := o.store.GetSaga(ctx, sagaID)
	if err != nil {
		return nil, fmt.Errorf("failed to get saga state: %w", err)
	}
	if state == nil {
		return nil, fmt.Errorf("saga not found: %s", sagaID)
	}

	def, ok := o.GetDefinition(state.SagaName)
	if !ok {
		return nil, fmt.Errorf("saga definition not found: %s", state.SagaName)
	}

	switch state.Status {
	case SagaStatusPending, SagaStatusRunning:
		return o.executeForward(ctx, state, def)
	case SagaStatusCompensating:
		return o.executeCompensation(ctx, state, def)
	case SagaStatusCompleted, SagaStatusCompensated, SagaStatusFailed:
		// Already finished
		return state, nil
	default:
		return nil, fmt.Errorf("unknown saga status: %s", state.Status)
	}
}

// executeForward runs saga steps in forward order.
func (o *SagaOrchestrator) executeForward(ctx context.Context, state *SagaState, def *SagaDefinition) (*SagaState, error) {
	state.Status = SagaStatusRunning
	state.LastUpdatedAt = time.Now()

	for i := state.CurrentStep; i < len(def.Steps); i++ {
		step := def.Steps[i]
		state.CurrentStep = i

		// Update step state
		now := time.Now()
		state.StepStates[i].Status = StepStatusRunning
		state.StepStates[i].StartedAt = &now

		if err := o.store.SaveSaga(ctx, state); err != nil {
			return state, fmt.Errorf("failed to save saga state: %w", err)
		}

		// Execute step with retry logic
		var stepErr error
		var outputData map[string]interface{}
		maxRetries := step.RetryCount
		if maxRetries < 0 {
			maxRetries = 0
		}

		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				// Wait before retry
				delay := step.RetryDelay
				if delay == 0 {
					delay = time.Second
				}
				select {
				case <-ctx.Done():
					return state, ctx.Err()
				case <-time.After(delay):
				}
				state.StepStates[i].RetryCount = attempt
			}

			// Create step context with timeout
			stepCtx := ctx
			timeout := step.Timeout
			if timeout == 0 {
				timeout = def.DefaultTimeout
			}
			if timeout > 0 {
				var cancel context.CancelFunc
				stepCtx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}

			outputData, stepErr = step.Action(stepCtx, state.Data)
			if stepErr == nil {
				break
			}
		}

		completedAt := time.Now()
		state.StepStates[i].CompletedAt = &completedAt

		if stepErr != nil {
			// Step failed - begin compensation
			state.StepStates[i].Status = StepStatusFailed
			state.StepStates[i].Error = stepErr.Error()
			state.Error = fmt.Sprintf("step %s failed: %v", step.Name, stepErr)
			state.Status = SagaStatusCompensating
			state.LastUpdatedAt = time.Now()

			if err := o.store.SaveSaga(ctx, state); err != nil {
				return state, fmt.Errorf("failed to save saga state: %w", err)
			}

			return o.executeCompensation(ctx, state, def)
		}

		// Step succeeded
		state.StepStates[i].Status = StepStatusCompleted
		state.StepStates[i].OutputData = outputData

		// Merge output data into saga data
		if outputData != nil {
			for k, v := range outputData {
				state.Data[k] = v
			}
		}

		state.LastUpdatedAt = time.Now()
		if err := o.store.SaveSaga(ctx, state); err != nil {
			return state, fmt.Errorf("failed to save saga state: %w", err)
		}
	}

	// All steps completed successfully
	now := time.Now()
	state.Status = SagaStatusCompleted
	state.CompletedAt = &now
	state.LastUpdatedAt = now

	if err := o.store.SaveSaga(ctx, state); err != nil {
		return state, fmt.Errorf("failed to save saga state: %w", err)
	}

	// Call completion callback
	if def.OnComplete != nil {
		def.OnComplete(ctx, state.ID, state.Data)
	}

	return state, nil
}

// executeCompensation runs compensation in reverse order.
func (o *SagaOrchestrator) executeCompensation(ctx context.Context, state *SagaState, def *SagaDefinition) (*SagaState, error) {
	originalError := state.Error

	// Compensate completed steps in reverse order
	for i := state.CurrentStep; i >= 0; i-- {
		stepState := &state.StepStates[i]

		// Only compensate steps that completed successfully
		if stepState.Status != StepStatusCompleted {
			continue
		}

		step := def.Steps[i]
		if step.Compensation == nil {
			// No compensation defined for this step
			stepState.Status = StepStatusCompensated
			continue
		}

		stepState.Status = StepStatusCompensating
		state.LastUpdatedAt = time.Now()

		if err := o.store.SaveSaga(ctx, state); err != nil {
			return state, fmt.Errorf("failed to save saga state: %w", err)
		}

		// Execute compensation
		// Use the output data from when the step completed
		compensationData := state.Data
		if stepState.OutputData != nil {
			// Merge step's output data for compensation context
			compensationData = make(map[string]interface{})
			for k, v := range state.Data {
				compensationData[k] = v
			}
			for k, v := range stepState.OutputData {
				compensationData[k] = v
			}
		}

		if err := step.Compensation(ctx, compensationData); err != nil {
			// Compensation failed - this is bad, saga is in inconsistent state
			stepState.Status = StepStatusFailed
			stepState.Error = fmt.Sprintf("compensation failed: %v", err)
			state.Status = SagaStatusFailed
			state.Error = fmt.Sprintf("compensation of step %s failed: %v (original error: %s)",
				step.Name, err, originalError)

			now := time.Now()
			state.CompletedAt = &now
			state.LastUpdatedAt = now

			if err := o.store.SaveSaga(ctx, state); err != nil {
				return state, fmt.Errorf("failed to save saga state: %w", err)
			}

			if def.OnFailed != nil {
				def.OnFailed(ctx, state.ID, state.Data, errors.New(state.Error))
			}

			return state, fmt.Errorf("saga compensation failed: %s", state.Error)
		}

		stepState.Status = StepStatusCompensated
		state.LastUpdatedAt = time.Now()

		if err := o.store.SaveSaga(ctx, state); err != nil {
			return state, fmt.Errorf("failed to save saga state: %w", err)
		}
	}

	// All compensations completed
	now := time.Now()
	state.Status = SagaStatusCompensated
	state.CompletedAt = &now
	state.LastUpdatedAt = now

	if err := o.store.SaveSaga(ctx, state); err != nil {
		return state, fmt.Errorf("failed to save saga state: %w", err)
	}

	if def.OnCompensated != nil {
		def.OnCompensated(ctx, state.ID, state.Data, errors.New(originalError))
	}

	return state, nil
}

// ProcessPendingSagas finds and processes all pending sagas.
// Useful for recovery after a crash.
func (o *SagaOrchestrator) ProcessPendingSagas(ctx context.Context, limit int) ([]*SagaState, error) {
	pending, err := o.store.ListPendingSagas(ctx, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending sagas: %w", err)
	}

	var results []*SagaState
	for _, state := range pending {
		result, err := o.ProcessSaga(ctx, state.ID)
		if err != nil {
			// Log error but continue with other sagas
			result = state
			result.Error = err.Error()
		}
		results = append(results, result)
	}

	return results, nil
}

// GetSaga retrieves the current state of a saga.
func (o *SagaOrchestrator) GetSaga(ctx context.Context, sagaID string) (*SagaState, error) {
	return o.store.GetSaga(ctx, sagaID)
}

// =============================================================================
// In-Memory Saga Store (for testing)
// =============================================================================

// MemorySagaStore is an in-memory saga store for testing.
type MemorySagaStore struct {
	sagas map[string]*SagaState
	mu    sync.RWMutex
}

// NewMemorySagaStore creates a new in-memory saga store.
func NewMemorySagaStore() *MemorySagaStore {
	return &MemorySagaStore{
		sagas: make(map[string]*SagaState),
	}
}

// SaveSaga stores a saga state.
func (s *MemorySagaStore) SaveSaga(ctx context.Context, state *SagaState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Deep copy to avoid mutation issues
	data, _ := json.Marshal(state)
	var copy SagaState
	json.Unmarshal(data, &copy)
	s.sagas[state.ID] = &copy

	return nil
}

// GetSaga retrieves a saga by ID.
func (s *MemorySagaStore) GetSaga(ctx context.Context, sagaID string) (*SagaState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.sagas[sagaID]
	if !ok {
		return nil, nil
	}

	// Return a copy
	data, _ := json.Marshal(state)
	var copy SagaState
	json.Unmarshal(data, &copy)
	return &copy, nil
}

// ListSagasByStatus returns sagas with the given status.
func (s *MemorySagaStore) ListSagasByStatus(ctx context.Context, status SagaStatus, limit int) ([]*SagaState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*SagaState
	for _, state := range s.sagas {
		if state.Status == status {
			data, _ := json.Marshal(state)
			var copy SagaState
			json.Unmarshal(data, &copy)
			result = append(result, &copy)

			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}

	return result, nil
}

// ListPendingSagas returns sagas that need processing.
func (s *MemorySagaStore) ListPendingSagas(ctx context.Context, limit int) ([]*SagaState, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var result []*SagaState
	for _, state := range s.sagas {
		if state.Status == SagaStatusPending ||
			state.Status == SagaStatusRunning ||
			state.Status == SagaStatusCompensating {
			data, _ := json.Marshal(state)
			var copy SagaState
			json.Unmarshal(data, &copy)
			result = append(result, &copy)

			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}

	return result, nil
}

// DeleteSaga removes a saga.
func (s *MemorySagaStore) DeleteSaga(ctx context.Context, sagaID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.sagas, sagaID)
	return nil
}

// =============================================================================
// Saga Builder (fluent API for defining sagas)
// =============================================================================

// SagaBuilder provides a fluent API for building saga definitions.
type SagaBuilder struct {
	definition *SagaDefinition
}

// NewSaga creates a new saga builder.
func NewSaga(name string) *SagaBuilder {
	return &SagaBuilder{
		definition: &SagaDefinition{
			Name:           name,
			Steps:          make([]SagaStep, 0),
			DefaultTimeout: 30 * time.Second,
		},
	}
}

// WithTimeout sets the default timeout for steps.
func (b *SagaBuilder) WithTimeout(timeout time.Duration) *SagaBuilder {
	b.definition.DefaultTimeout = timeout
	return b
}

// Step adds a step to the saga.
func (b *SagaBuilder) Step(name string, action func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error)) *StepBuilder {
	step := SagaStep{
		Name:   name,
		Action: action,
	}
	b.definition.Steps = append(b.definition.Steps, step)

	return &StepBuilder{
		sagaBuilder: b,
		stepIndex:   len(b.definition.Steps) - 1,
	}
}

// OnComplete sets the completion callback.
func (b *SagaBuilder) OnComplete(callback func(ctx context.Context, sagaID string, data map[string]interface{})) *SagaBuilder {
	b.definition.OnComplete = callback
	return b
}

// OnCompensated sets the compensation callback.
func (b *SagaBuilder) OnCompensated(callback func(ctx context.Context, sagaID string, data map[string]interface{}, err error)) *SagaBuilder {
	b.definition.OnCompensated = callback
	return b
}

// OnFailed sets the failure callback.
func (b *SagaBuilder) OnFailed(callback func(ctx context.Context, sagaID string, data map[string]interface{}, err error)) *SagaBuilder {
	b.definition.OnFailed = callback
	return b
}

// Build returns the saga definition.
func (b *SagaBuilder) Build() *SagaDefinition {
	return b.definition
}

// StepBuilder provides a fluent API for configuring saga steps.
type StepBuilder struct {
	sagaBuilder *SagaBuilder
	stepIndex   int
}

// WithCompensation sets the compensation action for this step.
func (b *StepBuilder) WithCompensation(compensation func(ctx context.Context, data map[string]interface{}) error) *StepBuilder {
	b.sagaBuilder.definition.Steps[b.stepIndex].Compensation = compensation
	return b
}

// WithTimeout sets the timeout for this step.
func (b *StepBuilder) WithTimeout(timeout time.Duration) *StepBuilder {
	b.sagaBuilder.definition.Steps[b.stepIndex].Timeout = timeout
	return b
}

// WithRetry configures retry behavior for this step.
func (b *StepBuilder) WithRetry(count int, delay time.Duration) *StepBuilder {
	b.sagaBuilder.definition.Steps[b.stepIndex].RetryCount = count
	b.sagaBuilder.definition.Steps[b.stepIndex].RetryDelay = delay
	return b
}

// Step adds another step (continues the chain).
func (b *StepBuilder) Step(name string, action func(ctx context.Context, data map[string]interface{}) (map[string]interface{}, error)) *StepBuilder {
	return b.sagaBuilder.Step(name, action)
}

// OnComplete sets the completion callback (continues to saga builder).
func (b *StepBuilder) OnComplete(callback func(ctx context.Context, sagaID string, data map[string]interface{})) *SagaBuilder {
	return b.sagaBuilder.OnComplete(callback)
}

// OnCompensated sets the compensation callback.
func (b *StepBuilder) OnCompensated(callback func(ctx context.Context, sagaID string, data map[string]interface{}, err error)) *SagaBuilder {
	return b.sagaBuilder.OnCompensated(callback)
}

// OnFailed sets the failure callback.
func (b *StepBuilder) OnFailed(callback func(ctx context.Context, sagaID string, data map[string]interface{}, err error)) *SagaBuilder {
	return b.sagaBuilder.OnFailed(callback)
}

// Build returns the saga definition.
func (b *StepBuilder) Build() *SagaDefinition {
	return b.sagaBuilder.Build()
}
