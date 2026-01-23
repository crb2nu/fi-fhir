package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/llm/explain"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm/copilot"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/index"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/semantic"
)

// runWorkflowGenerate generates a workflow from natural language description.
func runWorkflowGenerate(args []string) error {
	var (
		interactive = false
		jsonOutput  = false
		description = ""
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--interactive", "-i":
			interactive = true
		case "--json":
			jsonOutput = true
		case "--help", "-h":
			printWorkflowGenerateUsage()
			return nil
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
			if description == "" {
				description = args[i]
			}
		}
	}

	if description == "" && !interactive {
		return fmt.Errorf("description required (or use --interactive)")
	}

	// Create LLM client
	llmClient, err := llm.NewWithDefaults()
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	// Create copilot
	cop, err := copilot.NewWorkflowCopilot(copilot.CopilotConfig{
		Client: llmClient,
	})
	if err != nil {
		return fmt.Errorf("create copilot: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if interactive {
		return runInteractiveWorkflowSession(ctx, cop)
	}

	// Generate workflow
	result, err := cop.Generate(ctx, copilot.GenerateRequest{
		Description: description,
	})
	if err != nil {
		return fmt.Errorf("generate workflow: %w", err)
	}

	if jsonOutput {
		output, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(output))
	} else {
		fmt.Println("# Generated Workflow")
		fmt.Println()
		fmt.Println(result.Explanation)
		fmt.Println()
		fmt.Println("```yaml")
		fmt.Println(result.YAML)
		fmt.Println("```")
		if len(result.Warnings) > 0 {
			fmt.Println()
			fmt.Println("## Warnings")
			for _, w := range result.Warnings {
				fmt.Printf("- %s\n", w)
			}
		}
	}

	return nil
}

func runInteractiveWorkflowSession(ctx context.Context, cop *copilot.WorkflowCopilot) error {
	session := cop.NewInteractiveSession()

	fmt.Println("Interactive workflow builder started. Type 'quit' to exit.")
	fmt.Println()

	reader := os.Stdin

	for {
		fmt.Print("> ")
		var input string
		buf := make([]byte, 4096)
		n, err := reader.Read(buf)
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		input = string(buf[:n])
		input = input[:len(input)-1] // remove newline

		if input == "quit" || input == "exit" {
			break
		}

		if input == "show" || input == "yaml" {
			yaml := session.GetCurrentYAML()
			if yaml == "" {
				fmt.Println("No workflow generated yet.")
			} else {
				fmt.Println("```yaml")
				fmt.Println(yaml)
				fmt.Println("```")
			}
			continue
		}

		resp, err := session.Chat(ctx, input)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}

		fmt.Println(resp.Message)

		if resp.WorkflowYAML != "" {
			fmt.Println("\nCurrent workflow:")
			fmt.Println("```yaml")
			fmt.Println(resp.WorkflowYAML)
			fmt.Println("```")
		}

		if resp.IsComplete {
			fmt.Println("\nWorkflow is complete!")
		}
	}

	return nil
}

