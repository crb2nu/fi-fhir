// Package kernel is the engine path the browser playground runs: compile a
// Source Profile with the production compiler, parse one HL7v2 message with
// the production parser, and project the event into the conditional FHIR
// transaction Bundle a `fhir` destination would receive.
//
// It is the pure-Go core of cmd/fi-fhir-wasm and has no syscall/js, no
// filesystem, and no network, so it builds for GOOS=js and runs in the
// ordinary test binary. Its preview path is the integration session runner's
// (internal/integration/session/runner.go RunHL7v2): the same
// processor.CompileProfileRevision, the same hl7v2.NewParser configuration,
// the same diagnostic normalization, and the same failure diagnostic.
// TestWASMKernelMatchesSessionRunner holds the two byte-identical on the demo
// samples the IDE ships, so the playground cannot quietly become a different
// engine from the IDE.
//
// Every entry point takes and returns plain values and never panics to its
// caller: a bad request, profile, or message is OK=false with Problems.
package kernel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/fhirout"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/hl7v2"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/profile"
)

const (
	// MaxInputBytes caps every string the kernel accepts (a message, a
	// profile). It is the processor's own preview limit, so a message the
	// playground accepts is one the server-side preview accepts too.
	MaxInputBytes = processor.MaxPreviewSourceBytes
	// maxRequestBytes caps the JSON envelope of a preview request: a message
	// and a profile at their limits, with room for JSON string escaping.
	maxRequestBytes = 3 * MaxInputBytes
	// MaxDiagnostics caps the diagnostics a preview returns. Past it one
	// DIAGNOSTICS_TRUNCATED notice says how many were dropped.
	MaxDiagnostics = 200
	// MaxSegments caps the segments a preview returns. Past it one
	// SEGMENTS_TRUNCATED notice says how many were dropped.
	MaxSegments = 1000

	// FormatHL7v2 is the only message format the kernel parses.
	FormatHL7v2 = string(events.FormatHL7v2)
	// DefaultSource is the event source when a request names none.
	DefaultSource = "playground"
	// PlaygroundProfileID is the artifact identity a browser-supplied profile
	// compiles under; it becomes the event's source_profile_id.
	PlaygroundProfileID = "playground-profile"

	// playgroundTenantID is the tenant the canonical event is stored under
	// on its way to the FHIR projection. Nothing is stored; the projection
	// only needs a well-formed metadata envelope.
	playgroundTenantID = "playground"
)

// Problem codes. A problem means the request could not be previewed as asked;
// it is never a parser warning (those are Diagnostics).
const (
	CodeInputTooLarge      = "INPUT_TOO_LARGE"
	CodeInputInvalid       = "INPUT_INVALID"
	CodeMessageEmpty       = "MESSAGE_EMPTY"
	CodeMessageTooLarge    = "MESSAGE_TOO_LARGE"
	CodeFormatUnsupported  = "FORMAT_UNSUPPORTED"
	CodeProfileEmpty       = "PROFILE_EMPTY"
	CodeProfileTooLarge    = "PROFILE_TOO_LARGE"
	CodeProfileSyntax      = "PROFILE_SYNTAX"
	CodeProfileInvalid     = "PROFILE_INVALID"
	CodeProfileUnsupported = "PROFILE_UNSUPPORTED"
	CodeTimezoneInvalid    = "TIMEZONE_INVALID"
	CodeTimezoneConflict   = "TIMEZONE_CONFLICT"
	CodeParseFailed        = "PARSE_FAILED"
	CodeInternalError      = "INTERNAL_ERROR"

	// CodeFHIRProjectionUnsupported is the workflow planner's code for an
	// event type fhirout cannot project; the kernel reports it the same way.
	CodeFHIRProjectionUnsupported = "FHIR_PROJECTION_UNSUPPORTED"
	// CodeFHIRProjectionFailed means a supported event could not become a
	// conditional Bundle (for example a resource with no usable identifier).
	CodeFHIRProjectionFailed = "FHIR_PROJECTION_FAILED"

	codeDiagnosticsTruncated = "DIAGNOSTICS_TRUNCATED"
	codeSegmentsTruncated    = "SEGMENTS_TRUNCATED"
)

// PreviewRequest is the JSON document fiFhirPreview takes.
type PreviewRequest struct {
	// Message is the raw HL7v2 message. Segments may end in CR, LF, or CRLF.
	Message string `json:"message"`
	// Format must be "hl7v2".
	Format string `json:"format"`
	// ProfileYAML is an optional Source Profile (YAML or JSON). Without one
	// the parser runs with its defaults, as a session run without a profile
	// draft does.
	ProfileYAML string `json:"profileYaml,omitempty"`
	// Timezone is an optional IANA zone for unzoned HL7 timestamps when no
	// profile is given. A profile's hl7v2.timezone governs when one is; a
	// different Timezone beside it is TIMEZONE_CONFLICT.
	Timezone string `json:"timezone,omitempty"`
	// Source is the optional event source (the IDE uses the sample's source).
	Source string `json:"source,omitempty"`
}

