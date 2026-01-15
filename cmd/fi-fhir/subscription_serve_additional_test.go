package main

import "testing"

func TestSubscription_Serve_InvalidPort(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "serve", "--subscriptions", "/nonexistent/subscriptions.yaml", "--port", "nope")
	assertError(t, err)
	assertErrorContains(t, err, "invalid port")
}

func TestSubscription_Serve_UnknownFlag(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "serve", "--unknown-flag")
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestSubscription_Serve_MissingPortValue(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "serve", "--subscriptions", "/nonexistent/subscriptions.yaml", "--port")
	assertError(t, err)
	assertErrorContains(t, err, "--port requires a value")
}

func TestSubscription_Serve_SubscriptionsNotFound(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "serve", "--subscriptions", "/nonexistent/subscriptions.yaml")
	assertError(t, err)
	assertErrorContains(t, err, "load subscriptions")
}

func TestSubscription_Serve_WorkflowNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := createTempFile(t, tmpDir, "subscriptions*.yaml", "subscriptions: []\n")

	_, _, err := runCLI(t, "subscription", "serve", "--subscriptions", configPath, "--workflow", "/nonexistent/workflow.yaml")
	assertError(t, err)
	assertErrorContains(t, err, "load workflow")
}
