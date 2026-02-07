package main

import "testing"

// =============================================================================
// runSubscriptionStatus — flag-parsing and validation tests
// =============================================================================

func TestSubscriptionStatus_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "subscription", "status", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "subscription status")
}

func TestSubscriptionStatus_MissingConfigValue(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "status", "--config")
	assertError(t, err)
	assertErrorContains(t, err, "--config requires a value")
}

func TestSubscriptionStatus_MissingNameValue(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "status", "--name")
	assertError(t, err)
	assertErrorContains(t, err, "--name requires a value")
}

func TestSubscriptionStatus_ConfigRequired(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "status", "--name", "test")
	assertError(t, err)
	assertErrorContains(t, err, "--config is required")
}

func TestSubscriptionStatus_NameRequired(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "status", "--config", "/tmp/cfg.yaml")
	assertError(t, err)
	assertErrorContains(t, err, "subscription name is required")
}

func TestSubscriptionStatus_ConfigLoadError(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "status", "--config", "/nonexistent/cfg.yaml", "--name", "test")
	assertError(t, err)
	assertErrorContains(t, err, "load config")
}

func TestSubscriptionStatus_ShortFlags(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "status", "-c")
	assertError(t, err)
	assertErrorContains(t, err, "--config requires a value")
}

func TestSubscriptionStatus_ShortNameFlag(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "status", "-n")
	assertError(t, err)
	assertErrorContains(t, err, "--name requires a value")
}

// =============================================================================
// runSubscriptionDelete — flag-parsing and validation tests
// =============================================================================

func TestSubscriptionDelete_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "subscription", "delete", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "subscription delete")
}

func TestSubscriptionDelete_MissingConfigValue(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "delete", "--config")
	assertError(t, err)
	assertErrorContains(t, err, "--config requires a value")
}

func TestSubscriptionDelete_MissingNameValue(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "delete", "--name")
	assertError(t, err)
	assertErrorContains(t, err, "--name requires a value")
}

func TestSubscriptionDelete_MissingIDValue(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "delete", "--id")
	assertError(t, err)
	assertErrorContains(t, err, "--id requires a value")
}

func TestSubscriptionDelete_ConfigRequired(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "delete", "--name", "test", "--id", "abc")
	assertError(t, err)
	assertErrorContains(t, err, "--config is required")
}

func TestSubscriptionDelete_NameRequired(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "delete", "--config", "/tmp/cfg.yaml", "--id", "abc")
	assertError(t, err)
	assertErrorContains(t, err, "--name is required")
}

func TestSubscriptionDelete_IDRequired(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "delete", "--config", "/tmp/cfg.yaml", "--name", "test")
	assertError(t, err)
	assertErrorContains(t, err, "--id is required")
}

func TestSubscriptionDelete_ConfigLoadError(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "delete", "--config", "/nonexistent/cfg.yaml", "--name", "test", "--id", "abc")
	assertError(t, err)
	assertErrorContains(t, err, "load config")
}

// =============================================================================
// runSubscriptionPauseResume — flag-parsing and validation tests
// =============================================================================

func TestSubscriptionPause_Help(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runSubscriptionPauseResume([]string{"--help"}, true)
		assertNoError(t, err)
	})
	assertContains(t, stdout, "pause")
}

func TestSubscriptionResume_Help(t *testing.T) {
	stdout, _ := captureOutput(t, func() {
		err := runSubscriptionPauseResume([]string{"--help"}, false)
		assertNoError(t, err)
	})
	assertContains(t, stdout, "resume")
}

func TestSubscriptionPause_MissingConfigValue(t *testing.T) {
	err := runSubscriptionPauseResume([]string{"--config"}, true)
	assertError(t, err)
	assertErrorContains(t, err, "--config requires a value")
}

func TestSubscriptionPause_MissingNameValue(t *testing.T) {
	err := runSubscriptionPauseResume([]string{"--name"}, true)
	assertError(t, err)
	assertErrorContains(t, err, "--name requires a value")
}

func TestSubscriptionPause_MissingIDValue(t *testing.T) {
	err := runSubscriptionPauseResume([]string{"--id"}, true)
	assertError(t, err)
	assertErrorContains(t, err, "--id requires a value")
}

func TestSubscriptionPause_ConfigRequired(t *testing.T) {
	err := runSubscriptionPauseResume([]string{"--name", "test", "--id", "abc"}, true)
	assertError(t, err)
	assertErrorContains(t, err, "--config is required")
}

func TestSubscriptionPause_NameRequired(t *testing.T) {
	err := runSubscriptionPauseResume([]string{"--config", "/tmp/cfg.yaml", "--id", "abc"}, true)
	assertError(t, err)
	assertErrorContains(t, err, "--name is required")
}

func TestSubscriptionPause_IDRequired(t *testing.T) {
	err := runSubscriptionPauseResume([]string{"--config", "/tmp/cfg.yaml", "--name", "test"}, true)
	assertError(t, err)
	assertErrorContains(t, err, "--id is required")
}

func TestSubscriptionResume_ConfigRequired(t *testing.T) {
	err := runSubscriptionPauseResume([]string{"--name", "test", "--id", "abc"}, false)
	assertError(t, err)
	assertErrorContains(t, err, "--config is required")
}

func TestSubscriptionPause_ConfigLoadError(t *testing.T) {
	err := runSubscriptionPauseResume([]string{"--config", "/nonexistent/cfg.yaml", "--name", "test", "--id", "abc"}, true)
	assertError(t, err)
	assertErrorContains(t, err, "load config")
}

// =============================================================================
// runSubscriptionCreate — flag-parsing and validation tests
// =============================================================================

func TestSubscriptionCreate_Help(t *testing.T) {
	stdout, _, err := runCLI(t, "subscription", "create", "--help")
	assertNoError(t, err)
	assertContains(t, stdout, "subscription create")
}

func TestSubscriptionCreate_MissingConfigValue(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "create", "--config")
	assertError(t, err)
	assertErrorContains(t, err, "--config requires a value")
}

func TestSubscriptionCreate_MissingNameValue(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "create", "--name")
	assertError(t, err)
	assertErrorContains(t, err, "--name requires a value")
}

func TestSubscriptionCreate_ConfigRequired(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "create", "--name", "test")
	assertError(t, err)
	assertErrorContains(t, err, "--config is required")
}

func TestSubscriptionCreate_NameRequired(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "create", "--config", "/tmp/cfg.yaml")
	assertError(t, err)
	assertErrorContains(t, err, "subscription name is required")
}

func TestSubscriptionCreate_ConfigLoadError(t *testing.T) {
	_, _, err := runCLI(t, "subscription", "create", "--config", "/nonexistent/cfg.yaml", "--name", "test")
	assertError(t, err)
	assertErrorContains(t, err, "load config")
}
