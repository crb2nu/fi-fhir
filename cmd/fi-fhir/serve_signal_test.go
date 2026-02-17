package main

import (
	"os"
	"syscall"
	"testing"
	"time"
)

func TestServe_GracefulShutdownOnSignal(t *testing.T) {
	// Keep optional integrations from dialing external services.
	t.Setenv("FI_FHIR_TERMINOLOGY_DB_URL", "")
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_PINS", "")
	t.Setenv("FI_FHIR_TERMINOLOGY_POLICY", "pass")
	t.Setenv("FI_FHIR_DATABASE_HOST", "")
	t.Setenv("FI_FHIR_DATABASE_NAME", "")
	t.Setenv("FI_FHIR_DATABASE_USERNAME", "")
	t.Setenv("FI_FHIR_DATABASE_USER", "")
	t.Setenv("TEMPORAL_ADDRESS", "")
	t.Setenv("TEMPORAL_NAMESPACE", "")

	done := make(chan error, 1)
	go func() {
		_, _, err := runCLI(t,
			"serve",
			"--host", "127.0.0.1",
			"--port", "0",
			"--no-playground",
			"--no-introspection",
		)
		done <- err
	}()

	// Give runServe enough time to install its signal handler and start.
	time.Sleep(350 * time.Millisecond)

	self, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatalf("find self process: %v", err)
	}
	if err := self.Signal(syscall.SIGINT); err != nil {
		t.Fatalf("send SIGINT: %v", err)
	}

	select {
	case err := <-done:
		assertNoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not shut down after SIGINT")
	}
}
