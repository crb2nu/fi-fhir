package hl7v2

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

func TestParseWithResultExposesSourceMetadata(t *testing.T) {
	parser := NewParser("source", ParserConfig{StrictValidation: true})
	message := strictHardeningA01Message(
		"ADT^A01^ADT_A01",
		"CTRL-123",
		"2.5.1",
		"20240115080000-0500",
		"20240115090000-0500",
		"20240115110000-0500",
		"20240115100000-0500",
	)

	result, err := parser.ParseWithResult(message)
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}

	if result.MessageType != "ADT^A01^ADT_A01" {
		t.Errorf("MessageType = %q, want %q", result.MessageType, "ADT^A01^ADT_A01")
	}
	if result.ControlID != "CTRL-123" {
		t.Errorf("ControlID = %q, want %q", result.ControlID, "CTRL-123")
	}
	if result.MessageVersion != "2.5.1" {
		t.Errorf("MessageVersion = %q, want %q", result.MessageVersion, "2.5.1")
	}

	wantOccurredAt := time.Date(2024, time.January, 15, 16, 0, 0, 0, time.UTC)
	if !result.OccurredAt.Equal(wantOccurredAt) {
		t.Errorf("OccurredAt = %s, want instant %s", result.OccurredAt, wantOccurredAt)
	}
}

func TestParseRejectsMultipleMSHSegments(t *testing.T) {
	first := strictHardeningA01Message(
		"ADT^A01", "FIRST-CONTROL", "2.5", "20240115120000", "", "", "",
	)
	second := strictHardeningA01Message(
		"ADT^A01", "SECOND-CONTROL", "2.5", "20240115130000", "", "", "",
	)

	_, err := NewParser("source", ParserConfig{StrictValidation: true}).ParseWithResult(
		strings.Join([]string{first, second}, "\r"),
	)
	if err == nil {
		t.Fatal("ParseWithResult() error = nil, want duplicate MSH rejection")
	}
	if !strings.Contains(err.Error(), "multiple MSH") {
		t.Errorf("error = %q, want fixed multiple MSH error", err)
	}
	if strings.Contains(err.Error(), "SECOND-CONTROL") {
		t.Errorf("error exposed source data: %q", err)
	}
}

