package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/storage"
)

func runStorage(args []string) error {
	if len(args) == 0 {
		printStorageUsage()
		return nil
	}

	switch args[0] {
	case "test":
		return runStorageTest(args[1:])
	case "ls", "list":
		return runStorageList(args[1:])
	case "get":
		return runStorageGet(args[1:])
	case "put":
		return runStoragePut(args[1:])
	case "rm", "delete":
		return runStorageDelete(args[1:])
	case "stat":
		return runStorageStat(args[1:])
	case "-h", "--help", "help":
		printStorageUsage()
		return nil
	default:
		return fmt.Errorf("unknown storage subcommand: %s", args[0])
	}
}

func printStorageUsage() {
	fmt.Println(`fi-fhir storage - Object Storage Management

Usage:
  fi-fhir storage <subcommand> [options]

Subcommands:
  test     Test connectivity to MinIO/S3 storage
  ls       List files in a bucket or directory
  get      Download a file from S3 to local filesystem
  put      Upload a file from local filesystem to S3
  rm       Delete a file from S3
  stat     Show metadata for a file

Environment Variables:
  MINIO_ENDPOINT     MinIO/S3 endpoint (default: localhost:9000)
  MINIO_ACCESS_KEY   Access key ID
  MINIO_SECRET_KEY   Secret access key
  MINIO_USE_SSL      Use HTTPS (default: false)
  MINIO_BUCKET       Default bucket name

Examples:
  # Test connectivity
  fi-fhir storage test

  # List files in a bucket
  fi-fhir storage ls s3://terminology/umls/

  # Download a file
  fi-fhir storage get s3://terminology/umls/2024AB/MRCONSO.RRF.gz /tmp/MRCONSO.RRF.gz

  # Upload a file
  fi-fhir storage put /path/to/file.rrf s3://terminology/umls/2024AB/file.rrf

  # Delete a file
  fi-fhir storage rm s3://terminology/test/file.rrf

  # Get file metadata
  fi-fhir storage stat s3://terminology/umls/2024AB/MRCONSO.RRF.gz`)
}

// getMinIOConfig creates MinIO config from environment variables.
func getMinIOConfig() storage.MinIOConfig {
	endpoint := os.Getenv("MINIO_ENDPOINT")
	if endpoint == "" {
		endpoint = "localhost:9000"
	}

	useSSL := os.Getenv("MINIO_USE_SSL") == "true"

	return storage.MinIOConfig{
		Endpoint:        endpoint,
		AccessKeyID:     os.Getenv("MINIO_ACCESS_KEY"),
		SecretAccessKey: os.Getenv("MINIO_SECRET_KEY"),
		UseSSL:          useSSL,
		DefaultBucket:   os.Getenv("MINIO_BUCKET"),
	}
}

// createMinIOProvider creates a MinIO provider from environment.
func createMinIOProvider() (*storage.MinIOProvider, error) {
	cfg := getMinIOConfig()
	if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("MINIO_ACCESS_KEY and MINIO_SECRET_KEY environment variables are required")
	}
	return storage.NewMinIOProvider(cfg)
}

