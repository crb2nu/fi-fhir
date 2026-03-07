package main

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/etl"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/storage"
)

type fakeMinioSink struct {
	name      string
	listFiles map[string][]storage.FileInfo
}

func (s *fakeMinioSink) Name() string { return s.name }

func (s *fakeMinioSink) Write(ctx context.Context, path string, r io.Reader, size int64) error {
	_, _ = io.Copy(io.Discard, r)
	return nil
}

func (s *fakeMinioSink) Exists(ctx context.Context, path string) (bool, error) { return false, nil }

func (s *fakeMinioSink) Validate(ctx context.Context) error { return nil }

func (s *fakeMinioSink) List(ctx context.Context, prefix string) ([]storage.FileInfo, error) {
	if s.listFiles == nil {
		return nil, nil
	}
	return s.listFiles[prefix], nil
}

type fakeSource struct {
	name string
}

func (s *fakeSource) Name() string { return s.name }

func (s *fakeSource) AvailableVersions(ctx context.Context) ([]etl.VersionInfo, error) {
	return []etl.VersionInfo{{Version: "v1", IsLatest: true}}, nil
}

func (s *fakeSource) Download(ctx context.Context, version string, w io.Writer) (int64, error) {
	n, err := io.Copy(w, bytes.NewBufferString("fake"))
	return n, err
}

func (s *fakeSource) Validate(ctx context.Context) error { return nil }

func TestETLFetch_DryRun_Offline(t *testing.T) {
	oldMinioSinkFactory := minioSinkFactory
	oldSourcesProvider := sourcesProvider
	t.Cleanup(func() {
		minioSinkFactory = oldMinioSinkFactory
		sourcesProvider = oldSourcesProvider
	})

	minioSinkFactory = func() (minioSink, error) {
		return &fakeMinioSink{name: "fake-minio"}, nil
	}
	sourcesProvider = func() map[string]etl.Source {
		return map[string]etl.Source{
			"icd10cm": &fakeSource{name: "icd10cm"},
		}
	}

	err := runETLFetch([]string{"icd10cm", "--version", "FY2024", "--dry-run"})
	assertNoError(t, err)
}

func TestETLSync_DryRun_AllSources_Offline(t *testing.T) {
	oldMinioSinkFactory := minioSinkFactory
	oldSourcesProvider := sourcesProvider
	t.Cleanup(func() {
		minioSinkFactory = oldMinioSinkFactory
		sourcesProvider = oldSourcesProvider
	})

	minioSinkFactory = func() (minioSink, error) {
		return &fakeMinioSink{name: "fake-minio"}, nil
	}
	sourcesProvider = func() map[string]etl.Source {
		return map[string]etl.Source{
			"icd10cm": &fakeSource{name: "icd10cm"},
			"ndc":     &fakeSource{name: "ndc"},
		}
	}

	err := runETLSync([]string{"--dry-run"})
	assertNoError(t, err)
}

func TestETLStatus_Offline_ListVersions(t *testing.T) {
	oldMinioSinkFactory := minioSinkFactory
	oldSourcesProvider := sourcesProvider
	t.Cleanup(func() {
		minioSinkFactory = oldMinioSinkFactory
		sourcesProvider = oldSourcesProvider
	})

	minioSinkFactory = func() (minioSink, error) {
		return &fakeMinioSink{
			name: "fake-minio",
			listFiles: map[string][]storage.FileInfo{
				"icd10cm/": {
					{Path: "icd10cm/FY2024/data", LastModified: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
					{Path: "icd10cm/FY2025/data", LastModified: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)},
				},
			},
		}, nil
	}
	sourcesProvider = func() map[string]etl.Source {
		return map[string]etl.Source{
			"icd10cm": &fakeSource{name: "icd10cm"},
		}
	}

	stdout, _ := captureOutput(t, func() {
		err := runETLStatus(nil)
		assertNoError(t, err)
	})

	assertContains(t, stdout, "icd10cm")
	assertContains(t, stdout, "FY2025")
	assertContains(t, stdout, "synced")
}