func TestParseHL7DTMAcceptsStandardDTMAndLegacyTS(t *testing.T) {
	location, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("time.LoadLocation() error = %v", err)
	}
	tests := []struct {
		name  string
		input string
		want  time.Time
	}{
		{name: "year", input: "2024", want: time.Date(2024, time.January, 1, 0, 0, 0, 0, location)},
		{name: "month", input: "202402", want: time.Date(2024, time.February, 1, 0, 0, 0, 0, location)},
		{name: "day", input: "20240229", want: time.Date(2024, time.February, 29, 0, 0, 0, 0, location)},
		{name: "hour", input: "2024022912", want: time.Date(2024, time.February, 29, 12, 0, 0, 0, location)},
		{name: "minute", input: "202402291234", want: time.Date(2024, time.February, 29, 12, 34, 0, 0, location)},
		{name: "second", input: "20240229123456", want: time.Date(2024, time.February, 29, 12, 34, 56, 0, location)},
		{name: "one fractional digit", input: "20240229123456.1", want: time.Date(2024, time.February, 29, 12, 34, 56, 100000000, location)},
		{name: "two fractional digits", input: "20240229123456.12", want: time.Date(2024, time.February, 29, 12, 34, 56, 120000000, location)},
		{name: "three fractional digits", input: "20240229123456.123", want: time.Date(2024, time.February, 29, 12, 34, 56, 123000000, location)},
		{name: "fraction", input: "20240229123456.1234", want: time.Date(2024, time.February, 29, 12, 34, 56, 123400000, location)},
		{name: "negative offset", input: "20240229123456.1234-0500", want: time.Date(2024, time.February, 29, 17, 34, 56, 123400000, time.UTC)},
		{name: "positive offset", input: "20240229123456+0230", want: time.Date(2024, time.February, 29, 10, 4, 56, 0, time.UTC)},
		{name: "maximum legal offset", input: "20240229123456+1400", want: time.Date(2024, time.February, 28, 22, 34, 56, 0, time.UTC)},
		{name: "known UTC offset", input: "20240229123456+0000", want: time.Date(2024, time.February, 29, 12, 34, 56, 0, time.UTC)},
		{name: "unknown UTC offset", input: "20240229123456-0000", want: time.Date(2024, time.February, 29, 12, 34, 56, 0, time.UTC)},
		{name: "legacy year precision", input: "20240229123456^Y", want: time.Date(2024, time.January, 1, 0, 0, 0, 0, location)},
		{name: "legacy month precision", input: "20240229123456^L", want: time.Date(2024, time.February, 1, 0, 0, 0, 0, location)},
		{name: "legacy day precision", input: "20240229123456^D", want: time.Date(2024, time.February, 29, 0, 0, 0, 0, location)},
		{name: "legacy hour precision", input: "20240229123456^H", want: time.Date(2024, time.February, 29, 12, 0, 0, 0, location)},
		{name: "legacy minute precision", input: "20240229123456^M", want: time.Date(2024, time.February, 29, 12, 34, 0, 0, location)},
		{name: "legacy second precision", input: "20240229123456.1234^S", want: time.Date(2024, time.February, 29, 12, 34, 56, 0, location)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parsed, parseErr := parseHL7DTM(tt.input, '^', location)
			if parseErr != nil {
				t.Fatalf("parseHL7DTM() error = %v", parseErr)
			}
			if !parsed.Time.Equal(tt.want) {
				t.Errorf("parseHL7DTM() = %s, want instant %s", parsed.Time, tt.want)
			}
		})
	}
}

func TestParseHL7DTMUsesMessageComponentSeparator(t *testing.T) {
	parsed, err := parseHL7DTM("20240229123456%S", '%', time.UTC)
	if err != nil {
		t.Fatalf("parseHL7DTM() error = %v", err)
	}
	if parsed.Precision != 14 {
		t.Errorf("Precision = %d, want 14", parsed.Precision)
	}
	want := time.Date(2024, time.February, 29, 12, 34, 56, 0, time.UTC)
	if !parsed.Time.Equal(want) {
		t.Errorf("Time = %s, want instant %s", parsed.Time, want)
	}
}

func TestParseHL7DTMExplicitOffsetIsIndependentOfHostLocalTimezone(t *testing.T) {
	hostLocation, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Fatalf("time.LoadLocation() error = %v", err)
	}
	previousLocal := time.Local
	time.Local = hostLocation
	defer func() { time.Local = previousLocal }()

	parsed, err := parseHL7DTM("20240310123456-0400^D", '^', time.UTC)
	if err != nil {
		t.Fatalf("parseHL7DTM() error = %v", err)
	}
	_, offsetSeconds := parsed.Time.Zone()
	if offsetSeconds != -4*60*60 {
		t.Fatalf("normalized explicit offset = %d, want -14400", offsetSeconds)
	}
	want := time.Date(2024, time.March, 10, 4, 0, 0, 0, time.UTC)
	if !parsed.Time.Equal(want) {
		t.Fatalf("normalized explicit instant = %s, want %s", parsed.Time, want)
	}
}

func TestParseHL7DTMRejectsMalformedValuesWithoutEchoingInput(t *testing.T) {
	tests := []string{
		"20241",
		"2024011512345",
		"20240115123456.",
		"20240115123456.12345",
		"20240115123456-05:00",
		"20240115123456Z",
		"20240115123456-1260",
		"20240115123456+1401",
		"20240230120000",
		"20240115120000SECRET-SENSITIVE",
		" 20240115120000",
		"20240115120000^Q",
		"20240115120000^S^S",
	}

	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			_, err := parseHL7DTM(input, '^', time.UTC)
			if err == nil {
				t.Fatal("parseHL7DTM() error = nil, want malformed DTM rejection")
			}
			if strings.Contains(err.Error(), input) || strings.Contains(err.Error(), "SECRET") {
				t.Errorf("error exposed source value: %q", err)
			}
		})
	}
}

