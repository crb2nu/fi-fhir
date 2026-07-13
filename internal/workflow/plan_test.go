package workflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
)

func TestPlannerProducesDeterministicSideEffectFreePlan(t *testing.T) {
	trapPath := filepath.Join(t.TempDir(), "must-not-exist")
	event := map[string]interface{}{
		"type":   "patient_admit",
		"source": "adt-east",
		"patient": map[string]interface{}{
			"mrn": "MRN-1",
		},
	}
	definition := &Workflow{
		Name: "preview",
		Routes: []Route{
			{
				Name: "matched",
				Filter: Filter{
					EventType: StringOrSlice{"patient_admit"},
					Source:    StringOrSlice{"adt-east"},
					Condition: `event.patient.mrn == "MRN-1"`,
				},
				Transforms: []Transform{{SetField: "patient.mrn = MUTATED"}, {ExplainWarnings: &ExplainWarningsConfig{Model: "must-not-call"}}},
				Actions: []Action{
					{ID: "send", Type: "file", Destination: "archive", Config: map[string]string{"path": trapPath, "secret": "ACTION-CONFIG-SENTINEL"}},
					{ID: "notify", Type: "exec", Destination: "notifier", Config: map[string]string{"command": "touch " + trapPath}},
				},
			},
			{
				Name:       "unmatched",
				Filter:     Filter{EventType: StringOrSlice{"lab_result"}},
				Transforms: []Transform{{SetField: "mutated = true"}},
				Actions:    []Action{{ID: "ignored", Type: "log", Config: map[string]string{"message": "ACTION-CONFIG-SENTINEL"}}},
			},
		},
	}

	planner, err := NewPlanner(definition)
	if err != nil {
		t.Fatalf("NewPlanner() error = %v", err)
	}
	definition.Routes[0].Name = "mutated-after-construction"
	definition.Routes[0].Filter.EventType[0] = "lab_result"
	definition.Routes[0].Actions[0].ID = "mutated-after-construction"
	definition.Routes[0].Actions[0].Config["secret"] = "MUTATED-CONFIG-SENTINEL"
	first, err := planner.Plan(event)
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	second, err := planner.Plan(event)
	if err != nil {
		t.Fatalf("Plan() second error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("Plan() is nondeterministic\nfirst:  %#v\nsecond: %#v", first, second)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("json.Marshal(plan) error = %v", err)
	}
	if json.Valid(encoded) == false {
		t.Fatalf("plan JSON is invalid: %s", encoded)
	}
	if contains := string(encoded); containsString(contains, "ACTION-CONFIG-SENTINEL") || containsString(contains, trapPath) || containsString(contains, "must-not-call") {
		t.Fatalf("plan exposed execution configuration: %s", encoded)
	}
	if _, err := os.Stat(trapPath); !os.IsNotExist(err) {
		t.Fatalf("planner executed side effect at %s: %v", trapPath, err)
	}
	if got := event["patient"].(map[string]interface{})["mrn"]; got != "MRN-1" {
		t.Fatalf("planner transformed caller event: %v", got)
	}

	if len(first.Routes) != 2 {
		t.Fatalf("routes = %d, want 2", len(first.Routes))
	}
	matched := first.Routes[0]
	if matched.Name != "matched" || !matched.Matched || matched.Skipped || matched.TransformCount != 2 {
		t.Fatalf("matched route = %#v", matched)
	}
	if got, want := matched.Actions, []ActionPlan{
		{ID: "send", Type: "file", DestinationArtifactID: "archive"},
		{ID: "notify", Type: "exec", DestinationArtifactID: "notifier"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("actions = %#v, want %#v", got, want)
	}
	unmatched := first.Routes[1]
	if unmatched.Name != "unmatched" || unmatched.Matched || unmatched.Skipped || unmatched.TransformCount != 0 || len(unmatched.Actions) != 0 {
		t.Fatalf("unmatched route = %#v", unmatched)
	}
}

func TestPlannerReturnsSafeDiagnosticForInvalidCEL(t *testing.T) {
	planner, err := NewPlanner(&Workflow{
		Name: "invalid-cel",
		Routes: []Route{{
			Name:    "broken",
			Filter:  Filter{EventType: StringOrSlice{"lab_result"}, Condition: `event.???`},
			Actions: []Action{{ID: "never", Type: "log"}},
		}},
	})
	if err != nil {
		t.Fatalf("NewPlanner() error = %v", err)
	}
	plan, err := planner.Plan(map[string]interface{}{"type": "patient_admit", "source": "adt-east"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if len(plan.Routes) != 1 || plan.Routes[0].Matched || plan.Routes[0].Skipped || len(plan.Routes[0].Actions) != 0 {
		t.Fatalf("invalid-CEL route = %#v", plan.Routes)
	}
	wantDiagnostic := PlanDiagnostic{Code: "INVALID_CEL", Path: "routes[0].filter.condition"}
	if !reflect.DeepEqual(plan.Diagnostics, []PlanDiagnostic{wantDiagnostic}) {
		t.Fatalf("diagnostics = %#v, want %#v", plan.Diagnostics, []PlanDiagnostic{wantDiagnostic})
	}
	if got := plan.Routes[0].DiagnosticCodes; !reflect.DeepEqual(got, []string{"INVALID_CEL"}) {
		t.Fatalf("route diagnostic codes = %#v", got)
	}
	encoded, err := json.Marshal(plan)
	if err != nil {
		t.Fatalf("json.Marshal(plan) error = %v", err)
	}
	for _, forbidden := range []string{"???", "failed to compile", "Syntax error", "ACTION-CONFIG-SENTINEL"} {
		if containsString(string(encoded), forbidden) {
			t.Fatalf("diagnostic leaked internal/raw text %q: %s", forbidden, encoded)
		}
	}
}

func TestPlannerRejectsNonBooleanCELResult(t *testing.T) {
	planner, err := NewPlanner(&Workflow{
		Name: "non-boolean-cel",
		Routes: []Route{{
			Name:    "broken",
			Filter:  Filter{Condition: `"non-empty"`},
			Actions: []Action{{ID: "never", Type: "log"}},
		}},
	})
	if err != nil {
		t.Fatalf("NewPlanner() error = %v", err)
	}
	plan, err := planner.Plan(map[string]interface{}{"type": "patient_admit"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Routes[0].Matched || len(plan.Routes[0].Actions) != 0 {
		t.Fatalf("non-boolean CEL result matched route: %#v", plan.Routes[0])
	}
	wantDiagnostic := PlanDiagnostic{Code: "INVALID_CEL", Path: "routes[0].filter.condition"}
	if !reflect.DeepEqual(plan.Diagnostics, []PlanDiagnostic{wantDiagnostic}) {
		t.Fatalf("diagnostics = %#v, want %#v", plan.Diagnostics, []PlanDiagnostic{wantDiagnostic})
	}
}

func TestPlannerRejectsStaticallyNonBooleanCELBeforeCoarseMismatch(t *testing.T) {
	planner, err := NewPlanner(&Workflow{
		Name: "non-boolean-cel",
		Routes: []Route{{
			Name: "broken",
			Filter: Filter{
				EventType: StringOrSlice{"lab_result"},
				Condition: `"non-empty"`,
			},
			Actions: []Action{{ID: "never", Type: "log"}},
		}},
	})
	if err != nil {
		t.Fatalf("NewPlanner() error = %v", err)
	}
	plan, err := planner.Plan(map[string]interface{}{"type": "patient_admit"})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	wantDiagnostic := PlanDiagnostic{Code: "INVALID_CEL", Path: "routes[0].filter.condition"}
	if !reflect.DeepEqual(plan.Diagnostics, []PlanDiagnostic{wantDiagnostic}) {
		t.Fatalf("diagnostics = %#v, want %#v", plan.Diagnostics, []PlanDiagnostic{wantDiagnostic})
	}
}

func TestPlannerBoundsCELRuntimeCost(t *testing.T) {
	values := make([]interface{}, 20_000)
	for index := range values {
		values[index] = int64(index)
	}
	planner, err := NewPlanner(&Workflow{
		Name: "bounded-cel",
		Routes: []Route{{
			Name:    "expensive",
			Filter:  Filter{Condition: `event.values.filter(value, value >= 0).size() == 20000`},
			Actions: []Action{{ID: "never", Type: "log"}},
		}},
	})
	if err != nil {
		t.Fatalf("NewPlanner() error = %v", err)
	}
	plan, err := planner.Plan(map[string]interface{}{"values": values})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if plan.Routes[0].Matched || len(plan.Routes[0].Actions) != 0 {
		t.Fatalf("over-budget CEL matched route: %#v", plan.Routes[0])
	}
	wantDiagnostic := PlanDiagnostic{Code: "INVALID_CEL", Path: "routes[0].filter.condition"}
	if !reflect.DeepEqual(plan.Diagnostics, []PlanDiagnostic{wantDiagnostic}) {
		t.Fatalf("diagnostics = %#v, want %#v", plan.Diagnostics, []PlanDiagnostic{wantDiagnostic})
	}
}

func TestSharedMatcherPreservesLegacyCoarseFilterShortCircuit(t *testing.T) {
	evaluator, err := NewCELEvaluator()
	if err != nil {
		t.Fatalf("NewCELEvaluator() error = %v", err)
	}
	matched, err := matchWorkflowFilter(
		map[string]interface{}{"type": "patient_admit"},
		Filter{EventType: StringOrSlice{"lab_result"}, Condition: `event.???`},
		evaluator,
	)
	if err != nil {
		t.Fatalf("matchWorkflowFilter() evaluated CEL after coarse mismatch: %v", err)
	}
	if matched {
		t.Fatal("matchWorkflowFilter() matched a different event type")
	}
}

func TestPlannerLegacyActionIDsAreStableAndReserved(t *testing.T) {
	legacy := &Workflow{
		Name: "legacy",
		Routes: []Route{{
			Name: "route",
			Actions: []Action{
				{Type: "log"},
				{Type: "webhook", Destination: "primary"},
			},
		}},
	}
	planner, err := NewPlanner(legacy)
	if err != nil {
		t.Fatalf("NewPlanner() error = %v", err)
	}
	plan, err := planner.Plan(map[string]interface{}{})
	if err != nil {
		t.Fatalf("Plan() error = %v", err)
	}
	if got, want := plan.Routes[0].Actions, []ActionPlan{
		{ID: "legacy-action-0001", Type: "log"},
		{ID: "legacy-action-0002", Type: "webhook", DestinationArtifactID: "primary"},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("legacy actions = %#v, want %#v", got, want)
	}

	for name, actions := range map[string][]Action{
		"duplicate explicit": {{ID: "same", Type: "log"}, {ID: "same", Type: "log"}},
		"reserved explicit":  {{ID: "legacy-action-0042", Type: "log"}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := NewPlanner(&Workflow{Name: "invalid", Routes: []Route{{Name: "route", Actions: actions}}})
			if err == nil {
				t.Fatal("NewPlanner() accepted ambiguous action identity")
			}
		})
	}
}

func TestNewPlannerRejectsIncompleteWorkflow(t *testing.T) {
	tests := map[string]*Workflow{
		"no routes": {Name: "empty"},
		"no actions": {
			Name:   "empty-route",
			Routes: []Route{{Name: "route"}},
		},
	}
	for name, workflow := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := NewPlanner(workflow); err == nil {
				t.Fatal("NewPlanner() accepted incomplete workflow")
			}
		})
	}
}

func TestPlannerIsConcurrentAndRaceSafe(t *testing.T) {
	published, err := ParsePublishedWorkflow([]byte(validPublishedWorkflowYAML))
	if err != nil {
		t.Fatalf("ParsePublishedWorkflow() error = %v", err)
	}
	planner, err := NewPlanner(published.Workflow())
	if err != nil {
		t.Fatalf("NewPlanner() error = %v", err)
	}
	event := map[string]interface{}{
		"type":   "patient_admit",
		"source": "adt-east",
		"patient": map[string]interface{}{
			"mrn": "MRN-1",
		},
	}
	want, err := planner.Plan(event)
	if err != nil {
		t.Fatalf("Plan() baseline error = %v", err)
	}

	const workers = 32
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			got, planErr := planner.Plan(event)
			if planErr != nil {
				errs <- planErr
				return
			}
			if !reflect.DeepEqual(got, want) {
				errs <- &planMismatchError{}
			}
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent Plan() = %v", err)
	}
}

type planMismatchError struct{}

func (*planMismatchError) Error() string { return "concurrent plan mismatch" }

func containsString(value, substring string) bool {
	for index := 0; index+len(substring) <= len(value); index++ {
		if value[index:index+len(substring)] == substring {
			return true
		}
	}
	return false
}
