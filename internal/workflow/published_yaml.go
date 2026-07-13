package workflow

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	// MaxPublishedWorkflowYAMLBytes bounds executable workflow artifacts before parsing.
	MaxPublishedWorkflowYAMLBytes = 256 << 10

	maxPublishedWorkflowNodes              = 20_000
	maxPublishedWorkflowDepth              = 32
	maxPublishedWorkflowMappingPairs       = 256
	maxPublishedWorkflowSequenceItems      = 512
	maxPublishedWorkflowScalarBytes        = 16 << 10
	maxPublishedWorkflowRoutes             = 256
	maxPublishedWorkflowActionsPerRoute    = 128
	maxPublishedWorkflowTransformsPerRoute = 128
	maxPublishedWorkflowFilterValues       = 256
	maxPublishedWorkflowIdentifierBytes    = 128
	maxPublishedWorkflowActionIDBytes      = 64
	maxPublishedWorkflowIndentBytes        = 64
	maxPublishedWorkflowRawFlowOpeners     = 512
	publishedWorkflowDSLVersion            = "1"
	legacyActionIDPrefix                   = "legacy-action-"
)

var (
	errInvalidPublishedWorkflow = errors.New("invalid published workflow")
	publishedIdentifierPattern  = regexp.MustCompile(`^[a-z][a-z0-9_.-]*$`)
	publishedActionIDPattern    = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
)

// PublishedWorkflow seals an exact, strictly validated executable workflow artifact.
// Accessors return defensive copies so callers cannot mutate its bytes or definition.
type PublishedWorkflow struct {
	dslVersion string
	yaml       []byte
	workflow   Workflow
}

// DSLVersion returns the published workflow grammar version.
func (w *PublishedWorkflow) DSLVersion() string {
	if w == nil {
		return ""
	}
	return w.dslVersion
}

// YAML returns an exact defensive copy of the published bytes.
func (w *PublishedWorkflow) YAML() []byte {
	if w == nil {
		return nil
	}
	return append([]byte(nil), w.yaml...)
}

// Workflow returns a defensive copy of the executable definition.
func (w *PublishedWorkflow) Workflow() *Workflow {
	if w == nil {
		return nil
	}
	clone := cloneWorkflow(w.workflow)
	return &clone
}

// ParsePublishedWorkflow strictly validates one versioned executable workflow.
// Legacy ParseWorkflow deliberately remains permissive for non-runtime callers.
func ParsePublishedWorkflow(data []byte) (*PublishedWorkflow, error) {
	if len(data) == 0 {
		return nil, publishedWorkflowError("document is empty")
	}
	if len(data) > MaxPublishedWorkflowYAMLBytes {
		return nil, publishedWorkflowError("document exceeds byte limit")
	}
	if !utf8.Valid(data) {
		return nil, publishedWorkflowError("document is not valid UTF-8")
	}
	if err := preflightPublishedYAMLBytes(data); err != nil {
		return nil, err
	}
	if containsYAMLDirective(data) {
		return nil, publishedWorkflowError("YAML directives are not supported")
	}
	if containsExplicitNonSpecificTag(data) {
		return nil, publishedWorkflowError("explicit YAML tags are not supported")
	}

	document, err := decodeSingleYAMLDocument(data)
	if err != nil {
		return nil, err
	}
	if err := preflightPublishedYAML(document); err != nil {
		return nil, err
	}

	wrapped, err := publishedWorkflowRootMode(document)
	if err != nil {
		return nil, err
	}
	if err := validatePublishedDSLVersionNode(document, wrapped); err != nil {
		return nil, err
	}
	dto, err := decodePublishedWorkflowDTO(data, wrapped)
	if err != nil {
		return nil, err
	}
	definition, err := dto.toWorkflow()
	if err != nil {
		return nil, err
	}

	return &PublishedWorkflow{
		dslVersion: dto.DSLVersion,
		yaml:       append([]byte(nil), data...),
		workflow:   definition,
	}, nil
}