func runStorageTest(args []string) error {
	cfg := getMinIOConfig()

	fmt.Printf("Testing MinIO connection...\n")
	fmt.Printf("  Endpoint:  %s\n", cfg.Endpoint)
	fmt.Printf("  SSL:       %v\n", cfg.UseSSL)
	fmt.Printf("  Bucket:    %s\n", cfg.DefaultBucket)

	if cfg.AccessKeyID == "" {
		return fmt.Errorf("MINIO_ACCESS_KEY not set")
	}

	provider, err := storage.NewMinIOProvider(cfg)
	if err != nil {
		return fmt.Errorf("failed to create MinIO client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Check if default bucket exists
	if cfg.DefaultBucket != "" {
		exists, err := provider.BucketExists(ctx, cfg.DefaultBucket)
		if err != nil {
			return fmt.Errorf("failed to check bucket: %w", err)
		}
		if exists {
			fmt.Printf("\n✓ Connected successfully. Bucket '%s' exists.\n", cfg.DefaultBucket)
		} else {
			fmt.Printf("\n✓ Connected successfully. Bucket '%s' does not exist (will be created on first upload).\n", cfg.DefaultBucket)
		}
	} else {
		fmt.Println("\n✓ Connected successfully.")
	}

	return nil
}

func runStorageList(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("path required: fi-fhir storage ls s3://bucket/prefix/")
	}

	path := args[0]
	if !storage.IsS3URL(path) {
		return fmt.Errorf("S3 URL required (s3://bucket/prefix/)")
	}

	provider, err := createMinIOProvider()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	files, err := provider.ListRecursive(ctx, path)
	if err != nil {
		return fmt.Errorf("failed to list: %w", err)
	}

	if len(files) == 0 {
		fmt.Println("(no files)")
		return nil
	}

	// Print in a table-like format
	fmt.Printf("%-60s %15s %s\n", "PATH", "SIZE", "MODIFIED")
	fmt.Println(strings.Repeat("-", 95))
	for _, f := range files {
		sizeStr := humanizeSize(f.Size)
		modStr := f.LastModified.Format("2006-01-02 15:04")
		fmt.Printf("%-60s %15s %s\n", f.Path, sizeStr, modStr)
	}

	return nil
}

func runStorageGet(args []string) (err error) {
	if len(args) < 2 {
		return fmt.Errorf("usage: fi-fhir storage get <s3://bucket/key> <local-path>")
	}

	s3Path := args[0]
	localPath := args[1]

	if !storage.IsS3URL(s3Path) {
		return fmt.Errorf("first argument must be S3 URL (s3://bucket/key)")
	}

	provider, err := createMinIOProvider()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	fmt.Printf("Downloading %s -> %s\n", s3Path, localPath)

	// Get object info first for progress
	info, err := provider.Stat(ctx, s3Path)
	if err != nil {
		return fmt.Errorf("failed to stat object: %w", err)
	}
	fmt.Printf("Size: %s\n", humanizeSize(info.Size))

	// Open source
	reader, err := provider.Open(ctx, s3Path)
	if err != nil {
		return fmt.Errorf("failed to open S3 object: %w", err)
	}
	defer func() {
		if cerr := reader.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	// Create destination
	file, err := os.Create(localPath) //nolint:gosec // G304: path from CLI args
	if err != nil {
		return fmt.Errorf("failed to create local file: %w", err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	// Copy with progress
	written, err := io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to copy: %w", err)
	}

	fmt.Printf("Downloaded %s\n", humanizeSize(written))
	return nil
}

func runStoragePut(args []string) (err error) {
	if len(args) < 2 {
		return fmt.Errorf("usage: fi-fhir storage put <local-path> <s3://bucket/key>")
	}

	localPath := args[0]
	s3Path := args[1]

	if !storage.IsS3URL(s3Path) {
		return fmt.Errorf("second argument must be S3 URL (s3://bucket/key)")
	}

	// Get local file info
	info, err := os.Stat(localPath)
	if err != nil {
		return fmt.Errorf("failed to stat local file: %w", err)
	}
	if info.IsDir() {
		return fmt.Errorf("cannot upload directory")
	}

	provider, err := createMinIOProvider()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	fmt.Printf("Uploading %s -> %s\n", localPath, s3Path)
	fmt.Printf("Size: %s\n", humanizeSize(info.Size()))

	// Open source
	file, err := os.Open(localPath) //nolint:gosec // G304: path from CLI args
	if err != nil {
		return fmt.Errorf("failed to open local file: %w", err)
	}
	defer func() {
		if cerr := file.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	// Upload
	err = provider.Put(ctx, s3Path, file, info.Size())
	if err != nil {
		return fmt.Errorf("failed to upload: %w", err)
	}

	fmt.Println("Upload complete")
	return nil
}

func runStorageDelete(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: fi-fhir storage rm <s3://bucket/key>")
	}

	s3Path := args[0]
	if !storage.IsS3URL(s3Path) {
		return fmt.Errorf("argument must be S3 URL (s3://bucket/key)")
	}

	provider, err := createMinIOProvider()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Check if exists first
	exists, err := provider.Exists(ctx, s3Path)
	if err != nil {
		return fmt.Errorf("failed to check object: %w", err)
	}
	if !exists {
		fmt.Printf("Object does not exist: %s\n", s3Path)
		return nil
	}

	err = provider.Delete(ctx, s3Path)
	if err != nil {
		return fmt.Errorf("failed to delete: %w", err)
	}

	fmt.Printf("Deleted: %s\n", s3Path)
	return nil
}

func runStorageStat(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: fi-fhir storage stat <s3://bucket/key>")
	}

	s3Path := args[0]
	if !storage.IsS3URL(s3Path) {
		return fmt.Errorf("argument must be S3 URL (s3://bucket/key)")
	}

	provider, err := createMinIOProvider()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	info, err := provider.Stat(ctx, s3Path)
	if err != nil {
		return fmt.Errorf("failed to stat: %w", err)
	}

	fmt.Printf("Path:          %s\n", info.Path)
	fmt.Printf("Size:          %s (%d bytes)\n", humanizeSize(info.Size), info.Size)
	fmt.Printf("Last Modified: %s\n", info.LastModified.Format(time.RFC3339))
	fmt.Printf("ETag:          %s\n", info.ETag)
	fmt.Printf("Content Type:  %s\n", info.ContentType)

	return nil
}

// humanizeSize converts bytes to human-readable format.
func humanizeSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}
