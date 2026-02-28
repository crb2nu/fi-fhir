package main

import (
	"context"
	"testing"
)

func TestInitEventStoreFromEnv_MissingConfig_ReturnsNil(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_HOST", "")
	t.Setenv("FI_FHIR_DATABASE_NAME", "")
	t.Setenv("FI_FHIR_DATABASE_USERNAME", "")
	t.Setenv("FI_FHIR_DATABASE_USER", "")

	es, err := initEventStoreFromEnv(context.Background())
	assertNoError(t, err)
	if es != nil {
		t.Fatalf("expected nil event store, got %T", es)
	}
}

func TestInitEventStoreFromEnv_BackCompatUser_InvalidDriverErrors(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_HOST", "localhost")
	t.Setenv("FI_FHIR_DATABASE_NAME", "fi_fhir")
	t.Setenv("FI_FHIR_DATABASE_USER", "user") // backwards/compat env var
	t.Setenv("FI_FHIR_DATABASE_DRIVER", "nope")

	es, err := initEventStoreFromEnv(context.Background())
	assertError(t, err)
	assertErrorContains(t, err, "database DSN is empty")
	if es != nil {
		t.Fatalf("expected nil event store, got %T", es)
	}
}

func TestInitEventStoreFromEnv_PartialConfig_ReturnsNil(t *testing.T) {
	// Only host set, missing database and username — should gracefully return nil.
	t.Setenv("FI_FHIR_DATABASE_HOST", "localhost")
	t.Setenv("FI_FHIR_DATABASE_NAME", "")
	t.Setenv("FI_FHIR_DATABASE_USERNAME", "")
	t.Setenv("FI_FHIR_DATABASE_USER", "")

	es, err := initEventStoreFromEnv(context.Background())
	assertNoError(t, err)
	if es != nil {
		t.Fatalf("expected nil event store, got %T", es)
	}
}