// preflightPublishedYAMLBytes rejects recursively dangerous source shapes
// before yaml.v3 constructs a node tree. The later node walk still owns the
// authoritative schema budget; this lexical pass caps flow, compact, and
// indentation depth. A conservative source-wide flow-opener budget closes
// lexical ambiguities without asking yaml.v3 to inspect an adversarial tree.
func preflightPublishedYAMLBytes(data []byte) error {
	if publishedYAMLRawFlowOpeners(data) > maxPublishedWorkflowRawFlowOpeners {
		return publishedWorkflowError("document exceeds pre-decode nesting limit")
	}

	flowDepth := 0
	blockScalarIndent := -1
	inSingleQuote := false
	inDoubleQuote := false
	plainScalarStarted := false
	for _, line := range splitPublishedYAMLLines(data) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		indent := publishedYAMLIndent(line)
		if indent > maxPublishedWorkflowIndentBytes {
			return publishedWorkflowError("document exceeds pre-decode nesting limit")
		}
		if blockScalarIndent >= 0 {
			if indent > blockScalarIndent {
				continue
			}
			blockScalarIndent = -1
		}
		if flowDepth == 0 && !inSingleQuote && !inDoubleQuote {
			plainScalarStarted = false
		}

		compactDepth := 0
	lineLoop:
		for index := 0; index < len(line); index++ {
			character := line[index]
			switch {
			case inSingleQuote:
				if character == '\'' {
					if index+1 < len(line) && line[index+1] == '\'' {
						index++
					} else {
						inSingleQuote = false
						plainScalarStarted = true
					}
				}
				continue
			case inDoubleQuote:
				if character == '\\' {
					index++
				} else if character == '"' {
					inDoubleQuote = false
					plainScalarStarted = true
				}
				continue
			}

			switch character {
			case '\'':
				if !plainScalarStarted {
					inSingleQuote = true
				} else {
					plainScalarStarted = true
				}
			case '"':
				if !plainScalarStarted {
					inDoubleQuote = true
				} else {
					plainScalarStarted = true
				}
			case '#':
				if !plainScalarStarted || index == 0 || line[index-1] == ' ' || line[index-1] == '\t' {
					break lineLoop
				}
			case '[', '{':
				flowDepth++
				if flowDepth > maxPublishedWorkflowDepth {
					return publishedWorkflowError("document exceeds pre-decode nesting limit")
				}
				plainScalarStarted = false
			case ']', '}':
				if flowDepth > 0 {
					flowDepth--
				}
				plainScalarStarted = true
			case ',':
				plainScalarStarted = false
			case ':':
				if flowDepth > 0 || publishedYAMLValueIndicator(line, index+1) {
					plainScalarStarted = false
				} else {
					plainScalarStarted = true
				}
			case ' ', '\t':
				continue
			case '-', '?':
				if flowDepth == 0 &&
					publishedYAMLTokenBoundary(line, index-1) &&
					publishedYAMLTagTerminator(line, index+1) {
					compactDepth++
					if compactDepth > maxPublishedWorkflowDepth {
						return publishedWorkflowError("document exceeds pre-decode nesting limit")
					}
					plainScalarStarted = false
				} else {
					plainScalarStarted = true
				}
			case '|', '>':
				if !plainScalarStarted && flowDepth == 0 && publishedYAMLBlockScalarIndicator(line, index) {
					blockScalarIndent = indent
					break lineLoop
				}
				plainScalarStarted = true
			default:
				plainScalarStarted = true
			}
		}
	}
	return nil
}

func decodeSingleYAMLDocument(data []byte) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return nil, publishedWorkflowError("document cannot be decoded")
	}
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, publishedWorkflowError("document root is invalid")
	}
	var trailing yaml.Node
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, publishedWorkflowError("exactly one YAML document is required")
	}
	return &document, nil
}

func containsYAMLDirective(data []byte) bool {
	for _, line := range splitPublishedYAMLLines(data) {
		if len(line) > 0 && line[0] == '%' {
			return true
		}
	}
	return false
}

