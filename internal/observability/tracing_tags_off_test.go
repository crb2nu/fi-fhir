//go:build integration && !tracingleak

package observability_test

// tracingLeakTags is empty in a normal build, so the `fi-fhir serve` the
// collector proof drives is the shipped one.
//
// Its counterpart in tracing_tags_on_test.go propagates the `tracingleak` tag to
// that build. Without the pair the control is inert for the reason
// structured_logging_tags_off_test.go records: the tag would apply to the test
// binary alone, while the proof shells out to a separately built `fi-fhir` that
// never saw it, and `make tracing-negative-control` would report that the span
// scan "still passes with the raw header restored" when nothing was restored in
// the process under test.
const tracingLeakTags = ""
