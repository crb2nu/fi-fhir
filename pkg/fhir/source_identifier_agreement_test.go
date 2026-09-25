package fhir_test

import (
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/fhirout"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
)

// TestUSCoreMapper_SourceSystemAgreesWithFhirout pins the one namespace to two
// implementations.
//
// The 2026-09-08 decision names `fhirout.SourceIdentifierSystem` the place the
// `urn:fi-fhir:source:` namespace is minted. Slice 5.1c-α needs the mapper to
// emit the same system and may not change fhirout, so the mapper mints it too.
// Two mints are only safe while they agree byte for byte: if they diverged, the
// durable transport's ensureIdentifier would stop recognising the mapper's
// identifier as its key and append a second one to every delivered Encounter.
func TestUSCoreMapper_SourceSystemAgreesWithFhirout(t *testing.T) {
	for _, source := range []string{
		"main-hospital-adt",
		" padded-source ",
		"lab feed/2",
		"ümlaut?&=#",
	} {
		t.Run(source, func(t *testing.T) {
			want, err := fhirout.SourceIdentifierSystem(source)
			if err != nil {
				t.Fatalf("fhirout.SourceIdentifierSystem: %v", err)
			}
			mapper := fhir.NewUSCoreMapper()
			mapper.Source = source
			encounter := mapper.MapEncounter(&events.Encounter{ID: "V1"}, "")
			if got := encounter.Identifier[0].System; got != want {
				t.Fatalf("mapper mints %q, fhirout keys on %q", got, want)
			}
		})
	}
}