func TestETLLoad_UnknownSource(t *testing.T) {
	err := runETLLoad([]string{"nope"})
	assertError(t, err)
	assertErrorContains(t, err, "unknown source")
}

func TestETLLoad_MissingVersion(t *testing.T) {
	err := runETLLoad([]string{"umls"})
	assertError(t, err)
	assertErrorContains(t, err, "--version is required")
}

func TestETLLoad_DryRun_MissingDBEnv(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "")
	t.Setenv("DATABASE_URL", "")

	err := runETLLoad([]string{"umls", "--version", "2024AB", "--dry-run"})
	assertError(t, err)
	assertErrorContains(t, err, "FI_FHIR_DATABASE_URL")
}

func TestETLLoad_DryRun_FailsDBValidation(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://user:pass@localhost:5432/test?sslmode=disable")
	t.Setenv("MINIO_ACCESS_KEY", "fake")
	t.Setenv("MINIO_SECRET_KEY", "fake")
	t.Setenv("MINIO_ENDPOINT", "localhost:9000")

	// This should fail during pgSink.Validate(ctx) because PostgreSQL isn't running
	err := runETLLoad([]string{"umls", "--version", "2024AB", "--sabs", "SNOMEDCT_US", "--progress", "--dry-run"})
	assertError(t, err)
	assertErrorContains(t, err, "database validation failed")
}

func TestETLSync_SpecificSource_Offline(t *testing.T) {
	oldMinioSinkFactory := minioSinkFactory
	oldSourcesProvider := sourcesProvider
	t.Cleanup(func() {
		minioSinkFactory = oldMinioSinkFactory
		sourcesProvider = oldSourcesProvider
	})

	minioSinkFactory = func() (minioSink, error) {
		return &fakeMinioSink{name: "fake-minio"}, nil
	}
	sourcesProvider = func() map[string]etl.Source {
		return map[string]etl.Source{
			"icd10cm": &fakeSource{name: "icd10cm"},
		}
	}

	err := runETLSync([]string{"--source", "icd10cm", "--version", "FY2024", "--dry-run", "--overwrite"})
	assertNoError(t, err)
}

func TestETLSync_SpecificSource_Unknown_Offline(t *testing.T) {
	oldMinioSinkFactory := minioSinkFactory
	oldSourcesProvider := sourcesProvider
	t.Cleanup(func() {
		minioSinkFactory = oldMinioSinkFactory
		sourcesProvider = oldSourcesProvider
	})

	minioSinkFactory = func() (minioSink, error) {
		return &fakeMinioSink{name: "fake-minio"}, nil
	}
	sourcesProvider = func() map[string]etl.Source {
		return map[string]etl.Source{
			"icd10cm": &fakeSource{name: "icd10cm"},
		}
	}

	err := runETLSync([]string{"--source", "unknown"})
	assertError(t, err)
	assertErrorContains(t, err, "unknown source")
}

func TestETLSources_Offline(t *testing.T) {
	oldSourcesProvider := sourcesProvider
	t.Cleanup(func() {
		sourcesProvider = oldSourcesProvider
	})

	sourcesProvider = func() map[string]etl.Source {
		return map[string]etl.Source{
			"icd10cm": &fakeSource{name: "icd10cm"},
		}
	}

	stdout, _ := captureOutput(t, func() {
		err := runETLSources([]string{})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "NAME")
	assertContains(t, stdout, "icd10cm")
}

func TestETLValidate_Offline(t *testing.T) {
	t.Setenv("MINIO_ENDPOINT", "localhost")
	stdout, _ := captureOutput(t, func() {
		err := runETLValidate([]string{})
		assertNoError(t, err)
	})
	assertContains(t, stdout, "Validating ETL configuration")
	assertContains(t, stdout, "MinIO connectivity")
	assertContains(t, stdout, "Source availability")
}
