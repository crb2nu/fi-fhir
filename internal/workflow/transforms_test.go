package workflow

import (
	"context"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// mockTerminologyMapper implements TerminologyMapperInterface for testing.
type mockTerminologyMapper struct {
	mappings map[string][]CodeMappingResult
}

func (m *mockTerminologyMapper) Map(sourceSystem, sourceCode, targetSystem string) []CodeMappingResult {
	key := sourceSystem + ":" + sourceCode + ":" + targetSystem
	if result, ok := m.mappings[key]; ok {
		return result
	}
	return nil
}

func TestNewTransformer(t *testing.T) {
	t.Run("with nil mapper", func(t *testing.T) {
		transformer := NewTransformer(nil)
		if transformer == nil {
			t.Fatal("NewTransformer returned nil")
		}
		if transformer.terminologyMapper != nil {
			t.Error("terminologyMapper should be nil")
		}
	})

	t.Run("with mapper", func(t *testing.T) {
		mapper := &mockTerminologyMapper{}
		transformer := NewTransformer(mapper)
		if transformer == nil {
			t.Fatal("NewTransformer returned nil")
		}
		if transformer.terminologyMapper == nil {
			t.Error("terminologyMapper should not be nil")
		}
	})
}

func TestToMap(t *testing.T) {
	t.Run("from map", func(t *testing.T) {
		input := map[string]interface{}{
			"name": "John",
			"age":  30,
		}
		result, err := toMap(input)
		if err != nil {
			t.Fatalf("toMap error: %v", err)
		}
		if result["name"] != "John" {
			t.Errorf("name = %v, want John", result["name"])
		}
		// Verify it's a copy
		input["name"] = "Jane"
		if result["name"] != "John" {
			t.Error("toMap should return a copy, not modify original")
		}
	})

	t.Run("from struct", func(t *testing.T) {
		type Person struct {
			Name string `json:"name"`
			Age  int    `json:"age"`
		}
		input := Person{Name: "John", Age: 30}
		result, err := toMap(input)
		if err != nil {
			t.Fatalf("toMap error: %v", err)
		}
		if result["name"] != "John" {
			t.Errorf("name = %v, want John", result["name"])
		}
		// JSON unmarshal converts int to float64
		if result["age"] != float64(30) {
			t.Errorf("age = %v (%T), want 30", result["age"], result["age"])
		}
	})

	t.Run("with nested map", func(t *testing.T) {
		input := map[string]interface{}{
			"person": map[string]interface{}{
				"name": "John",
			},
		}
		result, err := toMap(input)
		if err != nil {
			t.Fatalf("toMap error: %v", err)
		}
		person, ok := result["person"].(map[string]interface{})
		if !ok {
			t.Fatal("person is not a map")
		}
		if person["name"] != "John" {
			t.Errorf("person.name = %v, want John", person["name"])
		}
		// Verify nested is also a copy
		originalPerson := input["person"].(map[string]interface{})
		originalPerson["name"] = "Jane"
		if person["name"] != "John" {
			t.Error("nested map should also be a copy")
		}
	})

	t.Run("with slice", func(t *testing.T) {
		input := map[string]interface{}{
			"items": []interface{}{"a", "b", "c"},
		}
		result, err := toMap(input)
		if err != nil {
			t.Fatalf("toMap error: %v", err)
		}
		items, ok := result["items"].([]interface{})
		if !ok {
			t.Fatal("items is not a slice")
		}
		if len(items) != 3 {
			t.Errorf("items length = %d, want 3", len(items))
		}
		// Verify slice is a copy
		originalItems := input["items"].([]interface{})
		originalItems[0] = "modified"
		if items[0] != "a" {
			t.Error("slice should also be a copy")
		}
	})

	t.Run("unmarshalable type", func(t *testing.T) {
		// Create a channel which cannot be marshaled to JSON
		input := make(chan int)
		_, err := toMap(input)
		if err == nil {
			t.Error("expected error for unmarshalable type")
		}
	})
}

func TestDeepCopyMap(t *testing.T) {
	t.Run("simple map", func(t *testing.T) {
		original := map[string]interface{}{
			"key": "value",
		}
		copied := deepCopyMap(original)

		// Modify original
		original["key"] = "modified"
		original["new"] = "added"

		if copied["key"] != "value" {
			t.Error("copy was modified when original changed")
		}
		if _, exists := copied["new"]; exists {
			t.Error("copy has keys that were added to original")
		}
	})

	t.Run("nested map", func(t *testing.T) {
		original := map[string]interface{}{
			"outer": map[string]interface{}{
				"inner": "value",
			},
		}
		copied := deepCopyMap(original)

		// Modify nested original
		original["outer"].(map[string]interface{})["inner"] = "modified"

		copiedOuter := copied["outer"].(map[string]interface{})
		if copiedOuter["inner"] != "value" {
			t.Error("nested copy was modified when original changed")
		}
	})

	t.Run("with slice", func(t *testing.T) {
		original := map[string]interface{}{
			"list": []interface{}{"a", "b"},
		}
		copied := deepCopyMap(original)

		// Modify original slice
		original["list"].([]interface{})[0] = "modified"

		copiedList := copied["list"].([]interface{})
		if copiedList[0] != "a" {
			t.Error("slice in copy was modified when original changed")
		}
	})

	t.Run("mixed types", func(t *testing.T) {
		original := map[string]interface{}{
			"string": "text",
			"number": 42,
			"bool":   true,
			"null":   nil,
		}
		copied := deepCopyMap(original)

		if copied["string"] != "text" {
			t.Errorf("string = %v", copied["string"])
		}
		if copied["number"] != 42 {
			t.Errorf("number = %v", copied["number"])
		}
		if copied["bool"] != true {
			t.Errorf("bool = %v", copied["bool"])
		}
		if copied["null"] != nil {
			t.Errorf("null = %v", copied["null"])
		}
	})
}

func TestDeepCopySlice(t *testing.T) {
	t.Run("simple slice", func(t *testing.T) {
		original := []interface{}{"a", "b", "c"}
		copied := deepCopySlice(original)

		original[0] = "modified"

		if copied[0] != "a" {
			t.Error("copy was modified when original changed")
		}
	})

	t.Run("slice with nested map", func(t *testing.T) {
		original := []interface{}{
			map[string]interface{}{"key": "value"},
		}
		copied := deepCopySlice(original)

		original[0].(map[string]interface{})["key"] = "modified"

		copiedMap := copied[0].(map[string]interface{})
		if copiedMap["key"] != "value" {
			t.Error("nested map in copy was modified")
		}
	})

	t.Run("slice with nested slice", func(t *testing.T) {
		original := []interface{}{
			[]interface{}{"inner1", "inner2"},
		}
		copied := deepCopySlice(original)

		original[0].([]interface{})[0] = "modified"

		copiedInner := copied[0].([]interface{})
		if copiedInner[0] != "inner1" {
			t.Error("nested slice in copy was modified")
		}
	})

	t.Run("empty slice", func(t *testing.T) {
		original := []interface{}{}
		copied := deepCopySlice(original)

		if len(copied) != 0 {
			t.Errorf("copied slice length = %d, want 0", len(copied))
		}
	})
}

func TestParseValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected interface{}
	}{
		{"double quoted string", `"hello"`, "hello"},
		{"single quoted string", `'hello'`, "hello"},
		{"true boolean", "true", true},
		{"True boolean", "True", true},
		{"TRUE boolean", "TRUE", true},
		{"false boolean", "false", false},
		{"False boolean", "False", false},
		{"null", "null", nil},
		{"nil", "nil", nil},
		{"integer", "42", int64(42)},
		{"negative integer", "-10", int64(-10)},
		{"float", "3.14", 3.14},
		{"negative float", "-2.5", -2.5},
		{"unquoted string", "hello", "hello"},
		{"string with spaces", "hello world", "hello world"},
		// Note: "42abc" parses as float 42 due to fmt.Sscanf behavior
		{"integer-like string with suffix", "42abc", float64(42)},
		{"pure text", "abc", "abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseValue(tt.input)
			if result != tt.expected {
				t.Errorf("parseValue(%q) = %v (%T), want %v (%T)",
					tt.input, result, result, tt.expected, tt.expected)
			}
		})
	}
}

