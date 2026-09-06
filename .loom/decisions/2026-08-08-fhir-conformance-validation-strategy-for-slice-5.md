### 2026-08-08: FHIR conformance validation strategy for Slice 5.1 (AMENDED AND RATIFIED IN PART — 2026-08-09, Sprint 5 Lane S5-E)

**Status: split ruling, in force as amended.** This entry was recorded by Sprint
4 Lane S4-D as a recommendation and carried "not ratified; requires human or
next-sprint ratification before any lane acts on it". Sprint 5's coordinator
ruled on it on 2026-08-09 (`.loom/33-sprint5-execution-specs.md`, Decisions
Required, ruling 3). The ruling is recorded in the **2026-08-09 amendment**
immediately below, which is part of this entry and takes precedence over the
proposed text where the two differ. The proposed text is kept verbatim because
its evidence is still the evidence.

#### 2026-08-09 amendment (Sprint 5, Lane S5-E, Slice 5.1a)

**1. The confinement half is RATIFIED, unconditionally.** `validator_cli.jar` is
CI-only and never enters the shipped image; the shipped image stays
`gcr.io/distroless/static-debian12:nonroot` (`Dockerfile:27,58` — no shell, no
package manager, no JRE); IG packages are vendored as pinned, digest-recorded,
offline `.tgz` artifacts and are placed deliberately with respect to the
`security:trivy` skip list. The premise holds on inspection: `security:trivy-image`
blocks on CRITICAL and HIGH-fixed with no `allow_failure` on MR, default branch,
and tags, and this repo's trivy database moves daily, so a green `main` does not
imply a green MR. A JRE in the shipped image would put a continuously moving CVE
surface behind a blocking gate.

**2. The ordering half is AMENDED.** The proposed order was "Option C now, Option
A later". The corrected order is **reconcile first (5.1a), then Option C, then
Option A.** The repository's actual defects are not that the checker is
structurally shallow. They are that the checker and the mapper *disagree*, that
the checker *fails open* on any mode string that is not exactly `us-core`, and
that CI has *no fixture* for the one input the mapper actually produces. A larger
validator built over a mapper it disagrees with certifies the disagreement at
higher resolution. This amendment resolves the open item further down this entry
— "A profile-version assertion policy must be chosen … and the mapper and checker
must agree. Today they cannot" — by making that reconciliation the next slice
rather than a precondition nobody owns.

**3. Slice 5.1's real prerequisite is a slice that does not exist: 4.1c-c, a
FHIR destination class.** This entry's own closing line reads "Slice 5.1 remains
blocked on Slice 4.1c-b regardless of which engine wins." 4.1c-b merged at
`e77c6218b`, which satisfied that condition formally and not substantively. The
`.loom/28-spec-fhir-ig-bulk-smart.md:206-212` kill-test was written for exactly
this moment and has now been **run**, not argued —
`TestFHIRConformance_DurableEngineProducesNoFHIRResource`
(`internal/integration/delivery/fhir_conformance_gate_test.go`) **passes on
unmodified `main`**: the delivered body is the Kafka delivery-command envelope
(`integration.delivery.v1`) carrying `integration_canonical_events.payload_json`
(`delivery/dispatcher.go:162,166,348`; `delivery/store.go:107,128`), the content
type is `application/json` and not `application/fhir+json`
(`destination/transport.go:325`), the transport vocabulary is exactly
`{kafka, https}` with no FHIR class (`destination/revision.go:57,61`),
`DestinationClass` is `production|sandbox` — an environment class
(`pkg/integration/contracts.go:602`) — and **zero** files under
`internal/integration/**` import `pkg/fhir`. Per `.loom/28`'s own instruction:
5.1 is still blocked, and the blocker is 4.1c-b's scope, not the validator.

4.1c-c is therefore named here as the real prerequisite: a destination class or
`TransportKind` meaning "this destination receives FHIR R4 resources", a
canonical-event→resource mapping step on the delivery path,
`application/fhir+json`, and a decision on whether the producing mapper is
`pkg/fhir` (reachable today from exactly two non-test files — `cmd/fi-fhir/main.go`
and `internal/workflow/actions.go` — neither of them the durable engine) or
something new. It is a Phase 4 delivery slice, not a Phase 5 standards slice, and
it is not in Sprint 5's scope.