func splitPublishedYAMLLines(data []byte) [][]byte {
	data = bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})
	return bytes.FieldsFunc(data, func(r rune) bool {
		return r == '\n' || r == '\r' || r == '\u0085' || r == '\u2028' || r == '\u2029'
	})
}

func publishedYAMLRawFlowOpeners(data []byte) int {
	count := 0
	for _, character := range data {
		if character == '[' || character == '{' {
			count++
		}
	}
	return count
}

func containsExplicitNonSpecificTag(data []byte) bool {
	blockScalarIndent := -1

lineLoop:
	for _, line := range splitPublishedYAMLLines(data) {
		indent := publishedYAMLIndent(line)
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if blockScalarIndent >= 0 {
			if indent > blockScalarIndent {
				continue
			}
			blockScalarIndent = -1
		}

		inSingleQuote := false
		inDoubleQuote := false
		plainScalarStarted := false
		for index := 0; index < len(line); index++ {
			character := line[index]
			switch {
			case inSingleQuote:
				if character == '\'' {
					if index+1 < len(line) && line[index+1] == '\'' {
						index++
					} else {
						inSingleQuote = false
					}
				}
				continue
			case inDoubleQuote:
				if character == '\\' {
					index++
				} else if character == '"' {
					inDoubleQuote = false
				}
				continue
			}

			switch character {
			case '\'':
				inSingleQuote = true
				plainScalarStarted = true
			case '"':
				inDoubleQuote = true
				plainScalarStarted = true
			case '#':
				continue lineLoop
			case ' ', '\t':
				continue
			case '!':
				if !plainScalarStarted && publishedYAMLTagTerminator(line, index+1) {
					return true
				}
				plainScalarStarted = true
			case ':':
				if publishedYAMLValueIndicator(line, index+1) {
					plainScalarStarted = false
				} else {
					plainScalarStarted = true
				}
			case ',', '[', '{':
				plainScalarStarted = false
			case '-', '?':
				if plainScalarStarted || !publishedYAMLTagTerminator(line, index+1) {
					plainScalarStarted = true
				}
			case '|', '>':
				if publishedYAMLBlockScalarIndicator(line, index) {
					blockScalarIndent = indent
				}
				plainScalarStarted = true
			default:
				plainScalarStarted = true
			}
		}
	}
	return false
}

func publishedYAMLIndent(line []byte) int {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	return indent
}

func publishedYAMLTokenBoundary(line []byte, index int) bool {
	if index < 0 {
		return true
	}
	character := line[index]
	return character == ' ' || character == '\t' || strings.ContainsRune("-?:,[]{}", rune(character))
}

func publishedYAMLTagTerminator(line []byte, index int) bool {
	if index >= len(line) {
		return true
	}
	character := line[index]
	return character == ' ' || character == '\t' || character == '#' || character == '[' || character == '{'
}

func publishedYAMLValueIndicator(line []byte, index int) bool {
	if index >= len(line) {
		return true
	}
	character := line[index]
	return character == ' ' || character == '\t' || strings.ContainsRune("[],{}", rune(character))
}

func publishedYAMLBlockScalarIndicator(line []byte, index int) bool {
	if !publishedYAMLTokenBoundary(line, index-1) {
		return false
	}
	index++
	for index < len(line) && (line[index] == '+' || line[index] == '-' || (line[index] >= '1' && line[index] <= '9')) {
		index++
	}
	for index < len(line) && (line[index] == ' ' || line[index] == '\t') {
		index++
	}
	return index == len(line) || line[index] == '#'
}

type publishedYAMLBudget struct {
	nodes int
}

func preflightPublishedYAML(document *yaml.Node) error {
	budget := &publishedYAMLBudget{}
	return budget.walk(document, 0)
}

