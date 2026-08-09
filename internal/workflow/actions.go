package workflow

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
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

	// Add event shape if debug level.
	//
	// The event is message content: by 4.2a's standard it may carry PHI, and
	// this action writes to stdout, which a log aggregator indexes and retains.
	// Marshalling the whole event here made `level: "debug"` a one-word switch
	// that turns every matched route into a payload dump. Record the shape, not
	// the values. See `.loom/40-decisions.md` 2026-08-09.
	//
	// The rendered `message` above is deliberately left alone: emitting an
	// author-written, event-templated string is this action's entire purpose,
	// and what a workflow author interpolates into it is their choice to make.
	if level == "debug" {
		output = fmt.Sprintf("%s\n  Event: %s", output, describeEventShape(event))
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

// describeEventShape summarises an event for a debug line without reproducing
// any of its values. It reports the serialized size and the top-level field
// count, both of which answer "did the event I expected arrive" — the question
// debug logging is for — while carrying nothing a compliance reviewer has to
// read.
func describeEventShape(event interface{}) string {
	data, err := json.Marshal(event)
	if err != nil {
		return "<redacted: event is not serializable>"
	}
	fields := 0
	var probe map[string]json.RawMessage
	if json.Unmarshal(data, &probe) == nil {
		fields = len(probe)
	}
	return fmt.Sprintf("<redacted: %d bytes, %d top-level fields>", len(data), fields)
}

// fileAction writes an event to a local file (useful for debugging, archives, and simple ETL).
// Config options:
//   - path: output file path (required, supports templates)
//   - base_dir: if set, path is resolved under this directory and cannot escape it
//   - format: json (default), pretty, ndjson
//   - perm: octal file mode (default: 0600)
func fileAction(event interface{}, config map[string]string) error {
	rawPath := strings.TrimSpace(config["path"])
	if rawPath == "" {
		return fmt.Errorf("file action requires 'path' config")
	}

	// Render path template
	rawPath = renderTemplate(rawPath, event)

	baseDir := strings.TrimSpace(config["base_dir"])
	resolvedPath, err := resolvePathUnderBaseDir(baseDir, rawPath)
	if err != nil {
		return err
	}

	format := strings.ToLower(strings.TrimSpace(config["format"]))
	if format == "" {
		format = "json"
	}

	perm := os.FileMode(0o600)
	if p := strings.TrimSpace(config["perm"]); p != "" {
		parsed, err := strconv.ParseUint(p, 8, 32)
		if err != nil {
			return fmt.Errorf("invalid file perm %q (expected octal like 0600): %w", p, err)
		}
		perm = os.FileMode(parsed)
	}

	var data []byte
	switch format {
	case "json":
		data, err = json.Marshal(event)
	case "pretty":
		data, err = json.MarshalIndent(event, "", "  ")
	case "ndjson":
		data, err = json.Marshal(event)
		if err == nil {
			data = append(data, '\n')
		}
	default:
		return fmt.Errorf("invalid file format %q (expected json, pretty, or ndjson)", format)
	}
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	dir := filepath.Dir(resolvedPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("failed to create output dir %q: %w", dir, err)
	}

	if format == "ndjson" {
		f, err := os.OpenFile(resolvedPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, perm)
		if err != nil {
			return fmt.Errorf("failed to open output file %q: %w", resolvedPath, err)
		}
		defer func() { _ = f.Close() }()
		if _, err := f.Write(data); err != nil {
			return fmt.Errorf("failed to write output file %q: %w", resolvedPath, err)
		}
		return nil
	}

	if err := atomicWriteFile(resolvedPath, data, perm); err != nil {
		return err
	}
	return nil
}

func resolvePathUnderBaseDir(baseDir, rawPath string) (string, error) {
	cleanBase := filepath.Clean(baseDir)
	if baseDir == "" {
		return filepath.Clean(rawPath), nil
	}

	var resolved string
	if filepath.IsAbs(rawPath) {
		resolved = filepath.Clean(rawPath)
	} else {
		resolved = filepath.Clean(filepath.Join(cleanBase, rawPath))
	}

	rel, err := filepath.Rel(cleanBase, resolved)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path %q under base_dir %q: %w", rawPath, baseDir, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("file action path %q escapes base_dir %q", rawPath, baseDir)
	}
	return resolved, nil
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".fi-fhir-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file in %q: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if err := tmp.Chmod(perm); err != nil {
		return fmt.Errorf("failed to chmod temp file: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to move temp file into place: %w", err)
	}
	return nil
}

