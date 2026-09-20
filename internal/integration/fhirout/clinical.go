package fhirout

import (
	"fmt"
	"net/url"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
)

func clinicalPatientReference(patient *events.Patient) string {
	if patient == nil {
		return ""
	}
	return "Patient/" + patient.MRN
}

// Clinical events describe one record, not a demographic update. Like lab
// results, they reference an existing Patient instead of overwriting it with
// the often incomplete demographics in a clinical document.
func projectClinical(meta events.EventMeta, patient *events.Patient, encounter *events.Encounter, resource fhir.Resource, withKeys bool) (Projection, error) {
	if patient == nil {
		return Projection{}, fmt.Errorf("%w: clinical event has no patient", ErrInvalidPayload)
	}
	entry := Resource{Type: resource.GetResourceType(), Resource: resource}
	projection := Projection{EventType: meta.Type, Resources: []Resource{entry}}
	if !withKeys {
		return projection, nil
	}
	patientKey, err := patientIdentifier(meta.Source, patient)
	if err != nil {
		return Projection{}, fmt.Errorf("Patient: %w", err)
	}
	key, err := clinicalEventIdentifier(meta)
	if err != nil {
		return Projection{}, fmt.Errorf("%s: %w", entry.Type, err)
	}
	projection.Resources[0] = keyed(entry, key)
	projection.references = map[string]string{
		clinicalPatientReference(patient): ConditionalReference("Patient", patientKey),
	}
	if encounter != nil && encounter.ID != "" {
		key, err := encounterIdentifier(meta.Source, encounter)
		if err != nil {
			return Projection{}, fmt.Errorf("Encounter: %w", err)
		}
		projection.references["Encounter/"+encounter.ID] = ConditionalReference("Encounter", key)
	}
	return projection, nil
}

// A source message may contain many clinical records. Keying on its message
// ID would overwrite siblings, so these events use their persisted canonical
// event ID. A later correction with a new event ID is a distinct resource.
func clinicalEventIdentifier(meta events.EventMeta) (fhir.Identifier, error) {
	source := strings.TrimSpace(meta.Source)
	if source == "" {
		return fhir.Identifier{}, fmt.Errorf("%w: event names no source", ErrNoUsableIdentifier)
	}
	return usableKey(fhir.Identifier{
		System: "urn:fi-fhir:event:" + url.PathEscape(source) + ":" + string(meta.Type),
		Value:  strings.TrimSpace(meta.ID),
	})
}
