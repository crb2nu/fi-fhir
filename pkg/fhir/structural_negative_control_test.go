//go:build fhirstructuralnegative

package fhir

import "encoding/json"

// mutateFixtureForNegativeControl, under the `fhirstructuralnegative` tag,
// removes `Patient.name` from `patient.json` before the structural gate sees
// it.
//
// This is the structural gate's negative control, in the same shape as
// `fhirdrnoteonly` is the 5.1a conformance table's. `patient.json` is one of the
// twenty-five fixtures recorded clean (all of them, since Slice 5.1c-α), and
// `Patient.name` is `1..*` in `us-core-patient`, so with this tag on
// `TestFHIRStructural_MapperFixturesMatchTheirRecordedCardinalityGaps` must fail
// on exactly `patient.json` and nowhere else. A control that passes would mean
// the gate is not actually reading the fixtures, and the twenty-five clean rows
// above it would be evidence of nothing.
//
// `make fhir-structural-negative-control` additionally requires the failure
// text to name `Patient.name`, so a control that failed for an unrelated reason
// — a missing package, a loader error — does not count as the gate working.
func mutateFixtureForNegativeControl(name string, data []byte) []byte {
	if name != "patient.json" {
		return data
	}

	var resource map[string]any
	if err := json.Unmarshal(data, &resource); err != nil {
		return data
	}
	delete(resource, "name")

	mutilated, err := json.Marshal(resource)
	if err != nil {
		return data
	}
	return mutilated
}
