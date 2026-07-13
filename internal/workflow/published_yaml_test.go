package workflow

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

const validPublishedWorkflowYAML = `dsl_version: "1"
name: adt-preview
version: "2026.07"
routes:
  - name: admit-primary
    filter:
      event_type:
        - patient_admit
      source: adt-east
      condition: 'event.patient.mrn == "MRN-1"'
    transform:
      - set_field: "encounter.status = planned"
      - explain_warnings:
          model: safe-model
          include_fix: "true"
          enable_cache: "false"
          cache_ttl: 5m
          warnings_field: warnings
    actions:
      - id: send-primary
        type: webhook
        destination: fhir-primary
      - id: audit-only
        type: log
`

func TestParsePublishedWorkflowPreservesSealedDefinition(t *testing.T) {
	raw := []byte(validPublishedWorkflowYAML)
	original := append([]byte(nil), raw...)

	published, err := ParsePublishedWorkflow(raw)
	if err != nil {
		t.Fatalf("ParsePublishedWorkflow() error = %v", err)
	}
	if got := published.DSLVersion(); got != "1" {
		t.Fatalf("DSLVersion() = %q, want 1", got)
	}
	if got := published.YAML(); !bytes.Equal(got, original) {
		t.Fatalf("YAML() changed exact bytes\ngot:  %q\nwant: %q", got, original)
	}

	// Neither the input nor either accessor may alias the sealed artifact.
	raw[0] = 'X'
	firstYAML := published.YAML()
	firstYAML[0] = 'X'
	if got := published.YAML(); !bytes.Equal(got, original) {
		t.Fatalf("YAML() returned aliased bytes: %q", got)
	}

	definition := published.Workflow()
	if definition.Name != "adt-preview" || len(definition.Routes) != 1 {
		t.Fatalf("Workflow() = %#v", definition)
	}
	if got := definition.Routes[0].Actions[0]; got.ID != "send-primary" || got.Type != "webhook" || got.Destination != "fhir-primary" {
		t.Fatalf("action metadata = %#v", got)
	}
	if got := definition.Routes[0].Actions[0].Config; len(got) != 0 {
		t.Fatalf("published action metadata leaked into Config: %#v", got)
	}

	definition.Name = "mutated"
	definition.Routes[0].Filter.EventType[0] = "mutated"
	definition.Routes[0].Actions[0].ID = "mutated"
	definition.Routes[0].Transforms[1].ExplainWarnings.Model = "mutated"
	again := published.Workflow()
	if again.Name != "adt-preview" || again.Routes[0].Filter.EventType[0] != "patient_admit" || again.Routes[0].Actions[0].ID != "send-primary" || again.Routes[0].Transforms[1].ExplainWarnings.Model != "safe-model" {
		t.Fatalf("Workflow() returned aliased definition: %#v", again)
	}
}

func TestParsePublishedWorkflowAcceptsDirectAndExactWrappedDocuments(t *testing.T) {
	tests := map[string]string{
		"direct with end marker": validPublishedWorkflowYAML + "... # one document\n# trailing comment\n",
		"wrapped":                "workflow:\n" + indentYAML(validPublishedWorkflowYAML, "  "),
		"quoted CEL bang":        strings.Replace(validPublishedWorkflowYAML, `event.patient.mrn == "MRN-1"`, `! event.patient.disabled`, 1),
		"plain CEL bang": strings.Replace(
			validPublishedWorkflowYAML,
			`      condition: 'event.patient.mrn == "MRN-1"'`,
			`      condition: event.patient.active && ! event.patient.disabled`,
			1,
		),
		"block CEL bang": strings.Replace(
			validPublishedWorkflowYAML,
			`      condition: 'event.patient.mrn == "MRN-1"'`,
			"      condition: >-\n        ! event.patient.disabled",
			1,
		),
		"quoted structural text": strings.Replace(
			validPublishedWorkflowYAML,
			`      condition: 'event.patient.mrn == "MRN-1"'`,
			"      condition: '"+strings.Repeat("[", maxPublishedWorkflowDepth+1)+"text'",
			1,
		),
		"block structural text": strings.Replace(
			validPublishedWorkflowYAML,
			`      condition: 'event.patient.mrn == "MRN-1"'`,
			"      condition: >-\n        "+strings.Repeat("[", maxPublishedWorkflowDepth+1)+"text",
			1,
		),
	}
	for name, document := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParsePublishedWorkflow([]byte(document)); err != nil {
				t.Fatalf("ParsePublishedWorkflow() error = %v", err)
			}
		})
	}
}

