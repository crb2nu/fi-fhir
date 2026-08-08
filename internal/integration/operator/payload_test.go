package operator

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestSummarizePayloadRendersStructureWithoutValues(t *testing.T) {
	payload := []byte(`{
		"id": "event-a",
		"type": "patient_admit",
		"patient": {
			"mrn": "RAW-PHI-SENTINEL",
			"name": {"family": "Sentinel", "given": ["Raw", "Phi"]},
			"deceased": false,
			"age": 42,
			"middleName": null
		},
		"identifiers": [{"system": "urn:oid:1.2.3", "value": "RAW-PHI-SENTINEL"}]
	}`)

	fields, truncated := summarizePayload(payload)
	if truncated {
		t.Fatalf("small payload reported truncation")
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		t.Fatalf("marshal payload summary: %v", err)
	}
	for _, forbidden := range []string{"RAW-PHI-SENTINEL", "Sentinel", "urn:oid:1.2.3", "42"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("payload summary leaked %q: %s", forbidden, encoded)
		}
	}

	byPath := make(map[string]PayloadField, len(fields))
	for _, field := range fields {
		byPath[field.Path] = field
	}
	expected := map[string]string{
		"id":                  "string",
		"type":                "string",
		"patient":             "object",
		"patient.mrn":         "string",
		"patient.name":        "object",
		"patient.name.family": "string",
		"patient.name.given":  "array",
		"patient.deceased":    "boolean",
		"patient.age":         "number",
		"patient.middleName":  "null",
		"identifiers":         "array",
	}
	for path, kind := range expected {
		field, ok := byPath[path]
		if !ok {
			t.Fatalf("payload summary is missing %q: %#v", path, byPath)
		}
		if field.Kind != kind {
			t.Fatalf("field %q kind = %q, want %q", path, field.Kind, kind)
		}
	}
	if !byPath["identifiers"].Repeated {
		t.Fatalf("array container should be marked repeated: %#v", byPath["identifiers"])
	}
	if !byPath["identifiers.system"].Repeated {
		t.Fatalf("array element field should be marked repeated: %#v", byPath["identifiers.system"])
	}
}

func TestSummarizePayloadRedactsNonCanonicalKeys(t *testing.T) {
	// A dynamic map key is caller-influenced data, so it can never be emitted
	// as a path component even though schema keys can.
	payload := []byte(`{"codes": {"urn:oid:2.16.840.1": "A01", "canonical_key": "x"}}`)
	fields, _ := summarizePayload(payload)
	paths := make([]string, 0, len(fields))
	for _, field := range fields {
		paths = append(paths, field.Path)
	}
	joined := strings.Join(paths, ",")
	if strings.Contains(joined, "urn:oid") {
		t.Fatalf("summary leaked a dynamic map key: %v", paths)
	}
	if !strings.Contains(joined, "codes."+redactedKey) {
		t.Fatalf("summary did not collapse the dynamic key: %v", paths)
	}
	if !strings.Contains(joined, "codes.canonical_key") {
		t.Fatalf("summary dropped the canonical key: %v", paths)
	}
}

func TestSummarizePayloadBoundsFieldCount(t *testing.T) {
	builder := strings.Builder{}
	builder.WriteString(`{`)
	for index := range MaxPayloadFields + 50 {
		if index > 0 {
			builder.WriteString(",")
		}
		fmt.Fprintf(&builder, `"field_%03d":"value"`, index)
	}
	builder.WriteString(`}`)

	fields, truncated := summarizePayload([]byte(builder.String()))
	if len(fields) > MaxPayloadFields {
		t.Fatalf("summary returned %d fields, want at most %d", len(fields), MaxPayloadFields)
	}
	if !truncated {
		t.Fatal("oversized payload did not report truncation")
	}
}

func TestSummarizePayloadHandlesUnusableInput(t *testing.T) {
	for name, raw := range map[string][]byte{
		"empty":   nil,
		"invalid": []byte("not-json"),
		"scalar":  []byte(`"RAW-PHI-SENTINEL"`),
	} {
		t.Run(name, func(t *testing.T) {
			fields, truncated := summarizePayload(raw)
			if len(fields) != 0 || truncated {
				t.Fatalf("summary = %#v truncated=%v, want empty", fields, truncated)
			}
		})
	}
}

func TestBoundedDetailDropsNonCanonicalKeysAndLongValues(t *testing.T) {
	detail := boundedDetail([]byte(`{"code":"KAFKA_PUBLISH_FAILED","urn:leak":"x","long":"` +
		strings.Repeat("a", 600) + `"}`))
	if detail["code"] != "KAFKA_PUBLISH_FAILED" {
		t.Fatalf("detail dropped a canonical key: %#v", detail)
	}
	if _, present := detail["urn:leak"]; present {
		t.Fatalf("detail kept a non-canonical key: %#v", detail)
	}
	long, ok := detail["long"].(string)
	if !ok || len(long) > 512 {
		t.Fatalf("detail did not bound a long value: %d", len(long))
	}
}
