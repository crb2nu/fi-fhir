package connection

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// specFixture is one testdata/<kind>.json file: a catalog-valid spec and the
// bindings it names. Every value in it is synthetic.
type specFixture struct {
	Kind           Kind                        `json:"kind"`
	Spec           map[string]any              `json:"spec"`
	SecretBindings []integration.SecretBinding `json:"secret_bindings"`
}

func loadSpecFixture(t *testing.T, kind Kind) specFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", string(kind)+".json"))
	if err != nil {
		t.Fatalf("read %s fixture: %v", kind, err)
	}
	var fixture specFixture
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatalf("decode %s fixture: %v", kind, err)
	}
	if fixture.Kind != kind {
		t.Fatalf("fixture %s declares kind %q", kind, fixture.Kind)
	}
	return fixture
}

// clone deep-copies a fixture so a mutation cannot leak into its siblings.
func (f specFixture) clone(t *testing.T) specFixture {
	t.Helper()
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	var clone specFixture
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&clone); err != nil {
		t.Fatalf("clone fixture: %v", err)
	}
	return clone
}

func (f specFixture) specJSON(t *testing.T) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(f.Spec)
	if err != nil {
		t.Fatalf("marshal spec: %v", err)
	}
	return raw
}

func (f specFixture) draft(t *testing.T, id string) Draft {
	t.Helper()
	direction, _ := f.Kind.Direction()
	return Draft{
		TenantID: "tenant-a", ID: id, Direction: direction, Kind: f.Kind,
		Name: "fixture " + string(f.Kind), Spec: f.specJSON(t),
		SecretBindings: append([]integration.SecretBinding(nil), f.SecretBindings...),
		Version:        1,
	}
}

var pathSegment = regexp.MustCompile(`([^.\[\]]+)|\[(\d+)\]`)

// resolveParent walks a dot/index path to the container of its last segment.
func resolveParent(t *testing.T, root map[string]any, path string) (any, string) {
	t.Helper()
	matches := pathSegment.FindAllStringSubmatch(path, -1)
	if len(matches) == 0 {
		t.Fatalf("empty path")
	}
	var current any = root
	for _, match := range matches[:len(matches)-1] {
		current = step(t, current, match, path, true)
	}
	return current, matches[len(matches)-1][0]
}

func step(t *testing.T, current any, match []string, path string, create bool) any {
	t.Helper()
	if match[1] != "" {
		object, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("%s: %q is not inside an object", path, match[1])
		}
		child, exists := object[match[1]]
		if !exists && create {
			child = map[string]any{}
			object[match[1]] = child
		}
		return child
	}
	index, _ := strconv.Atoi(match[2])
	list, ok := current.([]any)
	if !ok || index >= len(list) {
		t.Fatalf("%s: index %d is not in a list", path, index)
	}
	return list[index]
}

// setPath sets one member, creating intermediate objects.
func setPath(t *testing.T, root map[string]any, path string, value any) {
	t.Helper()
	parent, last := resolveParent(t, root, path)
	if strings.HasPrefix(last, "[") {
		index, _ := strconv.Atoi(strings.Trim(last, "[]"))
		list, ok := parent.([]any)
		if !ok || index >= len(list) {
			t.Fatalf("%s: not a list index", path)
		}
		list[index] = value
		return
	}
	object, ok := parent.(map[string]any)
	if !ok {
		t.Fatalf("%s: parent is not an object", path)
	}
	object[last] = value
}

// deletePath removes one object member.
func deletePath(t *testing.T, root map[string]any, path string) {
	t.Helper()
	parent, last := resolveParent(t, root, path)
	object, ok := parent.(map[string]any)
	if !ok {
		t.Fatalf("%s: parent is not an object", path)
	}
	if _, exists := object[last]; !exists {
		t.Fatalf("%s: nothing to delete", path)
	}
	delete(object, last)
}

// Small mutation builders for the table in checker_test.go.

// set assigns a deep copy of value, so a map or list literal shared by several
// table rows is never mutated by a later row.
func set(path string, value any) func(*testing.T, *specFixture) {
	return func(t *testing.T, f *specFixture) {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("marshal value for %s: %v", path, err)
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.UseNumber()
		var copied any
		if err := decoder.Decode(&copied); err != nil {
			t.Fatalf("copy value for %s: %v", path, err)
		}
		setPath(t, f.Spec, path, copied)
	}
}

func del(paths ...string) func(*testing.T, *specFixture) {
	return func(t *testing.T, f *specFixture) {
		for _, path := range paths {
			deletePath(t, f.Spec, path)
		}
	}
}

func all(mutations ...func(*testing.T, *specFixture)) func(*testing.T, *specFixture) {
	return func(t *testing.T, f *specFixture) {
		for _, mutation := range mutations {
			mutation(t, f)
		}
	}
}

func bindings(list ...integration.SecretBinding) func(*testing.T, *specFixture) {
	return func(_ *testing.T, f *specFixture) { f.SecretBindings = list }
}

func binding(name string) integration.SecretBinding {
	return integration.SecretBinding{
		Name:      name,
		Reference: integration.SecretReference{Provider: integration.SecretProviderFile, Key: "fixtures/" + name},
	}
}

func repeat(prefix string, count int) []any {
	values := make([]any, 0, count)
	for index := 0; index < count; index++ {
		values = append(values, fmt.Sprintf("%s-%d", prefix, index))
	}
	return values
}

func cidrs(count int) []any {
	values := make([]any, 0, count)
	for index := 0; index < count; index++ {
		values = append(values, fmt.Sprintf("10.%d.%d.0/24", index/256, index%256))
	}
	return values
}

func problemsAt(problems []Problem, path string) []Problem {
	var matched []Problem
	for _, problem := range problems {
		if problem.Path == path {
			matched = append(matched, problem)
		}
	}
	return matched
}

func blockingAt(problems []Problem, path string) bool {
	for _, problem := range problemsAt(problems, path) {
		if problem.Blocking() {
			return true
		}
	}
	return false
}

func hasCode(problems []Problem, code, path string) bool {
	for _, problem := range problems {
		if problem.Code == code && problem.Path == path {
			return true
		}
	}
	return false
}
