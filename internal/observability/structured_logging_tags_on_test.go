//go:build integration && structuredloggingleak

package observability_test

// structuredLoggingLeakTags propagates the negative control's build tag to the
// `fi-fhir` binary the kill-test drives, so the subprocess compiles
// queue_publish_leak.go and the payload print is genuinely restored in the
// process under test. See structured_logging_tags_off_test.go.
const structuredLoggingLeakTags = "structuredloggingleak"
