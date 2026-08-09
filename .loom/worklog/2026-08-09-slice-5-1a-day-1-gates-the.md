### 2026-08-09 - Slice 5.1a day-1 gates: the durable engine produces no FHIR resource, and the validator rejects the mapper

- Lane: **S5-E**, Sprint 5 (`.loom/33-sprint5-execution-specs.md`). Branch
  `feat/phase5-slice-5-1a-conformance-reconciliation`, worktree
  `.worktrees/sprint5-conformance`, based on `main` @ `2f8b3f609`.

- Owned files for this lane (declared before the first commit, per the
  coordination rules; no other lane touches these):
  - `pkg/fhir/**`, `pkg/validate/**`, `testdata/fhir/**`
  - `internal/fhir/subscription/**` (scoping statement only)
  - `docs/planning/FHIR-*.md`, `.loom/28-spec-fhir-ig-bulk-smart.md`
  - the S5-E amendment to `.loom/40-decisions.md` (amended in place, not appended)
  - one `.PHONY` line and one target block in `Makefile`
  - one new test-only file under `internal/integration/delivery/` for the second
    day-1 gate (see below for why it lives there and why it is untagged)
  - **No migration number is claimed by this lane.** No `.gitlab-ci.yml` change —
    S5-0a has not merged.
  - One contended surface, coordinated not edited: `test:benchmark`'s package
    list includes `./pkg/validate/...` and S5-A may narrow it.

- What changed (test-only plus docs; no production behaviour):
  - **`internal/integration/delivery/fhir_conformance_gate_test.go`** (new,
    untagged) — `TestFHIRConformance_DurableEngineProducesNoFHIRResource`.
  - **`pkg/fhir/conformance_day1_gate_test.go`** (new, behind the `fhirday1gate`
    build tag) — `TestFHIRConformance_ValidatorRejectsMapperOutputToday`.
  - **`Makefile`** — one `.PHONY` line and the `fhir-conformance-day1-gate`
    target, which runs both gates and asserts their *opposite* expected results.
  - **`.loom/40-decisions.md`** — the PROPOSED 2026-08-08 FHIR-validator entry is
    amended in place (one entry, not two) with the coordinator's split ruling.
  - **`.loom/28-spec-fhir-ig-bulk-smart.md`** — the replacement kill-test is
    recorded as RUN, with its answer, so no agent re-derives it.

- Why:
  - The sprint scope called 5.1 "the now-unblocked 5.1 code start" on the
    strength of 4.1c-b having merged. `.loom/28:153-160` wrote the kill-test for
    exactly that moment and nobody had run it. Running it is the single most
    consequential artifact this lane produces, because it is what a coordinator
    needs in order to decide whether to fund 4.1c-c.
  - The other half of 5.1 opens with a defect, not an integration: the validator
    this repo ships rejects the mapper this repo ships, and CI structurally
    cannot see it.

- Evidence:
  - **Gate 2 — must PASS on `main`, and does.**
    `go test -run TestFHIRConformance_DurableEngineProducesNoFHIRResource
    ./internal/integration/delivery` → `ok`. It stands a live TLS endpoint,
    deploys an `https`-transport destination pointing at it, and runs the real
    dispatcher (real `messageForWorkItem`, real `destination.Transport`, real
    `net/http`) for one claimed durable work item, then reads the bytes that
    crossed the wire:
    1. the body is the delivery-command envelope `integration.delivery.v1`, all
       eleven members present, whose `event` is the canonical event verbatim;
       neither carries `resourceType`;
    2. `Content-Type: application/json`, not `application/fhir+json`;
    3. the transport vocabulary admits exactly `{https, kafka}` — asserted by
       feeding nine candidate kinds (`fhir`, `fhir+json`, `fhir-r4`, `mllp`, …)
       through `destination.NewRevision` and recording which are accepted, so a
       kind added later turns the gate red instead of leaving it stale — and
       `DestinationClass` is `production|sandbox`, an environment class;
    4. an AST walk over every non-test `.go` file under `internal/integration/**`
       finds **zero** `pkg/fhir` imports (with a `scanned == 0` guard so a broken
       walk cannot manufacture the zero).
    Controls: the fake broker is asserted to have received nothing, so the gate
    is reading the HTTPS path and not the Kafka one; the destination is asserted
    to have served exactly one request, so an empty capture cannot pass.
  - **Gate 1 — must FAIL on `main`, and does, for the named reason.**
    `make fhir-conformance-day1-gate` →
    `day-1 gate reproduced: meta.profile does not include an expected profile for
    DiagnosticReport`. Raw:
    `--- FAIL: TestFHIRConformance_ValidatorRejectsMapperOutputToday` …
    `warning value: meta.profile does not include an expected profile for
    DiagnosticReport`, with the offending bytes printed —
    `"meta":{"profile":["…/us-core-diagnosticreport-lab"]}`. `MapLabResult`
    stamps `-lab` as a bare literal (`pkg/fhir/mapper.go:435`, never declared as
    a constant, which is how it fell out of the map) while
    `expectedProfilesForResourceType` accepts only `-note`
    (`pkg/fhir/validate.go:218-219`).
    Control in the same run: the repo's own
    `testdata/fhir/diagnosticreport_uscore_note.json` validates with **zero**
    issues, so the failure is the mapper/checker disagreement and not a broken
    checker. Second control: the report is marshalled and re-parsed, so the gate
    validates the bytes the wire would carry rather than a struct field.
    Why CI cannot see it today: `pkg/fhir/validate_golden_test.go:17` feeds the
    checker the hand-written `-note` fixture, and `testdata/fhir/` contains no
    lab `DiagnosticReport`. The only failing input is the one the mapper actually
    produces.
  - `go vet` clean under all three tag combinations (none, `integration`,
    `fhirday1gate`); `golangci-lint run ./pkg/fhir/... ./internal/integration/delivery/...`
    → **0 issues**.

