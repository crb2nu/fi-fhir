package main

import (
	"context"
	"testing"
)

// =============================================================================
// runServe — flag-parsing branches NOT covered by serve_additional_test.go
// =============================================================================

func TestServe_Help_ReturnsNil(t *testing.T) {
	stdout, _, err := runCLI(t, "serve", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "serve")
}

func TestServe_ShortHelp_ReturnsNil(t *testing.T) {
	stdout, _, err := runCLI(t, "serve", "-h")
	assertNoError(t, err)
	assertContains(t, stdout, "serve")
}

func TestServe_MissingPortValue(t *testing.T) {
	_, _, err := runCLI(t, "serve", "--port")
	assertError(t, err)
	assertErrorContains(t, err, "--port requires a value")
}

func TestServe_MissingPathValue(t *testing.T) {
	_, _, err := runCLI(t, "serve", "--path")
	assertError(t, err)
	assertErrorContains(t, err, "--path requires a value")
}

func TestServe_MissingPlaygroundPathValue(t *testing.T) {
	_, _, err := runCLI(t, "serve", "--playground-path")
	assertError(t, err)
	assertErrorContains(t, err, "--playground-path requires a value")
}

func TestServe_MissingWorkflowValue(t *testing.T) {
	_, _, err := runCLI(t, "serve", "--workflow")
	assertError(t, err)
	assertErrorContains(t, err, "--workflow requires a value")
}

func TestServe_MissingMaxDepthValue(t *testing.T) {
	_, _, err := runCLI(t, "serve", "--max-depth")
	assertError(t, err)
	assertErrorContains(t, err, "--max-depth requires a value")
}

func TestServe_InvalidMaxDepth(t *testing.T) {
	_, _, err := runCLI(t, "serve", "--max-depth", "abc")
	assertError(t, err)
	assertErrorContains(t, err, "invalid max-depth")
}

func TestServe_MissingMaxComplexityValue(t *testing.T) {
	_, _, err := runCLI(t, "serve", "--max-complexity")
	assertError(t, err)
	assertErrorContains(t, err, "--max-complexity requires a value")
}

func TestServe_InvalidMaxComplexity(t *testing.T) {
	_, _, err := runCLI(t, "serve", "--max-complexity", "abc")
	assertError(t, err)
	assertErrorContains(t, err, "invalid max-complexity")
}

func TestServe_MissingTimeoutValue(t *testing.T) {
	_, _, err := runCLI(t, "serve", "--timeout")
	assertError(t, err)
	assertErrorContains(t, err, "--timeout requires a value")
}

func TestServe_MissingTemporalValue(t *testing.T) {
	_, _, err := runCLI(t, "serve", "--temporal")
	assertError(t, err)
	assertErrorContains(t, err, "--temporal requires a value")
}

func TestServe_MissingTemporalNamespaceValue(t *testing.T) {
	_, _, err := runCLI(t, "serve", "--temporal-namespace")
	assertError(t, err)
	assertErrorContains(t, err, "--temporal-namespace requires a value")
}

func TestServe_ShortPortFlag(t *testing.T) {
	_, _, err := runCLI(t, "serve", "-p", "nope")
	assertError(t, err)
	assertErrorContains(t, err, "invalid port")
}

func TestServe_ShortHostFlag(t *testing.T) {
	_, _, err := runCLI(t, "serve", "-H")
	assertError(t, err)
	assertErrorContains(t, err, "--host requires a value")
}

func TestServe_ShortWorkflowFlag(t *testing.T) {
	_, _, err := runCLI(t, "serve", "-w")
	assertError(t, err)
	assertErrorContains(t, err, "--workflow requires a value")
}

// =============================================================================
// runSubscriptionServe — flag-parsing branches NOT covered by
// subscription_serve_additional_test.go and subscription_serve_cli_test.go
// =============================================================================

func TestSubscriptionServe_Help_ReturnsNil(t *testing.T) {
	stdout, _, err := runCLI(t, "subscription", "serve", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "subscription")
}

func TestSubscriptionServe_ShortHelp_ReturnsNil(t *testing.T) {
	stdout, _, err := runCLI(t, "subscription", "serve", "-h")
	assertNoError(t, err)
	assertContains(t, stdout, "subscription")
}

func TestSubscriptionServe_MissingSubscriptionsValue(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "serve", "--subscriptions")
	assertError(t, err)
	assertErrorContains(t, err, "--subscriptions requires a value")
}

func TestSubscriptionServe_SubscriptionsNotProvided(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "serve")
	assertError(t, err)
	assertErrorContains(t, err, "--subscriptions is required")
}

func TestSubscriptionServe_MissingWorkflowValue(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "serve", "--subscriptions", "/tmp/x.yaml", "--workflow")
	assertError(t, err)
	assertErrorContains(t, err, "--workflow requires a value")
}

func TestSubscriptionServe_MissingConfigValue(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "serve", "--subscriptions", "/tmp/x.yaml", "--config")
	assertError(t, err)
	assertErrorContains(t, err, "--config requires a value")
}

