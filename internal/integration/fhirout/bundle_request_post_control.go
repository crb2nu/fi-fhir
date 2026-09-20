//go:build fhirpostbundle

package fhirout

import (
	"net/http"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
)

// entryRequest under the fhirpostbundle tag is the pre-4.1c-c shape
// pkg/fhir.CreateTransactionBundle emits: `POST <Type>`, which inserts on every
// delivery. It exists only so `make fhir-destination-negative-control` can
// prove that TestFHIRDestination_RedeliveryIsIdempotent is measuring the
// request method: with this builder the second delivery must produce a second
// Patient and the proof must fail on exactly that count. Never build the
// product with this tag.
func entryRequest(resource Resource) *fhir.BundleEntryRequest {
	return &fhir.BundleEntryRequest{
		Method: http.MethodPost,
		URL:    resource.Type,
	}
}
