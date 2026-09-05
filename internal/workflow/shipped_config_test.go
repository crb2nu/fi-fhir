package workflow

import (
	"os"
	"path/filepath"
	"testing"
)

// TestShippedADTConfigRoutesWhatItClaims guards `configs/adt-workflow.yaml`.
//
// That file is not a sample nobody runs: `docker-compose.yaml` mounts it as
// FI_FHIR_WORKFLOW_CONFIG_PATH, and the getting-started guide passes it to
// `fi-fhir serve --workflow`, so it is the workflow the default development
// stack executes.
//
// Before slice 4.4b it was broken in three independent ways, none of which
// produced an error anyone would notice:
//
//   - `condition:` sat at the route's top level instead of under `filter:`.
//     An unrecognized key is silently dropped, leaving a zero-value Filter,
//     and a zero-value Filter matches every event. All five routes fired on
//     every event, including events that were not ADT at all.
//   - the actions were `type: emit`, which is not a registered action type, so
//     every action failed with `unknown action type: emit`.
//   - the templates used `{{ event.type }}`, which is not the engine's form,
//     so they emitted their own source text.
//
// The three failures partly masked each other, and the one signal that should
// have caught them — the route-match count — was never asserted anywhere. This
// test asserts the routing behaviour the file's comments claim, so a future
// edit that silently reverts to match-everything fails here instead of in a
// user's development stack.
func TestShippedADTConfigRoutesWhatItClaims(t *testing.T) {
	path := filepath.Join("..", "..", "configs", "adt-workflow.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}

	wf, err := ParseWorkflow(data)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}

	engine, err := NewEngine(wf)
	if err != nil {
		t.Fatalf("building engine: %v", err)
	}

	cases := []struct {
		name        string
		event       map[string]interface{}
		wantRoutes  []string
		description string
	}{
		{
			name: "a handled ADT type matches exactly its own route",
			event: map[string]interface{}{
				"type": "patient_admit", "source": "epic_adt",
				"patient": map[string]interface{}{"mrn": "12345"},
			},
			wantRoutes:  []string{"patient-admit"},
			description: "the catch-all must exclude types the named routes handle",
		},
		{
			name: "an unhandled patient_* type reaches only the catch-all",
			event: map[string]interface{}{
				"type": "patient_merge", "source": "epic_adt",
				"patient": map[string]interface{}{"mrn": "55555"},
			},
			wantRoutes:  []string{"adt-other"},
			description: "this is the catch-all's entire reason to exist",
		},
		{
			name: "a non-ADT event matches nothing",
			event: map[string]interface{}{
				"type": "lab_result", "source": "lab_system",
			},
			wantRoutes:  nil,
			description: "a zero-value Filter would match this; that was the original bug",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := engine.DryRun(tc.event)

			var matched []string
			for _, rr := range result.RouteResults {
				if rr.Matched {
					matched = append(matched, rr.RouteName)
				}
			}

			if len(matched) != len(tc.wantRoutes) {
				t.Fatalf("matched %v, want %v — %s", matched, tc.wantRoutes, tc.description)
			}
			for i, want := range tc.wantRoutes {
				if matched[i] != want {
					t.Errorf("match %d: got %q, want %q", i, matched[i], want)
				}
			}
		})
	}

	t.Run("every action type is registered", func(t *testing.T) {
		// `emit` was not, and the resulting per-event failure was invisible
		// beside a load-test summary that reported 100% achievement anyway.
		for _, route := range wf.Routes {
			for _, action := range route.Actions {
				if _, ok := engine.actions[action.Type]; !ok {
					t.Errorf("route %q uses unregistered action type %q",
						route.Name, action.Type)
				}
			}
		}
	})
}
