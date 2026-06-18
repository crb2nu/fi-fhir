# Source Profile Templates

These templates are intended as **starting points** for creating feed-specific Source Profiles.

- Copy a template into `profiles/` (or your own config repo), then set a unique `source_profile.id` and `source_profile.name`.
- Run `fi-fhir profile lint` with representative samples to validate schema + surface parsing warnings.
- Prefer cloning and tightening tolerances over starting from scratch.

Templates:
- `hl7v2/epic_adt.yaml`
- `hl7v2/cerner_adt.yaml`
- `hl7v2/meditech_adt.yaml`
- `hl7v2/allscripts_adt.yaml`
- `hl7v2/athenahealth_pm.yaml`
- `hl7v2/eclinicalworks_pm.yaml`
- `hl7v2/nextgen_ambulatory.yaml`
- `hl7v2/greenway_intergy.yaml`
- `hl7v2/advancedmd_pm.yaml`

## Research-Backed Ambulatory / PM Templates

The ambulatory and practice-management templates are deliberately broad starting points. Public vendor pages identify supported interface families, but site-specific HL7v2 profiles still vary by implementation, middleware, and customer build.

Use these templates to bootstrap intake, then narrow them with representative traffic:

| Template | Public basis | Starting assumption |
|----------|--------------|---------------------|
| `athenahealth_pm.yaml` | athenahealth lists HL7v2 documents for ADT demographics, SIU appointments, DFT charges, ORM orders, ORU results, and MFN. | Ambulatory PM feed with scheduling, charges, and clinical result adjacency. |
| `eclinicalworks_pm.yaml` | eClinicalWorks lists ADT demographics, SIU scheduling, MFN provider directory, ORM/ORU labs, and ORU/MDM reports. | Ambulatory EHR/PM feed with generous document/result tolerance. |
| `nextgen_ambulatory.yaml` | NextGen documents demographic, scheduling, order, result/document/image exchange and Mirth-based HL7 connectivity. | Ambulatory feed often mediated by an interface engine. |
| `greenway_intergy.yaml` | Greenway documents HL7, FHIR, and CDA standards for interoperability. | Ambulatory feed with profile-specific Z-segments preserved until mapped. |
| `advancedmd_pm.yaml` | AdvancedMD documents HL7, X12, FHIR, SMART on FHIR, ODBC, and REST API interoperability for private-practice workflows. | PM-centered feed where X12/EDI companion-guide auto-detection is useful. |
