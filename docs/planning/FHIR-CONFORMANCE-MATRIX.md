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
`docs/operations/SUPPORTED-1.0.md:26`, US Core 9.0.0 is a release *target* and
"official validator or conformance-suite evidence is not yet complete."

---

## 0. The finding that frames the matrix

**No FHIR resource is produced anywhere on the durable integration engine's
path.** The mapper is reachable from the legacy workflow engine and from one CLI
subcommand, and from nothing else.

| Claim | Proof |
|---|---|
| `pkg/fhir` has exactly two non-test importers | `git grep -n "fi-fhir/pkg/fhir" -- '*.go'` → `cmd/fi-fhir/main.go:49`, `internal/workflow/actions.go:23` (plus `internal/workflow/actions_test.go`) |
| The only mapper construction site is the legacy `fhir` action | `fhir.NewUSCoreMapper()` at `internal/workflow/actions.go:680`, inside `fhirAction` (`:670`) |
| That action is registered only on the legacy engine | `internal/workflow/engine.go:127` — `e.RegisterAction("fhir", ContextActionHandlerFunc(fhirAction))` |
| The only other importer is the CLI validator | `cmd/fi-fhir/main.go:309` — `fhirpkg.ValidateJSON(...)`, reached from `fi-fhir fhir validate` (`:243-244`) |
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
| Profile-URL **version** awareness | **No** | `USCoreBaseURL` (`pkg/fhir/types.go:13`) is unversioned; matching is exact string equality (`validate.go:171`) |

### 1.1 Four demonstrated behaviours

Reproduced with the shipped CLI built from `origin/main` @ `55412bda`
(`go build ./cmd/fi-fhir`; default mode is `us-core` and default `--strict` treats
warnings as failure — `cmd/fi-fhir/main.go:254-255,349`):

| # | Input | Result | Why |
|---|---|---|---|
| 1 | `DiagnosticReport` whose `meta.profile` is `…/us-core-diagnosticreport-lab` | **WARNING** `meta.profile does not include an expected profile for DiagnosticReport` → CLI exits non-zero | `expectedProfilesForResourceType` accepts only `us-core-diagnosticreport-note` for `DiagnosticReport` (`validate.go:218-219`), but the repo's own mapper stamps `us-core-diagnosticreport-lab` (`pkg/fhir/mapper.go:435`) |
| 2 | `Patient` whose `meta.profile` is `…/us-core-patient\|9.0.0` | **WARNING**, same class | Exact string equality (`validate.go:171`); a version-pinned canonical never matches |
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

US Core 9.0.0 (`hl7.fhir.us.core#9.0.0`, STU 9, published 2026-05-31, based on
FHIR 4.0.1, meets USCDI **v6**) defines **55 profiles**. `pkg/fhir/types.go:15-56`
defines **31** profile constants.

- **All 31 constants name slugs that still exist in 9.0.0.** There is no dangling
  profile URL in the codebase. The version problem is not slug rot.
- **24 profiles in 9.0.0 have no constant and no mapper**:
  `us-core-condition-encounter-diagnosis`, `us-core-device`,
  `us-core-diagnosticreport-lab` (used as a *literal* at `mapper.go:435`, never
  declared as a constant, which is exactly how it fell out of the checker's map),
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
- **One constant is dead**: `USCoreMedicationProfile` (`types.go:35`) is
  referenced nowhere outside its own declaration — no `Medication` mapper, no
  entry in `expectedProfilesForResourceType`.
- **Every constant is unversioned.** `USCoreBaseURL` (`types.go:13`) is
  `http://hl7.org/fhir/us/core/StructureDefinition/` with no `|<version>` suffix
  and no package pin, so nothing in the repository distinguishes a 6.1.0-era
  profile assertion from a 9.0.0 one. This is why §1.1 case 2 fails.

**Headline: 55 US Core 9.0.0 profiles; 31 named by constants; 24 unnamed; 1 named
but dead; 0 version-pinned.**

External IG facts in this section (profile inventory, version/USCDI alignment,
package identifier) were read from the published US Core 9.0.0 IG and the package
registry on 2026-08-08 and **must be re-verified against the pinned `.tgz` at
implementation start** — the IG is on an annual release cadence and
`.loom/28-spec-fhir-ig-bulk-smart.md` already carries that instruction.

---

## 5. What Slice 5.1 must therefore decide, in order

1. **Wait for 4.1c-b.** Until a destination revision can name a FHIR R4
   destination class and something calls a mapper on the durable path, a
   conformance gate has nothing to gate. See
   `.loom/30-implementation-plan-integration-engine-ide-completion.md` Slice 5.1
   and `.loom/32-sprint4-execution-specs.md` Lane S4-A.
2. **Choose the validation engine** before writing conformance code — see the
   proposed decision in `.loom/40-decisions.md`, "2026-08-08: FHIR conformance
   validation strategy (proposed)". It is a CI-image and supply-chain decision
   first and a code slice second.
3. **Pin the packages** (`hl7.fhir.r4.core#4.0.1`, `hl7.fhir.us.core#9.0.0`, and
   the terminology package the IG depends on) as vendored artifacts with recorded
   digests, whichever engine wins. Today nothing is pinned.
4. **Fix the two self-inconsistencies** found here regardless of engine choice —
   the missing `us-core-diagnosticreport-lab` mapping (§1.1 case 1) and the
   version-suffix mismatch (§1.1 case 2) — because both will produce false
   negatives in any migration.
5. **Decide the profile-version assertion policy**: whether emitted resources
   declare `…/us-core-patient` or `…/us-core-patient|9.0.0`, and make the checker
   agree with the mapper either way.

---

## Sources

Repository, all at `origin/main` @ `55412bda`:

- `pkg/fhir/validate.go:23-51,53-63,65-102,104-151,153-177,179-237,239-249,251-268,270-280`
- `pkg/fhir/types.go:2,13,15-56`
- `pkg/fhir/mapper.go:120,128,220,228,319,327,392,435,844,852,913,921,1241,1263,1287,1298-1313,1598,1606,1925,1930-1935,2352,2361,2564,2573,2799,2811,2859,3111,3122,3630,3640,3981,3990,4300,4309,4668,4677,4976,4985,5377,5386,5812,5821,6082,6091,6380,6389,6532,6541,6650,6659,6813,6822,6980,6989`
- `cmd/fi-fhir/main.go:49,243-244,254-255,309,330-350,366-383`
- `internal/workflow/actions.go:23,645-669,670,680,720,797,871`; `engine.go:127`; `plan.go:16-20,64,144-145`; `published_yaml.go:801`; `validate.go:306,630-670`
- `internal/integration/processor/workflow_plan.go:41,45,111-119,147-155`
- `internal/integration/processor/postgres_submission.go:262-278,375-388`
- `internal/integration/delivery/dispatcher.go:237-277`; `store.go:107,128`
- `docs/planning/FHIR-PROFILES.md:80,445,485,542`
- `docs/operations/SUPPORTED-1.0.md:26,62`
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