func TestParsePublishedWorkflowAcceptsDSLVersionOneActionTypes(t *testing.T) {
	for _, actionType := range []string{"log", "webhook", "fhir", "email", "exec", "file", "database", "queue", "event_store", "athena"} {
		t.Run(actionType, func(t *testing.T) {
			document := strings.Replace(validPublishedWorkflowYAML, "type: webhook", "type: "+actionType, 1)
			if _, err := ParsePublishedWorkflow([]byte(document)); err != nil {
				t.Fatalf("ParsePublishedWorkflow() rejected dsl_version 1 action type %q: %v", actionType, err)
			}
		})
	}
}

func TestParsePublishedWorkflowRejectsUnsafeYAMLAndSchemaDrift(t *testing.T) {
	tests := map[string]string{
		"numeric dsl version":       strings.Replace(validPublishedWorkflowYAML, `dsl_version: "1"`, `dsl_version: 1`, 1),
		"single quoted dsl version": strings.Replace(validPublishedWorkflowYAML, `dsl_version: "1"`, `dsl_version: '1'`, 1),
		"block scalar dsl version":  strings.Replace(validPublishedWorkflowYAML, `dsl_version: "1"`, "dsl_version: >-\n  1", 1),
		"unsupported version":       strings.Replace(validPublishedWorkflowYAML, `dsl_version: "1"`, `dsl_version: "2"`, 1),
		"second document":           validPublishedWorkflowYAML + "---\ndsl_version: \"1\"\nname: second\nversion: \"1\"\nroutes: []\n",
		"empty second document":     validPublishedWorkflowYAML + "---\n...\n",
		"directive": `%YAML 1.1
---
` + validPublishedWorkflowYAML,
		"directive after BOM":             "\xef\xbb\xbf%YAML 1.1\n---\n" + validPublishedWorkflowYAML,
		"directive after CR-only comment": "# preface\r%YAML 1.1\r---\r" + strings.ReplaceAll(validPublishedWorkflowYAML, "\n", "\r"),
		"tag directive":                   "%TAG !e! tag:example.invalid,2026:\n---\n" + validPublishedWorkflowYAML,
		"duplicate key":                   strings.Replace(validPublishedWorkflowYAML, "name: adt-preview", "name: adt-preview\nname: duplicate", 1),
		"anchor":                          strings.Replace(validPublishedWorkflowYAML, "filter:\n", "filter: &shared-filter\n", 1),
		"alias": strings.Replace(validPublishedWorkflowYAML,
			"  - name: admit-primary",
			"  - &shared-route\n    name: admit-primary", 1) + "  - *shared-route\n",
		"merge key":                 strings.Replace(validPublishedWorkflowYAML, "    filter:\n", "    filter:\n      <<: {source: adt-east}\n", 1),
		"explicit tag":              strings.Replace(validPublishedWorkflowYAML, `dsl_version: "1"`, `dsl_version: !!str "1"`, 1),
		"explicit non-specific tag": strings.Replace(validPublishedWorkflowYAML, "name: adt-preview", "name: ! adt-preview", 1),
		"unknown root field":        strings.Replace(validPublishedWorkflowYAML, "name: adt-preview", "unknown: value\nname: adt-preview", 1),
		"unknown nested field":      strings.Replace(validPublishedWorkflowYAML, "      source: adt-east", "      source: adt-east\n      surprise: value", 1),
		"action config":             strings.Replace(validPublishedWorkflowYAML, "        destination: fhir-primary", "        destination: fhir-primary\n        url: https://secret.invalid", 1),
		"unknown action type":       strings.Replace(validPublishedWorkflowYAML, "type: webhook", "type: custom_delivery", 1),
		"mixed root":                "workflow:\n" + indentYAML(validPublishedWorkflowYAML, "  ") + "name: smuggled\n",
		"non-string scalar":         strings.Replace(validPublishedWorkflowYAML, "name: adt-preview", "name: true", 1),
		"missing action id":         strings.Replace(validPublishedWorkflowYAML, "      - id: send-primary\n", "      - ", 1),
		"reserved action id":        strings.Replace(validPublishedWorkflowYAML, "id: send-primary", "id: legacy-action-0001", 1),
		"duplicate action id": strings.Replace(validPublishedWorkflowYAML,
			"      - id: audit-only",
			"      - id: send-primary", 1),
	}

	for name, document := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParsePublishedWorkflow([]byte(document)); err == nil {
				t.Fatal("ParsePublishedWorkflow() accepted invalid document")
			} else {
				for _, forbidden := range []string{"secret.invalid", "smuggled", "surprise"} {
					if strings.Contains(err.Error(), forbidden) {
						t.Fatalf("error exposed rejected YAML value %q: %v", forbidden, err)
					}
				}
			}
		})
	}
}

