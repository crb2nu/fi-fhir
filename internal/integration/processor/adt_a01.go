package processor

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/hl7v2"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const deterministicEventIDDomain = "fi-fhir/event-id/v1\x00"

var (
	// ErrInvalidADTA01 means parser output cannot be projected without ambiguity.
	ErrInvalidADTA01 = errors.New("invalid ADT A01 parser result")
	// ErrUnsupportedADTA01 means the source message is outside the executable v1 subset.
	ErrUnsupportedADTA01 = errors.New("unsupported ADT A01 source message")
)

func projectADTA01(
	result *hl7v2.ParseResult,
	request integration.ProcessRequest,
	revision integration.IntegrationDefinitionRevision,
	ordinal uint32,
) (integration.ProcessedEvent, []integration.Diagnostic, error) {
	if err := request.ValidateAgainst(revision); err != nil ||
		(request.Mode != integration.ExecutionModePreview && request.Mode != integration.ExecutionModeProduction) {
		return integration.ProcessedEvent{}, nil, ErrInvalidADTA01
	}
	if result == nil {
		return integration.ProcessedEvent{}, nil, ErrInvalidADTA01
	}
	parsed, ok := result.Event.(*events.PatientAdmitEvent)
	if !ok || parsed == nil {
		return integration.ProcessedEvent{}, nil, ErrInvalidADTA01
	}
	if result.MessageType != hl7v2.MsgADT_A01 && result.MessageType != "ADT^A01^ADT_A01" {
		return integration.ProcessedEvent{}, nil, ErrUnsupportedADTA01
	}
	if !supportedHL7Version(result.MessageVersion) {
		return integration.ProcessedEvent{}, nil, ErrUnsupportedADTA01
	}
	if !validA01MetadataToken(result.ControlID, 199) || result.ControlID != parsed.SourceMessageID {
		return integration.ProcessedEvent{}, nil, ErrInvalidADTA01
	}
	if parsed.Type != events.EventPatientAdmit || parsed.SourceFormat != events.FormatHL7v2 {
		return integration.ProcessedEvent{}, nil, ErrInvalidADTA01
	}
	if parsed.Source != revision.Source.SourceID || parsed.SourceProfileID != revision.Profile.ArtifactID || result.ProfileID != revision.Profile.ArtifactID {
		return integration.ProcessedEvent{}, nil, ErrInvalidADTA01
	}
	if hasA01Extensions(parsed) {
		return integration.ProcessedEvent{}, nil, ErrUnsupportedADTA01
	}

	occurredAt := result.OccurredAt
	diagnostics := projectA01Warnings(request.Security.TenantID, result.Warnings)
	if occurredAt.IsZero() {
		occurredAt = request.Envelope.ReceivedAt
		diagnostic, err := newA01Diagnostic(
			request.Security.TenantID,
			integration.DiagnosticSeverityWarning,
			"semantic",
			"EVENT_TIME_FALLBACK",
			"",
		)
		if err != nil {
			return integration.ProcessedEvent{}, nil, ErrInvalidADTA01
		}
		diagnostics = append(diagnostics, diagnostic)
	}

	clone := *parsed
	clone.EventMeta = events.EventMeta{
		ID:                deterministicA01EventID(request, revision, result.ControlID, ordinal),
		Type:              events.EventPatientAdmit,
		Timestamp:         occurredAt.UTC(),
		ReceivedAt:        request.Envelope.ReceivedAt.UTC(),
		Source:            revision.Source.SourceID,
		SourceFormat:      events.FormatHL7v2,
		SourceProfileID:   revision.Profile.ArtifactID,
		SourceMessageID:   result.ControlID,
		CorrelationID:     request.CorrelationID,
		ParseWarnings:     nil,
		ExtractedEntities: nil,
		QualityScore:      nil,
	}
	clone.RawPayload = nil

	projected, err := integration.NewProcessedEvent(integration.ProcessedEventMetadata{
		TenantID:       revision.TenantID,
		Classification: revision.Policy.Classification,
	}, &clone)
	if err != nil {
		return integration.ProcessedEvent{}, nil, ErrInvalidADTA01
	}
	return projected, diagnostics, nil
}

