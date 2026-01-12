package storage

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOConfig holds configuration for MinIO/S3 connection.
type MinIOConfig struct {
	Endpoint        string `json:"endpoint" yaml:"endpoint"`
	AccessKeyID     string `json:"access_key_id" yaml:"access_key_id"`
	SecretAccessKey string `json:"secret_access_key" yaml:"secret_access_key"`
	UseSSL          bool   `json:"use_ssl" yaml:"use_ssl"`
	Region          string `json:"region" yaml:"region"`
	DefaultBucket   string `json:"default_bucket" yaml:"default_bucket"`
}

// MinIOProvider implements Provider for S3-compatible object storage.
type MinIOProvider struct {
	client        *minio.Client
	defaultBucket string
}

// NewMinIOProvider creates a MinIO/S3 storage provider.
func NewMinIOProvider(cfg MinIOConfig) (*MinIOProvider, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MinIO client: %w", err)
	}

	return &MinIOProvider{
		client:        client,
		defaultBucket: cfg.DefaultBucket,
	}, nil
}

// NewMinIOProviderFromClient creates a provider from an existing MinIO client.
// Useful for testing with mock clients.
func NewMinIOProviderFromClient(client *minio.Client, defaultBucket string) *MinIOProvider {
	return &MinIOProvider{
		client:        client,
		defaultBucket: defaultBucket,
	}
}

// parsePath extracts bucket and key from a path.
// Accepts "bucket/key" or just "key" (uses default bucket).
func (m *MinIOProvider) parsePath(path string) (bucket, key string) {
	// Strip s3:// or minio:// prefix if present
	path = strings.TrimPrefix(path, "s3://")
	path = strings.TrimPrefix(path, "minio://")

	// Check if path contains bucket
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 2 {
		// If the first part is a known bucket name, use it as bucket
		// This handles explicit s3://bucket/key URLs
		if parts[0] == m.defaultBucket || m.defaultBucket == "" {
			return parts[0], parts[1]
		}
		// Otherwise, user provided a relative path - use default bucket
		return m.defaultBucket, path
	}

	// Single component - use default bucket if available
	if m.defaultBucket != "" {
		return m.defaultBucket, path
	}

	// No default bucket, treat single component as bucket with empty key
	return path, ""
}

// Open opens an object for reading.
// Path format: "bucket/key" or "key" (uses default bucket).
// Automatically decompresses .gz files.
func (m *MinIOProvider) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	bucket, key := m.parsePath(path)
	if bucket == "" {
		return nil, fmt.Errorf("bucket name required")
	}
	if key == "" {
		return nil, fmt.Errorf("object key required")
	}

	obj, err := m.client.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get object s3://%s/%s: %w", bucket, key, err)
	}

	// Check for gzip extension - transparent decompression
	if strings.HasSuffix(key, ".gz") {
		gz, err := gzip.NewReader(obj)
		if err != nil {
			_ = obj.Close() // Ignore close error on already-failed path
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		return &minioGzipReadCloser{gz: gz, obj: obj}, nil
	}

	return obj, nil
}

// Stat returns object metadata.
func (m *MinIOProvider) Stat(ctx context.Context, path string) (*FileInfo, error) {
	bucket, key := m.parsePath(path)
	if bucket == "" {
		return nil, fmt.Errorf("bucket name required")
	}
	if key == "" {
		return nil, fmt.Errorf("object key required")
	}

	info, err := m.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return nil, fmt.Errorf("object not found: s3://%s/%s", bucket, key)
		}
		return nil, fmt.Errorf("failed to stat object: %w", err)
	}

	return &FileInfo{
		Path:         path,
		Size:         info.Size,
		LastModified: info.LastModified,
		ETag:         info.ETag,
		ContentType:  info.ContentType,
		IsDir:        false, // S3 doesn't have real directories
	}, nil
}

// List returns objects with the given prefix.
func (m *MinIOProvider) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	bucket, keyPrefix := m.parsePath(prefix)
	if bucket == "" {
		return nil, fmt.Errorf("bucket name required")
	}

	var files []FileInfo

	// List objects
	for obj := range m.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    keyPrefix,
		Recursive: false, // Only immediate children
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list error: %w", obj.Err)
		}

		// Check if it's a "directory" (common prefix)
		isDir := strings.HasSuffix(obj.Key, "/")

		files = append(files, FileInfo{
			Path:         bucket + "/" + obj.Key,
			Size:         obj.Size,
			LastModified: obj.LastModified,
			ETag:         obj.ETag,
			ContentType:  obj.ContentType,
			IsDir:        isDir,
		})
	}

	return files, nil
}

