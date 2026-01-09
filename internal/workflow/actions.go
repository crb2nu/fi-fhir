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

	"github.com/cblevins/fi-fhir/pkg/events"
	"github.com/cblevins/fi-fhir/pkg/fhir"
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

// fhirAction sends events to a FHIR R4 server.
// Config options:
//   - endpoint: FHIR server base URL (required)
//   - resource: Resource type to create (Patient, Encounter, Observation) - auto-detected if not specified
//   - operation: create, update, or upsert (default: create)
//   - profile: us-core or base (default: us-core)
//   - token: Bearer token for authentication (static)
//   - authorization: Custom Authorization header
//   - token_url: OAuth2 token endpoint (enables OAuth2 client credentials)
//   - client_id: OAuth2 client ID
//   - client_secret: OAuth2 client secret
//   - scopes: OAuth2 scopes (space or comma separated)
//   - timeout: Request timeout duration (default: 30s)
//   - bundle: "true" to send as transaction bundle
func fhirAction(event interface{}, config map[string]string) error {
	endpoint := config["endpoint"]
	if endpoint == "" {
		return fmt.Errorf("fhir action requires 'endpoint' config")
	}

	// Ensure endpoint doesn't have trailing slash
	endpoint = strings.TrimSuffix(endpoint, "/")

	// Create mapper (us-core is default)
	mapper := fhir.NewUSCoreMapper()

	// Convert event to FHIR resources
	resources, err := eventToFHIRResources(event, mapper, config)
	if err != nil {
		return fmt.Errorf("failed to convert event to FHIR: %w", err)
	}

	if len(resources) == 0 {
		return fmt.Errorf("no FHIR resources generated from event")
	}

	// Parse timeout
	timeout := 30 * time.Second
	if timeoutStr := config["timeout"]; timeoutStr != "" {
		if parsed, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = parsed
		}
	}

	client := &http.Client{Timeout: timeout}

	// Determine if we should send as bundle
	useBundle := strings.ToLower(config["bundle"]) == "true" || len(resources) > 1

	if useBundle {
		return sendFHIRBundle(client, endpoint, resources, config)
	}

	// Send individual resources
	for _, resource := range resources {
		if err := sendFHIRResource(client, endpoint, resource, config); err != nil {
			return err
		}
	}

	return nil
}

// eventToFHIRResources converts a canonical event to FHIR resources.
func eventToFHIRResources(event interface{}, mapper *fhir.USCoreMapper, config map[string]string) ([]fhir.Resource, error) {
	var resources []fhir.Resource

	// Handle map types (from JSON parsing in workflow engine)
	if m, ok := event.(map[string]interface{}); ok {
		return mapEventToFHIR(m, mapper, config)
	}

	// Handle typed events
	switch e := event.(type) {
	case *events.PatientAdmitEvent:
		patient := mapper.MapPatient(&e.Patient)
		if patient != nil {
			resources = append(resources, patient)
		}
		encounter := mapper.MapEncounter(&e.Encounter, fmt.Sprintf("Patient/%s", e.Patient.MRN))
		if encounter != nil {
			resources = append(resources, encounter)
		}

	case events.PatientAdmitEvent:
		patient := mapper.MapPatient(&e.Patient)
		if patient != nil {
			resources = append(resources, patient)
		}
		encounter := mapper.MapEncounter(&e.Encounter, fmt.Sprintf("Patient/%s", e.Patient.MRN))
		if encounter != nil {
			resources = append(resources, encounter)
		}

	case *events.PatientDischargeEvent:
		patient := mapper.MapPatient(&e.Patient)
		if patient != nil {
			resources = append(resources, patient)
		}
		encounter := mapper.MapEncounter(&e.Encounter, fmt.Sprintf("Patient/%s", e.Patient.MRN))
		if encounter != nil {
			resources = append(resources, encounter)
		}

	case events.PatientDischargeEvent:
		patient := mapper.MapPatient(&e.Patient)
		if patient != nil {
			resources = append(resources, patient)
		}
		encounter := mapper.MapEncounter(&e.Encounter, fmt.Sprintf("Patient/%s", e.Patient.MRN))
		if encounter != nil {
			resources = append(resources, encounter)
		}

	case *events.LabResultEvent:
		report, observations := mapper.MapLabResult(e)
		if report != nil {
			resources = append(resources, report)
		}
		for _, obs := range observations {
			resources = append(resources, obs)
		}

	case events.LabResultEvent:
		report, observations := mapper.MapLabResult(&e)
		if report != nil {
			resources = append(resources, report)
		}
		for _, obs := range observations {
			resources = append(resources, obs)
		}

	default:
		// Try to extract patient data from generic event
		return nil, fmt.Errorf("unsupported event type: %T", event)
	}

	return resources, nil
}

