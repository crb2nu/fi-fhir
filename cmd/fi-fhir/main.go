// fi-fhir is a healthcare integration CLI that transforms legacy formats
// (HL7v2, flatfiles, EDI) into semantic events and FHIR resources.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/cblevins/fi-fhir/internal/parser/csv"
	"github.com/cblevins/fi-fhir/internal/parser/hl7v2"
	"github.com/cblevins/fi-fhir/internal/workflow"
	"github.com/cblevins/fi-fhir/pkg/events"
	"github.com/cblevins/fi-fhir/pkg/profile"
)

const version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "parse":
		if err := runParse(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "validate":
		if err := runValidate(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "workflow":
		if err := runWorkflow(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "version", "--version", "-v":
		fmt.Printf("fi-fhir version %s\n", version)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown command: %s\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`fi-fhir - Healthcare Integration CLI

Transform legacy healthcare formats into semantic events and FHIR resources.

Usage:
  fi-fhir <command> [arguments]

Commands:
  parse     Parse a healthcare message and output semantic event JSON
  validate  Validate Source Profile YAML files or messages
  workflow  Run events through workflow routing and actions
  version   Show version information
  help      Show this help message

Examples:
  # Parse an HL7v2 message file
  fi-fhir parse --format hl7v2 message.hl7

  # Parse from stdin
  cat message.hl7 | fi-fhir parse --format hl7v2 -

  # Parse with source identifier
  fi-fhir parse --format hl7v2 --source "lab_interface_a" message.hl7

  # Validate a Source Profile
  fi-fhir validate --profile profiles/lab_interface.yaml

  # Run workflow on events
  fi-fhir workflow run --config workflow.yaml events.json

For more information, visit: https://github.com/cblevins/fi-fhir`)
}

func runParse(args []string) error {
	var (
		format       = "hl7v2"
		source       = "unknown"
		profilePath  = ""
		input        = ""
		pretty       = false
		showWarnings = false
		// CSV-specific options
		csvHeader      = true
		csvDelimiter   = ","
		csvEventType   = ""
		csvInferSchema = false
	)

	// Parse flags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format", "-f":
			if i+1 >= len(args) {
				return fmt.Errorf("--format requires a value")
			}
			i++
			format = args[i]
		case "--source", "-s":
			if i+1 >= len(args) {
				return fmt.Errorf("--source requires a value")
			}
			i++
			source = args[i]
		case "--profile":
			if i+1 >= len(args) {
				return fmt.Errorf("--profile requires a value")
			}
			i++
			profilePath = args[i]
		case "--pretty", "-p":
			pretty = true
		case "--warnings", "-w":
			showWarnings = true
		case "--header":
			csvHeader = true
		case "--no-header":
			csvHeader = false
		case "--delimiter", "-d":
			if i+1 >= len(args) {
				return fmt.Errorf("--delimiter requires a value")
			}
			i++
			csvDelimiter = args[i]
		case "--event-type", "-t":
			if i+1 >= len(args) {
				return fmt.Errorf("--event-type requires a value")
			}
			i++
			csvEventType = args[i]
		case "--infer-schema":
			csvInferSchema = true
		case "--help", "-h":
			printParseUsage()
			return nil
		default:
			if args[i] == "-" {
				input = args[i] // stdin
			} else if len(args[i]) > 0 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag: %s", args[i])
			} else {
				input = args[i]
			}
		}
	}

	// Read input
	var data []byte
	var err error

	if input == "" || input == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(input)
	}
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	if len(data) == 0 {
		return fmt.Errorf("empty input")
	}

	// Load Source Profile if specified
	var sourceProfile *profile.SourceProfile
	if profilePath != "" {
		registry := profile.NewRegistry()
		sourceProfile, err = registry.LoadFromFile(profilePath)
		if err != nil {
			return fmt.Errorf("failed to load profile: %w", err)
		}
	}

	// Parse based on format
	var outputData interface{}
	var warnings []events.ParseWarning

	switch format {
	case "hl7v2", "hl7":
		parser := hl7v2.NewParser(source, hl7v2.ParserConfig{})
		if sourceProfile != nil {
			parser.SetProfile(sourceProfile)
		}
		result, err := parser.ParseWithResult(string(data))
		if err != nil {
			return fmt.Errorf("parse error: %w", err)
		}
		outputData = result.Event
		warnings = result.Warnings

	case "csv", "flatfile":
		// Parse delimiter (handle special cases like \t for tab)
		delimiter := ','
		if csvDelimiter != "" {
			switch csvDelimiter {
			case "\\t", "tab", "TAB":
				delimiter = '\t'
			case "\\s", "space":
				delimiter = ' '
			case "|", "pipe":
				delimiter = '|'
			case ";", "semicolon":
				delimiter = ';'
			default:
				if len(csvDelimiter) == 1 {
					delimiter = rune(csvDelimiter[0])
				} else {
					return fmt.Errorf("invalid delimiter: %s (use single character or \\t for tab)", csvDelimiter)
				}
			}
		}

		// Map event type string to EventType
		var eventType events.EventType
		switch strings.ToLower(csvEventType) {
		case "patient", "patient_admit", "patient_update":
			eventType = events.EventPatientUpdate
		case "lab", "lab_result":
			eventType = events.EventLabResult
		case "":
			// Default - will produce generic records
		default:
			return fmt.Errorf("unknown event type: %s (supported: patient, lab)", csvEventType)
		}

		parser := csv.NewParser(source, csv.ParserConfig{
			HasHeader:   csvHeader,
			Delimiter:   delimiter,
			EventType:   eventType,
			InferSchema: csvInferSchema,
		})
		if sourceProfile != nil {
			parser.SetProfile(sourceProfile)
		}
		result, err := parser.ParseString(string(data))
		if err != nil {
			return fmt.Errorf("parse error: %w", err)
		}

		// For CSV, output all events as an array (or the full result with schema if inferring)
		if csvInferSchema {
			outputData = result
		} else {
			outputData = result.Events
		}
		warnings = result.Warnings

	case "edi", "x12":
		return fmt.Errorf("EDI parser not yet implemented")
	case "fhir":
		return fmt.Errorf("FHIR parser not yet implemented")
	default:
		return fmt.Errorf("unknown format: %s (supported: hl7v2, csv, edi, fhir)", format)
	}

	// Print warnings to stderr if requested
	if showWarnings && len(warnings) > 0 {
		fmt.Fprintf(os.Stderr, "Warnings (%d):\n", len(warnings))
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "  [%s] %s: %s", w.Phase, w.Code, w.Message)
			if w.Path != "" {
				fmt.Fprintf(os.Stderr, " (at %s)", w.Path)
			}
			fmt.Fprintln(os.Stderr)
		}
	}

	// Output JSON
	var output []byte
	if pretty {
		output, err = json.MarshalIndent(outputData, "", "  ")
	} else {
		output, err = json.Marshal(outputData)
	}
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	fmt.Println(string(output))
	return nil
}