// ListRecursive returns all objects with the given prefix recursively.
func (m *MinIOProvider) ListRecursive(ctx context.Context, prefix string) ([]FileInfo, error) {
	bucket, keyPrefix := m.parsePath(prefix)
	if bucket == "" {
		return nil, fmt.Errorf("bucket name required")
	}

	var files []FileInfo

	for obj := range m.client.ListObjects(ctx, bucket, minio.ListObjectsOptions{
		Prefix:    keyPrefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("list error: %w", obj.Err)
		}

		files = append(files, FileInfo{
			Path:         bucket + "/" + obj.Key,
			Size:         obj.Size,
			LastModified: obj.LastModified,
			ETag:         obj.ETag,
			ContentType:  obj.ContentType,
			IsDir:        strings.HasSuffix(obj.Key, "/"),
		})
	}

	return files, nil
}

// Put uploads content to an object.
// size should be -1 if unknown.
func (m *MinIOProvider) Put(ctx context.Context, path string, r io.Reader, size int64) error {
	bucket, key := m.parsePath(path)
	if bucket == "" {
		return fmt.Errorf("bucket name required")
	}
	if key == "" {
		return fmt.Errorf("object key required")
	}

	// Ensure bucket exists
	exists, err := m.client.BucketExists(ctx, bucket)
	if err != nil {
		return fmt.Errorf("failed to check bucket existence: %w", err)
	}
	if !exists {
		if err := m.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
			return fmt.Errorf("failed to create bucket %s: %w", bucket, err)
		}
	}

	// Determine content type
	contentType := "application/octet-stream"
	if strings.HasSuffix(key, ".json") {
		contentType = "application/json"
	} else if strings.HasSuffix(key, ".csv") {
		contentType = "text/csv"
	} else if strings.HasSuffix(key, ".gz") {
		contentType = "application/gzip"
	} else if strings.HasSuffix(key, ".rrf") {
		contentType = "text/plain"
	}

	_, err = m.client.PutObject(ctx, bucket, key, r, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	if err != nil {
		return fmt.Errorf("failed to upload object s3://%s/%s: %w", bucket, key, err)
	}

	return nil
}

// Delete removes an object.
func (m *MinIOProvider) Delete(ctx context.Context, path string) error {
	bucket, key := m.parsePath(path)
	if bucket == "" {
		return fmt.Errorf("bucket name required")
	}
	if key == "" {
		return fmt.Errorf("object key required")
	}

	err := m.client.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete object: %w", err)
	}

	return nil
}

// Exists checks if an object exists.
func (m *MinIOProvider) Exists(ctx context.Context, path string) (bool, error) {
	bucket, key := m.parsePath(path)
	if bucket == "" {
		return false, fmt.Errorf("bucket name required")
	}
	if key == "" {
		return false, fmt.Errorf("object key required")
	}

	_, err := m.client.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" {
			return false, nil
		}
		return false, fmt.Errorf("failed to check object existence: %w", err)
	}

	return true, nil
}

// BucketExists checks if a bucket exists.
func (m *MinIOProvider) BucketExists(ctx context.Context, bucket string) (bool, error) {
	return m.client.BucketExists(ctx, bucket)
}

// CreateBucket creates a new bucket.
func (m *MinIOProvider) CreateBucket(ctx context.Context, bucket string) error {
	return m.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{})
}

// Client returns the underlying MinIO client for advanced operations.
func (m *MinIOProvider) Client() *minio.Client {
	return m.client
}

// minioGzipReadCloser wraps a gzip reader to close both the gzip and underlying object.
type minioGzipReadCloser struct {
	gz  *gzip.Reader
	obj *minio.Object
}

func (g *minioGzipReadCloser) Read(p []byte) (int, error) {
	return g.gz.Read(p)
}

func (g *minioGzipReadCloser) Close() error {
	gzErr := g.gz.Close()
	objErr := g.obj.Close()
	if gzErr != nil {
		return gzErr
	}
	return objErr
}
