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

// The Sprint 5 backlog gauge, at the wiring level: every class is published on
// every tick including the zeroes, and the gauge goes DOWN as well as up.
//
// A gauge that is only written when non-zero goes stale rather than going to
// zero, which makes "the backlog cleared" indistinguishable from "the purge
// component died" — the exact ambiguity D1 hid inside.
func TestRetentionPurgeObserverPublishesTheBacklogGaugeEveryTick(t *testing.T) {
	metrics := observability.NewMetrics("test")
	observe := retentionPurgeObserver(metrics)

	observe(integrationretention.PurgeResult{
		PurgeCounts: integrationretention.PurgeCounts{CanonicalEvents: 200},
		Passes:      1,
		Backlog: integrationretention.BacklogCounts{
			CanonicalEvents: 800, SessionSamples: 5,
		},
		BacklogKnown:    true,
		BudgetExhausted: true,
	}, nil)

	exposition := gatherRetentionMetrics(t, metrics)
	for _, want := range []string{
		`fi_fhir_retention_backlog_records{record_class="canonical_event"} 800`,
		`fi_fhir_retention_backlog_records{record_class="session_sample"} 5`,
		`fi_fhir_retention_backlog_records{record_class="session_export"} 0`,
		`fi_fhir_retention_backlog_records{record_class="stream_event"} 0`,
	} {
		if !strings.Contains(exposition, want) {
			t.Fatalf("exposition missing %q:\n%s", want, exposition)
		}
	}

	// Drained. The gauge must return to zero rather than hold its last value.
	observe(integrationretention.PurgeResult{
		PurgeCounts:  integrationretention.PurgeCounts{CanonicalEvents: 800},
		Passes:       5,
		BacklogKnown: true,
	}, nil)
	drained := gatherRetentionMetrics(t, metrics)
	if !strings.Contains(drained, `fi_fhir_retention_backlog_records{record_class="canonical_event"} 0`) {
		t.Fatalf("the gauge did not return to zero after the backlog drained:\n%s", drained)
	}

	// And a failing tick still reports the backlog: that is precisely when an
	// operator needs to know how far behind the purge is.
	observe(integrationretention.PurgeResult{
		Backlog:      integrationretention.BacklogCounts{CanonicalEvents: 42},
		BacklogKnown: true,
	}, errors.New("connection reset"))
	failed := gatherRetentionMetrics(t, metrics)
	if !strings.Contains(failed, `fi_fhir_retention_backlog_records{record_class="canonical_event"} 42`) {
		t.Fatalf("a failing tick published no backlog:\n%s", failed)
	}

	// And a tick that could not READ the backlog must leave the last known
	// value alone rather than publishing an unmeasured zero. "Not measured" and
	// "nothing is owed" are different claims, and a gauge can only carry one.
	observe(integrationretention.PurgeResult{}, errors.New("connection reset"))
	unmeasured := gatherRetentionMetrics(t, metrics)
	if !strings.Contains(unmeasured, `fi_fhir_retention_backlog_records{record_class="canonical_event"} 42`) {
		t.Fatalf("a tick with no backlog reading overwrote the gauge with an unmeasured zero:\n%s",
			unmeasured)
	}
}

func gatherRetentionMetrics(t *testing.T, metrics *observability.Metrics) string {
	t.Helper()
	values, err := observability.GatheredLabelValues(metrics.Registry())
	if err != nil {
		t.Fatalf("gather label values: %v", err)
	}
	for _, value := range values {
		// Two bounded sets reach retention exposition: `outcome` on the
		// counters and `record_class` on the Sprint 5 backlog gauge. Anything
		// else is an unbounded label and a PHI risk.
		if observability.KnownOutcome(value) || observability.KnownRetentionClass(value) {
			continue
		}
		if value == "test" {
			continue
		}
		t.Fatalf("retention metrics published the unbounded label %q", value)
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
			// Counters and gauges both: Sprint 5's retention backlog is a gauge,
			// and a renderer that silently skipped it would let an assertion on
			// the gauge pass by never seeing it.
			var value float64
			switch {
			case metric.GetCounter() != nil:
				value = metric.GetCounter().GetValue()
			case metric.GetGauge() != nil:
				value = metric.GetGauge().GetValue()
			default:
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
			builder.WriteString(" " + strconv.FormatFloat(value, 'f', -1, 64) + "\n")
		}
	}
	return builder.String()
}
