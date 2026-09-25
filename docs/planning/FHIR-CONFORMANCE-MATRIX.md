# FHIR R4 / US Core Conformance Coverage Matrix

**Read at**: `origin/main` @ `55412bda`. Every `file:line` is an `origin/main` line
number.
**Purpose**: state exactly what the shipped FHIR code does and does not check, so
Slice 5.1 ("Pin FHIR R4 4.0.1 and US Core 9.0.0 packages, integrate an official
validator, publish the USCDI/profile coverage matrix" —
`.loom/30-implementation-plan-integration-engine-ide-completion.md:805`) is scoped
against code rather than against the aspiration.
**Companion documents**: `.loom/28-spec-fhir-ig-bulk-smart.md` (scope),
`.loom/40-decisions.md` "2026-08-08: FHIR conformance validation strategy"
(proposed validator decision), `docs/planning/FHIR-PROFILES.md` (design notes,
still written against US Core 6.1.0).

This is a **planning matrix, not a conformance claim**. Per
`docs/operations/SUPPORTED-1.0.md`, US Core 9.0.0 is a release *target* and
"official validator or conformance-suite evidence is not yet complete."

> ### Update — 2026-08-09, Slice 5.1a (conformance reconciliation)
>
> This matrix was written at `55412bda` and is annotated in place rather than
> rewritten, so the findings and the repairs stay legible as a pair. What has
> changed since:
>
> **§0's finding is no longer an argument — it is a test.**
> `TestFHIRConformance_DurableEngineProducesNoFHIRResource`
> (`internal/integration/delivery/fhir_conformance_gate_test.go`) runs the real
> dispatcher against a live TLS `https` destination and asserts the body is the
> `integration.delivery.v1` command envelope carrying the canonical event, at
> `application/json`; that the transport vocabulary is `{kafka, https}` with no
> FHIR class; and that **zero** files under `internal/integration/**` import
> `pkg/fhir`. It passes on `main`. 4.1c-b merged and did not change the answer:
> 5.1's real prerequisite is **4.1c-c, a FHIR destination class**, which does not
> exist. See `.loom/28-spec-fhir-ig-bulk-smart.md` and `.loom/40-decisions.md`
> (2026-08-09).
>
> **2026-09-08: 4.1c-c exists, and that gate is inverted.** `§0` below is now
> history: `internal/integration/fhirout` is the durable engine's one door to
> `pkg/fhir`, and `TestFHIRDestination_DurableEngineDeliversFHIRResource`
> asserts the opposite of each of the four points above. See §5 row 1.
>
> **§1.1 cases 1 and 2 are fixed.** The mapper/checker DiagnosticReport
> disagreement and the version-suffix mismatch are closed; see the annotations on
> those rows. Case 3 (no bindings, no primitive-type checks) and case 4 stand —
> they are Slice 5.1b's subject, not 5.1a's.
>
> **A fifth behaviour that was not in this matrix is also fixed**: the checker
> failed open on mode. Any string that was not byte-exactly `us-core` — including
> `US-Core` and `""` — disabled both the required-element and the
> profile-presence checks, and the CLI passed `--mode` through unvalidated. The
> mode is now a closed, case-insensitive set and anything outside it is an error.
>
> **Every number in §3 and §4 is now asserted by
> `TestFHIRConformance_CheckerCoverageIsDerivableFromCode`**, computed from the
> checker rather than transcribed into prose. A doc claim and the code can no
> longer drift apart silently.
>
> **`cmd/fi-fhir/main.go` line numbers in this document are stale and are
> replaced by symbol references.** They were already wrong before this slice and
> they will be wrong again after it; `pkg/fhir/*` line numbers are exact as of
> `55412bda` and are kept for the historical findings.

---

## 0. The finding that frames the matrix

**No FHIR resource is produced anywhere on the durable integration engine's
path.** The mapper is reachable from the legacy workflow engine and from one CLI
subcommand, and from nothing else.

| Claim | Proof |
|---|---|
| `pkg/fhir` has exactly two non-test importers | `git grep -n "fi-fhir/pkg/fhir" -- '*.go'` → `cmd/fi-fhir/main.go` (the `fhirpkg` import), `internal/workflow/actions.go:23`. Asserted continuously for `internal/integration/**` by `TestFHIRConformance_DurableEngineProducesNoFHIRResource` |
| The only mapper construction site is the legacy `fhir` action | `fhir.NewUSCoreMapper()` at `internal/workflow/actions.go:680`, inside `fhirAction` (`:670`) |
| That action is registered only on the legacy engine | `internal/workflow/engine.go:127` — `e.RegisterAction("fhir", ContextActionHandlerFunc(fhirAction))` |
| The only other importer is the CLI validator | `cmd/fi-fhir/main.go` `runFHIRValidate` — `fhirpkg.ValidateJSON(...)`, reached from `fi-fhir fhir validate` (`runFHIR`) |
| The durable processor touches `internal/workflow` only through a planner | `internal/integration/processor/workflow_plan.go:41,45` — `workflow.ParsePublishedWorkflow`, `workflow.NewPlanner` |
| That planner cannot execute an action, by construction | `internal/workflow/plan.go:144` — "Plan returns a new deterministic plan and never invokes transforms or actions"; `ActionPlan` (`:16-20`) carries `ID`/`Type`/`DestinationArtifactID` and no configuration |
| A `fhir`-typed action *is* accepted by the published DSL | `internal/workflow/published_yaml.go:801`; `internal/integration/processor/workflow_plan.go:147-155` (`deliveryActionTypeV1`) |
| …and becomes a delivery row, not a FHIR resource | `workflow_plan.go:111-119` builds `integration.DeliveryResult`; `postgres_submission.go:375-388` inserts the outbox row |
| …whose broker command carries the **canonical event**, not FHIR | `internal/integration/delivery/dispatcher.go:246-259` marshals `Event: item.EventPayload`; `item.EventPayload` is `integration_canonical_events.payload_json` (`internal/integration/delivery/store.go:107,128`) |

