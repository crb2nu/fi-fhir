# User Guide (fi-fhir UI)

The fi-fhir UI is an HL7-first “Preview & Triage” workflow: start from sample messages, understand parse warnings, and iterate toward a stable Source Profile.

## Sample Inbox (MVP)

Use Sample Inbox to collect and organize sample messages for a feed/source.

Common flow:

1. Add a sample (paste or upload).
2. Tag it (feed/source, environment, message type, etc.).
3. Apply redaction controls when working with sensitive samples.
4. Run Parse Preview to see semantic events and warnings.

### Redaction notes

Redaction is best-effort and intended for safer iteration during development. It is not a substitute for proper PHI handling policies.

## Parse Preview (MVP)

Parse Preview focuses on the semantic layer:

- Canonical event(s) produced from the message
- Warnings grouped by parsing phase (Byte → Syntactic → Semantic)

Use this view to identify what’s “wrong” with a sample and what needs to be captured as profile rules/tolerances.

## Profiles workspace

The Profiles workspace supports:

- Browsing existing Source Profiles
- Viewing/editing profile YAML
- Reviewing profile revisions

Use this when repeated fixes need to be turned into a reusable per-feed configuration.

