// Package fhirout projects one stored canonical event into the FHIR R4
// resources a `fhir`-transport destination receives, and builds the conditional
// transaction Bundle that carries them.
//
// Slice 4.1c-c. Before it, `pkg/fhir` was reachable only from the legacy
// workflow engine and the CLI; the durable engine delivered a canonical-event
// command envelope and nothing on its path could produce a resource
// (`TestFHIRConformance_DurableEngineProducesNoFHIRResource`, Slice 5.1a).
//
// This is the only non-test package under internal/integration that imports
// pkg/fhir, and both engines map through it: the durable destination transport
// calls Project over the outbox row's payload_json, and the legacy engine's
// fhir action calls ProjectWorkflow over its typed or JSON event. The event→resource switch
// therefore has exactly one home and cannot drift between the two engines the
// way the mapper and the checker drifted before 5.1a.
//
// Coverage includes admission/discharge, lab results, and six clinical event
// families. Every other registered event type is refused with
// ErrUnsupportedEventType, and the workflow planner turns that refusal into a
// plan-time diagnostic so it is visible at dry-run rather than at dispatch.
package fhirout

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

var (
	// ErrUnsupportedEventType means the event type has no FHIR projection. It is
	// a property of the type, so the planner can report it before dispatch.
	ErrUnsupportedEventType = errors.New("event type has no FHIR projection")
	// ErrInvalidPayload means the stored payload could not be decoded into the
	// canonical struct its type declares, or the decoded event is unusable.
	ErrInvalidPayload = errors.New("canonical event payload cannot be projected")
	// ErrNoUsableIdentifier means a projected resource has nothing to key a
	// conditional write on. The projection is refused rather than downgraded to
	// a POST: a POST would create a duplicate on every redelivery.
	ErrNoUsableIdentifier = errors.New("projected resource has no usable identifier for a conditional write")
)

// fullURLDigestDomain separates the deterministic entry fullUrl derivation from
// every other digest in the repository.
const fullURLDigestDomain = "fi-fhir/fhirout/full-url/v1\x00"

// supportedEventTypes is the closed set Project accepts. Each maps, in
// pkg/integration's canonical registry, to one of the concrete structs
// ProjectEvent switches on; TestProjectSupportsExactlyTheRegisteredTypes pins
// that agreement so a registry change cannot silently widen or narrow it.
var supportedEventTypes = map[events.EventType]func() any{
	events.EventPatientAdmit:       func() any { return &events.PatientAdmitEvent{} },
	events.EventPatientTransfer:    func() any { return &events.PatientAdmitEvent{} },
	events.EventPatientUpdate:      func() any { return &events.PatientAdmitEvent{} },
	events.EventPatientDischarge:   func() any { return &events.PatientDischargeEvent{} },
	events.EventLabResult:          func() any { return &events.LabResultEvent{} },
	events.EventCondition:          func() any { return &events.ConditionEvent{} },
	events.EventProcedure:          func() any { return &events.ProcedureEvent{} },
	events.EventImmunization:       func() any { return &events.ImmunizationEvent{} },
	events.EventVitalSign:          func() any { return &events.VitalSignEvent{} },
	events.EventMedicationRequest:  func() any { return &events.MedicationRequestEvent{} },
	events.EventAllergyIntolerance: func() any { return &events.AllergyIntoleranceEvent{} },
}

// Supports reports whether the event type has a projection.
func Supports(eventType events.EventType) bool {
	_, supported := supportedEventTypes[eventType]
	return supported
}

// SupportedEventTypes returns the closed projectable set in a stable order.
func SupportedEventTypes() []events.EventType {
	types := make([]events.EventType, 0, len(supportedEventTypes))
	for eventType := range supportedEventTypes {
		types = append(types, eventType)
	}
	sort.Slice(types, func(i, j int) bool { return types[i] < types[j] })
	return types
}

// Projection is one canonical event as the FHIR resources a destination
// receives, each with the identifier its conditional write is keyed on.
type Projection struct {
	EventType events.EventType
	Resources []Resource
	// references maps every literal reference the mapper emitted between
	// projected resources (`Patient/<mrn>`, `Observation/obs-1`) to the
	// reference the conditional bundle carries instead: the target entry's
	// fullUrl when the target is in the bundle, a conditional reference when it
	// is not. The mapper's own output is left untouched so the legacy engine,
	// which sends resources individually, keeps its exact bytes.
	references map[string]string
}

