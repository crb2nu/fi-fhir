# HL7v2 Vendor Fixtures

Synthetic HL7v2 fixtures used to exercise profile-driven parsing across vendor variants.

Conventions:
- Fixtures are **not real patient data**.
- `*_clean.hl7` is expected to parse without warnings under the matching template.
- `*_drift_*.hl7` intentionally includes real-world drift (missing segments, delimiter variation, etc.)
  and is expected to parse with warnings (but without hard errors) under the matching template.

Template mapping:
- Epic → `profiles/templates/hl7v2/epic_adt.yaml`
- Cerner → `profiles/templates/hl7v2/cerner_adt.yaml`
- Meditech → `profiles/templates/hl7v2/meditech_adt.yaml`
- Allscripts → `profiles/templates/hl7v2/allscripts_adt.yaml`
