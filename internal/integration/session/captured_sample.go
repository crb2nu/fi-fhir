package session

import "strings"

// How the stores keep a captured or peeked sample (.loom/38 Lane C-2): the
// caller-named ID that makes a retried slot write idempotent, and the two
// rules that keep captured text out of places it should not be — sealed at
// rest when a retention key exists, and never copied into an export snapshot.

// maxSampleIDBytes bounds a caller-named sample ID.
const maxSampleIDBytes = 128

// validSampleID accepts the IDs AddSample generates ("sample_" and hex) and a
// caller-derived ID of the same shape: "sample_" and then ASCII letters,
// digits, '-', and '_'.
func validSampleID(id string) bool {
	rest, ok := strings.CutPrefix(id, "sample_")
	if !ok || rest == "" || len(id) > maxSampleIDBytes {
		return false
	}
	for _, character := range rest {
		switch {
		case character >= 'a' && character <= 'z', character >= 'A' && character <= 'Z',
			character >= '0' && character <= '9', character == '-', character == '_':
		default:
			return false
		}
	}
	return true
}

// sealsCapturedText reports whether a sample's stored text is sealed with the
// retention protector: captured text is, whenever the store has a protector,
// exactly as retained raw is. Without one it is stored as pasted samples are.
func sealsCapturedText(policy PHIPolicy, redaction SampleRedaction, protector PayloadProtector) bool {
	return policy == PHIPolicyRedact && redaction == SampleRedactionCapture && protector != nil
}

// strippedFromExport reports whether a sample's text is left out of an export
// snapshot. Retained raw always is; so is captured text: the snapshot is
// stored in plaintext and outlives the session's retention key, so copying
// captured text into it would undo the seal it has at rest.
func strippedFromExport(sample Sample) bool {
	return sample.PHIPolicy == PHIPolicyRetain || sample.Redaction == SampleRedactionCapture
}