Consequence for planning: a workflow may legally declare `type: fhir` on the
durable engine today. Planning accepts it, a delivery attempt is recorded, and a
Kafka command is published containing the canonical event. `pkg/fhir` is never
called. **Certifying `pkg/fhir` against US Core 9.0.0 would certify a path no
golden journey executes** — golden journey 1 ends "inspect trace and FHIR
delivery" (`.loom/20-product-spec-integration-engine-ide-completion.md:293`) and
journey 6 requires validating a US Core 9.0.0 R4 payload (`:302-304`). Both wait
on Slice 4.1c-b (a real destination consumer), not on a validator.

---

## 1. What the shipped checker actually is

`pkg/fhir/validate.go` is a **required-field presence check plus a profile-URL
string-membership check**. It is not a profile validator.

| Conformance mechanism | Present? | Evidence |
|---|---|---|
| JSON well-formedness | Yes | `validate.go:24-27` |
| `resourceType` present | Yes | `validate.go:54-57` |
| Bundle traversal into `entry[].resource` | Yes | `validate.go:65-102` |
| Per-resource required-element checks | **6 resource types only** | `validate.go:114-146` (see §3) |
| `meta.profile` contains an expected URL | **21 resource types only** | `validate.go:153-177`, `:179-237` |
| StructureDefinition / snapshot evaluation | **No** | `StructureDefinition` appears in no non-test Go file except as URL text in `pkg/fhir/types.go:13` |
| Pinned IG package (`.tgz`) | **No** | `git ls-files` matches no `.tgz`; no `hl7.fhir.us.core` package anywhere in the tree |
| Terminology binding evaluation (required/extensible) | **No** | zero `ValueSet` or `binding` occurrences in `pkg/fhir/validate.go` and `pkg/fhir/mapper.go` |
| Primitive-type / regex validation (`date`, `code`, `uri`) | **No** | `requireNonEmptyString` (`validate.go:239-249`) tests non-emptiness only |
| Cardinality beyond "non-empty array" | **No** | `requireNonEmptyArray` (`validate.go:270-280`) |
| Invariants / FHIRPath constraints | **No** | no FHIRPath engine in `go.mod` |
| Slicing, must-support, USCDI-Requirement categories | **No** | not modelled |
| Unknown-element rejection | **No** | unknown resource types are an explicit no-op (`validate.go:144-146`, `:155-157`) |
| Profile-URL **version** awareness | **Tolerant, not resolving** (5.1a) | `USCoreBaseURL` (`pkg/fhir/types.go:13`) is unversioned and no constant is pinned. `ProfileCanonical` strips `\|version` before the membership test, so a pinned canonical matches; nothing *resolves* the version. Resolution is 5.1b, over the pinned `.tgz`. |

### 1.1 Four demonstrated behaviours

Reproduced with the shipped CLI built from `origin/main` @ `55412bda`
(`go build ./cmd/fi-fhir`; default mode is `us-core` and default `--strict` treats
warnings as failure — `cmd/fi-fhir/main.go` `runFHIRValidate`):

| # | Input | Result | Why |
|---|---|---|---|
| 1 | `DiagnosticReport` whose `meta.profile` is `…/us-core-diagnosticreport-lab` | **WARNING** … → CLI exits non-zero. **FIXED in 5.1a**: `-lab` is now a declared constant and an accepted profile; regression-guarded by `make fhir-conformance-negative-control` | `expectedProfilesForResourceType` accepts only `us-core-diagnosticreport-note` for `DiagnosticReport` (`validate.go:218-219`), but the repo's own mapper stamps `us-core-diagnosticreport-lab` (`pkg/fhir/mapper.go:435`) |
| 2 | `Patient` whose `meta.profile` is `…/us-core-patient\|9.0.0` | **WARNING**, same class. **FIXED in 5.1a**: `ProfileCanonical` drops the `\|version` suffix before the lookup; policy is bare canonicals asserted, either form accepted | Exact string equality (`validate.go:171`); a version-pinned canonical never matches |
| 3 | `Patient` with `"gender":"purple"` and `"birthDate":"not-a-date"` plus an unknown element | **PASSES clean** | `requireNonEmptyString` checks non-emptiness only (`validate.go:239-249`); no bindings, no primitive regex, no unknown-element rule |
| 4 | `Device` with `meta.profile` = `http://example.org/not-a-real-profile` | **PASSES clean** | `Device` is absent from both switches (`validate.go:144-146`, `:234-236`) |

Case 1 is the sharpest: **the shipped validator rejects, by default, output the
shipped mapper produces.** Case 2 is the sharpest for 5.1: the moment US Core
9.0.0 is pinned and asserted the way IG-versioned publishing does it, today's
checker warns on every conformant resource.

---

## 2. Resource-type coverage

The mapper exposes 26 `Map*` entry points plus 2 `Create*Bundle` helpers,
producing **24 distinct non-Bundle resource types**. The profile-presence check
knows **21** resource types. The required-element check knows **6**.

