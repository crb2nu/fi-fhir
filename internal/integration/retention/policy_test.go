package retention

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const validPolicyDocument = `{
  "tenant_id": "tenant-a",
  "canonical_event_retain": "8760h",
  "session_sample_retain": "720h",
  "session_export_retain": "2160h",
  "stream_event_retain": "168h",
  "authorized_by": {"id": "privacy-officer-1", "kind": "human", "auth_method": "oidc"},
  "reason": "HIPAA minimum-necessary retention approved 2026-08-08"
}`

func TestDecodePolicyDocumentReadsEveryWindowAndAttribution(t *testing.T) {
	policy, err := DecodePolicyDocument(strings.NewReader(validPolicyDocument), "tenant-a")
	if err != nil {
		t.Fatalf("DecodePolicyDocument: %v", err)
	}
	if policy.CanonicalEventRetain != 8760*time.Hour ||
		policy.SessionSampleRetain != 720*time.Hour ||
		policy.SessionExportRetain != 2160*time.Hour ||
		policy.StreamEventRetain != 168*time.Hour {
		t.Fatalf("windows = %+v", policy)
	}
	if policy.Principal.ID != "privacy-officer-1" || policy.Reason == "" {
		t.Fatalf("attribution = %+v", policy.Principal)
	}
	if !strings.HasPrefix(policy.DocumentDigest, "sha256:") || len(policy.DocumentDigest) != 71 {
		t.Fatalf("document digest = %q", policy.DocumentDigest)
	}
	if policy.PurgesNothing() {
		t.Fatal("a policy with four windows reported that it purges nothing")
	}
}

// The digest is what makes a restart with an unchanged document a non-event: it
// must not depend on anything but the bytes.
func TestDecodePolicyDocumentDigestIsStableAcrossReads(t *testing.T) {
	first, err := DecodePolicyDocument(strings.NewReader(validPolicyDocument), "tenant-a")
	if err != nil {
		t.Fatalf("first decode: %v", err)
	}
	second, err := DecodePolicyDocument(strings.NewReader(validPolicyDocument), "tenant-a")
	if err != nil {
		t.Fatalf("second decode: %v", err)
	}
	if first.DocumentDigest != second.DocumentDigest {
		t.Fatalf("digest changed between reads: %q vs %q", first.DocumentDigest, second.DocumentDigest)
	}
	changed, err := DecodePolicyDocument(
		strings.NewReader(strings.Replace(validPolicyDocument, "8760h", "4380h", 1)), "tenant-a")
	if err != nil {
		t.Fatalf("changed decode: %v", err)
	}
	if changed.DocumentDigest == first.DocumentDigest {
		t.Fatal("a shortened retention window produced the same digest, so it would mint no policy version")
	}
}

// An omitted window is retain-indefinitely, which is the fail-closed default the
// whole slice rests on. It must never be read as zero-means-purge-immediately.
func TestDecodePolicyDocumentTreatsOmittedWindowsAsRetainIndefinitely(t *testing.T) {
	policy, err := DecodePolicyDocument(strings.NewReader(`{
	  "tenant_id": "tenant-a",
	  "authorized_by": {"id": "privacy-officer-1", "kind": "human", "auth_method": "oidc"},
	  "reason": "retain everything until the privacy review lands"
	}`), "tenant-a")
	if err != nil {
		t.Fatalf("DecodePolicyDocument: %v", err)
	}
	if !policy.PurgesNothing() {
		t.Fatalf("a document with no windows authorized a purge: %+v", policy)
	}
}

func TestDecodePolicyDocumentRefusals(t *testing.T) {
	for name, document := range map[string]string{
		"other tenant": strings.Replace(validPolicyDocument, `"tenant-a"`, `"tenant-b"`, 1),
		"no reason":    strings.Replace(validPolicyDocument, `"HIPAA minimum-necessary retention approved 2026-08-08"`, `"  "`, 1),
		"no principal id": strings.Replace(validPolicyDocument,
			`"id": "privacy-officer-1"`, `"id": ""`, 1),
		"unknown principal kind": strings.Replace(validPolicyDocument, `"kind": "human"`, `"kind": "robot"`, 1),
		"negative window":        strings.Replace(validPolicyDocument, `"8760h"`, `"-1h"`, 1),
		"unparseable window":     strings.Replace(validPolicyDocument, `"8760h"`, `"one year"`, 1),
		"stream window below the schema floor": strings.Replace(validPolicyDocument,
			`"stream_event_retain": "168h"`, `"stream_event_retain": "1m"`, 1),
		"unknown field": strings.Replace(validPolicyDocument,
			`"tenant_id": "tenant-a",`, `"tenant_id": "tenant-a", "purge_everything": true,`, 1),
		"not json": "tenant-a",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodePolicyDocument(strings.NewReader(document), "tenant-a"); err == nil {
				t.Fatal("document was accepted")
			} else if !errors.Is(err, ErrInvalidPolicy) {
				t.Fatalf("err = %v, want ErrInvalidPolicy", err)
			}
		})
	}
}

func TestDecodePolicyDocumentRefusesAnOversizedDocument(t *testing.T) {
	padded := strings.Replace(validPolicyDocument,
		`"reason": "HIPAA`, `"reason": "`+strings.Repeat("x", maxPolicyDocumentBytes)+`HIPAA`, 1)
	if _, err := DecodePolicyDocument(strings.NewReader(padded), "tenant-a"); err == nil {
		t.Fatal("an oversized document was accepted")
	}
}
