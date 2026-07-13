package hl7v2

import (
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/profile"
)

func TestStrictValidationAcceptsBoundedA01UTF8(t *testing.T) {
	message := strictReplaceField(t, strictA01Message(), "PID", 5, "GARCÍA^JANE")
	result, err := newStrictA01Parser(profile.ToleranceConfig{}, "strict", true).ParseWithResult(message)
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}
	if result.MessageType != "ADT^A01^ADT_A01" {
		t.Errorf("MessageType = %q, want ADT^A01^ADT_A01", result.MessageType)
	}
}

func TestStrictValidationRejectsInvalidUTF8WithoutEchoingInput(t *testing.T) {
	invalidName := string([]byte{'S', 'E', 'N', 'T', 'I', 'N', 'E', 'L', '^', 0xff})
	message := strictReplaceField(t, strictA01Message(), "PID", 5, invalidName)

	_, err := newStrictA01Parser(profile.ToleranceConfig{}, "strict", true).ParseWithResult(message)
	if err == nil {
		t.Fatal("ParseWithResult() error = nil, want invalid UTF-8 rejection")
	}
	if strings.Contains(err.Error(), "SENTINEL") || strings.Contains(err.Error(), invalidName) {
		t.Errorf("error exposed source data: %q", err)
	}
}

func TestStrictValidationEnforcesClaimedLineEndingMode(t *testing.T) {
	message := strictA01Message()
	tests := []struct {
		name    string
		message string
		wantErr bool
	}{
		{name: "CR", message: message},
		{name: "LF", message: strings.ReplaceAll(message, "\r", "\n"), wantErr: true},
		{name: "CRLF", message: strings.ReplaceAll(message, "\r", "\r\n"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newStrictA01Parser(profile.ToleranceConfig{}, "strict", true).ParseWithResult(tt.message)
			if tt.wantErr && err == nil {
				t.Fatal("ParseWithResult() error = nil, want strict line-ending rejection")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("ParseWithResult() error = %v", err)
			}
		})
	}
}

func TestStrictValidationWarnsWhenLineEndingsAreExplicitlyTolerated(t *testing.T) {
	message := strings.ReplaceAll(strictA01Message(), "\r", "\n")
	result, err := newStrictA01Parser(profile.ToleranceConfig{}, "tolerant", true).ParseWithResult(message)
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}
	if _, ok := hardeningWarningByCode(result.Warnings, "NON_STANDARD_LINE_ENDING"); !ok {
		t.Fatalf("warnings = %+v, want NON_STANDARD_LINE_ENDING", result.Warnings)
	}
}

func TestStrictValidationEnforcesDelimiterTolerance(t *testing.T) {
	message := strings.Replace(strictA01Message(), `MSH|^~\&|`, `MSH|^!\&|`, 1)

	_, err := newStrictA01Parser(profile.ToleranceConfig{}, "strict", true).ParseWithResult(message)
	if err == nil {
		t.Fatal("ParseWithResult() error = nil, want non-standard delimiter rejection")
	}

	tolerant := profile.ToleranceConfig{NonStandardDelimiters: true}
	result, err := newStrictA01Parser(tolerant, "strict", true).ParseWithResult(message)
	if err != nil {
		t.Fatalf("tolerant ParseWithResult() error = %v", err)
	}
	if _, ok := hardeningWarningByCode(result.Warnings, "NON_STANDARD_DELIMITERS"); !ok {
		t.Fatalf("warnings = %+v, want NON_STANDARD_DELIMITERS", result.Warnings)
	}
}

func TestStrictValidationEnforcesBoundedSegmentAllowlist(t *testing.T) {
	message := strictInsertAfter(t, strictA01Message(), "PID", "PD1|SECRET-SENTINEL")

	_, err := newStrictA01Parser(profile.ToleranceConfig{}, "strict", true).ParseWithResult(message)
	if err == nil {
		t.Fatal("ParseWithResult() error = nil, want unsupported segment rejection")
	}
	if strings.Contains(err.Error(), "PD1") || strings.Contains(err.Error(), "SECRET") {
		t.Errorf("error exposed source data: %q", err)
	}

	tolerant := profile.ToleranceConfig{UnknownSegments: true}
	result, err := newStrictA01Parser(tolerant, "strict", true).ParseWithResult(message)
	if err != nil {
		t.Fatalf("tolerant ParseWithResult() error = %v", err)
	}
	warning, ok := hardeningWarningByCode(result.Warnings, "UNKNOWN_SEGMENT")
	if !ok {
		t.Fatalf("warnings = %+v, want UNKNOWN_SEGMENT", result.Warnings)
	}
	if strings.Contains(warning.Message, "PD1") || strings.Contains(warning.Message, "SECRET") {
		t.Errorf("warning exposed source data: %+v", warning)
	}
}

