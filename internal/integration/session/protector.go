package session

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
)

const protectedPayloadVersion byte = 1

// AESGCMProtector encrypts explicitly retained raw samples with a deployment
// key. The key is never stored in PostgreSQL or session exports.
type AESGCMProtector struct {
	aead cipher.AEAD
}

func NewAESGCMProtector(key []byte) (*AESGCMProtector, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("create retained-sample cipher: AES-256 key must contain exactly 32 bytes")
	}
	block, err := aes.NewCipher(append([]byte(nil), key...))
	if err != nil {
		return nil, fmt.Errorf("create retained-sample cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create retained-sample protector: %w", err)
	}
	return &AESGCMProtector{aead: aead}, nil
}

func (p *AESGCMProtector) Protect(ctx context.Context, plaintext, additionalData []byte) ([]byte, error) {
	if p == nil || p.aead == nil || ctx == nil {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	nonce := make([]byte, p.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate retained-sample nonce: %w", err)
	}
	out := make([]byte, 1, 1+len(nonce)+len(plaintext)+p.aead.Overhead())
	out[0] = protectedPayloadVersion
	out = append(out, nonce...)
	out = p.aead.Seal(out, nonce, plaintext, additionalData)
	return out, nil
}

func (p *AESGCMProtector) Unprotect(ctx context.Context, protected, additionalData []byte) ([]byte, error) {
	if p == nil || p.aead == nil || ctx == nil {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	nonceSize := p.aead.NonceSize()
	if len(protected) < 1+nonceSize+p.aead.Overhead() || protected[0] != protectedPayloadVersion {
		return nil, ErrImmutable
	}
	nonce := protected[1 : 1+nonceSize]
	plaintext, err := p.aead.Open(nil, nonce, protected[1+nonceSize:], additionalData)
	if err != nil {
		return nil, ErrImmutable
	}
	return plaintext, nil
}

var _ PayloadProtector = (*AESGCMProtector)(nil)