func TestSetNestedValue(t *testing.T) {
	t.Run("simple path", func(t *testing.T) {
		m := map[string]interface{}{}
		err := setNestedValue(m, "name", "John")
		if err != nil {
			t.Fatalf("setNestedValue error: %v", err)
		}
		if m["name"] != "John" {
			t.Errorf("name = %v, want John", m["name"])
		}
	})

	t.Run("nested path creates intermediate maps", func(t *testing.T) {
		m := map[string]interface{}{}
		err := setNestedValue(m, "person.name", "John")
		if err != nil {
			t.Fatalf("setNestedValue error: %v", err)
		}
		person, ok := m["person"].(map[string]interface{})
		if !ok {
			t.Fatal("person is not a map")
		}
		if person["name"] != "John" {
			t.Errorf("person.name = %v, want John", person["name"])
		}
	})

	t.Run("deep nested path", func(t *testing.T) {
		m := map[string]interface{}{}
		err := setNestedValue(m, "a.b.c.d", "deep")
		if err != nil {
			t.Fatalf("setNestedValue error: %v", err)
		}
		a := m["a"].(map[string]interface{})
		b := a["b"].(map[string]interface{})
		c := b["c"].(map[string]interface{})
		if c["d"] != "deep" {
			t.Errorf("a.b.c.d = %v, want deep", c["d"])
		}
	})

	t.Run("overwrite existing value", func(t *testing.T) {
		m := map[string]interface{}{"name": "John"}
		err := setNestedValue(m, "name", "Jane")
		if err != nil {
			t.Fatalf("setNestedValue error: %v", err)
		}
		if m["name"] != "Jane" {
			t.Errorf("name = %v, want Jane", m["name"])
		}
	})

	t.Run("path through non-object fails", func(t *testing.T) {
		m := map[string]interface{}{"name": "John"}
		err := setNestedValue(m, "name.first", "John")
		if err == nil {
			t.Error("expected error when path goes through non-object")
		}
	})
}