**4. Slice 5.1a is what the FHIR lane ships instead**, and it needs no Java, no
IG package, no new `go.mod` dependency, and no image change. Its day-1 gate
`TestFHIRConformance_ValidatorRejectsMapperOutputToday`
(`pkg/fhir/conformance_day1_gate_test.go`, behind the `fhirday1gate` tag,
reproduced by `make fhir-conformance-day1-gate`) **fails on unmodified `main`**
with `warning value: meta.profile does not include an expected profile for
DiagnosticReport`, while the repository's own `-note` fixture validates clean in
the same run. The shipped validator rejects the shipped mapper's own output.

- Decision (proposed 2026-08-08; superseded on ordering by the amendment above):
  - **Adopt Option C now and Option A later, in that order, and never Option A
    inside the shipped image.**
  - **Now (Sprint 4, zero code):** keep the existing presence check as the only
    validation the product claims, and say so plainly. The claim already exists
    in the right place — `docs/operations/SUPPORTED-1.0.md:26` lists US Core
    9.0.0 as a *release target* and states that "official validator or
    conformance-suite evidence is not yet complete." Nothing needs to be
    weakened; the honest statement is already shipped. What is missing is the
    positive description of what the checker *is*, which is now published at
    `docs/planning/FHIR-CONFORMANCE-MATRIX.md`.
  - **Slice 5.1, after 4.1c-b:** integrate the HL7 `validator_cli.jar` as a
    **CI-only service**, in its own blocking job following the isolated-proof
    pattern of `test:phi-audit` and `test:observability-replicas`
    (`.gitlab-ci.yml:1397-1444,1455-1490`). The Java runtime exists in the CI
    image for that one job and nowhere else.
  - **The shipped image stays distroless static.** `Dockerfile:27` is
    `gcr.io/distroless/static-debian12:nonroot` running a `CGO_ENABLED=0` static
    binary as `USER nonroot` with no shell and no package manager
    (`Dockerfile:20-24,27,58`). No JRE is added to it under any option. If
    runtime conformance checking is ever required in-process, that is a separate
    decision requiring Option B.
  - **Package pinning is mandatory under every option.** `hl7.fhir.r4.core#4.0.1`
    and `hl7.fhir.us.core#9.0.0` are vendored as `.tgz` artifacts with recorded
    digests and resolved offline. The validator must never reach
    `packages.fhir.org` during a pipeline; a conformance gate that silently
    re-resolves its own IG is not a gate.
  - **Where the vendored artifacts live is part of the decision, not an
    accident.** The blocking `security:trivy` filesystem scan skips `.tmp`,
    `.go`, `.cache`, `ui/.npm`, and `sdk/typescript/.npm`
    (`.gitlab-ci.yml:1592-1595`). Dropping a fat jar into a skipped directory
    would exempt it from the scan by side effect. Vendored IG `.tgz` packages are
    data and should be scanned; a vendored `validator_cli.jar` should not be
    vendored at all — pull it in the job from a digest-pinned image.