func (b *publishedYAMLBudget) walk(node *yaml.Node, depth int) error {
	if node == nil {
		return publishedWorkflowError("document contains an empty node")
	}
	if depth > maxPublishedWorkflowDepth {
		return publishedWorkflowError("document exceeds nesting limit")
	}
	b.nodes++
	if b.nodes > maxPublishedWorkflowNodes {
		return publishedWorkflowError("document exceeds node limit")
	}
	if node.Kind == yaml.AliasNode || node.Alias != nil || node.Anchor != "" {
		return publishedWorkflowError("aliases and anchors are not supported")
	}
	if node.Style&yaml.TaggedStyle != 0 {
		return publishedWorkflowError("explicit YAML tags are not supported")
	}

	switch node.Kind {
	case yaml.DocumentNode:
		if len(node.Content) != 1 {
			return publishedWorkflowError("document root is invalid")
		}
	case yaml.MappingNode:
		if len(node.Content)%2 != 0 {
			return publishedWorkflowError("mapping is malformed")
		}
		if len(node.Content)/2 > maxPublishedWorkflowMappingPairs {
			return publishedWorkflowError("mapping exceeds pair limit")
		}
		seen := make(map[string]struct{}, len(node.Content)/2)
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index]
			if key.Kind != yaml.ScalarNode || key.Tag != "!!str" {
				return publishedWorkflowError("mapping keys must be strings")
			}
			if key.Value == "<<" || key.Tag == "!!merge" {
				return publishedWorkflowError("YAML merge keys are not supported")
			}
			if _, duplicate := seen[key.Value]; duplicate {
				return publishedWorkflowError("mapping contains a duplicate key")
			}
			seen[key.Value] = struct{}{}
		}
	case yaml.SequenceNode:
		if len(node.Content) > maxPublishedWorkflowSequenceItems {
			return publishedWorkflowError("sequence exceeds item limit")
		}
	case yaml.ScalarNode:
		if len(node.Value) > maxPublishedWorkflowScalarBytes {
			return publishedWorkflowError("scalar exceeds byte limit")
		}
		if node.Tag != "!!str" {
			return publishedWorkflowError("schema scalars must be strings")
		}
	default:
		return publishedWorkflowError("document contains an unsupported node")
	}

	for _, child := range node.Content {
		if err := b.walk(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

func publishedWorkflowRootMode(document *yaml.Node) (bool, error) {
	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return false, publishedWorkflowError("document root must be a mapping")
	}
	workflowIndex := -1
	for index := 0; index < len(root.Content); index += 2 {
		if root.Content[index].Value == "workflow" {
			workflowIndex = index
			break
		}
	}
	if workflowIndex < 0 {
		return false, nil
	}
	if len(root.Content) != 2 {
		return false, publishedWorkflowError("wrapped document root must contain only workflow")
	}
	if root.Content[workflowIndex+1].Kind != yaml.MappingNode {
		return false, publishedWorkflowError("wrapped workflow must be a mapping")
	}
	return true, nil
}

func validatePublishedDSLVersionNode(document *yaml.Node, wrapped bool) error {
	root := document.Content[0]
	if wrapped {
		root = root.Content[1]
	}
	for index := 0; index < len(root.Content); index += 2 {
		if root.Content[index].Value != "dsl_version" {
			continue
		}
		value := root.Content[index+1]
		if value.Kind != yaml.ScalarNode || value.Style&yaml.DoubleQuotedStyle == 0 || value.Value != publishedWorkflowDSLVersion {
			return publishedWorkflowError("dsl_version must use the canonical quoted form")
		}
		return nil
	}
	return publishedWorkflowError("dsl_version is required")
}

func decodePublishedWorkflowDTO(data []byte, wrapped bool) (publishedWorkflowDTO, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if wrapped {
		var envelope struct {
			Workflow publishedWorkflowDTO `yaml:"workflow"`
		}
		if err := decoder.Decode(&envelope); err != nil {
			return publishedWorkflowDTO{}, publishedWorkflowError("document does not match the published workflow schema")
		}
		return envelope.Workflow, nil
	}

	var workflow publishedWorkflowDTO
	if err := decoder.Decode(&workflow); err != nil {
		return publishedWorkflowDTO{}, publishedWorkflowError("document does not match the published workflow schema")
	}
	return workflow, nil
}

type publishedWorkflowDTO struct {
	DSLVersion string              `yaml:"dsl_version"`
	Name       string              `yaml:"name"`
	Version    string              `yaml:"version"`
	Routes     []publishedRouteDTO `yaml:"routes"`
}

type publishedRouteDTO struct {
	Name       string                  `yaml:"name"`
	Filter     publishedFilterDTO      `yaml:"filter"`
	Transforms []publishedTransformDTO `yaml:"transform,omitempty"`
	Actions    []publishedActionDTO    `yaml:"actions"`
}

type publishedFilterDTO struct {
	EventType publishedStringList `yaml:"event_type,omitempty"`
	Source    publishedStringList `yaml:"source,omitempty"`
	Condition string              `yaml:"condition,omitempty"`
}

type publishedActionDTO struct {
	ID          string `yaml:"id"`
	Type        string `yaml:"type"`
	Destination string `yaml:"destination,omitempty"`
}

type publishedTransformDTO struct {
	SetField        *string                      `yaml:"set_field,omitempty"`
	MapTerminology  *publishedTerminologyMapDTO  `yaml:"map_terminology,omitempty"`
	Redact          *publishedRedactDTO          `yaml:"redact,omitempty"`
	ExplainWarnings *publishedExplainWarningsDTO `yaml:"explain_warnings,omitempty"`
}

type publishedTerminologyMapDTO struct {
	Field string `yaml:"field"`
	From  string `yaml:"from"`
	To    string `yaml:"to"`
}

type publishedRedactDTO struct {
	Fields publishedStringList `yaml:"fields"`
}

type publishedExplainWarningsDTO struct {
	Model         string `yaml:"model,omitempty"`
	IncludeFix    string `yaml:"include_fix,omitempty"`
	EnableCache   string `yaml:"enable_cache,omitempty"`
	CacheTTL      string `yaml:"cache_ttl,omitempty"`
	WarningsField string `yaml:"warnings_field,omitempty"`
}

type publishedStringList []string

func (values *publishedStringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		*values = []string{node.Value}
		return nil
	case yaml.SequenceNode:
		result := make([]string, len(node.Content))
		for index, item := range node.Content {
			if item.Kind != yaml.ScalarNode {
				return errInvalidPublishedWorkflow
			}
			result[index] = item.Value
		}
		*values = result
		return nil
	default:
		return errInvalidPublishedWorkflow
	}
}

