package main

import "testing"

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
