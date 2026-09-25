package fhir

// recordedCardinalityGaps is the ledger of every place the mapper's own output
// violates a cardinality rule in the pinned packages.
//
// # IT IS EMPTY, AND THAT IS THE ASSERTION
//
// Slice 5.1b (2026-09-08) measured the mapper against the pinned packages for
// the first time and recorded nine violations across six of the twenty-five
// generated fixtures here rather than fixing them, because the fixes were
// product decisions that deserved their own review. MR !211 closed the two
// `MedicationRequest.substitution.allowed[x]` entries (2026-09-20). Slice
// 5.1c-α (2026-09-24) closed the remaining seven:
//
//   - careteam.json — `CareTeam.participant` (1..*): the mapper projects the
//     event's members and emits no CareTeam for an event that names none.
//   - coverage.json — `Coverage.relationship` (1..1): `self` when the 271
//     carries no dependent loop, DataAbsentReason `unknown` when it does.
//   - documentreference.json — `DocumentReference.content` (1..* in R4 and US
//     Core, reported twice): `content` is omitempty, and the mapper emits no
//     DocumentReference for an event with no attachment to reference.
//   - encounter.json — `Encounter.identifier.system` (1..1): a bare visit
//     number is qualified under `urn:fi-fhir:source:<source>`.
//   - encounter.json — `Encounter.type` (1..*): derived from PV1-2.
//   - vitalsign.json — `Observation.effective[x]` (1..1): the conformance row
//     now carries the Timestamp every shipped producer stamps.
//
// Each choice, with its code systems and fallbacks, is in
// `.loom/decisions/2026-09-24-close-the-mapper-s-us-core-cardinality.md`.
//
// # THE LEDGER IS STILL A GATE
//
// `TestFHIRStructural_MapperFixturesMatchTheirRecordedCardinalityGaps` requires
// EXACT equality with this map, so a violation that reappears fails the build
// just as a fixed-but-recorded one did. Do not add an entry to make a red build
// green: an entry is a recorded product decision, and needs one.
// `TestUSCoreMapper_FixturesCarryNoCardinalityViolations` asserts the same
// zero without reference to this map.
//
// Each entry, if one is ever added, is `location :: diagnostics`, the form
// renderIssues produces.
func recordedCardinalityGaps() map[string][]string {
	return map[string][]string{}
}
