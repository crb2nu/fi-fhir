//go:build integration && tracingleak

package observability_test

// tracingLeakTags propagates negative control (c)'s build tag to the `fi-fhir`
// binary the collector proof drives, so the subprocess compiles
// cmd/fi-fhir/serve_tracing_correlation_leak.go and the raw caller correlation
// header is genuinely restored on the span in the process under test. See
// tracing_tags_off_test.go.
const tracingLeakTags = "tracingleak"