- Rationale:
  - **There is nothing to validate yet, so the cheapest correct move is to
    describe reality accurately and wait.** `pkg/fhir` has exactly two non-test
    importers — `internal/workflow/actions.go:23` (legacy engine; mapper
    constructed at `:680`, action registered only at
    `internal/workflow/engine.go:127`) and `cmd/fi-fhir/main.go:49` (the `fhir
    validate` CLI, `:309`). The durable engine's only use of `internal/workflow`
    is a planner that "never invokes transforms or actions"
    (`internal/workflow/plan.go:144`), and its delivery command carries the
    canonical event, not a resource
    (`internal/integration/delivery/dispatcher.go:246-259`;
    `store.go:107,128`). Integrating a validator today would gate a path no
    golden journey executes.
  - **The current checker cannot be incrementally grown into a profile
    validator, and pretending otherwise is the expensive mistake.** It is a
    required-field presence check over 6 resource types plus a profile-URL
    string-membership check over 21 (`pkg/fhir/validate.go:104-151,153-177,
    179-237`). It has no `StructureDefinition`, no snapshot, no terminology
    binding, no primitive-type validation. Demonstrated on `origin/main` @
    `55412bda`: `"gender":"purple"` with `"birthDate":"not-a-date"` passes clean;
    a `Device` with a fabricated profile URL passes clean; a version-pinned
    canonical `…/us-core-patient|9.0.0` *fails*, because matching is exact string
    equality (`:171`) against an unversioned base (`pkg/fhir/types.go:13`). Each
    of those is a separate subsystem to build, and building four of them badly is
    worse than shelling out to the reference implementation once.
  - **`validator_cli.jar` is the reference implementation and the only thing that
    produces evidence the cross-cutting proof matrix will accept.** The plan
    requires "Official validator/conformance test evidence" for the Standards row
    (`.loom/30-implementation-plan-integration-engine-ide-completion.md`,
    cross-cutting proof matrix, `:844`) and warns that "ad-hoc similarly named endpoints
    do not count" (`:815`). A hand-rolled Go checker cannot produce that evidence
    by construction, no matter how good it is.
  - **CI-only confines the Java supply chain to a boundary that already exists.**
    CI already runs several third-party images from a mirrored prefix
    (`DOCKERHUB_CACHE_PREFIX`, `.gitlab-ci.yml:33`), including `aquasec/trivy`
    at `:1588,1670`. One more digest-pinned image in one job is a known,
    bounded cost. The shipped image is a different threat surface with a
    different gate: `security:trivy-image` fails the pipeline on any CRITICAL in
    the built image (`--exit-code 1 --severity CRITICAL`, `:1677-1678`) against a
    vulnerability database that moves daily. Putting a JRE into the artifact
    scanned by that gate means every future JRE CVE becomes an unrelated red
    pipeline on somebody else's MR.
  - **Image size is a real consequence, not a rounding error.** A
    `distroless/static-debian12` base plus a stripped static Go binary is
    single-digit megabytes; a JRE base is measured in the hundreds. Measure it
    before ratifying, but do not adopt Option A-in-image on the assumption it is
    cheap.

- Alternatives considered:
  - **A-in-image. Ship `validator_cli.jar` inside the product image so the engine
    can validate at runtime.** *Rejected.* It abandons distroless static
    (`Dockerfile:27`) for a JRE base, adds a shell and a package manager to an
    image deliberately built without them, grows the image by roughly two orders
    of magnitude, and routes every future JRE CVE through the blocking
    `security:trivy-image` gate (`.gitlab-ci.yml:1677-1678`). No requirement in
    `.loom/20` or `.loom/30` asks for in-process IG validation at runtime; the
    requirement is *evidence*, which CI produces.
  - **B. Write a Go structural validator over pinned `.tgz` IG packages.**
    *Rejected for 5.1; retained as the fallback if Option A is ever refused on
    supply-chain grounds.* It is the only option that could eventually run
    in-process without a JRE, and it keeps the image untouched. But it means
    building, from nothing: `.tgz` package loading, `StructureDefinition`
    snapshot walking, cardinality and slicing evaluation, FHIRPath invariants,
    and terminology binding evaluation with ValueSet expansion. `go.mod` has no
    FHIR dependency of any kind, and `pkg/terminology` is LOINC/UMLS
    mapping machinery, not a FHIR terminology server — there is no ValueSet
    expansion anywhere in the tree. It is a multi-slice subsystem competing with
    a `java -jar` invocation, and it still would not satisfy "official validator"
    evidence.
  - **C-alone. Keep the presence check permanently and document it as the
    ceiling.** *Rejected as an endpoint, adopted as the interim.* US Core 9.0.0
    conformance is a 1.0 gate (`.loom/30-implementation-plan-…:856`,
    `.loom/20-product-spec-…:262,302-304`), so "presence check forever" means
    dropping a 1.0 claim. That is a product decision, not an engineering one, and
    nobody has asked for it.
  - **Hosted/third-party validation service.** *Rejected without detailed
    analysis.* It would send resource content to an external endpoint. Every
    fixture in a conformance run is synthetic today, but the same job would be
    the obvious place to validate a captured golden-journey payload, and the PHI
    posture (`.loom/20-product-spec-…`, `docs/operations/PHI-RETENTION.md`) does
    not permit that egress path to exist by default.

