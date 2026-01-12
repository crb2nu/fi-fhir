// Package sink provides data destination handlers for the ETL pipeline.
package sink

import (
	"context"
	"fmt"
	"io"

	"github.com/crb2nu/fi-fhir/pkg/storage"
)

// MinIOSink writes data to MinIO/S3 storage.
type MinIOSink struct {
	name     string
	provider *storage.MinIOProvider
	bucket   string
}

// MinIOSinkConfig configures the MinIO sink.
type MinIOSinkConfig struct {
	Name     string
	Endpoint string
	Bucket   string
	storage.MinIOConfig
}

// NewMinIOSink creates a new MinIO sink.
func NewMinIOSink(cfg MinIOSinkConfig) (*MinIOSink, error) {
	provider, err := storage.NewMinIOProvider(cfg.MinIOConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO provider: %w", err)
	}

	bucket := cfg.Bucket
	if bucket == "" {
		bucket = cfg.DefaultBucket
	}

	return &MinIOSink{
		name:     cfg.Name,
		provider: provider,
		bucket:   bucket,
	}, nil
}

// NewMinIOSinkFromProvider creates a sink from an existing provider.
func NewMinIOSinkFromProvider(name string, provider *storage.MinIOProvider, bucket string) *MinIOSink {
	return &MinIOSink{
		name:     name,
		provider: provider,
		bucket:   bucket,
	}
}

// Name returns the sink identifier.
func (s *MinIOSink) Name() string {
	return s.name
}

// Write uploads data to MinIO.
func (s *MinIOSink) Write(ctx context.Context, path string, r io.Reader, size int64) error {
	fullPath := s.bucket + "/" + path

	if err := s.provider.Put(ctx, fullPath, r, size); err != nil {
		return fmt.Errorf("failed to write to MinIO: %w", err)
	}

	return nil
}

// Exists checks if data exists at the path.
func (s *MinIOSink) Exists(ctx context.Context, path string) (bool, error) {
	fullPath := s.bucket + "/" + path
	return s.provider.Exists(ctx, fullPath)
}

// Validate checks if the MinIO connection is valid.
func (s *MinIOSink) Validate(ctx context.Context) error {
	// Check bucket exists
	exists, err := s.provider.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket: %w", err)
	}

	if !exists {
		// Try to create it
		if err := s.provider.CreateBucket(ctx, s.bucket); err != nil {
			return fmt.Errorf("bucket %s does not exist and could not be created: %w", s.bucket, err)
		}
	}

	return nil
}

// Delete removes a file from MinIO.
func (s *MinIOSink) Delete(ctx context.Context, path string) error {
	fullPath := s.bucket + "/" + path
	return s.provider.Delete(ctx, fullPath)
}

// List returns files with the given prefix.
func (s *MinIOSink) List(ctx context.Context, prefix string) ([]storage.FileInfo, error) {
	fullPath := s.bucket + "/" + prefix
	return s.provider.ListRecursive(ctx, fullPath)
}

// Provider returns the underlying storage provider.
func (s *MinIOSink) Provider() *storage.MinIOProvider {
	return s.provider
}

// Bucket returns the configured bucket name.
func (s *MinIOSink) Bucket() string {
	return s.bucket
}
