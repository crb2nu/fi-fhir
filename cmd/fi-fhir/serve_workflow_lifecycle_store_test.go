package main

import (
	"context"
	"testing"
)

func TestInitWorkflowLifecycleStoreFromEnv_MissingConfig_ReturnsNil(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_HOST", "")
	t.Setenv("FI_FHIR_DATABASE_NAME", "")
	t.Setenv("FI_FHIR_DATABASE_USERNAME", "")
	t.Setenv("FI_FHIR_DATABASE_USER", "")

	store, err := initWorkflowLifecycleStoreFromEnv(context.Background())
	assertNoError(t, err)
	if store != nil {
		t.Fatalf("expected nil workflow lifecycle store, got %T", store)
	}
}

func TestInitWorkflowLifecycleStoreFromEnv_BackCompatUser_InvalidDriverErrors(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_HOST", "localhost")
	t.Setenv("FI_FHIR_DATABASE_NAME", "fi_fhir")
	t.Setenv("FI_FHIR_DATABASE_USER", "user") // backwards/compat env var
	t.Setenv("FI_FHIR_DATABASE_DRIVER", "nope")

	store, err := initWorkflowLifecycleStoreFromEnv(context.Background())
	assertError(t, err)
	assertErrorContains(t, err, "database DSN is empty")
	if store != nil {
		t.Fatalf("expected nil workflow lifecycle store, got %T", store)
	}
}
