//go:build tracingleak

package main

// spanCorrelationID restores the pre-!279 posture: the caller's X-Correlation-ID
// copied verbatim onto the serve span and its log line.
//
// This file exists ONLY to be negative control (c) for
// `TestStructuredLogging_CorrelatedAndPHIFree`. It is compiled out of every
// shipped build — no release, image, or CI job sets `tracingleak` — and
// `make tracing-negative-control` inverts the exit status of a tagged run of the
// serve subtest, so a zero exit there means the collector-side PHI assertion has
// stopped measuring anything.
//
// The idiom is `structuredloggingleak`'s (internal/workflow/queue_publish_leak.go):
// a sentinel scan is only trusted once a build that leaks the sentinel has been
// seen to fail it.
//
// Do not "fix" this file. Its whole job is to leak.
func spanCorrelationID(raw string) string { return raw }