func printParseUsage() {
	fmt.Println(`fi-fhir parse - Parse healthcare messages

Usage:
  fi-fhir parse [options] <file>

Options:
  -f, --format <format>   Input format: hl7v2, csv, edi, fhir (default: hl7v2)
  -s, --source <name>     Source system identifier (default: unknown)
      --profile <file>    Source Profile YAML file for custom parsing rules
  -p, --pretty            Pretty-print JSON output
  -w, --warnings          Show parse warnings on stderr
  -h, --help              Show this help message

CSV Options:
      --header            First row contains column headers (default: true)
      --no-header         First row is data, not headers
  -d, --delimiter <char>  Field delimiter (default: comma)
                          Special values: \t or tab, |, ;, space
  -t, --event-type <type> Event type to produce: patient, lab (default: generic)
      --infer-schema      Analyze columns and output inferred schema with events

Arguments:
  <file>                  Input file path, or "-" for stdin

Examples:
  # Parse HL7v2 message
  fi-fhir parse message.hl7
  fi-fhir parse --format hl7v2 --source "lab_system" --pretty message.hl7
  fi-fhir parse --profile profiles/epic_adt.yaml message.hl7

  # Parse CSV patient data
  fi-fhir parse -f csv -t patient --pretty patients.csv

  # Parse tab-separated lab results
  fi-fhir parse -f csv -d tab -t lab lab_results.tsv

  # Analyze CSV schema without specifying event type
  fi-fhir parse -f csv --infer-schema --pretty data.csv

  # Parse from stdin
  cat message.hl7 | fi-fhir parse -f hl7v2 -`)
}