func TestADTA01OccurredAtPrecedence(t *testing.T) {
	tests := []struct {
		name       string
		msh7       string
		evn2       string
		evn6       string
		pv1Field44 string
		wantUTC    time.Time
	}{
		{
			name:       "EVN-6 actual occurrence wins",
			msh7:       "20240115080000-0500",
			evn2:       "20240115090000-0500",
			evn6:       "20240115110000-0500",
			pv1Field44: "20240115100000-0500",
			wantUTC:    time.Date(2024, time.January, 15, 16, 0, 0, 0, time.UTC),
		},
		{
			name:       "PV1-44 admit time is second",
			msh7:       "20240115080000-0500",
			evn2:       "20240115090000-0500",
			pv1Field44: "20240115100000-0500",
			wantUTC:    time.Date(2024, time.January, 15, 15, 0, 0, 0, time.UTC),
		},
		{
			name:    "EVN-2 recorded time is third",
			msh7:    "20240115080000-0500",
			evn2:    "20240115090000-0500",
			wantUTC: time.Date(2024, time.January, 15, 14, 0, 0, 0, time.UTC),
		},
		{
			name:    "MSH-7 message creation is last",
			msh7:    "20240115080000-0500",
			wantUTC: time.Date(2024, time.January, 15, 13, 0, 0, 0, time.UTC),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser("source", ParserConfig{DefaultTimezone: time.UTC, StrictValidation: true})
			result, err := parser.ParseWithResult(strictHardeningA01Message(
				"ADT^A01", "CTRL", "2.5", tt.msh7, tt.evn2, tt.evn6, tt.pv1Field44,
			))
			if err != nil {
				t.Fatalf("ParseWithResult() error = %v", err)
			}

			occurredAt := result.OccurredAt
			if !occurredAt.Equal(tt.wantUTC) {
				t.Errorf("OccurredAt = %s, want instant %s", occurredAt, tt.wantUTC)
			}
			event := result.Event.(*events.PatientAdmitEvent)
			if !event.Timestamp.Equal(tt.wantUTC) {
				t.Errorf("event.Timestamp = %s, want instant %s", event.Timestamp, tt.wantUTC)
			}
		})
	}
}

func TestADTA01UsesMessageTimezoneAndKeepsAdmitTimeConsistent(t *testing.T) {
	parser := NewParser("source", ParserConfig{DefaultTimezone: time.UTC, StrictValidation: true})
	result, err := parser.ParseWithResult(strictHardeningA01Message(
		"ADT^A01",
		"CTRL",
		"2.5",
		"20240115080000-0500",
		"",
		"",
		"20240115100000",
	))
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}

	want := time.Date(2024, time.January, 15, 15, 0, 0, 0, time.UTC)
	occurredAt := result.OccurredAt
	if !occurredAt.Equal(want) {
		t.Errorf("OccurredAt = %s, want instant %s", occurredAt, want)
	}
	event := result.Event.(*events.PatientAdmitEvent)
	if !event.Encounter.AdmitDateTime.Equal(want) {
		t.Errorf("Encounter.AdmitDateTime = %s, want instant %s", event.Encounter.AdmitDateTime, want)
	}
}