| Resource type produced | Mapper entry point | `meta.profile` stamped by the mapper | Profile-presence checked? | Required-element checked? |
|---|---|---|---|---|
| Patient | `mapper.go:120` | `us-core-patient` (`:128`) | Yes (`validate.go:181-182`) | Yes (`validate.go:115-119`) |
| Encounter | `:220` | `us-core-encounter` (`:228`) | Yes (`:183-184`) | Yes (`:121-124`) |
| Observation (lab) | `:319` | `us-core-observation-lab` (`:327`) | Yes, 10 accepted URLs (`:185-197`) | Yes (`:126-129`) |
| Observation (vital signs) | `:2799` | one of 8, chosen at `:2859` (`:2811`) | Yes, same set | Yes |
| DiagnosticReport (lab) | `:392` | `us-core-diagnosticreport-lab` (`:435`) | Yes, but **always warns** — only `-note` is accepted (`:218-219`) | Yes (`:131-134`) |
| DiagnosticReport (note) | `:5812` | `us-core-diagnosticreport-note` (`:5821`) | Yes (`:218-219`) | Yes |
| Condition | `:844` | `us-core-condition-problems-health-concerns` (`:852`) | Yes (`:198-199`) | Yes, `subject.reference` only (`:136-137`) |
| Coverage | `:913` | `us-core-coverage` (`:921`) | Yes (`:200-201`) | Yes (`:139-142`) |
| Procedure | `:2352` | `us-core-procedure` (`:2361`) | Yes (`:202-203`) | **No** |
| Immunization | `:2564` | `us-core-immunization` (`:2573`) | Yes (`:204-205`) | **No** |
| MedicationRequest | `:3111` | `us-core-medicationrequest` (`:3122`) | Yes (`:206-207`) | **No** |
| AllergyIntolerance | `:3630` | `us-core-allergyintolerance` (`:3640`) | Yes (`:208-209`) | **No** |
| CarePlan | `:3981` | `us-core-careplan` (`:3990`) | Yes (`:210-211`) | **No** |
| Goal | `:4300` | `us-core-goal` (`:4309`) | Yes (`:212-213`) | **No** |
| CareTeam | `:4668` | `us-core-careteam` (`:4677`) | Yes (`:214-215`) | **No** |
| ServiceRequest | `:4976` | `us-core-servicerequest` (`:4985`) | Yes (`:216-217`) | **No** |
| DocumentReference | `:5377` | `us-core-documentreference` (`:5386`) | Yes (`:220-221`) | **No** |
| Provenance | `:6082` | `us-core-provenance` (`:6091`) | Yes (`:222-223`) | **No** |
| Location | `:6380` | `us-core-location` (`:6389`) | Yes (`:224-225`) | **No** |
| Organization | `:6532` | `us-core-organization` (`:6541`) | Yes (`:226-227`) | **No** |
| Practitioner | `:6650` | `us-core-practitioner` (`:6659`) | Yes (`:228-229`) | **No** |
| PractitionerRole | `:6813` | `us-core-practitionerrole` (`:6822`) | Yes (`:230-231`) | **No** |
| RelatedPerson | `:6980` | `us-core-relatedperson` (`:6989`) | Yes (`:232-233`) | **No** |
| Claim | `:1287` | Da Vinci PAS profile **only when `use == "preauthorization"`** (`:1298-1313`) | **No** — absent from `expectedProfilesForResourceType` | **No** |
| ExplanationOfBenefit | `:1598` | PDex `pdex-adjudication` (`:1606`) | **No** | **No** |
| CoverageEligibilityResponse | `:1925` | **none — `Meta` is never set** (`:1930-1935`) | **No** | **No** |
| Bundle (transaction / searchset) | `:1241`, `:1263` | n/a | Traversed only (`validate.go:65-102`) | n/a |

**Headline: 24 resource types produced; 21 profile-checked; 6
required-element-checked; 3 with no profile check at all; 18 with no
required-element check at all.** One of the 21 — `DiagnosticReport` — has an
incomplete accepted-profile set: it accepts the note variant and rejects the lab
variant the same mapper emits.

Two documentation claims in `docs/planning/FHIR-PROFILES.md` are false against
this table and must be corrected by the slice that owns that file:

- `:485` "Profile metadata injection (Meta.Profile set on all resources)" —
  `CoverageEligibilityResponse` never sets it (`mapper.go:1930-1935`), and
  `Claim` sets it only for pre-authorization (`mapper.go:1298-1313`).
- `:80` "Current version: **US Core 6.1.0**" and `:445` `version: "6.1.0"` and
  `:542` `-ig hl7.fhir.us.core#6.1.0` — the 1.0 target is 9.0.0
  (`.loom/20-product-spec-integration-engine-ide-completion.md:262`,
  `docs/operations/SUPPORTED-1.0.md:26`). The package comment
  `pkg/fhir/types.go:2` ("These types support US Core 6.1.0 profiles") says the
  same thing, and is the honest statement of what the constants were written
  against.

---

## 3. Required-element coverage, element by element

`validateResource` (`validate.go:104-151`) runs **only** when
`ValidationOptions.Mode == "us-core"`; any other mode returns `nil` immediately
(`:109-111`).

