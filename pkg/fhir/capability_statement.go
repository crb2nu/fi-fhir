package fhir

import (
	"sort"
	"time"
)

const capabilityResourceDocumentation = "Emitted by the US Core mapper. This endpoint exposes no read/search interactions for this type."

// mapperResourceAliases documents Map methods whose suffix is not the emitted
// FHIR resource type. Keep this table aligned with the reflection drift test.
var mapperResourceAliases = map[string]string{
	"DiagnosticReportNote": "DiagnosticReport",
	"LabObservation":       "Observation",
	"LabResult":            "DiagnosticReport",
	"Location":             "Location",
	"Organization":         "Organization",
	"Practitioner":         "Practitioner",
	"PractitionerRole":     "PractitionerRole",
	"RelatedPerson":        "RelatedPerson",
	"VitalSign":            "Observation",
}

var supportedResourceTypes = []string{
	"AllergyIntolerance", "CarePlan", "CareTeam", "Claim", "Condition",
	"Coverage", "CoverageEligibilityResponse", "DiagnosticReport", "DocumentReference",
	"Encounter", "ExplanationOfBenefit", "Goal", "Immunization", "Location",
	"MedicationRequest", "Observation", "Organization", "Patient", "Practitioner",
	"PractitionerRole", "Procedure", "Provenance", "RelatedPerson", "ServiceRequest",
}

// SupportedResourceTypes returns a sorted, de-duplicated copy of the FHIR
// resource types produced by USCoreMapper.
func SupportedResourceTypes() []string {
	resourceTypes := append([]string(nil), supportedResourceTypes...)
	sort.Strings(resourceTypes)
	return resourceTypes
}

// CapabilityStatementOptions supplies deployment-specific statement fields.
type CapabilityStatementOptions struct {
	Date            time.Time
	SoftwareVersion string
}

// CapabilityStatement is the truthful subset of the FHIR R4 resource needed
// to describe fi-fhir's mapper output surface.
type CapabilityStatement struct {
	ResourceType   string                            `json:"resourceType"`
	Status         string                            `json:"status"`
	Date           string                            `json:"date"`
	Kind           string                            `json:"kind"`
	Software       CapabilityStatementSoftware       `json:"software"`
	Implementation CapabilityStatementImplementation `json:"implementation"`
	FHIRVersion    string                            `json:"fhirVersion"`
	Format         []string                          `json:"format"`
	Rest           []CapabilityStatementRest         `json:"rest"`
}

type CapabilityStatementSoftware struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type CapabilityStatementImplementation struct {
	Description string `json:"description"`
}

type CapabilityStatementRest struct {
	Mode     string                        `json:"mode"`
	Resource []CapabilityStatementResource `json:"resource"`
}

type CapabilityStatementResource struct {
	Type             string   `json:"type"`
	SupportedProfile []string `json:"supportedProfile,omitempty"`
	Documentation    string   `json:"documentation"`
}

var supportedProfiles = map[string][]string{
	"AllergyIntolerance": {USCoreAllergyIntoleranceProfile},
	"CarePlan":           {USCoreCarePlanProfile},
	"CareTeam":           {USCoreCareTeamProfile},
	"Condition":          {USCoreConditionProfile},
	"Coverage":           {USCoreCoverageProfile},
	"DiagnosticReport":   {USCoreDiagnosticReportLabProfile, USCoreDiagnosticReportNoteProfile},
	"DocumentReference":  {USCoreDocumentReferenceProfile},
	"Encounter":          {USCoreEncounterProfile},
	"Goal":               {USCoreGoalProfile},
	"Immunization":       {USCoreImmunizationProfile},
	"Location":           {USCoreLocationProfile},
	"MedicationRequest":  {USCoreMedicationRequestProfile},
	"Observation": {
		USCoreObservationLabProfile,
		USCoreVitalSignsProfile,
		USCoreBloodPressureProfile,
		USCoreBodyHeightProfile,
		USCoreBodyWeightProfile,
		USCoreBodyTemperatureProfile,
		USCoreHeartRateProfile,
		USCoreRespiratoryRateProfile,
		USCorePulseOximetryProfile,
		USCoreBMIProfile,
	},
	"Organization":     {USCoreOrganizationProfile},
	"Patient":          {USCorePatientProfile},
	"Practitioner":     {USCorePractitionerProfile},
	"PractitionerRole": {USCorePractitionerRoleProfile},
	"Procedure":        {USCoreProcedureProfile},
	"Provenance":       {USCoreProvenanceProfile},
	"RelatedPerson":    {USCoreRelatedPersonProfile},
	"ServiceRequest":   {USCoreServiceRequestProfile},
}

// NewCapabilityStatement describes resources emitted by the US Core mapper.
// It intentionally declares no read or search interactions.
func NewCapabilityStatement(opts CapabilityStatementOptions) CapabilityStatement {
	resources := make([]CapabilityStatementResource, 0, len(supportedResourceTypes))
	for _, resourceType := range SupportedResourceTypes() {
		profiles := append([]string(nil), supportedProfiles[resourceType]...)
		sort.Strings(profiles)
		resources = append(resources, CapabilityStatementResource{
			Type:             resourceType,
			SupportedProfile: profiles,
			Documentation:    capabilityResourceDocumentation,
		})
	}

	return CapabilityStatement{
		ResourceType: "CapabilityStatement",
		Status:       "active",
		Date:         opts.Date.UTC().Format(time.RFC3339),
		Kind:         "instance",
		Software: CapabilityStatementSoftware{
			Name: "fi-fhir", Version: opts.SoftwareVersion,
		},
		Implementation: CapabilityStatementImplementation{Description: "Format-agnostic healthcare integration engine; resources are produced by the US Core mapper and delivered to configured destinations."},
		FHIRVersion:    "4.0.1",
		Format:         []string{"application/fhir+json"},
		Rest:           []CapabilityStatementRest{{Mode: "server", Resource: resources}},
	}
}
