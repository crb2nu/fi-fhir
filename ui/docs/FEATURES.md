# UI Features (HL7-first roadmap)

This roadmap is organized around fi-fhir primitives: **Source Profiles**, **Semantic Events**, **Warnings**, and **Workflows**.

## MVP 0 (now): Contract-first scaffold

- Typed GraphQL/OpenAPI clients (generated, committed, CI-enforced)
- Smoke-test HL7 preview using GraphQL `parsePreview`

## MVP 1: HL7 Feed “Preview & Triage”

Goal: given sample HL7 messages, make parsing understandable and fixable without HL7 expertise.

Screens:
- **Sample Inbox**: upload/paste messages; tag by feed/source; basic redaction controls.
- **Parse Preview**: show extracted semantic events + warnings.
- **HL7 Inspector** (secondary): raw segments/fields with “lineage” pointers to semantic fields.

Key behaviors:
- Default view is semantic (event-first).
- Warnings are grouped by phase (Byte/Syntactic/Semantic) and by “impact” (blocks workflows vs safe).

## MVP 2: Source Profile Builder (HL7)

Goal: turn repeated “fixes” into a reusable, versioned Source Profile.

Panels:
- **Tolerances**: missing segments, delimiter weirdness, extra components.
- **Timezone + date parsing**: defaults + per-field overrides (later).
- **Identifier normalization**: MRN/SSN/phone cleanup + validation policies (warn/error/pass).
- **Event classification**: e.g., A01 mapping based on PV1-2.

Output:
- `source_profile` YAML (exported to repo / saved via API).

## MVP 3: Workflow Builder (semantic routing)

Goal: route canonical events to actions without writing YAML.

Capabilities:
- Event-type filters with “common presets”
- Advanced conditions using CEL (expert mode)
- Actions: FHIR / webhook / log (start small)
- Dry-run simulation against sample events + warnings

## MVP 4: Production Feedback Loop

Goal: make “data drift” visible and actionable.

- Warning trends per feed (rate, top codes, top paths)
- “New message type detected” alerts
- Coverage meters for terminology mapping (later)