- Decisions recorded (`.loom/40-decisions.md`, 2026-08-09 amendment, in place):
  - **Confinement half RATIFIED** — `validator_cli.jar` stays CI-only, the
    shipped image stays distroless static, IG packages are pinned offline `.tgz`.
  - **Ordering half AMENDED** — reconcile first (5.1a), then Option C, then
    Option A. A larger validator built over a mapper it disagrees with certifies
    the disagreement at higher resolution.
  - **4.1c-c named** as 5.1's real prerequisite, out of Sprint 5 scope.
  - The entry's own closing line — "Slice 5.1 remains blocked on Slice 4.1c-b
    regardless of which engine wins" — is annotated rather than deleted: 4.1c-b
    merged, and 5.1 is still blocked.

- Notes for whoever picks this up:
  - Gate 2 is deliberately **untagged and service-free**. The obvious home was
    the `integration`-tagged harness in
    `internal/integration/delivery/destination_transport_*_test.go`, which has
    everything (Postgres, Kafka, recording TLS destinations) — but the
    `test:destination-transport` job pins `-run`/`-list` to two exact test names
    with an arity-2 existence guard, and this sprint forbids both appending to
    `.gitlab-ci.yml` before S5-0a merges and changing another job's `-list`
    arity. An untagged test needs no job: it runs in the ordinary `go test ./...`
    and cannot be skipped into greenness.
  - Gate 1 is behind a build tag because it must fail. Untagged, it would turn
    `test:go` red on a test-only MR. The Makefile target inverts the exit status
    **and** greps for the named reason, so a compile error cannot satisfy it.
    When 5.1a's reconciliation lands, the tag comes off, the assertion joins the
    untagged conformance table, and the target stops inverting.

- What's next (Slice 5.1a implementation, same branch):
  1. Reconcile DiagnosticReport: promote `-lab` to a declared constant beside the
     other 31, teach the checker to accept both `-lab` and `-note`, add the
     missing golden fixture, fix `docs/planning/FHIR-PROFILES.md:28`.
  2. Close the fail-open mode: `--mode US-Core` currently prints "FHIR validation
     passed" and exits 0 on a non-conformant resource.
  3. Choose and implement the profile-version policy so a `|9.0.0`-pinned
     canonical does not fail the byte-exact presence check.
  4. Reconcile Patient (`Patient.MRN` is dropped → hard `Patient.identifier is
     required`), dedupe the LOINC coding `MapLabResult` emits.
  5. The primary kill-test over all 26 `Map*` entry points, with the
     DiagnosticReport negative control behind a build tag.
  6. Publish honest coverage numbers (0 of 31 pinned, one dead constant, 6 of 24
     with required-element checks, three types with neither check), scope
     `internal/fhir/subscription/`, repair the stale citations.

- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` — Lane S5-E, corrections 40-52,
    the day-1 gate table, coordinator ruling 3
  - [S2] `.loom/28-spec-fhir-ig-bulk-smart.md:153-160` — the replacement
    kill-test, and the new "has been RUN" section
  - [S3] `pkg/fhir/mapper.go:435`; `pkg/fhir/validate.go:171,179-237,218-219`;
    `pkg/fhir/types.go:13,15-56,47`; `pkg/fhir/validate_golden_test.go:17`
  - [S4] `internal/integration/delivery/dispatcher.go:146,162,166,312-367`;
    `delivery/store.go:107,128`; `internal/integration/destination/transport.go:325`;
    `destination/revision.go:57,61`; `pkg/integration/contracts.go:602`
  - [S5] `.loom/40-decisions.md` — the 2026-08-08 entry, amended in place
  - [S6] `.gitlab-ci.yml:2672` (`test:destination-transport`, arity-2 existence
    guard) — why gate 2 is untagged
