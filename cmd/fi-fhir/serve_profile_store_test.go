package main

import (
	"context"
	"testing"
)

func TestInitProfileStoreFromEnv_MissingConfig_ReturnsNil(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_HOST", "")
	t.Setenv("FI_FHIR_DATABASE_NAME", "")
	t.Setenv("FI_FHIR_DATABASE_USERNAME", "")
	t.Setenv("FI_FHIR_DATABASE_USER", "")

	ps, err := initProfileStoreFromEnv(context.Background())
	assertNoError(t, err)
	if ps != nil {
		t.Fatalf("expected nil profile store, got %T", ps)
	}
}

func TestInitProfileStoreFromEnv_BackCompatUser_InvalidDriverErrors(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_HOST", "localhost")
	t.Setenv("FI_FHIR_DATABASE_NAME", "fi_fhir")
	t.Setenv("FI_FHIR_DATABASE_USER", "user") // backwards/compat env var
	t.Setenv("FI_FHIR_DATABASE_DRIVER", "nope")

	ps, err := initProfileStoreFromEnv(context.Background())
	assertError(t, err)
	assertErrorContains(t, err, "database DSN is empty")
	if ps != nil {
		t.Fatalf("expected nil profile store, got %T", ps)
	}
}
