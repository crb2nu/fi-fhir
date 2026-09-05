//go:build integration && !structuredloggingleak

package observability_test

// structuredLoggingLeakTags is empty in a normal build, so the `fi-fhir` binary
// the kill-test drives is the shipped one.
//
// Its counterpart in structured_logging_tags_on_test.go propagates the
// `structuredloggingleak` tag to that build. Without this pair the negative
// control is inert: the tag would apply only to the test binary's own
// compilation, while `runQueueLogWorkflow` shells out to a separately built
// `fi-fhir` that never saw it — so the control would report the kill-test
// "still passes with the payload print restored" when the payload print was
// never actually restored in the process under test.
const structuredLoggingLeakTags = ""
