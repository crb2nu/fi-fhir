package fhirout

import (
	"reflect"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

// eventSource returns the envelope Source of any canonical event, pointer or
// value, through the Meta method every event type inherits from its embedded
// events.EventMeta. A typed-nil pointer — which mapEvent's per-case nil checks
// turn into ErrInvalidPayload — and anything that is not an event yield "", so
// the mapper simply has no source rather than the caller panicking on a
// promoted value-receiver method.
func eventSource(event any) string {
	if value := reflect.ValueOf(event); value.Kind() == reflect.Pointer && value.IsNil() {
		return ""
	}
	if envelope, ok := event.(interface{ Meta() events.EventMeta }); ok {
		return envelope.Meta().Source
	}
	return ""
}