func runValidate(args []string) error {
	var (
		profilePath = ""
		messagePath = ""
		format      = "hl7v2"
		verbose     = false
	)

	// Parse flags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--profile", "-p":
			if i+1 >= len(args) {
				return fmt.Errorf("--profile requires a value")
			}
			i++
			profilePath = args[i]
		case "--message", "-m":
			if i+1 >= len(args) {
				return fmt.Errorf("--message requires a value")
			}
			i++
			messagePath = args[i]
		case "--format", "-f":
			if i+1 >= len(args) {
				return fmt.Errorf("--format requires a value")
			}
			i++
			format = args[i]
		case "--verbose", "-v":
			verbose = true
		case "--help", "-h":
			printValidateUsage()
			return nil
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
			// Positional argument - treat as profile if no profile specified
			if profilePath == "" {
				profilePath = args[i]
			}
		}
	}

	if profilePath == "" {
		return fmt.Errorf("no profile specified. Use --profile <file>")
	}

	// Validate the profile
	validationErrors := validateProfile(profilePath, verbose)
	if len(validationErrors) > 0 {
		fmt.Fprintf(os.Stderr, "Profile validation failed with %d error(s):\n", len(validationErrors))
		for _, err := range validationErrors {
			fmt.Fprintf(os.Stderr, "  - %s\n", err)
		}
		return fmt.Errorf("profile validation failed")
	}

	fmt.Printf("Profile %s is valid.\n", profilePath)

	// If a message is specified, validate it against the profile
	if messagePath != "" {
		messageErrors := validateMessage(profilePath, messagePath, format, verbose)
		if len(messageErrors) > 0 {
			fmt.Fprintf(os.Stderr, "Message validation failed with %d error(s):\n", len(messageErrors))
			for _, err := range messageErrors {
				fmt.Fprintf(os.Stderr, "  - %s\n", err)
			}
			return fmt.Errorf("message validation failed")
		}
		fmt.Printf("Message %s is valid against profile.\n", messagePath)
	}

	return nil
}

