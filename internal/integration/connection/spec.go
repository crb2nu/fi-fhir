package connection

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strconv"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// kindSpec is one kind's decoded spec. check reports every field-level
// problem; bindingFields lists the spec's `*_binding` fields that are set.
type kindSpec interface {
	check(c *checker)
}

// newKindSpec returns a zero spec of the kind's type, or nil for an unknown
// kind.
func newKindSpec(kind Kind) kindSpec {
	switch kind {
	case KindMLLP:
		return &MLLPSpec{}
	case KindHTTP:
		return &HTTPSpec{}
	case KindBatchS3:
		return &BatchS3Spec{}
	case KindBatchSFTP:
		return &BatchSFTPSpec{}
	case KindHTTPS:
		return &HTTPSSpec{}
	case KindFHIR:
		return &FHIRSpec{}
	case KindKafka:
		return &KafkaSpec{}
	default:
		return nil
	}
}

// checker accumulates problems in the order they are found.
type checker struct {
	problems []Problem
}

func (c *checker) add(code, path, message string) {
	c.problems = append(c.problems, Problem{Code: code, Path: path, Message: message})
}

// CheckSpec decodes one kind's spec and checks it field by field against the
// document constructor's bounds and against the draft's secret bindings. It
// never returns an error: everything wrong with the input is a Problem.
//
// The returned problems are the same ones compile would report; compile
// proceeds only when none of them blocks (HasBlocking).
func CheckSpec(kind Kind, raw json.RawMessage, bindings []integration.SecretBinding) []Problem {
	_, problems := decodeAndCheck(kind, raw, bindings)
	return problems
}

// decodeAndCheck is CheckSpec that also returns the decoded spec. The spec is
// non-nil whenever the input was one JSON object of the right broad shape,
// even if field checks failed, so the checker test can hand it to a
// constructor and prove the constructor would have refused it too.
func decodeAndCheck(kind Kind, raw json.RawMessage, bindings []integration.SecretBinding) (kindSpec, []Problem) {
	c := &checker{}
	spec := newKindSpec(kind)
	if spec == nil {
		c.add(CodeInvalidEnum, "", "connection kind is not supported")
		return nil, c.problems
	}
	tree, ok := decodeSpecTree(raw, c)
	if !ok {
		checkBindings(bindings, nil, c)
		return nil, c.problems
	}

	structural := len(c.problems)
	scanSecretValues(tree, "", c)
	walkShape(tree, reflect.TypeOf(spec).Elem(), "", c)
	if len(c.problems) == structural {
		// No unknown key, no secret-looking key, and no type error: the
		// strict decoder is the authority on the typed value.
		strict := newKindSpec(kind)
		if err := strictDecode(raw, strict); err != nil {
			c.add(CodeInvalidJSON, "", "spec is not a valid "+string(kind)+" document")
		} else {
			spec = strict
		}
	} else {
		// Type errors were reported with their paths above; decode what can be
		// decoded so the field checks still describe everything else.
		_ = json.Unmarshal(raw, spec)
	}
	spec.check(c)
	checkBindings(bindings, bindingFieldValues(tree), c)
	return spec, dropShadowedProblems(c.problems)
}

// dropShadowedProblems removes the follow-on finding at a path whose value
// had the wrong JSON type: `"timeouts": "5"` is one mistake, reported once as
// INVALID_TYPE rather than again as a missing object.
func dropShadowedProblems(problems []Problem) []Problem {
	typed := make(map[string]struct{})
	for _, problem := range problems {
		if problem.Code == CodeInvalidType {
			typed[problem.Path] = struct{}{}
		}
	}
	if len(typed) == 0 {
		return problems
	}
	kept := make([]Problem, 0, len(problems))
	for _, problem := range problems {
		if _, shadowed := typed[problem.Path]; shadowed && problem.Code != CodeInvalidType {
			continue
		}
		kept = append(kept, problem)
	}
	return kept
}

// decodeSpecTree parses the spec into a generic JSON tree, refusing anything
// but one JSON object with unique keys.
func decodeSpecTree(raw json.RawMessage, c *checker) (map[string]any, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		c.add(CodeRequired, "", "spec is required")
		return nil, false
	}
	if len(trimmed) > MaxSpecBytes {
		c.add(CodeOutOfRange, "", fmt.Sprintf("spec must be at most %d bytes", MaxSpecBytes))
		return nil, false
	}
	if path, err := duplicateJSONKey(trimmed); err != nil {
		if path != "" {
			c.add(CodeInvalidJSON, path, "key is repeated")
		} else {
			c.add(CodeInvalidJSON, "", "spec is not valid JSON")
		}
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		c.add(CodeInvalidJSON, "", "spec is not valid JSON")
		return nil, false
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		c.add(CodeInvalidJSON, "", "spec must be exactly one JSON object")
		return nil, false
	}
	tree, ok := value.(map[string]any)
	if !ok {
		c.add(CodeInvalidJSON, "", "spec must be a JSON object")
		return nil, false
	}
	return tree, true
}

