package main

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	integrationdelivery "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
)

const maxKafkaCABytes = 1 << 20

func deliveryWorkerEnabledFromEnv(allowProduction bool) (bool, error) {
	value := os.Getenv("FI_FHIR_DELIVERY_WORKER_ENABLED")
	if value == "" || !allowProduction {
		return false, nil
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("FI_FHIR_DELIVERY_WORKER_ENABLED must be true or false")
	}
	return enabled, nil
}

func loadDeliveryDispatcherFromEnv(db *sql.DB) (*integrationdelivery.Dispatcher, error) {
	if db == nil {
		return nil, fmt.Errorf("delivery worker requires the PostgreSQL submission database")
	}
	if os.Getenv("FI_FHIR_QUEUE_DRIVER") != "kafka" {
		return nil, fmt.Errorf("delivery worker requires FI_FHIR_QUEUE_DRIVER=kafka")
	}
	brokers, err := parseCSVConfig("FI_FHIR_QUEUE_BROKERS", os.Getenv("FI_FHIR_QUEUE_BROKERS"))
	if err != nil {
		return nil, err
	}
	workerID := os.Getenv("FI_FHIR_DELIVERY_WORKER_ID")
	if workerID == "" {
		hostname, hostnameErr := os.Hostname()
		if hostnameErr != nil || strings.TrimSpace(hostname) == "" {
			return nil, fmt.Errorf("FI_FHIR_DELIVERY_WORKER_ID is required when hostname is unavailable")
		}
		workerID = fmt.Sprintf("%s-%d", hostname, os.Getpid())
	}

	workerConfig := integrationdelivery.DefaultConfig()
	if err := applyDeliveryDurationEnv("FI_FHIR_DELIVERY_LEASE_DURATION", &workerConfig.LeaseDuration); err != nil {
		return nil, err
	}
	if err := applyDeliveryDurationEnv("FI_FHIR_DELIVERY_POLL_INTERVAL", &workerConfig.PollInterval); err != nil {
		return nil, err
	}
	if err := applyDeliveryDurationEnv("FI_FHIR_DELIVERY_PUBLISH_TIMEOUT", &workerConfig.PublishTimeout); err != nil {
		return nil, err
	}
	if err := applyDeliveryDurationEnv("FI_FHIR_DELIVERY_RETRY_BASE_DELAY", &workerConfig.RetryBaseDelay); err != nil {
		return nil, err
	}
	if err := applyDeliveryDurationEnv("FI_FHIR_DELIVERY_RETRY_MAX_DELAY", &workerConfig.RetryMaxDelay); err != nil {
		return nil, err
	}
	if err := applyDeliveryDurationEnv("FI_FHIR_DELIVERY_CIRCUIT_OPEN_DURATION", &workerConfig.CircuitOpenDuration); err != nil {
		return nil, err
	}
	if err := applyDeliveryIntEnv("FI_FHIR_DELIVERY_MAX_ATTEMPTS", &workerConfig.MaxAttempts); err != nil {
		return nil, err
	}
	if err := applyDeliveryIntEnv("FI_FHIR_DELIVERY_CIRCUIT_FAILURE_THRESHOLD", &workerConfig.CircuitFailureThreshold); err != nil {
		return nil, err
	}

	tlsEnabled, err := optionalBoolEnv("FI_FHIR_QUEUE_TLS")
	if err != nil {
		return nil, err
	}
	kafkaConfig := integrationdelivery.KafkaConfig{
		Brokers:         brokers,
		ClientID:        os.Getenv("FI_FHIR_QUEUE_CLIENT_ID"),
		TLS:             tlsEnabled,
		Username:        os.Getenv("FI_FHIR_QUEUE_USERNAME"),
		DialTimeout:     workerConfig.PublishTimeout,
		DeliveryTimeout: workerConfig.PublishTimeout,
	}
	if kafkaConfig.Username != "" {
		kafkaConfig.Password, err = loadSingleLineSecret(
			"FI_FHIR_QUEUE_PASSWORD",
			"FI_FHIR_QUEUE_PASSWORD_FILE",
			"Kafka password",
		)
		if err != nil {
			return nil, err
		}
	} else if os.Getenv("FI_FHIR_QUEUE_PASSWORD") != "" || os.Getenv("FI_FHIR_QUEUE_PASSWORD_FILE") != "" {
		return nil, fmt.Errorf("FI_FHIR_QUEUE_USERNAME is required with a Kafka password")
	}
	if caPath := os.Getenv("FI_FHIR_QUEUE_TLS_ROOT_CA_FILE"); caPath != "" {
		if !kafkaConfig.TLS {
			return nil, fmt.Errorf("FI_FHIR_QUEUE_TLS_ROOT_CA_FILE requires FI_FHIR_QUEUE_TLS=true")
		}
		kafkaConfig.RootCAPEM, err = loadBoundedKafkaCA(caPath)
		if err != nil {
			return nil, err
		}
	}
	store, err := integrationdelivery.NewPostgresStore(db, nil)
	if err != nil {
		return nil, fmt.Errorf("configure delivery store: %w", err)
	}
	publisher, err := integrationdelivery.NewKafkaPublisher(kafkaConfig)
	if err != nil {
		return nil, err
	}
	dispatcher, err := integrationdelivery.NewDispatcher(store, publisher, workerID, workerConfig)
	if err != nil {
		_ = publisher.Close()
		return nil, err
	}
	return dispatcher, nil
}

func applyDeliveryDurationEnv(name string, target *time.Duration) error {
	value := os.Getenv(name)
	if value == "" {
		return nil
	}
	duration, err := time.ParseDuration(value)
	if err != nil || duration <= 0 {
		return fmt.Errorf("%s must be a positive duration", name)
	}
	*target = duration
	return nil
}

func applyDeliveryIntEnv(name string, target *int) error {
	value := os.Getenv(name)
	if value == "" {
		return nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed < 1 {
		return fmt.Errorf("%s must be a positive integer", name)
	}
	*target = parsed
	return nil
}

func optionalBoolEnv(name string) (bool, error) {
	value := os.Getenv(name)
	if value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return parsed, nil
}

func loadBoundedKafkaCA(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open Kafka root CA: %w", err)
	}
	raw, readErr := io.ReadAll(io.LimitReader(file, maxKafkaCABytes+1))
	closeErr := file.Close()
	if readErr != nil || len(raw) == 0 || len(raw) > maxKafkaCABytes {
		return nil, fmt.Errorf("read Kafka root CA: invalid file")
	}
	if closeErr != nil {
		return nil, fmt.Errorf("close Kafka root CA: %w", closeErr)
	}
	return raw, nil
}