// emailAction sends an email notification (SMTP).
// Config options:
//   - smtp_host: SMTP hostname (required)
//   - smtp_port: SMTP port (required)
//   - starttls: "true" to use STARTTLS (default: false)
//   - tls_insecure: "true" to skip TLS cert verification (default: false)
//   - username/password: SMTP auth (optional; uses PLAIN)
//   - from: From address (required, supports templates)
//   - to: Comma-separated recipient list (required, supports templates)
//   - subject: Email subject (required, supports templates)
//   - body: Email body (optional, supports templates)
//   - content_type: MIME content type (default: text/plain; charset=utf-8)
//   - timeout: Dial/send timeout (default: 30s)
func emailAction(ctx context.Context, event interface{}, config map[string]string) error {
	host := strings.TrimSpace(config["smtp_host"])
	port := strings.TrimSpace(config["smtp_port"])
	if host == "" {
		return fmt.Errorf("email action requires 'smtp_host' config")
	}
	if port == "" {
		return fmt.Errorf("email action requires 'smtp_port' config")
	}

	timeout := 30 * time.Second
	if timeoutStr := strings.TrimSpace(config["timeout"]); timeoutStr != "" {
		if parsed, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = parsed
		}
	}

	from := strings.TrimSpace(renderTemplate(config["from"], event))
	to := strings.TrimSpace(renderTemplate(config["to"], event))
	subject := renderTemplate(config["subject"], event)
	body := renderTemplate(config["body"], event)
	contentType := strings.TrimSpace(config["content_type"])
	if contentType == "" {
		contentType = "text/plain; charset=utf-8"
	}

	if from == "" {
		return fmt.Errorf("email action requires 'from' config")
	}
	if to == "" {
		return fmt.Errorf("email action requires 'to' config")
	}
	if strings.TrimSpace(subject) == "" {
		return fmt.Errorf("email action requires 'subject' config")
	}

	var recipients []string
	for _, r := range strings.Split(to, ",") {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		recipients = append(recipients, r)
	}
	if len(recipients) == 0 {
		return fmt.Errorf("email action requires at least one recipient in 'to'")
	}

	addr := net.JoinHostPort(host, port)
	dialer := &net.Dialer{Timeout: timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("failed to dial SMTP server %q: %w", addr, err)
	}
	defer func() { _ = conn.Close() }()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	if err := sendSMTP(ctx, conn, host, config, from, recipients, subject, body, contentType); err != nil {
		return err
	}
	return nil
}

func sendSMTP(ctx context.Context, conn net.Conn, host string, config map[string]string, from string, to []string, subject string, body string, contentType string) error {
	client, err := smtp.NewClient(conn, host)
	if err != nil {
		return fmt.Errorf("failed to create SMTP client: %w", err)
	}
	defer func() { _ = client.Close() }()

	if strings.ToLower(strings.TrimSpace(config["starttls"])) == "true" {
		tlsConfig := &tls.Config{
			ServerName: host,
			MinVersion: tls.VersionTLS12,
		}
		if strings.ToLower(strings.TrimSpace(config["tls_insecure"])) == "true" {
			tlsConfig.InsecureSkipVerify = true
		}
		if err := client.StartTLS(tlsConfig); err != nil {
			return fmt.Errorf("failed to start TLS: %w", err)
		}
	}

	username := strings.TrimSpace(config["username"])
	password := config["password"]
	if username != "" {
		if err := client.Auth(smtp.PlainAuth("", username, password, host)); err != nil {
			return fmt.Errorf("SMTP auth failed: %w", err)
		}
	}

	if err := client.Mail(from); err != nil {
		return fmt.Errorf("SMTP MAIL FROM failed: %w", err)
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("SMTP RCPT TO %q failed: %w", rcpt, err)
		}
	}

	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA failed: %w", err)
	}

	msg := buildEmailMessage(from, to, subject, body, contentType)
	_, writeErr := w.Write([]byte(msg))
	closeErr := w.Close()

	if writeErr != nil {
		return fmt.Errorf("SMTP write failed: %w", writeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("SMTP DATA close failed: %w", closeErr)
	}

	select {
	case <-ctx.Done():
		return fmt.Errorf("SMTP send canceled: %w", ctx.Err())
	default:
	}

	if err := client.Quit(); err != nil {
		return fmt.Errorf("SMTP QUIT failed: %w", err)
	}
	return nil
}

