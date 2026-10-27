//go:build !tracingleak

package main

import (
	"crypto/sha256"
	"fmt"
)

// spanCorrelationID renders a caller's X-Correlation-ID for telemetry.
//
// The header is arbitrary caller text — a sender may put an MSH-10 or a patient
// identifier in it — so the serve span and its log line carry a SHA-256 of it:
// enough to join every stage that saw the same header, nothing of the text.
// (An unkeyed hash of low-entropy text is reversible by dictionary; moving to an
// HMAC under a deployment key is tracked as a follow-up to !279.)
//
// serve_tracing_correlation_leak.go is the build-tagged twin that restores the
// raw value. It exists only as negative control (c) for
// TestStructuredLogging_CorrelatedAndPHIFree.
func spanCorrelationID(raw string) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(raw)))
}
