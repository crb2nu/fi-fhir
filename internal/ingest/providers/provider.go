package providers

import (
	"context"
	"io"
	"time"
)

// FileInfo represents a file discovered on a remote provider.
type FileInfo struct {
	// Provider is the identifier for the specific source (e.g., "s3", "sftp").
	Provider string
	// Path is the full path or object key.
	Path string
	// Size is the file size in bytes.
	Size int64
	// ModifiedAt is the last modification time.
	ModifiedAt time.Time
}

// Config represents the base configuration for any polling provider.
type Config struct {
	// PollingInterval specifies how often the provider should be checked.
	PollingInterval time.Duration
	// MaxBatchSize dictates how many files to process in a single poll tick.
	MaxBatchSize int
	// DeleteOnSuccess indicates if the remote file should be deleted after processing.
	DeleteOnSuccess bool
	// ArchivePath indicates where a file should be moved on the remote system after processing (if not deleted).
	ArchivePath string
}

// Provider defines the interface that all external batch sources (S3, SFTP, etc.) must implement.
type Provider interface {
	// Type returns the string identifier of the provider (e.g., "s3", "sftp").
	Type() string

	// ListFiles retrieves a list of available files to process based on internal criteria (e.g. prefix, suffix, age).
	ListFiles(ctx context.Context) ([]FileInfo, error)

	// DownloadFile streams a specific file from the remote provider.
	DownloadFile(ctx context.Context, path string) (io.ReadCloser, error)

	// Ack acknowledges that the file was successfully processed (usually deletes or moves it).
	Ack(ctx context.Context, path string) error

	// Nack reports that the file failed processing, potentially moving it to an error directory.
	Nack(ctx context.Context, path string) error

	// Close shuts down any persistent connections (like SSH clients) the provider maintains.
	Close() error
}