// runWorkflowExplain explains a workflow YAML file.
func runWorkflowExplain(args []string) error {
	var (
		input      = ""
		audience   = ""
		jsonOutput = false
		diagram    = false
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--audience":
			if i+1 >= len(args) {
				return fmt.Errorf("--audience requires a value")
			}
			i++
			audience = args[i]
		case "--json":
			jsonOutput = true
		case "--diagram":
			diagram = true
		case "--help", "-h":
			printWorkflowExplainUsage()
			return nil
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
			if input == "" {
				input = args[i]
			}
		}
	}

	if input == "" {
		return fmt.Errorf("workflow file path required")
	}

	// Read workflow file
	var data []byte
	var err error
	if input == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(input)
	}
	if err != nil {
		return fmt.Errorf("read workflow: %w", err)
	}

	// Create LLM client
	llmClient, err := llm.NewWithDefaults()
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	// Create explainer
	explainer := explain.NewWorkflowExplainer(llmClient, "")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var result *explain.WorkflowExplanation
	if audience != "" {
		result, err = explainer.ExplainForAudience(ctx, string(data), audience)
	} else {
		result, err = explainer.Explain(ctx, string(data))
	}
	if err != nil {
		return fmt.Errorf("explain workflow: %w", err)
	}

	// Optionally add diagram
	if diagram && result.Diagram == "" {
		diagramStr, err := explainer.GenerateDiagram(ctx, string(data))
		if err == nil {
			result.Diagram = diagramStr
		}
	}

	if jsonOutput {
		output, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(output))
	} else {
		fmt.Println("# Workflow Explanation")
		fmt.Println()
		fmt.Printf("## Summary\n\n%s\n\n", result.Summary)
		fmt.Printf("## Description\n\n%s\n\n", result.Description)

		if len(result.RouteExplanations) > 0 {
			fmt.Println("## Routes")
			for _, route := range result.RouteExplanations {
				fmt.Printf("\n### %s\n\n", route.Name)
				fmt.Printf("**Trigger:** %s\n\n", route.Trigger)
				fmt.Printf("%s\n\n", route.Description)
				if len(route.Actions) > 0 {
					fmt.Println("**Actions:**")
					for _, action := range route.Actions {
						fmt.Printf("- %s\n", action)
					}
				}
			}
		}

		if result.Diagram != "" {
			fmt.Println("\n## Diagram")
			fmt.Println("```mermaid")
			fmt.Println(result.Diagram)
			fmt.Println("```")
		}

		if len(result.Warnings) > 0 {
			fmt.Println("\n## Warnings")
			for _, w := range result.Warnings {
				fmt.Printf("- %s\n", w)
			}
		}
	}

	return nil
}

// runWorkflowCEL generates a CEL expression from natural language.
func runWorkflowCEL(args []string) error {
	var (
		description = ""
		jsonOutput  = false
		validate    = ""
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--validate":
			if i+1 >= len(args) {
				return fmt.Errorf("--validate requires a CEL expression")
			}
			i++
			validate = args[i]
		case "--help", "-h":
			printWorkflowCELUsage()
			return nil
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
			if description == "" {
				description = args[i]
			}
		}
	}

	if description == "" && validate == "" {
		return fmt.Errorf("description required (or use --validate <expression>)")
	}

	// Create LLM client
	llmClient, err := llm.NewWithDefaults()
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	// Create CEL assistant
	assistant := copilot.NewCELAssistant(llmClient, "")

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	if validate != "" {
		// Validate mode
		result, err := assistant.ValidateAndCorrect(ctx, validate)
		if err != nil {
			return fmt.Errorf("validate CEL: %w", err)
		}

		if jsonOutput {
			output, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(output))
		} else {
			if result.Valid {
				fmt.Println("Expression is valid.")
			} else {
				fmt.Printf("Expression is invalid: %s\n", result.Error)
			}
			if len(result.Suggestions) > 0 {
				fmt.Println("\nSuggestions:")
				for _, s := range result.Suggestions {
					fmt.Printf("- %s\n", s)
				}
			}
			if result.Corrected != "" {
				fmt.Printf("\nCorrected expression:\n  %s\n", result.Corrected)
			}
		}
		return nil
	}

	// Generate mode
	result, err := assistant.Generate(ctx, copilot.GenerateCELRequest{
		Description: description,
	})
	if err != nil {
		return fmt.Errorf("generate CEL: %w", err)
	}

	if jsonOutput {
		output, _ := json.MarshalIndent(result, "", "  ")
		fmt.Println(string(output))
	} else {
		fmt.Printf("Expression: %s\n\n", result.Expression)
		fmt.Printf("Explanation: %s\n", result.Explanation)
		if len(result.FieldsUsed) > 0 {
			fmt.Println("\nFields used:")
			for _, f := range result.FieldsUsed {
				fmt.Printf("  - %s\n", f)
			}
		}
		if len(result.Alternatives) > 0 {
			fmt.Println("\nAlternatives:")
			for _, alt := range result.Alternatives {
				fmt.Printf("  - %s\n", alt)
			}
		}
	}

	return nil
}

