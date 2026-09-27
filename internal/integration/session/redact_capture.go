package session

import (
	"fmt"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// SampleRedaction selects the redactor AddSample applies under PHIPolicyRedact.
//
// The default is the pasted-sample redactor, redactHL7v2, whose behaviour is
// pinned and unchanged. SampleRedactionCapture selects RedactCapturedHL7v2 for
// messages captured from a live source or peeked from a batch object
// (.loom/38 Decision 6): a live feed carries PHI an engineer's synthetic sample
// does not, so its copy is masked more aggressively.
type SampleRedaction string

const (
	// SampleRedactionDefault is the pasted-sample redactor.
	SampleRedactionDefault SampleRedaction = ""
	// SampleRedactionCapture is the capture redactor. It is redact-only: a
	// captured sample is never retained, so it cannot be combined with
	// PHIPolicyRetain.
	SampleRedactionCapture SampleRedaction = "capture"
)

// redactedValue replaces a masked field, exactly as redactHL7v2 does. An empty
// field stays empty, so the message keeps its shape and a reader can still see
// which fields the sender populated.
const redactedValue = "REDACTED"

// CaptureRedactedFields is the capture redactor's field table: for each
// segment, the HL7 v2.5.1 field numbers whose whole value — every repetition
// and component — is replaced. docs/operations/PHI-RETENTION.md ("Captured and
// peeked samples") publishes the same table with each field's name, and
// TestRedactCapturedHL7v2_MasksEveryTableField pins that every field listed
// here is masked and every field not listed survives.
//
// The rule it encodes: every field that names, locates, contacts, dates, or
// numbers a person — the patient, the next of kin and associated parties, the
// insured, the guarantor, and their employers and household — and every person
// name in these segments whoever it belongs to. Dates are masked whole, year
// included: stricter than Safe Harbor's year exception, and required for the
// superset, because redactHL7v2 already masks PID-7 whole. Fields that identify
// the payer's organisation and product (IN1-2 through IN1-5, IN1-7, IN2-25,
// IN2-58) are kept: they name no person, and a profile maps them to Coverage.
var CaptureRedactedFields = map[string][]int{
	"PID": {2, 3, 4, 5, 6, 7, 9, 11, 12, 13, 14, 18, 19, 20, 21, 23, 29},
	"NK1": {2, 4, 5, 6, 8, 9, 12, 13, 16, 26, 30, 31, 32, 33, 37, 38},
	"IN1": {6, 8, 9, 10, 11, 12, 13, 14, 16, 18, 19, 24, 26, 28, 29, 30, 36, 44, 49, 51, 52},
	"IN2": {1, 2, 3, 6, 7, 8, 9, 10, 13, 17, 22, 26, 40, 44, 45, 49, 50, 52, 53, 55, 56, 61, 63, 64, 69, 70},
	"GT1": {2, 3, 4, 5, 6, 7, 8, 12, 13, 14, 16, 17, 18, 19, 21, 24, 29, 31, 32, 42, 45, 46, 51, 56},
}

// RedactCapturedHL7v2 masks every field CaptureRedactedFields lists in the PID,
// NK1, IN1, IN2, and GT1 segments of one HL7v2 message. It is a superset of
// redactHL7v2: every PID field that redactor masks is in the table.
//
// The field separator is read from MSH-1 rather than assumed, because a
// message that declares a different one would otherwise pass through with
// nothing masked. Segments are normalised to LF-separated lines, as
// redactHL7v2 normalises them. Segments outside the five — MSH, EVN, PV1, OBX,
// NTE, Z-segments — are not masked; a captured sample therefore stays PHI in
// the session and is governed by the session's retention, not treated as
// de-identified.
func RedactCapturedHL7v2(raw string) string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	separator := hl7FieldSeparator(lines)
	for index, line := range lines {
		if len(line) < 4 || line[3] != separator {
			continue
		}
		masked, ok := CaptureRedactedFields[line[:3]]
		if !ok {
			continue
		}
		fields := strings.Split(line, string(separator))
		for _, field := range masked {
			if field < len(fields) && fields[field] != "" {
				fields[field] = redactedValue
			}
		}
		lines[index] = strings.Join(fields, string(separator))
	}
	return strings.Join(lines, "\n")
}

// hl7FieldSeparator is MSH-1, the character that follows the segment name of
// the first MSH segment, or '|' when there is none.
func hl7FieldSeparator(lines []string) byte {
	for _, line := range lines {
		if len(line) >= 4 && strings.HasPrefix(line, "MSH") {
			return line[3]
		}
	}
	return '|'
}

// redactSampleWith applies the requested redactor. An unknown redaction is
// refused rather than defaulted, so a caller can never silently get the weaker
// one.
func redactSampleWith(redaction SampleRedaction, format events.SourceFormat, raw string) (string, error) {
	switch redaction {
	case SampleRedactionDefault:
		return redactSample(format, raw), nil
	case SampleRedactionCapture:
		if format != events.FormatHL7v2 {
			return "[redacted]", nil
		}
		return RedactCapturedHL7v2(raw), nil
	default:
		return "", fmt.Errorf("%w: unsupported sample redaction %q", ErrInvalid, redaction)
	}
}

// validateSampleRedaction refuses a redaction the policy cannot honour: the
// capture redactor is redact-only.
func validateSampleRedaction(redaction SampleRedaction, policy PHIPolicy) error {
	switch redaction {
	case SampleRedactionDefault:
		return nil
	case SampleRedactionCapture:
		if policy != PHIPolicyRedact {
			return fmt.Errorf("%w: a captured sample is never retained", ErrInvalid)
		}
		return nil
	default:
		return fmt.Errorf("%w: unsupported sample redaction %q", ErrInvalid, redaction)
	}
}
