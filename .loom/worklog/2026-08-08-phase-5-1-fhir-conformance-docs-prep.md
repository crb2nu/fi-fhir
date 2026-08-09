### 2026-08-08 - Phase 5.1 FHIR conformance docs prep

- What changed:
  - Sprint 4 Lane S4-D, docs-only, zero code. Rewrote
    `.loom/28-spec-fhir-ig-bulk-smart.md` against `origin/main` @ `55412bda`:
    retired "USCDI v3" for US Core 9.0.0, replaced the stale `libs/fi-fhir#12`
    tracking reference with the Phase 5 slices that actually govern the work, and
    replaced the "existing FHIR mapper" framing with the verified fact that
    `pkg/fhir` is reachable only from the legacy engine and the `fhir validate`
    CLI.
  - Published `docs/planning/FHIR-CONFORMANCE-MATRIX.md`: per-resource-type
    coverage of `pkg/fhir/mapper.go` and `pkg/fhir/validate.go`, every claim with
    a `file:line` citation.
  - Recorded a **proposed** (not ratified) validator-integration decision in
    `.loom/40-decisions.md` with rejected alternatives and image/supply-chain
    consequences.
  - Recorded the 5.1 → 4.1c-b sequencing dependency in `.loom/30`'s Slice 5.1
    section.
  - Corrected two planning docs that would otherwise contradict the new matrix:
    a version/accuracy banner on `docs/planning/FHIR-PROFILES.md`, and
    `docs/planning/README.md`'s FB-003 row (was "✅ Shipped" for "FHIR validation
    + conformance checks"; conformance checks are not shipped) plus the P3
    backlog line and the document index.

- Why:
  - `.loom/32-sprint4-execution-specs.md` correction 36: the durable integration
    engine produces no FHIR resource, so 5.1 code would certify a path no golden
    journey executes. Corrections 35 and 37 add that `.loom/28` is stale on the
    IG version and the dependency map, and that "integrate an official validator"
    is a CI-image and supply-chain decision before it is a code slice.

- Evidence:
  - `pkg/fhir` has exactly two non-test importers: `internal/workflow/actions.go:23`
    (legacy engine; `fhir.NewUSCoreMapper()` at `:680`, registered only at
    `internal/workflow/engine.go:127`) and `cmd/fi-fhir/main.go:49` (`:309`).
  - The durable processor reaches `internal/workflow` only through a planner
    whose contract is "never invokes transforms or actions"
    (`internal/workflow/plan.go:144`;
    `internal/integration/processor/workflow_plan.go:41,45`). A `fhir`-typed
    action is accepted (`published_yaml.go:801`; `workflow_plan.go:147-153`) and
    becomes a delivery row (`:112-119`) whose broker command carries the
    canonical event, not a resource
    (`internal/integration/delivery/dispatcher.go:246-259`; `store.go:107,128`).
  - Built the CLI from `origin/main` @ `55412bda` and ran four probes through
    `fi-fhir fhir validate --mode us-core`:
    1. a `DiagnosticReport` declaring `us-core-diagnosticreport-lab` — the exact
       profile the repo's own mapper stamps at `pkg/fhir/mapper.go:435` —
       **warns and exits non-zero**, because only `-note` is accepted
       (`pkg/fhir/validate.go:218-219`) and `--strict` is the default
       (`cmd/fi-fhir/main.go:254-255,349`);
    2. a `Patient` asserting `…/us-core-patient|9.0.0` **warns**, because
       matching is exact string equality (`validate.go:171`) against an
       unversioned base (`pkg/fhir/types.go:13`);
    3. `"gender":"purple"` with `"birthDate":"not-a-date"` **passes clean**;
    4. a `Device` with a fabricated profile URL **passes clean**.
  - Headline counts: 24 resource types produced by the mapper; 21
    profile-presence-checked (one against the wrong profile); 6
    required-element-checked; 3 with no profile check at all. Against US Core
    9.0.0's 55 profiles, `pkg/fhir/types.go:15-56` names 31 (all slugs still
    valid), leaves 24 unnamed, and version-pins none. `USCoreMedicationProfile`
    (`types.go:35`) is referenced nowhere.
  - `lint:docs` inputs pass locally: `scripts/validate-docs.sh` and
    `bash scripts/worklog.sh check`.

- What's next:
  - Ratify or reject the proposed decision in `.loom/40-decisions.md` before any
    CI-image change. Nothing in Sprint 4 depends on the outcome.
  - Slice 5.1 stays blocked on Slice 4.1c-b (Lane S4-A). Its first act, once
    unblocked, is the replacement kill-test in `.loom/28`: capture what the
    durable path actually delivers and validate that, with a negative control.
  - Fix the two self-inconsistencies (missing `us-core-diagnosticreport-lab`
    mapping; version-suffix mismatch) in the 5.1 slice — both produce false
    negatives under any validation engine.
  - `docs/planning/FHIR-PROFILES.md` body still reads US Core 6.1.0; the banner
    flags it, the 5.1 slice corrects it.

- Sources:
  - [S1] `.loom/32-sprint4-execution-specs.md` corrections 35-38, Lane S4-D
  - [S2] `docs/planning/FHIR-CONFORMANCE-MATRIX.md`
  - [S3] `pkg/fhir/validate.go:104-151,153-177,171,179-237,218-219,239-249`;
    `pkg/fhir/types.go:2,13,15-56`;
    `pkg/fhir/mapper.go:435,1298-1313,1930-1935`
  - [S4] `internal/workflow/actions.go:23,680`; `engine.go:127`; `plan.go:144`;
    `published_yaml.go:801`; `cmd/fi-fhir/main.go:49,254-255,309,349`
  - [S5] `internal/integration/processor/workflow_plan.go:41,45,112-119,147-153`;
    `internal/integration/delivery/dispatcher.go:246-259`; `store.go:107,128`
  - [S6] `.loom/20-product-spec-integration-engine-ide-completion.md:262,293,302-304`;
    `.loom/30-implementation-plan-integration-engine-ide-completion.md:803-807,815,844,856`
  - [S7] `Dockerfile:2-3,20-24,27,58`;
    `.gitlab-ci.yml:33,1592-1595,1667-1678,1397-1444,1455-1490`
  - [S8] US Core 9.0.0 (STU 9), `hl7.fhir.us.core#9.0.0`, FHIR 4.0.1, published
    2026-05-31, meets USCDI v6 — https://hl7.org/fhir/us/core/STU9/
