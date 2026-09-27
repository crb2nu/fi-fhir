package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

// Deployment-owned allowlists for the two surfaces that let a
// `graphql:operator` caller make this process act on caller-supplied input
// (docs/operations/SECURITY.md, SEC-2026-09-27-1 and -2). Every key is
// optional; unset means the most restrictive behaviour. A malformed value fails
// startup closed rather than being silently narrowed or widened.

// loadExecAllowlistFromEnv validates FI_FHIR_WORKFLOW_EXEC_ALLOWLIST. Engines
// read the key themselves (workflow.NewEngine), so serve only refuses to start
// on an entry the engine would silently drop.
func loadExecAllowlistFromEnv() ([]string, error) {
	raw := os.Getenv(workflow.EnvExecAllowlist)
	parsed := workflow.ParseExecAllowlist(raw)
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if !containsString(parsed, item) {
			return nil, fmt.Errorf("%s entry %q must be an absolute, clean executable path", workflow.EnvExecAllowlist, item)
		}
	}
	return parsed, nil
}

// loadWorkflowDebugPolicyFromEnv builds the debugger policy.
func loadWorkflowDebugPolicyFromEnv() (resolvers.WorkflowDebugPolicy, error) {
	policy := resolvers.DefaultWorkflowDebugPolicy()

	allowed, refused := workflow.ParseDebugActionAllowlist(os.Getenv(workflow.EnvDebugActions))
	if len(refused) > 0 {
		return policy, fmt.Errorf("%s cannot enable %s: the debugger never executes it", workflow.EnvDebugActions, strings.Join(refused, ", "))
	}
	if len(allowed) > 0 {
		known, err := builtinWorkflowActionTypes()
		if err != nil {
			return policy, err
		}
		for _, name := range allowed {
			if !containsString(known, name) {
				return policy, fmt.Errorf("%s names unknown action type %q (known: %s)", workflow.EnvDebugActions, name, strings.Join(known, ", "))
			}
		}
	}
	policy.ExecutableActions = allowed

	if raw := strings.TrimSpace(os.Getenv(resolvers.EnvWorkflowDebugMaxSessions)); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			return policy, fmt.Errorf("%s must be a non-negative integer (0 disables the debugger)", resolvers.EnvWorkflowDebugMaxSessions)
		}
		policy.MaxSessions = value
	}
	if raw := strings.TrimSpace(os.Getenv(resolvers.EnvWorkflowDebugSessionTTL)); raw != "" {
		value, err := time.ParseDuration(raw)
		if err != nil || value <= 0 {
			return policy, fmt.Errorf("%s must be a positive duration such as 15m", resolvers.EnvWorkflowDebugSessionTTL)
		}
		policy.SessionTTL = value
	}
	return policy, nil
}

// loadFHIRSubscriptionPolicyFromEnv builds the FHIR subscription destination
// policy.
func loadFHIRSubscriptionPolicyFromEnv() (resolvers.FHIRSubscriptionPolicy, error) {
	policy := resolvers.DefaultFHIRSubscriptionPolicy()
	hosts, err := resolvers.ParseFHIRSubscriptionAllowedHosts(os.Getenv(resolvers.EnvFHIRSubscriptionAllowedHosts))
	if err != nil {
		return policy, err
	}
	policy.AllowedHosts = hosts
	if raw := strings.TrimSpace(os.Getenv(resolvers.EnvFHIRSubscriptionMaxClients)); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value <= 0 {
			return policy, fmt.Errorf("%s must be a positive integer", resolvers.EnvFHIRSubscriptionMaxClients)
		}
		policy.MaxClients = value
	}
	return policy, nil
}

// builtinWorkflowActionTypes are the action types a fresh engine registers.
func builtinWorkflowActionTypes() ([]string, error) {
	probe, err := workflow.NewEngine(&workflow.Workflow{Name: "action-type-probe"})
	if err != nil {
		return nil, fmt.Errorf("enumerate workflow action types: %w", err)
	}
	return probe.RegisteredActionTypes(), nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
