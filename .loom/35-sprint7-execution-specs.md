# Sprint 7 — Execution Specs (2026-09-24)

> Coordinator: Claude (Fable 5.1). Lanes: Opus 5.5 agents in isolated
> worktrees. Board at launch: **0 open MRs**, main `1465aa516`, pipeline
> 28183 green. Sprint 6 is fully delivered (MRs !202, !204–!211).
> Predecessor: `.loom/34-sprint6-execution-specs.md`.

Sprint 7 takes the three items left in `ROADMAP.md` "Now" after the 2026-09-20
reconciliation and turns two of them into code. The third is still gated on an
operator action that no agent may take.

| Item | Sprint 7 disposition |
|---|---|
| **Slice 5.1c official validation** | Split in two: **S7-A** closes the seven recorded US Core cardinality gaps in the mapper; **S7-B** runs the HL7 `validator_cli.jar` as a CI-only blocking job with an exact-equality findings ledger. |
| **Slice 4.2c FHIR operator trace** | **S7-C** projects `integration_destination_deliveries` into `OperatorDeliveryAttempt` and renders it in the operator UI. |
| **S6-B budgets 1–3 certification** | **Still blocked.** `GET /projects/19/variables/FI_FHIR_PERF_RUNNER` → 404 on 2026-09-24. Runner 8 `fi-fhir-perf` is online. The spec (`.loom/34` "Decisions Required" item 2) rules this an operator action; the lane launches the day the variable exists. |

---

## Load-bearing facts at launch (all read at `1465aa516`)

- The structural ledger `recordedCardinalityGaps()` (`pkg/fhir/structural_gaps_test.go:67`)
  holds **7 violations across 5 fixtures**: `careteam.json` (participant),
  `coverage.json` (relationship), `documentreference.json` (content is JSON
  `null` — violates base R4, not only US Core), `encounter.json`
  (identifier.system, type), `vitalsign.json` (effective[x]). MR !211's
  medication fix already shrank it from 9. The 5.1b decision
  (`.loom/decisions/2026-09-08-record-the-mapper-s-us-core-cardinality.md`)
  named closing these "the next mapper slice" and explicitly deferred the
  `DocumentReference`-without-attachment question as a **product decision**.
- The fixtures under `testdata/fhir/mapper/` are **the mapper's own output**
  (`pkg/fhir/golden_mapper_test.go:26` regenerates them). Fixing a gap means
  changing the mapper, regenerating, and shrinking the ledger in the same
  commit — `test:fhir-structural` fails on a fixed-but-still-recorded gap by
  design.
- `ci/test-fhir-structural.yml` and `Makefile:1091` guard the assertion count
  at exactly **10** `TestFHIRStructural*` names. Adding one means changing
  both counts; naming a new test outside the prefix avoids it.
- The validator decision
  (`.loom/decisions/2026-08-08-fhir-conformance-validation-strategy-for-slice-5.md`,
  amended 2026-08-09) is in force: `validator_cli.jar` **CI-only**, never in
  the distroless image; the validator **must never reach `packages.fhir.org`**
  during a pipeline; the jar is **not vendored** — pulled in the job from a
  pinned source; the pinned IG `.tgz` archives live under
  `testdata/fhir/packages/` where `security:trivy` scans them.
