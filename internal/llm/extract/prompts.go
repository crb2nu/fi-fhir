package extract

import (
	"bytes"
	"encoding/json"
	"strings"
	"text/template"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm/prompts"
)

// promptTemplates contains the prompt templates for entity extraction.
var promptTemplates = struct {
	system     *template.Template
	extraction *template.Template
}{}

func init() {
	promptTemplates.system = template.Must(template.New("system").Parse(systemPromptTemplate))
	promptTemplates.extraction = template.Must(template.New("extraction").Parse(extractionPromptTemplate))
}

const systemPromptTemplate = `You are a clinical entity extraction system specialized in healthcare data processing.

Your task is to extract structured clinical entities from medical documents. Follow these guidelines:

1. ACCURACY: Extract only entities explicitly mentioned in the text. Do not infer or assume.

2. CODING SYSTEMS:
   - For conditions: Use SNOMED CT (preferred) or ICD-10-CM codes
   - For medications: Use RxNorm codes
   - For vital signs: Use LOINC codes
   - For procedures: Use CPT or SNOMED CT codes
   - For allergies: Use RxNorm (medications) or SNOMED CT (substances)

3. NEGATION: Mark entities as negated if they are explicitly denied in the text.
   Examples: "no diabetes", "denies chest pain", "patient does not have hypertension"

4. CONFIDENCE SCORES: Assign confidence scores from 0.0 to 1.0:
   - 1.0: Explicitly stated with clear terminology
   - 0.9: Clearly stated but using lay terms
   - 0.8: Strongly implied with specific details
   - 0.7: Mentioned but with some ambiguity
   - Below 0.7: Uncertain or vague mentions

5. CONTEXT: Consider the document type when extracting:
   - Progress notes: Focus on current findings and assessments
   - Discharge summaries: Include final diagnoses and discharge medications
   - Consult notes: Focus on the consultation findings and recommendations

6. OUTPUT FORMAT: Return valid JSON matching the requested schema exactly.`

const extractionPromptTemplate = `Extract clinical entities from this {{.DocumentType}} note.

{{if .PatientContext}}Patient: {{.PatientContext}}
{{end}}
---
CLINICAL TEXT:
{{.ClinicalText}}
---

Extract the following entity types:
{{if .ExtractConditions}}- Conditions/Diagnoses (use SNOMED CT or ICD-10-CM codes)
{{end}}{{if .ExtractMedications}}- Medications (use RxNorm codes)
{{end}}{{if .ExtractVitalSigns}}- Vital Signs (use LOINC codes)
{{end}}{{if .ExtractAllergies}}- Allergies/Intolerances (use RxNorm or SNOMED CT codes)
{{end}}{{if .ExtractProcedures}}- Procedures (use CPT or SNOMED CT codes)
{{end}}{{if .ExtractSocialHistory}}- Social History (smoking status, alcohol, substance use)
{{end}}
For each entity, include:
- The standardized code and code system
- A display name
- Confidence score (0.0-1.0)
- Whether the entity is negated
- The text span where it was found

Return ONLY valid JSON.`

// extractionPromptData holds data for rendering the extraction prompt.
type extractionPromptData struct {
	DocumentType         string
	PatientContext       string
	ClinicalText         string
	ExtractConditions    bool
	ExtractMedications   bool
	ExtractVitalSigns    bool
	ExtractAllergies     bool
	ExtractProcedures    bool
	ExtractSocialHistory bool
}

// buildSystemPrompt generates the system prompt for extraction.
func buildSystemPrompt(reg *prompts.Registry) string {
	if reg != nil {
		if p, err := reg.Get(prompts.ExtractionSystemV1); err == nil {
			if rendered, err := p.Render(nil); err == nil {
				return rendered
			}
		}
	}

	var buf bytes.Buffer
	_ = promptTemplates.system.Execute(&buf, nil)
	return buf.String()
}

// buildExtractionPrompt generates the user prompt for extraction.
func buildExtractionPrompt(text string, opts ExtractionOptions, reg *prompts.Registry) string {
	data := extractionPromptData{
		DocumentType:         opts.DocumentType,
		ClinicalText:         truncateText(text, 12000), // Limit text length
		ExtractConditions:    opts.ExtractConditions,
		ExtractMedications:   opts.ExtractMedications,
		ExtractVitalSigns:    opts.ExtractVitalSigns,
		ExtractAllergies:     opts.ExtractAllergies,
		ExtractProcedures:    opts.ExtractProcedures,
		ExtractSocialHistory: opts.ExtractSocialHistory,
	}

	// Build patient context
	var context []string
	if opts.PatientAge > 0 {
		context = append(context, intToString(opts.PatientAge)+" year old")
	}
	if opts.PatientGender != "" {
		context = append(context, opts.PatientGender)
	}
	if len(context) > 0 {
		data.PatientContext = strings.Join(context, " ")
	}

	// Default document type
	if data.DocumentType == "" {
		data.DocumentType = "clinical"
	}

	if reg != nil {
		if p, err := reg.Get(prompts.ExtractionUserV1); err == nil {
			if rendered, err := p.Render(data); err == nil {
				return rendered
			}
		}
	}

	var buf bytes.Buffer
	_ = promptTemplates.extraction.Execute(&buf, data)
	return buf.String()
}

