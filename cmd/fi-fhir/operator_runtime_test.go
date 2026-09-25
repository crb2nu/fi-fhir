package main

import (
	"context"
	"strings"
	"testing"
)

func configureOperatorRuntimeTest(t *testing.T) {
	t.Helper()
	clearGraphQLAuthenticationEnv(t)
	configurePreviewRuntimeForTest(t)
	for _, name := range []string{
		"FI_FHIR_GRAPHQL_SERVICE_BEARER_TOKEN_FILE",
		"FI_FHIR_GRAPHQL_SERVICE_PRINCIPAL_ID",
		"FI_FHIR_GRAPHQL_ACCESS_TEAM_DOMAIN",
		"FI_FHIR_GRAPHQL_ACCESS_AUDIENCE",
		"FI_FHIR_GRAPHQL_ACCESS_PRINCIPALS",
		"FI_FHIR_HTTP_INGRESS_AUTH_MODE",
		"FI_FHIR_MLLP_SOURCE_CONFIG_PATH",
		"FI_FHIR_BATCH_SOURCE_CONFIG_PATH",
		"FI_FHIR_DELIVERY_WORKER_ENABLED",
		"FI_FHIR_INTEGRATION_SESSION_ENABLED",
		"FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED",
		"FI_FHIR_DATABASE_HOST",
		"FI_FHIR_DATABASE_NAME",
		"FI_FHIR_DATABASE_USERNAME",
		"FI_FHIR_DATABASE_USER",
	} {
		t.Setenv(name, "")
	}
}

func assertOperatorRuntimeHasNoWorkers(t *testing.T, runtime *previewRuntime) {
	t.Helper()
	if runtime.ingressHandler != nil || runtime.ingressPath != "" || runtime.mllpServer != nil ||
		runtime.mllpRateQuota != nil || runtime.batchRunner != nil || runtime.batchProvider != nil ||
		runtime.deliveryWorker != nil || runtime.sessionStore != nil {
		t.Fatal("operator-only configuration unexpectedly enabled ingestion, delivery, or sessions")
	}
	if ids := serveDurableDefinitionIDs(runtime); len(ids) != 0 {
		t.Fatalf("operator-only configuration reports served ingress definitions: %v", ids)
	}
}

func TestOperatorRuntimeDisabledDoesNotRequireDatabase(t *testing.T) {
	for _, value := range []string{"", "false"} {
		t.Run("enabled="+value, func(t *testing.T) {
			configureOperatorRuntimeTest(t)
			t.Setenv("FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED", value)
			runtime, err := loadServeIntegrationRuntimeFromEnv(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = runtime.Close() })
			if runtime.submissionDB != nil {
				t.Fatal("disabled operator runtime opened a submission database")
			}
			assertOperatorRuntimeHasNoWorkers(t, runtime)
		})
	}
}

func TestOperatorRuntimeRejectsInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct {
		name    string
		enabled string
		serve   bool
		driver  string
		want    string
	}{
		{name: "malformed", enabled: "maybe", serve: true, want: "FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED"},
		{name: "preview command", enabled: "true", want: "available only with serve"},
		{name: "missing database", enabled: "true", serve: true, want: "FI_FHIR_DATABASE_HOST"},
		{name: "non postgres", enabled: "true", serve: true, driver: "sqlite", want: "postgres database driver"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			configureOperatorRuntimeTest(t)
			t.Setenv("FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED", tc.enabled)
			if tc.driver != "" {
				t.Setenv("FI_FHIR_DATABASE_DRIVER", tc.driver)
				t.Setenv("FI_FHIR_DATABASE_HOST", "unused.example.test")
				t.Setenv("FI_FHIR_DATABASE_NAME", "unused")
				t.Setenv("FI_FHIR_DATABASE_USERNAME", "unused")
			}
			_, err := loadIntegrationRuntimeFromEnv(context.Background(), tc.serve)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}
