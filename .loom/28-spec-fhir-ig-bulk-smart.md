# 28 - Spec: FHIR IG, Bulk Data, and SMART Scoping

**Status**: Scoping rewritten against `origin/main` @ `55412bda` (2026-08-08).
**Blocked on Slice 4.1c-b** — see "Blocking dependency" below. Docs-only
preparation is done; no code slice may start until the blocker clears.
**Lane**: F - Product expansion speclets, promoted to Phase 5 (Slices 5.1 and 5.2)
**Tracking**: the authority is `.loom/30-implementation-plan-integration-engine-ide-completion.md`
Slice 5.1 (`:803-807`) and Slice 5.2 (`:809-815`), which the release-gate map
makes 1.0-blocking (`:856`). GitLab `libs/fi-fhir#12` ("P3: Additional FHIR IG
Support (USCDI v3, Bulk Data, SMART App Launch)") is the **legacy P3 backlog
entry** for this work: it is still open, but its USCDI v3 framing and its P3
priority are both superseded by the plan above. Retitle or close it when a lane
picks this up; do not treat its description as scope.

## Blocking dependency (read before anything else)

**The durable integration engine produces no FHIR resource, so there is nothing
for a conformance gate to gate.** `pkg/fhir` has exactly two non-test importers:
`internal/workflow/actions.go:23` (the **legacy** workflow engine — the mapper is
constructed at `:680` inside `fhirAction`, registered only at
`internal/workflow/engine.go:127`) and `cmd/fi-fhir/main.go:49` (the `fi-fhir fhir
validate` CLI, `:309`).

The durable processor's only contact with `internal/workflow` is a planner
(`internal/integration/processor/workflow_plan.go:41,45`) whose contract is
"never invokes transforms or actions" (`internal/workflow/plan.go:144`). A
`fhir`-typed action *is* accepted by the published DSL
(`internal/workflow/published_yaml.go:801`) and *is* treated as a delivery action
(`workflow_plan.go:147-153`), but it produces a `DeliveryResult`
(`workflow_plan.go:112-119`) and then a broker command carrying the **canonical
event** (`internal/integration/delivery/dispatcher.go:246-259`, whose
`item.EventPayload` is `integration_canonical_events.payload_json` —
`internal/integration/delivery/store.go:107,128`). No mapper is called.

Therefore Slice 5.1's real prerequisite is **Slice 4.1c-b** — the first durable
destination consumer, which is what defines a FHIR destination class. Golden
journey 1 ends "inspect trace and FHIR delivery"
(`.loom/20-product-spec-integration-engine-ide-completion.md:293`) and journey 6
requires validating a US Core 9.0.0 R4 payload (`:302-304`); neither has a
producing path today. Pinning packages and integrating a validator before 4.1c-b
would certify a code path no golden journey executes.

## Goal

Split the broad "advanced interoperability standards" product goal into a
proof-first scope for FHIR IG conformance (Slice 5.1), Bulk Data export/import,
and SMART App Launch support (Slice 5.2), sequenced behind a durable FHIR
destination.

The next agent must **not** open with a mapper survey. The coverage matrix that
would have justified one is already published at
`docs/planning/FHIR-CONFORMANCE-MATRIX.md`; read it first. Its findings that
change this spec's shape:

- The shipped validator is a **required-field presence check plus a profile-URL
  string-membership check** (`pkg/fhir/validate.go:104-151,153-177`). There is no
  `StructureDefinition` evaluation, no terminology binding evaluation, no
  primitive-type validation, and no pinned IG package anywhere in the tree.
- `USCoreBaseURL` (`pkg/fhir/types.go:13`) is unversioned and matching is exact
  string equality (`validate.go:171`), so a version-pinned canonical such as
  `…/us-core-patient|9.0.0` fails the presence check today.
- The mapper produces 24 distinct resource types; 21 are profile-checked and 6
  are required-element-checked. One of the 21 has an **incomplete accepted-profile
  set**: a lab `DiagnosticReport` is stamped `us-core-diagnosticreport-lab`
  (`pkg/fhir/mapper.go:435`) but only `us-core-diagnosticreport-note` is accepted
  (`validate.go:218-219`), so the shipped CLI rejects the shipped mapper's own
  output by default.

## Non-Goals

- Do not start any 5.1 or 5.2 code before Slice 4.1c-b merges and defines what a
  FHIR destination is.
- Do not replace the R4/US Core mapper. Its resource coverage is broad; its
  *verification* is the gap.
- Do not attempt full certification-grade IG conformance in one slice.
- Do not build a SMART app UI; scope server-side launch, authorization, and
  context contracts only.
- Do not implement a production job runner for Bulk Data before storage,
  authorization, and NDJSON manifest contracts are agreed.
- Do not make CDA section expansion depend on this work; consume canonical events
  only after they exist.
- Do not add a Java runtime to CI or to the shipped image without ratifying the
  decision recorded in `.loom/40-decisions.md` ("2026-08-08: FHIR conformance
  validation strategy", proposed).

## Acceptance Criteria

**Preparation (done — this document, the matrix, and the proposed decision):**

- A standards/version matrix exists for the IGs in scope and is written against
  code with `file:line` citations: `docs/planning/FHIR-CONFORMANCE-MATRIX.md`.
- The IG versions named match the 1.0 targets: FHIR R4 **4.0.1**, US Core
  **9.0.0**, SMART App Launch **2.2.0**, Bulk Data **3.0.0**
  (`.loom/20-product-spec-integration-engine-ide-completion.md:262`;
  `docs/operations/SUPPORTED-1.0.md:26`). "USCDI v3" is retired from this spec:
  it corresponds to US Core 6.1.0, which is what
  `pkg/fhir/types.go:2` was written against and three annual releases behind the
  target — US Core 9.0.0 meets USCDI v6.
- A dated validator-integration decision exists with rejected alternatives and
  supply-chain consequences.

**Slice 5.1 (blocked on 4.1c-b):**

- A FHIR R4 destination class exists on the durable path and one golden journey
  produces a resource through it. Without this, 5.1 does not start.
- `hl7.fhir.r4.core#4.0.1` and `hl7.fhir.us.core#9.0.0` are pinned as vendored
  artifacts with recorded digests, resolved offline in CI.
- The chosen validation engine runs in a blocking CI job following the
  isolated-proof pattern already used by `test:phi-audit` and
  `test:observability-replicas` (`.gitlab-ci.yml:1397-1444,1455-1490`): dedicated
  services, a `make` target, and an `-list | rg -x | awk` existence guard so the
  job cannot silently run zero tests.
- The profile-version assertion policy is decided and the mapper and the checker
  agree with it (see matrix §5.5).
- The two self-inconsistencies in matrix §1.1 (missing
  `us-core-diagnosticreport-lab`; version-suffix mismatch) are fixed with
  regression tests.
- Conformance policy is surfaced in artifacts and diagnostics — **specifiable
  only after 4.1c-b**, because the destination revision is where the policy
  attaches.

**Slice 5.2 (blocked on 5.1):**

- Bulk Data scope defines supported levels, minimum resource types, NDJSON output
  layout, job lifecycle states, download URL/security model, and storage backend
  assumptions.
- SMART scope defines supported launch mode, OAuth2/OIDC responsibilities, token
  validation boundaries, context parameters, and explicit non-support cases.
- Validation strategy is documented: structural validation, the ratified
  validator path, golden NDJSON fixtures, and API contract tests.
- Work splits into independently shippable tasks if Bulk and SMART are too large
  for one implementation lane.

## Kill-Test

The original kill-test ("run a sample export fixture through the existing FHIR
mapper plus validator path") was **run during this rewrite and it failed**, in a
way that invalidates the premise it was written to test. Recorded here so it is
not run again as if the answer were unknown:

1. Built the CLI from `origin/main` @ `55412bda` and validated a
   `DiagnosticReport` declaring the exact profile the repo's own mapper stamps
   (`pkg/fhir/mapper.go:435`). Result: **WARNING** `meta.profile does not include
   an expected profile for DiagnosticReport`, non-zero exit, because `--strict`
   is the default (`cmd/fi-fhir/main.go:254-255,349`).
2. Validated a `Patient` asserting `…/us-core-patient|9.0.0`. Result: **WARNING**,
   same class — exact string equality against an unversioned constant.
3. Validated a `Patient` with `"gender":"purple"` and `"birthDate":"not-a-date"`.
   Result: **passes clean** — there are no bindings and no primitive-type checks.

So the "narrow mapper gap" the old kill-test expected to find is not narrow and
is not in the mapper: it is that the checker cannot express profile conformance
at all. **Replacement kill-test for 5.1, to run before any conformance code:**

> With Slice 4.1c-b merged, drive golden journey 1 end to end and capture the
> resource the durable path actually delivers. Run it through the candidate
> validation engine against the pinned `hl7.fhir.us.core#9.0.0` package. If no
> resource is captured — because the destination consumer delivers a canonical
> event rather than a FHIR resource — **5.1 is still blocked and the blocker is
> 4.1c-b's scope, not the validator.** Say so and stop.

A negative control is required: the same run against a deliberately
non-conformant resource must fail, or the gate is proving nothing.

## Dependencies

- **Slice 4.1c-b (Lane S4-A of `.loom/32-sprint4-execution-specs.md`)** — hard
  blocker; defines the FHIR destination class.
- `docs/planning/FHIR-CONFORMANCE-MATRIX.md` — the coverage matrix; read first.
- `.loom/40-decisions.md`, "2026-08-08: FHIR conformance validation strategy
  (proposed)" — must be ratified before any CI-image change.
- `pkg/fhir/validate.go` and `pkg/fhir/types.go` — the checker and the profile
  constants. `pkg/fhir/mapper.go` is **not** the engine's FHIR path; it is the
  legacy engine's and the CLI's. Treat it as a resource-construction library that
  currently has no durable caller.
- `docs/planning/FHIR-PROFILES.md` — design notes, still written against US Core
  6.1.0 (`:80,445,542`) and carrying one claim the matrix disproves (`:485`,
  "Meta.Profile set on all resources"). Correct it in the slice that touches it.
- Storage provider speclet (`.loom/26-spec-storage-provider-tests.md`) for Bulk
  Data artifact storage and presigned/download URL assumptions.
- Source Profile and terminology governance work only when their canonical
  outputs or mappings affect FHIR resource content.
- External standards version re-verification at implementation start. US Core is
  on an annual release cadence and 9.0.0 was published 2026-05-31, so the pinned
  version must be re-checked against the package registry rather than against
  this document.

## Sources

- `docs/planning/FHIR-CONFORMANCE-MATRIX.md` — full `file:line` evidence for every
  code claim above.
- `pkg/fhir/validate.go:104-151,153-177,171,179-237,218-219,239-249`;
  `pkg/fhir/types.go:2,13,15-56`; `pkg/fhir/mapper.go:435,1298-1313,1930-1935`
- `internal/workflow/actions.go:23,670,680`; `engine.go:127`; `plan.go:144`;
  `published_yaml.go:801`
- `internal/integration/processor/workflow_plan.go:41,45,112-119,147-153`;
  `internal/integration/delivery/dispatcher.go:246-259`; `store.go:107,128`
- `cmd/fi-fhir/main.go:49,254-255,309,349`
- `.loom/20-product-spec-integration-engine-ide-completion.md:262,293,302-304,363-364`
- `.loom/30-implementation-plan-integration-engine-ide-completion.md:803-807,809-815,856`
- `.loom/32-sprint4-execution-specs.md` corrections 35-38, Lane S4-D
- `docs/operations/SUPPORTED-1.0.md:26,62`
- `.gitlab-ci.yml:1397-1444,1455-1490` — the isolated-proof job pattern 5.1 must copy
- US Core 9.0.0 (STU 9), `hl7.fhir.us.core#9.0.0`, FHIR 4.0.1, published
  2026-05-31, meets USCDI v6 — https://hl7.org/fhir/us/core/STU9/

## Assignment Note

This speclet is **not** independently pickable today. An agent that wants to move
it forward has exactly two options: work Slice 4.1c-b (Lane S4-A), or ratify the
validator decision in `.loom/40-decisions.md` and turn it into a CI-image slice.
Anything else builds conformance machinery for a path that produces nothing.
