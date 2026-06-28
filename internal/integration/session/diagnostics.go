package session

import (
	"fmt"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

func NormalizeDiagnostics(warnings []events.ParseWarning) []Diagnostic {
	now := time.Now().UTC()
	diagnostics := make([]Diagnostic, 0, len(warnings))
	for i, warning := range warnings {
		severity := warning.Severity
		if severity == "" {
			severity = "warning"
		}
		phase := warning.Phase
		if phase == "" {
			phase = "semantic"
		}
		code := warning.Code
		if code == "" {
			code = "PARSE_WARNING"
		}
		diagnostics = append(diagnostics, Diagnostic{
			ID:        fmt.Sprintf("diag_%03d", i+1),
			Severity:  severity,
			Phase:     phase,
			Code:      code,
			Message:   warning.Message,
			Path:      warning.Path,
			Source:    "hl7v2_parser",
			Original:  warning.Explanation,
			CreatedAt: now,
		})
	}
	return diagnostics
}