func TestGetNestedValue(t *testing.T) {
	t.Run("simple path", func(t *testing.T) {
		m := map[string]interface{}{"name": "John"}
		result, err := getNestedValue(m, "name")
		if err != nil {
			t.Fatalf("getNestedValue error: %v", err)
		}
		if result != "John" {
			t.Errorf("result = %v, want John", result)
		}
	})

	t.Run("nested path", func(t *testing.T) {
		m := map[string]interface{}{
			"person": map[string]interface{}{
				"name": "John",
			},
		}
		result, err := getNestedValue(m, "person.name")
		if err != nil {
			t.Fatalf("getNestedValue error: %v", err)
		}
		if result != "John" {
			t.Errorf("result = %v, want John", result)
		}
	})

	t.Run("deep nested path", func(t *testing.T) {
		m := map[string]interface{}{
			"a": map[string]interface{}{
				"b": map[string]interface{}{
					"c": "deep",
				},
			},
		}
		result, err := getNestedValue(m, "a.b.c")
		if err != nil {
			t.Fatalf("getNestedValue error: %v", err)
		}
		if result != "deep" {
			t.Errorf("result = %v, want deep", result)
		}
	})

	t.Run("key not found", func(t *testing.T) {
		m := map[string]interface{}{}
		_, err := getNestedValue(m, "missing")
		if err == nil {
			t.Error("expected error for missing key")
		}
	})

	t.Run("nested key not found", func(t *testing.T) {
		m := map[string]interface{}{
			"person": map[string]interface{}{},
		}
		_, err := getNestedValue(m, "person.name")
		if err == nil {
			t.Error("expected error for missing nested key")
		}
	})

	t.Run("path through non-object", func(t *testing.T) {
		m := map[string]interface{}{"name": "John"}
		_, err := getNestedValue(m, "name.first")
		if err == nil {
			t.Error("expected error when path goes through non-object")
		}
	})
}