func TestADTA01UsesProfileTimezoneWithoutMessageOffset(t *testing.T) {
	location, err := time.LoadLocation("America/Los_Angeles")
	if err != nil {
		t.Fatalf("time.LoadLocation() error = %v", err)
	}
	parser := NewParser("source", ParserConfig{DefaultTimezone: location, StrictValidation: true})
	result, err := parser.ParseWithResult(strictHardeningA01Message(
		"ADT^A01",
		"CTRL",
		"2.5",
		"20240115080000",
		"",
		"20240115110000",
		"",
	))
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}

	want := time.Date(2024, time.January, 15, 19, 0, 0, 0, time.UTC)
	if !result.OccurredAt.Equal(want) {
		t.Errorf("OccurredAt = %s, want instant %s", result.OccurredAt, want)
	}
}

func TestADTA01CandidateOffsetOverridesMessageTimezone(t *testing.T) {
	parser := NewParser("source", ParserConfig{DefaultTimezone: time.UTC, StrictValidation: true})
	result, err := parser.ParseWithResult(strictHardeningA01Message(
		"ADT^A01",
		"CTRL",
		"2.5",
		"20240115080000-0500",
		"",
		"20240115110000+0200",
		"",
	))
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}

	want := time.Date(2024, time.January, 15, 9, 0, 0, 0, time.UTC)
	if !result.OccurredAt.Equal(want) {
		t.Errorf("OccurredAt = %s, want instant %s", result.OccurredAt, want)
	}
}

func TestADTA01InvalidAndImpreciseTimesWarnAndFallThrough(t *testing.T) {
	tests := []struct {
		name        string
		evn6        string
		warningCode string
	}{
		{name: "invalid", evn6: "20240115120000.SECRET-SENSITIVE", warningCode: "INVALID_DTM"},
		{name: "imprecise", evn6: "2024", warningCode: "IMPRECISE_EVENT_TIME"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := NewParser("source", ParserConfig{DefaultTimezone: time.UTC, StrictValidation: true})
			result, err := parser.ParseWithResult(strictHardeningA01Message(
				"ADT^A01",
				"CTRL",
				"2.5",
				"20240115080000-0500",
				"20240115090000-0500",
				tt.evn6,
				"20240115100000-0500",
			))
			if err != nil {
				t.Fatalf("ParseWithResult() error = %v", err)
			}

			want := time.Date(2024, time.January, 15, 15, 0, 0, 0, time.UTC)
			if !result.OccurredAt.Equal(want) {
				t.Errorf("OccurredAt = %s, want fallback instant %s", result.OccurredAt, want)
			}

			warning, ok := hardeningWarningByCode(result.Warnings, tt.warningCode)
			if !ok {
				t.Fatalf("warnings = %+v, want %s", result.Warnings, tt.warningCode)
			}
			if warning.Path != "EVN.6" {
				t.Errorf("warning.Path = %q, want EVN.6", warning.Path)
			}
			if strings.Contains(warning.Message, tt.evn6) || strings.Contains(warning.Message, "SECRET") {
				t.Errorf("warning exposed source value: %+v", warning)
			}
		})
	}
}

func TestADTA01HasNoParserClockFallback(t *testing.T) {
	parser := NewParser("source", ParserConfig{StrictValidation: true})
	result, err := parser.ParseWithResult(strictHardeningA01Message(
		"ADT^A01", "CTRL", "2.5", "", "", "", "",
	))
	if err != nil {
		t.Fatalf("ParseWithResult() error = %v", err)
	}

	if !result.OccurredAt.IsZero() {
		t.Errorf("OccurredAt = %s, want zero source time", result.OccurredAt)
	}
	event := result.Event.(*events.PatientAdmitEvent)
	if !event.Timestamp.IsZero() {
		t.Errorf("event.Timestamp = %s, want zero without a source time", event.Timestamp)
	}
}