// mapEventToFHIR converts a map-based event (from JSON) to FHIR resources.
func mapEventToFHIR(m map[string]interface{}, mapper *fhir.USCoreMapper, config map[string]string) ([]fhir.Resource, error) {
	// Re-serialize to JSON and parse into typed event
	data, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal event map: %w", err)
	}

	// Determine event type
	eventType, _ := m["type"].(string)

	switch eventType {
	case string(events.EventPatientAdmit):
		var e events.PatientAdmitEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse patient_admit event: %w", err)
		}
		return eventToFHIRResources(&e, mapper, config)

	case string(events.EventPatientDischarge):
		var e events.PatientDischargeEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse patient_discharge event: %w", err)
		}
		return eventToFHIRResources(&e, mapper, config)

	case string(events.EventLabResult):
		var e events.LabResultEvent
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("failed to parse lab_result event: %w", err)
		}
		return eventToFHIRResources(&e, mapper, config)

	default:
		// Try to extract patient from generic event structure
		if patientData, ok := m["patient"].(map[string]interface{}); ok {
			patientJSON, _ := json.Marshal(patientData)
			var patient events.Patient
			if err := json.Unmarshal(patientJSON, &patient); err == nil {
				fhirPatient := mapper.MapPatient(&patient)
				if fhirPatient != nil {
					return []fhir.Resource{fhirPatient}, nil
				}
			}
		}
		return nil, fmt.Errorf("unsupported event type: %s", eventType)
	}
}

// sendFHIRResource sends a single FHIR resource to the server.
func sendFHIRResource(client *http.Client, endpoint string, resource fhir.Resource, config map[string]string) error {
	resourceType := resource.GetResourceType()
	operation := config["operation"]
	if operation == "" {
		operation = "create"
	}

	// Marshal resource to JSON
	body, err := json.Marshal(resource)
	if err != nil {
		return fmt.Errorf("failed to marshal FHIR resource: %w", err)
	}

	// Build request URL and method
	var url, method string
	switch operation {
	case "create":
		url = fmt.Sprintf("%s/%s", endpoint, resourceType)
		method = "POST"
	case "update":
		// For update, we need the resource ID
		url = fmt.Sprintf("%s/%s", endpoint, resourceType)
		method = "PUT"
	case "upsert":
		// Upsert using conditional create (POST with If-None-Exist)
		url = fmt.Sprintf("%s/%s", endpoint, resourceType)
		method = "POST"
	default:
		return fmt.Errorf("unknown operation: %s", operation)
	}

	// Create request
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create FHIR request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/fhir+json")
	req.Header.Set("Accept", "application/fhir+json")
	req.Header.Set("User-Agent", "fi-fhir/1.0")

	// Add authentication (OAuth2 or static token)
	if err := addAuth(req, config); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("FHIR request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("FHIR server returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// sendFHIRBundle sends multiple resources as a transaction bundle.
func sendFHIRBundle(client *http.Client, endpoint string, resources []fhir.Resource, config map[string]string) error {
	bundle := fhir.CreateTransactionBundle(resources)

	// Marshal bundle to JSON
	body, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("failed to marshal FHIR bundle: %w", err)
	}

	// Create request
	req, err := http.NewRequest("POST", endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create FHIR bundle request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/fhir+json")
	req.Header.Set("Accept", "application/fhir+json")
	req.Header.Set("User-Agent", "fi-fhir/1.0")

	// Add authentication (OAuth2 or static token)
	if err := addAuth(req, config); err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("FHIR bundle request failed: %w", err)
	}
	defer resp.Body.Close()

	// Check response
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("FHIR server returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// addAuth adds authentication to a request using OAuth2 or static token.
func addAuth(req *http.Request, config map[string]string) error {
	// Check for custom Authorization header first (highest priority)
	if authHeader := config["authorization"]; authHeader != "" {
		req.Header.Set("Authorization", authHeader)
		return nil
	}

	// Get token (OAuth2 or static)
	token, err := getAuthToken(config)
	if err != nil {
		return err
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	return nil
}