func TestDeleteNestedValue(t *testing.T) {
	t.Run("simple path", func(t *testing.T) {
		m := map[string]interface{}{"name": "John", "age": 30}
		deleteNestedValue(m, "name")
		if _, exists := m["name"]; exists {
			t.Error("name should be deleted")
		}
		if m["age"] != 30 {
			t.Error("age should remain")
		}
	})

	t.Run("nested path", func(t *testing.T) {
		m := map[string]interface{}{
			"person": map[string]interface{}{
				"name": "John",
				"age":  30,
			},
		}
		deleteNestedValue(m, "person.name")
		person := m["person"].(map[string]interface{})
		if _, exists := person["name"]; exists {
			t.Error("person.name should be deleted")
		}
		if person["age"] != 30 {
			t.Error("person.age should remain")
		}
	})

	t.Run("missing path is no-op", func(t *testing.T) {
		m := map[string]interface{}{"name": "John"}
		deleteNestedValue(m, "missing.path")
		// Should not panic or error
		if m["name"] != "John" {
			t.Error("name should remain unchanged")
		}
	})

	t.Run("path through non-object is no-op", func(t *testing.T) {
		m := map[string]interface{}{"name": "John"}
		deleteNestedValue(m, "name.first")
		// Should not panic
		if m["name"] != "John" {
			t.Error("name should remain unchanged")
		}
	})
}

func TestTransformer_Apply_SetField(t *testing.T) {
	transformer := NewTransformer(nil)

	t.Run("set string value", func(t *testing.T) {
		event := map[string]interface{}{}
		transform := Transform{SetField: `name = "John"`}

		result, err := transformer.Apply(event, transform)
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		if resultMap["name"] != "John" {
			t.Errorf("name = %v, want John", resultMap["name"])
		}
	})

	t.Run("set integer value", func(t *testing.T) {
		event := map[string]interface{}{}
		transform := Transform{SetField: "age = 30"}

		result, err := transformer.Apply(event, transform)
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		if resultMap["age"] != int64(30) {
			t.Errorf("age = %v (%T), want 30", resultMap["age"], resultMap["age"])
		}
	})

	t.Run("set boolean value", func(t *testing.T) {
		event := map[string]interface{}{}
		transform := Transform{SetField: "active = true"}

		result, err := transformer.Apply(event, transform)
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		if resultMap["active"] != true {
			t.Errorf("active = %v, want true", resultMap["active"])
		}
	})

	t.Run("set nested value", func(t *testing.T) {
		event := map[string]interface{}{}
		transform := Transform{SetField: `person.name = "John"`}

		result, err := transformer.Apply(event, transform)
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		person := resultMap["person"].(map[string]interface{})
		if person["name"] != "John" {
			t.Errorf("person.name = %v, want John", person["name"])
		}
	})

	t.Run("invalid format", func(t *testing.T) {
		event := map[string]interface{}{}
		transform := Transform{SetField: "invalid-no-equals"}

		_, err := transformer.Apply(event, transform)
		if err == nil {
			t.Error("expected error for invalid set_field format")
		}
	})

	t.Run("original event not modified", func(t *testing.T) {
		event := map[string]interface{}{"original": "value"}
		transform := Transform{SetField: "new = added"}

		_, err := transformer.Apply(event, transform)
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}

		if _, exists := event["new"]; exists {
			t.Error("original event should not be modified")
		}
	})
}

func TestTransformer_Apply_Redact(t *testing.T) {
	transformer := NewTransformer(nil)

	t.Run("redact single field", func(t *testing.T) {
		event := map[string]interface{}{
			"name": "John",
			"ssn":  "123-45-6789",
		}
		transform := Transform{
			Redact: &RedactConfig{Fields: []string{"ssn"}},
		}

		result, err := transformer.Apply(event, transform)
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		if _, exists := resultMap["ssn"]; exists {
			t.Error("ssn should be redacted")
		}
		if resultMap["name"] != "John" {
			t.Error("name should remain")
		}
	})

	t.Run("redact multiple fields", func(t *testing.T) {
		event := map[string]interface{}{
			"name":  "John",
			"ssn":   "123-45-6789",
			"phone": "555-1234",
		}
		transform := Transform{
			Redact: &RedactConfig{Fields: []string{"ssn", "phone"}},
		}

		result, err := transformer.Apply(event, transform)
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		if _, exists := resultMap["ssn"]; exists {
			t.Error("ssn should be redacted")
		}
		if _, exists := resultMap["phone"]; exists {
			t.Error("phone should be redacted")
		}
		if resultMap["name"] != "John" {
			t.Error("name should remain")
		}
	})

	t.Run("redact nested field", func(t *testing.T) {
		event := map[string]interface{}{
			"patient": map[string]interface{}{
				"name": "John",
				"ssn":  "123-45-6789",
			},
		}
		transform := Transform{
			Redact: &RedactConfig{Fields: []string{"patient.ssn"}},
		}

		result, err := transformer.Apply(event, transform)
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		patient := resultMap["patient"].(map[string]interface{})
		if _, exists := patient["ssn"]; exists {
			t.Error("patient.ssn should be redacted")
		}
		if patient["name"] != "John" {
			t.Error("patient.name should remain")
		}
	})
}

