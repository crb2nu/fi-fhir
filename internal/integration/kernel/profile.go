package kernel

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/profile"
)

// ProfileJSON converts a Source Profile written as YAML (or JSON, which is
// YAML) into the JSON document the processor's compiler and a session's
// mapping-profile draft store. It accepts exactly one YAML document whose
// values are JSON values: string-keyed mappings, sequences, strings, numbers,
// booleans, and null.
func ProfileJSON(yamlText string) ([]byte, error) {
	decoder := yaml.NewDecoder(strings.NewReader(yamlText))
	var document any
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("the profile is empty")
		}
		return nil, err
	}
	var next any
	if err := decoder.Decode(&next); !errors.Is(err, io.EOF) {
		return nil, errors.New("the profile must be a single YAML document")
	}
	value, err := jsonValue(document, "$")
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}

func jsonValue(value any, path string) (any, error) {
	switch typed := value.(type) {
	case nil, string, bool, int, int64, uint64, float64:
		return typed, nil
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, member := range typed {
			converted, err := jsonValue(member, path+"."+key)
			if err != nil {
				return nil, err
			}
			out[key] = converted
		}
		return out, nil
	case []any:
		out := make([]any, len(typed))
		for index, member := range typed {
			converted, err := jsonValue(member, fmt.Sprintf("%s[%d]", path, index))
			if err != nil {
				return nil, err
			}
			out[index] = converted
		}
		return out, nil
	case map[any]any:
		return nil, fmt.Errorf("%s: mapping keys must be strings", path)
	default:
		return nil, fmt.Errorf("%s: %T is not a JSON value; quote it", path, value)
	}
}

// compileProfile runs the production compiler over one profile. prefix is
// prepended to every problem path ("profileYaml" inside a preview request,
// empty for fiFhirValidateProfile).
func compileProfile(
	yamlText string,
	identity ProfileIdentity,
	prefix string,
) (*profile.SourceProfile, *time.Location, []Problem) {
	at := func(path string) string {
		switch {
		case prefix == "":
			return path
		case path == "$":
			return prefix
		default:
			return prefix + "." + path
		}
	}
	if int64(len(yamlText)) > MaxInputBytes {
		return nil, nil, []Problem{{
			Code:    CodeProfileTooLarge,
			Path:    at("$"),
			Message: fmt.Sprintf("the profile is %d bytes; the limit is %d", len(yamlText), MaxInputBytes),
		}}
	}
	raw, err := ProfileJSON(yamlText)
	if err != nil {
		return nil, nil, []Problem{{Code: CodeProfileSyntax, Path: at("$"), Message: "the profile is not valid YAML: " + err.Error()}}
	}
	ref, err := processor.NewProfileRevisionReference(identity.ArtifactID, identity.Revision, raw)
	if err != nil {
		return nil, nil, []Problem{{Code: CodeProfileInvalid, Path: at("$"), Message: err.Error()}}
	}
	compiled, timezone, err := processor.CompileProfileRevision(ref, raw)
	if err != nil {
		return nil, nil, []Problem{compileProblem(err, at)}
	}
	return compiled, timezone, nil
}

// compileProblem maps the compiler's two sentinels to problem codes. The
// compiler's errors read "<sentinel>: <path>", and the path is the document
// path of the offending member.
func compileProblem(err error, at func(string) string) Problem {
	code := CodeProfileInvalid
	sentinel := processor.ErrInvalidSourceProfile
	if errors.Is(err, processor.ErrUnsupportedSourceProfile) {
		code = CodeProfileUnsupported
		sentinel = processor.ErrUnsupportedSourceProfile
	}
	path := "$"
	if rest, found := strings.CutPrefix(err.Error(), sentinel.Error()+": "); found && rest != "" {
		path = rest
	}
	return Problem{Code: code, Path: at(path), Message: err.Error()}
}
