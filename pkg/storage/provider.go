// Package storage provides a unified file access layer supporting local filesystem and S3/MinIO.
package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"
)

// FileInfo contains metadata about a file.
type FileInfo struct {
	Path         string    `json:"path"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
	ETag         string    `json:"etag,omitempty"`
	ContentType  string    `json:"content_type,omitempty"`
	IsDir        bool      `json:"is_dir"`
}

// Provider defines the interface for file storage operations.
// Implementations support local filesystem (file://) and S3-compatible storage (s3://).
type Provider interface {
	// Open returns a reader for the file at the given path.
	// Supports s3://bucket/key and file:///path/to/file URLs.
	Open(ctx context.Context, path string) (io.ReadCloser, error)

	// Stat returns metadata for the file without reading content.
	Stat(ctx context.Context, path string) (*FileInfo, error)

	// List returns files matching the prefix.
	// For S3, this lists objects with the given prefix.
	// For local filesystem, this lists files in the directory.
	List(ctx context.Context, prefix string) ([]FileInfo, error)

	// Put uploads content to the given path.
	// size should be -1 if unknown (will buffer the entire content).
	Put(ctx context.Context, path string, r io.Reader, size int64) error

	// Delete removes the file at the given path.
	Delete(ctx context.Context, path string) error

	// Exists checks if a file exists at the given path.
	Exists(ctx context.Context, path string) (bool, error)
}

// MultiProvider routes operations to the appropriate provider based on URL scheme.
type MultiProvider struct {
	local *LocalProvider
	minio *MinIOProvider
}

// NewMultiProvider creates a provider that routes to local or MinIO based on URL scheme.
// If minio is nil, S3 URLs will return an error.
func NewMultiProvider(local *LocalProvider, minio *MinIOProvider) *MultiProvider {
	return &MultiProvider{
		local: local,
		minio: minio,
	}
}

// Open opens a file from either local filesystem or MinIO based on URL scheme.
func (m *MultiProvider) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	provider, resolvedPath, err := m.resolve(path)
	if err != nil {
		return nil, err
	}
	return provider.Open(ctx, resolvedPath)
}

// Stat returns file metadata.
func (m *MultiProvider) Stat(ctx context.Context, path string) (*FileInfo, error) {
	provider, resolvedPath, err := m.resolve(path)
	if err != nil {
		return nil, err
	}
	return provider.Stat(ctx, resolvedPath)
}

// List returns files matching the prefix.
func (m *MultiProvider) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	provider, resolvedPath, err := m.resolve(prefix)
	if err != nil {
		return nil, err
	}
	return provider.List(ctx, resolvedPath)
}

// Put uploads content to the path.
func (m *MultiProvider) Put(ctx context.Context, path string, r io.Reader, size int64) error {
	provider, resolvedPath, err := m.resolve(path)
	if err != nil {
		return err
	}
	return provider.Put(ctx, resolvedPath, r, size)
}

// Delete removes a file.
func (m *MultiProvider) Delete(ctx context.Context, path string) error {
	provider, resolvedPath, err := m.resolve(path)
	if err != nil {
		return err
	}
	return provider.Delete(ctx, resolvedPath)
}

// Exists checks if a file exists.
func (m *MultiProvider) Exists(ctx context.Context, path string) (bool, error) {
	provider, resolvedPath, err := m.resolve(path)
	if err != nil {
		return false, err
	}
	return provider.Exists(ctx, resolvedPath)
}

// resolve determines which provider to use based on URL scheme.
func (m *MultiProvider) resolve(path string) (Provider, string, error) {
	// Parse as URL
	u, err := url.Parse(path)
	if err != nil || u.Scheme == "" {
		// Treat as local path
		if m.local == nil {
			return nil, "", fmt.Errorf("local provider not configured")
		}
		return m.local, path, nil
	}

	switch u.Scheme {
	case "file":
		if m.local == nil {
			return nil, "", fmt.Errorf("local provider not configured")
		}
		return m.local, u.Path, nil

	case "s3", "minio":
		if m.minio == nil {
			return nil, "", fmt.Errorf("MinIO provider not configured for s3:// URLs")
		}
		// s3://bucket/key -> bucket and key
		bucket := u.Host
		key := strings.TrimPrefix(u.Path, "/")
		return m.minio, bucket + "/" + key, nil

	default:
		return nil, "", fmt.Errorf("unsupported URL scheme: %s", u.Scheme)
	}
}

// ParseS3URL parses an s3:// URL into bucket and key.
func ParseS3URL(s3url string) (bucket, key string, err error) {
	u, err := url.Parse(s3url)
	if err != nil {
		return "", "", fmt.Errorf("invalid URL: %w", err)
	}

	if u.Scheme != "s3" && u.Scheme != "minio" {
		return "", "", fmt.Errorf("expected s3:// or minio:// URL, got %s://", u.Scheme)
	}

	bucket = u.Host
	key = strings.TrimPrefix(u.Path, "/")

	if bucket == "" {
		return "", "", fmt.Errorf("bucket name is required in s3:// URL")
	}

	return bucket, key, nil
}

// FormatS3URL creates an s3:// URL from bucket and key.
func FormatS3URL(bucket, key string) string {
	return fmt.Sprintf("s3://%s/%s", bucket, key)
}

// IsS3URL checks if a path is an S3 or MinIO URL.
func IsS3URL(path string) bool {
	return strings.HasPrefix(path, "s3://") || strings.HasPrefix(path, "minio://")
}
