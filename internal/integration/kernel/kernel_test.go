package kernel_test

import (
	"bytes"
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/kernel"
)

const ideDemoSamples = "../../../ui/src/lib/features/hl7/samples/demoSamples.ts"

// idePattern matches one entry of demoSamples.ts: a name, a source, and the
// raw message as a template literal.
var idePattern = regexp.MustCompile("(?s)\\{\\s*name:\\s*'([^']*)',\\s*source:\\s*'([^']*)',\\s*raw:\\s*`((?:[^`\\\\]|\\\\.)*)`\\s*\\}")

// TestSamplesMatchIDEDemoSamples holds the kernel's embedded samples
// byte-identical to the demo samples the IDE seeds, in the IDE's order, so the
// playground and the IDE cannot drift apart.
func TestSamplesMatchIDEDemoSamples(t *testing.T) {
	source, err := os.ReadFile(ideDemoSamples)
	if err != nil {
		t.Fatalf("read %s: %v", ideDemoSamples, err)
	}
	matches := idePattern.FindAllStringSubmatch(string(source), -1)
	if declared := strings.Count(string(source), "name: '"); declared != len(matches) {
		t.Fatalf("demoSamples.ts declares %d names but %d entries parse; update idePattern with the file's new shape", declared, len(matches))
	}
	samples, err := kernel.Samples()
	if err != nil {
		t.Fatalf("Samples: %v", err)
	}
	if len(samples) != len(matches) {
		t.Fatalf("kernel has %d samples, the IDE ships %d", len(samples), len(matches))
	}
	unescape := regexp.MustCompile(`\\(.)`)
	for index, match := range matches {
		name, sourceName, raw := match[1], match[2], match[3]
		if strings.Contains(raw, "${") {
			t.Fatalf("IDE sample %q interpolates; the drift check compares literal bytes only", name)
		}
		raw = unescape.ReplaceAllString(raw, "$1")
		sample := samples[index]
		if sample.Name != name || sample.Source != sourceName || sample.Format != kernel.FormatHL7v2 {
			t.Errorf("sample %d = {%q %q %q}, IDE = {%q %q hl7v2}", index, sample.Name, sample.Source, sample.Format, name, sourceName)
		}
		if sample.Text != raw {
			t.Errorf("sample %d (%s) bytes differ from the IDE's demo sample; regenerate internal/integration/kernel/testdata/samples from %s", index, sample.ID, ideDemoSamples)
		}
	}
}

// TestBuiltInProfilesMatchGoldenJSON holds the YAML golden profiles equal to
// the adt-http golden JSON the integration proofs use, and every built-in
// profile valid under the production compiler.
func TestBuiltInProfilesMatchGoldenJSON(t *testing.T) {
	golden := map[string]string{
		"adt-http-strict":   "../../../testdata/golden/integration/adt-http/strict-profile.json",
		"adt-http-tolerant": "../../../testdata/golden/integration/adt-http/tolerant-profile.json",
	}
	profiles, err := kernel.Profiles()
	if err != nil {
		t.Fatalf("Profiles: %v", err)
	}
	if len(profiles) == 0 {
		t.Fatal("no built-in profiles")
	}
	seen := map[string]bool{}
	for _, prof := range profiles {
		if prof.ID == "" || prof.Name == "" || prof.Description == "" || prof.YAML == "" {
			t.Errorf("profile %+v has an empty member", prof)
		}
		if seen[prof.ID] {
			t.Errorf("duplicate profile id %q", prof.ID)
		}
		seen[prof.ID] = true
		if result := kernel.ValidateProfile(prof.YAML); !result.OK {
			t.Errorf("built-in profile %s does not compile: %+v", prof.ID, result.Problems)
		}
		path, ok := golden[prof.ID]
		if !ok {
			continue
		}
		delete(golden, prof.ID)
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		got, err := kernel.ProfileJSON(prof.YAML)
		if err != nil {
			t.Fatalf("ProfileJSON(%s): %v", prof.ID, err)
		}
		if !bytes.Equal(canonical(t, want), canonical(t, got)) {
			t.Errorf("profile %s:\n yaml → %s\n golden %s", prof.ID, canonical(t, got), canonical(t, want))
		}
	}
	for id := range golden {
		t.Errorf("golden profile %s has no built-in YAML form", id)
	}
}