func TestLegacyADTA01MetadataRemainsCompatible(t *testing.T) {
	parser := NewParser("source", ParserConfig{DefaultTimezone: time.UTC})
	message := hardeningA01Message(
		"ADT^A01", "CTRL", "2.5", "20240115080000-0500",
		"20240115090000-0500", "20240115110000-0500", "20240115100000-0500",
	)
	before := time.Now().UTC()
	result, err := parser.ParseWithResult(message)
	after := time.Now().UTC()
	if err != nil {
		t.Fatalf("legacy ParseWithResult() error = %v", err)
	}
	if !result.OccurredAt.IsZero() {
		t.Fatalf("legacy OccurredAt = %s, want zero additive metadata", result.OccurredAt)
	}
	event := result.Event.(*events.PatientAdmitEvent)
	if event.Timestamp.Before(before) || event.Timestamp.After(after) {
		t.Fatalf("legacy event timestamp changed to source time: %s", event.Timestamp)
	}
	wantLegacyAdmit := time.Date(2024, time.January, 15, 10, 0, 0, 0, time.UTC)
	if !event.Encounter.AdmitDateTime.Equal(wantLegacyAdmit) {
		t.Fatalf("legacy admit time = %s, want prior parser behavior %s", event.Encounter.AdmitDateTime, wantLegacyAdmit)
	}
}

func TestStrictADTA01UsesMessageTimezoneForEncounterTimes(t *testing.T) {
	message := strictHardeningA01Message(
		"ADT^A01", "CTRL", "2.5", "20240115080000-0500", "", "", "20240115100000",
	)
	message = strictReplaceField(t, message, "PV1", 45, "20240115113000")
	result, err := NewParser("source", ParserConfig{DefaultTimezone: time.UTC, StrictValidation: true}).ParseWithResult(message)
	if err != nil {
		t.Fatalf("strict ParseWithResult() error = %v", err)
	}
	event := result.Event.(*events.PatientAdmitEvent)
	wantAdmit := time.Date(2024, time.January, 15, 15, 0, 0, 0, time.UTC)
	wantDischarge := time.Date(2024, time.January, 15, 16, 30, 0, 0, time.UTC)
	if !event.Encounter.AdmitDateTime.Equal(wantAdmit) || !event.Encounter.DischargeDateTime.Equal(wantDischarge) {
		t.Fatalf("strict encounter times = admit %s discharge %s, want %s / %s", event.Encounter.AdmitDateTime, event.Encounter.DischargeDateTime, wantAdmit, wantDischarge)
	}
}

func hardeningA01Message(
	messageType string,
	controlID string,
	version string,
	msh7 string,
	evn2 string,
	evn6 string,
	pv1Field44 string,
) string {
	msh := fmt.Sprintf(
		`MSH|^~\&|APP|FAC|APP|FAC|%s||%s|%s|P|%s`,
		msh7,
		messageType,
		controlID,
		version,
	)
	evn := hardeningSegment("EVN", 6, map[int]string{1: "A01", 2: evn2, 6: evn6})
	pid := hardeningSegment("PID", 8, map[int]string{
		1: "1", 3: "123456789^^^FAC^MR", 5: "DOE^JANE", 7: "19800101", 8: "F",
	})
	pv1 := hardeningSegment("PV1", 45, map[int]string{
		1: "1", 2: "I", 3: "ICU^101^A^FAC", 19: "VISIT-1", 44: pv1Field44,
	})
	return strings.Join([]string{msh, evn, pid, pv1}, "\n")
}

func strictHardeningA01Message(
	messageType string,
	controlID string,
	version string,
	msh7 string,
	evn2 string,
	evn6 string,
	pv1Field44 string,
) string {
	return strings.ReplaceAll(hardeningA01Message(messageType, controlID, version, msh7, evn2, evn6, pv1Field44), "\n", "\r")
}

func hardeningSegment(id string, lastField int, values map[int]string) string {
	fields := make([]string, lastField+1)
	fields[0] = id
	for index, value := range values {
		fields[index] = value
	}
	return strings.Join(fields, "|")
}

func hardeningWarningByCode(warnings []events.ParseWarning, code string) (events.ParseWarning, bool) {
	for _, warning := range warnings {
		if warning.Code == code {
			return warning, true
		}
	}
	return events.ParseWarning{}, false
}
