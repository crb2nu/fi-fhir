package processor

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCompileSourceProfileMapsSupportedADTProfile(t *testing.T) {
	t.Parallel()

	resolved := resolvedProfileForCompile(t, `{
		"hl7v2": {
			"default_version": "2.5.1",
			"timezone": "America/New_York",
			"tolerance": {
				"missing_segments": ["PV1"],
				"nte_anywhere": false,
				"extra_components": false,
				"unknown_segments": false,
				"non_standard_delimiters": false
			},
			"event_classifications": [
				{"message_type":"ADT^A01","event_type":"patient_admit","priority":100},
				{"message_type":"ADT^A01","condition":"PV1.2 == 'I'","event_type":"inpatient_admit","priority":20},
				{"message_type":"ADT^A01^ADT_A01","condition":"PV1.2 == 'E'","event_type":"emergency_registration","priority":10}
			]
		},
		"identifiers": {
			"assigning_authorities": [
				{"code":"HOSP","system":"urn:oid:1.2.3","name":"Hospital"}
			],
			"normalization": {
				"ssn_strip_dashes": true,
				"ssn_reject_patterns": ["000000000"],
				"phone_normalize": false
			}
		}
	}`)

	compiled, err := compileSourceProfile(resolved)
	if err != nil {
		t.Fatalf("compileSourceProfile: %v", err)
	}
	if compiled.source == nil {
		t.Fatal("compiled source profile is nil")
	}
	if compiled.source.ID != "profile-adt" || compiled.source.Name != "profile-adt" || compiled.source.Version != "7" {
		t.Fatalf("profile identity did not come from exact revision: %#v", compiled.source)
	}
	wantLocation, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("load expected timezone: %v", err)
	}
	if compiled.timezone.String() != wantLocation.String() {
		t.Fatalf("timezone = %q, want %q", compiled.timezone, wantLocation)
	}
	if !compiled.source.IsMissingSegmentTolerated("PV1") || compiled.source.IsMissingSegmentTolerated("PID") {
		t.Fatalf("unexpected missing-segment tolerance: %#v", compiled.source.HL7v2.Tolerate)
	}
	if got := compiled.source.GetEventClassification("ADT^A01", "E"); got != "emergency_registration" {
		t.Fatalf("E classification = %q", got)
	}
	if got := compiled.source.GetEventClassification("ADT^A01", "I"); got != "inpatient_admit" {
		t.Fatalf("I classification = %q", got)
	}
	if got := compiled.source.GetEventClassification("ADT^A01", "O"); got != "patient_admit" {
		t.Fatalf("default classification = %q", got)
	}
	if got := compiled.source.GetAssigningAuthoritySystem("HOSP"); got != "urn:oid:1.2.3" {
		t.Fatalf("authority mapping = %q", got)
	}
	if got := compiled.source.Identifiers.Normalization.SSN.RejectPatterns; !reflect.DeepEqual(got, []string{"000000000", "123456789", "111111111", "999999999"}) {
		t.Fatalf("SSN reject patterns = %#v", got)
	}
	if compiled.source.ZSegments == nil || compiled.source.ZSegments.PreserveRaw || len(compiled.source.ZSegments.Mappings) != 0 {
		t.Fatalf("Z-segment handling is not fail closed: %#v", compiled.source.ZSegments)
	}
}

func TestCompileSourceProfileIsStrictAndFailClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		json string
		kind error
	}{
		{name: "unknown top level", json: `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]},"secret":"sentinel"}`, kind: ErrInvalidSourceProfile},
		{name: "case variant top level", json: `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]},"HL7V2":{"default_version":"2.5.1","timezone":"UTC","event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]}}`, kind: ErrInvalidSourceProfile},
		{name: "unknown nested", json: `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}],"typo":true}}`, kind: ErrInvalidSourceProfile},
		{name: "case variant nested", json: `{"hl7v2":{"default_version":"2.5.1","timezone":"UTC","Timezone":"America/New_York","event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]}}`, kind: ErrInvalidSourceProfile},
		{name: "missing HL7", json: `{}`, kind: ErrInvalidSourceProfile},
		{name: "unsupported version", json: validProfileJSONWith(`"default_version":"9.9"`), kind: ErrUnsupportedSourceProfile},
		{name: "invalid timezone", json: validProfileJSONWith(`"timezone":"Mars/Olympus"`), kind: ErrInvalidSourceProfile},
		{name: "host local timezone", json: validProfileJSONWith(`"timezone":"Local"`), kind: ErrUnsupportedSourceProfile},
		{name: "unsupported missing segment", json: validProfileJSONWith(`"tolerance":{"missing_segments":["PID"]}`), kind: ErrUnsupportedSourceProfile},
		{name: "duplicate missing segment", json: validProfileJSONWith(`"tolerance":{"missing_segments":["PV1","PV1"]}`), kind: ErrInvalidSourceProfile},
		{name: "unsupported tolerance", json: validProfileJSONWith(`"tolerance":{"missing_segments":["PV1"],"unknown_segments":true}`), kind: ErrUnsupportedSourceProfile},
		{name: "unsupported message type", json: validProfileJSONWith(`"event_classifications":[{"message_type":"ADT^A04","event_type":"patient_admit","priority":1}]`), kind: ErrUnsupportedSourceProfile},
		{name: "malformed condition", json: validProfileJSONWith(`"event_classifications":[{"message_type":"ADT^A01","condition":"PV1.2 != 'I'","event_type":"patient_admit","priority":1}]`), kind: ErrInvalidSourceProfile},
		{name: "unknown event", json: validProfileJSONWith(`"event_classifications":[{"message_type":"ADT^A01","event_type":"made_up","priority":1}]`), kind: ErrUnsupportedSourceProfile},
		{name: "duplicate priority", json: validProfileJSONWith(`"event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1},{"message_type":"ADT^A01","condition":"PV1.2 == 'I'","event_type":"inpatient_admit","priority":1}]`), kind: ErrInvalidSourceProfile},
		{name: "multiple defaults", json: validProfileJSONWith(`"event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1},{"message_type":"ADT^A01","event_type":"inpatient_admit","priority":2}]`), kind: ErrInvalidSourceProfile},
		{name: "duplicate authority", json: validProfileJSONWith(`"identifiers":{"assigning_authorities":[{"code":"HOSP","system":"urn:oid:1"},{"code":"HOSP","system":"urn:oid:2"}]}`), kind: ErrInvalidSourceProfile},
		{name: "primary preference", json: validProfileJSONWith(`"identifiers":{"primary_id_preference":[{"type":"MR","priority":1}]}`), kind: ErrUnsupportedSourceProfile},
		{name: "validator settings", json: validProfileJSONWith(`"identifiers":{"validation":{"npi":{"enabled":true,"on_invalid":"warn"}}}`), kind: ErrUnsupportedSourceProfile},
		{name: "SSN no stripping", json: validProfileJSONWith(`"identifiers":{"normalization":{"ssn_strip_dashes":false}}`), kind: ErrUnsupportedSourceProfile},
		{name: "phone normalization", json: validProfileJSONWith(`"identifiers":{"normalization":{"phone_normalize":true}}`), kind: ErrUnsupportedSourceProfile},
		{name: "terminology mapping", json: validProfileJSONWith(`"terminology":{"mappings":[{"id":"local","source_system":"x","target_system":"y"}]}`), kind: ErrUnsupportedSourceProfile},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := compileSourceProfile(resolvedProfileForCompile(t, tt.json))
			if !errors.Is(err, tt.kind) {
				t.Fatalf("compileSourceProfile error = %v, want errors.Is(%v)", err, tt.kind)
			}
			if err != nil && (containsSensitive(err.Error(), "sentinel") || containsSensitive(err.Error(), "Mars/Olympus")) {
				t.Fatalf("compile error exposed profile content: %v", err)
			}
		})
	}
}