| Resource | Elements checked | Severity | Line | Elements US Core marks mandatory or Must Support that are **not** checked |
|---|---|---|---|---|
| Patient | `identifier`, `name`, `gender`, `birthDate` | error | `:116-119` | `identifier.system`/`.value` sub-elements; `name.family`/`.given`; `telecom`, `address`, `communication.language`; race/ethnicity/birthsex/sex extensions; `gender` value-set binding |
| Encounter | `status`, `class.code`, `subject.reference` | error | `:122-124` | `type`, `participant`, `period`, `reasonCode`, `hospitalization.dischargeDisposition`, `location.location`; `status` and `class` bindings |
| Observation | `status`, `code.text`, `subject.reference` | error, except `code.text` = **warning** (`:255-257`) | `:127-129` | `category`, `code.coding` (LOINC), `effective[x]`, `value[x]`/`dataAbsentReason`, vital-signs UCUM units and per-profile LOINC constraints |
| DiagnosticReport | `status`, `code.text`, `subject.reference` | error, except `code.text` = warning | `:132-134` | `category`, `code.coding`, `effective[x]`, `issued`, `performer`, `result`, `presentedForm` |
| Condition | `subject.reference` | error | `:137` | `clinicalStatus`, `verificationStatus`, `category`, `code` — all of which the mapper populates (`mapper.go:994,1025,1052`) but nothing verifies |
| Coverage | `status`, `beneficiary.reference`, `payor` | error | `:140-142` | `identifier`, `subscriberId`, `relationship`, `period`, `class`, `type` |
| **All 18 other produced types** | none | — | `:144-146` | everything |

Note the asymmetry this creates: `Condition` is checked for one element while
`Procedure`, `Immunization`, `MedicationRequest`, and `AllergyIntolerance` — all
USCDI data classes — are checked for none.

---

## 4. Profile-constant coverage against US Core 9.0.0

> **5.1a note on the denominator.** The "55" below is an *external* number read
> from the published IG on 2026-08-08 and never verified against a pinned
> package, because there is no pinned package. Do not cite "0 of 55" — it mixes
> an unverifiable external denominator with a code numerator. **Cite the code
> numbers**, which are computed by
> `TestFHIRConformance_CheckerCoverageIsDerivableFromCode`: **32 profile
> constants, 0 version-pinned, 1 declared-but-unused**; **24 non-Bundle resource
> types produced**, of which **21** have a profile-presence check, **6** have
> required-element checks, and **3** have neither. The external denominator
> becomes citable when 5.1b pins the `.tgz`.

US Core 9.0.0 (`hl7.fhir.us.core#9.0.0`, STU 9, published 2026-05-31, based on
FHIR 4.0.1, meets USCDI **v6**) defines **55 profiles** (external, unverified).
`pkg/fhir/types.go` defines **32** profile constants — 31 at `55412bda`, plus
`USCoreDiagnosticReportLabProfile`, which Slice 5.1a promoted from the bare
literal at `mapper.go:435`.

- **All constants name slugs that still exist in 9.0.0.** There is no dangling
  profile URL in the codebase. The version problem is not slug rot.
- **23 profiles in 9.0.0 have no constant and no mapper** (24 at `55412bda`;
  `us-core-diagnosticreport-lab` is no longer among them — Slice 5.1a declared it
  as `USCoreDiagnosticReportLabProfile` and added it to the checker's accepted
  set for `DiagnosticReport`):
  `us-core-condition-encounter-diagnosis`, `us-core-device`,
  `us-core-adi-documentreference`, `us-core-familymemberhistory`,
  `us-core-medicationdispense`, `us-core-average-blood-pressure`,
  `us-core-care-experience-preference`, `us-core-observation-adi-documentation`,
  `us-core-observation-clinical-result`, `us-core-observation-occupation`,
  `us-core-observation-pregnancyintent`, `us-core-observation-pregnancystatus`,
  `us-core-observation-screening-assessment`,
  `us-core-observation-sexual-orientation`, `us-core-simple-observation`,
  `us-core-smokingstatus`, `us-core-treatment-intervention-preference`,
  `pediatric-bmi-for-age`, `pediatric-weight-for-height`,
  `us-core-head-circumference`,
  `head-occipital-frontal-circumference-percentile`,
  `us-core-questionnaireresponse`, `us-core-specimen`.
- **One constant is declared but unused**: `USCoreMedicationProfile` is
  referenced nowhere outside its own declaration — no `Medication` mapper, no
  entry in `expectedProfilesForResourceType`. It is kept (it is the correct
  canonical for the day a Medication mapper exists) and documented as such at its
  declaration, and it is **not** counted as coverage.
- **Every constant is unversioned, and that is now a policy rather than an
  oversight.** `USCoreBaseURL` (`types.go:13`) has no `|<version>` suffix and
  there is no package pin. Slice 5.1a chose: the mapper asserts **bare
  canonicals**; the checker accepts a bare canonical or any pinned form of it
  (`ProfileCanonical`). Pinning the constants instead was rejected because this
  checker has no package-resolution step, so a pinned constant would assert a
  version it cannot verify and would reject a correct bare canonical. §1.1 case 2
  no longer fails.

**Headline (code, asserted): 32 profile constants; 0 version-pinned; 1 declared
but unused. 24 non-Bundle resource types produced; 21 profile-presence-checked;
6 required-element-checked; 3 with neither —** `Claim`, `ExplanationOfBenefit`,
`CoverageEligibilityResponse`. **`MapCoverageEligibilityResponse` never sets
`Meta` and `MapClaim` sets it only for pre-authorization**, which is why those
types have no fixture in `testdata/fhir/mapper/`: a fixture that validates
because nothing is checked would be evidence of nothing.

