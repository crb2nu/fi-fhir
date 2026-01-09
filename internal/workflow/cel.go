package workflow

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// CELEvaluator evaluates CEL expressions against events.
type CELEvaluator struct {
	env   *cel.Env
	cache map[string]cel.Program
	mu    sync.RWMutex
}

// NewCELEvaluator creates a new CEL evaluator with healthcare event schema.
func NewCELEvaluator() (*CELEvaluator, error) {
	// Create CEL environment with dynamic types for flexibility
	env, err := cel.NewEnv(
		cel.Variable("event", cel.DynType),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	return &CELEvaluator{
		env:   env,
		cache: make(map[string]cel.Program),
	}, nil
}

// Evaluate evaluates a CEL expression against an event.
// Returns true if the condition matches, false otherwise.
func (e *CELEvaluator) Evaluate(condition string, event interface{}) (bool, error) {
	if condition == "" {
		return true, nil
	}

	// Get or compile the program
	prg, err := e.getProgram(condition)
	if err != nil {
		return false, fmt.Errorf("failed to compile condition: %w", err)
	}

	// Convert event to CEL-compatible format
	eventMap, err := eventToMap(event)
	if err != nil {
		return false, fmt.Errorf("failed to convert event: %w", err)
	}

	// Evaluate the expression
	out, _, err := prg.Eval(map[string]interface{}{
		"event": eventMap,
	})
	if err != nil {
		return false, fmt.Errorf("failed to evaluate condition: %w", err)
	}

	// Convert result to boolean
	if out.Type() == types.BoolType {
		return out.Value().(bool), nil
	}

	// For non-boolean results, check if truthy
	return isTruthy(out), nil
}

// getProgram returns a cached program or compiles a new one.
func (e *CELEvaluator) getProgram(condition string) (cel.Program, error) {
	e.mu.RLock()
	if prg, ok := e.cache[condition]; ok {
		e.mu.RUnlock()
		return prg, nil
	}
	e.mu.RUnlock()

	// Compile the expression
	ast, issues := e.env.Compile(condition)
	if issues != nil && issues.Err() != nil {
		return nil, issues.Err()
	}

	prg, err := e.env.Program(ast)
	if err != nil {
		return nil, err
	}

	// Cache the program
	e.mu.Lock()
	e.cache[condition] = prg
	e.mu.Unlock()

	return prg, nil
}

// eventToMap converts an event to a map[string]interface{} for CEL evaluation.
func eventToMap(event interface{}) (map[string]interface{}, error) {
	// If already a map, return it
	if m, ok := event.(map[string]interface{}); ok {
		return m, nil
	}

	// Convert via JSON for struct types
	data, err := json.Marshal(event)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// isTruthy checks if a CEL value is truthy.
func isTruthy(val ref.Val) bool {
	switch v := val.Value().(type) {
	case bool:
		return v
	case int64:
		return v != 0
	case float64:
		return v != 0
	case string:
		return v != ""
	case nil:
		return false
	default:
		return true
	}
}