func deterministicA01EventID(
	request integration.ProcessRequest,
	revision integration.IntegrationDefinitionRevision,
	controlID string,
	ordinal uint32,
) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(deterministicEventIDDomain))
	ref := revision.Reference()
	for _, value := range []string{
		revision.TenantID,
		ref.ArtifactID,
		ref.RevisionID,
		ref.Digest,
		revision.Source.SourceID,
		controlID,
		request.Envelope.PayloadDigest,
		string(events.EventPatientAdmit),
	} {
		writeLengthDelimitedEventIdentity(hasher, value)
	}
	var ordinalBytes [4]byte
	binary.BigEndian.PutUint32(ordinalBytes[:], ordinal)
	_, _ = hasher.Write(ordinalBytes[:])
	sum := hasher.Sum(nil)
	var identifier [16]byte
	copy(identifier[:], sum[:16])
	identifier[6] = (identifier[6] & 0x0f) | 0x80
	identifier[8] = (identifier[8] & 0x3f) | 0x80
	encoded := hex.EncodeToString(identifier[:])
	return fmt.Sprintf("%s-%s-%s-%s-%s", encoded[0:8], encoded[8:12], encoded[12:16], encoded[16:20], encoded[20:32])
}

func writeLengthDelimitedEventIdentity(hasher hash.Hash, value string) {
	var length [8]byte
	binary.BigEndian.PutUint64(length[:], uint64(len(value)))
	_, _ = hasher.Write(length[:])
	_, _ = hasher.Write([]byte(value))
}

func hasA01Extensions(event *events.PatientAdmitEvent) bool {
	if event == nil || len(event.Patient.Extensions) > 0 || len(event.Encounter.Extensions) > 0 {
		return true
	}
	providers := []*events.Provider{
		event.Patient.PrimaryCareProvider,
		event.Encounter.AttendingProvider,
		event.Encounter.AdmittingProvider,
		event.Encounter.ReferringProvider,
		event.Encounter.ConsultingProvider,
	}
	for _, provider := range providers {
		if provider != nil && len(provider.Extensions) > 0 {
			return true
		}
	}
	return false
}

func projectA01Warnings(tenantID string, warnings []events.ParseWarning) []integration.Diagnostic {
	diagnostics := make([]integration.Diagnostic, 0, len(warnings))
	for _, warning := range warnings {
		code := warning.Code
		stage := warning.Phase
		path := warning.Path
		severity := parseWarningSeverity(warning.Severity)
		if !allowedA01WarningCode(code) || !allowedA01WarningStage(stage) {
			code = "PARSE_WARNING"
			stage = "semantic"
			path = ""
			severity = integration.DiagnosticSeverityWarning
		}
		diagnostic, err := newA01Diagnostic(tenantID, severity, stage, code, path)
		if err != nil {
			diagnostic, err = newA01Diagnostic(
				tenantID,
				integration.DiagnosticSeverityWarning,
				"semantic",
				"PARSE_WARNING",
				"",
			)
		}
		if err == nil {
			diagnostics = append(diagnostics, diagnostic)
		}
	}
	return diagnostics
}

func newA01Diagnostic(
	tenantID string,
	severity integration.DiagnosticSeverity,
	stage string,
	code string,
	path string,
) (integration.Diagnostic, error) {
	return integration.NewDiagnostic(integration.DiagnosticInput{
		TenantID:       tenantID,
		Severity:       severity,
		Stage:          stage,
		Code:           code,
		Path:           path,
		Source:         "hl7v2",
		Classification: integration.DataClassificationPHI,
	})
}

func allowedA01WarningCode(code string) bool {
	switch code {
	case "MISSING_PV1", "INVALID_DTM", "IMPRECISE_EVENT_TIME":
		return true
	}
	for _, prefix := range []string{"INVALID_NPI_", "INVALID_MBI_", "INVALID_SSN_"} {
		if strings.HasPrefix(code, prefix) && validA01DiagnosticCode(code) {
			return true
		}
	}
	return false
}

func validA01DiagnosticCode(code string) bool {
	if len(code) == 0 || len(code) > 128 {
		return false
	}
	for index := range len(code) {
		character := code[index]
		if (character >= 'A' && character <= 'Z') || (character >= '0' && character <= '9') || character == '_' {
			continue
		}
		return false
	}
	return true
}

func allowedA01WarningStage(stage string) bool {
	return stage == "byte" || stage == "syntactic" || stage == "semantic"
}

func parseWarningSeverity(severity string) integration.DiagnosticSeverity {
	switch severity {
	case "info":
		return integration.DiagnosticSeverityInfo
	case "error":
		return integration.DiagnosticSeverityError
	default:
		return integration.DiagnosticSeverityWarning
	}
}

func validA01MetadataToken(value string, limit int) bool {
	return value != "" && len(value) <= limit && value == strings.TrimSpace(value) && !strings.ContainsAny(value, "\r\n\x00")
}
