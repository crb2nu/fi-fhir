package autoroute

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm/prompts"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/semantic"
)

// Ranker uses LLM to rank and evaluate semantic search candidates.
type Ranker struct {
	client   llm.Client
	model    string
	registry *prompts.Registry // optional prompt registry
}

// RankerConfig configures the LLM ranker.
type RankerConfig struct {
	Model       string  // LLM model to use (default: from client config)
	Temperature float64 // Temperature for LLM (default: 0.1 for deterministic)
}

// NewRanker creates a new LLM ranker with hardcoded prompts.
func NewRanker(client llm.Client, cfg RankerConfig) *Ranker {
	return &Ranker{
		client: client,
		model:  cfg.Model,
	}
}

// NewRankerWithRegistry creates a ranker that uses the prompt registry for versioned prompts.
func NewRankerWithRegistry(client llm.Client, cfg RankerConfig, reg *prompts.Registry) *Ranker {
	return &Ranker{
		client:   client,
		model:    cfg.Model,
		registry: reg,
	}
}

// RankRequest contains the input for ranking candidates.
type RankRequest struct {
	SourceCode    string
	SourceDisplay string
	SourceSystem  string
	TargetSystem  string
	Candidates    []semantic.SemanticMatch
	MaxResults    int
}

// RankResult contains the ranked candidates with LLM reasoning.
type RankResult struct {
	Candidates    []Candidate
	Reasoning     string
	TopConfidence float64
	Model         string
	Duration      time.Duration
}

// rankingOutput is the structured JSON schema for LLM output.
type rankingOutput struct {
	BestMatch struct {
		Code        string  `json:"code"`
		Confidence  float64 `json:"confidence"`
		Equivalence string  `json:"equivalence"`
		Reasoning   string  `json:"reasoning"`
	} `json:"best_match"`
	Alternates []struct {
		Code       string  `json:"code"`
		Confidence float64 `json:"confidence"`
		Reasoning  string  `json:"reasoning"`
	} `json:"alternates"`
	OverallReasoning string `json:"overall_reasoning"`
}

// rankingOutputSchema is the JSON schema for structured output.
var rankingOutputSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"best_match": map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"code":        map[string]interface{}{"type": "string", "description": "The best matching code"},
				"confidence":  map[string]interface{}{"type": "number", "minimum": 0, "maximum": 1, "description": "Confidence score 0.0-1.0"},
				"equivalence": map[string]interface{}{"type": "string", "enum": []string{"equivalent", "wider", "narrower", "inexact"}, "description": "Semantic relationship"},
				"reasoning":   map[string]interface{}{"type": "string", "description": "Why this is the best match"},
			},
			"required": []string{"code", "confidence", "equivalence", "reasoning"},
		},
		"alternates": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"code":       map[string]interface{}{"type": "string"},
					"confidence": map[string]interface{}{"type": "number", "minimum": 0, "maximum": 1},
					"reasoning":  map[string]interface{}{"type": "string"},
				},
				"required": []string{"code", "confidence", "reasoning"},
			},
		},
		"overall_reasoning": map[string]interface{}{"type": "string", "description": "Summary of the decision process"},
	},
	"required": []string{"best_match", "overall_reasoning"},
}

const rankingSystemPrompt = `You are a healthcare terminology expert specializing in medical code mapping.

Your task is to evaluate candidate mappings between a source code and target vocabulary codes.

Consider these factors when evaluating:
1. **Semantic equivalence** - Does the meaning match exactly, or is it broader/narrower?
2. **Specificity** - Is the target too general or too specific for the source?
3. **Clinical context** - Would this mapping be appropriate in clinical workflows?
4. **Standard practices** - Is this a commonly accepted mapping in healthcare IT?

Confidence score guidelines:
- 0.95-1.0: Exact semantic match, very high certainty
- 0.85-0.94: Strong match, minor differences in specificity
- 0.70-0.84: Good match, some nuance differences
- 0.50-0.69: Partial match, may need human review
- Below 0.50: Weak match, likely incorrect

Equivalence types:
- "equivalent": The codes represent the same concept
- "wider": The target code covers a broader scope than the source
- "narrower": The target code is more specific than the source
- "inexact": Approximate match with meaningful differences`