**Headline (external, verified against the pinned package on 2026-09-08): 55 US
Core 9.0.0 profiles; 23 with neither a constant nor a mapper.** The 2026-08-08
reading, taken from the published IG rather than a package, was correct in both
numbers. `hl7.fhir.us.core-9.0.0.tgz` holds 70 `StructureDefinition` resources:
55 are `kind: resource`, `derivation: constraint` — the profile inventory — and
the other 15 are complex-type constraints. All 32 profile constants resolve to
one of the 55, asserted by
`TestFHIRStructural_EveryProfileConstantResolvesInThePinnedPackage`, so the
"declared but unused" `USCoreMedicationProfile` is unused rather than wrong.
The 23 uncovered profiles are the pediatric vital-signs family, the social-
determinants and screening `Observation` profiles (`us-core-smokingstatus`,
`us-core-observation-occupation`, `us-core-observation-pregnancystatus`,
`us-core-simple-observation` and siblings), `us-core-device`,
`us-core-specimen`, `us-core-medicationdispense`, `us-core-familymemberhistory`,
`us-core-questionnaireresponse`, the advance-directive documents, and
`us-core-condition-encounter-diagnosis`.

External IG facts in this section (profile inventory, version/USCDI alignment,
package identifier) were read from the published US Core 9.0.0 IG and the package
registry on 2026-08-08 and re-verified against the pinned
`testdata/fhir/packages/hl7.fhir.us.core-9.0.0.tgz` on 2026-09-08 (Slice 5.1b).
The IG is on an annual release cadence, so a re-pin is the next occasion to
re-verify; `.loom/28-spec-fhir-ig-bulk-smart.md` carries that instruction.

---

## 5. What Slice 5.1 must decide, in order — status as of 2026-08-09

The order in this section was **inverted** by Slice 5.1a and its coordinator
ruling. The original list put the engine choice first; reconciliation was item 4.
Building a bigger validator over a mapper it disagrees with certifies the
disagreement at higher resolution, so reconciliation went first. See
`.loom/40-decisions.md` (2026-08-09 amendment).

| # | Item | Status |
|---|---|---|
| 1 | **Wait for 4.1c-b** — a conformance gate has nothing to gate until a resource exists on the durable path | **Satisfied by Slice 4.1c-c (2026-09-08), proven by `TestFHIRDestination_DurableEngineDeliversFHIRResource`** (`internal/integration/delivery/fhir_conformance_gate_test.go`), the deliberate inversion of `TestFHIRConformance_DurableEngineProducesNoFHIRResource`, which had proven that 4.1c-b did not satisfy this. The real dispatcher over a `fhir`-transport destination delivers a conditional transaction Bundle of a US Core Patient and Encounter — projected by `internal/integration/fhirout` from the exact payload the outbox stores — at `application/fhir+json`; every resource in it validates at `us-core --strict` with zero issues; the transport vocabulary is `{fhir, https, kafka}`; and `fhirout` is the only importer of `pkg/fhir` under `internal/integration/**`. Journey 6's validation input now exists on a path journey 1 executes. MR !211 expands the shared projector to eleven event types, including six additional clinical families; see `docs/operations/DESTINATION-IDENTITY.md` "The FHIR transport". |
| 2 | **Choose the validation engine** | **Ratified in full (2026-09-24, Slice 5.1c-β).** The confinement half was in force since 2026-08-09: `validator_cli.jar` is CI-only, the shipped image stays distroless static, IG packages are pinned offline `.tgz`. The amended ordering — reconcile (5.1a), then Option C (5.1b), then Option A — is now complete: `validator_cli.jar` 6.10.4 runs offline as the blocking `test:fhir-official` job over the mapper fixtures and the delivered Bundles. See §5.2 and `.loom/decisions/2026-09-24-run-the-hl7-validator-offline-as-a.md`. |
| 3 | **Pin the packages** | **Done (Slice 5.1b).** `hl7.fhir.r4.core#4.0.1` (12,815,597 bytes) and `hl7.fhir.us.core#9.0.0` (2,749,959 bytes) are checked in as offline `.tgz` under `testdata/fhir/packages/` with sha256 sums in `SHA256SUMS`, verified on every run by `TestFHIRStructural_PinnedPackagesMatchTheirRecordedDigests`. Both reproduce the registry-published `dist.shasum`. §4's external denominator is now citable and was re-verified against the archive. Whole archives, not an extracted subset: no size or scan gate rejected them — see §5.1 below. |
| 4 | **Fix the two self-inconsistencies** | **Done (Slice 5.1a).** §1.1 cases 1 and 2. A third, not in the original list — the checker failing open on any mode string that was not byte-exactly `us-core` — is also fixed. A fourth, `Patient.MRN` being dropped and producing a hard `Patient.identifier is required`, is fixed too. |
| 5 | **Decide the profile-version assertion policy** | **Done (Slice 5.1a).** Bare canonicals asserted; bare or pinned accepted. Recorded here, in `.loom/40-decisions.md`, and in `docs/operations/SUPPORTED-1.0.md`. |

Remaining after **Slice 5.1b**: the official validator as a CI-only job
(Option A, Sprint 7). Slice 4.1c-c has merged, so the delivered path is available for that work.
Structural checks and live HAPI acceptance do not replace official validation.
**Delivered by Slice 5.1c-β (2026-09-24) — see §5.2.**

### 5.1 The Go structural validator (Option C) — Slice 5.1b, 2026-09-08

`pkg/fhir/structural.go` resolves resources against the pinned archives. Gate:
`make fhir-structural`, CI job `test:fhir-structural` (blocking); negative
control `make fhir-structural-negative-control`.

**What it checks.** For every resource: that `resourceType` names a
`StructureDefinition` in pinned R4 4.0.1; that every `meta.profile` canonical
resolves in the pinned packages, constrains that resource type, and has a
`baseDefinition` chain terminating at the R4 base; and cardinality — `min`,
`max`, and prohibition — from both the R4 base snapshot and every resolved
profile's snapshot, applied at **every depth** by walking snapshot paths against
the JSON, so `Encounter.identifier.system` is graded as well as
`Encounter.type`. Choice elements (`value[x]`) are resolved to their typed JSON
property. Must-support elements a resource leaves unpopulated are reported at
information severity and never fail: US Core `mustSupport` binds the system, not
the instance.

