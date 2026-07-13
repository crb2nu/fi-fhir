package integration

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

var sha256DigestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// ContractViolation identifies one invalid field in an integration contract.
type ContractViolation struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// ValidationError reports every contract violation found in one validation pass.
type ValidationError struct {
	Violations []ContractViolation `json:"violations"`
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Violations) == 0 {
		return "integration contract validation failed"
	}

	parts := make([]string, 0, len(e.Violations))
	for _, violation := range e.Violations {
		parts = append(parts, fmt.Sprintf("%s: %s", violation.Path, violation.Message))
	}
	return "integration contract validation failed: " + strings.Join(parts, "; ")
}

type validationCollector struct {
	violations []ContractViolation
}

func (v *validationCollector) add(condition bool, code, path, message string) {
	if condition {
		return
	}
	v.violations = append(v.violations, ContractViolation{
		Code:    code,
		Path:    path,
		Message: message,
	})
}

func (v *validationCollector) merge(path string, err error) {
	if err == nil {
		return
	}
	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		for _, violation := range validationErr.Violations {
			violation.Path = joinPath(path, violation.Path)
			v.violations = append(v.violations, violation)
		}
		return
	}
	v.violations = append(v.violations, ContractViolation{
		Code:    "INVALID",
		Path:    path,
		Message: err.Error(),
	})
}

func (v *validationCollector) err() error {
	if len(v.violations) == 0 {
		return nil
	}
	return &ValidationError{Violations: append([]ContractViolation(nil), v.violations...)}
}

func joinPath(prefix, path string) string {
	if prefix == "" {
		return path
	}
	if path == "" {
		return prefix
	}
	return prefix + "." + path
}

func validateArtifactRevision(path string, ref ArtifactRevisionRef, v *validationCollector) {
	v.add(strings.TrimSpace(ref.ArtifactID) != "", "REQUIRED", joinPath(path, "artifact_id"), "artifact ID is required")
	v.add(strings.TrimSpace(ref.RevisionID) != "", "REQUIRED", joinPath(path, "revision_id"), "revision ID is required")
	v.add(sha256DigestPattern.MatchString(ref.Digest), "INVALID_DIGEST", joinPath(path, "digest"), "digest must be sha256 followed by 64 lowercase hexadecimal characters")
}

func validateSourceFormat(path string, format events.SourceFormat, v *validationCollector) {
	valid := false
	switch format {
	case events.FormatHL7v2,
		events.FormatFHIR,
		events.FormatCSV,
		events.FormatEDI835,
		events.FormatEDI837,
		events.FormatEDI270,
		events.FormatEDI271,
		events.FormatEDI276,
		events.FormatEDI277,
		events.FormatCDA:
		valid = true
	}
	v.add(valid, "INVALID_FORMAT", path, "source format is not supported")
}