- Consequences (if ratified):
  - CI gains one image and one blocking job at Slice 5.1; the shipped image
    changes in no way.
  - The repository gains vendored, digest-recorded `.tgz` IG packages. Their
    location must be chosen deliberately with respect to the `security:trivy`
    skip list (`.gitlab-ci.yml:1592-1595`), not defaulted into a skipped
    directory.
  - Two self-inconsistencies must be fixed before any migration, because both
    produce false negatives under any engine: a lab `DiagnosticReport` is stamped
    `us-core-diagnosticreport-lab` (`pkg/fhir/mapper.go:435`) while only
    `us-core-diagnosticreport-note` is accepted
    (`pkg/fhir/validate.go:218-219`), and a version-pinned canonical never
    matches an unversioned constant (`:171`, `pkg/fhir/types.go:13`).
  - A profile-version assertion policy must be chosen — bare canonical or
    `|9.0.0` — and the mapper and checker must agree. Today they cannot, because
    the checker has no version concept. *(Owned by Slice 5.1a per the 2026-08-09
    amendment above.)*
  - `docs/planning/FHIR-PROFILES.md` remains stale until the ratifying slice
    updates it: `:80`, `:445`, and `:542` name US Core 6.1.0, and `:485` claims
    "Meta.Profile set on all resources", which
    `pkg/fhir/mapper.go:1298-1313,1930-1935` disproves.
  - Ratification unblocks nothing in Sprint 4. Slice 5.1 remains blocked on
    Slice 4.1c-b regardless of which engine wins. *(Corrected by the 2026-08-09
    amendment: 4.1c-b has merged and 5.1 is still blocked. The blocker is a slice
    that does not exist — 4.1c-c, a FHIR destination class — because 4.1c-b
    delivers a canonical-event command envelope, not a FHIR resource. Proven by
    `TestFHIRConformance_DurableEngineProducesNoFHIRResource`.)*

- Sources:
  - [S1] `.loom/32-sprint4-execution-specs.md` corrections 35-38 and Lane S4-D
  - [S2] `docs/planning/FHIR-CONFORMANCE-MATRIX.md` (full `file:line` evidence)
  - [S3] `pkg/fhir/validate.go:104-151,153-177,171,179-237,218-219,239-249`;
    `pkg/fhir/types.go:2,13`; `pkg/fhir/mapper.go:435,1298-1313,1930-1935`
  - [S4] `internal/workflow/actions.go:23,680`; `engine.go:127`; `plan.go:144`;
    `cmd/fi-fhir/main.go:49,254-255,309,349`
  - [S5] `internal/integration/delivery/dispatcher.go:246-259`; `store.go:107,128`;
    `internal/integration/processor/workflow_plan.go:41,45,112-119,147-153`
  - [S6] `Dockerfile:2-3,20-24,27,58`
  - [S7] `.gitlab-ci.yml:12,14,33,1585-1600,1592-1595,1667-1678,1397-1444,1455-1490`
  - [S8] `docs/operations/SUPPORTED-1.0.md:26,62`; `docs/planning/FHIR-PROFILES.md:80,445,485,542`
  - [S9] `.loom/30-implementation-plan-integration-engine-ide-completion.md:803-807,815,844,856`;
    `.loom/20-product-spec-integration-engine-ide-completion.md:262,293,302-304`
  - [S10] US Core 9.0.0 (STU 9), `hl7.fhir.us.core#9.0.0`, FHIR 4.0.1, published
    2026-05-31, meets USCDI v6 — https://hl7.org/fhir/us/core/STU9/