func TestCompileSourceProfilePreservesBuiltInSSNRejectPatternsWhenOmitted(t *testing.T) {
	t.Parallel()

	raw := validProfileJSONWith(`"identifiers":{"normalization":{"ssn_strip_dashes":true,"phone_normalize":false}}`)
	compiled, err := compileSourceProfile(resolvedProfileForCompile(t, raw))
	if err != nil {
		t.Fatalf("compileSourceProfile: %v", err)
	}
	if compiled.source.Identifiers == nil || compiled.source.Identifiers.Normalization == nil || compiled.source.Identifiers.Normalization.SSN == nil {
		t.Fatalf("missing compiled SSN normalization: %#v", compiled.source.Identifiers)
	}
	if compiled.source.Identifiers.Normalization.SSN.RejectPatterns != nil {
		t.Fatalf("omitted reject patterns became non-nil and disabled validator defaults: %#v", compiled.source.Identifiers.Normalization.SSN.RejectPatterns)
	}
}

func TestCompileSourceProfileRequiresOneDefaultAndSortsRules(t *testing.T) {
	t.Parallel()

	withoutDefault := validProfileJSONWith(`"event_classifications":[{"message_type":"ADT^A01","condition":"PV1.2 == 'I'","event_type":"inpatient_admit","priority":1}]`)
	if _, err := compileSourceProfile(resolvedProfileForCompile(t, withoutDefault)); !errors.Is(err, ErrInvalidSourceProfile) {
		t.Fatalf("missing default error = %v", err)
	}

	ordered := validProfileJSONWith(`"event_classifications":[
		{"message_type":"ADT^A01","condition":"PV1.2 == 'I'","event_type":"inpatient_admit","priority":20},
		{"message_type":"ADT^A01","event_type":"patient_admit","priority":100},
		{"message_type":"ADT^A01","condition":"PV1.2 == 'E'","event_type":"emergency_registration","priority":10}
	]`)
	compiled, err := compileSourceProfile(resolvedProfileForCompile(t, ordered))
	if err != nil {
		t.Fatalf("compileSourceProfile: %v", err)
	}
	want := []string{"PV1.2 == 'E'", "PV1.2 == 'I'"}
	got := []string{
		compiled.source.HL7v2.EventRules.ADTA01.Rules[0].Condition,
		compiled.source.HL7v2.EventRules.ADTA01.Rules[1].Condition,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("classification order = %#v, want %#v", got, want)
	}
}

func resolvedProfileForCompile(t *testing.T, raw string) ResolvedArtifactRevisions {
	t.Helper()
	ref, err := NewProfileRevisionReference("profile-adt", 7, []byte(raw))
	if err != nil {
		t.Fatalf("NewProfileRevisionReference: %v", err)
	}
	return ResolvedArtifactRevisions{
		profileRef:  ref,
		profileJSON: []byte(raw),
	}
}

func validProfileJSONWith(replacement string) string {
	fields := []string{
		`"default_version":"2.5.1"`,
		`"timezone":"UTC"`,
		`"event_classifications":[{"message_type":"ADT^A01","event_type":"patient_admit","priority":1}]`,
	}
	switch {
	case strings.HasPrefix(replacement, `"default_version"`):
		fields[0] = replacement
	case strings.HasPrefix(replacement, `"timezone"`):
		fields[1] = replacement
	case strings.HasPrefix(replacement, `"event_classifications"`):
		fields[2] = replacement
	case strings.HasPrefix(replacement, `"tolerance"`):
		fields = append(fields, replacement)
	default:
		return `{"hl7v2":{` + strings.Join(fields, `,`) + `},` + replacement + `}`
	}
	return `{"hl7v2":{` + strings.Join(fields, `,`) + `}}`
}

func containsSensitive(value, sensitive string) bool {
	for index := 0; index+len(sensitive) <= len(value); index++ {
		if value[index:index+len(sensitive)] == sensitive {
			return true
		}
	}
	return false
}