// runTerminologySearch performs semantic search across terminology indexes.
func runTerminologySearch(args []string) error {
	var (
		query      = ""
		vocabulary = ""
		limit      = 10
		minScore   = 0.0
		jsonOutput = false
		qdrantURL  = ""
		embedURL   = ""
		embedModel = ""
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--query", "-q":
			if i+1 >= len(args) {
				return fmt.Errorf("--query requires a value")
			}
			i++
			query = args[i]
		case "--vocabulary", "-v":
			if i+1 >= len(args) {
				return fmt.Errorf("--vocabulary requires a value")
			}
			i++
			vocabulary = args[i]
		case "--limit", "-l":
			if i+1 >= len(args) {
				return fmt.Errorf("--limit requires a value")
			}
			i++
			fmt.Sscanf(args[i], "%d", &limit)
		case "--min-score":
			if i+1 >= len(args) {
				return fmt.Errorf("--min-score requires a value")
			}
			i++
			fmt.Sscanf(args[i], "%f", &minScore)
		case "--json":
			jsonOutput = true
		case "--qdrant-url":
			if i+1 >= len(args) {
				return fmt.Errorf("--qdrant-url requires a value")
			}
			i++
			qdrantURL = args[i]
		case "--embedding-url":
			if i+1 >= len(args) {
				return fmt.Errorf("--embedding-url requires a value")
			}
			i++
			embedURL = args[i]
		case "--embedding-model":
			if i+1 >= len(args) {
				return fmt.Errorf("--embedding-model requires a value")
			}
			i++
			embedModel = args[i]
		case "--help", "-h":
			printTerminologySearchUsage()
			return nil
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
			if query == "" {
				query = args[i]
			}
		}
	}

	if query == "" {
		return fmt.Errorf("query required (use --query or positional argument)")
	}

	// Get configuration from environment if not provided
	if qdrantURL == "" {
		qdrantURL = os.Getenv("FI_FHIR_QDRANT_URL")
		if qdrantURL == "" {
			qdrantURL = "http://localhost:6333"
		}
	}
	if embedURL == "" {
		embedURL = os.Getenv("LLM_BASE_URL")
		if embedURL == "" {
			embedURL = "http://localhost:8000/v1"
		}
	}
	if embedModel == "" {
		embedModel = os.Getenv("LLM_EMBEDDING_MODEL")
		if embedModel == "" {
			embedModel = "bge-large-embeddings"
		}
	}

	// Create searcher
	searcher, err := semantic.NewSearcher(semantic.SearchConfig{
		QdrantURL:           qdrantURL,
		QdrantAPIKey:        os.Getenv("FI_FHIR_QDRANT_API_KEY"),
		EmbeddingBaseURL:    embedURL,
		EmbeddingAPIKey:     os.Getenv("LLM_API_KEY"),
		EmbeddingModel:      embedModel,
		EmbeddingDimensions: 1024,
		Timeout:             30 * time.Second,
		DefaultMaxResults:   limit,
		DefaultMinScore:     minScore,
		EnableCache:         true,
	})
	if err != nil {
		return fmt.Errorf("create searcher: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Determine vocabularies to search
	var vocabs []index.Vocabulary
	switch vocabulary {
	case "", "all":
		vocabs = []index.Vocabulary{index.VocabularyLOINC, index.VocabularySNOMED, index.VocabularyICD10CM}
	case "loinc":
		vocabs = []index.Vocabulary{index.VocabularyLOINC}
	case "snomed":
		vocabs = []index.Vocabulary{index.VocabularySNOMED}
	case "icd10", "icd-10", "icd10cm":
		vocabs = []index.Vocabulary{index.VocabularyICD10CM}
	case "rxnorm":
		vocabs = []index.Vocabulary{index.VocabularyRxNorm}
	default:
		vocabs = []index.Vocabulary{index.Vocabulary(vocabulary)}
	}

	// Perform search
	results, err := searcher.Search(ctx, query, semantic.SearchOptions{
		Vocabularies: vocabs,
		MaxResults:   limit,
		MinScore:     minScore,
	})
	if err != nil {
		return fmt.Errorf("search: %w", err)
	}

	if jsonOutput {
		output, _ := json.MarshalIndent(results, "", "  ")
		fmt.Println(string(output))
	} else {
		if len(results) == 0 {
			fmt.Println("No results found.")
			return nil
		}

		fmt.Printf("Found %d results for '%s':\n\n", len(results), query)
		for i, match := range results {
			fmt.Printf("%d. [%s] %s\n", i+1, match.Vocabulary, match.Display)
			fmt.Printf("   Code: %s | System: %s | Score: %.3f\n", match.Code, match.System, match.Score)
			fmt.Println()
		}
	}

	return nil
}

func printWorkflowGenerateUsage() {
	fmt.Println(`fi-fhir workflow generate - Generate workflow from natural language

Usage:
  fi-fhir workflow generate [options] "<description>"

Options:
  -i, --interactive  Start interactive workflow builder session
  --json             Output as JSON instead of markdown
  -h, --help         Show this help message

Examples:
  # Generate workflow from description
  fi-fhir workflow generate "Route critical lab results to the pager system"

  # Interactive mode
  fi-fhir workflow generate --interactive

  # JSON output
  fi-fhir workflow generate --json "Send ADT events to FHIR server"`)
}

func printWorkflowExplainUsage() {
	fmt.Println(`fi-fhir workflow explain - Explain a workflow in plain English

Usage:
  fi-fhir workflow explain [options] <workflow.yaml>

Options:
  --audience <type>  Target audience: technical, business, compliance, operations
  --diagram          Include Mermaid flow diagram
  --json             Output as JSON instead of markdown
  -h, --help         Show this help message

Examples:
  # Explain a workflow
  fi-fhir workflow explain workflow.yaml

  # Explain for business analysts
  fi-fhir workflow explain --audience business workflow.yaml

  # Include diagram
  fi-fhir workflow explain --diagram workflow.yaml

  # Read from stdin
  cat workflow.yaml | fi-fhir workflow explain -`)
}

func printWorkflowCELUsage() {
	fmt.Println(`fi-fhir workflow cel - Generate or validate CEL expressions

Usage:
  fi-fhir workflow cel [options] "<description>"
  fi-fhir workflow cel --validate "<expression>"

Options:
  --validate <expr>  Validate and suggest corrections for an expression
  --json             Output as JSON
  -h, --help         Show this help message

Examples:
  # Generate CEL from description
  fi-fhir workflow cel "patient over 65 with abnormal lab results"

  # Validate an expression
  fi-fhir workflow cel --validate "event.patient.age >= 65"

  # JSON output
  fi-fhir workflow cel --json "critical observation for adult patient"`)
}

func printTerminologySearchUsage() {
	fmt.Println(`fi-fhir terminology search - Semantic search across terminology indexes

Usage:
  fi-fhir terminology search [options] "<query>"

Options:
  -q, --query <text>       Search query (or positional argument)
  -v, --vocabulary <name>  Vocabulary to search: loinc, snomed, icd10, rxnorm, all (default: all)
  -l, --limit <n>          Maximum results (default: 10)
  --min-score <float>      Minimum similarity score (default: 0.0)
  --json                   Output as JSON
  --qdrant-url <url>       Qdrant server URL (or FI_FHIR_QDRANT_URL env)
  --embedding-url <url>    Embedding API URL (or LLM_BASE_URL env)
  --embedding-model <name> Embedding model name (or LLM_EMBEDDING_MODEL env)
  -h, --help               Show this help message

Examples:
  # Search all vocabularies
  fi-fhir terminology search "blood glucose"

  # Search specific vocabulary
  fi-fhir terminology search --vocabulary loinc "hemoglobin A1c"

  # JSON output with custom limit
  fi-fhir terminology search --json --limit 20 "diabetes mellitus"

Environment Variables:
  FI_FHIR_QDRANT_URL      Qdrant server URL (default: http://localhost:6333)
  FI_FHIR_QDRANT_API_KEY  Qdrant API key
  LLM_BASE_URL            LLM/Embedding API base URL
  LLM_API_KEY             LLM/Embedding API key
  LLM_EMBEDDING_MODEL     Embedding model name`)
}
