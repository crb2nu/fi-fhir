//go:build !tracingleak

package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// correlationDigestBytes is the keyed digest's length: 128 bits of the HMAC.
const correlationDigestBytes = 16

// spanCorrelationID renders a caller's X-Correlation-ID for telemetry.
//
// The header is arbitrary caller text — a sender may put an MSH-10, a sequential
// request number, or an account identifier in it — so the serve span and its log
// line carry a digest of it: enough to join every stage that saw the same header,
// nothing of the text.
//
// With a deployment key the digest is `hmac-sha256:` and the first 128 bits of
// HMAC-SHA256(key, header). Without the key, anyone holding the collector or the
// log archive could precompute digests of the low-entropy spaces callers put in
// that header; with it they also need the serve secret. 128 bits keeps accidental
// collisions negligible while halving the indexed value, and the prefix tells a
// reader keyed digests from the unkeyed `sha256:` form.
//
// A nil key keeps the unkeyed `sha256:` digest; serve warns at startup when it
// runs that way (loadTelemetryCorrelationKey).
//
// serve_tracing_correlation_leak.go is the build-tagged twin that restores the
// raw value. It exists only as negative control (c) for
// TestStructuredLogging_CorrelatedAndPHIFree.
func spanCorrelationID(key []byte, raw string) string {
	if key == nil {
		return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(raw)))
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(raw))
	return "hmac-sha256:" + hex.EncodeToString(mac.Sum(nil)[:correlationDigestBytes])
}
