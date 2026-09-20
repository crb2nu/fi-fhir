### 2026-09-08 - Slice 4.1c-c: the FHIR destination class delivers conditional US Core bundles

- Lane: **S6-A**, Sprint 6 (`.loom/34-sprint6-execution-specs.md`). Branch
  `feat/phase4-slice-4-1c-c-fhir-destination-impl`, stacked on the day-1 gate
  branch (MR !203) and rebased onto `main` after S6-0's merge surface
  (`chore/sprint6-merge-surface`, MR !202) landed. Coordinator ruling applied:
  **Option A**, a `fhir` transport kind with its own policy — recorded in
  `.loom/decisions/2026-09-08-slice-4-1c-c-is-a-fhir.md`.

- What changed, by task:
  1. **`pkg/integration/contracts.go`** — `DecodeCanonicalEventPayload` and
     `CanonicalEventRegistered`, the one exported door into the sealed decoder
     (replaces the day-1 `export_test.go` seam). Construction stays sealed.
  2. **`internal/integration/fhirout/`** (new) — `Project(eventType, payload)`
     decodes and maps; `MapEvent` is the legacy engine's view of the same
     switch; `CreateConditionalTransactionBundle` emits
     `PUT <Type>?identifier=<system>|<value>` entries with deterministic
     `urn:uuid:` fullUrls, rewrites intra-bundle references to those fullUrls,
     references the lab Patient conditionally, ensures the key is among the
     resource's identifiers, and strips mapper-assigned ids. Coverage is the
     five registered types the legacy switch handled. `bundle_request.go` /
     `bundle_request_post_control.go` (`fhirpostbundle` tag) are the proof and
     its negative control. `internal/workflow/actions.go`'s typed switch is
     replaced by `fhirout.MapEvent`, so the two engines share one switch.
  3. **`destination/revision.go`** — `TransportFHIR`, `FHIRPolicy`,
     `FHIRInteractionTransaction`, `validateFHIR`, the `validateSemanticFields`
     case (exactly one policy, the one the transport names), `cloneFHIR`,
     `SecretBindingNames`/`EndpointAdvisory` for fhir.
  4. **`destination/fhir_transport.go`** — `deliverFHIR`. The credential,
     trust-root, and client construction shared with `deliverHTTPS` is now one
     helper (`newDestinationClient` + `requestFailure`); `deliverHTTPS` calls
     it too, so 4.1c-b's tests prove the shared half unchanged. Headers per
     spec; response mapping per spec; a `2xx` with a non-2xx entry is a
     terminal rejection; OperationOutcome **issue codes only** are recorded,
     sanitised and bounded — never `diagnostics`.
  5. **Seam widening** — `DestinationTransport.DeliverDestination` and
     `dispatcher.deliverToDestination` pass `item.EventPayload`; `kafka` and
     `https` ignore it. Three test doubles and ten call sites updated.
  6. **`destination/migrations/0003_fhir_delivery_provenance.sql`**,
     `SchemaVersion` 2→3, `DeliveryRecord` gains `FHIRResourceTypes`,
     `FHIREntryCount`, `FHIROutcomeCodesAdvisory`; `RecordDelivery` accepts
     `fhir`. Additive, every `NOT NULL` with a `DEFAULT`, transport CHECK
     re-declared as `IN ('https','fhir')`.
  7. **`processor/workflow_plan.go`** — `FHIR_PROJECTION_UNSUPPORTED` (warning,
     `routes[i].actions[j]`) for a `fhir` action on a route whose event type
     `fhirout` cannot project; the action stays planned, nothing is queued.
  8. **`cmd/fi-fhir/delivery_runtime.go`** — the transport is wired when the
     registry holds any `https` **or** `fhir` destination. No `runServe`
     component; `errCh` untouched.
  9. **The 5.1a gate inverted** — `TestFHIRDestination_DurableEngineDeliversFHIRResource`
     (old doc comment kept as history), driven by `NewProcessedEvent` output in
     the strict-subset shape; asserts the bundle, the conditional urls, the
     fullUrl reference, the content type, the vocabulary `{fhir, https, kafka}`,
     that `fhirout` is the only `pkg/fhir` importer under
     `internal/integration/**`, zero `us-core` issues per resource, the ledger
     record, and — with the in-test server's referential-integrity mode on —
     one stored Patient and one stored Encounter pointing at it.
     Kill-test 2 inverted to `TestFHIRDestination_RedeliveryIsIdempotent`.
  10. **Docs** — `DESTINATION-IDENTITY.md` (transport table, contract, registry
      example, "The FHIR transport (4.1c-c)", verification, operator
      checklist), `WORKFLOW-DSL.md` (which legacy keys the durable `fhir`
      action ignores), `FHIR-CONFORMANCE-MATRIX.md` §5 row 1 and the §0 note,
      `SUPPORTED-1.0.md` standards row clause, `.loom/28` "4.1c-c has landed",
      `.env.example`, two decision entries.
  11. **CI** — `ci/test-fhir-destination.yml` (`test:fhir-destination`,
      blocking, PostgreSQL 16, `-race`, gate + kill-tests + digest pins +
      fhirout + ledger proof + negative control in one job; existence guards
      per package), `make fhir-destination` /
      `fhir-destination-negative-control`, `make fhir-conformance` retargeted
      to the inverted gate, `ci/job-inventory.txt` regenerated.

