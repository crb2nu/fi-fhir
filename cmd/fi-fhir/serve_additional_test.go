package main

import "testing"

func TestServe_MissingHostValue(t *testing.T) {
	_, _, err := runCLI(t, "serve", "--host")
	assertError(t, err)
	assertErrorContains(t, err, "--host requires a value")
}

func TestServe_InvalidPort(t *testing.T) {
	_, _, err := runCLI(t, "serve", "--port", "nope")
	assertError(t, err)
	assertErrorContains(t, err, "invalid port")
}

func TestServe_UnknownFlag(t *testing.T) {
	_, _, err := runCLI(t, "serve", "--unknown-flag")
	assertError(t, err)
	assertErrorContains(t, err, "unknown flag")
}

func TestServe_InvalidTimeout(t *testing.T) {
	_, _, err := runCLI(t, "serve", "--timeout", "notaduration")
	assertError(t, err)
	assertErrorContains(t, err, "invalid timeout")
}

func TestServe_Workflow_NotFound(t *testing.T) {
	_, _, err := runCLI(t, "serve", "--workflow", "/nonexistent/workflow.yaml")
	assertError(t, err)
	assertErrorContains(t, err, "failed to load workflow")
}
