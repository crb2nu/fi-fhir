package fhirout

import (
	"fmt"
	"net/url"
	"strings"
	"unicode"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
)

// How a conditional-write key is chosen.
//
// A conditional update `PUT <Type>?identifier=<system>|<value>` is only
// idempotent if the identifier has a system: `?identifier=|MRN-1` matches any
// system, so two facilities' MRNs would silently collide (`.loom/34`, Lane S6-A
// riskiest assumption). The rule here is therefore: the key always carries a
// system, and it is never guessed.
//
// Where the system comes from, in order:
//
//  1. The canonical identifier's own system — the source profile's
//     assigning-authority map or a CX.4 universal ID. This is the only case in
//     which the destination sees a system it already knows.
//  2. Otherwise, the deployment-owned SourceIdentifierSystem for the event's
//     source. HL7 semantics for a CX with no assigning authority are "assigned
//     by the sending facility", and the sending facility is the source; the
//     system names that source rather than guessing a foreign one. It is
//     deterministic, so a redelivery keys the same resource, and it is scoped
//     by source, so two sources' bare values cannot collide.
//
// Why the second case exists at all: the executable ADT A01 v1 subset caps
// PV1.19 at one component (internal/parser/hl7v2/strict_validation.go), so a
// durable visit number can never carry an assigning authority. Under a
// "no system → refuse" rule no Encounter the engine stores could ever be
// delivered. Recorded in `.loom/decisions/` (2026-09-08, Slice 4.1c-c).
//
// What is still refused: a resource with no identifier value at all. A Patient
// with no MRN, an Encounter with no visit number, or a lab result with neither
// an order number nor a source message id has nothing to key on, and the
// projection fails rather than falling back to a POST.

// sourceIdentifierSystemPrefix is the URN namespace this deployment owns for
// identifiers a source assigned without naming an assigning authority.
const sourceIdentifierSystemPrefix = "urn:fi-fhir:source:"

// maxIdentifierValueBytes bounds a key value so it stays usable as a FHIR
// token search parameter.
const maxIdentifierValueBytes = 256

// SourceIdentifierSystem is the deployment-owned identifier system for values
// the named source assigned without an assigning authority.
func SourceIdentifierSystem(source string) (string, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return "", fmt.Errorf("%w: event names no source", ErrNoUsableIdentifier)
	}
	return sourceIdentifierSystemPrefix + url.PathEscape(source), nil
}

// patientIdentifier chooses the Patient key: the medical record number.
//
// An `MR`-typed identifier is preferred, then the canonical MRN convenience
// field, then any other identifier. A Social Security number is never a key:
// the key travels in a request URL and a conditional reference, and neither is
// a place for an SSN.
func patientIdentifier(source string, patient *events.Patient) (fhir.Identifier, error) {
	if patient == nil {
		return fhir.Identifier{}, fmt.Errorf("%w: no patient", ErrNoUsableIdentifier)
	}
	for _, identifier := range patient.Identifiers.Identifiers {
		if identifier.Type == "MR" && strings.TrimSpace(identifier.Value) != "" {
			return qualify(source, identifier)
		}
	}
	if mrn := strings.TrimSpace(patient.MRN); mrn != "" {
		return qualify(source, events.Identifier{Value: mrn})
	}
	for _, identifier := range patient.Identifiers.Identifiers {
		if identifier.Type == "SS" || identifier.Type == "SSN" {
			continue
		}
		if strings.TrimSpace(identifier.Value) != "" {
			return qualify(source, identifier)
		}
	}
	return fhir.Identifier{}, fmt.Errorf("%w: no medical record number", ErrNoUsableIdentifier)
}

// encounterIdentifier chooses the Encounter key: a system-qualified visit or
// account identifier if the source supplied one, else the first bare one, else
// the canonical visit number.
func encounterIdentifier(source string, encounter *events.Encounter) (fhir.Identifier, error) {
	if encounter == nil {
		return fhir.Identifier{}, fmt.Errorf("%w: no encounter", ErrNoUsableIdentifier)
	}
	for _, identifier := range encounter.Identifiers.Identifiers {
		if strings.TrimSpace(identifier.System) != "" && strings.TrimSpace(identifier.Value) != "" {
			return qualify(source, identifier)
		}
	}
	for _, identifier := range encounter.Identifiers.Identifiers {
		if strings.TrimSpace(identifier.Value) != "" {
			return qualify(source, identifier)
		}
	}
	if visit := strings.TrimSpace(encounter.ID); visit != "" {
		return qualify(source, events.Identifier{Value: visit})
	}
	return fhir.Identifier{}, fmt.Errorf("%w: no visit number", ErrNoUsableIdentifier)
}

// reportIdentifier chooses the DiagnosticReport key: the order number the
// source assigned, else the source message id, always under the source's own
// system. A corrected result for the same order updates the same report.
func reportIdentifier(event *events.LabResultEvent) (fhir.Identifier, error) {
	if event == nil {
		return fhir.Identifier{}, fmt.Errorf("%w: no lab result", ErrNoUsableIdentifier)
	}
	value := strings.TrimSpace(event.Test.OrderID)
	if value == "" {
		value = strings.TrimSpace(event.SourceMessageID)
	}
	if value == "" {
		return fhir.Identifier{}, fmt.Errorf("%w: no order number or source message id", ErrNoUsableIdentifier)
	}
	return qualify(event.Source, events.Identifier{Value: value})
}

// observationIdentifier keys each Observation under its report: the report's
// value plus the mapper's positional observation id, which is stable per event.
func observationIdentifier(report fhir.Identifier, observationID string) (fhir.Identifier, error) {
	observationID = strings.TrimSpace(observationID)
	if observationID == "" {
		return fhir.Identifier{}, fmt.Errorf("%w: observation has no position", ErrNoUsableIdentifier)
	}
	return usableKey(fhir.Identifier{System: report.System, Value: report.Value + "#" + observationID})
}

// qualify turns a canonical identifier into a key, using its own system when it
// has one and the source's system otherwise.
func qualify(source string, identifier events.Identifier) (fhir.Identifier, error) {
	system := strings.TrimSpace(identifier.System)
	if system == "" {
		derived, err := SourceIdentifierSystem(source)
		if err != nil {
			return fhir.Identifier{}, err
		}
		system = derived
	}
	return usableKey(fhir.Identifier{System: system, Value: strings.TrimSpace(identifier.Value)})
}

// usableKey refuses a key that cannot travel as a FHIR token search value: an
// empty half, a `|` (the system/value separator), whitespace, a control
// character, or an unbounded length.
func usableKey(key fhir.Identifier) (fhir.Identifier, error) {
	if key.System == "" || key.Value == "" {
		return fhir.Identifier{}, fmt.Errorf("%w: key is incomplete", ErrNoUsableIdentifier)
	}
	if len(key.Value) > maxIdentifierValueBytes || len(key.System) > 2048 {
		return fhir.Identifier{}, fmt.Errorf("%w: key is too long", ErrNoUsableIdentifier)
	}
	for _, half := range []string{key.System, key.Value} {
		for _, character := range half {
			if character == '|' || unicode.IsSpace(character) || unicode.IsControl(character) {
				return fhir.Identifier{}, fmt.Errorf("%w: key is not a usable search token", ErrNoUsableIdentifier)
			}
		}
	}
	return key, nil
}
