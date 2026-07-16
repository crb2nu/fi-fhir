package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	integrationdelivery "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

type deliveryRecoveryArgs struct {
	tenantID       string
	attemptID      string
	idempotencyKey string
	reason         string
}

func runDelivery(args []string) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printDeliveryUsage()
		return nil
	}
	kind := args[0]
	if kind != "replay" && kind != "resubmit" {
		return fmt.Errorf("unknown delivery command %q", kind)
	}
	parsed, err := parseDeliveryRecoveryArgs(args[1:])
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := openSubmissionDatabaseFromEnv(ctx)
	if err != nil {
		return fmt.Errorf("open delivery database: %w", err)
	}
	defer func() { _ = db.Close() }()
	migrations, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{})
	if err != nil {
		return fmt.Errorf("configure delivery migrations: %w", err)
	}
	if err := migrations.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate delivery store: %w", err)
	}
	var databasePrincipal string
	if err := db.QueryRowContext(ctx, `SELECT current_user`).Scan(&databasePrincipal); err != nil {
		return fmt.Errorf("resolve authenticated database principal: %w", err)
	}
	store, err := integrationdelivery.NewPostgresStore(db, nil)
	if err != nil {
		return err
	}
	operation := integrationdelivery.Operation{
		IdempotencyKey: parsed.idempotencyKey,
		Principal: integration.Principal{
			ID:         "postgres:" + databasePrincipal,
			Kind:       integration.PrincipalKindHuman,
			AuthMethod: "postgres",
			Roles:      []string{integrationdelivery.OperatorRole},
		},
		Reason: parsed.reason,
	}
	var resultAttemptID string
	if kind == "replay" {
		resultAttemptID, err = store.Replay(ctx, parsed.tenantID, parsed.attemptID, operation)
	} else {
		resultAttemptID, err = store.Resubmit(ctx, parsed.tenantID, parsed.attemptID, operation)
	}
	if err != nil {
		return fmt.Errorf("delivery %s: %w", kind, err)
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]string{
		"operation":         kind,
		"tenant_id":         parsed.tenantID,
		"source_attempt_id": parsed.attemptID,
		"result_attempt_id": resultAttemptID,
		"principal_id":      operation.Principal.ID,
	})
}

func parseDeliveryRecoveryArgs(args []string) (deliveryRecoveryArgs, error) {
	var parsed deliveryRecoveryArgs
	for index := 0; index < len(args); index++ {
		name := args[index]
		if name == "--help" || name == "-h" {
			return deliveryRecoveryArgs{}, fmt.Errorf("delivery recovery help must be requested before the subcommand")
		}
		if index+1 >= len(args) {
			return deliveryRecoveryArgs{}, fmt.Errorf("%s requires a value", name)
		}
		index++
		value := args[index]
		switch name {
		case "--tenant":
			parsed.tenantID = value
		case "--attempt":
			parsed.attemptID = value
		case "--idempotency-key":
			parsed.idempotencyKey = value
		case "--reason":
			parsed.reason = value
		default:
			return deliveryRecoveryArgs{}, fmt.Errorf("unknown delivery recovery option %q", name)
		}
	}
	missing := make([]string, 0, 4)
	for name, value := range map[string]string{
		"--tenant": parsed.tenantID, "--attempt": parsed.attemptID,
		"--idempotency-key": parsed.idempotencyKey, "--reason": parsed.reason,
	} {
		if value == "" || strings.TrimSpace(value) != value {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return deliveryRecoveryArgs{}, fmt.Errorf("required canonical delivery recovery options are missing or invalid: %s", strings.Join(missing, ", "))
	}
	return parsed, nil
}

func printDeliveryUsage() {
	fmt.Println(`fi-fhir delivery - Recover durable delivery dead letters

The PostgreSQL connection authenticates the operator. The database principal,
reason, and idempotency key are stored in the append-only operation audit.

Usage:
  fi-fhir delivery replay \
    --tenant TENANT --attempt ATTEMPT --idempotency-key KEY --reason REASON
  fi-fhir delivery resubmit \
    --tenant TENANT --attempt ATTEMPT --idempotency-key KEY --reason REASON

Replay reuses the failed attempt identity. Resubmit creates one linked child
attempt. Both commands require FI_FHIR_DATABASE_* PostgreSQL settings.`)
}