func validateProfile(profilePath string, verbose bool) []string {
	var errors []string

	// Check file exists
	if _, err := os.Stat(profilePath); os.IsNotExist(err) {
		return []string{fmt.Sprintf("file not found: %s", profilePath)}
	}

	// Load the profile
	registry := profile.NewRegistry()
	loadedProfile, err := registry.LoadFromFile(profilePath)
	if err != nil {
		return []string{fmt.Sprintf("failed to parse profile: %v", err)}
	}

	// Validate required fields
	if loadedProfile.ID == "" {
		errors = append(errors, "source_profile.id is required")
	}
	if loadedProfile.Name == "" {
		errors = append(errors, "source_profile.name is required")
	}

	// Validate HL7v2 config if present
	if loadedProfile.HL7v2 != nil {
		// Check version format
		if loadedProfile.HL7v2.DefaultVersion != "" {
			validVersions := []string{"2.3", "2.3.1", "2.4", "2.5", "2.5.1", "2.6", "2.7", "2.7.1", "2.8"}
			versionValid := false
			for _, v := range validVersions {
				if loadedProfile.HL7v2.DefaultVersion == v {
					versionValid = true
					break
				}
			}
			if !versionValid {
				errors = append(errors, fmt.Sprintf("invalid HL7v2 version: %s (expected one of: %v)",
					loadedProfile.HL7v2.DefaultVersion, validVersions))
			}
		}

		if verbose {
			fmt.Printf("  HL7v2 config:\n")
			fmt.Printf("    Version: %s\n", loadedProfile.HL7v2.DefaultVersion)
			fmt.Printf("    Timezone: %s\n", loadedProfile.HL7v2.Timezone)
		}
	}

	// Validate Z-segment mappings if present
	if loadedProfile.ZSegments != nil {
		for segID, mappings := range loadedProfile.ZSegments.Mappings {
			if len(segID) < 2 || segID[0] != 'Z' {
				errors = append(errors, fmt.Sprintf("invalid Z-segment ID: %s (must start with 'Z')", segID))
			}
			for i, mapping := range mappings {
				if mapping.Field < 1 {
					errors = append(errors, fmt.Sprintf("%s mapping %d: field must be >= 1", segID, i))
				}
				if mapping.Target == "" {
					errors = append(errors, fmt.Sprintf("%s mapping %d: target is required", segID, i))
				}
				validTypes := []string{"string", "integer", "float", "boolean", "date", "datetime"}
				typeValid := false
				for _, t := range validTypes {
					if mapping.Type == t {
						typeValid = true
						break
					}
				}
				if !typeValid && mapping.Type != "" {
					errors = append(errors, fmt.Sprintf("%s mapping %d: invalid type %q (expected one of: %v)",
						segID, i, mapping.Type, validTypes))
				}
			}
		}

		if verbose && len(loadedProfile.ZSegments.Mappings) > 0 {
			fmt.Printf("  Z-segment mappings:\n")
			for segID, mappings := range loadedProfile.ZSegments.Mappings {
				fmt.Printf("    %s: %d field(s)\n", segID, len(mappings))
			}
		}
	}

	// Validate terminology mappings if present
	if loadedProfile.Terminology != nil {
		for i, mapping := range loadedProfile.Terminology.Mappings {
			if mapping.SourceSystem == "" {
				errors = append(errors, fmt.Sprintf("terminology mapping %d: source_system is required", i))
			}
			if mapping.TargetSystem == "" {
				errors = append(errors, fmt.Sprintf("terminology mapping %d: target_system is required", i))
			}
			if mapping.File == "" {
				errors = append(errors, fmt.Sprintf("terminology mapping %d: file is required", i))
			}
		}

		if verbose && len(loadedProfile.Terminology.Mappings) > 0 {
			fmt.Printf("  Terminology mappings:\n")
			for _, m := range loadedProfile.Terminology.Mappings {
				fmt.Printf("    %s -> %s (file: %s)\n", m.SourceSystem, m.TargetSystem, m.File)
			}
		}
	}

	if verbose {
		fmt.Printf("Profile ID: %s\n", loadedProfile.ID)
		fmt.Printf("Profile Name: %s\n", loadedProfile.Name)
		if loadedProfile.Version != "" {
			fmt.Printf("Profile Version: %s\n", loadedProfile.Version)
		}
	}

	return errors
}

func validateMessage(profilePath, messagePath, format string, verbose bool) []string {
	var errors []string

	// Load profile
	registry := profile.NewRegistry()
	loadedProfile, err := registry.LoadFromFile(profilePath)
	if err != nil {
		return []string{fmt.Sprintf("failed to load profile: %v", err)}
	}

	// Read message
	data, err := os.ReadFile(messagePath)
	if err != nil {
		return []string{fmt.Sprintf("failed to read message: %v", err)}
	}

	// Parse message
	switch format {
	case "hl7v2", "hl7":
		parser := hl7v2.NewParser("validation", hl7v2.ParserConfig{})
		parser.SetProfile(loadedProfile)
		result, err := parser.ParseWithResult(string(data))
		if err != nil {
			errors = append(errors, fmt.Sprintf("parse error: %v", err))
			return errors
		}

		// Report warnings as potential issues
		for _, w := range result.Warnings {
			if w.Severity == "error" {
				errors = append(errors, fmt.Sprintf("[%s] %s: %s (at %s)", w.Phase, w.Code, w.Message, w.Path))
			} else if verbose {
				fmt.Printf("  Warning: [%s] %s: %s (at %s)\n", w.Phase, w.Code, w.Message, w.Path)
			}
		}

		if verbose {
			fmt.Printf("  Message type: %s\n", result.ProfileID)
			fmt.Printf("  Warnings: %d\n", len(result.Warnings))
		}
	default:
		errors = append(errors, fmt.Sprintf("unsupported format: %s", format))
	}

	return errors
}