func TestPreviewRefusesBadRequests(t *testing.T) {
	adt := sampleText(t, "adt-a01-icu-admission")
	request := func(fields map[string]any) string {
		raw, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		return string(raw)
	}
	tests := []struct {
		name     string
		input    string
		wantCode string
		wantPath string
	}{
		{"not json", "MSH|^~\\&|", kernel.CodeInputInvalid, "$"},
		{"unknown field", request(map[string]any{"message": adt, "format": "hl7v2", "profile": "x"}), kernel.CodeInputInvalid, "$"},
		{"trailing content", request(map[string]any{"message": adt, "format": "hl7v2"}) + "{}", kernel.CodeInputInvalid, "$"},
		{"envelope too large", strings.Repeat(" ", 3<<20+1), kernel.CodeInputTooLarge, "$"},
		{"empty message", request(map[string]any{"message": " \r\n", "format": "hl7v2"}), kernel.CodeMessageEmpty, "message"},
		{"message too large", request(map[string]any{"message": "MSH|" + strings.Repeat("x", 1<<20), "format": "hl7v2"}), kernel.CodeMessageTooLarge, "message"},
		{"missing format", request(map[string]any{"message": adt}), kernel.CodeFormatUnsupported, "format"},
		{"other format", request(map[string]any{"message": adt, "format": "cda"}), kernel.CodeFormatUnsupported, "format"},
		{"blank profile", request(map[string]any{"message": adt, "format": "hl7v2", "profileYaml": "  \n"}), kernel.CodeProfileEmpty, "profileYaml"},
		{"profile syntax", request(map[string]any{"message": adt, "format": "hl7v2", "profileYaml": "hl7v2: [unclosed"}), kernel.CodeProfileSyntax, "profileYaml"},
		{"profile two documents", request(map[string]any{"message": adt, "format": "hl7v2", "profileYaml": "a: 1\n---\nb: 2\n"}), kernel.CodeProfileSyntax, "profileYaml"},
		{"profile non-string key", request(map[string]any{"message": adt, "format": "hl7v2", "profileYaml": "1: x\n"}), kernel.CodeProfileSyntax, "profileYaml"},
		{"profile not an object", request(map[string]any{"message": adt, "format": "hl7v2", "profileYaml": "- a\n"}), kernel.CodeProfileInvalid, "profileYaml"},
		{"profile bad timezone", request(map[string]any{"message": adt, "format": "hl7v2", "profileYaml": withTimezone(t, "Mars/Olympus")}), kernel.CodeProfileInvalid, "profileYaml.hl7v2.timezone"},
		{"profile Local timezone", request(map[string]any{"message": adt, "format": "hl7v2", "profileYaml": withTimezone(t, "Local")}), kernel.CodeProfileUnsupported, "profileYaml.hl7v2.timezone"},
		{"profile unknown key", request(map[string]any{"message": adt, "format": "hl7v2", "profileYaml": "hl7v2:\n  mystery: 1\n"}), kernel.CodeProfileInvalid, "profileYaml.hl7v2"},
		{"timezone invalid", request(map[string]any{"message": adt, "format": "hl7v2", "timezone": "Nowhere/Special"}), kernel.CodeTimezoneInvalid, "timezone"},
		{"timezone Local", request(map[string]any{"message": adt, "format": "hl7v2", "timezone": "Local"}), kernel.CodeTimezoneInvalid, "timezone"},
		{"timezone conflicts with profile", request(map[string]any{"message": adt, "format": "hl7v2", "profileYaml": builtInProfileYAML(t, "adt-http-strict"), "timezone": "America/Chicago"}), kernel.CodeTimezoneConflict, "timezone"},
		{"not HL7", request(map[string]any{"message": "PID|1||x", "format": "hl7v2"}), kernel.CodeParseFailed, "message"},
		{"unsupported message type", request(map[string]any{"message": sampleText(t, "orm-o01-lab-order"), "format": "hl7v2"}), kernel.CodeParseFailed, "message"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			response := kernel.Preview([]byte(tc.input))
			if response.OK {
				t.Fatalf("ok = true, want a %s problem", tc.wantCode)
			}
			if len(response.Problems) == 0 || response.Problems[0].Code != tc.wantCode || response.Problems[0].Path != tc.wantPath {
				t.Fatalf("problems = %+v, want %s at %s", response.Problems, tc.wantCode, tc.wantPath)
			}
			if response.Problems[0].Message == "" {
				t.Fatal("problem has no message")
			}
			assertEncodesAsContract(t, response)
		})
	}
}

