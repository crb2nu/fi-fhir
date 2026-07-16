package session

import (
	"bytes"
	"context"
	"testing"
)

func TestAESGCMProtectorRoundTripAndTamper(t *testing.T) {
	protector, err := NewAESGCMProtector(bytes.Repeat([]byte{0x42}, 32))
	if err != nil {
		t.Fatalf("NewAESGCMProtector: %v", err)
	}
	plaintext := []byte("RAW-PHI-SENTINEL")
	aad := []byte("tenant-a\x00session-a\x00sample-a")
	protected, err := protector.Protect(context.Background(), plaintext, aad)
	if err != nil {
		t.Fatalf("Protect: %v", err)
	}
	if bytes.Contains(protected, plaintext) {
		t.Fatal("protected payload contains plaintext")
	}
	recovered, err := protector.Unprotect(context.Background(), protected, aad)
	if err != nil {
		t.Fatalf("Unprotect: %v", err)
	}
	if !bytes.Equal(recovered, plaintext) {
		t.Fatalf("recovered = %q, want %q", recovered, plaintext)
	}
	if _, err := protector.Unprotect(context.Background(), protected, []byte("another sample")); err == nil {
		t.Fatal("protected payload decrypted under another sample identity")
	}

	protected[len(protected)-1] ^= 0xff
	if _, err := protector.Unprotect(context.Background(), protected, aad); err == nil {
		t.Fatal("tampered protected payload unexpectedly decrypted")
	}
}

func TestAESGCMProtectorRequiresAES256Key(t *testing.T) {
	if _, err := NewAESGCMProtector(bytes.Repeat([]byte{0x42}, 16)); err == nil {
		t.Fatal("accepted non-AES-256 key")
	}
}