func TestTransformer_Apply_MapTerminology(t *testing.T) {
	mapper := &mockTerminologyMapper{
		mappings: map[string][]CodeMappingResult{
			"ICD9:250.00:ICD10": {
				{TargetCode: "E11.9", TargetDisplay: "Type 2 diabetes mellitus without complications"},
			},
			"SNOMED:73211009:LOINC": {
				{TargetCode: "44877-9", TargetDisplay: ""},
			},
		},
	}
	transformer := NewTransformer(mapper)

	t.Run("map terminology with display", func(t *testing.T) {
		event := map[string]interface{}{
			"diagnosis_code": "250.00",
		}
		transform := Transform{
			MapTerminology: &TerminologyMap{
				Field: "diagnosis_code",
				From:  "ICD9",
				To:    "ICD10",
			},
		}

		result, err := transformer.Apply(event, transform)
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		if resultMap["diagnosis_code"] != "E11.9" {
			t.Errorf("diagnosis_code = %v, want E11.9", resultMap["diagnosis_code"])
		}
		if resultMap["diagnosis_code_display"] != "Type 2 diabetes mellitus without complications" {
			t.Errorf("diagnosis_code_display = %v", resultMap["diagnosis_code_display"])
		}
	})

	t.Run("map terminology without display", func(t *testing.T) {
		event := map[string]interface{}{
			"code": "73211009",
		}
		transform := Transform{
			MapTerminology: &TerminologyMap{
				Field: "code",
				From:  "SNOMED",
				To:    "LOINC",
			},
		}

		result, err := transformer.Apply(event, transform)
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		if resultMap["code"] != "44877-9" {
			t.Errorf("code = %v, want 44877-9", resultMap["code"])
		}
	})

	t.Run("no mapping found leaves unchanged", func(t *testing.T) {
		event := map[string]interface{}{
			"code": "unknown",
		}
		transform := Transform{
			MapTerminology: &TerminologyMap{
				Field: "code",
				From:  "ICD9",
				To:    "ICD10",
			},
		}

		result, err := transformer.Apply(event, transform)
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		if resultMap["code"] != "unknown" {
			t.Errorf("code = %v, want unchanged", resultMap["code"])
		}
	})

	t.Run("missing field is skipped", func(t *testing.T) {
		event := map[string]interface{}{
			"other": "value",
		}
		transform := Transform{
			MapTerminology: &TerminologyMap{
				Field: "code",
				From:  "ICD9",
				To:    "ICD10",
			},
		}

		result, err := transformer.Apply(event, transform)
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		if _, exists := resultMap["code"]; exists {
			t.Error("code should not be created")
		}
	})

	t.Run("non-string field is skipped", func(t *testing.T) {
		event := map[string]interface{}{
			"code": 12345,
		}
		transform := Transform{
			MapTerminology: &TerminologyMap{
				Field: "code",
				From:  "ICD9",
				To:    "ICD10",
			},
		}

		result, err := transformer.Apply(event, transform)
		if err != nil {
			t.Fatalf("Apply error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		if resultMap["code"] != 12345 {
			t.Errorf("code = %v, want unchanged", resultMap["code"])
		}
	})

	t.Run("nil mapper returns error", func(t *testing.T) {
		transformerNoMapper := NewTransformer(nil)
		event := map[string]interface{}{
			"code": "250.00",
		}
		transform := Transform{
			MapTerminology: &TerminologyMap{
				Field: "code",
				From:  "ICD9",
				To:    "ICD10",
			},
		}

		_, err := transformerNoMapper.Apply(event, transform)
		if err == nil {
			t.Error("expected error when terminology mapper is nil")
		}
	})
}

func TestTransformer_Apply_FromStruct(t *testing.T) {
	transformer := NewTransformer(nil)

	type Event struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	event := Event{Name: "John", Age: 30}
	transform := Transform{SetField: `active = true`}

	result, err := transformer.Apply(event, transform)
	if err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	resultMap := result.(map[string]interface{})
	if resultMap["name"] != "John" {
		t.Errorf("name = %v, want John", resultMap["name"])
	}
	if resultMap["active"] != true {
		t.Errorf("active = %v, want true", resultMap["active"])
	}
}

func TestConvertMappings(t *testing.T) {
	t.Run("nil input", func(t *testing.T) {
		result := convertMappings(nil)
		if result != nil {
			t.Errorf("result = %v, want nil", result)
		}
	})

	t.Run("slice of interface maps", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"TargetCode":    "E11.9",
				"TargetDisplay": "Diabetes",
			},
			map[string]interface{}{
				"TargetCode":    "E11.8",
				"TargetDisplay": "Other diabetes",
			},
		}

		result := convertMappings(input)
		if len(result) != 2 {
			t.Fatalf("result length = %d, want 2", len(result))
		}
		if result[0].TargetCode != "E11.9" {
			t.Errorf("result[0].TargetCode = %v", result[0].TargetCode)
		}
		if result[0].TargetDisplay != "Diabetes" {
			t.Errorf("result[0].TargetDisplay = %v", result[0].TargetDisplay)
		}
	})

	t.Run("missing target code is skipped", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"TargetDisplay": "No code",
			},
		}

		result := convertMappings(input)
		if len(result) != 0 {
			t.Errorf("result length = %d, want 0", len(result))
		}
	})

	t.Run("non-slice input", func(t *testing.T) {
		input := "not a slice"
		result := convertMappings(input)
		if result != nil {
			t.Errorf("result = %v, want nil", result)
		}
	})

	t.Run("slice with non-map items", func(t *testing.T) {
		input := []interface{}{"string", 123}
		result := convertMappings(input)
		if len(result) != 0 {
			t.Errorf("result length = %d, want 0", len(result))
		}
	})
}