// truncateText truncates text to a maximum length, preserving word boundaries.
func truncateText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}

	// Find a good break point
	truncated := text[:maxLen]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > 0 && lastSpace > maxLen-100 {
		truncated = truncated[:lastSpace]
	}

	return truncated + "...[truncated]"
}

// intToString converts an int to string without importing strconv.
func intToString(n int) string {
	if n == 0 {
		return "0"
	}

	var digits []byte
	negative := n < 0
	if negative {
		n = -n
	}

	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}

	if negative {
		return "-" + string(digits)
	}
	return string(digits)
}

// getExtractionSchema returns the extraction JSON schema. It falls back to the embedded inline map.
func getExtractionSchema(reg *prompts.Registry) interface{} {
	var schema interface{} = extractionSchema
	if reg != nil {
		if p, err := reg.Get(prompts.ExtractionUserV1); err == nil && p.HasSchema() {
			var registrySchema interface{}
			if json.Unmarshal(p.Schema, &registrySchema) == nil {
				return registrySchema
			}
		}
	}
	return schema
}

// extractionSchema defines the JSON schema for structured extraction output.
var extractionSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"conditions": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":        map[string]interface{}{"type": "string"},
					"code":        map[string]interface{}{"type": "string"},
					"code_system": map[string]interface{}{"type": "string"},
					"status":      map[string]interface{}{"type": "string"},
					"confidence":  map[string]interface{}{"type": "number"},
					"negated":     map[string]interface{}{"type": "boolean"},
					"text_span":   map[string]interface{}{"type": "string"},
					"onset_date":  map[string]interface{}{"type": "string"},
					"severity":    map[string]interface{}{"type": "string"},
				},
				"required": []string{"name", "confidence"},
			},
		},
		"medications": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":        map[string]interface{}{"type": "string"},
					"code":        map[string]interface{}{"type": "string"},
					"code_system": map[string]interface{}{"type": "string"},
					"dose":        map[string]interface{}{"type": "string"},
					"route":       map[string]interface{}{"type": "string"},
					"frequency":   map[string]interface{}{"type": "string"},
					"confidence":  map[string]interface{}{"type": "number"},
					"negated":     map[string]interface{}{"type": "boolean"},
					"text_span":   map[string]interface{}{"type": "string"},
				},
				"required": []string{"name", "confidence"},
			},
		},
		"vital_signs": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":       map[string]interface{}{"type": "string"},
					"loinc_code": map[string]interface{}{"type": "string"},
					"value":      map[string]interface{}{"type": "string"},
					"unit":       map[string]interface{}{"type": "string"},
					"confidence": map[string]interface{}{"type": "number"},
					"text_span":  map[string]interface{}{"type": "string"},
				},
				"required": []string{"name", "value", "confidence"},
			},
		},
		"allergies": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":        map[string]interface{}{"type": "string"},
					"code":        map[string]interface{}{"type": "string"},
					"code_system": map[string]interface{}{"type": "string"},
					"type":        map[string]interface{}{"type": "string"},
					"severity":    map[string]interface{}{"type": "string"},
					"reaction":    map[string]interface{}{"type": "string"},
					"confidence":  map[string]interface{}{"type": "number"},
					"negated":     map[string]interface{}{"type": "boolean"},
					"text_span":   map[string]interface{}{"type": "string"},
				},
				"required": []string{"name", "confidence"},
			},
		},
		"procedures": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":        map[string]interface{}{"type": "string"},
					"code":        map[string]interface{}{"type": "string"},
					"code_system": map[string]interface{}{"type": "string"},
					"status":      map[string]interface{}{"type": "string"},
					"date":        map[string]interface{}{"type": "string"},
					"confidence":  map[string]interface{}{"type": "number"},
					"negated":     map[string]interface{}{"type": "boolean"},
					"text_span":   map[string]interface{}{"type": "string"},
				},
				"required": []string{"name", "confidence"},
			},
		},
		"social_history": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":        map[string]interface{}{"type": "string"},
					"code":        map[string]interface{}{"type": "string"},
					"code_system": map[string]interface{}{"type": "string"},
					"value":       map[string]interface{}{"type": "string"},
					"confidence":  map[string]interface{}{"type": "number"},
					"text_span":   map[string]interface{}{"type": "string"},
				},
				"required": []string{"name", "confidence"},
			},
		},
		"overall_confidence": map[string]interface{}{"type": "number"},
	},
	"required": []string{"overall_confidence"},
}