- Evidence (local, `main`-rebased branch):
  - `make fhir-destination` green; `make fhir-destination-negative-control`
    → `negative control OK: the POST builder duplicates the Patient on
    redelivery (want 1, got 2)`; `make fhir-conformance` green.
  - `TestFHIRDestination_RedeliveryIsIdempotent`: **1 Patient, 1 Encounter**
    after two deliveries of one attempt (day-1: 2 and 2); first delivery
    entries `201 Created`, second `200 OK`; an admit with no visit number →
    `DELIVERY_FHIR_PROJECTION_FAILED`, terminal, zero requests.
  - `TestFHIRTransportRecordsIssueCodesAndNeverDiagnostics`: a 400 whose
    diagnostics carry an MRN and a name records `invalid,not-found` and none
    of the text.
  - `make migration-compatibility` green with `0003` (concurrent replica
    migration, rollback, restore round-trip, the NOT-NULL-needs-DEFAULT rule);
    `TestFHIRDestination_ProvenanceLedgerRecordsFHIRDeliveries` against
    PostgreSQL 16: fhir rows land, the N-1 thirteen-column `https` INSERT
    still lands with the DEFAULTs, `'mllp'` is refused.
  - Digest pins unchanged with the `fhir` field on `Revision`.
  - `go test ./cmd/... ./pkg/... ./internal/...` — 60 packages green
    (`internal/workflow` included: the legacy engine maps through
    `fhirout.MapEvent` with byte-identical output and no key rule).
  - `gofmt -l` clean, `go vet` clean, `golangci-lint run` 0 issues,
    `scripts/validate-docs.sh`, `worklog.sh check`, `decisions.sh check` pass.

- Found while implementing (beyond the day-1 premises):
  - **The legacy engine must not inherit the key rule.** The first cut routed
    `actions.go` through `ProjectEvent`; two legacy tests build events with no
    `Source` and failed on "event names no source". The legacy engine sends
    resources individually and never needed a key, so `MapEvent` (mapping
    only) is its entry point and `ProjectEvent` (mapping + keys) is the
    transport's. One switch, two views.
  - **`CreateTransactionBundle`'s literal references are dangling on a real
    server.** `Encounter.subject = Patient/<mrn>` and `Observation/obs-N` name
    ids the destination never issued; HAPI-class servers with referential
    integrity refuse them. The projection rewrites them to entry fullUrls (or a
    conditional reference for the lab Patient), and the in-test server's
    `strictReferences` mode proves the rewritten bundle is accepted and stored
    as `Patient/<id>`. Provider references (`Practitioner/<id>`) are still
    literal — a documented v1 limitation.
  - **An ORU's Patient is too thin to write.** The lab projection references
    the Patient conditionally instead of including a Patient that would fail
    `us-core` on `birthDate`.

- Needs a coordinator decision (flagged, implemented one way):
  - The deployment-owned identifier system for source-unqualified identifiers
    (`.loom/decisions/2026-09-08-source-assigned-identifiers-are-keyed-under-a.md`)
    — the spec's literal "no system → refuse" would have made every durable
    Encounter undeliverable.

- What's next: 4.2c (the trace shows the FHIR delivery — project
  `integration_destination_deliveries` into `OperatorDeliveryAttempt`), 5.1c
  (`validator_cli.jar` over the delivered bundle), widening the strict A01
  subset so `PV1.19` can carry an assigning authority, and 5.2 SMART Backend
  Services.

- Sources:
  - [S1] `.loom/34-sprint6-execution-specs.md` — Lane S6-A tasks 2–11 and
    acceptance criteria
  - [S2] `.loom/worklog/2026-09-08-slice-4-1c-c-day-1-gates.md` — the four
    inverted premises this implementation starts from
  - [S3] `internal/integration/fhirout/**`, `internal/integration/destination/**`,
    `internal/integration/delivery/{transport.go,dispatcher.go,fhir_*_test.go}`,
    `internal/integration/processor/workflow_plan.go`,
    `internal/workflow/actions.go`, `cmd/fi-fhir/delivery_runtime.go`
  - [S4] `docs/operations/DESTINATION-IDENTITY.md` "The FHIR transport (4.1c-c)"
  - [S5] `.loom/decisions/2026-09-08-slice-4-1c-c-is-a-fhir.md`,
    `.loom/decisions/2026-09-08-source-assigned-identifiers-are-keyed-under-a.md`
