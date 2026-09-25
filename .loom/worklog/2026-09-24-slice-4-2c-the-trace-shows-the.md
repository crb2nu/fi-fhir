### 2026-09-24 - Slice 4.2c the trace shows the FHIR delivery

- What changed: Lane S7-C projected the destination provenance ledger
  (`integration_destination_deliveries`) into the operator control plane.
  `PostgresProvenance.ListDeliveriesForAttempt` is the one new read;
  `operator.Service` composes it into `DeliveryAttemptSummary.Deliveries` for
  `GetAttempt` and control results (25 rows) and `ListAttempts` /
  `GetMessageTrace` (5 per attempt); GraphQL gains
  `OperatorDestinationDelivery` and `OperatorDeliveryAttempt.deliveries`
  (additive, no root field, transport gate unchanged). The IDE's message trace
  shows a Delivery block under each attempt and the DLQ console opens one per
  dead letter on demand: transport, outcome, resource types, entry count,
  issue codes, declared endpoint — nothing else. `serve` now migrates the
  destination ledger beside the lifecycle catalog.
- Why: Sprint 6 correction 10 — after 4.1c-c the ledger held the FHIR delivery
  facts and the trace could not show them, so Journey 1 ("inspect trace and
  FHIR delivery") stopped at the attempt row.
- Evidence: day-1 gate `TestOperatorControlPlane_TraceShowsTheFHIRDelivery`
  PASSED on unmodified main 1465aa516 (`deliveries` fails validation; no
  ledger-only value in the attempt, list, or trace bodies) and is committed in
  that form; inverted at ship it requires all six of attempt-a's rows newest
  first, the newest five on the list and trace, the ledger on a replay result,
  no row from another tenant under the same attempt id, and no raw-PHI
  sentinel. `make operator-control-plane` now runs both proofs behind an arity
  guard. Unit tests cover the read's argument contract, the list split, the
  service composition (tenant, bound, error mapping, no ledger read on a
  refused role), and the resolver projection. UI: 703 vitest tests, svelte-check
  0 errors, eslint, stylelint, typecheck, codegen check; gqlgen regenerate is
  diff-clean; `make transport-gate` green unchanged.
- What's next: nothing required. If a trace with ~100 attempts ever makes the
  per-attempt lookups visible, a batched `attempt_id = ANY($2)` read with a
  per-attempt window is the drop-in replacement behind the same seam.
- Sources:
  - [S1] `.loom/35-sprint7-execution-specs.md` (Lane S7-C)
  - [S2] `.loom/34-sprint6-execution-specs.md` correction 10
  - [S3] `internal/integration/destination/migrations/0003_fhir_delivery_provenance.sql` (PHI posture)
  - [S4] `.loom/decisions/2026-09-24-the-operator-trace-reads-the-delivery-ledger.md`
