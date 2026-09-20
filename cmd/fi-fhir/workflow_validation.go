package main

import (
	"fmt"
	"os"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
)

func validateWorkflowConfiguration(w *workflow.Workflow) error {
	// Preserve the CLI's structural requirements: the reusable validator treats
	// missing routes and actions as warnings, while Workflow.Validate rejects them.
	if issues := w.Validate(); len(issues) > 0 {
		fmt.Fprintln(os.Stderr, "Validation errors:")
		for _, issue := range issues {
			fmt.Fprintf(os.Stderr, "  - %v\n", issue)
		}
		return fmt.Errorf("workflow validation failed")
	}

	result, err := workflow.ValidateWorkflow(w)
	if err != nil {
		return fmt.Errorf("failed to validate workflow: %w", err)
	}
	if issues := result.AllIssues(); len(issues) > 0 {
		fmt.Fprintln(os.Stderr, "Validation diagnostics:")
		for _, issue := range issues {
			fmt.Fprintf(os.Stderr, "  %s [%s] %s: %s\n", issue.Severity, issue.Code, issue.Path, issue.Message)
		}
	}
	if result.HasErrors() {
		return fmt.Errorf("workflow validation failed")
	}
	return nil
}