func TestStrictValidationEnforcesConsumedFieldComponentLimits(t *testing.T) {
	tests := []struct {
		name      string
		segmentID string
		field     int
		value     string
	}{
		{name: "EVN-2 TS", segmentID: "EVN", field: 2, value: "20240115090000^S^EXTRA"},
		{name: "PID-3 CX", segmentID: "PID", field: 3, value: "123^^^FAC^MR^ASSIGNING_FACILITY^EXTRA"},
		{name: "PID-5 XPN subset", segmentID: "PID", field: 5, value: "DOE^JANE^MIDDLE^SUFFIX^PREFIX^EXTRA"},
		{name: "PID-11 XAD subset", segmentID: "PID", field: 11, value: "LINE1^LINE2^CITY^STATE^ZIP^US^EXTRA"},
		{name: "PV1-3 PL subset", segmentID: "PV1", field: 3, value: "ICU^101^A^FAC^EXTRA"},
		{name: "PV1-7 XCN subset", segmentID: "PV1", field: 7, value: "123^DOE^JANE^EXTRA"},
		{name: "PV1-44 TS", segmentID: "PV1", field: 44, value: "20240115100000^S^EXTRA"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			message := strictReplaceField(t, strictA01Message(), tt.segmentID, tt.field, tt.value)
			_, err := newStrictA01Parser(profile.ToleranceConfig{}, "strict", true).ParseWithResult(message)
			if err == nil {
				t.Fatal("ParseWithResult() error = nil, want extra component rejection")
			}
			if strings.Contains(err.Error(), "EXTRA") {
				t.Errorf("error exposed source data: %q", err)
			}
		})
	}
}

func TestStrictValidationWarnsWhenExtraComponentsAreExplicitlyTolerated(t *testing.T) {
	message := strictReplaceField(
		t,
		strictA01Message(),
		"PID",
		5,
		"DOE^JANE^MIDDLE^SUFFIX^PREFIX^SECRET-SENTINEL",
	)
	tolerant := profile.ToleranceConfig{ExtraComponents: true}
	result, err := newStrictA01Parser(tolerant, "strict", true).ParseWithResult(message)
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}
	warning, ok := hardeningWarningByCode(result.Warnings, "EXTRA_COMPONENTS")
	if !ok {
		t.Fatalf("warnings = %+v, want EXTRA_COMPONENTS", result.Warnings)
	}
	if strings.Contains(warning.Message, "SECRET") {
		t.Errorf("warning exposed source data: %+v", warning)
	}
}

func TestStrictValidationEnforcesNTEPlacement(t *testing.T) {
	valid := strictInsertAfter(t, strictA01Message(), "PID", "NTE|1||patient note")
	if _, err := newStrictA01Parser(profile.ToleranceConfig{}, "strict", true).ParseWithResult(valid); err != nil {
		t.Fatalf("valid NTE placement error = %v", err)
	}

	invalid := strictInsertAfter(t, strictA01Message(), "EVN", "NTE|1||SECRET-SENTINEL")
	_, err := newStrictA01Parser(profile.ToleranceConfig{}, "strict", true).ParseWithResult(invalid)
	if err == nil {
		t.Fatal("ParseWithResult() error = nil, want misplaced NTE rejection")
	}
	if strings.Contains(err.Error(), "SECRET") {
		t.Errorf("error exposed source data: %q", err)
	}

	tolerant := profile.ToleranceConfig{NTEAnywhere: true}
	result, err := newStrictA01Parser(tolerant, "strict", true).ParseWithResult(invalid)
	if err != nil {
		t.Fatalf("tolerant ParseWithResult() error = %v", err)
	}
	if _, ok := hardeningWarningByCode(result.Warnings, "NTE_OUT_OF_ORDER"); !ok {
		t.Fatalf("warnings = %+v, want NTE_OUT_OF_ORDER", result.Warnings)
	}
}

func TestStrictValidationEnforcesA01CoreStructure(t *testing.T) {
	base := strictA01Message()
	segments := strings.Split(base, "\r")
	pid := strictSegment(t, base, "PID")

	tests := []struct {
		name    string
		message string
	}{
		{name: "unsupported trigger", message: strings.Replace(base, "ADT^A01^ADT_A01", "ADT^A04^ADT_A01", 1)},
		{name: "wrong structure", message: strings.Replace(base, "ADT^A01^ADT_A01", "ADT^A01^OTHER", 1)},
		{name: "missing EVN", message: strictWithoutSegment(t, base, "EVN")},
		{name: "duplicate PID", message: strictInsertAfter(t, base, "PID", pid)},
		{name: "PV1 before PID", message: strings.Join([]string{segments[0], segments[1], segments[3], segments[2]}, "\r")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := newStrictA01Parser(profile.ToleranceConfig{}, "strict", true).ParseWithResult(tt.message)
			if err == nil {
				t.Fatal("ParseWithResult() error = nil, want strict A01 structure rejection")
			}
		})
	}
}

func TestStrictValidationOnlyToleratesMissingPV1WhenProfileAllowsIt(t *testing.T) {
	message := strictWithoutSegment(t, strictA01Message(), "PV1")
	if _, err := newStrictA01Parser(profile.ToleranceConfig{}, "strict", true).ParseWithResult(message); err == nil {
		t.Fatal("ParseWithResult() error = nil, want missing PV1 rejection")
	}

	tolerant := profile.ToleranceConfig{MissingSegments: []string{"PV1"}}
	result, err := newStrictA01Parser(tolerant, "strict", true).ParseWithResult(message)
	if err != nil {
		t.Fatalf("tolerant ParseWithResult() error = %v", err)
	}
	if _, ok := hardeningWarningByCode(result.Warnings, "MISSING_PV1"); !ok {
		t.Fatalf("warnings = %+v, want MISSING_PV1", result.Warnings)
	}
}