// strictDecode is the pkg/integration/revision.go idiom: unknown fields and
// trailing values are errors.
func strictDecode(raw json.RawMessage, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON value")
	}
	return nil
}

// duplicateJSONKey walks raw token by token and returns the dot path of the
// first repeated object key. A decoder that silently keeps the last duplicate
// would let two readers of the same bytes disagree about what they say.
func duplicateJSONKey(raw []byte) (string, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var walk func(path string) (string, error)
	walk = func(path string) (string, error) {
		token, err := decoder.Token()
		if err != nil {
			return "", err
		}
		delimiter, composite := token.(json.Delim)
		if !composite {
			return "", nil
		}
		switch delimiter {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return "", err
				}
				key, ok := keyToken.(string)
				if !ok {
					return "", errors.New("object member name is not a string")
				}
				child := joinPath(path, key)
				if _, duplicate := seen[key]; duplicate {
					return child, errors.New("duplicate JSON key")
				}
				seen[key] = struct{}{}
				if found, err := walk(child); err != nil {
					return found, err
				}
			}
			_, err := decoder.Token()
			return "", err
		case '[':
			index := 0
			for decoder.More() {
				if found, err := walk(fmt.Sprintf("%s[%d]", path, index)); err != nil {
					return found, err
				}
				index++
			}
			_, err := decoder.Token()
			return "", err
		default:
			return "", errors.New("unexpected JSON delimiter")
		}
	}
	return walk("")
}

// secretKeyFragments are the word pieces that make a key look like it holds a
// value rather than name a binding.
var secretKeyFragments = []string{
	"token", "password", "passwd", "secret", "passphrase",
	"private_key", "privatekey", "api_key", "apikey", "credential",
}

// secretLookingKey reports whether a spec key's name suggests it carries a
// secret value. A `*_binding` key names a binding and is always allowed.
func secretLookingKey(key string) bool {
	lower := strings.ToLower(key)
	if strings.HasSuffix(lower, "_binding") {
		return false
	}
	for _, fragment := range secretKeyFragments {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

// pemMarker is how a pasted certificate or private key announces itself.
const pemMarker = "-----BEGIN"

// scanSecretValues walks the whole tree — including subtrees under unknown
// keys, because a draft persists those — and reports every key whose name
// suggests a secret value and every string that carries PEM material.
func scanSecretValues(value any, path string, c *checker) {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range sortedKeys(typed) {
			child := joinPath(path, key)
			if secretLookingKey(key) {
				c.add(CodeSecretValueForbidden, child,
					"a spec never carries a secret value; name a secret binding in a *_binding field instead")
				continue
			}
			scanSecretValues(typed[key], child, c)
		}
	case []any:
		for index, element := range typed {
			scanSecretValues(element, fmt.Sprintf("%s[%d]", path, index), c)
		}
	case string:
		if strings.Contains(typed, pemMarker) {
			c.add(CodeSecretValueForbidden, path,
				"a spec never carries certificate or key material; name a secret binding instead")
		}
	}
}

// SecretValueProblems is the write-time half of the secret rule: the problems
// a draft write refuses (SpecError). It is empty for a spec that is not even
// JSON — that is a compile problem, and it cannot carry a recognisable key.
func SecretValueProblems(raw json.RawMessage) []Problem {
	c := &checker{}
	scratch := &checker{}
	tree, ok := decodeSpecTree(raw, scratch)
	if !ok {
		return nil
	}
	scanSecretValues(tree, "", c)
	return c.problems
}

// walkShape compares the generic tree against the spec type's JSON field set
// and reports every unknown key and every value of the wrong JSON type, with
// its path. Secret-looking keys were already reported by scanSecretValues and
// are not reported twice.
func walkShape(value any, typ reflect.Type, path string, c *checker) {
	if value == nil {
		return // JSON null reads as absent
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			c.add(CodeInvalidType, path, "must be an object")
			return
		}
		fields := jsonFields(typ)
		for _, key := range sortedKeys(object) {
			child := joinPath(path, key)
			field, known := fields[key]
			if !known {
				if !secretLookingKey(key) {
					c.add(CodeUnknownField, child, "is not a field of this connection kind")
				}
				continue
			}
			walkShape(object[key], field, child, c)
		}
	case reflect.Slice:
		list, ok := value.([]any)
		if !ok {
			c.add(CodeInvalidType, path, "must be a list")
			return
		}
		for index, element := range list {
			walkShape(element, typ.Elem(), fmt.Sprintf("%s[%d]", path, index), c)
		}
	case reflect.String:
		if _, ok := value.(string); !ok {
			c.add(CodeInvalidType, path, "must be a string")
		}
	case reflect.Bool:
		if _, ok := value.(bool); !ok {
			c.add(CodeInvalidType, path, "must be true or false")
		}
	case reflect.Int, reflect.Int64:
		number, ok := value.(json.Number)
		if !ok {
			c.add(CodeInvalidType, path, "must be a whole number")
			return
		}
		if _, err := strconv.ParseInt(number.String(), 10, 64); err != nil {
			c.add(CodeInvalidType, path, "must be a whole number")
		}
	}
}