func printValidateUsage() {
	fmt.Println(`fi-fhir validate - Validate Source Profiles and messages

Usage:
  fi-fhir validate [options] <profile-file>

Options:
  -p, --profile <file>    Source Profile YAML file to validate
  -m, --message <file>    Optional message file to validate against the profile
  -f, --format <format>   Message format: hl7v2 (default: hl7v2)
  -v, --verbose           Show detailed validation information
  -h, --help              Show this help message

Arguments:
  <profile-file>          Source Profile YAML file (if --profile not used)

Validation Checks:
  Profile:
    - YAML syntax validity
    - Required fields (id, name)
    - HL7v2 version format
    - Z-segment mapping validity
    - Terminology mapping completeness

  Message (if --message provided):
    - Parses successfully with the profile
    - Reports any parse warnings as issues

Examples:
  # Validate a profile
  fi-fhir validate profiles/lab_interface.yaml

  # Validate with verbose output
  fi-fhir validate --verbose profiles/epic_adt.yaml

  # Validate a message against a profile
  fi-fhir validate --profile profiles/lab_interface.yaml --message message.hl7`)
}

func runWorkflow(args []string) error {
	if len(args) == 0 {
		printWorkflowUsage()
		return nil
	}

	switch args[0] {
	case "run":
		return runWorkflowRun(args[1:])
	case "validate":
		return runWorkflowValidate(args[1:])
	case "dry-run":
		return runWorkflowDryRun(args[1:])
	case "help", "--help", "-h":
		printWorkflowUsage()
		return nil
	default:
		return fmt.Errorf("unknown workflow subcommand: %s", args[0])
	}
}

func runWorkflowRun(args []string) error {
	var (
		configPath = ""
		input      = ""
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config", "-c":
			if i+1 >= len(args) {
				return fmt.Errorf("--config requires a value")
			}
			i++
			configPath = args[i]
		case "--help", "-h":
			printWorkflowUsage()
			return nil
		default:
			if args[i] == "-" {
				input = args[i]
			} else if len(args[i]) > 0 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag: %s", args[i])
			} else {
				input = args[i]
			}
		}
	}

	if configPath == "" {
		return fmt.Errorf("--config is required")
	}

	// Load workflow
	w, err := workflow.LoadWorkflow(configPath)
	if err != nil {
		return fmt.Errorf("failed to load workflow: %w", err)
	}

	// Validate workflow
	if errors := w.Validate(); len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "Workflow validation errors:\n")
		for _, e := range errors {
			fmt.Fprintf(os.Stderr, "  - %v\n", e)
		}
		return fmt.Errorf("invalid workflow configuration")
	}

	// Read input events (JSON array or newline-delimited JSON)
	var data []byte
	if input == "" || input == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(input)
	}
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	// Parse events
	evts, err := parseEventInput(data)
	if err != nil {
		return fmt.Errorf("failed to parse events: %w", err)
	}

	// Create engine and process
	engine := workflow.NewEngine(w)

	var totalMatched, totalErrors int
	for i, evt := range evts {
		result := engine.Process(evt)
		for _, rr := range result.RouteResults {
			if rr.Matched {
				totalMatched++
			}
			totalErrors += len(rr.ActionErrors)
			for _, e := range rr.ActionErrors {
				fmt.Fprintf(os.Stderr, "Event %d, route %s: %v\n", i, rr.RouteName, e)
			}
		}
	}

	fmt.Printf("Processed %d events, %d route matches, %d errors\n",
		len(evts), totalMatched, totalErrors)

	if totalErrors > 0 {
		return fmt.Errorf("workflow completed with %d errors", totalErrors)
	}
	return nil
}