func (dto publishedWorkflowDTO) toWorkflow() (Workflow, error) {
	if dto.DSLVersion != publishedWorkflowDSLVersion {
		return Workflow{}, publishedWorkflowError("unsupported dsl_version")
	}
	if !validPublishedIdentifier(dto.Name, maxPublishedWorkflowIdentifierBytes) {
		return Workflow{}, publishedWorkflowError("workflow name is invalid")
	}
	if dto.Version == "" || len(dto.Version) > maxPublishedWorkflowIdentifierBytes {
		return Workflow{}, publishedWorkflowError("workflow version is invalid")
	}
	if len(dto.Routes) == 0 || len(dto.Routes) > maxPublishedWorkflowRoutes {
		return Workflow{}, publishedWorkflowError("workflow route count is invalid")
	}

	workflow := Workflow{Name: dto.Name, Version: dto.Version, Routes: make([]Route, len(dto.Routes))}
	seenRoutes := make(map[string]struct{}, len(dto.Routes))
	for routeIndex, routeDTO := range dto.Routes {
		if !validPublishedIdentifier(routeDTO.Name, maxPublishedWorkflowIdentifierBytes) {
			return Workflow{}, publishedWorkflowError("route name is invalid")
		}
		if _, duplicate := seenRoutes[routeDTO.Name]; duplicate {
			return Workflow{}, publishedWorkflowError("route name is duplicated")
		}
		seenRoutes[routeDTO.Name] = struct{}{}
		if len(routeDTO.Filter.EventType) > maxPublishedWorkflowFilterValues || len(routeDTO.Filter.Source) > maxPublishedWorkflowFilterValues {
			return Workflow{}, publishedWorkflowError("filter value count is invalid")
		}
		if err := validatePublishedFilterValues(routeDTO.Filter.EventType); err != nil {
			return Workflow{}, err
		}
		if err := validatePublishedFilterValues(routeDTO.Filter.Source); err != nil {
			return Workflow{}, err
		}
		if len(routeDTO.Actions) == 0 || len(routeDTO.Actions) > maxPublishedWorkflowActionsPerRoute {
			return Workflow{}, publishedWorkflowError("route action count is invalid")
		}
		if len(routeDTO.Transforms) > maxPublishedWorkflowTransformsPerRoute {
			return Workflow{}, publishedWorkflowError("route transform count is invalid")
		}

		route := Route{
			Name: routeDTO.Name,
			Filter: Filter{
				EventType: append(StringOrSlice(nil), routeDTO.Filter.EventType...),
				Source:    append(StringOrSlice(nil), routeDTO.Filter.Source...),
				Condition: routeDTO.Filter.Condition,
			},
			Transforms: make([]Transform, len(routeDTO.Transforms)),
			Actions:    make([]Action, len(routeDTO.Actions)),
		}
		for transformIndex, transformDTO := range routeDTO.Transforms {
			transform, err := transformDTO.toTransform()
			if err != nil {
				return Workflow{}, err
			}
			route.Transforms[transformIndex] = transform
		}
		seenActions := make(map[string]struct{}, len(routeDTO.Actions))
		for actionIndex, actionDTO := range routeDTO.Actions {
			if !validPublishedActionID(actionDTO.ID) {
				return Workflow{}, publishedWorkflowError("action ID is invalid")
			}
			if strings.HasPrefix(actionDTO.ID, legacyActionIDPrefix) {
				return Workflow{}, publishedWorkflowError("action ID uses a reserved prefix")
			}
			if _, duplicate := seenActions[actionDTO.ID]; duplicate {
				return Workflow{}, publishedWorkflowError("action ID is duplicated")
			}
			seenActions[actionDTO.ID] = struct{}{}
			if !validPublishedIdentifier(actionDTO.Type, maxPublishedWorkflowActionIDBytes) {
				return Workflow{}, publishedWorkflowError("action type is invalid")
			}
			if !knownPublishedActionType(actionDTO.Type) {
				return Workflow{}, publishedWorkflowError("action type is not supported by dsl_version 1")
			}
			if actionDTO.Destination != "" && !validPublishedIdentifier(actionDTO.Destination, maxPublishedWorkflowIdentifierBytes) {
				return Workflow{}, publishedWorkflowError("action destination is invalid")
			}
			route.Actions[actionIndex] = Action{ID: actionDTO.ID, Type: actionDTO.Type, Destination: actionDTO.Destination}
		}
		workflow.Routes[routeIndex] = route
	}
	return workflow, nil
}