func buildEmailMessage(from string, to []string, subject string, body string, contentType string) string {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(to, ", ") + "\r\n")
	b.WriteString("Subject: " + strings.TrimSpace(subject) + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: " + contentType + "\r\n")
	b.WriteString("\r\n")
	b.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

// execAction runs an external command (no shell) for custom integrations.
// This is intentionally strict: the command must be in an allowlist.
//
// Config options:
//   - command: absolute path to executable (required)
//   - allowlist: comma-separated absolute paths allowed to run (required)
//   - args: command args as JSON array (e.g. ["--flag","x"]) or a whitespace-separated string
//   - timeout: execution timeout (default: 30s)
//   - stdin: json (default), none, template
//   - stdin_template: used when stdin=template (supports templates)
//   - env_*: environment variables to set for the process (e.g. env_FOO: "bar")
func execAction(ctx context.Context, event interface{}, config map[string]string) error {
	command := strings.TrimSpace(config["command"])
	if command == "" {
		return fmt.Errorf("exec action requires 'command' config")
	}
	if !filepath.IsAbs(command) {
		return fmt.Errorf("exec action command must be an absolute path (got %q)", command)
	}

	allowlist := strings.TrimSpace(config["allowlist"])
	if allowlist == "" {
		return fmt.Errorf("exec action requires 'allowlist' config")
	}
	allowed := false
	for _, item := range strings.Split(allowlist, ",") {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if item == command {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("exec action command %q is not in allowlist", command)
	}

	timeout := 30 * time.Second
	if timeoutStr := strings.TrimSpace(config["timeout"]); timeoutStr != "" {
		parsed, err := time.ParseDuration(timeoutStr)
		if err != nil {
			return fmt.Errorf("invalid exec timeout %q: %w", timeoutStr, err)
		}
		timeout = parsed
	}

	args, err := parseExecArgs(renderTemplate(config["args"], event))
	if err != nil {
		return err
	}

	stdinMode := strings.ToLower(strings.TrimSpace(config["stdin"]))
	if stdinMode == "" {
		stdinMode = "json"
	}

	var stdin []byte
	switch stdinMode {
	case "none":
		stdin = nil
	case "json":
		stdin, err = json.Marshal(event)
		if err != nil {
			return fmt.Errorf("failed to marshal event for stdin: %w", err)
		}
	case "template":
		stdin = []byte(renderTemplate(config["stdin_template"], event))
	default:
		return fmt.Errorf("invalid stdin mode %q (expected json, none, or template)", stdinMode)
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Env = append([]string{}, os.Environ()...)
	for k, v := range config {
		if !strings.HasPrefix(k, "env_") {
			continue
		}
		key := strings.TrimPrefix(k, "env_")
		if key == "" {
			continue
		}
		cmd.Env = append(cmd.Env, key+"="+renderTemplate(v, event))
	}
	if len(stdin) > 0 {
		cmd.Stdin = bytes.NewReader(stdin)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(truncateForError(stdout.String(), 4096))
		errOut := strings.TrimSpace(truncateForError(stderr.String(), 4096))
		msg := "exec failed"
		if out != "" {
			msg += "; stdout: " + out
		}
		if errOut != "" {
			msg += "; stderr: " + errOut
		}
		return fmt.Errorf("%s: %w", msg, err)
	}

	return nil
}

func parseExecArgs(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "[") {
		var arr []string
		if err := json.Unmarshal([]byte(raw), &arr); err != nil {
			return nil, fmt.Errorf("failed to parse args as JSON array: %w", err)
		}
		return arr, nil
	}
	return strings.Fields(raw), nil
}

func truncateForError(s string, limit int) string {
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "...(truncated)"
}

// webhookAction sends an event to an HTTP endpoint with rate limiting, retry, and circuit breaker support.
// Config options:
//   - url: Target URL (required, supports templates)
//   - method: HTTP method (default: POST)
//   - token: Bearer token for authorization
//   - authorization: Custom Authorization header
//   - user_agent: Custom User-Agent header
//   - timeout: Request timeout duration (default: 30s)
//   - retry_max: Max retry attempts (default: 3, set 0 to disable)
//   - retry_delay: Initial retry delay (default: 1s)
//   - retry_max_delay: Max retry delay cap (default: 30s)
//   - retry_multiplier: Backoff multiplier (default: 2.0)
//   - retry_jitter: Jitter factor 0.0-1.0 (default: 0.1)
//   - retry_on_status: Comma-separated status codes to retry (default: 429,500,502,503,504)
//   - circuit_breaker: "true" to enable circuit breaker (default: disabled)
//   - circuit_failure_threshold: Failures before opening (default: 5)
//   - circuit_success_threshold: Successes in half-open before closing (default: 2)
//   - circuit_timeout: Time in open state before half-open (default: 30s)
//   - rate_limit: "true" to enable rate limiting (default: disabled)
//   - rate_limit_rate: Requests per second (default: 10)
//   - rate_limit_burst: Maximum burst size (default: 20)
//   - rate_limit_wait: "true" to wait when limited, "false" to fail fast (default: true)
func webhookAction(ctx context.Context, event interface{}, config map[string]string) error {
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

	// Parse timeout
	timeout := 30 * time.Second
	if timeoutStr := config["timeout"]; timeoutStr != "" {
		if parsed, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = parsed
		}
	}

	client := &http.Client{Timeout: timeout}

	// Parse retry configuration
	retryConfig := ParseRetryConfig(config)

	// Build headers once (reused for retries)
	headers := http.Header{}
	headers.Set("Content-Type", "application/json")
	if userAgent := config["user_agent"]; userAgent != "" {
		headers.Set("User-Agent", userAgent)
	} else {
		headers.Set("User-Agent", "fi-fhir/1.0")
	}
	if token := config["token"]; token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}
	if authHeader := config["authorization"]; authHeader != "" {
		headers.Set("Authorization", authHeader)
	}

	// Parse circuit breaker configuration (nil if disabled)
	cbConfig := ParseCircuitBreakerConfig(config)
	var cb *CircuitBreaker
	if cbConfig != nil {
		cb = GetCircuitBreaker(url)
	}

	// Parse rate limit configuration (nil if disabled)
	rlConfig := ParseRateLimitConfig(config)
	var limiter RateLimiter
	if rlConfig != nil {
		// Get or create limiter for this endpoint with the specified config
		limiter = GetOrCreateRateLimiter(url, *rlConfig)
	}
	waitOnRateLimit := ShouldWaitOnRateLimit(config)

	// Start HTTP span for distributed tracing
	tracer := GetGlobalTracer()
	httpCtx, httpSpan := tracer.StartSpan(ctx, SpanNameHTTP,
		WithSpanKind(SpanKindClient),
		WithAttributes(
			Attr(AttrHTTPMethod, method),
			Attr(AttrHTTPURL, url),
		),
	)
	defer httpSpan.End()

	// Execute request with rate limit, circuit breaker, and retry
	var resp *http.Response
	httpStart := time.Now()

	err = WithRateLimit(httpCtx, limiter, waitOnRateLimit, func() error {
		var innerErr error
		//nolint:bodyclose // Body is closed via defer after error check below
		resp, innerErr = WithCircuitBreaker(cb, func() (*http.Response, error) {
			return WithRetry(httpCtx, retryConfig, func() (*http.Response, error) {
				req, reqErr := http.NewRequestWithContext(httpCtx, method, url, bytes.NewReader(body))
				if reqErr != nil {
					return nil, fmt.Errorf("failed to create request: %w", reqErr)
				}
				req.Header = headers.Clone()
				return client.Do(req)
			})
		})
		return innerErr
	})

	httpDuration := time.Since(httpStart)

	if err != nil {
		// Record failed HTTP request metric (status code 0 for network errors)
		GetGlobalMetrics().HTTPRequestCompleted(url, method, 0, httpDuration)
		httpSpan.SetAttribute(AttrHTTPStatus, 0)
		httpSpan.RecordError(err)
		httpSpan.SetStatus(SpanStatusError, err.Error())
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Record span attributes and metrics
	httpSpan.SetAttribute(AttrHTTPStatus, resp.StatusCode)
	GetGlobalMetrics().HTTPRequestCompleted(url, method, resp.StatusCode, httpDuration)

	// Check response
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		httpSpan.SetStatus(SpanStatusError, fmt.Sprintf("HTTP %d", resp.StatusCode))
		return fmt.Errorf("webhook returned %d: %s", resp.StatusCode, string(respBody))
	}

	httpSpan.SetStatus(SpanStatusOK, "")
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
//   - validate_fhir: "true" to validate generated JSON before sending (default: false)
//   - validate_mode: "us-core" or "none" (default: derived from profile; falls back to us-core)
//   - allow_warnings: "true" to allow warnings when validate_fhir=true (default: false)
//   - token: Bearer token for authentication (static)
//   - authorization: Custom Authorization header
//   - token_url: OAuth2 token endpoint (enables OAuth2 client credentials)
//   - client_id: OAuth2 client ID
//   - client_secret: OAuth2 client secret
//   - scopes: OAuth2 scopes (space or comma separated)
//   - timeout: Request timeout duration (default: 30s)
//   - bundle: "true" to send as transaction bundle
//   - circuit_breaker: "true" to enable circuit breaker (default: disabled)
//   - circuit_failure_threshold: Failures before opening (default: 5)
//   - circuit_success_threshold: Successes in half-open before closing (default: 2)
//   - circuit_timeout: Time in open state before half-open (default: 30s)
//   - rate_limit: "true" to enable rate limiting (default: disabled)
//   - rate_limit_rate: Requests per second (default: 10)
//   - rate_limit_burst: Maximum burst size (default: 20)
//   - rate_limit_wait: "true" to wait when limited, "false" to fail fast (default: true)
func fhirAction(ctx context.Context, event interface{}, config map[string]string) error {
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
		return sendFHIRBundle(ctx, client, endpoint, resources, config)
	}

	// Send individual resources
	for _, resource := range resources {
		if err := sendFHIRResource(ctx, client, endpoint, resource, config); err != nil {
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

func shouldValidateFHIR(config map[string]string) bool {
	return strings.ToLower(strings.TrimSpace(config["validate_fhir"])) == "true" ||
		strings.ToLower(strings.TrimSpace(config["validate"])) == "true"
}

func fhirValidationMode(config map[string]string) string {
	if mode := strings.TrimSpace(config["validate_mode"]); mode != "" {
		return mode
	}

	switch strings.ToLower(strings.TrimSpace(config["profile"])) {
	case "base", "none":
		return "none"
	case "us-core", "uscore", "us_core":
		return "us-core"
	default:
		return "us-core"
	}
}

func allowFHIRWarnings(config map[string]string) bool {
	return strings.ToLower(strings.TrimSpace(config["allow_warnings"])) == "true"
}

func validateFHIRPayload(body []byte, config map[string]string) error {
	mode := fhirValidationMode(config)
	outcome, err := fhir.ValidateJSON(body, fhir.ValidationOptions{Mode: mode})
	if err != nil {
		return fmt.Errorf("FHIR payload validation error: %w", err)
	}

	var errors, warnings int
	for _, iss := range outcome.Issue {
		switch iss.Severity {
		case "fatal", "error":
			errors++
		case "warning":
			warnings++
		}
	}

	if errors == 0 && (warnings == 0 || allowFHIRWarnings(config)) {
		return nil
	}

	var details []string
	for _, iss := range outcome.Issue {
		loc := ""
		if len(iss.Location) > 0 {
			loc = " (" + strings.Join(iss.Location, ", ") + ")"
		}
		details = append(details, fmt.Sprintf("%s %s: %s%s", strings.ToUpper(iss.Severity), iss.Code, iss.Diagnostics, loc))
		if len(details) >= 3 {
			break
		}
	}

	if errors > 0 {
		return fmt.Errorf("FHIR payload validation failed (%d error(s), %d warning(s), mode=%s): %s", errors, warnings, mode, strings.Join(details, "; "))
	}
	return fmt.Errorf("FHIR payload validation failed (%d warning(s), mode=%s): %s", warnings, mode, strings.Join(details, "; "))
}

// sendFHIRResource sends a single FHIR resource to the server with rate limiting, retry, and circuit breaker support.
// Handles OAuth 401 by invalidating token cache and retrying with fresh token.
func sendFHIRResource(ctx context.Context, client *http.Client, endpoint string, resource fhir.Resource, config map[string]string) error {
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

	if shouldValidateFHIR(config) {
		if err := validateFHIRPayload(body, config); err != nil {
			return err
		}
	}

	// Build request URL and method
	var reqURL, method string
	switch operation {
	case "create":
		reqURL = fmt.Sprintf("%s/%s", endpoint, resourceType)
		method = "POST"
	case "update":
		// For update, we need the resource ID
		reqURL = fmt.Sprintf("%s/%s", endpoint, resourceType)
		method = "PUT"
	case "upsert":
		// Upsert using conditional create (POST with If-None-Exist)
		reqURL = fmt.Sprintf("%s/%s", endpoint, resourceType)
		method = "POST"
	default:
		return fmt.Errorf("unknown operation: %s", operation)
	}

	// Build headers once (reused for retries)
	headers := http.Header{}
	headers.Set("Content-Type", "application/fhir+json")
	headers.Set("Accept", "application/fhir+json")
	headers.Set("User-Agent", "fi-fhir/1.0")

	// Parse retry configuration
	retryConfig := ParseRetryConfig(config)

	// Parse circuit breaker configuration (nil if disabled)
	cbConfig := ParseCircuitBreakerConfig(config)
	var cb *CircuitBreaker
	if cbConfig != nil {
		cb = GetCircuitBreaker(endpoint)
	}

	// Parse rate limit configuration (nil if disabled)
	rlConfig := ParseRateLimitConfig(config)
	var limiter RateLimiter
	if rlConfig != nil {
		// Get or create limiter for this endpoint with the specified config
		limiter = GetOrCreateRateLimiter(endpoint, *rlConfig)
	}
	waitOnRateLimit := ShouldWaitOnRateLimit(config)

	// Start HTTP span for distributed tracing
	tracer := GetGlobalTracer()
	httpCtx, httpSpan := tracer.StartSpan(ctx, SpanNameHTTP,
		WithSpanKind(SpanKindClient),
		WithAttributes(
			Attr(AttrHTTPMethod, method),
			Attr(AttrHTTPURL, reqURL),
			Attr("fhir.resource_type", resourceType),
			Attr("fhir.operation", operation),
		),
	)
	defer httpSpan.End()

	// Execute request with rate limit, circuit breaker, OAuth 401 handling, and retry
	var resp *http.Response
	httpStart := time.Now()

	err = WithRateLimit(httpCtx, limiter, waitOnRateLimit, func() error {
		var innerErr error
		//nolint:bodyclose // Body is closed via defer after error check below
		resp, innerErr = WithCircuitBreaker(cb, func() (*http.Response, error) {
			return WithOAuthRetry(retryConfig, config,
				func() (*http.Request, error) {
					req, reqErr := http.NewRequestWithContext(httpCtx, method, reqURL, bytes.NewReader(body))
					if reqErr != nil {
						return nil, fmt.Errorf("failed to create FHIR request: %w", reqErr)
					}
					req.Header = headers.Clone()
					return req, nil
				},
				client.Do,
			)
		})
		return innerErr
	})

	httpDuration := time.Since(httpStart)

	if err != nil {
		// Record failed HTTP request metric (status code 0 for network errors)
		GetGlobalMetrics().HTTPRequestCompleted(reqURL, method, 0, httpDuration)
		httpSpan.SetAttribute(AttrHTTPStatus, 0)
		httpSpan.RecordError(err)
		httpSpan.SetStatus(SpanStatusError, err.Error())
		return fmt.Errorf("FHIR request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Record span attributes and metrics
	httpSpan.SetAttribute(AttrHTTPStatus, resp.StatusCode)
	GetGlobalMetrics().HTTPRequestCompleted(reqURL, method, resp.StatusCode, httpDuration)

	// Check response
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		httpSpan.SetStatus(SpanStatusError, fmt.Sprintf("FHIR %d", resp.StatusCode))
		return fmt.Errorf("FHIR server returned %d: %s", resp.StatusCode, string(respBody))
	}

	httpSpan.SetStatus(SpanStatusOK, "")
	return nil
}

// sendFHIRBundle sends multiple resources as a transaction bundle with rate limiting, retry, and circuit breaker support.
// Handles OAuth 401 by invalidating token cache and retrying with fresh token.
func sendFHIRBundle(ctx context.Context, client *http.Client, endpoint string, resources []fhir.Resource, config map[string]string) error {
	bundle := fhir.CreateTransactionBundle(resources)

	// Marshal bundle to JSON
	body, err := json.Marshal(bundle)
	if err != nil {
		return fmt.Errorf("failed to marshal FHIR bundle: %w", err)
	}

	if shouldValidateFHIR(config) {
		if err := validateFHIRPayload(body, config); err != nil {
			return err
		}
	}

	// Build headers once (reused for retries)
	headers := http.Header{}
	headers.Set("Content-Type", "application/fhir+json")
	headers.Set("Accept", "application/fhir+json")
	headers.Set("User-Agent", "fi-fhir/1.0")

	// Parse retry configuration
	retryConfig := ParseRetryConfig(config)

	// Parse circuit breaker configuration (nil if disabled)
	cbConfig := ParseCircuitBreakerConfig(config)
	var cb *CircuitBreaker
	if cbConfig != nil {
		cb = GetCircuitBreaker(endpoint)
	}

	// Parse rate limit configuration (nil if disabled)
	rlConfig := ParseRateLimitConfig(config)
	var limiter RateLimiter
	if rlConfig != nil {
		// Get or create limiter for this endpoint with the specified config
		limiter = GetOrCreateRateLimiter(endpoint, *rlConfig)
	}
	waitOnRateLimit := ShouldWaitOnRateLimit(config)

	// Start HTTP span for distributed tracing
	tracer := GetGlobalTracer()
	httpCtx, httpSpan := tracer.StartSpan(ctx, SpanNameHTTP,
		WithSpanKind(SpanKindClient),
		WithAttributes(
			Attr(AttrHTTPMethod, "POST"),
			Attr(AttrHTTPURL, endpoint),
			Attr("fhir.bundle_size", len(resources)),
		),
	)
	defer httpSpan.End()

	// Execute request with rate limit, circuit breaker, OAuth 401 handling, and retry
	var resp *http.Response
	httpStart := time.Now()

	err = WithRateLimit(httpCtx, limiter, waitOnRateLimit, func() error {
		var innerErr error
		//nolint:bodyclose // Body is closed via defer after error check below
		resp, innerErr = WithCircuitBreaker(cb, func() (*http.Response, error) {
			return WithOAuthRetry(retryConfig, config,
				func() (*http.Request, error) {
					req, reqErr := http.NewRequestWithContext(httpCtx, "POST", endpoint, bytes.NewReader(body))
					if reqErr != nil {
						return nil, fmt.Errorf("failed to create FHIR bundle request: %w", reqErr)
					}
					req.Header = headers.Clone()
					return req, nil
				},
				client.Do,
			)
		})
		return innerErr
	})

	httpDuration := time.Since(httpStart)

	if err != nil {
		// Record failed HTTP request metric (status code 0 for network errors)
		GetGlobalMetrics().HTTPRequestCompleted(endpoint, "POST", 0, httpDuration)
		httpSpan.SetAttribute(AttrHTTPStatus, 0)
		httpSpan.RecordError(err)
		httpSpan.SetStatus(SpanStatusError, err.Error())
		return fmt.Errorf("FHIR bundle request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Record span attributes and metrics
	httpSpan.SetAttribute(AttrHTTPStatus, resp.StatusCode)
	GetGlobalMetrics().HTTPRequestCompleted(endpoint, "POST", resp.StatusCode, httpDuration)

	// Check response
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		httpSpan.SetStatus(SpanStatusError, fmt.Sprintf("FHIR %d", resp.StatusCode))
		return fmt.Errorf("FHIR server returned %d: %s", resp.StatusCode, string(respBody))
	}

	httpSpan.SetStatus(SpanStatusOK, "")
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