func TestLegacyValidationPathRemainsPermissive(t *testing.T) {
	message := strings.ReplaceAll(strictA01Message(), "\r", "\n")
	message = strings.Replace(message, `MSH|^~\&|`, `MSH|^!\&|`, 1)
	message = strictReplaceField(t, message, "PID", 5, "DOE^JANE^MIDDLE^SUFFIX^PREFIX^EXTRA")
	message = strictInsertAfter(t, message, "EVN", "NTE|1||misplaced note")
	message = strictInsertAfter(t, message, "PID", "PD1|unknown in bounded subset")

	result, err := newStrictA01Parser(profile.ToleranceConfig{}, "strict", false).ParseWithResult(message)
	if err != nil {
		t.Fatalf("legacy ParseWithResult() error = %v", err)
	}
	for _, code := range []string{
		"NON_STANDARD_LINE_ENDING",
		"NON_STANDARD_DELIMITERS",
		"UNKNOWN_SEGMENT",
		"EXTRA_COMPONENTS",
		"NTE_OUT_OF_ORDER",
	} {
		if _, ok := hardeningWarningByCode(result.Warnings, code); ok {
			t.Errorf("legacy warnings unexpectedly contain %s: %+v", code, result.Warnings)
		}
	}
}

func strictA01Message() string {
	message := hardeningA01Message(
		"ADT^A01^ADT_A01",
		"CTRL-STRICT-1",
		"2.5.1",
		"20240115080000-0500",
		"20240115090000",
		"",
		"20240115100000",
	)
	return strings.ReplaceAll(message, "\n", "\r")
}

func newStrictA01Parser(
	tolerance profile.ToleranceConfig,
	lineEndingMode string,
	strict bool,
) *Parser {
	parser := NewParser("strict-source", ParserConfig{
		DefaultTimezone:  time.UTC,
		StrictValidation: strict,
	})
	parser.SetProfile(&profile.SourceProfile{
		ID:      "compiled-profile",
		Name:    "Compiled Preview Profile",
		Version: "revision-1",
		HL7v2: &profile.HL7v2Config{
			DefaultVersion: "2.5.1",
			Timezone:       "UTC",
			Encoding: &profile.EncodingConfig{
				CharsetDefault:   "UTF-8",
				CharsetDetection: false,
				LineEndingMode:   lineEndingMode,
			},
			Tolerate: &tolerance,
			EventRules: &profile.EventRulesConfig{
				ADTA01: &profile.EventRule{Default: "patient_admit"},
			},
		},
	})
	return parser
}

func strictReplaceField(t *testing.T, message, segmentID string, field int, value string) string {
	t.Helper()
	separator := strictLineSeparator(message)
	segments := strings.Split(message, separator)
	for i, segment := range segments {
		if !strings.HasPrefix(segment, segmentID+"|") {
			continue
		}
		fields := strings.Split(segment, "|")
		if field >= len(fields) {
			extended := make([]string, field+1)
			copy(extended, fields)
			fields = extended
		}
		fields[field] = value
		segments[i] = strings.Join(fields, "|")
		return strings.Join(segments, separator)
	}
	t.Fatalf("segment %s not found", segmentID)
	return ""
}

func strictInsertAfter(t *testing.T, message, segmentID, inserted string) string {
	t.Helper()
	separator := strictLineSeparator(message)
	segments := strings.Split(message, separator)
	result := make([]string, 0, len(segments)+1)
	for i, segment := range segments {
		result = append(result, segment)
		if strings.HasPrefix(segment, segmentID+"|") {
			result = append(result, inserted)
			result = append(result, segments[i+1:]...)
			return strings.Join(result, separator)
		}
	}
	t.Fatalf("segment %s not found", segmentID)
	return ""
}

func strictWithoutSegment(t *testing.T, message, segmentID string) string {
	t.Helper()
	separator := strictLineSeparator(message)
	segments := strings.Split(message, separator)
	result := make([]string, 0, len(segments)-1)
	found := false
	for _, segment := range segments {
		if strings.HasPrefix(segment, segmentID+"|") && !found {
			found = true
			continue
		}
		result = append(result, segment)
	}
	if !found {
		t.Fatalf("segment %s not found", segmentID)
	}
	return strings.Join(result, separator)
}

func strictSegment(t *testing.T, message, segmentID string) string {
	t.Helper()
	for _, segment := range strings.Split(message, strictLineSeparator(message)) {
		if strings.HasPrefix(segment, segmentID+"|") {
			return segment
		}
	}
	t.Fatalf("segment %s not found", segmentID)
	return ""
}

func strictLineSeparator(message string) string {
	if strings.Contains(message, "\r\n") {
		return "\r\n"
	}
	if strings.Contains(message, "\r") {
		return "\r"
	}
	return "\n"
}
