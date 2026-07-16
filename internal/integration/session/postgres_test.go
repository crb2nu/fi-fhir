package session

import (
	"errors"
	"testing"
	"time"
)

func TestDecodeArtifactUsesExactContentBytes(t *testing.T) {
	content := []byte(`{"hl7v2":{"timezone":"UTC","default_version":"2.5.1"}}`)
	record := ArtifactDraft{
		ID: "artifact-a", RevisionID: "revision-a", SessionID: "session-a",
		Kind: ArtifactKindMappingProfile, Name: "profile", Version: 1,
		Digest: recordDigest(content), CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC(),
	}
	raw, err := encodeRecord(record)
	if err != nil {
		t.Fatalf("encodeRecord: %v", err)
	}
	decoded, err := decodeArtifact(raw, content, record.SessionID)
	if err != nil {
		t.Fatalf("decodeArtifact: %v", err)
	}
	if string(decoded.Content) != string(content) || decoded.Digest != record.Digest {
		t.Fatalf("decoded artifact = %#v", decoded)
	}
	if _, err := decodeArtifact(raw, append(content, ' '), record.SessionID); !errors.Is(err, ErrImmutable) {
		t.Fatalf("mutated content error = %v, want ErrImmutable", err)
	}
}
