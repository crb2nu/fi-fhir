//go:build !fhirpostbundle

package fhirout

import (
	"net/http"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/fhir"
)

// entryRequest is the conditional update every bundle entry carries:
// `PUT <Type>?identifier=<system>|<value>`. A server that holds a resource
// with that identifier updates it in place; one that does not creates it. The
// same bundle delivered twice therefore leaves one resource, which is what
// makes the at-least-once outbox safe against a FHIR destination.
//
// The `fhirpostbundle` build tag swaps this for the pre-4.1c-c `POST <Type>`
// builder (bundle_request_post_control.go) so the idempotency proof's negative
// control can require the duplicate to come back.
func entryRequest(resource Resource) *fhir.BundleEntryRequest {
	return &fhir.BundleEntryRequest{
		Method: http.MethodPut,
		URL:    ConditionalReference(resource.Type, resource.Key),
	}
}
