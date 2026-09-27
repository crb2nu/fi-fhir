package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/connection"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/observability"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

type passThroughProcessor struct{}

func (passThroughProcessor) Process(context.Context, integration.ProcessRequest) (integration.ProcessResult, error) {
	return integration.ProcessResult{}, nil
}

// TestConnectionIntakeLabelsAreTheConnectionPackagesOwn pins the metric label
// allowlists to the values the connection package reports, so a new tap
// failure cannot reach an exposition as an unknown label, and cannot vanish
// into a dropped one either.
func TestConnectionIntakeLabelsAreTheConnectionPackagesOwn(t *testing.T) {
	failures := connection.TapFailures()
	for _, failure := range failures {
		if !observability.KnownCaptureTapError(string(failure)) {
			t.Errorf("tap failure %q is not an allowlisted reason label", failure)
		}
	}
	for _, reason := range []string{
		observability.CaptureTapErrorLedger, observability.CaptureTapErrorSessionStore,
		observability.CaptureTapErrorRefresh, observability.CaptureTapErrorPanic,
	} {
		found := false
		for _, failure := range failures {
			found = found || string(failure) == reason
		}
		if !found {
			t.Errorf("reason label %q names no connection.TapFailure", reason)
		}
	}
	for _, mode := range []connection.CaptureMode{connection.CaptureModePeek, connection.CaptureModeStream} {
		if !observability.KnownCaptureMode(string(mode)) {
			t.Errorf("capture mode %q is not an allowlisted mode label", mode)
		}
	}
}

func TestConnectionIntakeWrapsAdmissionOnlyWithASessionWorkspace(t *testing.T) {
	inner := passThroughProcessor{}
	off := newConnectionIntakeRuntime(false)
	if off.tapMLLP(inner) != inner || off.tapIngress(inner) != inner {
		t.Fatal("a replica without a session workspace wrapped its admission processors")
	}
	if enabled, err := off.bind(context.Background(), connectionIntakeBinding{}); enabled || err != nil {
		t.Fatalf("bind without a session workspace = %v, %v", enabled, err)
	}
	on := newConnectionIntakeRuntime(true)
	if on.tapMLLP(inner) == inner || on.tapIngress(inner) == inner {
		t.Fatal("a replica with a session workspace did not tap admission")
	}
	// Without a catalog service there is nothing to bind to; the taps stay
	// pass-throughs.
	if enabled, err := on.bind(context.Background(), connectionIntakeBinding{}); enabled || err != nil {
		t.Fatalf("bind without a catalog = %v, %v", enabled, err)
	}
}

func TestCaptureObserverMetersBoundedLabels(t *testing.T) {
	metrics := observability.NewMetrics("test")
	observer := captureObserver(metrics, nil)
	observer.Captured(connection.CaptureModeStream, 2)
	observer.Captured(connection.CaptureModePeek, 5)
	observer.TapFailed(connection.TapFailureSessionStore, errors.New("session store closed"))
	observer.TapFailed(connection.TapFailure("Synthetic^Patient"), errors.New("never a label"))

	values, err := observability.GatheredLabelValues(metrics.Registry())
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, value := range values {
		seen[value] = true
		if strings.Contains(value, "Synthetic") {
			t.Fatalf("an unbounded value reached a label: %q", value)
		}
	}
	for _, want := range []string{"stream", "peek", "session_store"} {
		if !seen[want] {
			t.Errorf("label %q was not recorded", want)
		}
	}
}
