//go:build !fhirstructuralnegative

package fhir

// mutateFixtureForNegativeControl is the identity in every ordinary build.
//
// Its tagged counterpart in structural_negative_control_test.go removes one
// required element from one fixture, so that
// `make fhir-structural-negative-control` can require the structural gate to go
// red on exactly that fixture. Nothing but the control build should ever set
// the `fhirstructuralnegative` tag.
func mutateFixtureForNegativeControl(_ string, data []byte) []byte { return data }
