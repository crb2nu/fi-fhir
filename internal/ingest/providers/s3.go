package providers

import (
	"context"
	"fmt"
	"io"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/storage"
)

// S3Provider implements the ingest.Provider interface for AWS S3 / MinIO.
type S3Provider struct {
	config Config
	store  *storage.MinIOProvider
	prefix string // Directory prefix to poll within the bucket
}

// NewS3Provider initializes a new S3 ingest provider.
func NewS3Provider(cfg Config, minioCfg storage.MinIOConfig, prefix string) (*S3Provider, error) {
	store, err := storage.NewMinIOProvider(minioCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize s3 storage provider: %w", err)
	}

	return &S3Provider{
		config: cfg,
		store:  store,
		prefix: prefix,
	}, nil
}

// Type returns the provider identifier.
func (s *S3Provider) Type() string {
	return "s3"
}

// ListFiles retrieves a list of available files to process based on the configured prefix.
func (s *S3Provider) ListFiles(ctx context.Context) ([]FileInfo, error) {
	// Use List (non-recursive) or ListRecursive based on typical ingest patterns.
	// For ingest drops, a flat directory is standard.
	objects, err := s.store.List(ctx, s.prefix)
	if err != nil {
		return nil, fmt.Errorf("s3 list error: %w", err)
	}

	var files []FileInfo
	for _, obj := range objects {
		if obj.IsDir {
			continue // Skip directories
		}

		files = append(files, FileInfo{
			Provider:   s.Type(),
			Path:       obj.Path,
			Size:       obj.Size,
			ModifiedAt: obj.LastModified,
		})

		if len(files) >= s.config.MaxBatchSize && s.config.MaxBatchSize > 0 {
			break
		}
	}

	return files, nil
}

// DownloadFile streams a specific file from S3.
func (s *S3Provider) DownloadFile(ctx context.Context, filePath string) (io.ReadCloser, error) {
	rc, err := s.store.Open(ctx, filePath)
	if err != nil {
		return nil, fmt.Errorf("s3 download error: %w", err)
	}
	return rc, nil
}

// Ack acknowledges that the file was successfully processed.
// Depending on config, it deletes the file or moves it to an archive path.
func (s *S3Provider) Ack(ctx context.Context, filePath string) error {
	if s.config.DeleteOnSuccess {
		return s.store.Delete(ctx, filePath)
	}

	if s.config.ArchivePath != "" {
		// Moving in S3 is a Copy + Delete.
		// For now, MinIOProvider doesn't expose a native CopyObject, so we'd have to stream it
		// or expose Copy on MinIOProvider.
		// For MVP, we will just delete if DeleteOnSuccess is true, or leave it if false.
		// A full archive implementation would require expanding `storage.MinIOProvider`.
		return fmt.Errorf("s3 archiving not yet implemented in storage package, use DeleteOnSuccess")
	}

	return nil
}

// Nack reports that the file failed processing.
// Typically moves it to an error or dead-letter prefix.
func (s *S3Provider) Nack(ctx context.Context, filePath string) error {
	// Similar to Archive, moving objects requires Copy.
	// For MVP, we leave the file in place so it gets picked up again (or rely on Temporal retries).
	// Advanced DLQ routing would happen at the workflow layer.
	return nil
}

// Close shuts down any persistent connections (no-op for S3 as HTTP client pools handle it).
func (s *S3Provider) Close() error {
	return nil
}