func validatePublishedFilterValues(values publishedStringList) error {
	for _, value := range values {
		if value == "" || len(value) > maxPublishedWorkflowIdentifierBytes {
			return publishedWorkflowError("filter value is invalid")
		}
	}
	return nil
}

func (dto publishedTransformDTO) toTransform() (Transform, error) {
	configured := 0
	if dto.SetField != nil {
		configured++
	}
	if dto.MapTerminology != nil {
		configured++
	}
	if dto.Redact != nil {
		configured++
	}
	if dto.ExplainWarnings != nil {
		configured++
	}
	if configured != 1 {
		return Transform{}, publishedWorkflowError("transform must select exactly one kind")
	}

	var transform Transform
	switch {
	case dto.SetField != nil:
		if *dto.SetField == "" {
			return Transform{}, publishedWorkflowError("set_field transform is empty")
		}
		transform.SetField = *dto.SetField
	case dto.MapTerminology != nil:
		if dto.MapTerminology.Field == "" || dto.MapTerminology.From == "" || dto.MapTerminology.To == "" {
			return Transform{}, publishedWorkflowError("map_terminology transform is incomplete")
		}
		transform.MapTerminology = &TerminologyMap{Field: dto.MapTerminology.Field, From: dto.MapTerminology.From, To: dto.MapTerminology.To}
	case dto.Redact != nil:
		if len(dto.Redact.Fields) == 0 || len(dto.Redact.Fields) > maxPublishedWorkflowFilterValues {
			return Transform{}, publishedWorkflowError("redact transform field count is invalid")
		}
		transform.Redact = &RedactConfig{Fields: append([]string(nil), dto.Redact.Fields...)}
	case dto.ExplainWarnings != nil:
		includeFix, err := parsePublishedBool(dto.ExplainWarnings.IncludeFix)
		if err != nil {
			return Transform{}, err
		}
		enableCache, err := parsePublishedBool(dto.ExplainWarnings.EnableCache)
		if err != nil {
			return Transform{}, err
		}
		transform.ExplainWarnings = &ExplainWarningsConfig{
			Model:         dto.ExplainWarnings.Model,
			IncludeFix:    includeFix,
			EnableCache:   enableCache,
			CacheTTL:      dto.ExplainWarnings.CacheTTL,
			WarningsField: dto.ExplainWarnings.WarningsField,
		}
	}
	return transform, nil
}

