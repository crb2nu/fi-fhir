// Package autoroute provides LLM-powered terminology mapping suggestions.
//
// The autoroute engine combines semantic search (vector embeddings) with
// LLM-based ranking to suggest mappings between source codes and standard
// terminologies like LOINC, SNOMED CT, and ICD-10.
package autoroute

import (
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

// SuggestRequest represents a request to suggest mappings for a source code.
type SuggestRequest struct {
	SourceCode    string // The code to map (e.g., "LAB001")
	SourceSystem  string // The source system URI (e.g., "epic_custom_labs")
	SourceDisplay string // Human-readable description (helps semantic matching)
	TargetSystem  string // Target vocabulary URI (e.g., "http://loinc.org")
	ProfileID     string // Optional: scope suggestions to a profile
	MaxCandidates int    // Maximum candidates to return (default: 5)
}

// SuggestResult contains the autoroute suggestion with full decision trace.
type SuggestResult struct {
	// Best match
	BestMatch *Candidate

	// Alternative candidates considered
	Alternates []Candidate

	// Decision metadata
	Confidence float64 // Overall confidence (0.0-1.0)
	Reasoning  string  // LLM's reasoning for the decision
	Model      string  // LLM model used

	// Timing
	SearchDuration time.Duration // Time spent on semantic search
	RankDuration   time.Duration // Time spent on LLM ranking
	TotalDuration  time.Duration // Total processing time

	// Decision trace for auditability
	Trace *DecisionTrace
}

// Candidate represents a potential mapping target.
type Candidate struct {
	Code        string                // Target code (e.g., "2345-7")
	Display     string                // Human-readable name
	System      string                // Code system URI
	Confidence  float64               // Confidence score (0.0-1.0)
	Equivalence db.MappingEquivalence // Semantic equivalence type
	Reasoning   string                // Why this candidate was selected/ranked
	Score       float64               // Raw semantic search score (for debugging)
}

// DecisionTrace captures the full decision-making process for auditability.
type DecisionTrace struct {
	TraceID   string         `json:"trace_id"`
	Timestamp time.Time      `json:"timestamp"`
	Request   TraceRequest   `json:"request"`
	Steps     []DecisionStep `json:"steps"`
	Result    *TraceResult   `json:"result,omitempty"`
	Duration  Duration       `json:"duration"`
}

// TraceRequest records the original request parameters.
type TraceRequest struct {
	SourceCode    string `json:"source_code"`
	SourceSystem  string `json:"source_system"`
	SourceDisplay string `json:"source_display,omitempty"`
	TargetSystem  string `json:"target_system"`
	ProfileID     string `json:"profile_id,omitempty"`
}

// DecisionStep represents one step in the decision-making process.
type DecisionStep struct {
	Step       string                 `json:"step"`   // Step name (e.g., "semantic_search", "llm_ranking")
	Result     string                 `json:"result"` // Step outcome (e.g., "found_5_candidates", "selected_2345-7")
	DurationMs int64                  `json:"duration_ms"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"` // Step-specific data
}

// TraceResult records the final decision.
type TraceResult struct {
	Code        string  `json:"code"`
	Display     string  `json:"display"`
	System      string  `json:"system"`
	Confidence  float64 `json:"confidence"`
	Equivalence string  `json:"equivalence"`
}

// Duration holds timing information.
type Duration struct {
	SearchMs int64 `json:"search_ms"`
	RankMs   int64 `json:"rank_ms"`
	TotalMs  int64 `json:"total_ms"`
}

// DecisionType categorizes the autoroute decision outcome.
type DecisionType string

const (
	// DecisionHighConfidence indicates a high-confidence suggestion (≥0.90).
	DecisionHighConfidence DecisionType = "AUTOROUTE_HIGH_CONF"

	// DecisionMediumConfidence indicates medium confidence (≥0.70, <0.90).
	DecisionMediumConfidence DecisionType = "AUTOROUTE_MED_CONF"

	// DecisionLowConfidence indicates low confidence (≥0.50, <0.70).
	DecisionLowConfidence DecisionType = "AUTOROUTE_LOW_CONF"

	// DecisionNoMatch indicates no suitable mapping was found.
	DecisionNoMatch DecisionType = "NO_MATCH"
)

// Classify returns the decision type based on confidence score.
func (r *SuggestResult) Classify(highThreshold, medThreshold float64) DecisionType {
	if r.BestMatch == nil || r.Confidence < 0.5 {
		return DecisionNoMatch
	}
	if r.Confidence >= highThreshold {
		return DecisionHighConfidence
	}
	if r.Confidence >= medThreshold {
		return DecisionMediumConfidence
	}
	return DecisionLowConfidence
}

// ToCodeMapping converts the best match to a CustomMapping for persistence.
func (r *SuggestResult) ToCodeMapping(req SuggestRequest, createdBy string) *db.CustomMapping {
	if r.BestMatch == nil {
		return nil
	}

	conf := r.Confidence
	return &db.CustomMapping{
		SourceSystem:  req.SourceSystem,
		SourceCode:    req.SourceCode,
		SourceDisplay: req.SourceDisplay,
		TargetSystem:  r.BestMatch.System,
		TargetCode:    r.BestMatch.Code,
		TargetDisplay: r.BestMatch.Display,
		Equivalence:   r.BestMatch.Equivalence,
		Confidence:    &conf,
		Origin:        db.OriginApprovedAutoroute,
		ProfileID:     req.ProfileID,
		CreatedBy:     createdBy,
	}
}