func TestParsePublishedWorkflowEnforcesResourceLimits(t *testing.T) {
	tests := map[string]struct {
		document []byte
		want     string
	}{
		"bytes":          {document: bytes.Repeat([]byte{'#'}, MaxPublishedWorkflowYAMLBytes+1), want: "byte limit"},
		"nodes":          {document: []byte("dsl_version: \"1\"\nname: nodes\nversion: \"1\"\nroutes: []\nunknown:\n" + nestedSequences(40, 500)), want: "node limit"},
		"depth":          {document: []byte("dsl_version: \"1\"\nname: depth\nversion: \"1\"\nroutes: []\nunknown: " + nestedFlowSequence(maxPublishedWorkflowDepth+1) + "\n"), want: "nesting limit"},
		"mapping pairs":  {document: []byte("dsl_version: \"1\"\nname: pairs\nversion: \"1\"\nroutes: []\nunknown:\n" + mappingPairs(maxPublishedWorkflowMappingPairs+1)), want: "pair limit"},
		"sequence items": {document: []byte("dsl_version: \"1\"\nname: sequence\nversion: \"1\"\nroutes: []\nunknown:\n" + sequenceItems(maxPublishedWorkflowSequenceItems+1, "  ")), want: "item limit"},
		"scalar bytes":   {document: []byte("dsl_version: \"1\"\nname: \"" + strings.Repeat("x", maxPublishedWorkflowScalarBytes+1) + "\"\nversion: \"1\"\nroutes: []\n"), want: "scalar exceeds byte limit"},
		"routes":         {document: []byte(publishedWorkflowWithRoutes(maxPublishedWorkflowRoutes + 1)), want: "route count"},
		"actions":        {document: []byte(publishedWorkflowWithActions(maxPublishedWorkflowActionsPerRoute + 1)), want: "action count"},
		"transforms":     {document: []byte(publishedWorkflowWithTransforms(maxPublishedWorkflowTransformsPerRoute + 1)), want: "transform count"},
		"filter values":  {document: []byte(publishedWorkflowWithFilterValues(maxPublishedWorkflowFilterValues + 1)), want: "filter value count"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParsePublishedWorkflow(test.document); err == nil {
				t.Fatal("ParsePublishedWorkflow() accepted document beyond its resource limit")
			} else if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("ParsePublishedWorkflow() error = %v, want category %q", err, test.want)
			}
		})
	}
}

