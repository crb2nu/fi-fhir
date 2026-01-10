package cda

// XML namespaces used in CDA/CCDA documents.
const (
	// NSHL7V3 is the primary HL7 CDA namespace.
	NSHL7V3 = "urn:hl7-org:v3"

	// NSSDTC is the HL7 Structured Document Technical Corrections namespace.
	// Used for extensions not in the base CDA schema.
	NSSDTC = "urn:hl7-org:sdtc"

	// NSXsi is the XML Schema Instance namespace.
	NSXsi = "http://www.w3.org/2001/XMLSchema-instance"
)

// Namespaces maps namespace prefixes to URIs for XPath queries.
var Namespaces = map[string]string{
	"cda":  NSHL7V3,
	"sdtc": NSSDTC,
	"xsi":  NSXsi,
}

// Common CCDA Template OIDs for document types.
const (
	// Document Templates
	TemplateOIDCCD              = "2.16.840.1.113883.10.20.22.1.2"  // Continuity of Care Document
	TemplateOIDDischargeSummary = "2.16.840.1.113883.10.20.22.1.8"  // Discharge Summary
	TemplateOIDProgressNote     = "2.16.840.1.113883.10.20.22.1.9"  // Progress Note
	TemplateOIDConsultationNote = "2.16.840.1.113883.10.20.22.1.4"  // Consultation Note
	TemplateOIDHistoryPhysical  = "2.16.840.1.113883.10.20.22.1.3"  // History and Physical
	TemplateOIDOperativeNote    = "2.16.840.1.113883.10.20.22.1.7"  // Operative Note
	TemplateOIDReferralNote     = "2.16.840.1.113883.10.20.22.1.14" // Referral Note
	TemplateOIDTransferSummary  = "2.16.840.1.113883.10.20.22.1.13" // Transfer Summary
	TemplateOIDCareCoordination = "2.16.840.1.113883.10.20.22.1.15" // Care Coordination
	TemplateOIDUSRealmHeader    = "2.16.840.1.113883.10.20.22.1.1"  // US Realm Header
)

// Common CCDA Section Template OIDs.
const (
	// Section Templates
	TemplateSectionProblems          = "2.16.840.1.113883.10.20.22.2.5.1"  // Problems Section (entries required)
	TemplateSectionMedications       = "2.16.840.1.113883.10.20.22.2.1.1"  // Medications Section (entries required)
	TemplateSectionAllergies         = "2.16.840.1.113883.10.20.22.2.6.1"  // Allergies Section (entries required)
	TemplateSectionResults           = "2.16.840.1.113883.10.20.22.2.3.1"  // Results Section (entries required)
	TemplateSectionVitalSigns        = "2.16.840.1.113883.10.20.22.2.4.1"  // Vital Signs Section (entries required)
	TemplateSectionProcedures        = "2.16.840.1.113883.10.20.22.2.7.1"  // Procedures Section (entries required)
	TemplateSectionEncounters        = "2.16.840.1.113883.10.20.22.2.22.1" // Encounters Section (entries required)
	TemplateSectionImmunizations     = "2.16.840.1.113883.10.20.22.2.2.1"  // Immunizations Section (entries required)
	TemplateSectionPlanOfCare        = "2.16.840.1.113883.10.20.22.2.10"   // Plan of Treatment Section
	TemplateSectionGoals             = "2.16.840.1.113883.10.20.22.2.60"   // Goals Section
	TemplateSectionSocialHistory     = "2.16.840.1.113883.10.20.22.2.17"   // Social History Section
	TemplateSectionFunctionalStatus  = "2.16.840.1.113883.10.20.22.2.14"   // Functional Status Section
	TemplateSectionMentalStatus      = "2.16.840.1.113883.10.20.22.2.56"   // Mental Status Section
	TemplateSectionAdvanceDirectives = "2.16.840.1.113883.10.20.22.2.21.1" // Advance Directives Section
	TemplateSectionPayerSection      = "2.16.840.1.113883.10.20.22.2.18"   // Payers Section
)

// Common CCDA Entry Template OIDs.
const (
	// Entry Templates
	TemplateEntryProblemConcern       = "2.16.840.1.113883.10.20.22.4.3"  // Problem Concern Act
	TemplateEntryProblemObservation   = "2.16.840.1.113883.10.20.22.4.4"  // Problem Observation
	TemplateEntryMedicationActivity   = "2.16.840.1.113883.10.20.22.4.16" // Medication Activity
	TemplateEntryAllergyIntolerance   = "2.16.840.1.113883.10.20.22.4.7"  // Allergy Intolerance Observation
	TemplateEntryResultOrganizer      = "2.16.840.1.113883.10.20.22.4.1"  // Result Organizer
	TemplateEntryResultObservation    = "2.16.840.1.113883.10.20.22.4.2"  // Result Observation
	TemplateEntryVitalSignsOrganizer  = "2.16.840.1.113883.10.20.22.4.26" // Vital Signs Organizer
	TemplateEntryVitalSignObservation = "2.16.840.1.113883.10.20.22.4.27" // Vital Sign Observation
	TemplateEntryProcedureActivity    = "2.16.840.1.113883.10.20.22.4.14" // Procedure Activity Procedure
	TemplateEntryEncounterActivity    = "2.16.840.1.113883.10.20.22.4.49" // Encounter Activity
	TemplateEntryImmunizationActivity = "2.16.840.1.113883.10.20.22.4.52" // Immunization Activity
)