// ProfileIdentity is the artifact identity a profile compiles under. The
// compiled profile's ID becomes the event's source_profile_id.
type ProfileIdentity struct {
	ArtifactID string
	Revision   int
}

// Problem is one reason a request could not be previewed as asked.
type Problem struct {
	Code    string `json:"code"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

// Segment is one HL7v2 segment as the parser tokenized it. Fields is indexed
// by HL7 field number: Fields[0] is the segment ID, and for MSH Fields[1] is
// the field separator and Fields[2] the encoding characters.
type Segment struct {
	Index  int      `json:"index"`
	ID     string   `json:"id"`
	Fields []string `json:"fields"`
}

// Event is one semantic event: its type and the canonical event JSON.
type Event struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Diagnostic is one parser finding, normalized the way a session run
// normalizes it.
type Diagnostic struct {
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Path     string `json:"path"`
	Message  string `json:"message"`
}

// PreviewResponse is the JSON document fiFhirPreview returns.
type PreviewResponse struct {
	OK          bool         `json:"ok"`
	Segments    []Segment    `json:"segments"`
	Events      []Event      `json:"events"`
	Diagnostics []Diagnostic `json:"diagnostics"`
	// Bundle is the conditional FHIR R4 transaction Bundle the event projects
	// to, present only when the event type has a FHIR projection.
	Bundle json.RawMessage `json:"bundle,omitempty"`
	// BundleProblem says why a parsed event has no Bundle.
	BundleProblem *Problem  `json:"bundleProblem,omitempty"`
	Problems      []Problem `json:"problems"`
}

// ValidateResponse is the JSON document fiFhirValidateProfile returns.
type ValidateResponse struct {
	OK       bool      `json:"ok"`
	Problems []Problem `json:"problems"`
}

// Preview decodes one PreviewRequest JSON document and previews it under the
// playground's profile identity.
func Preview(input []byte) PreviewResponse {
	if int64(len(input)) > maxRequestBytes {
		return failed(Problem{
			Code:    CodeInputTooLarge,
			Path:    "$",
			Message: fmt.Sprintf("the preview request is %d bytes; the limit is %d", len(input), maxRequestBytes),
		})
	}
	decoder := json.NewDecoder(bytes.NewReader(input))
	decoder.DisallowUnknownFields()
	var request PreviewRequest
	if err := decoder.Decode(&request); err != nil {
		return failed(Problem{Code: CodeInputInvalid, Path: "$", Message: "the preview request is not a valid JSON object: " + err.Error()})
	}
	if err := decoder.Decode(&json.RawMessage{}); !errors.Is(err, io.EOF) {
		return failed(Problem{Code: CodeInputInvalid, Path: "$", Message: "the preview request has content after its JSON object"})
	}
	return PreviewWith(request, ProfileIdentity{ArtifactID: PlaygroundProfileID, Revision: 1})
}

// PreviewWith previews one request with the profile compiled under identity.
func PreviewWith(request PreviewRequest, identity ProfileIdentity) (response PreviewResponse) {
	defer func() {
		if recovered := recover(); recovered != nil {
			response = failed(Problem{Code: CodeInternalError, Path: "$", Message: fmt.Sprintf("the kernel failed: %v", recovered)})
		}
	}()

	if problems := checkRequest(request); len(problems) > 0 {
		return failed(problems...)
	}
	source := request.Source
	if source == "" {
		source = DefaultSource
	}
	segments, droppedSegments := segmentsOf(request.Message)
	response = PreviewResponse{
		OK:          true,
		Segments:    segments,
		Events:      []Event{},
		Diagnostics: []Diagnostic{},
		Problems:    []Problem{},
	}
	// A segment list past MaxSegments is cut, and says so after whatever the
	// parser reported. It is the one diagnostic a session run never has.
	defer func() {
		if droppedSegments > 0 && len(response.Segments) > 0 {
			response.Diagnostics = append(response.Diagnostics, Diagnostic{
				Severity: "info",
				Code:     codeSegmentsTruncated,
				Path:     "",
				Message:  fmt.Sprintf("%d more segments are not shown", droppedSegments),
			})
		}
	}()

	parserConfig := hl7v2.ParserConfig{}
	var compiled *profile.SourceProfile
	if request.ProfileYAML != "" {
		var timezone *time.Location
		var problems []Problem
		compiled, timezone, problems = compileProfile(request.ProfileYAML, identity, "profileYaml")
		if len(problems) > 0 {
			// The session runner turns a profile that does not compile into a
			// failed run whose one diagnostic is PARSE_FAILED; mirror it so the
			// playground's diagnostics are the IDE's.
			response.OK = false
			response.Problems = problems
			response.Diagnostics = append(response.Diagnostics, failureDiagnostic("compile profile revision: "+problems[0].Message))
			return response
		}
		if request.Timezone != "" && request.Timezone != timezone.String() {
			return failed(Problem{
				Code:    CodeTimezoneConflict,
				Path:    "timezone",
				Message: fmt.Sprintf("the profile reads timestamps as %s; omit timezone or match it", timezone.String()),
			})
		}
		parserConfig.DefaultTimezone = timezone
	} else if request.Timezone != "" {
		location, problem := loadTimezone(request.Timezone)
		if problem != nil {
			return failed(*problem)
		}
		parserConfig.DefaultTimezone = location
	}

	parser := hl7v2.NewParser(source, parserConfig)
	if compiled != nil {
		parser.SetProfile(compiled)
	}
	result, err := parser.ParseWithResult(request.Message)
	if err != nil {
		response.OK = false
		response.Problems = []Problem{{Code: CodeParseFailed, Path: "message", Message: err.Error()}}
		response.Diagnostics = append(response.Diagnostics, failureDiagnostic(err.Error()))
		return response
	}

	response.Diagnostics = boundDiagnostics(normalizeDiagnostics(result.Warnings))
	payload, err := json.Marshal(result.Event)
	if err != nil {
		return failed(Problem{Code: CodeInternalError, Path: "$", Message: "the parsed event could not be encoded: " + err.Error()})
	}
	eventType, _ := fhirout.PayloadEventType(payload)
	response.Events = []Event{{Type: string(eventType), Payload: payload}}
	response.Bundle, response.BundleProblem = bundleFor(eventType, result.Event)
	return response
}

// ValidateProfile compiles a Source Profile with the production compiler and
// reports whether it is executable. Paths are relative to the profile.
func ValidateProfile(yamlText string) ValidateResponse {
	if strings.TrimSpace(yamlText) == "" {
		return ValidateResponse{Problems: []Problem{{Code: CodeProfileEmpty, Path: "$", Message: "the profile is empty"}}}
	}
	_, _, problems := compileProfile(yamlText, ProfileIdentity{ArtifactID: PlaygroundProfileID, Revision: 1}, "")
	if len(problems) > 0 {
		return ValidateResponse{Problems: problems}
	}
	return ValidateResponse{OK: true, Problems: []Problem{}}
}

func checkRequest(request PreviewRequest) []Problem {
	var problems []Problem
	switch {
	case strings.TrimSpace(request.Message) == "":
		problems = append(problems, Problem{Code: CodeMessageEmpty, Path: "message", Message: "the message is empty"})
	case int64(len(request.Message)) > MaxInputBytes:
		problems = append(problems, Problem{
			Code:    CodeMessageTooLarge,
			Path:    "message",
			Message: fmt.Sprintf("the message is %d bytes; the limit is %d", len(request.Message), MaxInputBytes),
		})
	}
	if request.Format != FormatHL7v2 {
		problems = append(problems, Problem{
			Code:    CodeFormatUnsupported,
			Path:    "format",
			Message: fmt.Sprintf("format %q is not supported; the kernel parses %q", request.Format, FormatHL7v2),
		})
	}
	if request.ProfileYAML != "" && strings.TrimSpace(request.ProfileYAML) == "" {
		problems = append(problems, Problem{Code: CodeProfileEmpty, Path: "profileYaml", Message: "the profile is empty; omit profileYaml to parse with the defaults"})
	}
	return problems
}

func loadTimezone(name string) (*time.Location, *Problem) {
	// processor's compiler refuses "Local" for the same reason: it means the
	// host's zone, which is not a property of the message or the profile.
	if name == "Local" {
		return nil, &Problem{Code: CodeTimezoneInvalid, Path: "timezone", Message: `"Local" is not a portable timezone; name an IANA zone such as "America/New_York"`}
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		return nil, &Problem{Code: CodeTimezoneInvalid, Path: "timezone", Message: fmt.Sprintf("%q is not an IANA timezone", name)}
	}
	return location, nil
}

func failed(problems ...Problem) PreviewResponse {
	return PreviewResponse{
		Segments:    []Segment{},
		Events:      []Event{},
		Diagnostics: []Diagnostic{},
		Problems:    problems,
	}
}

// segmentsOf is the parser's own tokenization, cut at MaxSegments; it also
// returns how many segments were cut. A message the parser cannot tokenize
// (no leading MSH) has no segments; its parse fails with the reason.
func segmentsOf(message string) ([]Segment, int) {
	split, err := hl7v2.SplitMessage(message)
	if err != nil {
		return []Segment{}, 0
	}
	kept := min(len(split.Segments), MaxSegments)
	segments := make([]Segment, 0, kept)
	for index, segment := range split.Segments[:kept] {
		segments = append(segments, Segment{Index: index, ID: segment.ID, Fields: segment.Fields})
	}
	return segments, len(split.Segments) - kept
}

// normalizeDiagnostics is session.NormalizeDiagnostics projected onto the
// fields the playground shows: the same defaults for an empty severity and
// code, in the same order.
func normalizeDiagnostics(warnings []events.ParseWarning) []Diagnostic {
	diagnostics := make([]Diagnostic, 0, len(warnings))
	for _, warning := range warnings {
		severity := warning.Severity
		if severity == "" {
			severity = "warning"
		}
		code := warning.Code
		if code == "" {
			code = "PARSE_WARNING"
		}
		diagnostics = append(diagnostics, Diagnostic{
			Severity: severity,
			Code:     code,
			Path:     warning.Path,
			Message:  warning.Message,
		})
	}
	return diagnostics
}

// failureDiagnostic is the one diagnostic session.Runner.finishFailed records.
func failureDiagnostic(message string) Diagnostic {
	return Diagnostic{Severity: "error", Code: CodeParseFailed, Path: "", Message: message}
}

func boundDiagnostics(diagnostics []Diagnostic) []Diagnostic {
	if len(diagnostics) <= MaxDiagnostics {
		return diagnostics
	}
	dropped := len(diagnostics) - MaxDiagnostics
	bounded := append([]Diagnostic(nil), diagnostics[:MaxDiagnostics]...)
	return append(bounded, Diagnostic{
		Severity: "info",
		Code:     codeDiagnosticsTruncated,
		Path:     "",
		Message:  fmt.Sprintf("%d more diagnostics are not shown", dropped),
	})
}

// bundleFor projects the event the way the durable engine does: the
// processor stores integration.NewProcessedEvent's raw-free canonical payload
// (source raw-data fields such as parse-warning excerpts stripped), and the
// `fhir` destination (internal/integration/destination/fhir_transport.go)
// runs fhirout.Project over that payload and builds the conditional
// transaction Bundle.
func bundleFor(eventType events.EventType, event any) (json.RawMessage, *Problem) {
	if !fhirout.Supports(eventType) {
		return nil, &Problem{
			Code:    CodeFHIRProjectionUnsupported,
			Path:    "events[0].type",
			Message: fmt.Sprintf("%q events have no FHIR projection", eventType),
		}
	}
	correlated, err := withCorrelationID(event)
	if err != nil {
		return nil, &Problem{Code: CodeFHIRProjectionFailed, Path: "events[0]", Message: err.Error()}
	}
	stored, err := integration.NewProcessedEvent(integration.ProcessedEventMetadata{
		TenantID:       playgroundTenantID,
		Classification: integration.DataClassificationPHI,
	}, correlated)
	if err != nil {
		return nil, &Problem{Code: CodeFHIRProjectionFailed, Path: "events[0]", Message: err.Error()}
	}
	projection, err := fhirout.Project(eventType, stored.PayloadJSON())
	if err != nil {
		return nil, &Problem{Code: CodeFHIRProjectionFailed, Path: "events[0]", Message: err.Error()}
	}
	bundle, err := fhirout.CreateConditionalTransactionBundle(projection)
	if err != nil {
		return nil, &Problem{Code: CodeFHIRProjectionFailed, Path: "events[0]", Message: err.Error()}
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		return nil, &Problem{Code: CodeFHIRProjectionFailed, Path: "events[0]", Message: "the Bundle could not be encoded: " + err.Error()}
	}
	return encoded, nil
}

// withCorrelationID returns a copy of a parsed event (a pointer to a
// pkg/events struct embedding EventMeta) whose correlation ID is set, because
// a stored canonical event requires one. The processor sets it from the
// request; the playground has no request, so the event's own ID stands in.
// The parsed event itself, and so the events[] the page shows, is unchanged.
func withCorrelationID(event any) (any, error) {
	pointer := reflect.ValueOf(event)
	if pointer.Kind() != reflect.Pointer || pointer.IsNil() || pointer.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("parsed event %T is not a pointer to an event struct", event)
	}
	clone := reflect.New(pointer.Elem().Type())
	clone.Elem().Set(pointer.Elem())
	meta := clone.Elem().FieldByName("EventMeta")
	if !meta.IsValid() || meta.Type() != reflect.TypeOf(events.EventMeta{}) {
		return nil, fmt.Errorf("parsed event %T has no event metadata", event)
	}
	current, _ := meta.Interface().(events.EventMeta)
	if current.CorrelationID == "" {
		current.CorrelationID = current.ID
		meta.Set(reflect.ValueOf(current))
	}
	return clone.Interface(), nil
}