func runWorkflowValidate(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("workflow file path required")
	}

	configPath := args[0]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			printWorkflowUsage()
			return nil
		default:
			if len(args[i]) > 0 && args[i][0] != '-' {
				configPath = args[i]
			}
		}
	}

	w, err := workflow.LoadWorkflow(configPath)
	if err != nil {
		return fmt.Errorf("failed to load workflow: %w", err)
	}

	errors := w.Validate()
	if len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "Validation errors:\n")
		for _, e := range errors {
			fmt.Fprintf(os.Stderr, "  - %v\n", e)
		}
		return fmt.Errorf("workflow validation failed")
	}

	fmt.Printf("Workflow '%s' is valid.\n", w.Name)
	fmt.Printf("  Version: %s\n", w.Version)
	fmt.Printf("  Routes: %d\n", len(w.Routes))
	for _, r := range w.Routes {
		fmt.Printf("    - %s (%d actions)\n", r.Name, len(r.Actions))
	}

	return nil
}

func runWorkflowDryRun(args []string) error {
	var (
		configPath = ""
		input      = ""
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config", "-c":
			if i+1 >= len(args) {
				return fmt.Errorf("--config requires a value")
			}
			i++
			configPath = args[i]
		case "--help", "-h":
			printWorkflowUsage()
			return nil
		default:
			if args[i] == "-" {
				input = args[i]
			} else if len(args[i]) > 0 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag: %s", args[i])
			} else {
				input = args[i]
			}
		}
	}

	if configPath == "" {
		return fmt.Errorf("--config is required")
	}

	w, err := workflow.LoadWorkflow(configPath)
	if err != nil {
		return fmt.Errorf("failed to load workflow: %w", err)
	}

	var data []byte
	if input == "" || input == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(input)
	}
	if err != nil {
		return fmt.Errorf("failed to read input: %w", err)
	}

	evts, err := parseEventInput(data)
	if err != nil {
		return fmt.Errorf("failed to parse events: %w", err)
	}

	engine := workflow.NewEngine(w)

	fmt.Println("Dry-run results:")
	for i, evt := range evts {
		result := engine.DryRun(evt)
		fmt.Printf("\nEvent %d:\n", i)
		for _, rr := range result.RouteResults {
			status := "NO MATCH"
			if rr.Matched {
				status = fmt.Sprintf("MATCH - would run %d action(s)", rr.ActionsRun)
			}
			fmt.Printf("  Route '%s': %s\n", rr.RouteName, status)
		}
	}

	return nil
}

// parseEventInput parses JSON input as either an array or newline-delimited objects.
func parseEventInput(data []byte) ([]interface{}, error) {
	data = []byte(strings.TrimSpace(string(data)))
	if len(data) == 0 {
		return nil, fmt.Errorf("empty input")
	}

	// Try parsing as array first
	if data[0] == '[' {
		var arr []map[string]interface{}
		if err := json.Unmarshal(data, &arr); err != nil {
			return nil, fmt.Errorf("failed to parse JSON array: %w", err)
		}
		result := make([]interface{}, len(arr))
		for i, m := range arr {
			result[i] = m
		}
		return result, nil
	}

	// Try newline-delimited JSON
	lines := strings.Split(string(data), "\n")
	var result []interface{}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			return nil, fmt.Errorf("failed to parse JSON line: %w", err)
		}
		result = append(result, m)
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no events found in input")
	}

	return result, nil
}

func printWorkflowUsage() {
	fmt.Println(`fi-fhir workflow - Event routing and action execution

Usage:
  fi-fhir workflow <subcommand> [options]

Subcommands:
  run       Process events through workflow routes
  validate  Validate workflow configuration
  dry-run   Simulate workflow without executing actions

Options:
  -c, --config <file>   Workflow YAML configuration file
  -h, --help            Show this help message

Examples:
  # Run workflow on events file
  fi-fhir workflow run --config workflow.yaml events.json

  # Run workflow from piped parse output
  fi-fhir parse -f csv -t patient patients.csv | fi-fhir workflow run -c workflow.yaml -

  # Validate workflow configuration
  fi-fhir workflow validate workflow.yaml

  # Dry-run to see what would match
  fi-fhir workflow dry-run --config workflow.yaml events.json`)
}
