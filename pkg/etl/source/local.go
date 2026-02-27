package source

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/etl"
)

// LocalSource reads data from local filesystem.
type LocalSource struct {
	name     string
	basePath string
	versions map[string]string // version -> relative path
}

// LocalSourceConfig configures the local source.
type LocalSourceConfig struct {
	Name     string
	BasePath string
	Versions map[string]string // Version to relative path mapping
}

// NewLocalSource creates a new local filesystem source.
func NewLocalSource(cfg LocalSourceConfig) *LocalSource {
	return &LocalSource{
		name:     cfg.Name,
		basePath: cfg.BasePath,
		versions: cfg.Versions,
	}
}

// Name returns the source identifier.
func (s *LocalSource) Name() string {
	return s.name
}

// AvailableVersions returns configured versions.
func (s *LocalSource) AvailableVersions(ctx context.Context) ([]etl.VersionInfo, error) {
	versions := make([]etl.VersionInfo, 0, len(s.versions))

	for version, relPath := range s.versions {
		fullPath := filepath.Join(s.basePath, relPath)
		info, err := os.Stat(fullPath)

		vi := etl.VersionInfo{
			Version: version,
		}

		if err == nil {
			vi.FileSize = info.Size()
			vi.ReleaseDate = info.ModTime()
		}

		versions = append(versions, vi)
	}

	// Mark first as latest
	if len(versions) > 0 {
		versions[0].IsLatest = true
	}

	return versions, nil
}

// Download streams data from local file.
func (s *LocalSource) Download(ctx context.Context, version string, w io.Writer) (int64, error) {
	relPath, ok := s.versions[version]
	if !ok {
		return 0, fmt.Errorf("unknown version %q for source %s", version, s.name)
	}

	fullPath := filepath.Join(s.basePath, relPath)

	f, err := os.Open(fullPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open %s: %w", fullPath, err)
	}
	defer func() { _ = f.Close() }()

	// Check for context cancellation periodically
	done := ctx.Done()
	buf := make([]byte, 32*1024)
	var total int64

	for {
		select {
		case <-done:
			return total, ctx.Err()
		default:
		}

		n, err := f.Read(buf)
		if n > 0 {
			written, werr := w.Write(buf[:n])
			total += int64(written)
			if werr != nil {
				return total, fmt.Errorf("write error: %w", werr)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return total, fmt.Errorf("read error: %w", err)
		}
	}

	return total, nil
}

// Validate checks if the source paths exist.
func (s *LocalSource) Validate(ctx context.Context) error {
	for version, relPath := range s.versions {
		fullPath := filepath.Join(s.basePath, relPath)
		if _, err := os.Stat(fullPath); err != nil {
			if os.IsNotExist(err) {
				return fmt.Errorf("version %s path not found: %s", version, fullPath)
			}
			return fmt.Errorf("failed to stat %s: %w", fullPath, err)
		}
	}
	return nil
}

// AddVersion adds or updates a version path.
func (s *LocalSource) AddVersion(version, relPath string) {
	if s.versions == nil {
		s.versions = make(map[string]string)
	}
	s.versions[version] = relPath
}

// SetBasePath updates the base path.
func (s *LocalSource) SetBasePath(path string) {
	s.basePath = path
}
