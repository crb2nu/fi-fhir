package integration

import (
	"encoding/json"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// DecodeCanonicalEventPayloadForTest exposes the sealed canonical decoder to
// this package's external tests.
//
// Slice 4.1c-c's day-1 kill-test has to prove, on unmodified main, that the
// decoder reverses NewProcessedEvent's projection into the exact concrete
// pkg/events type the FHIR mapper consumes — before the slice exports a thin
// wrapper for the delivery engine. Until that wrapper lands, the test binary is
// the only caller outside this package, and construction stays sealed.
func DecodeCanonicalEventPayloadForTest(eventType events.EventType, payload json.RawMessage) (any, error) {
	return decodeCanonicalEventPayload(eventType, payload)
}
