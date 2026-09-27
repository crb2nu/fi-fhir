package workflow

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestParseExecAllowlist(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want []string
	}{
		{name: "empty", raw: "", want: nil},
		{name: "blank entries", raw: " , ,", want: nil},
		{name: "exact paths", raw: "/usr/bin/true, /opt/fi-fhir/bin/notify", want: []string{"/usr/bin/true", "/opt/fi-fhir/bin/notify"}},
		{name: "duplicates collapse", raw: "/usr/bin/true,/usr/bin/true", want: []string{"/usr/bin/true"}},
		{name: "relative dropped", raw: "true,bin/sh,/usr/bin/true", want: []string{"/usr/bin/true"}},
		{name: "unclean dropped", raw: "/usr/bin/../bin/sh,/usr//bin/sh", want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ParseExecAllowlist(tt.raw); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("ParseExecAllowlist(%q) = %#v, want %#v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestExecCommandAllowedRequiresBothLists(t *testing.T) {
	tests := []struct {
		name       string
		command    string
		deployment []string
		action     string
		want       bool
	}{
		{name: "empty deployment refuses", command: "/bin/echo", deployment: nil, action: "/bin/echo", want: false},
		{name: "yaml cannot widen", command: "/bin/sh", deployment: []string{"/bin/echo"}, action: "/bin/sh,/bin/echo", want: false},
		{name: "yaml can narrow", command: "/bin/echo", deployment: []string{"/bin/echo", "/bin/true"}, action: "/bin/true", want: false},
		{name: "in both", command: "/bin/echo", deployment: []string{"/bin/echo"}, action: " /bin/echo ", want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := execCommandAllowed(tt.command, tt.deployment, tt.action); got != tt.want {
				t.Fatalf("execCommandAllowed = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestEngineExecAllowlistIsDeploymentOwned is the regression test for
// SEC-2026-09-27-1: a workflow whose YAML lists a command in its own
// `allowlist` cannot run it unless FI_FHIR_WORKFLOW_EXEC_ALLOWLIST also does.
func TestEngineExecAllowlistIsDeploymentOwned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture not supported on windows")
	}
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	script := filepath.Join(dir, "touch.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n: > \"$1\"\n"), 0o700); err != nil {
		t.Fatalf("write script: %v", err)
	}

	wf, err := ParseWorkflow([]byte(`name: exec-widen
version: "1.0"
routes:
  - name: r
    filter:
      event_type: TEST
    actions:
      - type: exec
        command: ` + script + `
        allowlist: ` + script + `
        args: '["` + marker + `"]'
        stdin: none
        timeout: 5s
`))
	if err != nil {
		t.Fatalf("ParseWorkflow: %v", err)
	}
	event := map[string]interface{}{"type": "TEST", "source": "exec-policy"}

	t.Setenv(EnvExecAllowlist, "")
	engine, err := NewEngine(wf)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	result := engine.ProcessWithContext(context.Background(), event)
	errs := result.AllErrors()
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), EnvExecAllowlist) {
		t.Fatalf("expected exec to be refused by the empty deployment allowlist, got %v", errs)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("exec ran although the deployment allowlist is empty (stat err %v)", statErr)
	}

	// Positive control: the same YAML runs once the deployment names the script.
	t.Setenv(EnvExecAllowlist, script)
	engine, err = NewEngine(wf)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if errs := engine.ProcessWithContext(ctx, event).AllErrors(); len(errs) != 0 {
		t.Fatalf("expected exec to run with the deployment allowlist, got %v", errs)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("exec did not run: %v", statErr)
	}

	// SetExecAllowlist replaces the environment-derived list.
	engine.SetExecAllowlist(nil)
	if got := engine.ExecAllowlist(); len(got) != 0 {
		t.Fatalf("SetExecAllowlist(nil) left %v", got)
	}
}
