package workflow

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"text/template"
	"time"
)

// logAction logs an event to stdout/stderr.
func logAction(event interface{}, config map[string]string) error {
	level := config["level"]
	if level == "" {
		level = "info"
	}

	message := config["message"]
	if message == "" {
		message = "Event processed"
	}

	// Render template in message
	message = renderTemplate(message, event)

	// Format output
	timestamp := time.Now().Format(time.RFC3339)
	output := fmt.Sprintf("[%s] %s: %s", timestamp, strings.ToUpper(level), message)

	// Add event data if debug level
	if level == "debug" {
		eventJSON, _ := json.Marshal(event)
		output = fmt.Sprintf("%s\n  Event: %s", output, string(eventJSON))
	}

	// Write to appropriate output
	switch level {
	case "error", "warn":
		fmt.Fprintln(os.Stderr, output)
	default:
		fmt.Println(output)
	}

	return nil
}

// webhookAction sends an event to an HTTP endpoint.
func webhookAction(event interface{}, config map[string]string) error {
	url := config["url"]
	if url == "" {
		return fmt.Errorf("webhook action requires 'url' config")
	}

	method := config["method"]
	if method == "" {
		method = "POST"
	}

	// Render URL template
	url = renderTemplate(url, event)

	// Marshal event to JSON
	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Create request
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	if userAgent := config["user_agent"]; userAgent != "" {
		req.Header.Set("User-Agent", userAgent)
	} else {
		req.Header.Set("User-Agent", "fi-fhir/1.0")
	}

	// Add authorization if configured
	if token := config["token"]; token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if authHeader := config["authorization"]; authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}

	// Parse timeout
	timeout := 30 * time.Second
	if timeoutStr := config["timeout"]; timeoutStr != "" {
		if parsed, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = parsed
		}
	}

	// Execute request
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// renderTemplate renders a Go template string with event data.
func renderTemplate(tmplStr string, data interface{}) string {
	// Check if template contains any placeholders
	if !strings.Contains(tmplStr, "{{") {
		return tmplStr
	}

	tmpl, err := template.New("msg").Parse(tmplStr)
	if err != nil {
		log.Printf("Warning: failed to parse template: %v", err)
		return tmplStr
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		log.Printf("Warning: failed to execute template: %v", err)
		return tmplStr
	}

	return buf.String()
}

// FHIRAction is a placeholder for FHIR server integration (Phase 2).
func fhirAction(event interface{}, config map[string]string) error {
	endpoint := config["endpoint"]
	if endpoint == "" {
		return fmt.Errorf("fhir action requires 'endpoint' config")
	}

	resource := config["resource"]
	if resource == "" {
		return fmt.Errorf("fhir action requires 'resource' config")
	}

	// TODO: Implement FHIR mapping and API calls
	return fmt.Errorf("FHIR action not yet implemented")
}
