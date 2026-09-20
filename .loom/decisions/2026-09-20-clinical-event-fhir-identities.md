### 2026-09-20: Clinical FHIR resources use source-scoped canonical event identities

- Decision: condition, procedure, immunization, vital sign, medication request,
  and allergy intolerance projections use the persisted event ID under
  `urn:fi-fhir:event:<source>:<event_type>` for durable conditional writes.
- Reason: these canonical structs do not expose a common clinical-record
  identifier. A document/message can produce many records, so its source
  message ID cannot safely identify one resource. Separate events must not
  overwrite siblings. This namespace also separates vital-sign Observations
  from order-scoped lab Observations.
- Consequence: retrying a stored event updates the same resource. A correction
  with a new canonical event ID is a new resource; merging corrections requires
  an explicit record-identity contract in a later slice. Missing source, event
  ID, or patient identifier refuses durable delivery.
- Related behavior: clinical records reference existing Patient/Encounter
  resources by qualified identifiers and do not update their demographics.
  Literal provider/location references remain the existing mapper limitation.
- Workflow compatibility: workflow creates retain POST semantics. Resource
  selection is now enforced, transaction references resolve through fullUrls,
  and unsupported JSON event types fail instead of emitting only a Patient.
- Evidence: `internal/integration/fhirout/clinical_test.go` proves stable retry
  keys, sibling/source separation, typed/JSON/stored parity and pinned-package
  structural checks. `test/e2e/integration_test.go` reads the emitted resources
  back from HAPI FHIR with referential integrity enabled.
