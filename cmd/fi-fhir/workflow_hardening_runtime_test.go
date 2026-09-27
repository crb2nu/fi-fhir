package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

func clearWorkflowHardeningEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		workflow.EnvExecAllowlist, workflow.EnvDebugActions,
		resolvers.EnvWorkflowDebugMaxSessions, resolvers.EnvWorkflowDebugSessionTTL,
		resolvers.EnvFHIRSubscriptionAllowedHosts, resolvers.EnvFHIRSubscriptionMaxClients,
	} {
		t.Setenv(key, "")
	}
}

func TestWorkflowHardeningDefaultsAreMostRestrictive(t *testing.T) {
	clearWorkflowHardeningEnv(t)

	exec, err := loadExecAllowlistFromEnv()
	if err != nil || len(exec) != 0 {
		t.Fatalf("exec allowlist = %v, %v; want empty", exec, err)
	}
	debug, err := loadWorkflowDebugPolicyFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(debug.ExecutableActions) != 0 || debug.MaxSessions != resolvers.DefaultWorkflowDebugMaxSessions || debug.SessionTTL != resolvers.DefaultWorkflowDebugSessionTTL {
		t.Fatalf("debug policy = %+v", debug)
	}
	subscriptions, err := loadFHIRSubscriptionPolicyFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(subscriptions.AllowedHosts) != 0 || subscriptions.MaxClients != resolvers.DefaultFHIRSubscriptionMaxClients {
		t.Fatalf("subscription policy = %+v", subscriptions)
	}
}

func TestWorkflowHardeningEnvParsing(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
		check   func(t *testing.T)
	}{
		{
			name:    "exec relative path fails startup",
			env:     map[string]string{workflow.EnvExecAllowlist: "/usr/bin/true,notify"},
			wantErr: workflow.EnvExecAllowlist,
		},
		{
			name: "exec absolute paths accepted",
			env:  map[string]string{workflow.EnvExecAllowlist: "/usr/bin/true, /opt/bin/notify"},
			check: func(t *testing.T) {
				got, _ := loadExecAllowlistFromEnv()
				if !reflect.DeepEqual(got, []string{"/usr/bin/true", "/opt/bin/notify"}) {
					t.Fatalf("got %v", got)
				}
			},
		},
		{
			name:    "debugger can never enable exec",
			env:     map[string]string{workflow.EnvDebugActions: "log,exec"},
			wantErr: "cannot enable exec",
		},
		{
			name:    "debugger unknown action fails startup",
			env:     map[string]string{workflow.EnvDebugActions: "log,webhok"},
			wantErr: "unknown action type",
		},
		{
			name: "debugger allowlist and bounds",
			env: map[string]string{
				workflow.EnvDebugActions:              "log, webhook",
				resolvers.EnvWorkflowDebugMaxSessions: "0",
				resolvers.EnvWorkflowDebugSessionTTL:  "90s",
			},
			check: func(t *testing.T) {
				policy, _ := loadWorkflowDebugPolicyFromEnv()
				if !reflect.DeepEqual(policy.ExecutableActions, []string{"log", "webhook"}) || policy.MaxSessions != 0 || policy.SessionTTL != 90*time.Second {
					t.Fatalf("policy = %+v", policy)
				}
			},
		},
		{
			name:    "negative max sessions",
			env:     map[string]string{resolvers.EnvWorkflowDebugMaxSessions: "-1"},
			wantErr: resolvers.EnvWorkflowDebugMaxSessions,
		},
		{
			name:    "bad ttl",
			env:     map[string]string{resolvers.EnvWorkflowDebugSessionTTL: "0s"},
			wantErr: resolvers.EnvWorkflowDebugSessionTTL,
		},
		{
			name:    "subscription host with scheme",
			env:     map[string]string{resolvers.EnvFHIRSubscriptionAllowedHosts: "https://fhir.example.org"},
			wantErr: resolvers.EnvFHIRSubscriptionAllowedHosts,
		},
		{
			name:    "subscription max clients zero",
			env:     map[string]string{resolvers.EnvFHIRSubscriptionMaxClients: "0"},
			wantErr: resolvers.EnvFHIRSubscriptionMaxClients,
		},
		{
			name: "subscription allowlist",
			env: map[string]string{
				resolvers.EnvFHIRSubscriptionAllowedHosts: "fhir.example.org,*.partner.example.com",
				resolvers.EnvFHIRSubscriptionMaxClients:   "4",
			},
			check: func(t *testing.T) {
				policy, _ := loadFHIRSubscriptionPolicyFromEnv()
				if !reflect.DeepEqual(policy.AllowedHosts, []string{"fhir.example.org", "*.partner.example.com"}) || policy.MaxClients != 4 {
					t.Fatalf("policy = %+v", policy)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearWorkflowHardeningEnv(t)
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			var errs []string
			if _, err := loadExecAllowlistFromEnv(); err != nil {
				errs = append(errs, err.Error())
			}
			if _, err := loadWorkflowDebugPolicyFromEnv(); err != nil {
				errs = append(errs, err.Error())
			}
			if _, err := loadFHIRSubscriptionPolicyFromEnv(); err != nil {
				errs = append(errs, err.Error())
			}
			joined := strings.Join(errs, "; ")
			if tt.wantErr == "" && joined != "" {
				t.Fatalf("unexpected error: %s", joined)
			}
			if tt.wantErr != "" && !strings.Contains(joined, tt.wantErr) {
				t.Fatalf("error %q does not mention %q", joined, tt.wantErr)
			}
			if tt.check != nil {
				tt.check(t)
			}
		})
	}
}
