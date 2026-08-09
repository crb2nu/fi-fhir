package main

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	integrationretention "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/retention"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
)

// The fail-closed default is the whole safety story of Slice 4.1e: with no
// policy path there must be no component, no policy record, and no error that
// might tempt an operator to "fix" it by loosening something.
func TestLoadRetentionPurgerIsDisabledWithoutAPolicyPath(t *testing.T) {
	t.Setenv(retentionPolicyPathEnv, "")
	purger, policy, err := loadRetentionPurgerFromEnv(context.Background(), nil, "tenant-a", nil)
	if err != nil {
		t.Fatalf("an unconfigured deployment failed startup: %v", err)
	}
	if purger != nil || policy != nil {
		t.Fatalf("an unconfigured deployment built a purger: %v, %v", purger, policy)
	}
}

// A configured policy path without a database is a misconfiguration that must
// stop startup, not silently disable the retention control.
func TestLoadRetentionPurgerRefusesAPolicyPathWithNoDatabase(t *testing.T) {
	t.Setenv(retentionPolicyPathEnv, filepath.Join(t.TempDir(), "policy.json"))
	_, _, err := loadRetentionPurgerFromEnv(context.Background(), nil, "tenant-a", nil)
	if err == nil {
		t.Fatal("a policy path with no database was accepted")
	}
	if !strings.Contains(err.Error(), retentionPolicyPathEnv) {
		t.Fatalf("the refusal did not name the setting at fault: %v", err)
	}
}

func TestRetentionDurationEnv(t *testing.T) {
	const name = "FI_FHIR_RETENTION_PURGE_INTERVAL_TEST"
	for value, want := range map[string]time.Duration{"": time.Hour, "15m": 15 * time.Minute} {
		t.Setenv(name, value)
		got, err := retentionDurationEnv(name, time.Hour)
		if err != nil || got != want {
			t.Fatalf("retentionDurationEnv(%q) = %s, %v; want %s", value, got, err, want)
		}
	}
	for _, value := range []string{"0s", "-5m", "hourly"} {
		t.Setenv(name, value)
		if _, err := retentionDurationEnv(name, time.Hour); err == nil {
			t.Fatalf("retentionDurationEnv accepted %q", value)
		}
	}
}

func TestRetentionIntEnv(t *testing.T) {
	const name = "FI_FHIR_RETENTION_PURGE_BATCH_SIZE_TEST"
	for value, want := range map[string]int{"": 200, "50": 50} {
		t.Setenv(name, value)
		got, err := retentionIntEnv(name, 200)
		if err != nil || got != want {
			t.Fatalf("retentionIntEnv(%q) = %d, %v; want %d", value, got, err, want)
		}
	}
	for _, value := range []string{"0", "-1", "many"} {
		t.Setenv(name, value)
		if _, err := retentionIntEnv(name, 200); err == nil {
			t.Fatalf("retentionIntEnv accepted %q", value)
		}
	}
}

// The observer is the only path from purge results to metrics, so it must charge
// a failure to the error outcome rather than reporting a silent zero-record
// success — the difference between "nothing expired" and "the purge is broken".
func TestRetentionPurgeObserverSeparatesFailureFromAnEmptyPass(t *testing.T) {
	metrics := observability.NewMetrics("test")
	observe := retentionPurgeObserver(metrics)

	observe(integrationretention.PurgeResult{}, errors.New("connection reset"))
	observe(integrationretention.PurgeResult{
		PurgeCounts: integrationretention.PurgeCounts{CanonicalEvents: 2, SessionSamples: 1},
	}, nil)
	observe(integrationretention.PurgeResult{}, nil)

	exposition := gatherRetentionMetrics(t, metrics)
	for _, want := range []string{
		`fi_fhir_retention_purges_total{outcome="error"} 1`,
		`fi_fhir_retention_purges_total{outcome="processed"} 2`,
		`fi_fhir_retention_records_purged_total{outcome="purged"} 3`,
	} {
		if !strings.Contains(exposition, want) {
			t.Fatalf("exposition missing %q:\n%s", want, exposition)
		}
	}
}

func gatherRetentionMetrics(t *testing.T, metrics *observability.Metrics) string {
	t.Helper()
	values, err := observability.GatheredLabelValues(metrics.Registry())
	if err != nil {
		t.Fatalf("gather label values: %v", err)
	}
	for _, value := range values {
		if !observability.KnownOutcome(value) && value != "test" {
			t.Fatalf("retention metrics published the unbounded label %q", value)
		}
	}
	return renderMetrics(t, metrics)
}

func renderMetrics(t *testing.T, metrics *observability.Metrics) string {
	t.Helper()
	families, err := metrics.Registry().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	var builder strings.Builder
	for _, family := range families {
		for _, metric := range family.GetMetric() {
			counter := metric.GetCounter()
			if counter == nil {
				continue
			}
			builder.WriteString(family.GetName())
			if labels := metric.GetLabel(); len(labels) > 0 {
				builder.WriteString("{")
				for index, label := range labels {
					if index > 0 {
						builder.WriteString(",")
					}
					builder.WriteString(label.GetName() + `="` + label.GetValue() + `"`)
				}
				builder.WriteString("}")
			}
			builder.WriteString(" " + strconv.FormatFloat(counter.GetValue(), 'f', -1, 64) + "\n")
		}
	}
	return builder.String()
}
