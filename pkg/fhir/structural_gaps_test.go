package fhir

// recordedCardinalityGaps is the ledger of every place the mapper's own output
// violates a cardinality rule in the pinned packages.
//
// # WHY THIS FILE EXISTS RATHER THAN A GREEN TEST
//
// `.loom/34-sprint6-execution-specs.md` Lane S6-D expected every mapper fixture
// to pass the structural validator. Twenty of twenty-five now do. The remaining
// five carry seven violations, and every one of them is real — each was checked
// against the fixture by hand after the validator reported it, and one earlier
// report (`Coverage.payor` as a JSON array) was a validator bug that was fixed
// rather than recorded.
//
// Slice 5.1b did not fix them, on purpose. Its deliverable is a *measurement*:
// resolve what this product emits against the implementation guide it claims to
// follow, for the first time. Changing the mapper in the same change that first
// makes the mapper measurable would mean the measurement was taken against
// output nobody had reviewed, and the emitted-resource changes involved are
// product decisions (what does a DocumentReference with no attachment emit?)
// rather than defect fixes. They belong to a slice that can be reviewed as
// such. `docs/planning/FHIR-CONFORMANCE-MATRIX.md` §5 names it.
//
// # THE LEDGER IS A GATE, NOT A SUPPRESSION LIST
//
// `TestFHIRStructural_MapperFixturesMatchTheirRecordedCardinalityGaps` requires
// EXACT equality. A new violation fails the build; so does a fixed one. The
// ledger can only ever be edited deliberately, and it can only shrink to zero.
//
// THE REMAINING SEVEN
//
//  1. careteam.json — `CareTeam.participant` is 1..* in `us-core-careteam`.
//     MapCareTeam emits a CareTeam with a subject and no participants, which is
//     a care team with nobody on it.
//
//  2. coverage.json — `Coverage.relationship` is 1..1 in `us-core-coverage`
//     (the subscriber's relationship to the beneficiary). The mapper emits
//     subscriber and beneficiary but never the relationship between them.
//
//     3-4. documentreference.json — `DocumentReference.content` is 1..* in BOTH R4
//     and US Core, and the mapper emits `"content": null`. This is the only gap
//     that violates base R4 rather than a US Core tightening, and an explicit
//     JSON null is worse than an omission: it asserts the element is present
//     and empty. Reported twice because two StructureDefinitions in the chain
//     require it, which is correct — a reader should see that this is not a
//     profile-only concern.
//
//  5. encounter.json — `Encounter.identifier.system` is 1..1 in
//     `us-core-encounter`. The fixture's identifier carries a value with no
//     system, so the identifier is not resolvable to a namespace. Note the
//     location, `identifier[0].system`: this is a nested rule found by walking
//     into a present optional parent, not a top-level check.
//
//  6. encounter.json — `Encounter.type` is 1..* in `us-core-encounter`.
//
// The two original MedicationRequest.substitution.allowed[x] violations were
// fixed by preserving allowedBoolean:false in JSON (2026-09-20).
//
//  7. vitalsign.json — `Observation.effective[x]` is 1..1 in
//     `us-core-heart-rate` (a vital sign without a time is not interpretable).
//     Note that `labobservation.json` and the three `labresult` fixtures are
//     clean: this is specific to the vital-signs profile chain, which is
//     exactly the kind of per-profile difference that only package resolution
//     can see.
//
// Each entry is `location :: diagnostics`, the form renderIssues produces.
func recordedCardinalityGaps() map[string][]string {
	const (
		r4   = "http://hl7.org/fhir/StructureDefinition/"
		core = "http://hl7.org/fhir/us/core/StructureDefinition/"
	)

	return map[string][]string{
		"careteam.json": {
			"participant :: CareTeam.participant is required by " + core + "us-core-careteam (min 1)",
		},
		"coverage.json": {
			"relationship :: Coverage.relationship is required by " + core + "us-core-coverage (min 1)",
		},
		"documentreference.json": {
			"content :: DocumentReference.content is required by " + r4 + "DocumentReference (min 1) but is null",
			"content :: DocumentReference.content is required by " + core + "us-core-documentreference (min 1) but is null",
		},
		"encounter.json": {
			"identifier[0].system :: Encounter.identifier.system is required by " + core + "us-core-encounter (min 1)",
			"type :: Encounter.type is required by " + core + "us-core-encounter (min 1)",
		},
		"vitalsign.json": {
			"effective :: Observation.effective[x] is required by " + core + "us-core-heart-rate (min 1)",
		},
	}
}