// Resource is one projected resource and the key of its conditional write.
type Resource struct {
	// Type is the FHIR resourceType.
	Type string
	// Resource is the mapper's output, byte-identical to what the legacy engine
	// produces for the same event.
	Resource fhir.Resource
	// Key is the identifier the conditional write is keyed on. System and Value
	// are both non-empty by construction; see identifier.go for how it is chosen.
	Key fhir.Identifier
	// FullURL is the entry's deterministic `urn:uuid:` fullUrl, derived from Type
	// and Key so the same event projects to the same bundle on redelivery and
	// intra-bundle references stay stable.
	FullURL string
}

// ResourceTypes returns the projected resource types in bundle order, without
// repeats. It is what the delivery ledger records.
func (p Projection) ResourceTypes() []string {
	seen := make(map[string]struct{}, len(p.Resources))
	types := make([]string, 0, len(p.Resources))
	for _, resource := range p.Resources {
		if _, duplicate := seen[resource.Type]; duplicate {
			continue
		}
		seen[resource.Type] = struct{}{}
		types = append(types, resource.Type)
	}
	return types
}

// PayloadEventType reads the event type a stored canonical payload declares.
//
// The outbox work item does not carry the row's event_type column; the payload
// does, and NewProcessedEvent validated the two equal before the row was
// written (pkg/integration/contracts.go validateProcessedEvent).
func PayloadEventType(payload json.RawMessage) (events.EventType, error) {
	var meta struct {
		Type events.EventType `json:"type"`
	}
	if err := json.Unmarshal(payload, &meta); err != nil || meta.Type == "" {
		return "", fmt.Errorf("%w: payload declares no event type", ErrInvalidPayload)
	}
	return meta.Type, nil
}

// Project decodes one stored canonical payload and projects it.
//
// The decode is pkg/integration's own (DecodeCanonicalEventPayload), so the
// value handed to the mapper is exactly the struct the registry declares for
// the type — which TestFHIRDestination_DurablePayloadRoundTripsToMapperInput
// proved is the mapper's input, byte for byte.
func Project(eventType events.EventType, payload json.RawMessage) (Projection, error) {
	if !Supports(eventType) {
		return Projection{}, fmt.Errorf("%w: %s", ErrUnsupportedEventType, eventType)
	}
	decoded, err := integration.DecodeCanonicalEventPayload(eventType, payload)
	if err != nil {
		return Projection{}, fmt.Errorf("%w: %w", ErrInvalidPayload, err)
	}
	projection, err := ProjectEvent(decoded)
	if err != nil {
		return Projection{}, err
	}
	projection.EventType = eventType
	return projection, nil
}

// ProjectEvent projects an already-decoded concrete canonical event for the
// durable `fhir` transport: the mapping plus, per resource, the key of its
// conditional write. A resource that cannot be keyed is a projection error.
func ProjectEvent(event any) (Projection, error) {
	return mapEvent(event, true)
}

// MapEvent is the legacy engine's view of the same switch: the mapper's
// resources for the event, and nothing else. No conditional-write key is
// derived, because the legacy engine sends resources individually with its
// own request semantics; an event the durable transport would refuse for
// having no usable key still maps here exactly as it did before Slice 4.1c-c.
//
// It exists so both engines dispatch through ONE switch (this file) and cannot
// drift on which resources an event becomes.
func MapEvent(event any) ([]fhir.Resource, error) {
	projection, err := mapEvent(event, false)
	if err != nil {
		return nil, err
	}
	resources := make([]fhir.Resource, 0, len(projection.Resources))
	for _, resource := range projection.Resources {
		resources = append(resources, resource.Resource)
	}
	return resources, nil
}

