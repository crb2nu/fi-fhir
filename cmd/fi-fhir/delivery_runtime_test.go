package main

import (
	"database/sql"
	"strings"
	"testing"

	_ "github.com/lib/pq"
)

func TestDeliveryWorkerEnabledFromEnvIsExplicit(t *testing.T) {
	t.Setenv("FI_FHIR_DELIVERY_WORKER_ENABLED", "true")
	if enabled, err := deliveryWorkerEnabledFromEnv(true); err != nil || !enabled {
		t.Fatalf("enabled=%v error=%v", enabled, err)
	}
	if enabled, err := deliveryWorkerEnabledFromEnv(false); err != nil || enabled {
		t.Fatalf("preview enabled=%v error=%v", enabled, err)
	}
	t.Setenv("FI_FHIR_DELIVERY_WORKER_ENABLED", "sometimes")
	if _, err := deliveryWorkerEnabledFromEnv(true); err == nil {
		t.Fatal("invalid delivery enablement accepted")
	}
}

func TestLoadDeliveryDispatcherFromEnvValidatesKafkaBoundary(t *testing.T) {
	db, err := sql.Open("postgres", "postgres://unused")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for name, value := range map[string]string{
		"FI_FHIR_QUEUE_DRIVER":        "kafka",
		"FI_FHIR_QUEUE_BROKERS":       "kafka-a:9092,kafka-b:9092",
		"FI_FHIR_DELIVERY_WORKER_ID":  "delivery-worker-a",
		"FI_FHIR_QUEUE_TLS":           "false",
		"FI_FHIR_QUEUE_USERNAME":      "",
		"FI_FHIR_QUEUE_PASSWORD":      "",
		"FI_FHIR_QUEUE_PASSWORD_FILE": "",
	} {
		t.Setenv(name, value)
	}
	dispatcher, err := loadDeliveryDispatcherFromEnv(db)
	if err != nil {
		t.Fatalf("loadDeliveryDispatcherFromEnv: %v", err)
	}
	if err := dispatcher.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	t.Setenv("FI_FHIR_QUEUE_TLS", "invalid")
	if _, err := loadDeliveryDispatcherFromEnv(db); err == nil || !strings.Contains(err.Error(), "true or false") {
		t.Fatalf("invalid TLS error = %v", err)
	}
}

func TestLoadDeliveryDispatcherRequiresTLSForCredentials(t *testing.T) {
	db, err := sql.Open("postgres", "postgres://unused")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	t.Setenv("FI_FHIR_QUEUE_DRIVER", "kafka")
	t.Setenv("FI_FHIR_QUEUE_BROKERS", "kafka:9092")
	t.Setenv("FI_FHIR_DELIVERY_WORKER_ID", "delivery-worker-a")
	t.Setenv("FI_FHIR_QUEUE_TLS", "false")
	t.Setenv("FI_FHIR_QUEUE_USERNAME", "delivery-user")
	t.Setenv("FI_FHIR_QUEUE_PASSWORD", "delivery-password")
	t.Setenv("FI_FHIR_QUEUE_PASSWORD_FILE", "")
	if _, err := loadDeliveryDispatcherFromEnv(db); err == nil || !strings.Contains(err.Error(), "require TLS") {
		t.Fatalf("credential transport error = %v", err)
	}
}