func TestSubscriptionServe_MissingHostValue(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "serve", "--subscriptions", "/tmp/x.yaml", "--host")
	assertError(t, err)
	assertErrorContains(t, err, "--host requires a value")
}

func TestSubscriptionServe_MissingCertValue(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "serve", "--subscriptions", "/tmp/x.yaml", "--cert")
	assertError(t, err)
	assertErrorContains(t, err, "--cert requires a value")
}

func TestSubscriptionServe_MissingKeyValue(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "serve", "--subscriptions", "/tmp/x.yaml", "--key")
	assertError(t, err)
	assertErrorContains(t, err, "--key requires a value")
}

func TestSubscriptionServe_UnexpectedPositionalArg(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "serve", "unexpected-arg")
	assertError(t, err)
	assertErrorContains(t, err, "unexpected arg")
}

func TestSubscriptionServe_ShortFlags(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "serve", "-s")
	assertError(t, err)
	assertErrorContains(t, err, "--subscriptions requires a value")
}

// =============================================================================
// runServe — flag-parsing and dry-run coverage
// =============================================================================

func TestServe_DryRun_NoArgs(t *testing.T) {
	stdout, _, err := runCLI(t, "serve", "--dry-run")
	assertNoError(t, err)
	assertContains(t, stdout, "\"playground_enabled\": true")
}

func TestServe_DryRun_WithFlags(t *testing.T) {
	stdout, _, err := runCLI(t, "serve", "--host", "127.0.0.1", "--port", "9090", "--path", "/api", "--no-playground", "--no-introspection", "--dry-run")
	assertNoError(t, err)
	assertContains(t, stdout, "\"host\": \"127.0.0.1\"")
	assertContains(t, stdout, "\"port\": 9090")
	assertContains(t, stdout, "\"path\": \"/api\"")
	assertContains(t, stdout, "\"playground_enabled\": false")
	assertContains(t, stdout, "\"introspection\": false")
}

// =============================================================================
// Store Init Coverage (initEventStoreFromEnv, initProfileStoreFromEnv, etc)
// =============================================================================

func TestInitEventStoreFromEnv_MissingVars(t *testing.T) {
	// Need to clear them to ensure it's empty during the test
	t.Setenv("FI_FHIR_DATABASE_HOST", "")
	t.Setenv("DATABASE_URL", "") // To ensure fallback is empty too

	es, err := initEventStoreFromEnv(context.Background())
	assertNoError(t, err)
	if es != nil {
		t.Fatalf("expected nil event store when env vars are missing")
	}
}

func TestInitEventStoreFromEnv_InvalidConnection(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_HOST", "invalid-host")
	t.Setenv("FI_FHIR_DATABASE_PORT", "5432")
	t.Setenv("FI_FHIR_DATABASE_NAME", "testdb")
	t.Setenv("FI_FHIR_DATABASE_USER", "testuser")
	t.Setenv("FI_FHIR_DATABASE_SCHEMA", "fi_fhir_events")

	_, err := initEventStoreFromEnv(context.Background())
	assertError(t, err)
}

func TestInitProfileStoreFromEnv_MissingVars(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_HOST", "")
	t.Setenv("DATABASE_URL", "")

	ps, err := initProfileStoreFromEnv(context.Background())
	assertNoError(t, err)
	if ps != nil {
		t.Fatalf("expected nil profile store when env vars are missing")
	}
}

func TestInitProfileStoreFromEnv_InvalidConnection(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_HOST", "invalid-host")
	t.Setenv("FI_FHIR_DATABASE_PORT", "5432")
	t.Setenv("FI_FHIR_DATABASE_NAME", "testdb")
	t.Setenv("FI_FHIR_DATABASE_USER", "testuser")
	t.Setenv("FI_FHIR_DATABASE_SCHEMA", "fi_fhir_profiles")

	_, err := initProfileStoreFromEnv(context.Background())
	assertError(t, err)
}

func TestInitWorkflowLifecycleStoreFromEnv_MissingVars(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_HOST", "")
	t.Setenv("DATABASE_URL", "")

	ws, err := initWorkflowLifecycleStoreFromEnv(context.Background())
	assertNoError(t, err)
	if ws != nil {
		t.Fatalf("expected nil workflow lifecycle store when env vars are missing")
	}
}

func TestInitWorkflowLifecycleStoreFromEnv_InvalidConnection(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_HOST", "invalid-host")
	t.Setenv("FI_FHIR_DATABASE_PORT", "5432")
	t.Setenv("FI_FHIR_DATABASE_NAME", "testdb")
	t.Setenv("FI_FHIR_DATABASE_USER", "testuser")
	t.Setenv("FI_FHIR_DATABASE_SCHEMA", "fi_fhir_workflows")

	_, err := initWorkflowLifecycleStoreFromEnv(context.Background())
	assertError(t, err)
}
