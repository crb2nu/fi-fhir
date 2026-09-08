# 34 — Sprint 6 Execution Specs (the 1.0 critical path opens)

Planned 2026-09-07 against `main` @ `13bf9f4e9` (pipeline 25819 green). Sprint 5
closed 2026-09-05 with every August MR merged; the board has **zero open MRs** and
**zero drafts**. Everything below was read or executed against that commit; line
numbers are exact at that SHA and will drift.

## Goal

Fund the one slice every 1.0 document points at and nobody has written — **4.1c-c,
a FHIR destination class** — and close the three Sprint 5 Wave 3 items whose
blockers have since disappeared: budgets 1–3 certification (the pinned runner now
exists), the red legacy e2e tree (named with evidence, never repaired), and 5.1b
package pinning (5.1a reconciled the mapper and the checker, so a validator over
them no longer certifies a disagreement).

Four premises in the program's own state description invert, one decisively:
**the pinned runner is not missing — it has been online since 2026-08-09 and has
never run a fi-fhir benchmark.**

## Non-Goals

- Do not implement in this planning slice.
- Do not start 5.2 (SMART/Bulk) or 5.3 in any form. 5.1 certification of a
  live path is the *outcome* of this sprint, not its input.
- Do not put a JRE, `validator_cli.jar`, or an IG `.tgz` in the shipped image.
  The confinement half of the validator decision is ratified (2026-08-09).
- Do not add a `runServe` background component. `errCh` is 12
  (`cmd/fi-fhir/main.go:5154`) with S5-D's headroom; nothing here needs it.
- Do not reopen 4.1c-a/4.1c-b contracts. 4.1c-c extends the transport seam
  4.1c-b built; it does not redesign it.
- Do not measure a wall-clock budget in the shared `k3s-ci` pool. Budgets 1–3
  are certified on runner id 8 or not at all.

---

## Backlog state (the first half of the ask)