func TestPreviewSamples(t *testing.T) {
	samples, err := kernel.Samples()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		ok         bool
		eventType  string
		bundle     bool
		bundleCode string
	}{
		"adt-a01-icu-admission":           {ok: true, eventType: "patient_admit", bundle: true},
		"oru-r01-lab-results-cbc":         {ok: true, eventType: "lab_result", bundle: true},
		"siu-s12-appointment-scheduled":   {ok: true, eventType: "appointment_scheduled", bundleCode: kernel.CodeFHIRProjectionUnsupported},
		"adt-a03-discharge-with-warnings": {ok: true, eventType: "patient_discharge", bundle: true},
		"orm-o01-lab-order":               {ok: false},
		"mdm-t02-document-with-content":   {ok: true, eventType: "document_original", bundleCode: kernel.CodeFHIRProjectionUnsupported},
	}
	for _, sample := range samples {
		t.Run(sample.ID, func(t *testing.T) {
			expected, ok := want[sample.ID]
			if !ok {
				t.Fatalf("no expectation for sample %s", sample.ID)
			}
			input, _ := json.Marshal(kernel.PreviewRequest{Message: sample.Text, Format: sample.Format, Source: sample.Source})
			response := kernel.Preview(input)
			assertEncodesAsContract(t, response)
			if response.OK != expected.ok {
				t.Fatalf("ok = %t, want %t (problems %+v)", response.OK, expected.ok, response.Problems)
			}
			if len(response.Segments) == 0 || response.Segments[0].ID != "MSH" || response.Segments[0].Fields[1] != "|" {
				t.Fatalf("segments = %+v, want the parser's MSH-first tokenization", response.Segments)
			}
			if !expected.ok {
				if len(response.Events) != 0 || response.Bundle != nil {
					t.Fatalf("a failed parse returned events %+v or a bundle", response.Events)
				}
				return
			}
			if len(response.Events) != 1 || response.Events[0].Type != expected.eventType {
				t.Fatalf("events = %+v, want one %s", response.Events, expected.eventType)
			}
			if expected.bundle {
				var bundle struct {
					ResourceType string `json:"resourceType"`
					Type         string `json:"type"`
					Entry        []struct {
						FullURL string `json:"fullUrl"`
						Request struct {
							Method string `json:"method"`
						} `json:"request"`
					} `json:"entry"`
				}
				if err := json.Unmarshal(response.Bundle, &bundle); err != nil {
					t.Fatalf("bundle: %v", err)
				}
				if bundle.ResourceType != "Bundle" || bundle.Type != "transaction" || len(bundle.Entry) == 0 {
					t.Fatalf("bundle = %s", response.Bundle)
				}
				for _, entry := range bundle.Entry {
					if !strings.HasPrefix(entry.FullURL, "urn:uuid:") || entry.Request.Method != "PUT" {
						t.Fatalf("entry %+v is not a conditional update with a deterministic fullUrl", entry)
					}
				}
				if response.BundleProblem != nil {
					t.Fatalf("bundle and bundleProblem both set: %+v", response.BundleProblem)
				}
				return
			}
			if response.Bundle != nil || response.BundleProblem == nil || response.BundleProblem.Code != expected.bundleCode {
				t.Fatalf("bundle = %s, bundleProblem = %+v, want %s", response.Bundle, response.BundleProblem, expected.bundleCode)
			}
		})
	}
}

// TestPreviewBundleIsDeterministic: the event carries a fresh random ID and
// clock reading on every parse, but the Bundle a destination receives is
// keyed on identifiers, so two previews of one message give the same bytes.
func TestPreviewBundleIsDeterministic(t *testing.T) {
	for _, id := range []string{"adt-a01-icu-admission", "oru-r01-lab-results-cbc", "adt-a03-discharge-with-warnings"} {
		input, _ := json.Marshal(kernel.PreviewRequest{Message: sampleText(t, id), Format: kernel.FormatHL7v2})
		first := kernel.Preview(input)
		second := kernel.Preview(input)
		if first.Bundle == nil || !bytes.Equal(first.Bundle, second.Bundle) {
			t.Fatalf("%s: bundles differ between two previews of the same message", id)
		}
	}
}