func parsePublishedBool(value string) (bool, error) {
	if value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil || (value != "true" && value != "false") {
		return false, publishedWorkflowError("boolean option must be true or false")
	}
	return parsed, nil
}

func validPublishedIdentifier(value string, maxBytes int) bool {
	return value != "" && len(value) <= maxBytes && publishedIdentifierPattern.MatchString(value)
}

func validPublishedActionID(value string) bool {
	return value != "" && len(value) <= maxPublishedWorkflowActionIDBytes && publishedActionIDPattern.MatchString(value)
}

func knownPublishedActionType(value string) bool {
	switch value {
	case "log", "webhook", "fhir", "email", "exec", "file", "database", "queue", "event_store", "athena":
		return true
	default:
		return false
	}
}

func publishedWorkflowError(message string) error {
	return fmt.Errorf("%w: %s", errInvalidPublishedWorkflow, message)
}

func cloneWorkflow(source Workflow) Workflow {
	clone := source
	clone.Routes = make([]Route, len(source.Routes))
	for routeIndex, sourceRoute := range source.Routes {
		route := sourceRoute
		route.Filter.EventType = append(StringOrSlice(nil), sourceRoute.Filter.EventType...)
		route.Filter.Source = append(StringOrSlice(nil), sourceRoute.Filter.Source...)
		route.Transforms = make([]Transform, len(sourceRoute.Transforms))
		for transformIndex, sourceTransform := range sourceRoute.Transforms {
			transform := sourceTransform
			if sourceTransform.MapTerminology != nil {
				value := *sourceTransform.MapTerminology
				transform.MapTerminology = &value
			}
			if sourceTransform.Redact != nil {
				value := *sourceTransform.Redact
				value.Fields = append([]string(nil), sourceTransform.Redact.Fields...)
				transform.Redact = &value
			}
			if sourceTransform.ExplainWarnings != nil {
				value := *sourceTransform.ExplainWarnings
				transform.ExplainWarnings = &value
			}
			route.Transforms[transformIndex] = transform
		}
		route.Actions = make([]Action, len(sourceRoute.Actions))
		for actionIndex, sourceAction := range sourceRoute.Actions {
			action := sourceAction
			if sourceAction.Config != nil {
				action.Config = make(map[string]string, len(sourceAction.Config))
				for key, value := range sourceAction.Config {
					action.Config[key] = value
				}
			}
			route.Actions[actionIndex] = action
		}
		clone.Routes[routeIndex] = route
	}
	return clone
}
