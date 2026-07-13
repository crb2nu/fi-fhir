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
	env            *cel.Env
	cache          map[string]cel.Program
	outputTypes    map[string]*cel.Type
	programOptions []cel.ProgramOption
	mu             sync.RWMutex
}

const maxPublishedWorkflowCELCost uint64 = 10_000

// NewCELEvaluator creates a new CEL evaluator with healthcare event schema.
func NewCELEvaluator() (*CELEvaluator, error) {
	return newCELEvaluator()
}

func newPublishedCELEvaluator() (*CELEvaluator, error) {
	return newCELEvaluator(cel.CostLimit(maxPublishedWorkflowCELCost))
}

func newCELEvaluator(programOptions ...cel.ProgramOption) (*CELEvaluator, error) {
	// Create CEL environment with dynamic types for flexibility
	env, err := cel.NewEnv(
		cel.Variable("event", cel.DynType),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL environment: %w", err)
	}

	return &CELEvaluator{
		env:            env,
		cache:          make(map[string]cel.Program),
		outputTypes:    make(map[string]*cel.Type),
		programOptions: append([]cel.ProgramOption(nil), programOptions...),
	}, nil
}

// Evaluate evaluates a CEL expression against an event.
// Returns true if the condition matches, false otherwise.
func (e *CELEvaluator) Evaluate(condition string, event interface{}) (bool, error) {
	out, err := e.evaluate(condition, event)
	if err != nil {
		return false, err
	}

	// Convert result to boolean. Legacy workflow evaluation intentionally keeps
	// its historical truthy behavior; published planning uses EvaluateBoolean.
	if out.Type() == types.BoolType {
		val, ok := out.Value().(bool)
		if !ok {
			return false, fmt.Errorf("CEL result type mismatch: expected bool, got %T", out.Value())
		}
		return val, nil
	}
	return isTruthy(out), nil
}

// EvaluateBoolean evaluates a condition and requires an actual CEL boolean.
// Published executable workflows use this fail-closed contract so a string,
// number, or object cannot become an accidentally truthy route match.
func (e *CELEvaluator) EvaluateBoolean(condition string, event interface{}) (bool, error) {
	out, err := e.evaluate(condition, event)
	if err != nil {
		return false, err
	}
	if out.Type() != types.BoolType {
		return false, fmt.Errorf("CEL result must be boolean")
	}
	val, ok := out.Value().(bool)
	if !ok {
		return false, fmt.Errorf("CEL result must be boolean")
	}
	return val, nil
}

func (e *CELEvaluator) evaluate(condition string, event interface{}) (ref.Val, error) {
	if condition == "" {
		return types.True, nil
	}

	// Get or compile the program
	prg, err := e.getProgram(condition)
	if err != nil {
		return nil, fmt.Errorf("failed to compile condition: %w", err)
	}

	// Convert event to CEL-compatible format
	eventMap, err := eventToMap(event)
	if err != nil {
		return nil, fmt.Errorf("failed to convert event: %w", err)
	}

	// Evaluate the expression
	out, _, err := prg.Eval(map[string]interface{}{
		"event": eventMap,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to evaluate condition: %w", err)
	}
	return out, nil
}

// Compile validates that a CEL expression is syntactically correct.
// It returns an error if the expression cannot be compiled.
func (e *CELEvaluator) Compile(condition string) error {
	_, err := e.getProgram(condition)
	return err
}

// CompileBoolean validates syntax and rejects statically non-Boolean results.
// Dynamic results remain valid and are checked by EvaluateBoolean at runtime.
func (e *CELEvaluator) CompileBoolean(condition string) error {
	_, outputType, err := e.getProgramAndType(condition)
	if err != nil {
		return err
	}
	if !outputType.IsExactType(cel.BoolType) && !outputType.IsExactType(cel.DynType) {
		return fmt.Errorf("CEL result must be boolean")
	}
	return nil
}

// getProgram returns a cached program or compiles a new one.
func (e *CELEvaluator) getProgram(condition string) (cel.Program, error) {
	program, _, err := e.getProgramAndType(condition)
	return program, err
}

func (e *CELEvaluator) getProgramAndType(condition string) (cel.Program, *cel.Type, error) {
	e.mu.RLock()
	if prg, ok := e.cache[condition]; ok {
		outputType := e.outputTypes[condition]
		e.mu.RUnlock()
		return prg, outputType, nil
	}
	e.mu.RUnlock()

	// Compile the expression
	ast, issues := e.env.Compile(condition)
	if issues != nil && issues.Err() != nil {
		return nil, nil, issues.Err()
	}

	prg, err := e.env.Program(ast, e.programOptions...)
	if err != nil {
		return nil, nil, err
	}
	outputType := ast.OutputType()

	// Cache the program
	e.mu.Lock()
	if cached, ok := e.cache[condition]; ok {
		prg = cached
		outputType = e.outputTypes[condition]
	} else {
		e.cache[condition] = prg
		e.outputTypes[condition] = outputType
	}
	e.mu.Unlock()

	return prg, outputType, nil
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