// mapEvent is the three-case switch that lived in internal/workflow/actions.go
// eventToFHIRResources until Slice 4.1c-c. Both pointer and value forms are
// accepted, as the legacy switch accepted them. withKeys selects whether the
// conditional-write keys and fullUrls are derived.
func mapEvent(event any, withKeys bool) (Projection, error) {
	// Workflow JSON can include the original raw payload; durable Project uses
	// the stricter canonical registry decoder before reaching this switch.
	if object, ok := event.(map[string]any); ok {
		eventType, _ := object["type"].(string)
		factory, supported := supportedEventTypes[events.EventType(eventType)]
		if !supported {
			return Projection{}, fmt.Errorf("%w: %s", ErrUnsupportedEventType, eventType)
		}
		payload, err := json.Marshal(object)
		if err != nil {
			return Projection{}, fmt.Errorf("%w: %w", ErrInvalidPayload, err)
		}
		event = factory()
		if err := json.Unmarshal(payload, event); err != nil {
			return Projection{}, fmt.Errorf("%w: %w", ErrInvalidPayload, err)
		}
	}

	mapper := fhir.NewUSCoreMapper()
	switch typed := event.(type) {
	case *events.PatientAdmitEvent:
		if typed == nil {
			return Projection{}, fmt.Errorf("%w: nil event", ErrInvalidPayload)
		}
		return projectAdmission(mapper, typed.EventMeta, &typed.Patient, &typed.Encounter, withKeys)
	case events.PatientAdmitEvent:
		return projectAdmission(mapper, typed.EventMeta, &typed.Patient, &typed.Encounter, withKeys)
	case *events.PatientDischargeEvent:
		if typed == nil {
			return Projection{}, fmt.Errorf("%w: nil event", ErrInvalidPayload)
		}
		return projectAdmission(mapper, typed.EventMeta, &typed.Patient, &typed.Encounter, withKeys)
	case events.PatientDischargeEvent:
		return projectAdmission(mapper, typed.EventMeta, &typed.Patient, &typed.Encounter, withKeys)
	case *events.LabResultEvent:
		if typed == nil {
			return Projection{}, fmt.Errorf("%w: nil event", ErrInvalidPayload)
		}
		return projectLabResult(mapper, typed, withKeys)
	case events.LabResultEvent:
		return projectLabResult(mapper, &typed, withKeys)
	case *events.ConditionEvent:
		if typed == nil {
			return Projection{}, fmt.Errorf("%w: nil event", ErrInvalidPayload)
		}
		return projectClinical(typed.EventMeta, typed.Patient, typed.Encounter,
			mapper.MapCondition(typed, clinicalPatientReference(typed.Patient)), withKeys)
	case events.ConditionEvent:
		return mapEvent(&typed, withKeys)

	case *events.ProcedureEvent:
		if typed == nil {
			return Projection{}, fmt.Errorf("%w: nil event", ErrInvalidPayload)
		}
		return projectClinical(typed.EventMeta, typed.Patient, typed.Encounter,
			mapper.MapProcedure(typed, clinicalPatientReference(typed.Patient)), withKeys)
	case events.ProcedureEvent:
		return mapEvent(&typed, withKeys)

	case *events.ImmunizationEvent:
		if typed == nil {
			return Projection{}, fmt.Errorf("%w: nil event", ErrInvalidPayload)
		}
		return projectClinical(typed.EventMeta, typed.Patient, typed.Encounter,
			mapper.MapImmunization(typed, clinicalPatientReference(typed.Patient)), withKeys)
	case events.ImmunizationEvent:
		return mapEvent(&typed, withKeys)

	case *events.VitalSignEvent:
		if typed == nil {
			return Projection{}, fmt.Errorf("%w: nil event", ErrInvalidPayload)
		}
		return projectClinical(typed.EventMeta, typed.Patient, typed.Encounter,
			mapper.MapVitalSign(typed, clinicalPatientReference(typed.Patient)), withKeys)
	case events.VitalSignEvent:
		return mapEvent(&typed, withKeys)

	case *events.MedicationRequestEvent:
		if typed == nil {
			return Projection{}, fmt.Errorf("%w: nil event", ErrInvalidPayload)
		}
		return projectClinical(typed.EventMeta, typed.Patient, typed.Encounter,
			mapper.MapMedicationRequest(typed, clinicalPatientReference(typed.Patient)), withKeys)
	case events.MedicationRequestEvent:
		return mapEvent(&typed, withKeys)

	case *events.AllergyIntoleranceEvent:
		if typed == nil {
			return Projection{}, fmt.Errorf("%w: nil event", ErrInvalidPayload)
		}
		return projectClinical(typed.EventMeta, typed.Patient, typed.Encounter,
			mapper.MapAllergyIntolerance(typed, clinicalPatientReference(typed.Patient)), withKeys)
	case events.AllergyIntoleranceEvent:
		return mapEvent(&typed, withKeys)

	default:
		return Projection{}, fmt.Errorf("%w: %T", ErrUnsupportedEventType, event)
	}
}