| Check | Result |
|---|---|
| Open MRs | 0 |
| Draft MRs | 0 |
| Last merged | !200 (Cloudflare Access identity, 2026-09-06), !199 (decisions one-file-per-entry) |
| `main` pipeline | 25819 `success` on `13bf9f4e9` |
| Open issues | 11 — one roadmap tracker (#19), nine P2/P3 planning placeholders from 2026-07-02, the Renovate dashboard (#4). None is a completion blocker. |
| Local worktrees | 12 `.claude/worktrees/*` plus `.worktrees/branding`; all merged or abandoned content. Cleanup is hygiene, not sprint work. |

Nothing is waiting on a merge. The next impact is entirely in new work.

---

## Current-State Corrections From Code

Numbered so lanes can cite them. Each was read at `13bf9f4e9`.

### The FHIR destination gap (Lane S6-A scope)

1. **The vocabulary is unchanged since 5.1a's gate.** `TransportKind` is exactly
   `{kafka, https}` (`internal/integration/destination/revision.go:58,61`);
   `DestinationClass` is exactly `{production, sandbox}`
   (`pkg/integration/revision.go:40-41`). `validateSemanticFields`
   (`revision.go:255-276`) requires exactly one policy per kind and rejects any
   other kind with `ErrInvalidRevision`. The HTTPS transport sets
   `Content-Type: application/json` (`destination/transport.go:369`).

2. **The durable payload is the mapper's input, one decode away.** The outbox
   row's `payload_json` is `ProcessedEvent.PayloadJSON()`
   (`processor/postgres_submission.go:291`; `pkg/integration/contracts.go:282`),
   and `NewProcessedEvent` (`contracts.go:233-252`) builds it by marshalling a
   **concrete `pkg/events` struct** — `adt_a01.go:39` stores
   `*events.PatientAdmitEvent` — after zeroing only the eight
   `forbiddenRawPayloadKeys` (`contracts.go:1200-1209`: `original`,
   `originalpayload`, `parsewarnings`, `raw`, `rawmessage`, `rawpayload`,
   `sourcepayload`, `sourceraw`). No clinical field is touched.
   **`canonicalEventRegistry` (`contracts.go:959-1003`, 44 event types) and
   `decodeCanonicalEventPayload` (`contracts.go:1005`) already reverse the
   projection into the exact types `pkg/fhir.USCoreMapper` consumes.** The
   "which mapper produces the resource" decision the Sprint 5 spec left open
   (`.loom/33:759`) collapses: `pkg/fhir` over the existing decoder. No new
   mapper, no new dependency.

3. **The gate's fixture is not the wire shape.** `fhir_conformance_gate_test.go:28-34`
   hand-writes `{"event_id","event_type":"patient.admitted","tenant_id",…}`.
   The real stored payload is a `pkg/events` struct whose embedded metadata
   carries `id` and `type` (validated to match the row at
   `contracts.go:930-935`), and the real type string is `patient_admit`
   (`pkg/events/events.go:18`). The gate's four assertions are still true, but
   4.1c-c's inverted gate must be driven by `NewProcessedEvent` output, not by
   that literal, or it proves delivery of a payload the engine never stores.

4. **The legacy engine already owns the event→resource switch, and it is
   narrow.** `eventToFHIRResources` (`internal/workflow/actions.go:747-823`)
   dispatches exactly three concrete types — `PatientAdmitEvent` (also carries
   transfer and update per the registry), `PatientDischargeEvent`,
   `LabResultEvent` — and returns `unsupported event type` for the other 41.
   The 26 `Map*` entry points exist (`pkg/fhir/mapper.go`); the switch that
   reaches them does not. 4.1c-c v1 covers the same three, because journey 1
   is ADT and journey 6's validation input is whatever v1 emits.

5. **`CreateTransactionBundle` is not replay-safe.** Every entry is
   `request.method: POST` with `url: <ResourceType>`
   (`pkg/fhir/mapper.go:1288-1305`). The outbox is at-least-once: a lease
   reclaim redelivers the same attempt, and the HTTPS transport's only
   duplicate defence is `Idempotency-Key: <attemptID>`
   (`transport.go:374`), a header no FHIR server honours. **Reusing the bundle
   builder as-is creates a second Patient and a second Encounter on every
   redelivery.** This is the sprint's riskiest assumption; see below.

6. **The identifiers needed for conditional writes exist.** 5.1a backfills
   `Patient.identifier` from `MRN` as an `MR`-typed identifier (worklog
   2026-08-09 "5.1a reconciliation"); `MapEncounter` populates
   `Encounter.identifier` from `e.Identifiers` or `e.ID`
   (`mapper.go:241-244`). Lab `DiagnosticReport`/`Observation` identifiers must
   be checked per resource by the lane — the mapper does not guarantee one.

7. **A plan-level `fhir` action already exists and is inert at dispatch.**
   `deliveryActionTypeV1` (`processor/workflow_plan.go:147-154`) admits
   `fhir` and requires a `DestinationArtifactID`; the planner writes an outbox
   delivery with `Route`, `Action` (the action *ID*), and the destination ref
   (`workflow_plan.go:105-118`). Nothing downstream reads the action *type*:
   the dispatcher carries `item.Action` as an opaque string (`delivery/types.go:45`)
   and the transport switches on `revision.Transport` alone
   (`transport.go:218-232`). So today a `fhir` action on an `https` destination
   delivers the command envelope. This is the documented posture — "the
   transport is a property of the server-owned destination revision, never of
   the workflow" (`docs/operations/DESTINATION-IDENTITY.md:21-22`) — and
   4.1c-c must keep it: the FHIR class is a **transport**, not an action flag.

8. **The revision digest is stable under an added optional policy.** The digest
   is `sha256(domain ‖ JSON(revision-without-digest))` (`revision.go:336-338`)
   and both existing policies are `omitempty` pointers (`revision.go:121-122`).
   A nil `fhir` policy is elided, so every deployed `kafka`/`https` digest is
   byte-stable. This must be *asserted*, not assumed — a day-1 gate pins one.

9. **The provenance ledger cannot record a FHIR delivery.**
   `0002_https_delivery_provenance.sql` declares
   `transport TEXT NOT NULL CHECK (transport IN ('https'))`; destination ledger
   `SchemaVersion = 2` (`destination/postgres.go:16`). 4.1c-c is the first
   destination-ledger migration since Sprint 4: `0003`, `SchemaVersion` 3,
   under the 4.4a rules (additive; every `NOT NULL` carries a `DEFAULT`; the
   N-1 binary must still insert its own rows).

10. **The IDE cannot show a FHIR delivery.** `OperatorDeliveryAttempt`
    (`internal/api/graphql/schema.graphql:2606-2628`) exposes route, action,
    status, error code, lease, and DLQ — and **no field from
    `integration_destination_deliveries`**. Journey 1 ends "inspect trace and
    FHIR delivery"; after 4.1c-c the ledger will hold the fact and the trace
    will not surface it. That projection is a separate, gqlgen-touching slice
    (4.2c below), deliberately not folded into 4.1c-c.

11. **The legacy DSL's `fhir` action carries transport configuration the
    durable model forbids.** `docs/planning/WORKFLOW-DSL.md:150-165` documents
    `endpoint`, `token`, `token_url`, `client_id`, `client_secret` on the
    action. Under 4.1c-a/b, URLs and credential *bindings* live on the
    server-owned revision and a workflow cannot name a URL
    (`DESTINATION-IDENTITY.md:20-21`). 4.1c-c's docs must say which keys the
    durable `fhir` action ignores, or the IDE will let authors write
    configuration that never takes effect.

### The pinned runner (Lane S6-B scope)

12. **Runner id 8 `fi-fhir-perf` exists, is online, and is idle.** Registered
    2026-08-09 (`platform/gitops` `a159dad74`, HelmRelease
    `k3s/ci/gitlab/helmrelease-runner-perf.yaml` on gitops `main`, namespace
    `ci`, `concurrent: 1`, job pods `nodeSelector` → `cblevins-5930k`,
    `cpu_request = "4"`). Live on 2026-09-07: manager pod
    `gitlab-runner-perf-*` Running on `k3s-w-4` (18 restarts in 14 days — note
    it), node `cblevins-5930k` Ready. `GET /runners/8/jobs` returns **one job
    in its history**: 226438 `fi-fhir-perf-smoke`, canceled, 2026-08-09.

13. **The repo side is wired and switched off.** `test:performance-profile`
    (`ci/test-performance-profile.yml:138-181`) carries `tags: [fi-fhir-perf]`,
    4 CPU / 8 GiB, `-benchtime=300x` over the four `BenchmarkDurableAccept_*`
    benchmarks (`internal/integration/perf/accept_bench_test.go:23,46,79,100`),
    the allocation gate, and `scripts/performance-report.sh` producing
    `performance-report.json` (`schema_version: 1`, `certified`,
    `certification.runner_tag`, `budgets[1..7]`). Its first rule is
    `$FI_FHIR_PERF_RUNNER != "1" → when: never`. **The project has no
    `FI_FHIR_PERF_RUNNER` variable** (`GET /projects/19/variables` lists one
    unrelated variable). The job has therefore never existed in any pipeline.

14. **`SUPPORTED-1.0.md` still states the blocker that is gone.** Rows 1–3 of
    the budgets table (`docs/operations/SUPPORTED-1.0.md:160-162`) read
    "Harnessed, uncertified … Certification needs a pinned runner"; row 2 adds
    "and additionally blocked on 4.4e". 4.4e merged 2026-09-05 (!180) and the
    runner exists. Both blockers are stale text.

### The legacy e2e tree (Lane S6-C scope)

15. **Nine tests are red and named, and nothing runs them.**
    `ci/s5b-chaos-dr.yml:90-135` records the execution: in
    `test/e2e/e2e_test.go` (644 lines, tag `e2e`, no infrastructure)
    `TestWorkflowCELFilter` (3 subtests), `TestWorkflowTransform`,
    `TestConfigValidation/invalid_cel`, `TestEndToEndPipeline` (nil-interface
    panic at `e2e_test.go:493`); in `integration_test.go` (450 lines)
    `TestDatabaseAction`, `TestWebhookAction`, `TestWorkflowWithRetry`. Cause
    for most: templates in Go dot-path form (`{{.Patient.Status}}`,
    `e2e_test.go:366`) while the engine binds JSON snake_case keys — the drift
    `5d07101c4` corrected in every document and not in the tests. S5-B made
    exactly one assertion blocking (`TestObservabilityEndpoints`) and left the
    rest unrun. Last touch: `42a1d8f68` (2026-08-09).

### 5.1b (Lane S6-D scope)

16. **Nothing is pinned.** No `hl7.fhir.us.core` or `hl7.fhir.r4.core` artifact
    exists anywhere in the tree outside docs; `testdata/fhir/packages/` does not
    exist. `FHIR-CONFORMANCE-MATRIX.md:295` row 3 says so. The checker is 437
    lines (`pkg/fhir/validate.go`) of profile-presence plus six
    required-element checks; 5.1a generated the 21 byte-exact mapper fixtures
    (`testdata/fhir/mapper/`) that a structural validator must be run over.

### Docs, CI, process

17. **`.loom/30` still blames 4.1c-b.** `30-implementation-plan…:1038-1039`
    reads "5.1 is blocked on Slice 4.1c-b". The 5.1a gate proved 4.1c-b merged
    and did not clear it (`FHIR-CONFORMANCE-MATRIX.md:293`). S6-0 corrects the
    plan to name 4.1c-c.

18. **`ROADMAP.md` "Now" is two phases stale.** Lines 45-77 still show Golden
    Path 001 and Phase 2 unchecked; both merged in June–July. Phase 6 requires
    "docs/status/roadmap match executable behavior".

19. **Shared append points re-conflict on every merge; pre-register them.**
    Sprint 5 lost an afternoon to six MRs appending to the same three files
    (`.loom/40-decisions.md` — now fixed by one-file-per-entry — the Makefile
    `.PHONY` lane block at `Makefile:18-36`, and the `.gitlab-ci.yml` include
    list at `:14-26`). `include: local` of a missing file fails the pipeline,
    and so does a comment-only `ci/*.yml` — it parses to `null`, GitLab
    reports "does not have valid YAML syntax", and the pipeline fails at
    config time with zero jobs (S6-0's first pipeline, 25968, did exactly
    that). A stub must be a non-empty mapping: a hidden, dot-prefixed job is
    valid, adds no job, and passes `ci-job-inventory.sh --check`. S6-0 lands
    every lane's stub include file, include line, `.PHONY` line, and the
    regenerated `ci/job-inventory.txt` on day 1; lanes then edit only files
    they own.

20. **Any `.gitlab-ci.yml` edit runs `security:npm-audit-ui`** (the file is in
    `*ui-changes`). S6-0 runs `cd ui && npm audit --audit-level=high` first and
    carries a `--package-lock-only` fix if an advisory has landed since !197.

21. **Any `when: manual` job on `merge_request_event` gets `allow_failure: true`
    on that rule** or it re-parks every Go MR at `manual` (the !179 lesson,
    `d90506d8b`). Review with `grep -n -B2 'when: manual' ci/*.yml`.

---

## The Sprint's Riskiest Load-Bearing Assumption

> **"The stored canonical payload round-trips into the exact mapper input, and a
> FHIR write built from it can be redelivered without creating duplicates."**

Two halves, both testable on day 1 with no product change.

### Kill-test 1 — must PASS on unmodified `main`: `TestFHIRDestination_DurablePayloadRoundTripsToMapperInput`

Parse the golden ADT A01 fixture through the real processor path to a
`*events.PatientAdmitEvent`; `NewProcessedEvent` it; take `PayloadJSON()`;
`decodeCanonicalEventPayload(events.EventPatientAdmit, payload)`; map the
decoded event **and** the original with `NewUSCoreMapper().MapPatient/MapEncounter`;
assert the two resource sets are byte-equal after marshalling; assert zero
issues from `ValidateJSON` at `us-core --strict`. Repeat for the discharge and
lab-result fixtures. If this fails, the blocker is the redaction list or the
registry, and S6-A's first task changes from "build the transport" to "widen the
decoder" — say so and stop.

### Kill-test 2 — must FAIL on unmodified `main`, for the named reason: `TestFHIRDestination_RedeliveryIsIdempotent`

Stand an in-test FHIR server that implements transaction-Bundle semantics with
a resource store keyed on `identifier` (conditional `PUT <Type>?identifier=…`
updates in place; `POST` always inserts). Feed the mapper's Patient+Encounter
through `CreateTransactionBundle` **twice** with the same attempt id. On `main`
the store holds **two** Patients and **two** Encounters, and the test fails on
exactly `want 1 Patient, got 2`. After S6-A the FHIR transport must emit
conditional entries and the count must be 1. A negative control keeps the
`POST` builder behind a build tag and requires the count to return to 2.

The second gate is what a coordinator needs to refuse a "reuse
`CreateTransactionBundle`" shortcut: it would pass every existing test and
duplicate every patient on the first lease reclaim.

### Sprint-level negative control

`TestFHIRConformance_DurableEngineProducesNoFHIRResource` must still PASS on
`main` on day 1 (it is the record of the world before 4.1c-c) and must FAIL on
S6-A's branch for all four of its reasons before S6-A rewrites it as the
inverted gate. A branch on which it still passes has not built a FHIR
destination.

---

## Parallelization Map

| Lane | Slice | Owns | Collides with | Merge position |
|---|---|---|---|---|
| **S6-0** | merge surface + stale-doc repair | stub `ci/*.yml`, include lines, `.PHONY` lines, `ci/job-inventory.txt`, `.loom/30` 5.1 block, `ROADMAP.md` Now/Next | everyone on day 1 — that is the point | **FIRST**, day 1 |
| **S6-A** | 4.1c-c FHIR destination class | `internal/integration/destination/**`, new `internal/integration/fhirout/**`, `delivery/{transport.go,dispatcher.go}` seam widening, `delivery/fhir_conformance_gate_test.go` (inverted), `pkg/integration/contracts.go` (export one decoder wrapper), `cmd/fi-fhir/delivery_runtime.go`, `ci/test-fhir-destination.yml`, docs rows named below | S6-D on `FHIR-CONFORMANCE-MATRIX.md` (§5 row 1 vs row 3) | LAST — widest |
| **S6-B** | budgets 1–3 certification on runner 8 | `docs/operations/SUPPORTED-1.0.md` rows 1–3, `scripts/performance-report.sh` (if the schema needs a negative-control field), `ci/test-performance-profile.yml` (only if a rule changes) | S6-A on `SUPPORTED-1.0.md` (standards row vs budgets rows — different sections) | whenever; smallest |
| **S6-C** | legacy e2e tree repair | `test/e2e/**`, `ci/test-e2e-legacy.yml` | nobody | whenever |
| **S6-D** | 5.1b package pinning + Option C | `pkg/fhir/**`, `testdata/fhir/packages/**`, `ci/test-fhir-structural.yml`, `.trivyignore`/scan skip list if needed | S6-A on the matrix | before S6-A |

Only two orderings are load-bearing: **S6-0 first** (everyone's include line),
and **S6-D before S6-A** if S6-A wants to run the structural validator over its
delivered bundle in its own gate (optional; S6-A's acceptance uses 5.1a's
checker, which is enough for v1).

## Schema Freeze Status Per Ledger

| Ledger | Head | Sprint 6 |
|---|---|---|
| processor | `0005_retention_expiry.sql` | FROZEN. `0006` is free (S5-F released it; 4.4a's `0006_export_attribution_defaults.sql` is in the *session* ledger). |
| destination | `0002_https_delivery_provenance.sql`, `SchemaVersion = 2` | **UNFROZEN for S6-A only**: `0003_fhir_delivery_provenance.sql`, `SchemaVersion = 3`. |
| lifecycle | `0002` (S5-D) | FROZEN |
| session | `0007` (S4-C) | FROZEN |
| terminology, retention | unchanged | FROZEN |

Rules that bind S6-A's migration (from 4.4a, enforced by `make migration-compatibility`):
additive only; every `NOT NULL` column carries a `DEFAULT`; the widened CHECK
is re-declared, not `ALTER`ed in place, and is N-1 safe because the N-1 binary
never writes `'fhir'`; the N-1 restore proof must still insert its own rows.

## Coordination Rules

- **S6-0 lands before any lane opens an implementation MR.** Lanes may open
  test-only day-1 gate MRs immediately; those touch no shared file.
- **No lane touches `CHANGELOG.md`.** The coordinator writes one `[Unreleased]`
  block at sprint close. This removes the last shared append point.
- **Worklog and decisions are one file per entry** (`make worklog-new`,
  `make decisions-new`). Never append to `50-worklog.md` or `40-decisions.md`.
- **Every new CI job**: `scripts/ci-job-inventory.sh --write` in the same
  commit; `--check` is blocking in `lint:docs`.
- **Migration numbers are claimed in the ledger by the MR that ships them**,
  not in the worklog. Only S6-A claims one this sprint.
- **Watchers**: the coordinator owns MR babysitting (background children die
  with their agent). Arm MWPS after every push; pushing cancels an armed MWPS.
- **Scratchpad payload files carry a lane prefix** (`s6a-mr.json`), never
  `mr.json`.

## Merge Order

`S6-0 → {S6-B, S6-C} whenever → S6-D → S6-A`. With shared append points
pre-registered, a merge no longer conflicts its siblings; if one does, rebase
with `--onto` so only the lane's own commits replay (see Sprint 5's recipe).

---

## Lane S6-0 — Merge Surface and Stale-Doc Repair

**Branch**: `chore/sprint6-merge-surface`. Coordinator-owned, day 1, one MR.

### Tasks

1. Stub `ci/test-fhir-destination.yml`, `ci/test-e2e-legacy.yml`,
   `ci/test-fhir-structural.yml` — each a comment block naming its lane and
   the job it will hold, plus one hidden (dot-prefixed) placeholder job so the
   file is a non-empty mapping (correction 19). Add the three `include: local:` lines
   (`.gitlab-ci.yml:14-26`, no leading slash). Add three `.PHONY` lines in the
   lane block (`Makefile:18-36`, one line per lane with the `# <slice> — <lane>`
   comment). Regenerate `ci/job-inventory.txt` (unchanged output proves the
   stubs add no job).
2. `cd ui && npm audit --audit-level=high`; if red, `npm audit fix --package-lock-only`
   rides in this MR (correction 20).
3. `.loom/30…:1038-1065`: replace "blocked on Slice 4.1c-b" with the 4.1c-c
   finding and cite `TestFHIRConformance_DurableEngineProducesNoFHIRResource`.
   Add a "Slice 4.1c-c" subsection under 4.1 pointing at this spec.
4. `ROADMAP.md`: check the merged Now/Next items with their MR numbers; add
   Sprint 6 under Now. Keep it to state, not narrative.
5. Worklog entry: "Sprint 6 merge surface".

### Acceptance

- Pipeline green with the stubs; `ci-job-inventory.sh --check` passes.
- `grep -n '4.1c-b' .loom/30-*.md` no longer returns the 5.1 blocker sentence.

---

## Lane S6-A — Slice 4.1c-c: FHIR Destination Class

**Branch**: `feat/phase4-slice-4-1c-c-fhir-destination`. The 1.0 critical path.

### The transport-vs-flag decision (required deliverable; the lane's gate)

**Option A — a new `TransportKind = "fhir"` with its own `FHIRPolicy`.**
`FHIRPolicy{BaseURL, TokenBinding, CABundleBinding, Interaction}` where
`Interaction` is the closed set `{transaction}` in v1. Validation mirrors
`validateHTTPS` (`https` scheme, no userinfo, no fragment, ≤ 2048 bytes,
bindings by name). Provenance rows carry `transport = 'fhir'`.

**Option B — an encoding flag on `HTTPSPolicy`** (`content: fhir-r4-transaction`).

**Recommendation: A.** Correction 7's principle (transport is a property of the
revision) is preserved either way, but only A gives the FHIR path its own
response semantics — a transaction response is `200` with per-entry statuses,
and a `4xx` carries an `OperationOutcome` that must be *bounded and stripped of
`diagnostics` text* before it touches a ledger — without contaminating the
generic HTTPS class. A also makes the 5.1a gate's inversion explicit:
`assertNoFHIRTransportVocabulary` flips on exactly one new admitted value.

### Goal

One production submission of an ADT A01 through durable admission, routed by a
published `fhir` action to a `fhir`-transport destination, delivers a FHIR R4
transaction Bundle of a US Core Patient and Encounter over TLS under the
destination's declared identity, at `application/fhir+json`, with conditional
writes such that redelivery is idempotent, and records the delivery in the
destination ledger with the resource types delivered.

### Non-Goals

- No new `runServe` component. The transport runs inside the dispatcher's
  existing lease and `PublishTimeout` bound (`dispatcher.go:239-262`).
- No `PUT`-per-resource interaction, no `$process-message`, no Subscriptions.
  `transaction` only.
- No IDE surface for the delivery (that is 4.2c, below).
- No coverage beyond the three event types the legacy switch already handles
  (correction 4). Others produce a **plan-time** diagnostic, not a dispatch
  failure.
- No OAuth client-credentials flow. Bearer material by binding, exactly as
  HTTPS (`transport.go:320-333`). SMART Backend Services is 5.2.
- No change to `store.go`. The state machine stays transport-blind.

### Tasks

1. **Day 1, test-only** — the two kill-tests above and the digest-stability
   gate: pin the current digest of a fixed `https` revision and a fixed `kafka`
   revision as string constants; after the `fhir` field lands they must still
   match (correction 8). Plus the sprint-level negative control run.
2. **`internal/integration/fhirout/`** (new) — `Project(eventType events.EventType,
   payload json.RawMessage) (Projection, error)` using an exported thin wrapper
   over `decodeCanonicalEventPayload` (`contracts.go:1005`; export only the
   decode, keep construction sealed) and the three-case switch ported from
   `actions.go:747-823`. `Projection` carries the resources **and, per
   resource, the identifier chosen for the conditional write**; a resource
   with no usable identifier is a projection error, never a `POST`.
   `CreateConditionalTransactionBundle(projection)` emits
   `PUT <Type>?identifier=<system>|<value>` entries. Then make
   `internal/workflow/actions.go` call `fhirout.Project` so the two engines
   cannot drift the way the mapper and checker did (5.1a's disease).
3. **`destination/revision.go`** — `TransportFHIR`, `FHIRPolicy`,
   `validateFHIR`, the `validateSemanticFields` case, `cloneFHIR`. Registry
   loading needs nothing: it validates through `NewRevision`.
4. **`destination/fhir_transport.go`** — `deliverFHIR`: extract the
   credential/trust-root/client construction shared with `deliverHTTPS` into
   one helper (do not copy `transport.go:320-363`); body is the conditional
   bundle; headers `Content-Type: application/fhir+json; charset=utf-8`,
   `Accept: application/fhir+json`, `Prefer: return=minimal`, and the same
   `Idempotency-Key`. Response mapping: `200/201` with every entry
   `2xx` → delivered; any entry `4xx` or a top-level `4xx` → `FailureRejected`
   (terminal) with **issue codes only** recorded; `408/429/5xx` → retryable as
   HTTPS does (`transport.go:393-410`). Drain and bound the body as HTTPS does.
5. **Seam widening** — `delivery/transport.go`'s `DestinationTransport`
   interface and `dispatcher.deliverToDestination` pass `item.EventPayload`
   alongside the encoded command. `handled` semantics unchanged; `kafka` and
   `https` ignore the new argument.
6. **`destination/migrations/0003_fhir_delivery_provenance.sql`** — re-declare
   the `transport` CHECK as `IN ('https','fhir')`; add
   `fhir_resource_types TEXT NOT NULL DEFAULT ''` (bounded 512, comma-joined
   types), `fhir_entry_count INTEGER NOT NULL DEFAULT 0`,
   `fhir_outcome_codes_advisory TEXT NOT NULL DEFAULT ''` (bounded 512,
   `COMMENT ON COLUMN` stating it never carries `diagnostics` text).
   `SchemaVersion = 3`; `DeliveryRecord` gains the three fields.
7. **Plan-time diagnostic** — in `workflow_plan.go`, when an action of type
   `fhir` targets a route whose event type `fhirout` cannot project, emit a
   diagnostic code (`FHIR_PROJECTION_UNSUPPORTED`) on the route so dry-run and
   publish show it (IDE/runtime parity, proof-matrix row 5).
8. **`cmd/fi-fhir/delivery_runtime.go`** — wire the transport when the
   registry holds any `https` **or** `fhir` destination.
9. **Invert the 5.1a gate deliberately.** Rename to
   `TestFHIRDestination_DurableEngineDeliversFHIRResource`; drive it with
   `NewProcessedEvent` output (correction 3); assert `resourceType: Bundle`,
   `type: transaction`, conditional `request.url`s, the content type, the
   admitted vocabulary `{fhir, https, kafka}`, and that
   `internal/integration/fhirout` is the **only** non-test importer of
   `pkg/fhir` under `internal/integration/**`. Keep the old function's doc
   comment as history above the new one.
10. **Docs** — `DESTINATION-IDENTITY.md` transport table gains the `fhir` row;
    `WORKFLOW-DSL.md` FHIR action section states which legacy keys the durable
    engine ignores (correction 11); `FHIR-CONFORMANCE-MATRIX.md` §5 row 1
    flips to "satisfied by 4.1c-c, proven by <gate>"; `SUPPORTED-1.0.md`
    standards row gains "durable delivery of US Core Patient/Encounter/
    DiagnosticReport/Observation over the `fhir` transport"; `.loom/28` and
    `.loom/30` 5.1 blocks updated. Decision entry for Option A.
11. **CI** — fill `ci/test-fhir-destination.yml` with `test:fhir-destination`
    (PostgreSQL 16 service, `-race`, runs the gate, kill-tests, and the
    negative control in one job; blocking; correction 21 if any manual rule).
    `make fhir-destination` / `fhir-destination-negative-control`.

### Acceptance Criteria

- Kill-test 1 passes; kill-test 2 passes with the count at 1 and its negative
  control returns it to 2.
- The inverted gate passes; the digest-stability gate passes.
- `make migration-compatibility` green with `0003` (N-1 insert, restore
  round-trip, NOT-NULL-needs-DEFAULT rule).
- `make delivery-identity`, `make delivery-reliability`,
  `make destination-transport` unchanged and green — `kafka` and `https`
  behaviour is byte-identical (the HTTPS gate still reads the envelope).
- `make fhir-conformance`: every resource in the delivered bundle validates at
  `us-core --strict` with zero issues.
- A `fhir` action on a `vital_sign` route produces
  `FHIR_PROJECTION_UNSUPPORTED` at dry-run and publishes nothing to the outbox
  for that action.
- No `runServe` change; `errCh` untouched.
- gofmt, `golangci-lint`, `go vet`, gosec, govulncheck clean.

### Riskiest Assumption (lane-level)

> "A conditional `PUT ?identifier=` is enough for idempotency against real
> FHIR servers."

It is enough for HAPI and for the in-test server, and it is the standard
answer. It is not enough if the destination's identifier system is empty:
`?identifier=|MRN-1` matches any system. Task 2 must refuse a projection whose
chosen identifier has no `system`, and the day-1 in-test server must reject a
bare-value identifier search with `400` so the refusal is exercised, not
assumed.

---

## Lane S6-B — Budgets 1–3 Certification on the Pinned Runner

**Branch**: `docs/perf-budgets-certified`. Small, high-leverage: it changes what
the RC gate is allowed to claim.

### The one step outside the repo (operator action, before day 1)

Set the GitLab project variable `FI_FHIR_PERF_RUNNER = "1"` on project 19
(unprotected, unmasked). This is persistent CI configuration and is **not**
something an agent sets on its own; the coordinator asks. Until it is set,
`test:performance-profile` is `when: never` (correction 13) and this lane
cannot start.

### Tasks

1. **Day 1, negative control first.** Add a build-tagged regression to the
   durable accept path (a `time.Sleep(300ms)` behind `-tags perfregress` in
   the benchmark setup, never in product code). Play `test:performance-profile`
   on a branch with the tag; the report must come back `certified: false`
   with budget 1 failed for the named reason. A report that still certifies
   proves the harness measures something other than the budget — stop and fix
   the harness before certifying anything (the MinIO hollow-green lesson).
2. Play the job three times on identical `main` code. Record `cpu:` header,
   p50/p95/p99 per benchmark, `allocs/op`. Spread across runs must be
   ≤ 1.15× on p95 or the node is not a reference host — say so in
   `SUPPORTED-1.0.md` and stop.
3. Flip rows 1 and 3 to **Certified** with job ids and the archived
   `performance-report.json` cited; row 2 (one-hour steady state) stays
   **Harnessed** unless a `-benchtime=1h` variant is added and run — decide
   in the lane, record either way. Remove the "needs a pinned runner" and
   "blocked on 4.4e" text.
4. If `performance-report.sh` lacks a field for the negative-control run, add
   `negative_control: {ran, failed_budget}` under `schema_version: 2`.
5. Decision entry: "Budgets 1 and 3 are certified on runner 8; budget 2's
   status". Worklog entry with the three job ids.

### Acceptance

- Three archived reports on `main`, `certified: true`, from runner id 8
  (`certification.runner_tag == "fi-fhir-perf"`).
- One archived report from the negative-control branch, `certified: false`.
- `SUPPORTED-1.0.md` rows 1–3 no longer mention a missing runner or 4.4e.

### Riskiest Assumption

> "Runner 8's node is a stable reference host."

It is the control-plane node (`cblevins-5930k`, `control-plane,master`) and the
runner manager has restarted 18 times in 14 days. Task 2's spread check is the
kill-test; if it fails, the fix is in `platform/gitops`, not here.

---

## Lane S6-C — Legacy e2e Tree Repair

**Branch**: `test/e2e-legacy-repair`.

### Honest scoping

These are legacy-engine tests. They are not on the 1.0 critical path and they
prove no golden journey. They are on the Phase 6 path because "docs/status
match executable behavior" is false while `make test-e2e` is a target that
fails and no job says so.

### Tasks

1. **Day 1**: run `make test-e2e` and `go test -tags=e2e,integration ./test/e2e/...`
   against the S5-B service set; record the failing set. It must be exactly
   the nine in correction 15 — a different set means the tree moved again.
2. Repair the template-syntax drift (`{{.Patient.Status}}` → snake_case JSON
   keys) across both files. Re-run. Whatever still fails is **not** drift.
3. `TestEndToEndPipeline`'s nil-interface panic at `e2e_test.go:493` and
   `TestWorkflowWithRetry`'s timing assertion are the two most likely to
   survive step 2. A panic that survives is an engine defect: file it with the
   reproduction and skip the test with the issue id in the skip message. A
   timing assertion gets the `TestQuickBenchmark` treatment — assert on
   attempt count, not elapsed time.
4. Fill `ci/test-e2e-legacy.yml` with `test:e2e-legacy` (blocking; same
   service pattern as `ci/s5b-chaos-dr.yml`, binary exec'd against the
   service container, `FI_FHIR_E2E_REQUIRED_SERVICES` declared).
5. Retire `make e2e-up`/`e2e-down` and `test/e2e/docker-compose.yaml` if the
   job proves the Compose file runs no application container (correction 15,
   `s5b-chaos-dr.yml:128-130`), or make it run one. Do not leave a Compose
   file that "works" by starting nothing.

### Acceptance

- `test:e2e-legacy` blocking and green; zero `t.Skipf` without an issue id.
- Any surviving failure is filed with a reproduction, not hidden.

### Riskiest Assumption

> "It is only template drift."

Kill-test is task 2's re-run: repair *only* templates first and read what is
left before touching anything else.

---

## Lane S6-D — Slice 5.1b: Package Pinning and the Structural Validator

**Branch**: `feat/phase5-slice-5-1b-package-pinning`.

### Tasks

1. **Pin** `hl7.fhir.r4.core#4.0.1` and `hl7.fhir.us.core#9.0.0` as offline
   `.tgz` under `testdata/fhir/packages/` with sha256 sums checked in and
   verified by a test. Confirm placement against the trivy skip list per the
   ratified confinement half (the image is untouched; `security:trivy-image`
   scans the binary, not the tree — but `security:trivy-fs`, if any, must
   not choke on the archives).
2. **Option C** — a Go structural validator over the pinned packages: resolve
   each profile's `StructureDefinition`, enforce cardinality and must-support
   presence for the six required-element types 5.1a counted (Condition,
   Coverage, DiagnosticReport, Encounter, Observation, Patient), and run it
   over all 21 mapper fixtures in `testdata/fhir/mapper/`. Version tolerance
   stays as 5.1a decided (bare canonical asserted; `|9.0.0` accepted).
3. Replace the "0 of 32 pinned" statement in `FHIR-CONFORMANCE-MATRIX.md` §5
   row 3 and `SUPPORTED-1.0.md` with the pinned versions and what the
   structural validator does and does not check. Do **not** claim official
   conformance — Option A (`validator_cli.jar`, CI-only) is Sprint 7.
4. Fill `ci/test-fhir-structural.yml` with `test:fhir-structural` (blocking).

### Acceptance

- Every mapper fixture passes the structural validator; a fixture with a
  required element removed fails it (negative control).
- No `go.mod` dependency for package parsing beyond `archive/tar` and
  `compress/gzip`; no image change.

### Riskiest Assumption

> "The package `.tgz` files can live in the tree."

`hl7.fhir.r4.core` is ~10 MB. If a size or scan gate rejects it, vendor only
the `StructureDefinition` resources v1 needs, extracted by a documented script
with the sums of the source packages recorded — and say so in the matrix.

---

## Suggested Execution Order

### Day 1 — in parallel

- S6-0 MR (coordinator).
- S6-A: three test-only gate MRs (kill-test 1, kill-test 2, digest stability)
  plus the negative-control run of the 5.1a gate.
- S6-B: operator sets `FI_FHIR_PERF_RUNNER`; negative-control branch played.
- S6-C: failing-set run recorded.
- S6-D: packages pinned, sums checked in (test-only MR).

### Wave 2 — implementation; merge `S6-0 → {S6-B, S6-C} → S6-D → S6-A`

### Wave 3 — beyond Sprint 6

- **4.2c — the trace shows the FHIR delivery.** Project
  `integration_destination_deliveries` into `OperatorDeliveryAttempt`
  (correction 10). gqlgen-touching (`lint:gqlgen` cold-cache ~20 min); own
  slice.
- **5.1c — Option A**, `validator_cli.jar` CI-only over the delivered bundle
  from 4.1c-c's gate. Then and only then the "official validator evidence"
  clause in `SUPPORTED-1.0.md:26` can change.
- **4.4d-b — tracing exporter and `correlation_id` at the Observe seam** (S5-C
  follow-on; no OTLP exporter exists in `cmd/fi-fhir` today).
- **Budget 2 at one hour** if S6-B leaves it harnessed; **budget 7** on
  Kubernetes 1.36 (needs a cluster run, not CI).
- **Inbound conformance decision** for `internal/fhir/subscription`
  (matrix §6's three questions).
- **5.2 SMART Backend Services + Bulk Data** — journey 6's remaining half,
  unblocked by 4.1c-c producing a validated resource.

---

## Decisions Required Before Lanes Launch

1. **4.1c-c shape — Option A (new transport kind) or B (HTTPS flag).**
   Recommendation A, above. Blocks S6-A's first implementation commit; not its
   day-1 gates.
2. **`FI_FHIR_PERF_RUNNER = "1"` on project 19.** Operator action. Blocks S6-B
   entirely.
3. **Sprint 6 scope for S6-A's projection coverage** — three event types
   (recommended) or the full 26-mapper surface. Three is what journey 1 needs
   and what the legacy engine has ever executed; the plan-time diagnostic makes
   the boundary visible instead of silent.

### Two things the coordinator should know even though they need no decision

- **Nothing in this sprint is a release blocker found in merged code.** Sprint
  5 opened with two P0s (D1, D2); this one opens with zero. The risk is in the
  new seam, and it is testable on day 1 before a line of product code.
- **The perf runner's manager pod restart count** (18 in 14 days) is a
  `platform/gitops` observation, not a fi-fhir defect. S6-B's spread check will
  either clear it or turn it into a gitops task with numbers attached.

---

## Sources

All read at `main` @ `13bf9f4e9` on 2026-09-07.

- **Destination and delivery seam**: `internal/integration/destination/revision.go`
  (`:36,48,58,61,106-131,255-276,336-338`), `destination/transport.go`
  (`:142-167,218-232,307-410`), `destination/postgres.go:16`,
  `destination/migrations/0002_https_delivery_provenance.sql`,
  `internal/integration/delivery/dispatcher.go` (`:215-262,327-360`),
  `delivery/types.go:36-51`, `delivery/fhir_conformance_gate_test.go`,
  `cmd/fi-fhir/delivery_runtime.go`, `cmd/fi-fhir/main.go:5154`.
- **Canonical event projection**: `pkg/integration/contracts.go`
  (`:222-252,282,930-935,940-1070,1200-1209`), `pkg/integration/revision.go:40-41`,
  `internal/integration/processor/adt_a01.go:39`,
  `processor/postgres_submission.go:281-300`,
  `processor/workflow_plan.go:97-118,147-154`.
- **Mapper and checker**: `pkg/fhir/mapper.go` (`:120,226-244,398,1288-1305`,
  26 `Map*`), `pkg/fhir/types.go:240-242`, `pkg/fhir/validate.go` (437 lines),
  `internal/workflow/actions.go` (`:707,747-823,937,1064`),
  `testdata/fhir/mapper/` (21 fixtures).
- **IDE**: `internal/api/graphql/schema.graphql:2606-2628`.
- **Perf**: `ci/test-performance-profile.yml:138-181`,
  `scripts/performance-report.sh`, `internal/integration/perf/accept_bench_test.go`,
  `docs/operations/SUPPORTED-1.0.md:151-178`; `platform/gitops` `a159dad74`,
  `719bbc9fe`, `k3s/ci/gitlab/helmrelease-runner-perf.yaml`; GitLab
  `GET /runners/8`, `GET /runners/8/jobs`, `GET /projects/19/variables`;
  `kubectl -n ci get pods`, `kubectl get nodes` on 2026-09-07.
- **e2e**: `ci/s5b-chaos-dr.yml:85-135`, `test/e2e/*.go`, `git log -- test/e2e/`.
- **Docs and process**: `docs/operations/DESTINATION-IDENTITY.md:10-24`,
  `docs/planning/WORKFLOW-DSL.md:150-165`,
  `docs/planning/FHIR-CONFORMANCE-MATRIX.md:283-340`,
  `.loom/30-implementation-plan-integration-engine-ide-completion.md:1030-1121`,
  `.loom/33-sprint5-execution-specs.md:746-1006`,
  `.loom/decisions/2026-08-08-fhir-conformance-validation-strategy-for-slice-5.md`,
  `.loom/worklog/2026-08-09-slice-5-1a-reconciliation-the-mapper-validates.md`,
  `ROADMAP.md:45-77`, `Makefile:18-36`, `.gitlab-ci.yml:14-26,438`,
  `ci/job-inventory.txt`; GitLab `GET /projects/19/merge_requests?state=opened`
  (0), `?state=merged` (through !200), `GET /projects/19/issues?state=opened` (11),
  `GET /projects/19/pipelines?ref=main` (25819 success).
