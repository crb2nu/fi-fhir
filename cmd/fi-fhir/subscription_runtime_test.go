package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestSubscriptionStatus_FindsMatchingSubscriptionOnServer(t *testing.T) {
	const endpoint = "http://example.test/fhir/notify/patient_changes"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/Subscription" {
			http.NotFound(w, r)
			return
		}
		resp := map[string]any{
			"resourceType": "Bundle",
			"type":         "searchset",
			"entry": []any{
				map[string]any{
					"resource": map[string]any{
						"resourceType": "Subscription",
						"id":           "sub-1",
						"status":       "active",
						"channel": map[string]any{
							"type":     "rest-hook",
							"endpoint": endpoint,
						},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "subscriptions.yaml")
	if err := os.WriteFile(cfgPath, []byte(
		`subscriptions:
  - name: patient_changes
    server: `+srv.URL+`
    criteria: "Patient?"
    channel:
      endpoint: "`+endpoint+`"
`), 0o600); err != nil {
		t.Fatalf("write subscriptions.yaml: %v", err)
	}

	stdout, _, err := runCLI(t, "subscription", "status", "--config", cfgPath, "--name", "patient_changes")
	assertNoError(t, err)
	assertContains(t, stdout, "Server Status")
	assertContains(t, stdout, "Status:")
	assertContains(t, stdout, "active")
}

func TestSubscriptionStatus_NotRegisteredOnServer(t *testing.T) {
	const endpoint = "http://example.test/fhir/notify/patient_changes"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/Subscription" {
			http.NotFound(w, r)
			return
		}
		resp := map[string]any{
			"resourceType": "Bundle",
			"type":         "searchset",
			"entry":        []any{}, // no subscriptions
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "subscriptions.yaml")
	if err := os.WriteFile(cfgPath, []byte(
		`subscriptions:
  - name: patient_changes
    server: `+srv.URL+`
    criteria: "Patient?"
    channel:
      endpoint: "`+endpoint+`"
`), 0o600); err != nil {
		t.Fatalf("write subscriptions.yaml: %v", err)
	}

	stdout, _, err := runCLI(t, "subscription", "status", "--config", cfgPath, "--name", "patient_changes")
	assertNoError(t, err)
	assertContains(t, stdout, "NOT REGISTERED")
}

func TestSubscriptionServe_DryRun_Defaults(t *testing.T) {
	tmpDir := t.TempDir()
	subsPath := filepath.Join(tmpDir, "subscriptions.yaml")

	if err := os.WriteFile(subsPath, []byte(
		`subscriptions:
  - name: patient_changes
    server: http://example.test/fhir
    criteria: "Patient?"
    channel:
      endpoint: "http://localhost:8081/fhir/notify/patient_changes"
`), 0o600); err != nil {
		t.Fatalf("write subscriptions.yaml: %v", err)
	}

	stdout, _, err := runCLI(t, "subscription", "serve", "--subscriptions", subsPath, "--dry-run")
	assertNoError(t, err)
	assertContains(t, stdout, "Dry-run: subscription receiver config")
	assertContains(t, stdout, "TLS: disabled")
	assertContains(t, stdout, "Workflow: (none)")
}

func TestSubscriptionServe_DryRun_UsesReceiverConfigAndTLS(t *testing.T) {
	tmpDir := t.TempDir()

	subsPath := filepath.Join(tmpDir, "subscriptions.yaml")
	if err := os.WriteFile(subsPath, []byte(
		`subscriptions:
  - name: patient_changes
    server: http://example.test/fhir
    criteria: "Patient?"
    channel:
      endpoint: "http://localhost:8081/fhir/notify/patient_changes"
`), 0o600); err != nil {
		t.Fatalf("write subscriptions.yaml: %v", err)
	}

	certPath := filepath.Join(tmpDir, "cert.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")
	_ = os.WriteFile(certPath, []byte(""), 0o600)
	_ = os.WriteFile(keyPath, []byte(""), 0o600)

	appCfgPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(appCfgPath, []byte(
		`subscription_receiver:
  host: 127.0.0.1
  port: 9090
  path_prefix: fhir/notify
  max_bundle_size: 123
  verify_source: true
  tls:
    enabled: true
    cert_file: `+certPath+`
    key_file: `+keyPath+`
`), 0o600); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	stdout, _, err := runCLI(t,
		"subscription", "serve",
		"--subscriptions", subsPath,
		"--config", appCfgPath,
		"--dry-run",
	)
	assertNoError(t, err)
	assertContains(t, stdout, "Bind: 127.0.0.1:9090")
	assertContains(t, stdout, "Path prefix: /fhir/notify")
	assertContains(t, stdout, "TLS: enabled")
}

func TestSubscriptionServe_TLSCertKeyMismatch_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	subsPath := filepath.Join(tmpDir, "subscriptions.yaml")

	if err := os.WriteFile(subsPath, []byte(
		`subscriptions:
  - name: patient_changes
    server: http://example.test/fhir
    criteria: "Patient?"
    channel:
      endpoint: "http://localhost:8081/fhir/notify/patient_changes"
`), 0o600); err != nil {
		t.Fatalf("write subscriptions.yaml: %v", err)
	}

	_, _, err := runCLI(t, "subscription", "serve", "--subscriptions", subsPath, "--cert", filepath.Join(tmpDir, "cert.pem"), "--dry-run")
	assertError(t, err)
	assertErrorContains(t, err, "both TLS cert and key are required")
}

func TestSubscriptionServe_PortInUse_ReturnsError(t *testing.T) {
	tmpDir := t.TempDir()
	subsPath := filepath.Join(tmpDir, "subscriptions.yaml")

	if err := os.WriteFile(subsPath, []byte(
		`subscriptions:
  - name: patient_changes
    server: http://example.test/fhir
    criteria: "Patient?"
    channel:
      endpoint: "http://localhost:8081/fhir/notify/patient_changes"
`), 0o600); err != nil {
		t.Fatalf("write subscriptions.yaml: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	port := ln.Addr().(*net.TCPAddr).Port

	// Binding to an already-used port forces server.Start() to fail quickly,
	// exercising the non-dry-run path without hanging tests.
	_, _, err = runCLI(t,
		"subscription", "serve",
		"--subscriptions", subsPath,
		"--host", "127.0.0.1",
		"--port", strconv.Itoa(port),
	)
	assertError(t, err)
}
