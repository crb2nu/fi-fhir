package sink

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// LocalSink writes data to local filesystem.
type LocalSink struct {
	name     string
	basePath string
}

// NewLocalSink creates a new local filesystem sink.
func NewLocalSink(name, basePath string) *LocalSink {
	return &LocalSink{
		name:     name,
		basePath: basePath,
	}
}

// Name returns the sink identifier.
func (s *LocalSink) Name() string {
	return s.name
}

// Write saves data to a local file.
func (s *LocalSink) Write(ctx context.Context, path string, r io.Reader, size int64) (err error) {
	fullPath := filepath.Join(s.basePath, path)

	// Create parent directories
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Create file
	f, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file %s: %w", fullPath, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	// Copy data with context checking
	done := ctx.Done()
	buf := make([]byte, 32*1024)

	for {
		select {
		case <-done:
			return ctx.Err()
		default:
		}

		n, rerr := r.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return fmt.Errorf("write error: %w", werr)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("read error: %w", rerr)
		}
	}

	return nil
}

// Exists checks if a file exists.
func (s *LocalSink) Exists(ctx context.Context, path string) (bool, error) {
	fullPath := filepath.Join(s.basePath, path)
	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// Validate checks if the base path is accessible.
func (s *LocalSink) Validate(ctx context.Context) error {
	// Create base path if it doesn't exist
	if err := os.MkdirAll(s.basePath, 0o750); err != nil {
		return fmt.Errorf("failed to create base path %s: %w", s.basePath, err)
	}

	// Check we can write
	testFile := filepath.Join(s.basePath, ".etl-test")
	f, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("failed to write test file: %w", err)
	}
	_ = f.Close()
	_ = os.Remove(testFile)

	return nil
}

// Delete removes a file.
func (s *LocalSink) Delete(ctx context.Context, path string) error {
	fullPath := filepath.Join(s.basePath, path)
	err := os.Remove(fullPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// BasePath returns the configured base path.
func (s *LocalSink) BasePath() string {
	return s.basePath
}
