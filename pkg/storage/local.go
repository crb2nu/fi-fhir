package storage

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalProvider implements Provider for local filesystem operations.
type LocalProvider struct {
	// BasePath is an optional prefix for all paths.
	// If set, paths are resolved relative to this directory.
	BasePath string
}

// NewLocalProvider creates a local filesystem provider.
// basePath is optional - if empty, paths are used as-is.
func NewLocalProvider(basePath string) *LocalProvider {
	return &LocalProvider{
		BasePath: basePath,
	}
}

// resolvePath converts a path to an absolute path, optionally relative to BasePath.
func (l *LocalProvider) resolvePath(path string) string {
	if l.BasePath == "" {
		return path
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(l.BasePath, path)
}

// Open opens a file for reading.
// Automatically handles .gz files with transparent decompression.
func (l *LocalProvider) Open(ctx context.Context, path string) (io.ReadCloser, error) {
	resolved := l.resolvePath(path)

	f, err := os.Open(resolved) //nolint:gosec // G304: path from caller
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	// Check for gzip extension - transparent decompression
	if strings.HasSuffix(resolved, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			_ = f.Close() // Ignore close error on already-failed path
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		return &gzipReadCloser{gz: gz, f: f}, nil
	}

	return f, nil
}

// Stat returns file metadata.
func (l *LocalProvider) Stat(ctx context.Context, path string) (*FileInfo, error) {
	resolved := l.resolvePath(path)

	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s", path)
		}
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	return &FileInfo{
		Path:         path,
		Size:         info.Size(),
		LastModified: info.ModTime(),
		IsDir:        info.IsDir(),
	}, nil
}

// List returns files in a directory.
func (l *LocalProvider) List(ctx context.Context, prefix string) ([]FileInfo, error) {
	resolved := l.resolvePath(prefix)

	// Check if it's a file or directory
	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Empty list for non-existent paths
		}
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	if !info.IsDir() {
		// Single file
		return []FileInfo{{
			Path:         prefix,
			Size:         info.Size(),
			LastModified: info.ModTime(),
			IsDir:        false,
		}}, nil
	}

	// List directory contents
	entries, err := os.ReadDir(resolved)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var files []FileInfo
	for _, entry := range entries {
		entryInfo, err := entry.Info()
		if err != nil {
			continue // Skip entries we can't read
		}

		files = append(files, FileInfo{
			Path:         filepath.Join(prefix, entry.Name()),
			Size:         entryInfo.Size(),
			LastModified: entryInfo.ModTime(),
			IsDir:        entry.IsDir(),
		})
	}

	return files, nil
}

// ListRecursive returns all files under the given path recursively.
func (l *LocalProvider) ListRecursive(ctx context.Context, prefix string) ([]FileInfo, error) {
	resolved := l.resolvePath(prefix)

	// Check if it's a file or directory
	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Empty list for non-existent paths
		}
		return nil, fmt.Errorf("failed to stat path: %w", err)
	}

	if !info.IsDir() {
		// Single file
		return []FileInfo{{
			Path:         prefix,
			Size:         info.Size(),
			LastModified: info.ModTime(),
			IsDir:        false,
		}}, nil
	}

	// Walk directory tree
	var files []FileInfo
	err = filepath.WalkDir(resolved, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if path == resolved {
			return nil
		}

		entryInfo, err := d.Info()
		if err != nil {
			return nil // Skip entries we can't read
		}

		// Calculate relative path from prefix
		relPath, err := filepath.Rel(resolved, path)
		if err != nil {
			return nil
		}

		files = append(files, FileInfo{
			Path:         filepath.Join(prefix, relPath),
			Size:         entryInfo.Size(),
			LastModified: entryInfo.ModTime(),
			IsDir:        d.IsDir(),
		})

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return files, nil
}

// Put writes content to a file.
// Creates parent directories if they don't exist.
func (l *LocalProvider) Put(ctx context.Context, path string, r io.Reader, size int64) (err error) {
	resolved := l.resolvePath(path)

	// Create parent directories
	dir := filepath.Dir(resolved)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	// Create file
	f, err := os.Create(resolved) //nolint:gosec // G304: path from caller
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()

	// Check if we should gzip
	if strings.HasSuffix(resolved, ".gz") {
		gz := gzip.NewWriter(f)
		defer func() {
			if cerr := gz.Close(); cerr != nil && err == nil {
				err = cerr
			}
		}()
		if _, err := io.Copy(gz, r); err != nil {
			return fmt.Errorf("failed to write gzip content: %w", err)
		}
		return nil
	}

	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("failed to write file content: %w", err)
	}

	return nil
}

// Delete removes a file.
func (l *LocalProvider) Delete(ctx context.Context, path string) error {
	resolved := l.resolvePath(path)

	if err := os.Remove(resolved); err != nil {
		if os.IsNotExist(err) {
			return nil // Already deleted
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

// Exists checks if a file exists.
func (l *LocalProvider) Exists(ctx context.Context, path string) (bool, error) {
	resolved := l.resolvePath(path)

	_, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check file existence: %w", err)
	}

	return true, nil
}

// gzipReadCloser wraps a gzip reader to close both the gzip and underlying file.
type gzipReadCloser struct {
	gz *gzip.Reader
	f  *os.File
}

func (g *gzipReadCloser) Read(p []byte) (int, error) {
	return g.gz.Read(p)
}

func (g *gzipReadCloser) Close() error {
	gzErr := g.gz.Close()
	fErr := g.f.Close()
	if gzErr != nil {
		return gzErr
	}
	return fErr
}