// projectAdmission is the Patient + Encounter projection shared by admit,
// transfer, update, and discharge.
func projectAdmission(
	mapper *fhir.USCoreMapper,
	meta events.EventMeta,
	patient *events.Patient,
	encounter *events.Encounter,
	withKeys bool,
) (Projection, error) {
	// The literal the legacy switch passes (actions.go) — kept so the mapper's
	// output is identical for both engines, then rewritten for the bundle.
	patientRef := "Patient/" + patient.MRN
	patientEntry := Resource{Type: "Patient", Resource: mapper.MapPatient(patient)}
	encounterEntry := Resource{Type: "Encounter", Resource: mapper.MapEncounter(encounter, patientRef)}
	projection := Projection{EventType: meta.Type, Resources: []Resource{patientEntry, encounterEntry}}
	if !withKeys {
		return projection, nil
	}

	patientKey, err := patientIdentifier(meta.Source, patient)
	if err != nil {
		return Projection{}, fmt.Errorf("Patient: %w", err)
	}
	encounterKey, err := encounterIdentifier(meta.Source, encounter)
	if err != nil {
		return Projection{}, fmt.Errorf("Encounter: %w", err)
	}
	projection.Resources[0] = keyed(patientEntry, patientKey)
	projection.Resources[1] = keyed(encounterEntry, encounterKey)
	projection.references = map[string]string{patientRef: projection.Resources[0].FullURL}
	return projection, nil
}

// projectLabResult is the DiagnosticReport + Observation projection.
//
// The Patient is not part of the bundle: an ORU's PID is routinely too thin for
// a US Core Patient (no birth date), and a resource that cannot validate must
// not be written to satisfy a reference. The subject is a conditional reference
// to the Patient the admit already delivered, keyed the same way.
func projectLabResult(mapper *fhir.USCoreMapper, event *events.LabResultEvent, withKeys bool) (Projection, error) {
	report, observations := mapper.MapLabResult(event)
	if report == nil {
		return Projection{}, fmt.Errorf("%w: lab result mapped to no report", ErrInvalidPayload)
	}
	projection := Projection{EventType: event.Type, Resources: []Resource{{Type: "DiagnosticReport", Resource: report}}}
	for _, observation := range observations {
		projection.Resources = append(projection.Resources, Resource{Type: "Observation", Resource: observation})
	}
	if !withKeys {
		return projection, nil
	}

	patientKey, err := patientIdentifier(event.Source, &event.Patient)
	if err != nil {
		return Projection{}, fmt.Errorf("Patient: %w", err)
	}
	reportKey, err := reportIdentifier(event)
	if err != nil {
		return Projection{}, fmt.Errorf("DiagnosticReport: %w", err)
	}
	patientRef := "Patient/" + event.Patient.MRN
	projection.references = map[string]string{patientRef: ConditionalReference("Patient", patientKey)}
	projection.Resources[0] = keyed(projection.Resources[0], reportKey)
	for index, observation := range observations {
		key, err := observationIdentifier(reportKey, observation.ID)
		if err != nil {
			return Projection{}, fmt.Errorf("Observation: %w", err)
		}
		entry := keyed(projection.Resources[index+1], key)
		projection.references["Observation/"+observation.ID] = entry.FullURL
		projection.Resources[index+1] = entry
	}
	return projection, nil
}

func keyed(resource Resource, key fhir.Identifier) Resource {
	resource.Key = key
	resource.FullURL = deterministicFullURL(resource.Type, key)
	return resource
}

// deterministicFullURL derives a name-based UUID from the resource type and its
// conditional key. The same event therefore projects to the same fullUrls on
// every delivery, and a Bundle's intra-bundle references do not depend on a
// random source.
func deterministicFullURL(resourceType string, key fhir.Identifier) string {
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(fullURLDigestDomain))
	_, _ = hasher.Write([]byte(resourceType))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(key.System))
	_, _ = hasher.Write([]byte{0})
	_, _ = hasher.Write([]byte(key.Value))
	sum := hasher.Sum(nil)
	var identifier [16]byte
	copy(identifier[:], sum[:16])
	identifier[6] = (identifier[6] & 0x0f) | 0x50 // version 5, name-based
	identifier[8] = (identifier[8] & 0x3f) | 0x80 // RFC 4122 variant
	encoded := hex.EncodeToString(identifier[:])
	return "urn:uuid:" + encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" +
		encoded[16:20] + "-" + encoded[20:32]
}
