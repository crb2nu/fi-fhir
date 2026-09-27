package connection

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

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
		return nil, dropShadowedProblems(c.problems)
	}

	structural := len(c.problems)
	inspection{c: c}.value(tree, reflect.TypeOf(spec).Elem(), "")
	if len(c.problems) == structural {
		// No unknown key, no secret material, and no type error: the strict
		// decoder is the authority on the typed value.
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

// writeProblems is the write-time gate: what a draft may never persist,
// whatever state the rest of its spec is in. A draft may be incomplete — a
// missing field, a value out of range, or a scalar of the wrong type is a
// compile problem — but it never stores a key its kind does not define (not
// even inside a container of the wrong type), secret material in any form, a
// malformed secret binding reference, or a `*_binding` value that does not
// name one of the draft's own declared bindings: a string in a binding field
// that names nothing is indistinguishable from a pasted credential. Every
// problem it returns is one CheckSpec would report too, so validate shows
// everything a write refuses.
func writeProblems(kind Kind, tree map[string]any, bindings []integration.SecretBinding) []Problem {
	c := &checker{}
	spec := newKindSpec(kind)
	if spec == nil {
		c.add(CodeInvalidEnum, "", "connection kind is not supported")
		return c.problems
	}
	inspection{c: c, write: true}.value(tree, reflect.TypeOf(spec).Elem(), "")
	names := checkBindingReferences(bindings, c)
	checkBoundFields(bindingFieldValues(tree), names, c)
	return dropShadowedProblems(c.problems)
}

// dropShadowedProblems keeps one finding per path: the most specific one.
// Secret material outranks everything else at its path — a URL carrying a
// token is refused for the token, not also for its query — and a value of
// the wrong JSON type outranks what follows from it: `"timeouts": "5"` is
// one mistake, reported once as INVALID_TYPE rather than again as a missing
// object.
func dropShadowedProblems(problems []Problem) []Problem {
	rank := func(code string) int {
		switch code {
		case CodeSecretValueForbidden:
			return 2
		case CodeInvalidType:
			return 1
		default:
			return 0
		}
	}
	top := make(map[string]int)
	for _, problem := range problems {
		if level := rank(problem.Code); level > top[problem.Path] {
			top[problem.Path] = level
		}
	}
	if len(top) == 0 {
		return problems
	}
	kept := make([]Problem, 0, len(problems))
	for _, problem := range problems {
		if rank(problem.Code) < top[problem.Path] {
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

// secretKeyFragments are the word pieces that make a key, or a URL query
// parameter, look like it holds a value rather than name a binding. They are
// matched against normalizeKey's spelling, so `X-Api-Key`, `x_api_key`, and
// `xApiKey` all read as x_api_key.
var secretKeyFragments = []string{
	"token", "password", "passwd", "pwd", "secret", "passphrase",
	"private_key", "privatekey", "api_key", "apikey", "access_key", "accesskey",
	"credential", "bearer", "authorization", "auth",
}

// normalizeKey spells a key or parameter name the one way the fragment lists
// are written: lower case, with `-` and camelCase word breaks as `_`.
func normalizeKey(name string) string {
	var normalized strings.Builder
	var previous rune
	for _, character := range name {
		switch {
		case character == '-':
			normalized.WriteByte('_')
		case unicode.IsUpper(character):
			if unicode.IsLower(previous) || unicode.IsDigit(previous) {
				normalized.WriteByte('_')
			}
			normalized.WriteRune(unicode.ToLower(character))
		default:
			normalized.WriteRune(character)
		}
		previous = character
	}
	return normalized.String()
}

// bindingKey reports whether a key names a secret binding: `*_binding`.
func bindingKey(key string) bool {
	return strings.HasSuffix(normalizeKey(key), "_binding")
}

func containsSecretFragment(normalized string) bool {
	for _, fragment := range secretKeyFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}

// secretLookingKey reports whether the name of a key the kind does not define
// suggests it carries a secret value. A `*_binding` key names a binding and
// is judged by its value instead (holdsMoreThanAName). A key the kind does
// define is never judged by its name: `auth_mode` and `oauth` are settings.
func secretLookingKey(key string) bool {
	return !bindingKey(key) && containsSecretFragment(normalizeKey(key))
}

// holdsMoreThanAName reports whether a `*_binding` member carries anything but
// a binding name. JSON null reads as absent.
func holdsMoreThanAName(value any) bool {
	switch value.(type) {
	case nil, string:
		return false
	default:
		return true
	}
}

// pemMarker is how a pasted certificate or private key announces itself.
const pemMarker = "-----BEGIN"

// secretParameterName reports whether a URL query or fragment parameter's
// name says it carries a credential: any secret key fragment, a signature
// (AWS and GCS signed URLs, Azure SAS `sig`), or a key (`key`, `*_key`).
func secretParameterName(name string) bool {
	normalized := normalizeKey(name)
	switch {
	case normalized == "key", normalized == "sig", normalized == "signature",
		strings.HasSuffix(normalized, "_key"), strings.HasSuffix(normalized, "_sig"),
		strings.Contains(normalized, "signature"):
		return true
	}
	return containsSecretFragment(normalized)
}

// urlSchemePattern is RFC 3986's scheme production.
var urlSchemePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*$`)

// urlSecretProblem returns why a string that is an absolute URL carries a
// credential — userinfo, or a query or fragment parameter secretParameterName
// flags — or "" when it does not. It splits the URL by hand rather than with
// url.Parse, so a URL that does not parse (a bad escape) cannot smuggle a
// token past it. Any other query parameter is data and stays allowed.
func urlSecretProblem(value string) string {
	scheme, rest, found := strings.Cut(value, "://")
	if !found || !urlSchemePattern.MatchString(scheme) {
		return ""
	}
	authority := rest
	if end := strings.IndexAny(rest, "/?#"); end >= 0 {
		authority = rest[:end]
	}
	if strings.Contains(authority, "@") {
		return "a URL never carries credentials; name a secret binding instead"
	}
	var query, fragment string
	if before, after, hasFragment := strings.Cut(rest, "#"); hasFragment {
		rest, fragment = before, after
	}
	if _, after, hasQuery := strings.Cut(rest, "?"); hasQuery {
		query = after
	}
	for _, parameters := range []string{query, fragment} {
		for _, pair := range strings.FieldsFunc(parameters, func(r rune) bool { return r == '&' || r == ';' }) {
			name, _, _ := strings.Cut(pair, "=")
			if unescaped, err := url.QueryUnescape(name); err == nil {
				name = unescaped
			}
			if secretParameterName(name) {
				return "a URL never carries a key, token, or signature in its query; name a secret binding instead"
			}
		}
	}
	return ""
}

// inspection is one walk of a spec's generic JSON tree against its kind's
// type. It reports every key the kind does not define (UNKNOWN_FIELD), every
// value of the wrong JSON type (INVALID_TYPE), and secret material wherever
// it sits (SECRET_VALUE_FORBIDDEN): a key the kind does not define whose name
// suggests a value, a `*_binding` member holding more than a name, PEM
// material in any string, and a URL carrying a credential. A write
// inspection reports only what a draft may never persist: a value of the
// wrong type is refused there only when it is a container, because the
// catalog cannot vouch for the keys inside it.
type inspection struct {
	c     *checker
	write bool
}

// value inspects one value against typ. typ is nil inside a value the kind
// does not describe — under an unknown key, or inside a container of the
// wrong type — where only secret material is looked for.
func (in inspection) value(value any, typ reflect.Type, path string) {
	if value == nil {
		return // JSON null reads as absent
	}
	if text, ok := value.(string); ok {
		in.secretString(text, path)
	}
	if typ == nil {
		in.undescribed(value, path)
		return
	}
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			in.mismatch(value, path, "must be an object")
			return
		}
		in.object(object, jsonFields(typ), path)
	case reflect.Slice:
		list, ok := value.([]any)
		if !ok {
			in.mismatch(value, path, "must be a list")
			return
		}
		for index, element := range list {
			in.value(element, typ.Elem(), fmt.Sprintf("%s[%d]", path, index))
		}
	case reflect.String:
		if _, ok := value.(string); !ok {
			in.mismatch(value, path, "must be a string")
		}
	case reflect.Bool:
		if _, ok := value.(bool); !ok {
			in.mismatch(value, path, "must be true or false")
		}
	case reflect.Int, reflect.Int64:
		number, ok := value.(json.Number)
		if ok {
			_, err := strconv.ParseInt(number.String(), 10, 64)
			ok = err == nil
		}
		if !ok {
			in.mismatch(value, path, "must be a whole number")
		}
	}
}

// object inspects the members of an object the kind describes.
func (in inspection) object(object map[string]any, fields map[string]reflect.Type, path string) {
	for _, key := range sortedKeys(object) {
		child := joinPath(path, key)
		member := object[key]
		field, known := fields[key]
		switch {
		case bindingKey(key) && holdsMoreThanAName(member):
			in.c.add(CodeSecretValueForbidden, child,
				"a *_binding field holds the name of a secret binding, never a value")
		case !known && secretLookingKey(key):
			in.c.add(CodeSecretValueForbidden, child,
				"a spec never carries a secret value; name a secret binding in a *_binding field instead")
		case !known:
			in.c.add(CodeUnknownField, child, "is not a field of this connection kind")
			in.value(member, nil, child)
		default:
			in.value(member, field, child)
		}
	}
}

// undescribed looks for secret material inside a value the kind does not
// describe. Its keys are not reported one by one: the unknown key or the
// wrong-typed value that holds them already was.
func (in inspection) undescribed(value any, path string) {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range sortedKeys(typed) {
			child := joinPath(path, key)
			member := typed[key]
			switch {
			case bindingKey(key) && holdsMoreThanAName(member):
				in.c.add(CodeSecretValueForbidden, child,
					"a *_binding field holds the name of a secret binding, never a value")
			case secretLookingKey(key):
				in.c.add(CodeSecretValueForbidden, child,
					"a spec never carries a secret value; name a secret binding in a *_binding field instead")
			default:
				in.value(member, nil, child)
			}
		}
	case []any:
		for index, element := range typed {
			in.value(element, nil, fmt.Sprintf("%s[%d]", path, index))
		}
	}
}

// mismatch reports a value of the wrong JSON type and looks inside it for
// secret material.
func (in inspection) mismatch(value any, path, message string) {
	_, isObject := value.(map[string]any)
	_, isList := value.([]any)
	if !in.write || isObject || isList {
		in.c.add(CodeInvalidType, path, message)
	}
	if isObject || isList {
		in.undescribed(value, path)
	}
}

// secretString reports PEM material or a credential-carrying URL.
func (in inspection) secretString(text, path string) {
	if strings.Contains(text, pemMarker) {
		in.c.add(CodeSecretValueForbidden, path,
			"a spec never carries certificate or key material; name a secret binding instead")
		return
	}
	if message := urlSecretProblem(text); message != "" {
		in.c.add(CodeSecretValueForbidden, path, message)
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
// every binding is named by some field (UNUSED_BINDING, a warning). A write
// refuses a malformed reference and an unbound field too; only the unused
// warning is compile's alone.
func checkBindings(bindings []integration.SecretBinding, fields []bindingField, c *checker) {
	names := checkBindingReferences(bindings, c)
	checkBoundFields(fields, names, c)
	used := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		used[field.name] = struct{}{}
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

// checkBoundFields reports every `*_binding` field whose value is not exactly
// the name of a declared binding. The message never repeats the value: a
// string that names no binding may be a credential pasted into the field.
func checkBoundFields(fields []bindingField, names map[string]struct{}, c *checker) {
	for _, field := range fields {
		if _, bound := names[field.name]; !bound {
			c.add(CodeUnboundSecret, field.path, "names a secret binding this draft does not declare")
		}
	}
}

// checkBindingReferences validates each secret binding on its own — the count,
// the name, the provider, the key and version tokens, and that none of them
// carries certificate or key material — and returns the valid names. Draft
// writes and compile both apply it.
func checkBindingReferences(bindings []integration.SecretBinding, c *checker) map[string]struct{} {
	if len(bindings) > MaxSecretBindings {
		c.add(CodeOutOfRange, "secret_bindings", fmt.Sprintf("at most %d secret bindings", MaxSecretBindings))
	}
	names := make(map[string]struct{}, len(bindings))
	for index, binding := range bindings {
		path := fmt.Sprintf("secret_bindings[%d]", index)
		for _, field := range []struct{ name, value string }{
			{"name", binding.Name}, {"key", binding.Reference.Key}, {"version", binding.Reference.Version},
		} {
			if strings.Contains(field.value, pemMarker) {
				c.add(CodeSecretValueForbidden, path+"."+field.name,
					"a binding names a secret; it never carries certificate or key material")
			}
		}
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
	return names
}

func joinPath(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}