func TestTerminologyMapperAdapter(t *testing.T) {
	t.Run("new adapter", func(t *testing.T) {
		adapter := NewTerminologyMapperAdapter("anything")
		if adapter == nil {
			t.Fatal("adapter is nil")
		}
		if adapter.mapper != "anything" {
			t.Error("mapper not set correctly")
		}
	})

	t.Run("map with non-implementing type", func(t *testing.T) {
		adapter := NewTerminologyMapperAdapter("not a mapper")
		result := adapter.Map("ICD9", "250.00", "ICD10")
		if result != nil {
			t.Errorf("result = %v, want nil", result)
		}
	})

	t.Run("map with implementing type", func(t *testing.T) {
		// Create a mock that implements the expected interface
		mock := &mockMapperWithMap{
			result: []interface{}{
				map[string]interface{}{
					"TargetCode":    "E11.9",
					"TargetDisplay": "Diabetes",
				},
			},
		}
		adapter := NewTerminologyMapperAdapter(mock)

		result := adapter.Map("ICD9", "250.00", "ICD10")
		if len(result) != 1 {
			t.Fatalf("result length = %d, want 1", len(result))
		}
		if result[0].TargetCode != "E11.9" {
			t.Errorf("result[0].TargetCode = %v", result[0].TargetCode)
		}
	})
}

// mockMapperWithMap implements the interface expected by TerminologyMapperAdapter.
type mockMapperWithMap struct {
	result interface{}
}

func (m *mockMapperWithMap) Map(sourceSystem, sourceCode, targetSystem string) interface{} {
	return m.result
}

// mockWarningExplainer implements WarningExplainerInterface for testing.
type mockWarningExplainer struct {
	explainFunc func(ctx context.Context, warning events.ParseWarning, format events.SourceFormat) (*ExplainedWarningResult, error)
	calls       []events.ParseWarning
}

func (m *mockWarningExplainer) Explain(ctx context.Context, warning events.ParseWarning, format events.SourceFormat) (*ExplainedWarningResult, error) {
	m.calls = append(m.calls, warning)
	if m.explainFunc != nil {
		return m.explainFunc(ctx, warning, format)
	}
	return &ExplainedWarningResult{
		Explanation:   "Test explanation for " + warning.Code,
		FixSuggestion: "Test fix for " + warning.Code,
		Impact:        "Test impact for " + warning.Code,
	}, nil
}