// Common Code System OIDs.
const (
	CodeSystemSNOMEDCT = "2.16.840.1.113883.6.96"   // SNOMED CT
	CodeSystemLOINC    = "2.16.840.1.113883.6.1"    // LOINC
	CodeSystemRxNorm   = "2.16.840.1.113883.6.88"   // RxNorm
	CodeSystemICD10CM  = "2.16.840.1.113883.6.90"   // ICD-10-CM
	CodeSystemICD10PCS = "2.16.840.1.113883.6.4"    // ICD-10-PCS
	CodeSystemCPT      = "2.16.840.1.113883.6.12"   // CPT
	CodeSystemCVX      = "2.16.840.1.113883.12.292" // CVX (Vaccines)
	CodeSystemNDC      = "2.16.840.1.113883.6.69"   // NDC
	CodeSystemHCPCS    = "2.16.840.1.113883.6.285"  // HCPCS
	CodeSystemNUBC     = "2.16.840.1.113883.6.301"  // NUBC
	CodeSystemHL7V3    = "2.16.840.1.113883.5"      // HL7 V3 Code Systems
	CodeSystemHL7V2    = "2.16.840.1.113883.12"     // HL7 V2 Tables
)

// Identifier System OIDs.
const (
	IdentifierSystemSSN = "2.16.840.1.113883.4.1" // Social Security Number
	IdentifierSystemNPI = "2.16.840.1.113883.4.6" // National Provider Identifier
	IdentifierSystemMRN = "2.16.840.1.113883.4.2" // Medical Record Number (generic)
)

// OIDToFHIRSystem maps CDA code system OIDs to FHIR URIs.
var OIDToFHIRSystem = map[string]string{
	CodeSystemSNOMEDCT:  "http://snomed.info/sct",
	CodeSystemLOINC:     "http://loinc.org",
	CodeSystemRxNorm:    "http://www.nlm.nih.gov/research/umls/rxnorm",
	CodeSystemICD10CM:   "http://hl7.org/fhir/sid/icd-10-cm",
	CodeSystemICD10PCS:  "http://www.cms.gov/Medicare/Coding/ICD10",
	CodeSystemCPT:       "http://www.ama-assn.org/go/cpt",
	CodeSystemCVX:       "http://hl7.org/fhir/sid/cvx",
	CodeSystemNDC:       "http://hl7.org/fhir/sid/ndc",
	CodeSystemHCPCS:     "http://www.cms.gov/Medicare/Coding/HCPCSReleaseCodeSets",
	IdentifierSystemSSN: "http://hl7.org/fhir/sid/us-ssn",
	IdentifierSystemNPI: "http://hl7.org/fhir/sid/us-npi",
}

// DocumentTypeToEvent maps document template OIDs to semantic event types.
var DocumentTypeToEvent = map[string]string{
	TemplateOIDCCD:              "patient_summary",
	TemplateOIDDischargeSummary: "patient_discharge",
	TemplateOIDProgressNote:     "clinical_note",
	TemplateOIDConsultationNote: "consultation",
	TemplateOIDHistoryPhysical:  "admission_note",
	TemplateOIDOperativeNote:    "procedure_note",
	TemplateOIDReferralNote:     "referral",
	TemplateOIDTransferSummary:  "patient_transfer",
}

// DocumentTypeName maps document template OIDs to human-readable names.
var DocumentTypeName = map[string]string{
	TemplateOIDCCD:              "Continuity of Care Document",
	TemplateOIDDischargeSummary: "Discharge Summary",
	TemplateOIDProgressNote:     "Progress Note",
	TemplateOIDConsultationNote: "Consultation Note",
	TemplateOIDHistoryPhysical:  "History and Physical",
	TemplateOIDOperativeNote:    "Operative Note",
	TemplateOIDReferralNote:     "Referral Note",
	TemplateOIDTransferSummary:  "Transfer Summary",
	TemplateOIDUSRealmHeader:    "US Realm Clinical Document",
}

// SectionTypeToEvent maps section template OIDs to event source types.
var SectionTypeToEvent = map[string]string{
	TemplateSectionProblems:      "conditions",
	TemplateSectionMedications:   "medications",
	TemplateSectionAllergies:     "allergies",
	TemplateSectionResults:       "lab_results",
	TemplateSectionVitalSigns:    "vital_signs",
	TemplateSectionProcedures:    "procedures",
	TemplateSectionEncounters:    "encounters",
	TemplateSectionImmunizations: "immunizations",
	TemplateSectionPlanOfCare:    "care_plan",
	TemplateSectionGoals:         "goals",
	TemplateSectionSocialHistory: "social_history",
}

// SectionTypeName maps section template OIDs to human-readable names.
var SectionTypeName = map[string]string{
	TemplateSectionProblems:         "Problems",
	TemplateSectionMedications:      "Medications",
	TemplateSectionAllergies:        "Allergies",
	TemplateSectionResults:          "Results",
	TemplateSectionVitalSigns:       "Vital Signs",
	TemplateSectionProcedures:       "Procedures",
	TemplateSectionEncounters:       "Encounters",
	TemplateSectionImmunizations:    "Immunizations",
	TemplateSectionPlanOfCare:       "Plan of Treatment",
	TemplateSectionGoals:            "Goals",
	TemplateSectionSocialHistory:    "Social History",
	TemplateSectionFunctionalStatus: "Functional Status",
	TemplateSectionMentalStatus:     "Mental Status",
}
