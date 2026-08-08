package operator

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

// canonicalKeyPattern matches the engine-authored field names of a canonical
// event document. Anything outside it is treated as caller-influenced data and
// is collapsed so a dynamic map key can never become an output value.
var canonicalKeyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

const redactedKey = "*"

// summarizePayload renders a canonical event payload as structure only.
//
// This is the whole of "policy-aware semantic payload rendering" for Slice
// 4.2a: an operator learns which fields a message carried and their JSON
// shape, and never learns a stored value. Scalars contribute their kind, never
// their content or length. Object keys are emitted only when they match the
// canonical field grammar; every other key is replaced by "*". Arrays collapse
// to one repeated entry so element cardinality cannot leak either.
func summarizePayload(raw []byte) ([]PayloadField, bool) {
	if len(raw) == 0 {
		return []PayloadField{}, false
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		return []PayloadField{}, false
	}
	collector := &payloadCollector{seen: make(map[string]PayloadField, MaxPayloadFields)}
	collector.walk("", document, false, 0)
	paths := make([]string, 0, len(collector.seen))
	for path := range collector.seen {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	truncated := collector.truncated
	if len(paths) > MaxPayloadFields {
		paths = paths[:MaxPayloadFields]
		truncated = true
	}
	fields := make([]PayloadField, 0, len(paths))
	for _, path := range paths {
		fields = append(fields, collector.seen[path])
	}
	return fields, truncated
}

type payloadCollector struct {
	seen      map[string]PayloadField
	truncated bool
}

const maxPayloadDepth = 12

func (c *payloadCollector) walk(path string, node any, repeated bool, depth int) {
	if depth > maxPayloadDepth {
		c.truncated = true
		return
	}
	switch typed := node.(type) {
	case map[string]any:
		c.record(path, "object", repeated)
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			c.walk(joinPath(path, safeKey(key)), typed[key], repeated, depth+1)
		}
	case []any:
		c.record(path, "array", repeated)
		for _, element := range typed {
			c.walk(path, element, true, depth+1)
		}
	case string:
		c.record(path, "string", repeated)
	case float64:
		c.record(path, "number", repeated)
	case bool:
		c.record(path, "boolean", repeated)
	default:
		c.record(path, "null", repeated)
	}
}

func (c *payloadCollector) record(path, kind string, repeated bool) {
	if path == "" {
		return
	}
	if len(c.seen) >= MaxPayloadFields {
		if _, exists := c.seen[path]; !exists {
			c.truncated = true
			return
		}
	}
	existing, exists := c.seen[path]
	if !exists {
		c.seen[path] = PayloadField{Path: path, Kind: kind, Repeated: repeated}
		return
	}
	// A field observed with more than one shape reports the container shape it
	// was reached through rather than inventing a union type.
	if existing.Kind != kind && existing.Kind != "array" && kind == "array" {
		existing.Kind = "array"
	}
	existing.Repeated = existing.Repeated || repeated
	c.seen[path] = existing
}

func safeKey(key string) string {
	if !canonicalKeyPattern.MatchString(key) {
		return redactedKey
	}
	return key
}

func joinPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

// boundedDetail keeps an audit detail document small and value-free enough to
// be returned to an operator. Detail documents are engine-authored classifiers
// such as a failure code and a schedule timestamp.
func boundedDetail(raw []byte) map[string]any {
	detail := make(map[string]any)
	if len(raw) == 0 {
		return detail
	}
	if err := json.Unmarshal(raw, &detail); err != nil {
		return map[string]any{}
	}
	if len(detail) > 32 {
		return map[string]any{}
	}
	for key, value := range detail {
		if !canonicalKeyPattern.MatchString(key) {
			delete(detail, key)
			continue
		}
		if text, ok := value.(string); ok && len(text) > 512 {
			detail[key] = strings.TrimSpace(text[:512])
		}
	}
	return detail
}
