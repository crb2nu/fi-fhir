// fi-fhir is a healthcare integration CLI that transforms legacy formats
// (HL7v2, flatfiles, EDI) into semantic events and FHIR resources.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/fhir/subscription"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/cda"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/csv"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/edi"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/edi/companion"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/edi/companion/builtin"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/hl7v2"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/workflow"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/config"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	fhirpkg "gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/profile"
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
	case "companion":
		if err := runCompanion(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "validate":
		if err := runValidate(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "profile":
		if err := runProfile(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "fhir":
		if err := runFHIR(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "workflow":
		if err := runWorkflow(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "config":
		if err := runConfig(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "subscription":
		if err := runSubscription(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "serve":
		if err := runServe(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "eventstore":
		if err := runEventStore(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "projection":
		if err := runProjection(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "terminology":
		if err := runTerminology(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "storage":
		if err := runStorage(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "etl":
		if err := runETL(os.Args[2:]); err != nil {
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
  parse        Parse a healthcare message and output semantic event JSON
  companion    List/show/validate EDI companion guides
  validate     Validate Source Profile YAML files or messages
  profile      Infer and lint Source Profiles from samples
  fhir         Validate FHIR JSON resources and bundles
  workflow     Run events through workflow routing and actions
  config       Manage application configuration
  subscription Manage FHIR subscriptions for bidirectional integration
  serve        Start the GraphQL API server
  eventstore   Manage event store (init, stats, streams, read)
  projection   Manage projections (list, status, run, rebuild)
  terminology  Manage terminology database (init, load, status, crosswalk)
  storage      Manage object storage (test, ls, get, put, rm)
  etl          ETL pipeline for terminology data (sync, fetch, status)
  version      Show version information
  help         Show this help message

Examples:
  # Parse an HL7v2 message file
  fi-fhir parse --format hl7v2 message.hl7

  # Parse from stdin
  cat message.hl7 | fi-fhir parse --format hl7v2 -

  # Parse with source identifier
  fi-fhir parse --format hl7v2 --source "lab_interface_a" message.hl7

  # Validate a Source Profile
  fi-fhir validate --profile profiles/lab_interface.yaml

  # Infer a Source Profile skeleton from HL7v2 samples
  fi-fhir profile infer --id epic_adt --name "Epic ADT Feed" testdata/adt_a01_sample.hl7

  # Validate a FHIR JSON resource or Bundle
  fi-fhir fhir validate --mode us-core patient.json

  # List built-in EDI companion guides
  fi-fhir companion list

  # Run workflow on events
  fi-fhir workflow run --config workflow.yaml events.json

  # Initialize event store and run projections
  fi-fhir eventstore init --db "$DATABASE_URL"
  fi-fhir projection run --db "$DATABASE_URL"

For more information, visit: https://gitlab.flexinfer.ai/libs/fi-fhir`)
}

func runProfile(args []string) error {
	if len(args) == 0 {
		printProfileUsage()
		return nil
	}

	switch args[0] {
	case "infer":
		return runProfileInfer(args[1:])
	case "lint":
		return runProfileLint(args[1:])
	case "help", "--help", "-h":
		printProfileUsage()
		return nil
	default:
		return fmt.Errorf("unknown profile subcommand: %s (use `fi-fhir profile --help`)", args[0])
	}
}

func runFHIR(args []string) error {
	if len(args) == 0 {
		printFHIRUsage()
		return nil
	}

	switch args[0] {
	case "validate":
		return runFHIRValidate(args[1:])
	case "help", "--help", "-h":
		printFHIRUsage()
		return nil
	default:
		return fmt.Errorf("unknown fhir subcommand: %s (use `fi-fhir fhir --help`)", args[0])
	}
}

func runFHIRValidate(args []string) error {
	var (
		mode    = "us-core"
		jsonOut = false
		strict  = true
		input   = ""
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--mode":
			if i+1 >= len(args) {
				return fmt.Errorf("--mode requires a value")
			}
			i++
			mode = args[i]
		case "--json":
			jsonOut = true
		case "--strict":
			strict = true
		case "--allow-warnings":
			strict = false
		case "--help", "-h":
			printFHIRValidateUsage()
			return nil
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
			if input == "" {
				input = args[i]
			} else {
				return fmt.Errorf("unexpected argument: %s", args[i])
			}
		}
	}

	if input == "" {
		return fmt.Errorf("no input specified (pass a JSON file path or '-' for stdin)")
	}

	var data []byte
	if input == "-" {
		var err error
		data, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
	} else {
		var err error
		data, err = os.ReadFile(input)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", input, err)
		}
	}

	outcome, err := fhirpkg.ValidateJSON(data, fhirpkg.ValidationOptions{Mode: mode})
	if err != nil {
		return err
	}

	hasErrors := false
	hasWarnings := false
	for _, iss := range outcome.Issue {
		switch iss.Severity {
		case "error", "fatal":
			hasErrors = true
		case "warning":
			hasWarnings = true
		}
	}

	if jsonOut {
		b, err := json.MarshalIndent(outcome, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal OperationOutcome: %w", err)
		}
		fmt.Println(string(b))
	} else {
		if hasErrors {
			fmt.Printf("FHIR validation failed (%d issue(s)).\n", len(outcome.Issue))
		} else if hasWarnings {
			fmt.Printf("FHIR validation warnings (%d issue(s)).\n", len(outcome.Issue))
		} else {
			fmt.Printf("FHIR validation passed.\n")
		}
		for _, iss := range outcome.Issue {
			loc := ""
			if len(iss.Location) > 0 {
				loc = " (" + strings.Join(iss.Location, ", ") + ")"
			}
			fmt.Printf("  - %s %s: %s%s\n", strings.ToUpper(iss.Severity), iss.Code, iss.Diagnostics, loc)
		}
	}

	if hasErrors || (strict && hasWarnings) {
		return fmt.Errorf("fhir validation failed")
	}
	return nil
}

func printFHIRUsage() {
	fmt.Println(`fi-fhir fhir - FHIR utilities

Usage:
  fi-fhir fhir <subcommand> [options]

Subcommands:
  validate   Validate a FHIR JSON resource/Bundle

Run:
  fi-fhir fhir <subcommand> --help`)
}

func printFHIRValidateUsage() {
	fmt.Println(`fi-fhir fhir validate - Validate FHIR JSON

Usage:
  fi-fhir fhir validate [options] <file|'-'>

Options:
      --mode <mode>   Validation mode: us-core (default) or none
      --json          Print OperationOutcome JSON to stdout
      --strict        Treat warnings as errors (default)
      --allow-warnings
                     Do not fail validation on warnings
  -h, --help          Show this help message

Examples:
  fi-fhir fhir validate --mode us-core patient.json
  cat bundle.json | fi-fhir fhir validate --json -`)
}

func runProfileInfer(args []string) error {
	var (
		format   = "hl7v2"
		id       = ""
		name     = ""
		ver      = ""
		timezone = ""
		outPath  = ""
		verbose  = false
		maxFiles = 200
		inputs   []string
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--format", "-f":
			if i+1 >= len(args) {
				return fmt.Errorf("--format requires a value")
			}
			i++
			format = args[i]
		case "--id":
			if i+1 >= len(args) {
				return fmt.Errorf("--id requires a value")
			}
			i++
			id = args[i]
		case "--name":
			if i+1 >= len(args) {
				return fmt.Errorf("--name requires a value")
			}
			i++
			name = args[i]
		case "--version":
			if i+1 >= len(args) {
				return fmt.Errorf("--version requires a value")
			}
			i++
			ver = args[i]
		case "--timezone":
			if i+1 >= len(args) {
				return fmt.Errorf("--timezone requires a value")
			}
			i++
			timezone = args[i]
		case "--out", "-o":
			if i+1 >= len(args) {
				return fmt.Errorf("--out requires a value")
			}
			i++
			outPath = args[i]
		case "--verbose", "-v":
			verbose = true
		case "--max-files":
			if i+1 >= len(args) {
				return fmt.Errorf("--max-files requires a value")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				return fmt.Errorf("--max-files must be a positive integer")
			}
			maxFiles = n
		case "--help", "-h":
			printProfileInferUsage()
			return nil
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
			inputs = append(inputs, args[i])
		}
	}

	if len(inputs) == 0 {
		return fmt.Errorf("no samples provided (pass one or more HL7v2 files/directories)")
	}

	switch format {
	case "hl7v2", "hl7":
		var (
			paths     []string
			useStdin  bool
			samples   []string
			inputUsed []string
		)
		for _, in := range inputs {
			if in == "-" {
				useStdin = true
				continue
			}
			paths = append(paths, in)
		}

		if len(paths) > 0 {
			var err error
			samples, inputUsed, err = profile.ReadHL7v2Samples(paths, profile.ReadHL7v2SamplesOptions{
				MaxFiles: maxFiles,
			})
			if err != nil {
				return err
			}
		}

		if useStdin {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("failed to read stdin: %w", err)
			}
			s := strings.TrimSpace(string(data))
			if s == "" {
				return fmt.Errorf("stdin sample is empty")
			}
			samples = append(samples, s)
			inputUsed = append(inputUsed, "-")
		}

		p, report, err := profile.InferHL7v2ProfileFromSamples(samples, inputUsed, profile.InferHL7v2Options{
			ID:       id,
			Name:     name,
			Version:  ver,
			Timezone: timezone,
		})
		if err != nil {
			return err
		}

		yamlBytes, err := profile.MarshalYAML(p)
		if err != nil {
			return err
		}

		if outPath != "" {
			if err := os.WriteFile(outPath, yamlBytes, 0600); err != nil {
				return fmt.Errorf("failed to write %s: %w", outPath, err)
			}
		} else {
			fmt.Print(string(yamlBytes))
		}

		if verbose && report != nil && report.Stats != nil {
			stats := report.Stats
			fmt.Fprintf(os.Stderr, "Inferred from %d sample(s)\n", stats.MessageCount)
			if v, _ := profileMostCommon(stats.Versions); v != "" {
				fmt.Fprintf(os.Stderr, "  HL7v2 version: %s\n", v)
			}
			if mt, _ := profileMostCommon(stats.MessageTypes); mt != "" {
				fmt.Fprintf(os.Stderr, "  Common message type: %s\n", mt)
			}
			if len(stats.ZSegments) > 0 {
				fmt.Fprintf(os.Stderr, "  Z-segments: %d\n", len(stats.ZSegments))
			}
		}

		return nil
	default:
		return fmt.Errorf("unsupported format for infer: %s (expected hl7v2)", format)
	}
}

func runProfileLint(args []string) error {
	var (
		profilePath = ""
		format      = "hl7v2"
		samplesPath = ""
		jsonOut     = false
		strict      = true
		verbose     = false
		maxFiles    = 200
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--profile", "-p":
			if i+1 >= len(args) {
				return fmt.Errorf("--profile requires a value")
			}
			i++
			profilePath = args[i]
		case "--samples":
			if i+1 >= len(args) {
				return fmt.Errorf("--samples requires a value")
			}
			i++
			samplesPath = args[i]
		case "--format", "-f":
			if i+1 >= len(args) {
				return fmt.Errorf("--format requires a value")
			}
			i++
			format = args[i]
		case "--json":
			jsonOut = true
		case "--strict":
			strict = true
		case "--allow-warnings":
			strict = false
		case "--verbose", "-v":
			verbose = true
		case "--max-files":
			if i+1 >= len(args) {
				return fmt.Errorf("--max-files requires a value")
			}
			i++
			n, err := strconv.Atoi(args[i])
			if err != nil || n <= 0 {
				return fmt.Errorf("--max-files must be a positive integer")
			}
			maxFiles = n
		case "--help", "-h":
			printProfileLintUsage()
			return nil
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
			if profilePath == "" {
				profilePath = args[i]
			} else {
				return fmt.Errorf("unexpected argument: %s", args[i])
			}
		}
	}

	if profilePath == "" {
		return fmt.Errorf("no profile specified. Use --profile <file>")
	}

	var (
		samples     []string
		sampleFiles []string
	)
	if samplesPath == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("failed to read stdin: %w", err)
		}
		s := strings.TrimSpace(string(data))
		if s == "" {
			return fmt.Errorf("stdin sample is empty")
		}
		samples = []string{s}
		sampleFiles = []string{"-"}
		samplesPath = ""
	}

	report, err := profile.LintProfileFile(profilePath, profile.LintOptions{
		Format:      format,
		SamplesPath: samplesPath,
		Verbose:     verbose,
		Samples:     samples,
		SampleFiles: sampleFiles,
		MaxFiles:    maxFiles,
	})
	if err != nil {
		return err
	}

	// When sample data is provided, also parse it with the profile to surface any parse warnings/errors.
	if samplesPath != "" || len(samples) > 0 {
		parseErrors, parseWarnings, err := lintHL7v2Samples(profilePath, samplesPath, samples, sampleFiles)
		if err != nil {
			return err
		}
		report.Errors = append(report.Errors, parseErrors...)
		report.Warnings = append(report.Warnings, parseWarnings...)
		sort.Strings(report.Errors)
		sort.Strings(report.Warnings)
	}

	if verbose && report.SampleStats != nil {
		stats := report.SampleStats
		fmt.Fprintf(os.Stderr, "Sample stats (%d message(s)):\n", stats.MessageCount)
		if v, _ := profileMostCommon(stats.Versions); v != "" {
			fmt.Fprintf(os.Stderr, "  MSH-12 version: %s\n", v)
		}
		if mt, _ := profileMostCommon(stats.MessageTypes); mt != "" {
			fmt.Fprintf(os.Stderr, "  MSH-9 message type: %s\n", mt)
		}
		if len(stats.ZSegments) > 0 {
			fmt.Fprintf(os.Stderr, "  Z-segments: %d\n", len(stats.ZSegments))
		}
	}

	if len(report.Errors) > 0 {
		if !jsonOut {
			fmt.Fprintf(os.Stderr, "Lint failed with %d error(s):\n", len(report.Errors))
			for _, e := range report.Errors {
				fmt.Fprintf(os.Stderr, "  - %s\n", e)
			}
		}
		if jsonOut {
			printProfileLintJSON(profilePath, format, report, strict)
		}
		return fmt.Errorf("profile lint failed")
	}

	if len(report.Warnings) > 0 {
		if !jsonOut {
			fmt.Fprintf(os.Stderr, "Lint warnings (%d):\n", len(report.Warnings))
			for _, w := range report.Warnings {
				fmt.Fprintf(os.Stderr, "  - %s\n", w)
			}
		}
		if strict {
			if jsonOut {
				printProfileLintJSON(profilePath, format, report, strict)
			}
			return fmt.Errorf("profile lint failed (warnings treated as errors)")
		}
	}

	if jsonOut {
		printProfileLintJSON(profilePath, format, report, strict)
	} else {
		fmt.Printf("Profile %s passed lint.\n", profilePath)
	}
	return nil
}

type profileLintJSONSampleStats struct {
	MessageCount int            `json:"message_count"`
	Versions     map[string]int `json:"versions,omitempty"`
	CharSets     map[string]int `json:"charsets,omitempty"`
	MessageTypes map[string]int `json:"message_types,omitempty"`
	ZSegments    map[string]int `json:"z_segments,omitempty"`
	HasLF        bool           `json:"has_lf,omitempty"`
	HasCR        bool           `json:"has_cr,omitempty"`
}

type profileLintJSONOutput struct {
	Profile string `json:"profile"`
	Format  string `json:"format,omitempty"`
	OK      bool   `json:"ok"`

	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`

	SampleFiles []string                    `json:"sample_files,omitempty"`
	SampleStats *profileLintJSONSampleStats `json:"sample_stats,omitempty"`
}

func printProfileLintJSON(profilePath, format string, report *profile.LintReport, strict bool) {
	out := profileLintJSONOutput{
		Profile: profilePath,
		Format:  format,
	}
	if report != nil {
		out.Errors = report.Errors
		out.Warnings = report.Warnings
		out.SampleFiles = report.SampleFiles
		if report.SampleStats != nil {
			out.SampleStats = &profileLintJSONSampleStats{
				MessageCount: report.SampleStats.MessageCount,
				Versions:     report.SampleStats.Versions,
				CharSets:     report.SampleStats.CharSets,
				MessageTypes: report.SampleStats.MessageTypes,
				ZSegments:    report.SampleStats.ZSegments,
				HasLF:        report.SampleStats.HasLF,
				HasCR:        report.SampleStats.HasCR,
			}
		}
	}

	out.OK = len(out.Errors) == 0 && (!strict || len(out.Warnings) == 0)

	b, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to marshal JSON: %v\n", err)
		return
	}
	fmt.Println(string(b))
}

func lintHL7v2Samples(profilePath, samplesPath string, stdinSamples, stdinSampleFiles []string) ([]string, []string, error) {
	var (
		samples     []string
		sampleFiles []string
	)

	if samplesPath != "" {
		var err error
		samples, sampleFiles, err = profile.ReadHL7v2Samples([]string{samplesPath}, profile.ReadHL7v2SamplesOptions{})
		if err != nil {
			return nil, nil, err
		}
	} else {
		samples = stdinSamples
		sampleFiles = stdinSampleFiles
	}

	if len(samples) == 0 {
		return nil, nil, nil
	}

	reg := profile.NewRegistry()
	p, err := reg.LoadFromFile(profilePath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to load profile: %w", err)
	}

	parser := hl7v2.NewParser("profile_lint", hl7v2.ParserConfig{})
	parser.SetProfile(p)

	var errs []string
	var warns []string

	for i, raw := range samples {
		src := fmt.Sprintf("sample[%d]", i)
		if i < len(sampleFiles) && sampleFiles[i] != "" {
			src = sampleFiles[i]
		}

		res, parseErr := parser.ParseWithResult(raw)
		if parseErr != nil {
			errs = append(errs, fmt.Sprintf("%s: parse error: %v", src, parseErr))
			continue
		}
		for _, w := range res.Warnings {
			msg := fmt.Sprintf("%s: [%s] %s: %s (at %s)", src, w.Phase, w.Code, w.Message, w.Path)
			if w.Severity == "error" {
				errs = append(errs, msg)
			} else {
				warns = append(warns, msg)
			}
		}
	}

	sort.Strings(errs)
	sort.Strings(warns)
	return errs, warns, nil
}

func printProfileUsage() {
	fmt.Println(`fi-fhir profile - Infer and lint Source Profiles

Usage:
  fi-fhir profile <subcommand> [options]

Subcommands:
  infer   Infer a Source Profile skeleton from sample messages
  lint    Lint a Source Profile (optionally against sample messages)

Run:
  fi-fhir profile <subcommand> --help`)
}

func printProfileInferUsage() {
	fmt.Println(`fi-fhir profile infer - Infer a Source Profile skeleton

Usage:
  fi-fhir profile infer [options] <sample-path>...

Options:
  -f, --format <format>    Input format (default: hl7v2)
      --id <id>            Profile id to set (default: inferred_profile)
      --name <name>        Profile name to set (default: Inferred Profile)
      --version <version>  Profile version to set (default: 0.1.0)
      --timezone <tz>      HL7v2 timezone (default: UTC)
      --max-files <n>      Maximum files to read from directories (default: 200)
  -o, --out <file>         Write YAML to a file instead of stdout
  -v, --verbose            Print inference summary to stderr
  -h, --help               Show this help message

Examples:
  fi-fhir profile infer --id epic_adt --name "Epic ADT Feed" testdata/adt_a01_sample.hl7
  cat testdata/adt_a01_sample.hl7 | fi-fhir profile infer --id epic_adt --name "Epic ADT Feed" -
  fi-fhir profile infer testdata/ --out profiles/inferred.yaml`)
}

func printProfileLintUsage() {
	fmt.Println(`fi-fhir profile lint - Lint a Source Profile

Usage:
  fi-fhir profile lint [options] <profile-file>

Options:
  -p, --profile <file>   Source Profile YAML file to lint
      --samples <path>   Optional sample file/dir to lint against
  -f, --format <format>  Sample format (default: hl7v2)
      --max-files <n>    Maximum files to read from directories (default: 200)
      --json             Print a machine-readable JSON report to stdout
      --strict           Treat warnings as errors (default)
      --allow-warnings   Do not fail lint on warnings
  -v, --verbose          Print sample stats (when --samples is provided)
  -h, --help             Show this help message

Examples:
  fi-fhir profile lint profiles/epic_adt.yaml
  cat testdata/adt_a01_sample.hl7 | fi-fhir profile lint profiles/epic_adt.yaml --samples -
  fi-fhir profile lint profiles/epic_adt.yaml --samples testdata/adt_a01_sample.hl7`)
}

func profileMostCommon(m map[string]int) (string, int) {
	var (
		bestVal string
		bestCnt int
	)
	for v, c := range m {
		if c > bestCnt {
			bestVal = v
			bestCnt = c
		}
	}
	return bestVal, bestCnt
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
		// EDI-specific options
		ediCompanionMode = "" // "", "auto", guide ID, or guide file path
		ediCompanionDir  = "" // optional directory of YAML/JSON guide files
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
		case "--edi-companion":
			if i+1 >= len(args) {
				return fmt.Errorf("--edi-companion requires a value")
			}
			i++
			ediCompanionMode = args[i]
		case "--edi-companion-dir":
			if i+1 >= len(args) {
				return fmt.Errorf("--edi-companion-dir requires a value")
			}
			i++
			ediCompanionDir = args[i]
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

	case "edi", "x12", "837", "835":
		parser := edi.NewParser()
		result, err := parser.Parse(string(data))
		if err != nil {
			return fmt.Errorf("parse error: %w", err)
		}

		// Convert parse warnings to events warnings
		for _, w := range result.Warnings {
			warnings = append(warnings, events.ParseWarning{
				Phase:   w.Phase,
				Code:    w.Code,
				Message: w.Message,
			})
		}

		// Optional companion guide validation (payer-specific rules).
		effectiveCompanionMode := ediCompanionMode
		effectiveCompanionDir := ediCompanionDir
		if sourceProfile != nil && sourceProfile.EDI != nil {
			if effectiveCompanionMode == "" {
				effectiveCompanionMode = sourceProfile.EDI.CompanionGuide
			}
			if effectiveCompanionDir == "" {
				effectiveCompanionDir = sourceProfile.EDI.CompanionGuideDir
			}
		}

		if effectiveCompanionMode != "" {
			registry := companion.NewRegistry()
			if err := builtin.RegisterAll(registry); err != nil {
				return fmt.Errorf("failed to load built-in companion guides: %w", err)
			}
			if effectiveCompanionDir != "" {
				if err := registry.LoadAll(effectiveCompanionDir); err != nil {
					return fmt.Errorf("failed to load companion guides from dir: %w", err)
				}
			}

			switch strings.ToLower(effectiveCompanionMode) {
			case "auto":
				validation := companion.DetectAndValidate(result, registry)
				if validation == nil {
					info := companion.GetParseResultInfo(result)
					warnings = append(warnings, events.ParseWarning{
						Phase:    "edi_companion",
						Code:     "NO_COMPANION_GUIDE",
						Message:  fmt.Sprintf("no companion guide detected (receiver=%s tx=%s)", info.ReceiverID, info.TransactionType),
						Severity: "info",
					})
				} else {
					warnings = append(warnings, companionIssuesToWarnings(validation)...)
				}
			default:
				// Treat as a file path if it exists; otherwise treat as a guide ID.
				var guide *companion.CompanionGuide
				if st, statErr := os.Stat(effectiveCompanionMode); statErr == nil && !st.IsDir() {
					loaded, err := companion.LoadGuide(effectiveCompanionMode)
					if err != nil {
						return fmt.Errorf("failed to load companion guide: %w", err)
					}
					guide = loaded
				} else {
					guide = registry.Get(effectiveCompanionMode)
					if guide == nil {
						return fmt.Errorf("unknown companion guide %q (use --edi-companion auto or provide a guide file path)", effectiveCompanionMode)
					}
				}

				validation := companion.ValidateParseResult(result, guide)
				warnings = append(warnings, companionIssuesToWarnings(validation)...)
			}
		}

		// Map transactions to events
		var allEvents []interface{}
		for _, fg := range result.Interchange.FunctionalGroups {
			for _, tx := range fg.Transactions {
				txType := edi.GetTransactionType(tx)
				switch txType {
				case edi.Transaction837P, edi.Transaction837I, edi.Transaction837D:
					claims, err := edi.Map837ToEvents(tx, source)
					if err != nil {
						return fmt.Errorf("failed to map 837: %w", err)
					}
					for _, c := range claims {
						c.ParseWarnings = append(c.ParseWarnings, warnings...)
						allEvents = append(allEvents, c)
					}
				case edi.Transaction835:
					remittances, err := edi.Map835ToEvents(tx, source)
					if err != nil {
						return fmt.Errorf("failed to map 835: %w", err)
					}
					for _, r := range remittances {
						r.ParseWarnings = append(r.ParseWarnings, warnings...)
						allEvents = append(allEvents, r)
					}
				default:
					// Unknown transaction type - output raw transaction info
					allEvents = append(allEvents, map[string]interface{}{
						"type":           "unknown_transaction",
						"set_identifier": tx.SetIdentifier,
						"control_number": tx.ControlNumber,
						"segment_count":  len(tx.Segments),
					})
				}
			}
		}
		outputData = allEvents
	case "cda", "ccda":
		parser := cda.NewParser(source, &cda.ParserConfig{
			ExtractNarrative: true,
		})
		result, err := parser.ParseWithResult(data)
		if err != nil {
			return fmt.Errorf("parse error: %w", err)
		}

		// Convert parse warnings
		for _, w := range result.Warnings {
			warnings = append(warnings, events.ParseWarning{
				Phase:    "cda",
				Code:     w.Code,
				Message:  w.Message,
				Path:     w.Location,
				Severity: "warning",
			})
		}

		// Map to canonical events
		mapper := cda.NewMapper(&cda.MapperConfig{
			Source:             source,
			EmitDocumentEvents: true,
			EmitSectionEvents:  true,
		})
		mapResult, err := mapper.Map(result.Document)
		if err != nil {
			return fmt.Errorf("mapping error: %w", err)
		}

		// Add mapping warnings
		for _, w := range mapResult.Warnings {
			warnings = append(warnings, events.ParseWarning{
				Phase:    "mapping",
				Message:  w,
				Severity: "warning",
			})
		}

		outputData = map[string]interface{}{
			"document": result.Document,
			"patient":  mapResult.Patient,
			"events":   mapResult.Events,
		}

	case "fhir":
		return fmt.Errorf("FHIR parser not yet implemented")
	default:
		return fmt.Errorf("unknown format: %s (supported: hl7v2, csv, edi, cda, fhir)", format)
	}

	// Optionally validate terminology version pins (adds ParseWarnings or fails, depending on policy).
	dbURL, pins, policy := loadTerminologyPinConfigFromEnv()
	pinWarnings, err := checkTerminologyPins(context.Background(), dbURL, pins, policy)
	if err != nil {
		return err
	}
	if len(pinWarnings) > 0 {
		warnings = append(warnings, pinWarnings...)
		appendParseWarningsToOutputData(outputData, pinWarnings)
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
  -f, --format <format>   Input format: hl7v2, csv, edi, cda, fhir (default: hl7v2)
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

EDI Options:
      --edi-companion <mode>     Companion guide validation: auto | <guide-id> | <path-to-guide.yaml>
      --edi-companion-dir <dir>  Load additional guide files from a directory (YAML/JSON)

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

  # Parse EDI X12 837 claim
  fi-fhir parse -f edi --source "clearinghouse" claim.edi

  # Parse EDI X12 837 claim with companion guide validation (auto-detect)
  fi-fhir parse -f edi --edi-companion auto --warnings claim.edi

  # Parse EDI X12 835 remittance
  fi-fhir parse -f 835 --pretty remittance.edi

  # Parse from stdin
  cat message.hl7 | fi-fhir parse -f hl7v2 -`)
}

func companionIssuesToWarnings(result *companion.ValidationResult) []events.ParseWarning {
	if result == nil {
		return nil
	}

	var out []events.ParseWarning

	// Include the guide used for easier debugging.
	if result.GuideID != "" {
		out = append(out, events.ParseWarning{
			Phase:    "edi_companion",
			Code:     "COMPANION_GUIDE",
			Message:  fmt.Sprintf("validated against companion guide %q", result.GuideID),
			Severity: "info",
		})
	}

	appendIssue := func(issue companion.ValidationIssue) {
		msg := issue.Message
		if issue.Value != "" {
			msg = fmt.Sprintf("%s (value=%q)", msg, issue.Value)
		}
		out = append(out, events.ParseWarning{
			Phase:    "edi_companion",
			Code:     issue.Code,
			Message:  msg,
			Path:     issue.Path,
			Severity: string(issue.Severity),
		})
	}

	for _, issue := range result.Info {
		appendIssue(issue)
	}
	for _, issue := range result.Warnings {
		appendIssue(issue)
	}
	for _, issue := range result.Errors {
		appendIssue(issue)
	}

	return out
}

func printCompanionUsage() {
	fmt.Print(`fi-fhir companion - EDI companion guide utilities

Usage:
  fi-fhir companion list [--dir <dir>] [--json]
  fi-fhir companion show <guide-id> [--dir <dir>] [--format yaml|json]
  fi-fhir companion validate <guide.(yaml|yml|json)>
  fi-fhir companion export <guide-id> <out.(yaml|yml|json)> [--dir <dir>]

Flags:
  --dir <dir>        Load additional guide files from a directory (YAML/JSON)
  --format <format>  Output format for show/export: yaml | json (default: yaml)
  --json             JSON output for list
`)
}

func runCompanion(args []string) error {
	if len(args) == 0 {
		printCompanionUsage()
		return nil
	}

	subcmd := args[0]
	if subcmd == "--help" || subcmd == "-h" || subcmd == "help" {
		printCompanionUsage()
		return nil
	}

	loadRegistry := func(dir string) (*companion.Registry, error) {
		registry := companion.NewRegistry()
		if err := builtin.RegisterAll(registry); err != nil {
			return nil, fmt.Errorf("failed to load built-in companion guides: %w", err)
		}
		if dir != "" {
			if err := registry.LoadAll(dir); err != nil {
				return nil, fmt.Errorf("failed to load companion guides from dir: %w", err)
			}
		}
		return registry, nil
	}

	parseFlags := func(in []string) (positional []string, dir string, format string, jsonOut bool, help bool, err error) {
		format = "yaml"
		for i := 0; i < len(in); i++ {
			switch in[i] {
			case "--dir":
				if i+1 >= len(in) {
					return nil, "", "", false, false, fmt.Errorf("--dir requires a value")
				}
				i++
				dir = in[i]
			case "--format":
				if i+1 >= len(in) {
					return nil, "", "", false, false, fmt.Errorf("--format requires a value")
				}
				i++
				format = strings.ToLower(in[i])
			case "--json":
				jsonOut = true
			case "--help", "-h":
				return nil, "", "", false, true, nil
			default:
				if strings.HasPrefix(in[i], "-") {
					return nil, "", "", false, false, fmt.Errorf("unknown flag: %s", in[i])
				}
				positional = append(positional, in[i])
			}
		}
		if format != "yaml" && format != "json" {
			return nil, "", "", false, false, fmt.Errorf("invalid --format %q (expected yaml or json)", format)
		}
		return positional, dir, format, jsonOut, false, nil
	}

	switch subcmd {
	case "list":
		positional, dir, _, jsonOut, help, err := parseFlags(args[1:])
		if err != nil {
			return err
		}
		if help {
			printCompanionUsage()
			return nil
		}
		if len(positional) != 0 {
			return fmt.Errorf("unexpected args: %v", positional)
		}

		registry, err := loadRegistry(dir)
		if err != nil {
			return err
		}

		guides := registry.All()
		sort.Slice(guides, func(i, j int) bool { return guides[i].ID < guides[j].ID })

		if jsonOut {
			type item struct {
				ID               string   `json:"id"`
				Name             string   `json:"name"`
				PayerID          string   `json:"payer_id,omitempty"`
				ReceiverIDs      []string `json:"receiver_ids,omitempty"`
				BaseGuide        string   `json:"base_guide,omitempty"`
				TransactionTypes []string `json:"transaction_types,omitempty"`
			}
			out := make([]item, 0, len(guides))
			for _, g := range guides {
				out = append(out, item{
					ID:               g.ID,
					Name:             g.Name,
					PayerID:          g.PayerID,
					ReceiverIDs:      g.ReceiverIDs,
					BaseGuide:        g.BaseGuide,
					TransactionTypes: g.TransactionTypes,
				})
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(out)
		}

		for _, g := range guides {
			line := g.ID
			if g.Name != "" {
				line += " - " + g.Name
			}
			fmt.Println(line)
		}
		return nil

	case "show":
		positional, dir, format, _, help, err := parseFlags(args[1:])
		if err != nil {
			return err
		}
		if help {
			printCompanionUsage()
			return nil
		}
		if len(positional) != 1 {
			return fmt.Errorf("usage: fi-fhir companion show <guide-id> [--dir <dir>] [--format yaml|json]")
		}

		registry, err := loadRegistry(dir)
		if err != nil {
			return err
		}

		guide := registry.Get(positional[0])
		if guide == nil {
			return fmt.Errorf("unknown companion guide %q (use `fi-fhir companion list`)", positional[0])
		}

		switch format {
		case "yaml":
			return companion.SaveGuideToYAML(guide, os.Stdout)
		case "json":
			return companion.SaveGuideToJSON(guide, os.Stdout, true)
		default:
			return fmt.Errorf("invalid --format %q", format)
		}

	case "validate":
		positional, _, _, _, help, err := parseFlags(args[1:])
		if err != nil {
			return err
		}
		if help {
			printCompanionUsage()
			return nil
		}
		if len(positional) != 1 {
			return fmt.Errorf("usage: fi-fhir companion validate <guide.(yaml|yml|json)>")
		}
		guide, err := companion.LoadGuide(positional[0])
		if err != nil {
			return err
		}
		fmt.Printf("ok: %s (%s)\n", guide.ID, guide.Name)
		return nil

	case "export":
		positional, dir, format, _, help, err := parseFlags(args[1:])
		if err != nil {
			return err
		}
		if help {
			printCompanionUsage()
			return nil
		}
		if len(positional) != 2 {
			return fmt.Errorf("usage: fi-fhir companion export <guide-id> <out.(yaml|yml|json)> [--dir <dir>]")
		}

		registry, err := loadRegistry(dir)
		if err != nil {
			return err
		}

		guide := registry.Get(positional[0])
		if guide == nil {
			return fmt.Errorf("unknown companion guide %q (use `fi-fhir companion list`)", positional[0])
		}

		var buf bytes.Buffer
		switch format {
		case "yaml":
			if err := companion.SaveGuideToYAML(guide, &buf); err != nil {
				return err
			}
		case "json":
			if err := companion.SaveGuideToJSON(guide, &buf, true); err != nil {
				return err
			}
		}

		if err := os.WriteFile(positional[1], buf.Bytes(), 0600); err != nil {
			return fmt.Errorf("failed to write %s: %w", positional[1], err)
		}
		return nil

	default:
		return fmt.Errorf("unknown companion subcommand: %s (use `fi-fhir companion --help`)", subcmd)
	}
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
	data, err := os.ReadFile(messagePath) //nolint:gosec // G304: CLI tool reads user-specified file
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
	case "record":
		return runWorkflowRecord(args[1:])
	case "replay":
		return runWorkflowReplay(args[1:])
	case "simulate":
		return runWorkflowSimulate(args[1:])
	case "loadtest":
		return runWorkflowLoadtest(args[1:])
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
	engine, err := workflow.NewEngine(w)
	if err != nil {
		return fmt.Errorf("failed to create workflow engine: %w", err)
	}

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

	engine, err := workflow.NewEngine(w)
	if err != nil {
		return fmt.Errorf("failed to create workflow engine: %w", err)
	}

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

func runWorkflowRecord(args []string) error {
	var (
		configPath = ""
		outputPath = ""
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
		case "--output", "-o":
			if i+1 >= len(args) {
				return fmt.Errorf("--output requires a value")
			}
			i++
			outputPath = args[i]
		case "--help", "-h":
			printWorkflowRecordUsage()
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
	if outputPath == "" {
		return fmt.Errorf("--output is required")
	}

	// Load workflow
	w, err := workflow.LoadWorkflow(configPath)
	if err != nil {
		return fmt.Errorf("failed to load workflow: %w", err)
	}

	// Read input events
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

	// Create recording engine
	recorder := workflow.NewMemoryRecorder()
	engine, err := workflow.NewRecordingEngine(w, recorder)
	if err != nil {
		return fmt.Errorf("failed to create recording engine: %w", err)
	}

	// Process events
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

	// Export recordings
	if err := recorder.Export(outputPath); err != nil {
		return fmt.Errorf("failed to export recordings: %w", err)
	}

	fmt.Printf("Processed %d events, %d route matches, %d errors\n",
		len(evts), totalMatched, totalErrors)
	fmt.Printf("Recorded %d events to %s\n", recorder.Len(), outputPath)

	return nil
}

func runWorkflowReplay(args []string) error {
	var (
		configPath     = ""
		recordingsPath = ""
		eventTypes     []string
		sources        []string
		limit          = 0
		showDiffs      = false
		outputPath     = ""
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config", "-c":
			if i+1 >= len(args) {
				return fmt.Errorf("--config requires a value")
			}
			i++
			configPath = args[i]
		case "--recordings", "-r":
			if i+1 >= len(args) {
				return fmt.Errorf("--recordings requires a value")
			}
			i++
			recordingsPath = args[i]
		case "--event-type", "-t":
			if i+1 >= len(args) {
				return fmt.Errorf("--event-type requires a value")
			}
			i++
			eventTypes = append(eventTypes, args[i])
		case "--source", "-s":
			if i+1 >= len(args) {
				return fmt.Errorf("--source requires a value")
			}
			i++
			sources = append(sources, args[i])
		case "--limit", "-l":
			if i+1 >= len(args) {
				return fmt.Errorf("--limit requires a value")
			}
			i++
			if _, err := fmt.Sscanf(args[i], "%d", &limit); err != nil {
				return fmt.Errorf("invalid limit: %s", args[i])
			}
		case "--diffs", "-d":
			showDiffs = true
		case "--output", "-o":
			if i+1 >= len(args) {
				return fmt.Errorf("--output requires a value")
			}
			i++
			outputPath = args[i]
		case "--help", "-h":
			printWorkflowReplayUsage()
			return nil
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
			if recordingsPath == "" {
				recordingsPath = args[i]
			}
		}
	}

	if configPath == "" {
		return fmt.Errorf("--config is required")
	}
	if recordingsPath == "" {
		return fmt.Errorf("recordings file path is required")
	}

	// Load workflow
	w, err := workflow.LoadWorkflow(configPath)
	if err != nil {
		return fmt.Errorf("failed to load workflow: %w", err)
	}

	// Create engine
	engine, err := workflow.NewEngine(w)
	if err != nil {
		return fmt.Errorf("failed to create engine: %w", err)
	}

	// Load recordings
	recorder := workflow.NewMemoryRecorder()
	if err := recorder.Import(recordingsPath); err != nil {
		return fmt.Errorf("failed to load recordings: %w", err)
	}

	// Create replayer
	replayer := workflow.NewEventReplayer(engine, recorder)

	// Run replay
	config := &workflow.ReplayConfig{
		EventTypes: eventTypes,
		Sources:    sources,
		Limit:      limit,
	}

	summary, err := replayer.Replay(context.Background(), config)
	if err != nil {
		return fmt.Errorf("replay failed: %w", err)
	}

	// Output summary
	fmt.Printf("Replay Summary:\n")
	fmt.Printf("  Total events: %d\n", summary.TotalEvents)
	fmt.Printf("  Routing matches: %d\n", summary.MatchedRouting)
	fmt.Printf("  Routing differences: %d\n", summary.DifferentRouting)
	fmt.Printf("  Error state matches: %d\n", summary.MatchedErrors)
	fmt.Printf("  Error state differences: %d\n", summary.DifferentErrors)
	fmt.Printf("  Duration: %v\n", summary.TotalDuration)

	// Show diffs if requested
	if showDiffs && len(summary.Diffs) > 0 {
		fmt.Printf("\nDifferences:\n")
		for _, diff := range summary.Diffs {
			fmt.Printf("  Event %s:\n", diff.EventID)
			if !diff.RoutingMatch {
				if len(diff.AddedRoutes) > 0 {
					fmt.Printf("    Added routes: %v\n", diff.AddedRoutes)
				}
				if len(diff.RemovedRoutes) > 0 {
					fmt.Printf("    Removed routes: %v\n", diff.RemovedRoutes)
				}
			}
			if !diff.ErrorMatch {
				if len(diff.OriginalErrors) > 0 {
					fmt.Printf("    Original errors: %v\n", diff.OriginalErrors)
				}
				if len(diff.ReplayErrors) > 0 {
					fmt.Printf("    Replay errors: %v\n", diff.ReplayErrors)
				}
			}
		}
	}

	// Write output file if specified
	if outputPath != "" {
		outputData, err := json.MarshalIndent(summary, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal summary: %w", err)
		}
		if err := os.WriteFile(outputPath, outputData, 0644); err != nil { //nolint:gosec // G306: non-sensitive output file
			return fmt.Errorf("failed to write output: %w", err)
		}
		fmt.Printf("\nSummary written to %s\n", outputPath)
	}

	// Exit with error if there are differences
	if summary.DifferentRouting > 0 || summary.DifferentErrors > 0 {
		return fmt.Errorf("replay detected %d routing and %d error differences",
			summary.DifferentRouting, summary.DifferentErrors)
	}

	return nil
}

func runWorkflowSimulate(args []string) error {
	var (
		configPath = ""
		input      = ""
		outputPath = ""
		verbose    = false
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config", "-c":
			if i+1 >= len(args) {
				return fmt.Errorf("--config requires a value")
			}
			i++
			configPath = args[i]
		case "--output", "-o":
			if i+1 >= len(args) {
				return fmt.Errorf("--output requires a value")
			}
			i++
			outputPath = args[i]
		case "--verbose", "-v":
			verbose = true
		case "--help", "-h":
			printWorkflowSimulateUsage()
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

	// Read input events
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

	// Create simulation engine
	sim, err := workflow.NewSimulationEngine(w)
	if err != nil {
		return fmt.Errorf("failed to create simulation engine: %w", err)
	}

	// Process events
	for _, evt := range evts {
		sim.Process(evt)
	}

	// Generate report
	report := sim.Report()

	// Output summary
	fmt.Printf("Simulation Report:\n")
	fmt.Printf("  Events processed: %d\n", len(evts))
	fmt.Printf("  Total actions: %d\n", report.TotalActions)
	fmt.Printf("  Actions by type:\n")
	for actionType, count := range report.ActionsByType {
		fmt.Printf("    %s: %d\n", actionType, count)
	}

	if len(report.Errors) > 0 {
		fmt.Printf("  Errors: %d\n", len(report.Errors))
		for _, e := range report.Errors {
			fmt.Fprintf(os.Stderr, "    - %s\n", e)
		}
	}

	// Show detailed invocations if verbose
	if verbose {
		fmt.Printf("\nAction Invocations:\n")
		for i, inv := range report.Invocations {
			fmt.Printf("  %d. %s\n", i+1, inv.ActionType)
			if inv.Error != "" {
				fmt.Printf("     Error: %s\n", inv.Error)
			}
		}
	}

	// Write output file if specified
	if outputPath != "" {
		outputData, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal report: %w", err)
		}
		if err := os.WriteFile(outputPath, outputData, 0644); err != nil { //nolint:gosec // G306: non-sensitive output file
			return fmt.Errorf("failed to write output: %w", err)
		}
		fmt.Printf("\nReport written to %s\n", outputPath)
	}

	return nil
}

func printWorkflowRecordUsage() {
	fmt.Println(`fi-fhir workflow record - Process events and record for replay

Usage:
  fi-fhir workflow record --config <workflow.yaml> --output <recordings.json> [events.json]

Options:
  -c, --config <file>   Workflow YAML configuration file (required)
  -o, --output <file>   Output file for recorded events (required)
  -h, --help            Show this help message

Arguments:
  [events.json]         Input events file, or "-" for stdin

Description:
  Processes events through the workflow and records each event along with
  its processing result. The recordings can later be replayed to test
  workflow changes.

Examples:
  # Record events from a file
  fi-fhir workflow record -c workflow.yaml -o recordings.json events.json

  # Record from piped input
  fi-fhir parse -f hl7v2 message.hl7 | fi-fhir workflow record -c workflow.yaml -o recordings.json -`)
}

func printWorkflowReplayUsage() {
	fmt.Println(`fi-fhir workflow replay - Replay recorded events and compare results

Usage:
  fi-fhir workflow replay --config <workflow.yaml> <recordings.json>

Options:
  -c, --config <file>     Workflow YAML configuration file (required)
  -r, --recordings <file> Recordings file from 'workflow record' (or positional arg)
  -t, --event-type <type> Filter by event type (can be repeated)
  -s, --source <source>   Filter by source (can be repeated)
  -l, --limit <n>         Maximum number of events to replay
  -d, --diffs             Show detailed differences
  -o, --output <file>     Write summary JSON to file
  -h, --help              Show this help message

Description:
  Replays recorded events through a workflow and compares the results with
  the original processing. This is useful for:
  - Testing workflow changes don't break existing behavior
  - Debugging production issues by replaying problematic events
  - Regression testing after workflow modifications

Exit Codes:
  0  All events replayed with same results
  1  Differences detected between original and replay

Examples:
  # Replay all events and show differences
  fi-fhir workflow replay -c workflow.yaml -d recordings.json

  # Replay only patient_admit events
  fi-fhir workflow replay -c workflow.yaml -t patient_admit recordings.json

  # Replay with JSON output for CI
  fi-fhir workflow replay -c workflow.yaml -o results.json recordings.json`)
}

func printWorkflowSimulateUsage() {
	fmt.Println(`fi-fhir workflow simulate - Process events with mock actions

Usage:
  fi-fhir workflow simulate --config <workflow.yaml> [events.json]

Options:
  -c, --config <file>   Workflow YAML configuration file (required)
  -o, --output <file>   Write simulation report JSON to file
  -v, --verbose         Show detailed action invocations
  -h, --help            Show this help message

Arguments:
  [events.json]         Input events file, or "-" for stdin

Description:
  Processes events through the workflow with all actions replaced by mocks.
  No real HTTP calls, database writes, or queue publishes occur. This is
  useful for:
  - Testing event routing without side effects
  - Verifying which actions would be triggered
  - Understanding workflow behavior in isolation

Examples:
  # Simulate processing and show report
  fi-fhir workflow simulate -c workflow.yaml events.json

  # Verbose output with all invocations
  fi-fhir workflow simulate -c workflow.yaml -v events.json

  # Save report to file
  fi-fhir workflow simulate -c workflow.yaml -o report.json events.json`)
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
  record    Process events and record for replay
  replay    Replay recorded events and compare results
  simulate  Process events with mock actions (no side effects)
  loadtest  Run load tests against workflow configuration

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
  fi-fhir workflow dry-run --config workflow.yaml events.json

  # Record events for regression testing
  fi-fhir workflow record -c workflow.yaml -o recordings.json events.json

  # Replay recordings after workflow changes
  fi-fhir workflow replay -c workflow_v2.yaml -d recordings.json

  # Simulate without side effects
  fi-fhir workflow simulate -c workflow.yaml -v events.json

  # Run load test with standard scenario
  fi-fhir workflow loadtest -c workflow.yaml --scenario smoke`)
}

func runWorkflowLoadtest(args []string) error {
	var (
		configPath   = ""
		scenarioName = ""
		duration     = 30 * time.Second
		rps          = 1000
		workers      = 4
		warmup       = 5 * time.Second
		verbose      = false
		jsonOutput   = false
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-c", "--config":
			if i+1 >= len(args) {
				return fmt.Errorf("--config requires a file path")
			}
			i++
			configPath = args[i]
		case "-s", "--scenario":
			if i+1 >= len(args) {
				return fmt.Errorf("--scenario requires a name")
			}
			i++
			scenarioName = args[i]
		case "-d", "--duration":
			if i+1 >= len(args) {
				return fmt.Errorf("--duration requires a value")
			}
			i++
			d, err := time.ParseDuration(args[i])
			if err != nil {
				return fmt.Errorf("invalid duration: %w", err)
			}
			duration = d
		case "-r", "--rps":
			if i+1 >= len(args) {
				return fmt.Errorf("--rps requires a value")
			}
			i++
			r, err := strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("invalid rps: %w", err)
			}
			rps = r
		case "-w", "--workers":
			if i+1 >= len(args) {
				return fmt.Errorf("--workers requires a value")
			}
			i++
			w, err := strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("invalid workers: %w", err)
			}
			workers = w
		case "--warmup":
			if i+1 >= len(args) {
				return fmt.Errorf("--warmup requires a value")
			}
			i++
			w, err := time.ParseDuration(args[i])
			if err != nil {
				return fmt.Errorf("invalid warmup: %w", err)
			}
			warmup = w
		case "-v", "--verbose":
			verbose = true
		case "--json":
			jsonOutput = true
		case "-h", "--help":
			printWorkflowLoadtestUsage()
			return nil
		case "--list-scenarios":
			fmt.Println("Available load test scenarios:")
			for _, s := range workflow.StandardScenarios() {
				fmt.Printf("  %-12s %s\n", s.Name, s.Description)
			}
			return nil
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
		}
	}

	if configPath == "" {
		return fmt.Errorf("--config is required")
	}

	// Load workflow
	workflowData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read workflow: %w", err)
	}

	wf, err := workflow.ParseWorkflow(workflowData)
	if err != nil {
		return fmt.Errorf("failed to parse workflow: %w", err)
	}

	engine, err := workflow.NewEngine(wf)
	if err != nil {
		return fmt.Errorf("failed to create engine: %w", err)
	}

	// Build config
	var config *workflow.LoadTestConfig
	if scenarioName != "" {
		scenario := workflow.GetScenario(scenarioName)
		if scenario == nil {
			return fmt.Errorf("unknown scenario: %s (use --list-scenarios to see available)", scenarioName)
		}
		config = scenario.Config
		if verbose {
			fmt.Printf("Using scenario: %s - %s\n", scenario.Name, scenario.Description)
		}
	} else {
		config = &workflow.LoadTestConfig{
			Duration:         duration,
			TargetRPS:        rps,
			Workers:          workers,
			WarmupDuration:   warmup,
			EventGenerator:   workflow.NewHealthcareEventGenerator(),
			ProgressInterval: 5 * time.Second,
		}
	}

	// Progress callback
	if verbose && !jsonOutput {
		config.OnProgress = func(stats workflow.LoadTestProgress) {
			fmt.Printf("[%v] Events: %d, Rate: %.0f/s, P50: %v, P99: %v, Errors: %d\n",
				stats.Elapsed.Round(time.Second),
				stats.EventsTotal,
				stats.EventsPerSec,
				stats.P50Latency.Round(time.Microsecond),
				stats.P99Latency.Round(time.Microsecond),
				stats.ErrorCount)
		}
	}

	if verbose && !jsonOutput {
		fmt.Printf("Starting load test:\n")
		fmt.Printf("  Duration:   %v\n", config.Duration)
		fmt.Printf("  Target RPS: %d\n", config.TargetRPS)
		fmt.Printf("  Workers:    %d\n", config.Workers)
		fmt.Printf("  Warmup:     %v\n", config.WarmupDuration)
		fmt.Println()
	}

	// Run load test
	tester := workflow.NewLoadTester(engine)
	result, err := tester.Run(context.Background(), config)
	if err != nil {
		return fmt.Errorf("load test failed: %w", err)
	}

	// Output results
	if jsonOutput {
		output, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(output))
	} else {
		fmt.Println(result.Summary())
	}

	// Return error if load test failed thresholds
	if !result.Passed(0.90, 0.01, 10*time.Millisecond) {
		return fmt.Errorf("load test did not meet performance targets")
	}

	return nil
}

func printWorkflowLoadtestUsage() {
	fmt.Println(`fi-fhir workflow loadtest - Run load tests against workflow

Usage:
  fi-fhir workflow loadtest [options]

Options:
  -c, --config <file>      Workflow YAML configuration file (required)
  -s, --scenario <name>    Use a predefined scenario (smoke, standard, stress, burst, soak)
  -d, --duration <dur>     Test duration (default: 30s)
  -r, --rps <num>          Target requests per second (default: 1000, 0 for unlimited)
  -w, --workers <num>      Number of concurrent workers (default: 4)
  --warmup <dur>           Warmup duration (default: 5s)
  -v, --verbose            Show progress during test
  --json                   Output results as JSON
  --list-scenarios         List available predefined scenarios
  -h, --help               Show this help message

Predefined Scenarios:
  smoke     Quick smoke test (10s, 100 RPS)
  standard  Standard load test (60s, 1000 RPS)
  stress    Stress test (120s, 5000 RPS)
  burst     Burst test (30s, max throughput)
  soak      Soak test (5min, 500 RPS)

Examples:
  # Run a quick smoke test
  fi-fhir workflow loadtest -c workflow.yaml -s smoke -v

  # Custom load test
  fi-fhir workflow loadtest -c workflow.yaml -d 60s -r 2000 -w 8 -v

  # Stress test with JSON output
  fi-fhir workflow loadtest -c workflow.yaml -s stress --json > results.json

  # Burst test (maximum throughput)
  fi-fhir workflow loadtest -c workflow.yaml -r 0 -d 10s -v`)
}

// runConfig handles the config command and its subcommands.
func runConfig(args []string) error {
	if len(args) == 0 {
		printConfigUsage()
		return nil
	}

	switch args[0] {
	case "show":
		return runConfigShow(args[1:])
	case "validate":
		return runConfigValidate(args[1:])
	case "env":
		return runConfigEnv(args[1:])
	case "init":
		return runConfigInit(args[1:])
	case "help", "--help", "-h":
		printConfigUsage()
		return nil
	default:
		return fmt.Errorf("unknown config subcommand: %s", args[0])
	}
}

func runConfigShow(args []string) error {
	var (
		configPath = ""
		format     = "yaml"
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-c", "--config":
			if i+1 >= len(args) {
				return fmt.Errorf("--config requires a file path")
			}
			i++
			configPath = args[i]
		case "-f", "--format":
			if i+1 >= len(args) {
				return fmt.Errorf("--format requires a value")
			}
			i++
			format = args[i]
		case "-h", "--help":
			printConfigShowUsage()
			return nil
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
			if configPath == "" {
				configPath = args[i]
			}
		}
	}

	// Load config (or defaults if no path)
	var cfg *config.Config
	var err error
	if configPath != "" {
		cfg, err = config.LoadWithSecrets(configPath)
		if err != nil {
			return fmt.Errorf("failed to load config: %w", err)
		}
	} else {
		cfg = config.Default()
		cfg.ApplyEnv()
	}

	// Output in requested format
	switch format {
	case "yaml":
		output, err := marshalYAML(cfg)
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}
		fmt.Println(string(output))
	case "json":
		output, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal config: %w", err)
		}
		fmt.Println(string(output))
	default:
		return fmt.Errorf("unknown format: %s (use yaml or json)", format)
	}

	return nil
}

func runConfigValidate(args []string) error {
	var configPath string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-h", "--help":
			printConfigValidateUsage()
			return nil
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
			configPath = args[i]
		}
	}

	if configPath == "" {
		return fmt.Errorf("config file path required")
	}

	cfg, err := config.LoadFromFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	errors := cfg.Validate()
	if len(errors) > 0 {
		fmt.Fprintf(os.Stderr, "Configuration validation failed:\n")
		for _, e := range errors {
			fmt.Fprintf(os.Stderr, "  - %v\n", e)
		}
		return fmt.Errorf("validation failed with %d error(s)", len(errors))
	}

	fmt.Printf("Configuration %s is valid.\n", configPath)
	return nil
}

func runConfigEnv(args []string) error {
	var (
		format  = "list"
		section = ""
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-f", "--format":
			if i+1 >= len(args) {
				return fmt.Errorf("--format requires a value")
			}
			i++
			format = args[i]
		case "-s", "--section":
			if i+1 >= len(args) {
				return fmt.Errorf("--section requires a value")
			}
			i++
			section = args[i]
		case "-h", "--help":
			printConfigEnvUsage()
			return nil
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
		}
	}

	// Define all environment variables
	envVars := []struct {
		section     string
		name        string
		description string
		defaultVal  string
	}{
		// Server
		{"server", "FI_FHIR_SERVER_HOST", "HTTP server bind address", "0.0.0.0"},
		{"server", "FI_FHIR_SERVER_PORT", "HTTP server port", "8080"},
		{"server", "FI_FHIR_SERVER_READ_TIMEOUT", "HTTP read timeout", "30s"},
		{"server", "FI_FHIR_SERVER_WRITE_TIMEOUT", "HTTP write timeout", "30s"},
		{"server", "FI_FHIR_SERVER_SHUTDOWN_TIMEOUT", "Graceful shutdown timeout", "10s"},
		{"server", "FI_FHIR_SERVER_TLS_CERT_FILE", "TLS certificate file path", ""},
		{"server", "FI_FHIR_SERVER_TLS_KEY_FILE", "TLS key file path", ""},

		// Workflow
		{"workflow", "FI_FHIR_WORKFLOW_CONFIG_PATH", "Workflow configuration file path", ""},
		{"workflow", "FI_FHIR_WORKFLOW_DRY_RUN", "Enable dry-run mode (no actions executed)", "false"},
		{"workflow", "FI_FHIR_WORKFLOW_MAX_CONCURRENCY", "Maximum concurrent action executions", "10"},
		{"workflow", "FI_FHIR_WORKFLOW_ACTION_TIMEOUT", "Timeout for individual actions", "30s"},
		{"workflow", "FI_FHIR_WORKFLOW_RETRY_MAX_ATTEMPTS", "Maximum retry attempts for failed actions", "3"},
		{"workflow", "FI_FHIR_WORKFLOW_RETRY_INITIAL_WAIT", "Initial wait between retries", "1s"},
		{"workflow", "FI_FHIR_WORKFLOW_RETRY_MAX_WAIT", "Maximum wait between retries", "30s"},
		{"workflow", "FI_FHIR_WORKFLOW_DLQ_ENABLED", "Enable dead letter queue for failed events", "true"},
		{"workflow", "FI_FHIR_WORKFLOW_DLQ_MAX_SIZE", "Maximum DLQ size", "10000"},

		// FHIR
		{"fhir", "FI_FHIR_FHIR_BASE_URL", "FHIR server base URL", ""},
		{"fhir", "FI_FHIR_FHIR_TIMEOUT", "FHIR request timeout", "30s"},
		{"fhir", "FI_FHIR_FHIR_AUTH_TYPE", "Authentication type (none, basic, bearer, oauth2)", "none"},
		{"fhir", "FI_FHIR_FHIR_USERNAME", "Basic auth username", ""},
		{"fhir", "FI_FHIR_FHIR_PASSWORD", "Basic auth password (use ${secret:KEY} for secrets)", ""},
		{"fhir", "FI_FHIR_FHIR_BEARER_TOKEN", "Bearer token (use ${secret:KEY} for secrets)", ""},
		{"fhir", "FI_FHIR_FHIR_OAUTH2_TOKEN_URL", "OAuth2 token endpoint URL", ""},
		{"fhir", "FI_FHIR_FHIR_OAUTH2_CLIENT_ID", "OAuth2 client ID", ""},
		{"fhir", "FI_FHIR_FHIR_OAUTH2_CLIENT_SECRET", "OAuth2 client secret (use ${secret:KEY})", ""},

		// Terminology
		{"terminology", "FI_FHIR_TERMINOLOGY_DB_URL", "PostgreSQL connection string for terminology database", ""},
		{"terminology", "FI_FHIR_TERMINOLOGY_PINS", "Terminology version pins (e.g. \"loinc=2.77,icd10cm=FY2024\")", ""},
		{"terminology", "FI_FHIR_TERMINOLOGY_POLICY", "Pin enforcement policy (pass, warn, error)", "warn"},

		// Database
		{"database", "FI_FHIR_DATABASE_DRIVER", "Database driver (postgres, mysql, sqlite)", "postgres"},
		{"database", "FI_FHIR_DATABASE_HOST", "Database host", ""},
		{"database", "FI_FHIR_DATABASE_PORT", "Database port", "5432"},
		{"database", "FI_FHIR_DATABASE_NAME", "Database name", ""},
		{"database", "FI_FHIR_DATABASE_USERNAME", "Database username", ""},
		{"database", "FI_FHIR_DATABASE_PASSWORD", "Database password (use ${secret:KEY})", ""},
		{"database", "FI_FHIR_DATABASE_SSL_MODE", "SSL mode (disable, require, verify-full)", "require"},
		{"database", "FI_FHIR_DATABASE_MAX_OPEN_CONNS", "Maximum open connections", "25"},
		{"database", "FI_FHIR_DATABASE_MAX_IDLE_CONNS", "Maximum idle connections", "5"},
		{"database", "FI_FHIR_DATABASE_CONN_MAX_LIFETIME", "Connection maximum lifetime", "5m"},

		// Queue
		{"queue", "FI_FHIR_QUEUE_DRIVER", "Queue driver (kafka, rabbitmq, sqs)", "kafka"},
		{"queue", "FI_FHIR_QUEUE_BROKERS", "Comma-separated broker addresses", ""},
		{"queue", "FI_FHIR_QUEUE_USERNAME", "Queue auth username", ""},
		{"queue", "FI_FHIR_QUEUE_PASSWORD", "Queue auth password (use ${secret:KEY})", ""},
		{"queue", "FI_FHIR_QUEUE_TLS", "Enable TLS for queue connections", "false"},

		// Observability
		{"observability", "FI_FHIR_METRICS_ENABLED", "Enable Prometheus metrics endpoint", "true"},
		{"observability", "FI_FHIR_METRICS_ENDPOINT", "Metrics endpoint path", "/metrics"},
		{"observability", "FI_FHIR_METRICS_PORT", "Metrics server port", "9090"},
		{"observability", "FI_FHIR_TRACING_ENABLED", "Enable OpenTelemetry tracing", "false"},
		{"observability", "FI_FHIR_TRACING_ENDPOINT", "Tracing collector endpoint", ""},
		{"observability", "FI_FHIR_TRACING_SAMPLER", "Trace sampling rate (0.0-1.0)", "0.1"},
		{"observability", "FI_FHIR_LOG_LEVEL", "Log level (debug, info, warn, error)", "info"},
		{"observability", "FI_FHIR_LOG_FORMAT", "Log format (json, text)", "json"},

		// Secrets
		{"secrets", "FI_FHIR_SECRETS_PROVIDER", "Secrets provider (env, file, vault, aws-ssm, k8s)", "env"},
	}

	// Filter by section if specified
	if section != "" {
		var filtered []struct {
			section     string
			name        string
			description string
			defaultVal  string
		}
		for _, v := range envVars {
			if v.section == section {
				filtered = append(filtered, v)
			}
		}
		if len(filtered) == 0 {
			return fmt.Errorf("unknown section: %s (available: server, workflow, fhir, database, queue, observability, secrets)", section)
		}
		envVars = filtered
	}

	// Output based on format
	switch format {
	case "list":
		currentSection := ""
		for _, v := range envVars {
			if v.section != currentSection {
				if currentSection != "" {
					fmt.Println()
				}
				currentSection = v.section
				fmt.Printf("# %s\n", strings.ToUpper(currentSection))
			}
			if v.defaultVal != "" {
				fmt.Printf("%-45s # %s (default: %s)\n", v.name+"=", v.description, v.defaultVal)
			} else {
				fmt.Printf("%-45s # %s\n", v.name+"=", v.description)
			}
		}

	case "export":
		for _, v := range envVars {
			if v.defaultVal != "" {
				fmt.Printf("export %s=%q\n", v.name, v.defaultVal)
			} else {
				fmt.Printf("# export %s=\"\"\n", v.name)
			}
		}

	case "markdown":
		fmt.Println("| Variable | Description | Default |")
		fmt.Println("|----------|-------------|---------|")
		for _, v := range envVars {
			def := v.defaultVal
			if def == "" {
				def = "-"
			}
			fmt.Printf("| `%s` | %s | `%s` |\n", v.name, v.description, def)
		}

	default:
		return fmt.Errorf("unknown format: %s (use list, export, or markdown)", format)
	}

	return nil
}

func runConfigInit(args []string) error {
	var (
		outputPath = "config.yaml"
		minimal    = false
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--output":
			if i+1 >= len(args) {
				return fmt.Errorf("--output requires a file path")
			}
			i++
			outputPath = args[i]
		case "-m", "--minimal":
			minimal = true
		case "-h", "--help":
			printConfigInitUsage()
			return nil
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
		}
	}

	// Check if file already exists
	if _, err := os.Stat(outputPath); err == nil {
		return fmt.Errorf("file %s already exists (use different path with -o)", outputPath)
	}

	var content string
	if minimal {
		content = `# fi-fhir minimal configuration
# Full reference: fi-fhir config env

server:
  port: 8080

workflow:
  config_path: workflow.yaml

observability:
  log_level: info
`
	} else {
		content = `# fi-fhir configuration
# Generated by: fi-fhir config init
# Full reference: fi-fhir config env

server:
  host: "0.0.0.0"
  port: 8080
  read_timeout: 30s
  write_timeout: 30s
  shutdown_timeout: 10s
  # tls_cert_file: /path/to/cert.pem
  # tls_key_file: /path/to/key.pem

workflow:
  config_path: workflow.yaml
  dry_run: false
  max_concurrency: 10
  action_timeout: 30s
  retry_max_attempts: 3
  retry_initial_wait: 1s
  retry_max_wait: 30s
  dlq_enabled: true
  dlq_max_size: 10000

fhir:
  # base_url: https://fhir.example.com/r4
  timeout: 30s
  auth_type: none  # none, basic, bearer, oauth2
  # For basic auth:
  # username: user
  # password: ${secret:FHIR_PASSWORD}
  # For bearer:
  # bearer_token: ${secret:FHIR_TOKEN}
  # For OAuth2:
  # oauth2:
  #   token_url: https://auth.example.com/token
  #   client_id: your-client-id
  #   client_secret: ${secret:OAUTH_CLIENT_SECRET}
  #   scopes:
  #     - system/*.read
  #     - system/*.write

database:
  driver: postgres  # postgres, mysql, sqlite
  # host: localhost
  # port: 5432
  # database: fi_fhir
  # username: app
  # password: ${secret:DATABASE_PASSWORD}
  ssl_mode: require
  max_open_conns: 25
  max_idle_conns: 5
  conn_max_lifetime: 5m

queue:
  driver: kafka  # kafka, rabbitmq, sqs
  # brokers:
  #   - localhost:9092
  # username: ""
  # password: ${secret:QUEUE_PASSWORD}
  tls: false
  options: {}

observability:
  metrics_enabled: true
  metrics_endpoint: /metrics
  metrics_port: 9090
  tracing_enabled: false
  # tracing_endpoint: http://jaeger:4317
  tracing_sampler: 0.1
  log_level: info
  log_format: json

secrets:
  provider: env  # env, file, vault, aws-ssm, k8s
  options: {}
  # For file provider:
  # options:
  #   path: /run/secrets
  # For env provider with prefix:
  # options:
  #   prefix: APP_SECRET_
`
	}

	if err := os.WriteFile(outputPath, []byte(content), 0644); err != nil { //nolint:gosec // G306: config template file
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("Created %s\n", outputPath)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Edit the configuration file with your settings")
	fmt.Println("  2. Validate: fi-fhir config validate " + outputPath)
	fmt.Println("  3. View effective config: fi-fhir config show -c " + outputPath)

	return nil
}

// marshalYAML marshals config to YAML format (simple implementation)
func marshalYAML(cfg *config.Config) ([]byte, error) {
	// Use JSON as intermediate since we already have JSON tags
	jsonData, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}

	var data map[string]interface{}
	if err := json.Unmarshal(jsonData, &data); err != nil {
		return nil, err
	}

	// Simple YAML output (for a full implementation, use yaml.Marshal)
	return json.MarshalIndent(data, "", "  ")
}

func printConfigUsage() {
	fmt.Println(`fi-fhir config - Configuration management

Usage:
  fi-fhir config <subcommand> [options]

Subcommands:
  show        Show effective configuration
  validate    Validate configuration file
  env         List available environment variables
  init        Generate sample configuration file

Examples:
  # Show default configuration
  fi-fhir config show

  # Show configuration from file with env overrides
  fi-fhir config show -c config.yaml

  # Validate configuration file
  fi-fhir config validate config.yaml

  # List all environment variables
  fi-fhir config env

  # Generate environment variable export script
  fi-fhir config env -f export > .env

  # Generate sample config file
  fi-fhir config init -o config.yaml`)
}

func printConfigShowUsage() {
	fmt.Println(`fi-fhir config show - Show effective configuration

Usage:
  fi-fhir config show [options] [config-file]

Options:
  -c, --config <file>   Configuration file to load
  -f, --format <fmt>    Output format: yaml, json (default: yaml)
  -h, --help            Show this help message

Description:
  Shows the effective configuration after applying:
  1. Default values
  2. Config file settings (if provided)
  3. Environment variable overrides
  4. Secret resolution

Examples:
  # Show defaults with env overrides
  fi-fhir config show

  # Show config from file
  fi-fhir config show -c config.yaml

  # Output as JSON
  fi-fhir config show -c config.yaml -f json`)
}

func printConfigValidateUsage() {
	fmt.Println(`fi-fhir config validate - Validate configuration file

Usage:
  fi-fhir config validate <config-file>

Options:
  -h, --help            Show this help message

Description:
  Validates a configuration file for:
  - YAML syntax
  - Required fields
  - Valid enum values (drivers, auth types, log levels)
  - Value constraints (ports, timeouts, sampler rates)

Examples:
  fi-fhir config validate config.yaml`)
}

func printConfigEnvUsage() {
	fmt.Println(`fi-fhir config env - List environment variables

Usage:
  fi-fhir config env [options]

Options:
  -f, --format <fmt>    Output format: list, export, markdown (default: list)
  -s, --section <name>  Filter by section (server, workflow, fhir, database, queue, observability, secrets)
  -h, --help            Show this help message

Description:
  Lists all environment variables that can be used to configure fi-fhir.
  Environment variables override config file settings.

Output Formats:
  list      Human-readable list with comments (default)
  export    Shell export statements for .env files
  markdown  Markdown table for documentation

Examples:
  # List all variables
  fi-fhir config env

  # Show only database variables
  fi-fhir config env -s database

  # Generate .env template
  fi-fhir config env -f export > .env.template

  # Generate documentation
  fi-fhir config env -f markdown >> docs/configuration.md`)
}

func printConfigInitUsage() {
	fmt.Println(`fi-fhir config init - Generate sample configuration

Usage:
  fi-fhir config init [options]

Options:
  -o, --output <file>   Output file path (default: config.yaml)
  -m, --minimal         Generate minimal config (defaults only)
  -h, --help            Show this help message

Description:
  Generates a sample configuration file with all options documented.
  The generated file includes comments explaining each setting.

Examples:
  # Generate full config with comments
  fi-fhir config init

  # Generate to specific path
  fi-fhir config init -o /etc/fi-fhir/config.yaml

  # Generate minimal config
  fi-fhir config init -m -o minimal.yaml`)
}

// --- Subscription Commands ---

func runSubscription(args []string) error {
	if len(args) == 0 {
		printSubscriptionUsage()
		return nil
	}

	switch args[0] {
	case "list":
		return runSubscriptionList(args[1:])
	case "status":
		return runSubscriptionStatus(args[1:])
	case "create":
		return runSubscriptionCreate(args[1:])
	case "delete":
		return runSubscriptionDelete(args[1:])
	case "pause":
		return runSubscriptionPauseResume(args[1:], true)
	case "resume":
		return runSubscriptionPauseResume(args[1:], false)
	case "serve":
		return runSubscriptionServe(args[1:])
	case "validate":
		return runSubscriptionValidate(args[1:])
	case "test":
		return runSubscriptionTest(args[1:])
	case "help", "--help", "-h":
		printSubscriptionUsage()
		return nil
	default:
		return fmt.Errorf("unknown subscription command: %s", args[0])
	}
}

func printSubscriptionUsage() {
	fmt.Println(`fi-fhir subscription - Manage FHIR R4 Subscriptions

Bidirectional FHIR integration: receive events FROM FHIR servers via subscriptions.

Usage:
  fi-fhir subscription <command> [arguments]

Commands:
  list      List configured subscriptions
  status    Show subscription status
  create    Create a subscription on a FHIR server
  delete    Delete a subscription from a FHIR server
  pause     Pause a subscription (set status to off)
  resume    Resume a paused subscription
  serve     Start notification receiver server
  validate  Validate subscription configuration
  test      Test subscription endpoint with sample notification
  help      Show this help message

Examples:
  # List configured subscriptions
  fi-fhir subscription list --config subscriptions.yaml

  # Create subscription on FHIR server
  fi-fhir subscription create --config subscriptions.yaml --name patient_changes

  # Start notification receiver with workflow
  fi-fhir subscription serve --subscriptions subscriptions.yaml --workflow workflow.yaml

  # Validate subscription configuration
  fi-fhir subscription validate subscriptions.yaml

For more information, see: docs/planning/FHIR-SUBSCRIPTIONS.md`)
}

func runSubscriptionList(args []string) error {
	var configPath string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config", "-c":
			if i+1 >= len(args) {
				return fmt.Errorf("--config requires a value")
			}
			i++
			configPath = args[i]
		case "--help", "-h":
			printSubscriptionListUsage()
			return nil
		default:
			if configPath == "" && !strings.HasPrefix(args[i], "-") {
				configPath = args[i]
			}
		}
	}

	if configPath == "" {
		return fmt.Errorf("configuration file required")
	}

	config, err := subscription.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	fmt.Printf("Configured Subscriptions (%d):\n\n", len(config.Subscriptions))
	for _, sub := range config.Subscriptions {
		fmt.Printf("  Name:        %s\n", sub.Name)
		if sub.Description != "" {
			fmt.Printf("  Description: %s\n", sub.Description)
		}
		fmt.Printf("  Server:      %s\n", sub.Server)
		fmt.Printf("  Criteria:    %s\n", sub.Criteria)
		fmt.Printf("  Endpoint:    %s\n", sub.Channel.Endpoint)
		fmt.Println()
	}

	return nil
}

func printSubscriptionListUsage() {
	fmt.Println(`fi-fhir subscription list - List configured subscriptions

Usage:
  fi-fhir subscription list [options] [config-file]

Options:
  -c, --config <file>   Subscriptions configuration file
  -h, --help            Show this help message

Examples:
  fi-fhir subscription list subscriptions.yaml
  fi-fhir subscription list --config /etc/fi-fhir/subscriptions.yaml`)
}

func runSubscriptionStatus(args []string) error {
	var (
		configPath string
		name       string
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config", "-c":
			if i+1 >= len(args) {
				return fmt.Errorf("--config requires a value")
			}
			i++
			configPath = args[i]
		case "--name", "-n":
			if i+1 >= len(args) {
				return fmt.Errorf("--name requires a value")
			}
			i++
			name = args[i]
		case "--help", "-h":
			fmt.Println("Usage: fi-fhir subscription status --config <file> --name <subscription>")
			return nil
		default:
			if name == "" && !strings.HasPrefix(args[i], "-") {
				name = args[i]
			}
		}
	}

	if configPath == "" {
		return fmt.Errorf("--config is required")
	}
	if name == "" {
		return fmt.Errorf("subscription name is required")
	}

	config, err := subscription.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Find subscription
	var subDef *subscription.SubscriptionDefinition
	for i := range config.Subscriptions {
		if config.Subscriptions[i].Name == name {
			subDef = &config.Subscriptions[i]
			break
		}
	}

	if subDef == nil {
		return fmt.Errorf("subscription %q not found", name)
	}

	// Create client and query status
	var auth subscription.AuthProvider
	if subDef.Auth.Token != "" {
		auth = &subscription.StaticTokenAuth{Token: subDef.Auth.Token}
	}

	client, err := subscription.NewClient(&subscription.ClientConfig{
		FHIREndpoint: subDef.Server,
		AuthProvider: auth,
	})
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	// List subscriptions on server
	subs, err := client.List(context.Background())
	if err != nil {
		return fmt.Errorf("list subscriptions: %w", err)
	}

	fmt.Printf("Subscription: %s\n", name)
	fmt.Printf("Server:       %s\n", subDef.Server)
	fmt.Printf("Criteria:     %s\n", subDef.Criteria)
	fmt.Printf("Endpoint:     %s\n", subDef.Channel.Endpoint)
	fmt.Println()

	// Find matching subscription on server
	found := false
	for _, sub := range subs {
		if sub.Channel.Endpoint == subDef.Channel.Endpoint {
			fmt.Printf("Server Status:\n")
			fmt.Printf("  ID:     %s\n", sub.ID)
			fmt.Printf("  Status: %s\n", sub.Status)
			if sub.Error != "" {
				fmt.Printf("  Error:  %s\n", sub.Error)
			}
			found = true
			break
		}
	}

	if !found {
		fmt.Println("Server Status: NOT REGISTERED")
		fmt.Println("  Run 'fi-fhir subscription create' to register")
	}

	return nil
}

func runSubscriptionCreate(args []string) error {
	var (
		configPath string
		name       string
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config", "-c":
			if i+1 >= len(args) {
				return fmt.Errorf("--config requires a value")
			}
			i++
			configPath = args[i]
		case "--name", "-n":
			if i+1 >= len(args) {
				return fmt.Errorf("--name requires a value")
			}
			i++
			name = args[i]
		case "--help", "-h":
			fmt.Println("Usage: fi-fhir subscription create --config <file> --name <subscription>")
			return nil
		default:
			if name == "" && !strings.HasPrefix(args[i], "-") {
				name = args[i]
			}
		}
	}

	if configPath == "" {
		return fmt.Errorf("--config is required")
	}
	if name == "" {
		return fmt.Errorf("subscription name is required")
	}

	config, err := subscription.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Find subscription
	var subDef *subscription.SubscriptionDefinition
	for i := range config.Subscriptions {
		if config.Subscriptions[i].Name == name {
			subDef = &config.Subscriptions[i]
			break
		}
	}

	if subDef == nil {
		return fmt.Errorf("subscription %q not found in config", name)
	}

	// Create client
	var auth subscription.AuthProvider
	if subDef.Auth.Token != "" {
		auth = &subscription.StaticTokenAuth{Token: subDef.Auth.Token}
	}

	client, err := subscription.NewClient(&subscription.ClientConfig{
		FHIREndpoint: subDef.Server,
		AuthProvider: auth,
	})
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	// Create subscription
	sub := &subscription.Subscription{
		Status:   subscription.StatusRequested,
		Reason:   subDef.Description,
		Criteria: subDef.Criteria,
		Channel: subscription.Channel{
			Type:     subscription.ChannelRestHook,
			Endpoint: subDef.Channel.Endpoint,
			Payload:  subDef.Channel.Payload,
			Header:   subDef.Channel.Headers,
		},
	}

	created, err := client.Create(context.Background(), sub)
	if err != nil {
		return fmt.Errorf("create subscription: %w", err)
	}

	fmt.Printf("Subscription created successfully!\n")
	fmt.Printf("  ID:       %s\n", created.ID)
	fmt.Printf("  Status:   %s\n", created.Status)
	fmt.Printf("  Criteria: %s\n", created.Criteria)
	fmt.Printf("  Endpoint: %s\n", created.Channel.Endpoint)

	return nil
}

func runSubscriptionDelete(args []string) error {
	var (
		configPath     string
		name           string
		subscriptionID string
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config", "-c":
			if i+1 >= len(args) {
				return fmt.Errorf("--config requires a value")
			}
			i++
			configPath = args[i]
		case "--name", "-n":
			if i+1 >= len(args) {
				return fmt.Errorf("--name requires a value")
			}
			i++
			name = args[i]
		case "--id":
			if i+1 >= len(args) {
				return fmt.Errorf("--id requires a value")
			}
			i++
			subscriptionID = args[i]
		case "--help", "-h":
			fmt.Println("Usage: fi-fhir subscription delete --config <file> --name <subscription> --id <sub-id>")
			return nil
		}
	}

	if configPath == "" {
		return fmt.Errorf("--config is required")
	}
	if name == "" {
		return fmt.Errorf("--name is required")
	}
	if subscriptionID == "" {
		return fmt.Errorf("--id is required")
	}

	config, err := subscription.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Find subscription
	var subDef *subscription.SubscriptionDefinition
	for i := range config.Subscriptions {
		if config.Subscriptions[i].Name == name {
			subDef = &config.Subscriptions[i]
			break
		}
	}

	if subDef == nil {
		return fmt.Errorf("subscription %q not found in config", name)
	}

	// Create client
	var auth subscription.AuthProvider
	if subDef.Auth.Token != "" {
		auth = &subscription.StaticTokenAuth{Token: subDef.Auth.Token}
	}

	client, err := subscription.NewClient(&subscription.ClientConfig{
		FHIREndpoint: subDef.Server,
		AuthProvider: auth,
	})
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	if err := client.Delete(context.Background(), subscriptionID); err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}

	fmt.Printf("Subscription %s deleted successfully\n", subscriptionID)
	return nil
}

func runSubscriptionPauseResume(args []string, pause bool) error {
	var (
		configPath     string
		name           string
		subscriptionID string
	)

	action := "resume"
	if pause {
		action = "pause"
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config", "-c":
			if i+1 >= len(args) {
				return fmt.Errorf("--config requires a value")
			}
			i++
			configPath = args[i]
		case "--name", "-n":
			if i+1 >= len(args) {
				return fmt.Errorf("--name requires a value")
			}
			i++
			name = args[i]
		case "--id":
			if i+1 >= len(args) {
				return fmt.Errorf("--id requires a value")
			}
			i++
			subscriptionID = args[i]
		case "--help", "-h":
			fmt.Printf("Usage: fi-fhir subscription %s --config <file> --name <subscription> --id <sub-id>\n", action)
			return nil
		}
	}

	if configPath == "" {
		return fmt.Errorf("--config is required")
	}
	if name == "" {
		return fmt.Errorf("--name is required")
	}
	if subscriptionID == "" {
		return fmt.Errorf("--id is required")
	}

	config, err := subscription.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Find subscription
	var subDef *subscription.SubscriptionDefinition
	for i := range config.Subscriptions {
		if config.Subscriptions[i].Name == name {
			subDef = &config.Subscriptions[i]
			break
		}
	}

	if subDef == nil {
		return fmt.Errorf("subscription %q not found in config", name)
	}

	// Create client
	var auth subscription.AuthProvider
	if subDef.Auth.Token != "" {
		auth = &subscription.StaticTokenAuth{Token: subDef.Auth.Token}
	}

	client, err := subscription.NewClient(&subscription.ClientConfig{
		FHIREndpoint: subDef.Server,
		AuthProvider: auth,
	})
	if err != nil {
		return fmt.Errorf("create client: %w", err)
	}

	if pause {
		err = client.Pause(context.Background(), subscriptionID)
	} else {
		err = client.Resume(context.Background(), subscriptionID)
	}

	if err != nil {
		return fmt.Errorf("%s subscription: %w", action, err)
	}

	fmt.Printf("Subscription %s %sd successfully\n", subscriptionID, action)
	return nil
}

func runSubscriptionServe(args []string) error {
	var (
		subscriptionsPath string
		workflowPath      string
		configPath        string
		host              string
		port              int
		portSet           bool
		certFile          string
		keyFile           string
		dryRun            bool
	)

	port = 8081 // default

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--subscriptions", "-s":
			if i+1 >= len(args) {
				return fmt.Errorf("--subscriptions requires a value")
			}
			i++
			subscriptionsPath = args[i]
		case "--workflow", "-w":
			if i+1 >= len(args) {
				return fmt.Errorf("--workflow requires a value")
			}
			i++
			workflowPath = args[i]
		case "--config", "-c":
			if i+1 >= len(args) {
				return fmt.Errorf("--config requires a value")
			}
			i++
			configPath = args[i]
		case "--host":
			if i+1 >= len(args) {
				return fmt.Errorf("--host requires a value")
			}
			i++
			host = args[i]
		case "--port", "-p":
			if i+1 >= len(args) {
				return fmt.Errorf("--port requires a value")
			}
			i++
			var err error
			port, err = strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("invalid port: %s", args[i])
			}
			portSet = true
		case "--cert":
			if i+1 >= len(args) {
				return fmt.Errorf("--cert requires a value")
			}
			i++
			certFile = args[i]
		case "--key":
			if i+1 >= len(args) {
				return fmt.Errorf("--key requires a value")
			}
			i++
			keyFile = args[i]
		case "--dry-run":
			dryRun = true
		case "--help", "-h":
			printSubscriptionServeUsage()
			return nil
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
			return fmt.Errorf("unexpected arg: %s", args[i])
		}
	}

	if subscriptionsPath == "" {
		return fmt.Errorf("--subscriptions is required")
	}

	fullConfig, err := subscription.LoadFullConfig(subscriptionsPath, configPath)
	if err != nil {
		return fmt.Errorf("load subscription config: %w", err)
	}

	// Create event router
	var router subscription.EventRouter

	if workflowPath != "" {
		// Load workflow
		wfConfig, err := workflow.LoadWorkflow(workflowPath)
		if err != nil {
			return fmt.Errorf("load workflow: %w", err)
		}

		engine, err := workflow.NewEngine(wfConfig)
		if err != nil {
			return fmt.Errorf("create workflow engine: %w", err)
		}
		router = subscription.NewWorkflowRouter(engine)
		fmt.Printf("Loaded workflow: %s\n", wfConfig.Name)
	} else {
		// Use logging router
		router = subscription.NewCallbackRouter(func(ctx context.Context, event interface{}) error {
			data, _ := json.MarshalIndent(event, "", "  ")
			fmt.Printf("Event received:\n%s\n", string(data))
			return nil
		})
		fmt.Println("No workflow specified, using logging router")
	}

	pathPrefix := fullConfig.Receiver.PathPrefix
	if pathPrefix == "" {
		pathPrefix = "/fhir/notify"
	}
	if !strings.HasPrefix(pathPrefix, "/") {
		pathPrefix = "/" + pathPrefix
	}

	receiverOpts := &subscription.ReceiverOptions{
		PathPrefix:     pathPrefix,
		MaxBundleSize:  fullConfig.Receiver.MaxBundleSize,
		AllowedSources: fullConfig.Receiver.AllowedSources,
		VerifySource:   fullConfig.Receiver.VerifySource,
	}

	receiver := subscription.NewReceiver(router, receiverOpts)

	// Register subscriptions
	for _, sub := range fullConfig.Subscriptions {
		receiver.RegisterSubscription(&subscription.SubscriptionConfig{
			Name:         sub.Name,
			EventMapping: sub.Mapping,
		})
		fmt.Printf("Registered subscription: %s\n", sub.Name)
	}

	// Create server
	if host == "" {
		host = fullConfig.Receiver.Host
	}
	if host == "" {
		host = "0.0.0.0"
	}
	if !portSet {
		port = fullConfig.Receiver.Port
	}
	if port == 0 {
		port = 8081
	}

	tlsCert := certFile
	tlsKey := keyFile
	if tlsCert == "" && tlsKey == "" && fullConfig.Receiver.TLS.Enabled {
		tlsCert = fullConfig.Receiver.TLS.CertFile
		tlsKey = fullConfig.Receiver.TLS.KeyFile
	}
	if (tlsCert == "") != (tlsKey == "") {
		return fmt.Errorf("both TLS cert and key are required (use --cert/--key or set subscription_receiver.tls)")
	}

	if dryRun {
		fmt.Println("Dry-run: subscription receiver config")
		fmt.Printf("  Subscriptions: %s (%d)\n", subscriptionsPath, len(fullConfig.Subscriptions))
		if configPath != "" {
			fmt.Printf("  Config: %s\n", configPath)
		}
		fmt.Printf("  Bind: %s:%d\n", host, port)
		fmt.Printf("  Path prefix: %s\n", pathPrefix)
		fmt.Printf("  Max bundle size: %d\n", receiverOpts.MaxBundleSize)
		fmt.Printf("  Verify source: %v\n", receiverOpts.VerifySource)
		if tlsCert != "" && tlsKey != "" {
			fmt.Printf("  TLS: enabled (cert=%s key=%s)\n", tlsCert, tlsKey)
		} else {
			fmt.Printf("  TLS: disabled\n")
		}
		if workflowPath != "" {
			fmt.Printf("  Workflow: %s\n", workflowPath)
		} else {
			fmt.Printf("  Workflow: (none)\n")
		}
		return nil
	}

	server := subscription.NewServer(receiver, &subscription.ServerConfig{
		Host: host,
		Port: port,
	})

	// Setup signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx) // Graceful shutdown in signal handler
	}()

	fmt.Printf("Starting subscription receiver on %s:%d\n", host, port)
	fmt.Printf("Notification endpoint: http://%s:%d%s/<subscription>\n", host, port, pathPrefix)

	var serveErr error
	if tlsCert != "" && tlsKey != "" {
		fmt.Println("TLS enabled")
		serveErr = server.Start(tlsCert, tlsKey)
	} else {
		fmt.Println("WARNING: TLS not enabled. Use --cert and --key for production.")
		serveErr = server.Start("", "")
	}

	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}

	return nil
}

func printSubscriptionServeUsage() {
	fmt.Println(`fi-fhir subscription serve - Start notification receiver server

Usage:
  fi-fhir subscription serve [options]

Options:
  -s, --subscriptions <file>  Subscriptions configuration file (required)
  -w, --workflow <file>       Workflow configuration for event routing
  -c, --config <file>         Application configuration file
      --host <host>           Bind address (default: 0.0.0.0)
  -p, --port <port>           Listen port (default: 8081)
      --cert <file>           TLS certificate file
      --key <file>            TLS key file
      --dry-run               Print effective receiver config and exit
  -h, --help                  Show this help message

Description:
  Starts an HTTP server that receives FHIR subscription notifications.
  Notifications are mapped to canonical events and routed through the
  workflow engine for processing.

Examples:
  # Start with workflow routing
  fi-fhir subscription serve \
    --subscriptions subscriptions.yaml \
    --workflow workflow.yaml

  # Start with TLS
  fi-fhir subscription serve \
    --subscriptions subscriptions.yaml \
    --workflow workflow.yaml \
    --cert /etc/fi-fhir/tls/cert.pem \
    --key /etc/fi-fhir/tls/key.pem

  # Start on custom port
  fi-fhir subscription serve \
    --subscriptions subscriptions.yaml \
    --port 9090`)
}

func runSubscriptionValidate(args []string) error {
	var configPath string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--help", "-h":
			fmt.Println("Usage: fi-fhir subscription validate <config-file>")
			return nil
		default:
			if configPath == "" && !strings.HasPrefix(args[i], "-") {
				configPath = args[i]
			}
		}
	}

	if configPath == "" {
		return fmt.Errorf("configuration file required")
	}

	config, err := subscription.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	fmt.Printf("✓ Configuration valid\n")
	fmt.Printf("  File: %s\n", configPath)
	fmt.Printf("  Subscriptions: %d\n", len(config.Subscriptions))

	for _, sub := range config.Subscriptions {
		fmt.Printf("    - %s (%s)\n", sub.Name, sub.Criteria)
	}

	return nil
}

func runSubscriptionTest(args []string) error {
	var (
		configPath   string
		name         string
		resourceFile string
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--config", "-c":
			if i+1 >= len(args) {
				return fmt.Errorf("--config requires a value")
			}
			i++
			configPath = args[i]
		case "--name", "-n":
			if i+1 >= len(args) {
				return fmt.Errorf("--name requires a value")
			}
			i++
			name = args[i]
		case "--resource", "-r":
			if i+1 >= len(args) {
				return fmt.Errorf("--resource requires a value")
			}
			i++
			resourceFile = args[i]
		case "--help", "-h":
			printSubscriptionTestUsage()
			return nil
		}
	}

	if configPath == "" {
		return fmt.Errorf("--config is required")
	}
	if name == "" {
		return fmt.Errorf("--name is required")
	}
	if resourceFile == "" {
		return fmt.Errorf("--resource is required")
	}

	// Load subscription config
	config, err := subscription.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	// Find subscription
	var subDef *subscription.SubscriptionDefinition
	for i := range config.Subscriptions {
		if config.Subscriptions[i].Name == name {
			subDef = &config.Subscriptions[i]
			break
		}
	}

	if subDef == nil {
		return fmt.Errorf("subscription %q not found", name)
	}

	// Load resource
	data, err := os.ReadFile(resourceFile) //nolint:gosec // G304: CLI tool reads user-specified file
	if err != nil {
		return fmt.Errorf("read resource: %w", err)
	}

	var resource map[string]interface{}
	if err := json.Unmarshal(data, &resource); err != nil {
		return fmt.Errorf("parse resource: %w", err)
	}

	// Create notification bundle
	bundle := subscription.NotificationBundle{
		ResourceType: "Bundle",
		Type:         "history",
		Entry: []subscription.NotificationEntry{
			{
				Resource: resource,
				Request: &subscription.EntryRequest{
					Method: "POST",
					URL:    fmt.Sprintf("%s/%s", resource["resourceType"], resource["id"]),
				},
			},
		},
	}

	// Map to canonical event
	mapper := subscription.NewFHIRMapper()
	events, err := mapper.MapBundle(&bundle, &subDef.Mapping)
	if err != nil {
		return fmt.Errorf("map resource: %w", err)
	}

	fmt.Printf("Test Results:\n")
	fmt.Printf("  Subscription: %s\n", name)
	fmt.Printf("  Resource: %s/%s\n", resource["resourceType"], resource["id"])
	fmt.Printf("  Events generated: %d\n\n", len(events))

	for i, event := range events {
		data, _ := json.MarshalIndent(event, "  ", "  ")
		fmt.Printf("Event %d:\n  %s\n", i+1, string(data))
	}

	return nil
}

func printSubscriptionTestUsage() {
	fmt.Println(`fi-fhir subscription test - Test subscription with sample notification

Usage:
  fi-fhir subscription test [options]

Options:
  -c, --config <file>    Subscriptions configuration file (required)
  -n, --name <name>      Subscription name to test (required)
  -r, --resource <file>  FHIR resource JSON file (required)
  -h, --help             Show this help message

Description:
  Simulates receiving a notification for the specified subscription.
  Shows how the resource would be mapped to canonical events.

Examples:
  fi-fhir subscription test \
    --config subscriptions.yaml \
    --name patient_changes \
    --resource testdata/patient.json`)
}

// =============================================================================
// Serve Command - GraphQL API Server
// =============================================================================

func runServe(args []string) error {
	var (
		host           = "0.0.0.0"
		port           = 8081
		path           = "/graphql"
		playgroundPath = "/"
		playground     = true
		introspection  = true
		workflowPath   = ""
		maxDepth       = 10
		maxComplexity  = 1000
		timeout        = 30 * time.Second
		dryRun         = false
	)

	// Parse flags
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--host", "-H":
			if i+1 >= len(args) {
				return fmt.Errorf("--host requires a value")
			}
			i++
			host = args[i]
		case "--port", "-p":
			if i+1 >= len(args) {
				return fmt.Errorf("--port requires a value")
			}
			i++
			var err error
			port, err = strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("invalid port: %s", args[i])
			}
		case "--path":
			if i+1 >= len(args) {
				return fmt.Errorf("--path requires a value")
			}
			i++
			path = args[i]
		case "--playground-path":
			if i+1 >= len(args) {
				return fmt.Errorf("--playground-path requires a value")
			}
			i++
			playgroundPath = args[i]
		case "--no-playground":
			playground = false
		case "--no-introspection":
			introspection = false
		case "--workflow", "-w":
			if i+1 >= len(args) {
				return fmt.Errorf("--workflow requires a value")
			}
			i++
			workflowPath = args[i]
		case "--max-depth":
			if i+1 >= len(args) {
				return fmt.Errorf("--max-depth requires a value")
			}
			i++
			var err error
			maxDepth, err = strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("invalid max-depth: %s", args[i])
			}
		case "--max-complexity":
			if i+1 >= len(args) {
				return fmt.Errorf("--max-complexity requires a value")
			}
			i++
			var err error
			maxComplexity, err = strconv.Atoi(args[i])
			if err != nil {
				return fmt.Errorf("invalid max-complexity: %s", args[i])
			}
		case "--timeout":
			if i+1 >= len(args) {
				return fmt.Errorf("--timeout requires a value")
			}
			i++
			var err error
			timeout, err = time.ParseDuration(args[i])
			if err != nil {
				return fmt.Errorf("invalid timeout: %s", args[i])
			}
		case "--dry-run":
			dryRun = true
		case "--help", "-h":
			printServeUsage()
			return nil
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
		}
	}

	var (
		loadedWorkflow *workflow.Workflow
		workflowEngine *workflow.Engine
	)

	if workflowPath != "" {
		w, err := workflow.LoadWorkflow(workflowPath)
		if err != nil {
			return fmt.Errorf("failed to load workflow: %w", err)
		}
		if errors := w.Validate(); len(errors) > 0 {
			fmt.Fprintf(os.Stderr, "Workflow validation warnings:\n")
			for _, e := range errors {
				fmt.Fprintf(os.Stderr, "  - %v\n", e)
			}
		}
		engine, err := workflow.NewEngine(w)
		if err != nil {
			return fmt.Errorf("failed to create workflow engine: %w", err)
		}
		loadedWorkflow = w
		workflowEngine = engine
	}

	if dryRun {
		type workflowInfo struct {
			Path   string `json:"path"`
			Name   string `json:"name"`
			Routes int    `json:"routes"`
		}
		type serveDryRunOutput struct {
			Host              string        `json:"host"`
			Port              int           `json:"port"`
			Path              string        `json:"path"`
			PlaygroundPath    string        `json:"playground_path"`
			PlaygroundEnabled bool          `json:"playground_enabled"`
			WebSocketPath     string        `json:"websocket_path"`
			MaxDepth          int           `json:"max_depth"`
			MaxComplexity     int           `json:"max_complexity"`
			Timeout           time.Duration `json:"-"`
			TimeoutString     string        `json:"timeout"`
			Introspection     bool          `json:"introspection"`
			Workflow          *workflowInfo `json:"workflow,omitempty"`
		}

		var wf *workflowInfo
		if loadedWorkflow != nil {
			wf = &workflowInfo{
				Path:   workflowPath,
				Name:   loadedWorkflow.Name,
				Routes: len(loadedWorkflow.Routes),
			}
		}

		out := serveDryRunOutput{
			Host:              host,
			Port:              port,
			Path:              path,
			PlaygroundPath:    playgroundPath,
			PlaygroundEnabled: playground,
			WebSocketPath:     path + "/ws",
			MaxDepth:          maxDepth,
			MaxComplexity:     maxComplexity,
			Timeout:           timeout,
			TimeoutString:     timeout.String(),
			Introspection:     introspection,
			Workflow:          wf,
		}

		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to render dry-run output: %w", err)
		}
		fmt.Println(string(b))
		return nil
	}

	// Enforce terminology version pins (if configured)
	dbURL, pins, policy := loadTerminologyPinConfigFromEnv()
	if pinWarnings, err := checkTerminologyPins(context.Background(), dbURL, pins, policy); err != nil {
		return err
	} else if len(pinWarnings) > 0 {
		fmt.Fprintf(os.Stderr, "Terminology pin warnings (%d):\n", len(pinWarnings))
		for _, w := range pinWarnings {
			fmt.Fprintf(os.Stderr, "  [%s] %s: %s\n", w.Phase, w.Code, w.Message)
		}
	}

	// Build resolver options
	resolverOpts := []resolvers.ResolverOption{
		resolvers.WithVersion(version),
	}

	if profileStore, err := initProfileStoreFromEnv(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: profile store disabled: %v\n", err)
	} else if profileStore != nil {
		resolverOpts = append(resolverOpts, resolvers.WithProfileStore(profileStore))
	}

	// Load workflow engine if specified
	if workflowEngine != nil && loadedWorkflow != nil {
		resolverOpts = append(resolverOpts, resolvers.WithWorkflowEngine(workflowEngine))
		fmt.Printf("Loaded workflow: %s (%d routes)\n", loadedWorkflow.Name, len(loadedWorkflow.Routes))
	}

	// Create resolver
	resolver := resolvers.NewResolver(resolverOpts...)

	// Create server config
	serverConfig := &graphql.ServerConfig{
		Host:              host,
		Port:              port,
		Path:              path,
		PlaygroundEnabled: playground,
		PlaygroundPath:    playgroundPath,
		WebSocketPath:     path + "/ws",
		MaxDepth:          maxDepth,
		MaxComplexity:     maxComplexity,
		Timeout:           timeout,
		Introspection:     introspection,
		AllowedOrigins:    []string{"*"},
	}

	// Create and start server
	server := graphql.NewServer(resolver, serverConfig)

	// Set up signal handling for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start()
	}()

	// Wait for signal or error
	select {
	case sig := <-sigCh:
		fmt.Printf("\nReceived %v, shutting down...\n", sig)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return server.Shutdown(ctx)
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}
}

func printServeUsage() {
	fmt.Println(`fi-fhir serve - Start GraphQL API server

Start a GraphQL API server for healthcare event management. The server provides:
- Queries for events and patients
- Mutations to submit messages and events
- Subscriptions for real-time event streaming
- Interactive GraphQL Playground (optional)

Usage:
  fi-fhir serve [options]

Options:
  -H, --host <addr>         Host address to bind (default: 0.0.0.0)
  -p, --port <port>         Port to listen on (default: 8081)
      --path <path>         GraphQL endpoint path (default: /graphql)
      --playground-path <p> Playground path (default: /)
      --no-playground       Disable GraphQL Playground
      --no-introspection    Disable GraphQL introspection
  -w, --workflow <file>     Workflow DSL file for event processing
      --max-depth <n>       Maximum query depth (default: 10)
      --max-complexity <n>  Maximum query complexity (default: 1000)
      --timeout <duration>  Request timeout (default: 30s)
      --dry-run             Print effective server config and exit
  -h, --help                Show this help message

Endpoints:
  GET  /          - GraphQL Playground (if enabled)
  POST /graphql   - GraphQL query endpoint
  WS   /graphql/ws - GraphQL WebSocket subscriptions
  GET  /health    - Health check endpoint

Examples:
  # Start server with defaults
  fi-fhir serve

  # Start on custom port with workflow
  fi-fhir serve --port 8080 --workflow workflow.yaml

  # Production mode (no playground, no introspection)
  fi-fhir serve --no-playground --no-introspection --port 443

  # With custom timeouts
  fi-fhir serve --timeout 60s --max-depth 15

GraphQL Query Examples:
  # List recent events
  query { events(first: 10) { edges { node { id type timestamp } } } }

  # Get patient info
  query { patient(mrn: "MRN001") { familyName givenName dateOfBirth } }

  # Subscribe to events
  subscription { eventStream { ... on LabResultEvent { patient { mrn } } } }`)
}