func TestTransformer_ApplyWithContext_ExplainWarnings(t *testing.T) {
	ctx := context.Background()

	t.Run("explains warnings successfully", func(t *testing.T) {
		mock := &mockWarningExplainer{}
		transformer := NewTransformer(nil)
		transformer.SetWarningExplainer(mock)

		event := map[string]interface{}{
			"type": "patient_admit",
			"warnings": []interface{}{
				map[string]interface{}{
					"code":     "INVALID_NPI",
					"message":  "Invalid NPI format",
					"path":     "PID-3",
					"phase":    "validation",
					"severity": "warning",
				},
			},
		}

		transform := Transform{
			ExplainWarnings: &ExplainWarningsConfig{
				IncludeFix: true,
			},
		}

		result, err := transformer.ApplyWithContext(ctx, event, transform)
		if err != nil {
			t.Fatalf("ApplyWithContext error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		warnings := resultMap["warnings"].([]map[string]interface{})
		if len(warnings) != 1 {
			t.Fatalf("warnings length = %d, want 1", len(warnings))
		}

		w := warnings[0]
		if w["explanation"] != "Test explanation for INVALID_NPI" {
			t.Errorf("explanation = %v, want 'Test explanation for INVALID_NPI'", w["explanation"])
		}
		if w["fix_suggestion"] != "Test fix for INVALID_NPI" {
			t.Errorf("fix_suggestion = %v, want 'Test fix for INVALID_NPI'", w["fix_suggestion"])
		}
		if w["impact"] != "Test impact for INVALID_NPI" {
			t.Errorf("impact = %v, want 'Test impact for INVALID_NPI'", w["impact"])
		}
	})

	t.Run("skips fix_suggestion when IncludeFix is false", func(t *testing.T) {
		mock := &mockWarningExplainer{}
		transformer := NewTransformer(nil)
		transformer.SetWarningExplainer(mock)

		event := map[string]interface{}{
			"warnings": []interface{}{
				map[string]interface{}{
					"code":    "MISSING_PV1",
					"message": "Missing PV1 segment",
				},
			},
		}

		transform := Transform{
			ExplainWarnings: &ExplainWarningsConfig{
				IncludeFix: false,
			},
		}

		result, err := transformer.ApplyWithContext(ctx, event, transform)
		if err != nil {
			t.Fatalf("ApplyWithContext error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		warnings := resultMap["warnings"].([]map[string]interface{})
		w := warnings[0]

		if _, exists := w["fix_suggestion"]; exists {
			t.Error("fix_suggestion should not be present when IncludeFix is false")
		}
	})

	t.Run("uses custom warnings_field", func(t *testing.T) {
		mock := &mockWarningExplainer{}
		transformer := NewTransformer(nil)
		transformer.SetWarningExplainer(mock)

		event := map[string]interface{}{
			"parse_warnings": []interface{}{
				map[string]interface{}{
					"code":    "INVALID_DATE",
					"message": "Invalid date format",
				},
			},
		}

		transform := Transform{
			ExplainWarnings: &ExplainWarningsConfig{
				WarningsField: "parse_warnings",
			},
		}

		result, err := transformer.ApplyWithContext(ctx, event, transform)
		if err != nil {
			t.Fatalf("ApplyWithContext error: %v", err)
		}

		resultMap := result.(map[string]interface{})
		warnings := resultMap["parse_warnings"].([]map[string]interface{})
		if len(warnings) != 1 {
			t.Fatalf("parse_warnings length = %d, want 1", len(warnings))
		}
		if warnings[0]["explanation"] == nil {
			t.Error("explanation should be present")
		}
	})

	t.Run("no error when warnings field missing", func(t *testing.T) {
		mock := &mockWarningExplainer{}
		transformer := NewTransformer(nil)
		transformer.SetWarningExplainer(mock)

		event := map[string]interface{}{
			"type": "patient_admit",
		}

		transform := Transform{
			ExplainWarnings: &ExplainWarningsConfig{},
		}

		_, err := transformer.ApplyWithContext(ctx, event, transform)
		if err != nil {
			t.Fatalf("ApplyWithContext should not error when warnings missing: %v", err)
		}

		if len(mock.calls) != 0 {
			t.Error("explainer should not be called when no warnings")
		}
	})

	t.Run("error when explainer not configured", func(t *testing.T) {
		transformer := NewTransformer(nil)
		// Don't set warning explainer

		event := map[string]interface{}{
			"warnings": []interface{}{
				map[string]interface{}{"code": "TEST"},
			},
		}

		transform := Transform{
			ExplainWarnings: &ExplainWarningsConfig{},
		}

		_, err := transformer.ApplyWithContext(ctx, event, transform)
		if err == nil {
			t.Fatal("expected error when explainer not configured")
		}
		if !strings.Contains(err.Error(), "explainer not configured") {
			t.Errorf("error = %v, should contain 'explainer not configured'", err)
		}
	})
}

func TestToParseWarnings(t *testing.T) {
	t.Run("converts []interface{} to ParseWarnings", func(t *testing.T) {
		input := []interface{}{
			map[string]interface{}{
				"code":     "INVALID_NPI",
				"message":  "Invalid NPI format",
				"path":     "PID-3",
				"phase":    "validation",
				"severity": "warning",
			},
			map[string]interface{}{
				"code":    "MISSING_PV1",
				"message": "Missing segment",
			},
		}

		result, err := toParseWarnings(input)
		if err != nil {
			t.Fatalf("toParseWarnings error: %v", err)
		}

		if len(result) != 2 {
			t.Fatalf("result length = %d, want 2", len(result))
		}

		if result[0].Code != "INVALID_NPI" {
			t.Errorf("result[0].Code = %v, want INVALID_NPI", result[0].Code)
		}
		if result[0].Severity != "warning" {
			t.Errorf("result[0].Severity = %v, want warning", result[0].Severity)
		}
		if result[1].Code != "MISSING_PV1" {
			t.Errorf("result[1].Code = %v, want MISSING_PV1", result[1].Code)
		}
	})

	t.Run("returns error for unsupported type", func(t *testing.T) {
		_, err := toParseWarnings("invalid")
		if err == nil {
			t.Error("expected error for unsupported type")
		}
	})
}

func TestDetectSourceFormat(t *testing.T) {
	tests := []struct {
		name     string
		event    map[string]interface{}
		expected events.SourceFormat
	}{
		{
			name: "from meta.source_format",
			event: map[string]interface{}{
				"meta": map[string]interface{}{
					"source_format": "hl7v2",
				},
			},
			expected: events.SourceFormat("hl7v2"),
		},
		{
			name: "from source field with HL7",
			event: map[string]interface{}{
				"source": "hl7v2-parser",
			},
			expected: events.FormatHL7v2,
		},
		{
			name: "from source field with FHIR",
			event: map[string]interface{}{
				"source": "fhir-server",
			},
			expected: events.FormatFHIR,
		},
		{
			name: "from source field with 835",
			event: map[string]interface{}{
				"source": "edi-835-processor",
			},
			expected: events.FormatEDI835,
		},
		{
			name: "from source field with 837",
			event: map[string]interface{}{
				"source": "837-claims",
			},
			expected: events.FormatEDI837,
		},
		{
			name: "returns unknown for unrecognized",
			event: map[string]interface{}{
				"source": "custom-source",
			},
			expected: events.FormatUnknown,
		},
		{
			name:     "returns unknown for empty event",
			event:    map[string]interface{}{},
			expected: events.FormatUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := detectSourceFormat(tt.event)
			if result != tt.expected {
				t.Errorf("detectSourceFormat = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestGetString(t *testing.T) {
	t.Run("returns string value", func(t *testing.T) {
		m := map[string]interface{}{"key": "value"}
		if getString(m, "key") != "value" {
			t.Error("expected 'value'")
		}
	})

	t.Run("returns empty for non-string", func(t *testing.T) {
		m := map[string]interface{}{"key": 123}
		if getString(m, "key") != "" {
			t.Error("expected empty string for non-string value")
		}
	})

	t.Run("returns empty for missing key", func(t *testing.T) {
		m := map[string]interface{}{}
		if getString(m, "key") != "" {
			t.Error("expected empty string for missing key")
		}
	})
}