func TestParsePublishedWorkflowRejectsDeepFlowBeforeYAMLDecode(t *testing.T) {
	const helperEnvironment = "FI_FHIR_DEEP_YAML_HELPER"
	if os.Getenv(helperEnvironment) == "1" {
		depth := 9_000
		deepValue := strings.Repeat("[", depth) + `"value"` + strings.Repeat("]", depth)
		documents := []string{
			"dsl_version: \"1\"\nname: deep\nversion: \"1\"\nroutes: []\nunknown: " + deepValue + "\n",
			"dsl_version: \"1\"\nname: deep\nversion: \"1\"\nroutes: []\nunknown: {a: \"multi\n  line\", b: " + deepValue + "}\n",
			"dsl_version: \"1\"\nname: deep\nversion: \"1\"\nroutes: []\nunknown: {a: don't, b: " + deepValue + "}\n",
			"dsl_version: \"1\"\nname: deep\nversion: \"1\"\nroutes: []\nunknown: {a: plain\n  'continuation, b: " + deepValue + "}\n",
			"dsl_version: \"1\"\nname: deep\nversion: \"1\"\nroutes: []\nunknown: {a: scheme:'continuation, b: " + deepValue + "}\n",
		}
		for _, document := range documents {
			if _, err := ParsePublishedWorkflow([]byte(document)); err == nil {
				os.Exit(2)
			} else if !strings.Contains(err.Error(), "pre-decode nesting limit") {
				os.Exit(3)
			}
		}
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestParsePublishedWorkflowRejectsDeepFlowBeforeYAMLDecode$")
	command.Env = append(os.Environ(), helperEnvironment+"=1")
	output, err := command.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("deep YAML guard did not terminate before decode: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("deep YAML helper crashed or accepted input: %v\n%s", err, output)
	}
}

func TestSplitPublishedYAMLLinesRecognizesEveryYAMLBreak(t *testing.T) {
	lines := splitPublishedYAMLLines([]byte("a\nb\rc\r\nd\u0085e\u2028f\u2029g"))
	if got, want := len(lines), 7; got != want {
		t.Fatalf("split YAML lines = %d, want %d: %#v", got, want, lines)
	}
}

func TestParseWorkflowLegacyBehaviorRemainsPermissive(t *testing.T) {
	legacy := []byte(`name: legacy
version: "1"
unknown_root: retained-compatibility
routes:
  - name: route
    filter: {}
    actions:
      - type: webhook
        url: https://example.invalid
`)
	parsed, err := ParseWorkflow(legacy)
	if err != nil {
		t.Fatalf("ParseWorkflow() legacy error = %v", err)
	}
	if got := parsed.Routes[0].Actions[0].Config["url"]; got != "https://example.invalid" {
		t.Fatalf("legacy action Config[url] = %q", got)
	}
}

func TestLegacyActionParserPreservesConfigWhileExposingPlanningMetadata(t *testing.T) {
	var action Action
	if err := yaml.Unmarshal([]byte("id: send\ntype: webhook\ndestination: primary\nurl: https://example.invalid\n"), &action); err != nil {
		t.Fatalf("yaml.Unmarshal(Action) error = %v", err)
	}
	if action.ID != "send" || action.Type != "webhook" || action.Destination != "primary" {
		t.Fatalf("action metadata = %#v", action)
	}
	if got := action.Config["id"]; got != "send" {
		t.Fatalf("legacy action Config[id] = %q", got)
	}
	if got := action.Config["destination"]; got != "primary" {
		t.Fatalf("legacy action Config[destination] = %q", got)
	}
	if got := action.Config["url"]; got != "https://example.invalid" {
		t.Fatalf("action Config[url] = %q", got)
	}
}

func indentYAML(input, prefix string) string {
	lines := strings.SplitAfter(input, "\n")
	var result strings.Builder
	for _, line := range lines {
		if line == "" {
			continue
		}
		result.WriteString(prefix)
		result.WriteString(line)
	}
	return result.String()
}

func nestedSequences(groups, items int) string {
	var result strings.Builder
	for group := 0; group < groups; group++ {
		result.WriteString("  - [")
		for item := 0; item < items; item++ {
			if item > 0 {
				result.WriteByte(',')
			}
			result.WriteByte('x')
		}
		result.WriteString("]\n")
	}
	return result.String()
}

func nestedFlowSequence(depth int) string {
	return strings.Repeat("[", depth) + `"value"` + strings.Repeat("]", depth)
}

func mappingPairs(count int) string {
	var result strings.Builder
	for index := 0; index < count; index++ {
		fmt.Fprintf(&result, "  key_%04d: value\n", index)
	}
	return result.String()
}

func sequenceItems(count int, indent string) string {
	var result strings.Builder
	for index := 0; index < count; index++ {
		fmt.Fprintf(&result, "%s- value_%04d\n", indent, index)
	}
	return result.String()
}

func publishedWorkflowWithRoutes(count int) string {
	var result strings.Builder
	result.WriteString("dsl_version: \"1\"\nname: routes\nversion: \"1\"\nroutes:\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&result, "  - name: route_%04d\n    filter: {}\n    actions:\n      - id: action\n        type: log\n", index)
	}
	return result.String()
}

func publishedWorkflowWithActions(count int) string {
	var result strings.Builder
	result.WriteString("dsl_version: \"1\"\nname: actions\nversion: \"1\"\nroutes:\n  - name: route\n    filter: {}\n    actions:\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&result, "      - id: action_%04d\n        type: log\n", index)
	}
	return result.String()
}

func publishedWorkflowWithTransforms(count int) string {
	var result strings.Builder
	result.WriteString("dsl_version: \"1\"\nname: transforms\nversion: \"1\"\nroutes:\n  - name: route\n    filter: {}\n    transform:\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&result, "      - set_field: field_%04d\n", index)
	}
	result.WriteString("    actions:\n      - id: action\n        type: log\n")
	return result.String()
}

func publishedWorkflowWithFilterValues(count int) string {
	var result strings.Builder
	result.WriteString("dsl_version: \"1\"\nname: filters\nversion: \"1\"\nroutes:\n  - name: route\n    filter:\n      event_type:\n")
	for index := 0; index < count; index++ {
		fmt.Fprintf(&result, "        - event_%04d\n", index)
	}
	result.WriteString("    actions:\n      - id: action\n        type: log\n")
	return result.String()
}
