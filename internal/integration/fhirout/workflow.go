package fhirout

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
)

// ProjectWorkflow selects resources for the legacy workflow's create action.
// Unlike Project, it does not require a durable write key for every resource.
// References to omitted Patients/Encounters still require qualified keys;
// otherwise a business identifier would be mistaken for a server-assigned ID.
func ProjectWorkflow(event any, resourceType string) (Projection, error) {
	all, err := mapEvent(event, false)
	if err != nil {
		return Projection{}, err
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return Projection{}, fmt.Errorf("%w: %w", ErrInvalidPayload, err)
	}
	var context struct {
		events.EventMeta
		Patient   *events.Patient   `json:"patient"`
		Encounter *events.Encounter `json:"encounter"`
	}
	if err := json.Unmarshal(payload, &context); err != nil {
		return Projection{}, fmt.Errorf("%w: %w", ErrInvalidPayload, err)
	}

	resourceType = strings.TrimSpace(resourceType)
	selected := Projection{EventType: all.EventType, references: map[string]string{}}
	aliases := map[string]Resource{}
	bodies := make([]map[string]any, 0, len(all.Resources))
	for index, resource := range all.Resources {
		raw, err := json.Marshal(resource.Resource)
		if err != nil {
			return Projection{}, err
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			return Projection{}, err
		}
		resource.FullURL = deterministicFullURL(resource.Type, fhir.Identifier{
			System: "urn:fi-fhir:workflow", Value: fmt.Sprintf("%x:%d", sha256.Sum256(raw), index),
		})
		var alias string
		switch resource.Type {
		case "Patient":
			if context.Patient != nil {
				alias = clinicalPatientReference(context.Patient)
				resource.Key, _ = patientIdentifier(context.Source, context.Patient)
			}
		case "Encounter":
			if context.Encounter != nil {
				alias = "Encounter/" + context.Encounter.ID
				resource.Key, _ = encounterIdentifier(context.Source, context.Encounter)
			}
		default:
			if context.Type != events.EventLabResult {
				resource.Key, _ = clinicalEventIdentifier(context.EventMeta)
			}
			if id, _ := body["id"].(string); id != "" {
				alias = resource.Type + "/" + id
			}
		}
		if alias != "" {
			aliases[alias] = resource
		}
		if resourceType == "" || resource.Type == resourceType {
			selected.Resources = append(selected.Resources, resource)
			bodies = append(bodies, body)
			if alias != "" {
				selected.references[alias] = resource.FullURL
			}
		}
	}
	if len(selected.Resources) == 0 {
		return Projection{}, fmt.Errorf("event produces no FHIR resource of type %q", resourceType)
	}

	// Only resolve references actually used by selected resources. Selecting
	// Patient alone must not require an encounter ID or a complete encounter.
	for _, body := range bodies {
		for _, literal := range resourceReferences(body) {
			if _, local := selected.references[literal]; local {
				continue
			}
			var key fhir.Identifier
			var target string
			var keyErr error
			switch {
			case context.Patient != nil && literal == clinicalPatientReference(context.Patient):
				target = "Patient"
				key, keyErr = patientIdentifier(context.Source, context.Patient)
			case context.Encounter != nil && literal == "Encounter/"+context.Encounter.ID:
				target = "Encounter"
				key, keyErr = encounterIdentifier(context.Source, context.Encounter)
			default:
				if _, omitted := aliases[literal]; omitted {
					return Projection{}, fmt.Errorf("resource selection omits referenced resource %q", literal)
				}
				continue
			}
			if keyErr != nil {
				return Projection{}, fmt.Errorf("%s reference: %w", target, keyErr)
			}
			selected.references[literal] = ConditionalReference(target, key)
		}
	}
	return selected, nil
}

func resourceReferences(value any) []string {
	var references []string
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == "reference" {
				if literal, ok := child.(string); ok {
					references = append(references, literal)
				}
			} else {
				references = append(references, resourceReferences(child)...)
			}
		}
	case []any:
		for _, child := range typed {
			references = append(references, resourceReferences(child)...)
		}
	}
	return references
}

// CreateWorkflowTransactionBundle preserves the legacy create semantics while
// resolving references within the transaction. Durable delivery continues to
// use CreateConditionalTransactionBundle and conditional PUTs.
func CreateWorkflowTransactionBundle(projection Projection) (*fhir.Bundle, error) {
	if len(projection.Resources) == 0 {
		return nil, fmt.Errorf("%w: projection has no resources", ErrInvalidPayload)
	}
	bundle := &fhir.Bundle{ResourceType: "Bundle", Type: "transaction"}
	for _, resource := range projection.Resources {
		encoded, err := encodeForBundle(resource, projection.references)
		if err != nil {
			return nil, err
		}
		bundle.Entry = append(bundle.Entry, fhir.BundleEntry{
			FullURL: resource.FullURL, Resource: encoded,
			Request: &fhir.BundleEntryRequest{Method: "POST", URL: resource.Type},
		})
	}
	return bundle, nil
}

// WorkflowPatient is the selected Patient for a direct create request. It
// carries the same qualified identifier a later clinical transaction resolves.
func WorkflowPatient(projection Projection) (fhir.Resource, error) {
	if len(projection.Resources) != 1 || projection.Resources[0].Type != "Patient" {
		return nil, fmt.Errorf("direct workflow create requires one Patient")
	}
	raw, err := encodeForBundle(projection.Resources[0], nil)
	if err != nil {
		return nil, err
	}
	var patient fhir.Patient
	if err := json.Unmarshal(raw, &patient); err != nil {
		return nil, err
	}
	return &patient, nil
}