**Version resolution, finally.** Item 5's policy is unchanged — the mapper
asserts bare canonicals, the checker accepts bare or pinned — but the checker
can now tell a *correct* pin from a wrong one. `…/us-core-patient|9.0.0`
resolves; `…/us-core-patient|8.0.0` is rejected. Under 5.1a's tolerance alone
both passed. This is the sentence `docs/operations/SUPPORTED-1.0.md` was waiting
on: version tolerance has become version resolution.

**What it does NOT check, and this list is the point of the section.** No
terminology: a `ValueSet` binding of any strength is not evaluated, so a code
outside a required binding passes. No invariants: FHIRPath `constraint`
expressions are not evaluated. No slicing: elements whose snapshot `id` carries
a `:` discriminator are skipped, because assigning a JSON element to a slice
needs the terminology this validator does not have — the *unsliced* element's
cardinality is still checked. No primitive-type or regex validation, no
reference-target checking, no extension validation, no `contentReference`
expansion. **This is not official conformance and produces no certificate.**
`validator_cli.jar` (Option A) remains Sprint 7.

**Result over the 21-resource-type fixture set** (25 files in
`testdata/fhir/mapper/`; `labresult_1/2/3`, `labobservation` and `vitalsign` are
all `Observation` or `DiagnosticReport`): **25 files clean, ledger empty as of
Slice 5.1c-α (2026-09-24).** `recordedCardinalityGaps()` returns an empty map
and is still asserted by **exact equality**, so a violation that reappears fails
the build; `TestUSCoreMapper_FixturesCarryNoCardinalityViolations` asserts the
same zero without reference to the ledger. That test failed on `main` at
`1465aa516` naming exactly the seven violations in the history table below.

This is cardinality only. Terminology bindings, invariants and slicing are
unmeasured by this validator (see the list above); the official validator is
what measures them.

How each gap was closed (choices, code systems and fallbacks in
`.loom/decisions/2026-09-24-close-the-mapper-s-us-core-cardinality.md`):

| Fixture | Element | Closed by |
|---|---|---|
| `careteam.json` | `CareTeam.participant` | The event's members are projected; an event naming nobody produces **no** CareTeam. A member with no role gets DataAbsentReason `unknown`, as US Core's own missing-data example does. |
| `coverage.json` | `Coverage.relationship` | `self` when the 271 carries no dependent loop; DataAbsentReason `unknown` when it does, because the canonical event does not carry INS02. |
| `documentreference.json` | `DocumentReference.content` (×2) | `content` is `omitempty`, never `null`; an event with no attachment that locates or carries a document produces **no** DocumentReference. |
| `encounter.json` | `Encounter.identifier.system` | A bare visit number is qualified under `urn:fi-fhir:source:<source>` when the mapper is told the event's source (`USCoreMapper.Source`). |
| `encounter.json` | `Encounter.type` | Derived from PV1-2: SNOMED CT `86181006` (I) or `4525004` (E) plus the HL7 v2 table 0004 code; the other classes carry the v2 code only. |
| `vitalsign.json` | `Observation.effective[x]` | The conformance row now carries the `Timestamp` every shipped producer stamps; a zero `Timestamp` is left absent, never filled with the receipt time. |

#### History — the Slice 5.1b measurement (2026-09-08, as amended 2026-09-20)

**20 files clean, 5 carrying 7 cardinality violations** after the 2026-09-20
medication serialization fix. Every one was confirmed against the fixture by
hand; one earlier report — `Coverage.payor` serialised as a JSON array — was a
validator defect and was fixed rather than recorded, because a profile narrowing
`max` from `*` to `1` constrains the count, not the JSON wire shape.

| Fixture | Violation | Required by |
|---|---|---|
| `careteam.json` | `CareTeam.participant` absent | `us-core-careteam` 1..* |
| `coverage.json` | `Coverage.relationship` absent | `us-core-coverage` 1..1 |
| `documentreference.json` | `DocumentReference.content` is JSON `null` | R4 **and** `us-core-documentreference` 1..* |
| `encounter.json` | `Encounter.identifier.system` absent | `us-core-encounter` 1..1 |
| `encounter.json` | `Encounter.type` absent | `us-core-encounter` 1..* |
| `vitalsign.json` | `Observation.effective[x]` absent | `us-core-heart-rate` 1..1 |

The explicit `null` violated base R4, not just a US Core tightening. The
MedicationRequest mapper had already been changed to preserve
`allowedBoolean: false`, closing its two recorded violations. Slice 5.1b itself
did **not** fix any of them. Its deliverable was the measurement, and changing
the mapper in the change that first made it measurable would have taken the
measurement against unreviewed output; what a `DocumentReference` with no
attachment should emit was a product decision, not a defect fix. Slice 5.1c-α
made those decisions.

**Two places the shipped checker is stricter than the IG.** `validate.go` makes
an absent `Patient.gender` and an absent `Patient.birthDate` hard errors; in US
Core 9.0.0 `Patient.gender` is `0..1` with no `mustSupport` flag at all and
`Patient.birthDate` is `0..1` mustSupport. Neither can produce a non-conformant
`Patient`, so nothing was changed;
`TestFHIRStructural_RequiredElementChecksAgreeWithPinnedProfiles` records them
as deliberate local policy and fails if the IG ever makes them required. The
other 15 of the 17 hand-written required-element checks agree with the pinned
profiles exactly.

