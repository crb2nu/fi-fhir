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

// TestConnectionSecretResolverResolvesOnlyConnectionSecrets is review W3's
// kill-test: a peek's draft names its own bindings and its own endpoint, so
// the resolver it gets must refuse the process's credentials — the GraphQL
// bearer token, any arbitrary variable, a destination's credential file — and
// resolve only what was provisioned for connections.
func TestConnectionSecretResolverResolvesOnlyConnectionSecrets(t *testing.T) {
	root := t.TempDir()
	writeDestinationSecret(t, root, "connections/adt-drop-access", "synthetic-connection-access")
	writeDestinationSecret(t, root, "destinations/epic-token", "synthetic-destination-token")
	t.Setenv("FI_FHIR_GRAPHQL_BEARER_TOKEN", "synthetic-graphql-bearer")
	t.Setenv("SYNTHETIC_ARBITRARY_VARIABLE", "synthetic-arbitrary-value")
	t.Setenv("FI_FHIR_CONNECTION_SECRET_ADT_DROP_ACCESS", "synthetic-env-access")
	t.Setenv("FI_FHIR_CONNECTION_SECRET_", "synthetic-bare-prefix")
	inner, err := newDestinationSecretResolver(root)
	if err != nil {
		t.Fatal(err)
	}
	resolver := connectionSecretResolver{inner: inner}
	env := func(key string) integration.SecretReference {
		return integration.SecretReference{Provider: integration.SecretProviderEnvironment, Key: key}
	}
	file := func(key string) integration.SecretReference {
		return integration.SecretReference{Provider: integration.SecretProviderFile, Key: key}
	}
	for name, reference := range map[string]integration.SecretReference{
		"the GraphQL bearer token":           env("FI_FHIR_GRAPHQL_BEARER_TOKEN"),
		"an arbitrary variable":              env("SYNTHETIC_ARBITRARY_VARIABLE"),
		"the bare prefix":                    env("FI_FHIR_CONNECTION_SECRET_"),
		"a destination credential file":      file("destinations/epic-token"),
		"a path escaping connections/":       file("connections/../destinations/epic-token"),
		"the connections directory itself":   file("connections/"),
		"a provider the resolver never uses": {Provider: integration.SecretProviderVault, Key: "FI_FHIR_CONNECTION_SECRET_ADT_DROP_ACCESS"},
	} {
		if material, err := resolver.Resolve(context.Background(), reference); !errors.Is(err, integration.ErrSecretUnresolvable) || material != nil {
			t.Errorf("%s resolved (%d bytes, %v); want ErrSecretUnresolvable", name, len(material), err)
		}
	}
	for name, test := range map[string]struct {
		reference integration.SecretReference
		want      string
	}{
		"a connection env secret":  {env("FI_FHIR_CONNECTION_SECRET_ADT_DROP_ACCESS"), "synthetic-env-access"},
		"a connection secret file": {file("connections/adt-drop-access"), "synthetic-connection-access"},
	} {
		material, err := resolver.Resolve(context.Background(), test.reference)
		if err != nil || string(material) != test.want {
			t.Errorf("%s = %q, %v; want the provisioned value", name, material, err)
		}
	}
	if _, err := (connectionSecretResolver{}).Resolve(context.Background(), env("FI_FHIR_CONNECTION_SECRET_ADT_DROP_ACCESS")); !errors.Is(err, integration.ErrSecretResolverUnavailable) {
		t.Fatalf("a resolver with nothing inside = %v, want ErrSecretResolverUnavailable", err)
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