func TestPreviewProfileChangesClassification(t *testing.T) {
	adt := sampleText(t, "adt-a01-icu-admission") // PV1-2 = I
	input, _ := json.Marshal(kernel.PreviewRequest{
		Message:     adt,
		Format:      kernel.FormatHL7v2,
		ProfileYAML: builtInProfileYAML(t, "adt-patient-class"),
	})
	response := kernel.Preview(input)
	if !response.OK {
		t.Fatalf("problems = %+v", response.Problems)
	}
	var payload struct {
		SourceProfileID string `json:"source_profile_id"`
		Encounter       struct {
			ClassifiedEventType string `json:"classified_event_type"`
		} `json:"encounter"`
	}
	if err := json.Unmarshal(response.Events[0].Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Encounter.ClassifiedEventType != "inpatient_admit" || payload.SourceProfileID != kernel.PlaygroundProfileID {
		t.Fatalf("payload = %+v, want inpatient_admit under %s", payload, kernel.PlaygroundProfileID)
	}
}

func TestPreviewTimezoneWithoutProfile(t *testing.T) {
	message := sampleText(t, "adt-a01-icu-admission")
	parse := func(timezone string) string {
		input, _ := json.Marshal(kernel.PreviewRequest{Message: message, Format: kernel.FormatHL7v2, Timezone: timezone})
		response := kernel.Preview(input)
		if !response.OK {
			t.Fatalf("timezone %q: problems %+v", timezone, response.Problems)
		}
		var payload struct {
			Patient struct {
				BirthDate string `json:"date_of_birth"`
			} `json:"patient"`
		}
		if err := json.Unmarshal(response.Events[0].Payload, &payload); err != nil {
			t.Fatal(err)
		}
		return payload.Patient.BirthDate
	}
	utc, eastern := parse(""), parse("America/New_York")
	if utc == eastern {
		t.Fatalf("birth date %s did not move with the timezone", utc)
	}
}

func TestValidateProfile(t *testing.T) {
	if result := kernel.ValidateProfile(builtInProfileYAML(t, "adt-http-strict")); !result.OK || len(result.Problems) != 0 {
		t.Fatalf("strict profile: %+v", result)
	}
	tests := []struct {
		name, yaml, code, path string
	}{
		{"empty", "", kernel.CodeProfileEmpty, "$"},
		{"syntax", "{", kernel.CodeProfileSyntax, "$"},
		{"no hl7v2", "identifiers: {}\n", kernel.CodeProfileInvalid, "hl7v2"},
		{"terminology unsupported", strings.Replace(builtInProfileYAML(t, "adt-http-strict"), "identifiers:", "terminology:\n  mappings:\n    - {}\nidentifiers:", 1), kernel.CodeProfileUnsupported, "terminology.mappings"},
		{"too large", "# " + strings.Repeat("x", 1<<20), kernel.CodeProfileTooLarge, "$"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := kernel.ValidateProfile(tc.yaml)
			if result.OK || len(result.Problems) != 1 || result.Problems[0].Code != tc.code || result.Problems[0].Path != tc.path {
				t.Fatalf("result = %+v, want %s at %s", result, tc.code, tc.path)
			}
		})
	}
}

// assertEncodesAsContract checks the JSON a page receives: every array is an
// array (never null), so the page can iterate without guards.
func assertEncodesAsContract(t *testing.T, response kernel.PreviewResponse) {
	t.Helper()
	raw, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(raw, &shape); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"ok", "segments", "events", "diagnostics", "problems"} {
		value, ok := shape[key]
		if !ok || string(value) == "null" {
			t.Fatalf("response member %q is %s", key, value)
		}
	}
}

func canonical(t *testing.T, raw []byte) []byte {
	t.Helper()
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	out, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func sampleText(t *testing.T, id string) string {
	t.Helper()
	samples, err := kernel.Samples()
	if err != nil {
		t.Fatal(err)
	}
	for _, sample := range samples {
		if sample.ID == id {
			return sample.Text
		}
	}
	t.Fatalf("no sample %q", id)
	return ""
}

func withTimezone(t *testing.T, timezone string) string {
	t.Helper()
	return strings.Replace(builtInProfileYAML(t, "adt-http-strict"), "timezone: UTC", "timezone: "+timezone, 1)
}