**One base chain leaves the pinned set.** `us-core-questionnaireresponse` bases
on `sdc-questionnaireresponse` in `hl7.fhir.uv.sdc`, one of the seven
dependencies US Core 9.0.0 declares. The transitive closure was not pinned —
`hl7.terminology.r4` and `us.cdc.phinvads` are terminology this validator does
not evaluate, and no mapper emits a `QuestionnaireResponse`. The exception is
asserted by name, so a second one fails the build.

### 5.2 The official validator (Option A) — Slice 5.1c-β, 2026-09-24

HL7's reference implementation, **`validator_cli.jar` 6.10.4** (sha256
`1106b9d58f9e363e47bea7c4fc065841e5fc91fe9d062775c3bfdd212bd653cc`, pinned by
version and digest in `scripts/fhir-official-validate.sh`), runs offline over
**36 inputs**: the 25 mapper fixtures in `testdata/fhir/mapper/` and the
transaction Bundle the durable `fhir` transport delivers for each of the 11
supported event types — captured by
`TestDeliveredBundlesForEverySupportedEventType` (`internal/integration/fhirout`),
byte-identical to the request body. Target: R4 4.0.1 +
`hl7.fhir.us.core#9.0.0`, `-tx n/a`, `-no-http-access`. The findings must
**equal** `testdata/fhir/official/findings.ledger.txt` exactly — a new finding
and a fixed one both fail the build, the same shape as
`recordedCardinalityGaps()`. Gate: `make fhir-official`, CI
`test:fhir-official-capture` (Go) → `test:fhir-official` (JRE), both blocking;
negative control `make fhir-official-negative-control` (Patient.name removed
from a copy of `patient.json` changes exactly that file's rows, adding
`Patient.name: minimum required = 1, but only found 0`).

**Hermetic, with 23 archives, not 2.** The day-1 kill-test refuted the sprint's
riskiest assumption as written: with `--network none` and only
`hl7.fhir.r4.core#4.0.1` and `hl7.fhir.us.core#9.0.0`, the validator stops at
`Unable to resolve package id hl7.fhir.r4.core#4.0.1` (the core must sit in its
package cache) and then demands its own defaults and US Core's full transitive
closure. Exactly the demanded 21 further archives (99,247,081 bytes, none
licence-gated, each reproducing the registry `dist.shasum`) are pinned under
`testdata/fhir/packages/`; the README lists each one and what demands it. (This
is the transitive closure §5.1 declined to pin; the structural validator still
loads only the first two.) The script builds a private package cache from
exactly those 23, fails if the
validator installs anything, and fails if its own `Package Summary` differs from
the pinned set in either direction. offline == online: identical 23-package
summary, identical findings — an online run with an empty cache and network
access produced the same ledger, row for row, over all 36 inputs.

**What it proves that §5.1 did not.** Everything the reference validator
evaluates without a terminology server:

- **Invariants** — FHIRPath `constraint`s from R4 and US Core
  (`us-core-8`/`us-core-9` on the lab `DiagnosticReport`, `us-core-21` on
  `MedicationRequest`, `us-core-17` NPI Luhn check, `pd-1` on
  `PractitionerRole`, `dom-6` everywhere).
- **Slicing** — `DiagnosticReport.category:LaboratorySlice` is required and not
  matched.
- **Primitive and datatype rules** — OID syntax, `urn:ietf:rfc:3986`
  identifier values, example-domain URLs, resolvable canonical URLs.
- **References inside a Bundle** — the delivered ADT Bundles'
  `Encounter.subject` cannot match a US Core Patient, because that Patient fails
  its own profile.
- **Cardinality, independently** — before Slice 5.1c-α every one of §5.1's
  seven structural gaps appeared here too, found by a second engine; after it,
  this engine also reports **no** cardinality finding on any of the 36 inputs,
  agreeing with §5.1's empty ledger.
- **Terminology posture under `-tx n/a`.** Membership *is* checked where the
  code system's content ships in the pinned THO packages — it caught `LAB`
  (not a code in `observation-category`), `discharge` (not in US Core
  `careplan-category`) and `physician` (not in `practitioner-role`). Everything
  that needs a server is recorded as "could not check", not as a pass: LOINC,
  SNOMED CT, RxNorm, UCUM and CVX membership, and every VSAC value set
  (`cts.nlm.nih.gov`, `ValueSet … not found`). `us.nlm.vsac` is not a US Core
  9.0.0 dependency and was not pinned.

**The ledger at landing** (over `main` with Slice 5.1c-α merged):

| Inputs | Files | Error | Warning | Information | Total |
|---|---|---|---|---|---|
| Mapper fixtures | 25 | 16 | 56 | 27 | 99 |
| Delivered Bundles | 11 | 12 | 30 | 16 | 58 |
| **All** | **36** | **28** | **86** | **43** | **157** |

The 28 errors by class: invariants 8, datatype/identifier rules 10 (OID
syntax, `urn:ietf:rfc:3986` values, example-domain URLs, an unresolvable
system URL), local code-system membership 4, in-Bundle reference profile match
4, slicing 2 — and cardinality **0**. 16 of the 36 inputs carry at least one
error. Some errors belong to the *test input* rather than the mapper
(`http://hospital.example.org/mrn`, `urn:oid:1.2.3`); they are recorded on the
same terms, because the gate measures what is emitted, not who is to blame.

