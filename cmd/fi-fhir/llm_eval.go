package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm/eval"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm/prompts"
)

func runLLMEval(args []string) error {
	var (
		taskType string
		promptID string
		model    string
		input    string
		jsonOut  bool
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--task-type", "-t":
			if i+1 >= len(args) {
				return fmt.Errorf("--task-type requires a value")
			}
			i++
			taskType = args[i]
		case "--prompt", "-p":
			if i+1 >= len(args) {
				return fmt.Errorf("--prompt requires a value")
			}
			i++
			promptID = args[i]
		case "--model", "-m":
			if i+1 >= len(args) {
				return fmt.Errorf("--model requires a value")
			}
			i++
			model = args[i]
		case "--input", "-i":
			if i+1 >= len(args) {
				return fmt.Errorf("--input requires a value")
			}
			i++
			input = args[i]
		case "--json":
			jsonOut = true
		case "--help", "-h":
			printLLMEvalUsage()
			return nil
		default:
			if len(args[i]) > 0 && args[i][0] == '-' {
				return fmt.Errorf("unknown flag: %s", args[i])
			}
		}
	}

	if taskType == "" && input == "" {
		return fmt.Errorf("either --task-type or --input is required")
	}

	if input == "" {
		// Use default testdata path if not explicitly provided
		input = filepath.Join("testdata", "llm", "eval", fmt.Sprintf("%s_cases.json", taskType))
	}

	data, err := os.ReadFile(input)
	if err != nil {
		return fmt.Errorf("failed to read test cases from %s: %w", input, err)
	}

	var cases []eval.EvalCase
	if err := json.Unmarshal(data, &cases); err != nil {
		return fmt.Errorf("failed to parse test cases: %w", err)
	}

	if len(cases) == 0 {
		return fmt.Errorf("no test cases found in %s", input)
	}

	llmClient, err := llmClientFactory()
	if err != nil {
		return fmt.Errorf("create LLM client: %w", err)
	}

	registry := prompts.Default()

	// Override system prompts if a registry prompt was requested
	if promptID != "" && registry != nil {
		promptTpl, err := registry.Get(prompts.PromptID(promptID))
		if err == nil && promptTpl != nil {
			systemText, err := promptTpl.Render(nil)
			if err == nil {
				for i := range cases {
					// Inject the template instead of the static test case system prompt
					cases[i].System = systemText
				}
			} else {
				fmt.Fprintf(os.Stderr, "Warning: failed to render prompt %s from registry: %v\n", promptID, err)
			}
		} else {
			fmt.Fprintf(os.Stderr, "Warning: failed to load prompt %s from registry: %v\n", promptID, err)
		}
	}

	cfg := eval.DefaultEvalConfig()
	if model != "" {
		cfg.Model = model
	}
	if promptID != "" {
		cfg.PromptVersion = promptID
	}

	evaluator := eval.NewEvaluator(llmClient, cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	summary := evaluator.RunAll(ctx, cases)

	if jsonOut {
		output, _ := json.MarshalIndent(summary, "", "  ")
		fmt.Println(string(output))
		if summary.ErrorCount > 0 || summary.FailCount > 0 {
			os.Exit(1)
		}
		return nil
	}

	fmt.Printf("\n# Evaluation Results: %s\n\n", taskType)
	fmt.Println(summary.FormatSummary())
	fmt.Println()

	if len(summary.Results) > 0 {
		fmt.Println("## Case Detail")
		for _, res := range summary.Results {
			icon := "✅"
			if !res.Passed {
				icon = "❌"
			}
			if res.Error != "" {
				icon = "🚨"
				fmt.Printf("%s [%s] Error: %s (%v)\n", icon, res.CaseID, res.Error, res.Latency.Round(time.Millisecond))
			} else {
				fmt.Printf("%s [%s] Score: %.2f | Latency: %v | Tokens: %d\n", icon, res.CaseID, res.Score, res.Latency.Round(time.Millisecond), res.TokensUsed)
			}
		}
		fmt.Println()
	}

	if len(summary.ByTaskType) > 0 {
		fmt.Println("## By Task Type")
		for tt, ts := range summary.ByTaskType {
			fmt.Printf("- %s: %d cases, %.0f%% pass rate, %.2f mean score\n",
				tt, ts.Cases, float64(ts.PassCount)/float64(ts.Cases)*100, ts.MeanScore)
		}
		fmt.Println()
	}

	// Exit code 1 if there were failures or errors
	if summary.ErrorCount > 0 || summary.FailCount > 0 {
		os.Exit(1)
	}

	return nil
}

func printLLMEvalUsage() {
	fmt.Println(`fi-fhir llm eval - Evaluate LLM prompts and models against golden test cases

Usage:
  fi-fhir llm eval [options]

Options:
  -t, --task-type <type>  Type of task to evaluate (e.g., ranking, extraction)
  -i, --input <file>      Path to JSON test cases (defaults to testdata/llm/eval/<task-type>_cases.json)
  -p, --prompt <id>       Override system prompt with a template from the registry
  -m, --model <name>      Model to use for evaluation (overrides default)
  --json                  Output full results as JSON
  -h, --help              Show this help message

Examples:
  # Evaluate ranking prompts using default test cases
  fi-fhir llm eval --task-type=ranking

  # Evaluate a specific prompt version against a specific model
  fi-fhir llm eval --task-type=ranking --prompt=ranking_v1 --model=qwen3-8b`)
}