// jsonFields maps a struct's JSON member names to their field types.
func jsonFields(typ reflect.Type) map[string]reflect.Type {
	fields := make(map[string]reflect.Type, typ.NumField())
	for index := 0; index < typ.NumField(); index++ {
		field := typ.Field(index)
		name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
		if name == "" || name == "-" {
			continue
		}
		fields[name] = field.Type
	}
	return fields
}

func sortedKeys(object map[string]any) []string {
	keys := make([]string, 0, len(object))
	for key := range object {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// bindingField is one set `*_binding` member of a spec.
type bindingField struct {
	path string
	name string
}

// bindingFieldValues collects every `*_binding` member that holds a non-empty
// string, wherever it sits in the tree. Collecting by suffix rather than by a
// per-kind list keeps the rule true for a field a later kind adds.
func bindingFieldValues(tree map[string]any) []bindingField {
	var fields []bindingField
	var walk func(value any, path string)
	walk = func(value any, path string) {
		switch typed := value.(type) {
		case map[string]any:
			for _, key := range sortedKeys(typed) {
				child := joinPath(path, key)
				if name, ok := typed[key].(string); ok && strings.HasSuffix(key, "_binding") && name != "" {
					fields = append(fields, bindingField{path: child, name: name})
					continue
				}
				walk(typed[key], child)
			}
		case []any:
			for index, element := range typed {
				walk(element, fmt.Sprintf("%s[%d]", path, index))
			}
		}
	}
	walk(tree, "")
	return fields
}

// checkBindings validates the binding references themselves and the two
// cross rules: every `*_binding` field names a binding (UNBOUND_SECRET), and
// every binding is named by some field (UNUSED_BINDING, a warning).
func checkBindings(bindings []integration.SecretBinding, fields []bindingField, c *checker) {
	if len(bindings) > MaxSecretBindings {
		c.add(CodeOutOfRange, "secret_bindings", fmt.Sprintf("at most %d secret bindings", MaxSecretBindings))
	}
	names := make(map[string]struct{}, len(bindings))
	for index, binding := range bindings {
		path := fmt.Sprintf("secret_bindings[%d]", index)
		switch {
		case binding.Name == "":
			c.add(CodeRequired, path+".name", "binding name is required")
		case !validIdentity(binding.Name):
			c.add(CodeInvalidValue, path+".name", "binding name must be at most 256 characters with no whitespace")
		default:
			if _, duplicate := names[binding.Name]; duplicate {
				c.add(CodeDuplicate, path+".name", "binding name is repeated")
			}
			names[binding.Name] = struct{}{}
		}
		switch binding.Reference.Provider {
		case integration.SecretProviderEnvironment, integration.SecretProviderFile, integration.SecretProviderVault,
			integration.SecretProviderAWSSSM, integration.SecretProviderKubernetes:
		case "":
			c.add(CodeRequired, path+".provider", "provider is required")
		default:
			c.add(CodeInvalidEnum, path+".provider", "provider must be env, file, vault, aws-ssm, or k8s")
		}
		switch {
		case binding.Reference.Key == "":
			c.add(CodeRequired, path+".key", "key is required")
		case !validSecretToken(binding.Reference.Key):
			c.add(CodeInvalidValue, path+".key", "key must be at most 256 characters with no whitespace")
		}
		if binding.Reference.Version != "" && !validSecretToken(binding.Reference.Version) {
			c.add(CodeInvalidValue, path+".version", "version must be at most 256 characters with no whitespace")
		}
	}
	used := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		used[field.name] = struct{}{}
		if _, bound := names[field.name]; !bound {
			c.add(CodeUnboundSecret, field.path, fmt.Sprintf("names secret binding %q, which is not declared", field.name))
		}
	}
	for index, binding := range bindings {
		if binding.Name == "" {
			continue
		}
		if _, isUsed := used[binding.Name]; !isUsed {
			c.add(CodeUnusedBinding, fmt.Sprintf("secret_bindings[%d].name", index),
				fmt.Sprintf("binding %q is not named by any *_binding field", binding.Name))
		}
	}
}

func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}