- `OperatorDeliveryAttempt` (`internal/api/graphql/schema.graphql:2606-2628`)
  exposes **nothing** from `integration_destination_deliveries` (Sprint 6
  correction 10). The ledger has three readers today, all under
  `internal/integration/destination` and its tests; `internal/integration/operator`
  does not import it. The ledger is **clinical-content-free by construction**
  (0003's header) — `fhir_outcome_codes_advisory` carries issue codes only.
- `lint:gqlgen` runs `gqlgen generate` and `git diff --exit-code` on any MR
  touching `internal/api/graphql/**` or `ui/**`; cold-cache it takes 16–24
  minutes and looks hung. It is blocking. Do not cancel-retry it.
- Public-host GitLab GETs return **403 from this LAN today**; every API call
  and every push goes through the ingress IP (recipe below).

---

## Parallelization Map

| Lane | Slice | Owns | Collides with | Merge position |
|---|---|---|---|---|
| **S7-A** | 5.1c-α close the cardinality gaps | `pkg/fhir/**` (mapper, clinical mappers, `structural_gaps_test.go`), `testdata/fhir/mapper/**`, `ci/test-fhir-structural.yml` comment block, `FHIR-CONFORMANCE-MATRIX.md` §5.1 result table, `docs/user-guide/fhir-output.md` if it lists gaps | S7-B on the matrix (different sections) | **before S7-B** |
| **S7-B** | 5.1c-β official validator in CI | new `ci/test-fhir-official*.yml`, ONE include line appended after `ci/test-event-backends.yml`, ONE `.PHONY` line appended after line 37, Makefile targets, `scripts/fhir-official-validate.sh`, `testdata/fhir/official/**` ledger, `testdata/fhir/packages/**` if transitive IG deps must be pinned, `ci/job-inventory.txt` (via script), `FHIR-CONFORMANCE-MATRIX.md` §5 row 2 + new §5.2, `SUPPORTED-1.0.md` standards row (line 26) | S7-A on the matrix; S7-A's fixture fixes change S7-B's ledger | **after S7-A** — rebase, re-run, regenerate ledger |
| **S7-C** | 4.2c operator FHIR trace | `internal/api/graphql/schema.graphql` (add fields only), generated code, `resolvers/operator_control_plane.go`, `internal/integration/operator/**`, one new read method in `internal/integration/destination/postgres.go`, `ui/src/lib/features/operator/**`, `ui/src/lib/gen/graphql.ts`, `docs/operations/DESTINATION-IDENTITY.md` operator paragraph | nobody | whenever |
| **Coordinator** | spec, close-out | this file, `ROADMAP.md`, `.loom/30` 5.1 block, `CHANGELOG.md` `[Unreleased]` (written ONCE at close), memory | — | first and last |

Nobody but the coordinator edits `ROADMAP.md`, `.loom/30-*.md`, or
`CHANGELOG.md`. Worklog and decisions are one file per entry
(`make worklog-new TITLE=…`, `make decisions-new TITLE=…`); the pointer pages
`.loom/50-worklog.md` and `.loom/40-decisions.md` are CI-enforced read-only.

---

## Lane S7-A — Close the mapper's US Core cardinality gaps (Slice 5.1c-α)

**Outcome.** `recordedCardinalityGaps()` returns an empty map; all 25 mapper
fixtures validate clean under `make fhir-structural`; the negative control
still turns red on exactly `patient.json`; `make fhir-destination`,
`make fhir-conformance`, and the `fhirout` unit tests stay green.

**Per-gap direction** (the lane decides the codes, records every choice in
one decision entry):

1. `CareTeam.participant` 1..* — project the event's care-team members; if the
   producing event type carries none, the mapper must not emit a `CareTeam`
   for it (an empty CareTeam is not a US Core CareTeam).
2. `Coverage.relationship` 1..1 — from the insured-relationship field
   (IN1-17 / IN2 family) mapped to the `subscriber-relationship` code system;
   `self` when the insured is the patient.
3. `DocumentReference.content` — JSON `null` is a serialisation defect first
   (`omitempty` or an explicit slice), and a product decision second: with no
   attachment the mapper either omits the resource or attaches the source
   message (`contentType` `x-application/hl7-v2+er7`, content-addressed).
   Choose, justify, record.
4. `Encounter.identifier.system` 1..1 — the deployment-owned
   `urn:fi-fhir:source:<source_id>` scheme ruled on 2026-09-08
   (`.loom/decisions/2026-09-08-source-assigned-identifiers-are-keyed-under-a.md`)
   applies to every source-assigned identifier, not only Patient.
5. `Encounter.type` 1..* — derive from PV1-2 patient class through the
   `us-core-encounter-type` extensible binding (SNOMED CT encounter concepts);
   the code choice is the decision entry's business.
6. `Observation.effective[x]` 1..1 on vital signs — OBX-14, then OBR-7, then
   MSH-7; record the fallback order.

**Day-1 gate.** A new test outside the `TestFHIRStructural` prefix
(e.g. `TestUSCoreMapper_FixturesCarryNoCardinalityViolations`) that walks
every fixture through `pkg/fhir/structural.go` and asserts zero violations.
It must **fail on unmodified main naming exactly the seven**, and pass at
ship. Do not change the 10-count guards.

**Not in scope.** Terminology bindings, invariants, slicing — S7-B measures
those. `SUPPORTED-1.0.md` — S7-B owns it. Any change to `fhirout`.

---

## Lane S7-B — The official validator as a CI-only gate (Slice 5.1c-β)

**Outcome.** A blocking job runs HL7 `validator_cli.jar` **offline** over
(a) all 25 mapper fixtures and (b) the transaction Bundles the durable
`fhir` transport actually delivers, against R4 4.0.1 with
`hl7.fhir.us.core#9.0.0`, and compares the findings to a checked-in ledger by
**exact equality** — same shape as `recordedCardinalityGaps()`: a fixed
finding and a new finding both fail the build; the ledger can only shrink
deliberately. A negative control proves the gate is live.

**Riskiest assumption (kill-test on day 1, before any product code).** The
validator can run with **no network** given only the two pinned `.tgz`
archives and `-tx n/a`. Run it in a `--network none` container on
`docker --context 7900xtx` against `patient.json`. If it demands US Core's
transitive dependencies (`hl7.terminology.r4`, `us.nlm.vsac`,
`hl7.fhir.uv.sdc`, `hl7.fhir.uv.extensions`, `hl7.fhir.uv.smart-app-launch`,
`hl7.fhir.uv.bulkdata`, `us.cdc.phinvads`), pin exactly the ones it demands
under `testdata/fhir/packages/` with sha256 sums reproduced against the
registry `dist.shasum`, the 5.1b way — and record the size and scan outcome.
If one is licence-gated, say so in the decision and record what the validator
reports without it. Write the result to the status file before proceeding.

**Delivered bundles.** Capture what `internal/integration/fhirout` produces
for every supported event type (the `Project`/`CreateConditionalTransactionBundle`
path) via an opt-in env var in the existing unit tests; no PostgreSQL. A two-job
shape — a Go job that emits the bundles as artifacts, a JRE job that validates
them with `needs:` — keeps Java out of the Go image. Both jobs go through
`scripts/ci-job-inventory.sh --write`.

**Tool provenance.** Pin the validator by version **and** sha256 in
`scripts/fhir-official-validate.sh`; pin the JRE image the way the repo pins
every other image (`${DOCKERHUB_CACHE_PREFIX}/…:<tag from a variable>`). The
jar never enters the checkout.

**Docs.** Matrix §5 row 2 → ratified in full; new §5.2 "The official
validator (Option A) — Slice 5.1c" listing what it now proves and what the
ledger still records; `SUPPORTED-1.0.md:26` "official validator or
conformance-suite evidence is not yet complete" → the true sentence. Decision
entry. Worklog entry.

**Sequencing.** S7-A merges first. When it does: rebase, re-run, regenerate
the ledger, re-arm. If S7-A has not merged within 6 hours of S7-B being
otherwise ready, open S7-B against main with today's ledger and note the
pending shrink in the MR description.

---

## Lane S7-C — The trace shows the FHIR delivery (Slice 4.2c)

**Outcome.** `OperatorDeliveryAttempt` gains
`deliveries: [OperatorDestinationDelivery!]!` — every provenance-ledger row
for that attempt, tenant-scoped, newest first, bounded — with `transport`,
`destination{artifactId,revisionId,class}`, `digestVerified`, `outcome`,
`failureCode`, `httpStatusClass`, `endpointAdvisory`,
`servedCertificateSubjectAdvisory`, `completedAt`, and the 4.1c-c facts
`fhirResourceTypes: [String!]!`, `fhirEntryCount: Int!`,
`fhirOutcomeCodesAdvisory: [String!]!`. The operator UI's trace and delivery
console render it: transport badge, outcome, resource-type chips, entry count,
outcome codes, endpoint. Nothing else from the ledger, nothing from the
payload — the ledger is clinical-content-free and the UI keeps it that way.

**Seams.** `internal/integration/destination/postgres.go` gains one read
(`ListDeliveriesForAttempt(ctx, tenant, attemptID, limit)`);
`internal/integration/operator` composes it into `DeliveryAttemptSummary`
(new `Deliveries []DestinationDeliverySummary`) for `GetAttempt`,
`ListAttempts`, and `GetMessageTrace`; `projectOperatorAttempt`
(`resolvers/operator_control_plane.go:223`) projects it. Then
`cd internal/api/graphql && go run github.com/99designs/gqlgen generate --config gqlgen.yml`
and commit the generated diff. UI: `operatorApi.ts` query, `npm run
codegen:graphql`, a pure presentation helper in `attemptPresentation.ts` with
tests, and the panels in `MessageTrace.svelte` / `DeliveryConsole.svelte`.

**Day-1 gate.** A resolver-level test that, on unmodified main, proves the
projection of a `fhir` delivery exposes no ledger field (passes today), then
is inverted at ship to assert `deliveries[0].transport == "fhir"`,
`fhirResourceTypes == [Patient, Encounter]`, `fhirEntryCount == 2`, alongside
an `https` row with empty FHIR facts. Use the existing destination/delivery
test fixtures.

**Guards.** No migration; no root-field additions, so `make transport-gate`
must stay green unchanged; `graphql:operator` remains the role. Adding fields
to existing types is additive for the SDK — run the SDK codegen check if the
repo has one.

---

## Shared lane policy (every lane, verbatim in its prompt)

- Branch from a fresh `origin/main`; branch names
  `feat/sprint7-slice-5-1c-a-mapper-gaps`,
  `feat/sprint7-slice-5-1c-b-official-validator`,
  `feat/sprint7-slice-4-2c-operator-fhir-trace`.
- **Git and API from this LAN**: probe `nc -z -G 3 192.168.50.227 443`; push
  with `git -c http.sslVerify=false -c http.extraHeader="Host: gitlab.flexinfer.ai" push https://oauth2:${GITLAB_PAT}@192.168.50.227/libs/fi-fhir.git <branch>`;
  API with `curl --resolve gitlab.flexinfer.ai:443:192.168.50.227 -H "PRIVATE-TOKEN: $GITLAB_PAT"`;
  print `http=%{http_code}` on every write and treat `000` as failure. Never
  use the `gitlab` MCP tools. Never print the token.
- **MR**: `POST /projects/19/merge_requests` (`remove_source_branch=true`,
  `squash=false`); arm with `PUT /merge_requests/:iid/merge`
  `merge_when_pipeline_succeeds=true` only after the **new** head pipeline id
  exists (a 405 means the old one — wait, re-PUT). Poll no more often than
  every 180 s.
- **Retry budget**: read the trace first. Retry a job **once** only for a
  known flake (`test:mllp-runtime` frame timeout,
  `test:observability-replicas` wall-clock, `test:integration` budget under
  load, `security:govulncheck` OOM, `security:trivy-image` daily-DB drift).
  Anything else is yours to fix. After two failures of *different* jobs, or a
  pipeline older than 3 hours, STOP and write status.
- **Never touch** `CHANGELOG.md`, `ROADMAP.md`, `.loom/30-*.md`,
  `.loom/50-worklog.md`, `.loom/40-decisions.md`, or another lane's files.
- **Go env**: `.tmp/go-mod-cache/` is committed — never delete it or point
  `GOMODCACHE` at it. No local Docker Desktop: PostgreSQL via
  `docker --context 7900xtx run -d -p <port>:5432 postgres:16`, connect to
  `cblevins-7900xtx:<port>`. Run `gofmt -l`, `golangci-lint run ./...`
  (installed locally), and the slice's `make` targets before pushing.
- **UI env** (S7-C): symlink `ui/node_modules` from the canonical checkout
  `/Users/cblevins/workspace/libs/fi-fhir/ui/node_modules`, `cd ui && npx svelte-kit sync`,
  then `npx vitest run`, `npm run check`, `npm run lint`.
- **zsh**: unquoted `$VAR` does not word-split; `noclobber` is on (`>|`);
  `mv`/`rm`/`cp` are interactive (`command mv -f`); never `echo` JSON into
  `jq` (`printf '%s\n'`).
- **Status file**: write progress, decisions, and any STOP to the path named
  in your prompt; the coordinator reads that file, not your transcript.
- Commit messages conventional, scoped, with the `Co-Authored-By` trailer.

---

## Riskiest assumption of the sprint

That the HL7 validator can be made **hermetic** with a pinnable set of
archives. If it cannot — if a required dependency is licence-gated or the
validator insists on the registry — S7-B's deliverable changes from "official
conformance evidence" to "the exact, recorded reason official evidence cannot
be produced offline, and the terminology-less findings we can produce". That
is still a truthful `SUPPORTED-1.0.md` row and still worth the job. The
day-1 kill-test decides which sprint we are in before any product code is
written.