The same gate run over `main` *before* 5.1c-α recorded 163 findings, 41 of
them errors: 5.1c-α removed the 14 cardinality error rows (its seven gaps,
counted once per profile that states them, plus `Encounter.type` in the four
ADT Bundles) and, by emitting the codings those elements now carry, added
VSAC `ValueSet … not found` warnings for `Encounter.type` and
`CareTeam.participant.role` and one example-URL error on the new
`DocumentReference` attachment. That is the ledger doing its job: the fix and
its consequences are both visible rows.

**What is still not certified.** This is evidence, not a certificate, and the
ledger is not a pass: it records 28 errors and holds them still. No terminology
server — external code-system and value-set membership is unchecked by design.
Only the outbound surface (§6 stays out of scope), and only the resources the
mapper fixtures and the 11 projectable event types produce. SMART App Launch
2.2.0 and Bulk Data 3.0.0 have no conformance evidence of any kind; their
packages are loaded only because US Core depends on them. `security:trivy`
still does not decompress `.tgz` (vuln and secret gates exit 0 over all 23
archives with trivy 0.63.0).

---

## 6. The second FHIR surface: inbound, and out of 5.1's scope

Every 5.1 document analyses the **outbound** direction — `pkg/fhir`, canonical
event → FHIR resource. There is a second FHIR surface pointing the other way, and
no 5.1 document mentioned it.

`internal/fhir/subscription/` (7 files) is an **inbound** FHIR→canonical-event
mapper: `mapper.go:30` — "FHIRMapper converts FHIR resources to canonical
events". It does not import `pkg/fhir`, it performs **no validation**, and it is
imported by `cmd/fi-fhir/main.go` and two GraphQL resolvers.

**Scope statement (Slice 5.1a): inbound conformance is OUT of scope for Slice
5.1, and this is a deliberate decision rather than an omission.**

Reasoning:

- **5.1's deliverable is a conformance *claim* about what this product emits.**
  "fi-fhir produces US Core 9.0.0-conformant resources" is a statement about
  outbound bytes. Validating what a *sender* hands us is an ingest-robustness
  property, not a conformance claim, and it is graded against a different
  question: what should happen to a non-conformant inbound resource? Rejecting it
  is a breaking behaviour change for every existing subscription sender; accepting
  it with a warning needs a warning channel that does not exist on that path.
  Neither is a standards decision that 5.1 can make on its own.
- **The engines differ.** An inbound validator would sit on a GraphQL-resolver
  path, in-process, in the shipped image. The ratified confinement half of the
  validator decision keeps `validator_cli.jar` in CI and out of the image, so
  inbound validation cannot reuse 5.1b's engine and would need its own — which is
  Option B, explicitly deferred.
- **It is not a silent gap.** It is recorded here, and the surface is small and
  enumerable (7 files, one entry point).

**What a future slice would need to decide first**, before writing any code:
whether a non-conformant inbound resource is rejected, accepted-with-warning, or
accepted-and-recorded; where that outcome is observable to the sender; and
whether the check runs in-process or as a pre-ingest gate. Until those are
answered, adding a validator to that path would pick the answer by accident.

---

## Sources

Repository, all at `origin/main` @ `55412bda`:

- `pkg/fhir/validate.go:23-51,53-63,65-102,104-151,153-177,179-237,239-249,251-268,270-280`
- `pkg/fhir/types.go:2,13,15-56`
- `pkg/fhir/mapper.go:120,128,220,228,319,327,392,435,844,852,913,921,1241,1263,1287,1298-1313,1598,1606,1925,1930-1935,2352,2361,2564,2573,2799,2811,2859,3111,3122,3630,3640,3981,3990,4300,4309,4668,4677,4976,4985,5377,5386,5812,5821,6082,6091,6380,6389,6532,6541,6650,6659,6813,6822,6980,6989`
- `cmd/fi-fhir/main.go` — `runFHIR`, `runFHIRValidate`, `printFHIRValidateUsage` (symbols, not line numbers: correction 2 found every `main.go` line number in the merged 5.1 docs to be wrong, and this file moves under every lane)
- `internal/workflow/actions.go:23,645-669,670,680,720,797,871`; `engine.go:127`; `plan.go:16-20,64,144-145`; `published_yaml.go:801`; `validate.go:306,630-670`
- `internal/integration/processor/workflow_plan.go:41,45,111-119,147-155`
- `internal/fhir/subscription/mapper.go:30` (§6, the inbound surface)
- `internal/integration/processor/postgres_submission.go:262-278,375-388`
- `internal/integration/delivery/dispatcher.go:237-277`; `store.go:107,128`
- `docs/planning/FHIR-PROFILES.md` — the 6.1.0 references and the "Meta.Profile on all resources" claim, both annotated by Slice 5.1a
- `docs/operations/SUPPORTED-1.0.md` — the standards row
- `.loom/20-product-spec-integration-engine-ide-completion.md:262,293,302-304,363-364`
- `.loom/30-implementation-plan-integration-engine-ide-completion.md:803-807,856`
- `.loom/32-sprint4-execution-specs.md` corrections 35-38 and Lane S4-D

External, read 2026-08-08:

- US Core Implementation Guide 9.0.0 (STU 9), `hl7.fhir.us.core#9.0.0`, based on
  FHIR 4.0.1, published 2026-05-31, meets USCDI v6 —
  https://hl7.org/fhir/us/core/STU9/
- US Core 9.0.0 profile inventory —
  https://hl7.org/fhir/us/core/STU9/profiles-and-extensions.html
- HL7 FHIR Validator (`validator_cli.jar`) —
  https://confluence.hl7.org/display/FHIR/Using+the+FHIR+Validator
