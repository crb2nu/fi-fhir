package workflow

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ValidationError represents a single validation issue.
type ValidationError struct {
	// Path is the location in the config (e.g., "routes[0].filter.condition")
	Path string `json:"path"`
	// Message describes the validation failure
	Message string `json:"message"`
	// Severity indicates how serious the issue is
	Severity ValidationSeverity `json:"severity"`
	// Code is a machine-readable error code
	Code string `json:"code,omitempty"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Path, e.Message)
}

// ValidationSeverity indicates how serious a validation issue is.
type ValidationSeverity string

const (
	// SeverityError indicates a fatal issue that will prevent the workflow from running.
	SeverityError ValidationSeverity = "error"
	// SeverityWarning indicates an issue that should be reviewed but won't prevent execution.
	SeverityWarning ValidationSeverity = "warning"
	// SeverityInfo provides informational feedback.
	SeverityInfo ValidationSeverity = "info"
)

// ValidationResult contains the results of validating a workflow.
type ValidationResult struct {
	Valid    bool              `json:"valid"`
	Errors   []ValidationError `json:"errors,omitempty"`
	Warnings []ValidationError `json:"warnings,omitempty"`
	Info     []ValidationError `json:"info,omitempty"`
}

// HasErrors returns true if there are any error-level issues.
func (r *ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// HasWarnings returns true if there are any warning-level issues.
func (r *ValidationResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}

// AllIssues returns all validation issues regardless of severity.
func (r *ValidationResult) AllIssues() []ValidationError {
	all := make([]ValidationError, 0, len(r.Errors)+len(r.Warnings)+len(r.Info))
	all = append(all, r.Errors...)
	all = append(all, r.Warnings...)
	all = append(all, r.Info...)
	return all
}

// Summary returns a human-readable summary of the validation result.
func (r *ValidationResult) Summary() string {
	if r.Valid && !r.HasWarnings() {
		return "Workflow configuration is valid"
	}

	parts := []string{}
	if len(r.Errors) > 0 {
		parts = append(parts, fmt.Sprintf("%d error(s)", len(r.Errors)))
	}
	if len(r.Warnings) > 0 {
		parts = append(parts, fmt.Sprintf("%d warning(s)", len(r.Warnings)))
	}
	if len(r.Info) > 0 {
		parts = append(parts, fmt.Sprintf("%d info", len(r.Info)))
	}

	status := "valid with issues"
	if !r.Valid {
		status = "invalid"
	}
	return fmt.Sprintf("Workflow configuration %s: %s", status, strings.Join(parts, ", "))
}

// Validator validates workflow configurations.
type Validator struct {
	celEvaluator *CELEvaluator
}

// NewValidator creates a new workflow validator.
func NewValidator() (*Validator, error) {
	cel, err := NewCELEvaluator()
	if err != nil {
		return nil, fmt.Errorf("failed to create CEL evaluator: %w", err)
	}
	return &Validator{celEvaluator: cel}, nil
}

// Validate checks a workflow configuration for errors.
func (v *Validator) Validate(wf *Workflow) *ValidationResult {
	result := &ValidationResult{Valid: true}

	// Validate top-level fields
	v.validateWorkflowMeta(wf, result)

	// Validate routes
	if len(wf.Routes) == 0 {
		result.Warnings = append(result.Warnings, ValidationError{
			Path:     "routes",
			Message:  "No routes defined; workflow will not process any events",
			Severity: SeverityWarning,
			Code:     "NO_ROUTES",
		})
	}

	routeNames := make(map[string]bool)
	for i, route := range wf.Routes {
		path := fmt.Sprintf("routes[%d]", i)
		v.validateRoute(&route, path, routeNames, result)
	}

	// Set overall validity
	result.Valid = len(result.Errors) == 0

	return result
}

func (v *Validator) validateWorkflowMeta(wf *Workflow, result *ValidationResult) {
	if wf.Name == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     "name",
			Message:  "Workflow name is required",
			Severity: SeverityError,
			Code:     "MISSING_NAME",
		})
	} else if !isValidIdentifier(wf.Name) {
		result.Warnings = append(result.Warnings, ValidationError{
			Path:     "name",
			Message:  "Workflow name contains special characters; consider using alphanumeric and underscores only",
			Severity: SeverityWarning,
			Code:     "INVALID_NAME_CHARS",
		})
	}

	if wf.Version == "" {
		result.Warnings = append(result.Warnings, ValidationError{
			Path:     "version",
			Message:  "Version not specified; consider adding a version for tracking",
			Severity: SeverityWarning,
			Code:     "MISSING_VERSION",
		})
	}
}

func (v *Validator) validateRoute(route *Route, path string, routeNames map[string]bool, result *ValidationResult) {
	// Route name
	if route.Name == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".name",
			Message:  "Route name is required",
			Severity: SeverityError,
			Code:     "MISSING_ROUTE_NAME",
		})
	} else {
		if routeNames[route.Name] {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".name",
				Message:  fmt.Sprintf("Duplicate route name: %s", route.Name),
				Severity: SeverityError,
				Code:     "DUPLICATE_ROUTE_NAME",
			})
		}
		routeNames[route.Name] = true
	}

	// Filter
	v.validateFilter(&route.Filter, path+".filter", result)

	// Transforms
	for i, transform := range route.Transforms {
		v.validateTransform(&transform, fmt.Sprintf("%s.transform[%d]", path, i), result)
	}

	// Actions
	if len(route.Actions) == 0 {
		result.Warnings = append(result.Warnings, ValidationError{
			Path:     path + ".actions",
			Message:  "No actions defined for route; events will match but nothing will happen",
			Severity: SeverityWarning,
			Code:     "NO_ACTIONS",
		})
	}

	for i, action := range route.Actions {
		v.validateAction(&action, fmt.Sprintf("%s.actions[%d]", path, i), result)
	}
}

func (v *Validator) validateFilter(filter *Filter, path string, result *ValidationResult) {
	hasAnyFilter := len(filter.EventType) > 0 || len(filter.Source) > 0 || filter.Condition != ""

	if !hasAnyFilter {
		result.Warnings = append(result.Warnings, ValidationError{
			Path:     path,
			Message:  "No filter criteria defined; route will match all events",
			Severity: SeverityWarning,
			Code:     "NO_FILTER",
		})
	}

	// Validate CEL condition if present
	if filter.Condition != "" {
		if err := v.celEvaluator.Compile(filter.Condition); err != nil {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".condition",
				Message:  fmt.Sprintf("Invalid CEL expression: %v", err),
				Severity: SeverityError,
				Code:     "INVALID_CEL",
			})
		}
	}
}

func (v *Validator) validateTransform(transform *Transform, path string, result *ValidationResult) {
	transformCount := 0

	if transform.SetField != "" {
		transformCount++
		// Validate set_field format: "field.path = value"
		if !strings.Contains(transform.SetField, "=") {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".set_field",
				Message:  "set_field must be in format 'field.path = value'",
				Severity: SeverityError,
				Code:     "INVALID_SET_FIELD",
			})
		}
	}

	if transform.MapTerminology != nil {
		transformCount++
		if transform.MapTerminology.Field == "" {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".map_terminology.field",
				Message:  "map_terminology requires field to be specified",
				Severity: SeverityError,
				Code:     "MISSING_TERMINOLOGY_FIELD",
			})
		}
	}

	if transform.Redact != nil {
		transformCount++
		if len(transform.Redact.Fields) == 0 {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".redact.fields",
				Message:  "redact requires at least one field",
				Severity: SeverityError,
				Code:     "MISSING_REDACT_FIELDS",
			})
		}
	}

	if transformCount == 0 {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path,
			Message:  "Transform has no operation defined",
			Severity: SeverityError,
			Code:     "EMPTY_TRANSFORM",
		})
	}

	if transformCount > 1 {
		result.Warnings = append(result.Warnings, ValidationError{
			Path:     path,
			Message:  "Transform has multiple operations; only one will be applied",
			Severity: SeverityWarning,
			Code:     "MULTIPLE_TRANSFORMS",
		})
	}
}

func (v *Validator) validateAction(action *Action, path string, result *ValidationResult) {
	if action.Type == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".type",
			Message:  "Action type is required",
			Severity: SeverityError,
			Code:     "MISSING_ACTION_TYPE",
		})
		return
	}

	// Validate based on action type
	switch action.Type {
	case "log":
		v.validateLogAction(action, path, result)
	case "webhook":
		v.validateWebhookAction(action, path, result)
	case "fhir":
		v.validateFHIRAction(action, path, result)
	case "email":
		v.validateEmailAction(action, path, result)
	case "exec":
		v.validateExecAction(action, path, result)
	case "file":
		v.validateFileAction(action, path, result)
	case "database":
		v.validateDatabaseAction(action, path, result)
	case "queue":
		v.validateQueueAction(action, path, result)
	default:
		result.Warnings = append(result.Warnings, ValidationError{
			Path:     path + ".type",
			Message:  fmt.Sprintf("Unknown action type '%s'; ensure it's registered", action.Type),
			Severity: SeverityWarning,
			Code:     "UNKNOWN_ACTION_TYPE",
		})
	}
}

func (v *Validator) validateExecAction(action *Action, path string, result *ValidationResult) {
	if action.Config["command"] == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".command",
			Message:  "Exec action requires command",
			Severity: SeverityError,
			Code:     "MISSING_EXEC_COMMAND",
		})
	}
	if action.Config["allowlist"] == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".allowlist",
			Message:  "Exec action requires allowlist",
			Severity: SeverityError,
			Code:     "MISSING_EXEC_ALLOWLIST",
		})
	}
	if action.Config["timeout"] != "" {
		if _, err := time.ParseDuration(action.Config["timeout"]); err != nil {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".timeout",
				Message:  fmt.Sprintf("Invalid timeout '%s': %v", action.Config["timeout"], err),
				Severity: SeverityError,
				Code:     "INVALID_EXEC_TIMEOUT",
			})
		}
	}
	if stdin, ok := action.Config["stdin"]; ok && stdin != "" {
		stdin = strings.ToLower(stdin)
		valid := map[string]bool{"json": true, "none": true, "template": true}
		if !valid[stdin] {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".stdin",
				Message:  fmt.Sprintf("Invalid stdin mode '%s'; must be json, none, or template", stdin),
				Severity: SeverityError,
				Code:     "INVALID_EXEC_STDIN",
			})
		}
	}
}

func (v *Validator) validateEmailAction(action *Action, path string, result *ValidationResult) {
	if action.Config["smtp_host"] == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".smtp_host",
			Message:  "Email action requires smtp_host",
			Severity: SeverityError,
			Code:     "MISSING_SMTP_HOST",
		})
	}
	if action.Config["smtp_port"] == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".smtp_port",
			Message:  "Email action requires smtp_port",
			Severity: SeverityError,
			Code:     "MISSING_SMTP_PORT",
		})
	} else if _, err := strconv.Atoi(action.Config["smtp_port"]); err != nil {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".smtp_port",
			Message:  fmt.Sprintf("Invalid smtp_port '%s'; expected integer", action.Config["smtp_port"]),
			Severity: SeverityError,
			Code:     "INVALID_SMTP_PORT",
		})
	}

	if action.Config["from"] == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".from",
			Message:  "Email action requires from",
			Severity: SeverityError,
			Code:     "MISSING_EMAIL_FROM",
		})
	}
	if action.Config["to"] == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".to",
			Message:  "Email action requires to",
			Severity: SeverityError,
			Code:     "MISSING_EMAIL_TO",
		})
	}
	if action.Config["subject"] == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".subject",
			Message:  "Email action requires subject",
			Severity: SeverityError,
			Code:     "MISSING_EMAIL_SUBJECT",
		})
	}

	if v, ok := action.Config["starttls"]; ok && v != "" {
		v = strings.ToLower(v)
		if v != "true" && v != "false" {
			result.Warnings = append(result.Warnings, ValidationError{
				Path:     path + ".starttls",
				Message:  fmt.Sprintf("starttls should be true or false (got '%s')", v),
				Severity: SeverityWarning,
				Code:     "INVALID_STARTTLS",
			})
		}
	}
	if v, ok := action.Config["tls_insecure"]; ok && v != "" {
		v = strings.ToLower(v)
		if v != "true" && v != "false" {
			result.Warnings = append(result.Warnings, ValidationError{
				Path:     path + ".tls_insecure",
				Message:  fmt.Sprintf("tls_insecure should be true or false (got '%s')", v),
				Severity: SeverityWarning,
				Code:     "INVALID_TLS_INSECURE",
			})
		}
	}
	if action.Config["timeout"] != "" {
		if _, err := time.ParseDuration(action.Config["timeout"]); err != nil {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".timeout",
				Message:  fmt.Sprintf("Invalid timeout '%s': %v", action.Config["timeout"], err),
				Severity: SeverityError,
				Code:     "INVALID_EMAIL_TIMEOUT",
			})
		}
	}
}

func (v *Validator) validateFileAction(action *Action, path string, result *ValidationResult) {
	if action.Config["path"] == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".path",
			Message:  "File action requires path",
			Severity: SeverityError,
			Code:     "MISSING_FILE_PATH",
		})
	}

	if format, ok := action.Config["format"]; ok && format != "" {
		format = strings.ToLower(format)
		validFormats := map[string]bool{"json": true, "pretty": true, "ndjson": true}
		if !validFormats[format] {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".format",
				Message:  fmt.Sprintf("Invalid file format '%s'; must be json, pretty, or ndjson", format),
				Severity: SeverityError,
				Code:     "INVALID_FILE_FORMAT",
			})
		}
	}

	if perm, ok := action.Config["perm"]; ok && perm != "" {
		if _, err := strconv.ParseUint(perm, 8, 32); err != nil {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".perm",
				Message:  fmt.Sprintf("Invalid file perm '%s'; expected octal like 0600", perm),
				Severity: SeverityError,
				Code:     "INVALID_FILE_PERM",
			})
		}
	}
}

func (v *Validator) validateLogAction(action *Action, path string, result *ValidationResult) {
	if level, ok := action.Config["level"]; ok {
		validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
		if !validLevels[level] {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".level",
				Message:  fmt.Sprintf("Invalid log level '%s'; must be debug, info, warn, or error", level),
				Severity: SeverityError,
				Code:     "INVALID_LOG_LEVEL",
			})
		}
	}

	// Validate message template if present
	if msg, ok := action.Config["message"]; ok && msg != "" {
		if err := validateGoTemplate(msg); err != nil {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".message",
				Message:  fmt.Sprintf("Invalid message template: %v", err),
				Severity: SeverityError,
				Code:     "INVALID_TEMPLATE",
			})
		}
	}
}

func (v *Validator) validateWebhookAction(action *Action, path string, result *ValidationResult) {
	urlStr, hasURL := action.Config["url"]
	if !hasURL || urlStr == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".url",
			Message:  "Webhook action requires url",
			Severity: SeverityError,
			Code:     "MISSING_WEBHOOK_URL",
		})
	} else {
		// Check if it's a template or a literal URL
		if !strings.Contains(urlStr, "{{") {
			if _, err := url.Parse(urlStr); err != nil {
				result.Errors = append(result.Errors, ValidationError{
					Path:     path + ".url",
					Message:  fmt.Sprintf("Invalid URL: %v", err),
					Severity: SeverityError,
					Code:     "INVALID_URL",
				})
			}
		} else {
			// Validate template
			if err := validateGoTemplate(urlStr); err != nil {
				result.Errors = append(result.Errors, ValidationError{
					Path:     path + ".url",
					Message:  fmt.Sprintf("Invalid URL template: %v", err),
					Severity: SeverityError,
					Code:     "INVALID_URL_TEMPLATE",
				})
			}
		}
	}

	// Validate method if present
	if method, ok := action.Config["method"]; ok {
		validMethods := map[string]bool{
			"GET": true, "POST": true, "PUT": true, "PATCH": true, "DELETE": true,
		}
		if !validMethods[strings.ToUpper(method)] {
			result.Warnings = append(result.Warnings, ValidationError{
				Path:     path + ".method",
				Message:  fmt.Sprintf("Unusual HTTP method '%s'", method),
				Severity: SeverityWarning,
				Code:     "UNUSUAL_HTTP_METHOD",
			})
		}
	}

	// Check for auth configuration
	hasToken := action.Config["token"] != ""
	hasAuth := action.Config["authorization"] != ""
	if !hasToken && !hasAuth {
		result.Info = append(result.Info, ValidationError{
			Path:     path,
			Message:  "No authentication configured for webhook",
			Severity: SeverityInfo,
			Code:     "NO_WEBHOOK_AUTH",
		})
	}
}

func (v *Validator) validateFHIRAction(action *Action, path string, result *ValidationResult) {
	endpoint, hasEndpoint := action.Config["endpoint"]
	if !hasEndpoint || endpoint == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".endpoint",
			Message:  "FHIR action requires endpoint",
			Severity: SeverityError,
			Code:     "MISSING_FHIR_ENDPOINT",
		})
	} else {
		if _, err := url.Parse(endpoint); err != nil {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".endpoint",
				Message:  fmt.Sprintf("Invalid FHIR endpoint URL: %v", err),
				Severity: SeverityError,
				Code:     "INVALID_FHIR_ENDPOINT",
			})
		}
	}

	// Check for auth
	hasToken := action.Config["token"] != ""
	hasOAuth := action.Config["token_url"] != ""
	hasAuth := action.Config["authorization"] != ""

	if !hasToken && !hasOAuth && !hasAuth {
		result.Warnings = append(result.Warnings, ValidationError{
			Path:     path,
			Message:  "No authentication configured for FHIR action",
			Severity: SeverityWarning,
			Code:     "NO_FHIR_AUTH",
		})
	}

	// Validate OAuth config if present
	if hasOAuth {
		if action.Config["client_id"] == "" {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".client_id",
				Message:  "OAuth2 requires client_id when token_url is specified",
				Severity: SeverityError,
				Code:     "MISSING_OAUTH_CLIENT_ID",
			})
		}
		if action.Config["client_secret"] == "" {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".client_secret",
				Message:  "OAuth2 requires client_secret when token_url is specified",
				Severity: SeverityError,
				Code:     "MISSING_OAUTH_CLIENT_SECRET",
			})
		}
	}

	// Validate operation if present
	if op, ok := action.Config["operation"]; ok {
		validOps := map[string]bool{"create": true, "update": true, "upsert": true}
		if !validOps[op] {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".operation",
				Message:  fmt.Sprintf("Invalid FHIR operation '%s'; must be create, update, or upsert", op),
				Severity: SeverityError,
				Code:     "INVALID_FHIR_OPERATION",
			})
		}
	}

	// Validate optional FHIR payload validation config
	if mode, ok := action.Config["validate_mode"]; ok && mode != "" {
		mode = strings.ToLower(mode)
		validModes := map[string]bool{"us-core": true, "none": true}
		if !validModes[mode] {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".validate_mode",
				Message:  fmt.Sprintf("Invalid validate_mode '%s'; must be us-core or none", mode),
				Severity: SeverityError,
				Code:     "INVALID_FHIR_VALIDATE_MODE",
			})
		}
	}

	if v, ok := action.Config["validate_fhir"]; ok && v != "" {
		v = strings.ToLower(v)
		if v != "true" && v != "false" {
			result.Warnings = append(result.Warnings, ValidationError{
				Path:     path + ".validate_fhir",
				Message:  fmt.Sprintf("validate_fhir should be true or false (got '%s')", v),
				Severity: SeverityWarning,
				Code:     "INVALID_FHIR_VALIDATE_FLAG",
			})
		}
	}

	if v, ok := action.Config["allow_warnings"]; ok && v != "" {
		v = strings.ToLower(v)
		if v != "true" && v != "false" {
			result.Warnings = append(result.Warnings, ValidationError{
				Path:     path + ".allow_warnings",
				Message:  fmt.Sprintf("allow_warnings should be true or false (got '%s')", v),
				Severity: SeverityWarning,
				Code:     "INVALID_FHIR_ALLOW_WARNINGS",
			})
		}
	}
}

func (v *Validator) validateDatabaseAction(action *Action, path string, result *ValidationResult) {
	if action.Config["connection"] == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".connection",
			Message:  "Database action requires connection string",
			Severity: SeverityError,
			Code:     "MISSING_DB_CONNECTION",
		})
	}

	if action.Config["table"] == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".table",
			Message:  "Database action requires table name",
			Severity: SeverityError,
			Code:     "MISSING_DB_TABLE",
		})
	}

	// Check for at least one mapping
	hasMappings := false
	for key := range action.Config {
		if strings.HasPrefix(key, "mapping_") {
			hasMappings = true
			break
		}
	}
	if !hasMappings {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path,
			Message:  "Database action requires at least one mapping_* field",
			Severity: SeverityError,
			Code:     "NO_DB_MAPPINGS",
		})
	}

	// Validate operation
	if op, ok := action.Config["operation"]; ok {
		validOps := map[string]bool{"insert": true, "upsert": true}
		if !validOps[op] {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".operation",
				Message:  fmt.Sprintf("Invalid database operation '%s'; must be insert or upsert", op),
				Severity: SeverityError,
				Code:     "INVALID_DB_OPERATION",
			})
		}

		// Upsert requires conflict_on
		if op == "upsert" && action.Config["conflict_on"] == "" {
			result.Errors = append(result.Errors, ValidationError{
				Path:     path + ".conflict_on",
				Message:  "Upsert operation requires conflict_on columns",
				Severity: SeverityError,
				Code:     "MISSING_UPSERT_CONFLICT",
			})
		}
	}
}

func (v *Validator) validateQueueAction(action *Action, path string, result *ValidationResult) {
	if action.Config["driver"] == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".driver",
			Message:  "Queue action requires driver",
			Severity: SeverityError,
			Code:     "MISSING_QUEUE_DRIVER",
		})
	}

	if action.Config["topic"] == "" {
		result.Errors = append(result.Errors, ValidationError{
			Path:     path + ".topic",
			Message:  "Queue action requires topic",
			Severity: SeverityError,
			Code:     "MISSING_QUEUE_TOPIC",
		})
	} else {
		// Validate topic template if it contains template syntax
		topic := action.Config["topic"]
		if strings.Contains(topic, "{{") {
			if err := validateGoTemplate(topic); err != nil {
				result.Errors = append(result.Errors, ValidationError{
					Path:     path + ".topic",
					Message:  fmt.Sprintf("Invalid topic template: %v", err),
					Severity: SeverityError,
					Code:     "INVALID_TOPIC_TEMPLATE",
				})
			}
		}
	}
}

// Helper functions

var identifierRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

func isValidIdentifier(s string) bool {
	return identifierRegex.MatchString(s)
}

func validateGoTemplate(tmpl string) error {
	// Quick validation by checking balanced braces
	openCount := strings.Count(tmpl, "{{")
	closeCount := strings.Count(tmpl, "}}")
	if openCount != closeCount {
		return fmt.Errorf("unbalanced template braces: %d {{ and %d }}", openCount, closeCount)
	}
	return nil
}

// ValidateWorkflow is a convenience function for one-off validation.
func ValidateWorkflow(wf *Workflow) (*ValidationResult, error) {
	v, err := NewValidator()
	if err != nil {
		return nil, err
	}
	return v.Validate(wf), nil
}