// Rank evaluates candidates and returns ranked results with reasoning.
func (r *Ranker) Rank(ctx context.Context, req RankRequest) (*RankResult, error) {
	if len(req.Candidates) == 0 {
		return &RankResult{
			Candidates:    nil,
			Reasoning:     "No candidates to evaluate",
			TopConfidence: 0,
		}, nil
	}

	start := time.Now()

	// Resolve system prompt: use registry if available, fallback to hardcoded
	systemPrompt := rankingSystemPrompt
	if r.registry != nil {
		if p, err := r.registry.Get(prompts.RankingSystemV1); err == nil {
			if rendered, err := p.Render(nil); err == nil {
				systemPrompt = rendered
			}
		}
	}

	// Build the user prompt: use registry template if available
	var prompt string
	if r.registry != nil {
		if p, err := r.registry.Get(prompts.RankingUserV1); err == nil {
			data := map[string]interface{}{
				"SourceCode":    req.SourceCode,
				"SourceDisplay": req.SourceDisplay,
				"SourceSystem":  req.SourceSystem,
				"TargetSystem":  req.TargetSystem,
				"Candidates":    req.Candidates,
			}
			if rendered, err := p.Render(data); err == nil {
				prompt = rendered
			}
		}
	}
	if prompt == "" {
		prompt = buildRankingPrompt(req)
	}

	// Request structured output from LLM
	model := r.model
	if model == "" {
		model = "" // Use client default
	}

	llmReq := llm.CompletionRequest{
		Messages: []llm.Message{
			llm.SystemMessage(systemPrompt),
			llm.UserMessage(prompt),
		},
		Model:       model,
		Temperature: 0.1, // Low temperature for consistent rankings
	}

	// Use co-located schema from registry if available, else inline schema
	var schema interface{} = rankingOutputSchema
	if r.registry != nil {
		if p, err := r.registry.Get(prompts.RankingUserV1); err == nil && p.HasSchema() {
			var registrySchema interface{}
			if json.Unmarshal(p.Schema, &registrySchema) == nil {
				schema = registrySchema
			}
		}
	}

	jsonResp, err := r.client.CompleteStructured(ctx, llmReq, "terminology_ranking", schema)
	if err != nil {
		return nil, fmt.Errorf("LLM ranking failed: %w", err)
	}

	var output rankingOutput
	if err := json.Unmarshal(jsonResp, &output); err != nil {
		return nil, fmt.Errorf("failed to parse ranking output: %w", err)
	}

	// Build result
	candidates := make([]Candidate, 0, len(output.Alternates)+1)

	// Find the semantic match for best match to get display
	bestMatchDisplay := findDisplay(req.Candidates, output.BestMatch.Code)

	// Add best match
	candidates = append(candidates, Candidate{
		Code:        output.BestMatch.Code,
		Display:     bestMatchDisplay,
		System:      req.TargetSystem,
		Confidence:  output.BestMatch.Confidence,
		Equivalence: parseEquivalence(output.BestMatch.Equivalence),
		Reasoning:   output.BestMatch.Reasoning,
	})

	// Add alternates
	for _, alt := range output.Alternates {
		if req.MaxResults > 0 && len(candidates) >= req.MaxResults {
			break
		}
		altDisplay := findDisplay(req.Candidates, alt.Code)
		candidates = append(candidates, Candidate{
			Code:       alt.Code,
			Display:    altDisplay,
			System:     req.TargetSystem,
			Confidence: alt.Confidence,
			Reasoning:  alt.Reasoning,
		})
	}

	return &RankResult{
		Candidates:    candidates,
		Reasoning:     output.OverallReasoning,
		TopConfidence: output.BestMatch.Confidence,
		Model:         model,
		Duration:      time.Since(start),
	}, nil
}

// buildRankingPrompt creates the user prompt for ranking.
func buildRankingPrompt(req RankRequest) string {
	var sb strings.Builder

	sb.WriteString("## Source Code to Map\n")
	sb.WriteString(fmt.Sprintf("- **Code**: `%s`\n", req.SourceCode))
	if req.SourceDisplay != "" {
		sb.WriteString(fmt.Sprintf("- **Display**: %s\n", req.SourceDisplay))
	}
	sb.WriteString(fmt.Sprintf("- **Source System**: %s\n", req.SourceSystem))
	sb.WriteString(fmt.Sprintf("- **Target System**: %s\n\n", req.TargetSystem))

	sb.WriteString("## Candidate Matches\n\n")
	for i, c := range req.Candidates {
		sb.WriteString(fmt.Sprintf("%d. **%s** - %s (similarity: %.2f)\n",
			i+1, c.Code, c.Display, c.Score))
	}

	sb.WriteString("\n## Task\n")
	sb.WriteString("Evaluate these candidates and select the best mapping. ")
	sb.WriteString("Consider semantic equivalence, clinical appropriateness, and specificity. ")
	sb.WriteString("Provide confidence scores and reasoning for your decision.")

	return sb.String()
}

// findDisplay looks up the display name for a code from candidates.
func findDisplay(candidates []semantic.SemanticMatch, code string) string {
	for _, c := range candidates {
		if c.Code == code {
			return c.Display
		}
	}
	return ""
}

// parseEquivalence converts string to MappingEquivalence.
func parseEquivalence(s string) db.MappingEquivalence {
	switch strings.ToLower(s) {
	case "equivalent":
		return db.EquivalenceEquivalent
	case "wider":
		return db.EquivalenceWider
	case "narrower":
		return db.EquivalenceNarrower
	case "inexact":
		return db.EquivalenceInexact
	default:
		return db.EquivalenceInexact
	}
}
