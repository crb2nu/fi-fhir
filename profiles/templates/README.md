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

