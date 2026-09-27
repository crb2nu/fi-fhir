package workflow

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

func TestParseDebugActionAllowlist(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		wantAllowed []string
		wantRefused []string
	}{
		{name: "empty stubs everything", raw: ""},
		{name: "names normalised and sorted", raw: " Webhook ,log,log", wantAllowed: []string{"log", "webhook"}},
		{name: "exec is refused", raw: "log,exec,EXEC", wantAllowed: []string{"log"}, wantRefused: []string{"exec"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, refused := ParseDebugActionAllowlist(tt.raw)
			if !reflect.DeepEqual(allowed, tt.wantAllowed) || !reflect.DeepEqual(refused, tt.wantRefused) {
				t.Fatalf("ParseDebugActionAllowlist(%q) = %v, %v; want %v, %v", tt.raw, allowed, refused, tt.wantAllowed, tt.wantRefused)
			}
		})
	}
}

func TestNewDebugEngineStubsEverythingByDefault(t *testing.T) {
	engine, err := NewDebugEngine(&Workflow{Name: "stubs"}, nil)
	if err != nil {
		t.Fatalf("NewDebugEngine: %v", err)
	}
	if !engine.IsDebugEngine() {
		t.Fatal("expected a debug engine")
	}
	for _, name := range engine.RegisteredActionTypes() {
		if !engine.DebugStubbedAction(name) {
			t.Errorf("action %q is executable with an empty debug allowlist", name)
		}
	}

	production, err := NewEngine(&Workflow{Name: "prod"})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	if production.IsDebugEngine() || production.DebugStubbedAction("webhook") {
		t.Fatal("a production engine must not report debug stubs")
	}
}

func TestNewDebugEngineNeverExecutesExec(t *testing.T) {
	engine, err := NewDebugEngine(&Workflow{Name: "exec"}, []string{"log", "exec"})
	if err != nil {
		t.Fatalf("NewDebugEngine: %v", err)
	}
	if engine.DebugStubbedAction("log") {
		t.Error("log is allowlisted and should execute")
	}
	if !engine.DebugStubbedAction("exec") {
		t.Error("exec must always be stubbed in the debugger")
	}
	if !engine.DebugStubbedAction("webhook") {
		t.Error("webhook is not allowlisted and should be stubbed")
	}
}

// TestDebugSessionRecordsStubbedActions is the regression test for
// SEC-2026-09-27-1 on the debugger path: a caller-supplied workflow with a
// webhook and an exec action — whose YAML allowlists the command, and with the
// deployment exec allowlist naming it too — runs to completion without either
// side effect, and each action step records what the action would have done.
func TestDebugSessionRecordsStubbedActions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture not supported on windows")
	}
	var webhookHits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		webhookHits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	script := filepath.Join(dir, "touch.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n: > \""+marker+"\"\n"), 0o700); err != nil {
		t.Fatalf("write script: %v", err)
	}
	// Even a deployment that allows the script for production exec does not
	// let the debugger run it.
	t.Setenv(EnvExecAllowlist, script)

	wf, err := ParseWorkflow([]byte(`name: debug-stubs
version: "1.0"
routes:
  - name: notify
    filter:
      event_type: TEST
    actions:
      - type: webhook
        url: ` + server.URL + `/hook/{{.source}}
        auth_token: synthetic-debug-token
      - type: exec
        command: ` + script + `
        allowlist: ` + script + `
        stdin: none
`))
	if err != nil {
		t.Fatalf("ParseWorkflow: %v", err)
	}
	engine, err := NewDebugEngine(wf, nil)
	if err != nil {
		t.Fatalf("NewDebugEngine: %v", err)
	}
	session := NewDebugSession("stubs", engine)
	session.Start(context.Background(), map[string]interface{}{"type": "TEST", "source": "unit"})

	// route -> webhook -> exec, then completion.
	select {
	case <-session.stepCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for the route step")
	}
	webhookStep := session.Step()
	execStep := session.Step()
	if webhookStep == nil || execStep == nil {
		t.Fatalf("expected two action steps, got %v and %v", webhookStep, execStep)
	}
	if done := session.Step(); done != nil {
		t.Fatalf("expected completion, got %+v", done)
	}

	if hits := webhookHits.Load(); hits != 0 {
		t.Fatalf("debugger sent %d webhook requests", hits)
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("debugger ran the exec command (stat err %v)", statErr)
	}

	for _, step := range []*DebugStep{webhookStep, execStep} {
		if step.Kind != DebugStepAction {
			t.Fatalf("step %d kind = %s, want action", step.StepNumber, step.Kind)
		}
		if stubbed, _ := step.Variables[AttrActionStubbed].(bool); !stubbed {
			t.Errorf("step %s: %s = %v, want true", step.Name, AttrActionStubbed, step.Variables[AttrActionStubbed])
		}
		if _, ok := step.Variables[AttrActionInputs].(map[string]interface{}); !ok {
			t.Errorf("step %s: missing %s", step.Name, AttrActionInputs)
		}
	}
	inputs := webhookStep.Variables[AttrActionInputs].(map[string]interface{})
	if got, want := inputs["url"], server.URL+"/hook/unit"; got != want {
		t.Errorf("webhook url input = %v, want resolved template %v", got, want)
	}
	if got := inputs["auth_token"]; got != debugRedacted {
		t.Errorf("credential-named input rendered as %v, want %s", got, debugRedacted)
	}

	// The recorded trace shows the stub succeeded without side effects.
	snapshot := session.Snapshot()
	for _, step := range snapshot.Steps {
		if step.Kind != DebugStepAction {
			continue
		}
		if success, _ := step.Variables[AttrActionSuccess].(bool); !success {
			t.Errorf("recorded step %s: %s = %v, want true", step.Name, AttrActionSuccess, step.Variables[AttrActionSuccess])
		}
	}
}

func TestResolveDebugActionInputsBoundsAndRedacts(t *testing.T) {
	config := map[string]string{
		"url":      "https://user:pw@example.org/path",
		"password": "never-shown",
		"message":  string(make([]byte, debugInputMaxValueBytes+100)),
	}
	for i := 0; i < debugInputMaxKeys+5; i++ {
		config["k"+string(rune('a'+i%26))+string(rune('a'+i/26))] = "v"
	}
	inputs := resolveDebugActionInputs(config, map[string]interface{}{})
	if len(inputs) != debugInputMaxKeys+1 {
		t.Fatalf("expected %d keys plus the truncation marker, got %d", debugInputMaxKeys, len(inputs))
	}
	if _, ok := inputs["_truncated"]; !ok {
		t.Fatal("missing truncation marker")
	}
	full := resolveDebugActionInputs(map[string]string{
		"url": config["url"], "password": config["password"], "message": config["message"],
	}, nil)
	if got := full["url"]; got != "https://user:xxxxx@example.org/path" {
		t.Errorf("url password not redacted: %v", got)
	}
	if got := full["password"]; got != debugRedacted {
		t.Errorf("password = %v", got)
	}
	if got := full["message"].(string); len(got) > debugInputMaxValueBytes+32 {
		t.Errorf("message not bounded: %d bytes", len(got))
	}
}
