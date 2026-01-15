package main

import (
	"os"
	"path/filepath"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/etl"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/etl/source"
)

func TestETL_Fetch_UnknownSource(t *testing.T) {
	origFactory := minioSinkFactory
	origSources := sourcesProvider
	minioSinkFactory = func() (minioSink, error) { return &stubSink{}, nil }
	sourcesProvider = func() map[string]etl.Source { return map[string]etl.Source{} }
	defer func() {
		minioSinkFactory = origFactory
		sourcesProvider = origSources
	}()

	_, _, err := runCLI(t, "etl", "fetch", "does-not-exist")
	assertError(t, err)
	assertErrorContains(t, err, "unknown source")
}

func TestETL_Fetch_UsesStubs(t *testing.T) {
	origFactory := minioSinkFactory
	origSources := sourcesProvider
	src := &stubSource{
		name: "stubsrc",
		versions: []etl.VersionInfo{
			{Version: "v1", IsLatest: true},
		},
		payload: []byte("payload"),
	}
	minioSinkFactory = func() (minioSink, error) { return &stubSink{}, nil }
	sourcesProvider = func() map[string]etl.Source { return map[string]etl.Source{"stubsrc": src} }
	defer func() {
		minioSinkFactory = origFactory
		sourcesProvider = origSources
	}()

	stdout, _, err := runCLI(t, "etl", "fetch", "stubsrc", "--dry-run")
	assertNoError(t, err)
	assertContains(t, stdout, "Syncing stubsrc")
}

func TestETL_Validate_UsesSourcesProvider(t *testing.T) {
	origFactory := minioSinkFactory
	origSources := sourcesProvider
	tmpDir := t.TempDir()
	dataPath := filepath.Join(tmpDir, "data.txt")
	if err := os.WriteFile(dataPath, []byte("x"), 0600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	local := source.NewLocalSource(source.LocalSourceConfig{
		Name:     "local",
		BasePath: tmpDir,
		Versions: map[string]string{"v1": "data.txt"},
	})
	minioSinkFactory = func() (minioSink, error) { return &stubSink{}, nil }
	sourcesProvider = func() map[string]etl.Source { return map[string]etl.Source{"local": local} }
	defer func() {
		minioSinkFactory = origFactory
		sourcesProvider = origSources
	}()

	stdout, _, err := runCLI(t, "etl", "validate")
	assertNoError(t, err)
	assertContains(t, stdout, "Validating ETL configuration")
	assertContains(t, stdout, "MinIO connectivity: OK")
	assertContains(t, stdout, "local: OK")
}

func TestETL_Sources_UsesSourcesProvider(t *testing.T) {
	origSources := sourcesProvider
	sourcesProvider = func() map[string]etl.Source {
		return map[string]etl.Source{
			"http": source.NewHTTPSource(source.HTTPSourceConfig{
				Name: "http",
				URLs: map[string]string{"v1": "http://example.invalid"},
			}),
		}
	}
	defer func() { sourcesProvider = origSources }()

	stdout, _, err := runCLI(t, "etl", "sources")
	assertNoError(t, err)
	assertContains(t, stdout, "NAME")
	assertContains(t, stdout, "http")
}

func TestETL_Load_UnknownSource(t *testing.T) {
	_, _, err := runCLI(t, "etl", "load", "does-not-exist", "--version", "v1")
	assertError(t, err)
	assertErrorContains(t, err, "unknown source")
}

func TestETL_Load_MissingMinIOCredentials(t *testing.T) {
	t.Setenv("FI_FHIR_DATABASE_URL", "postgres://example/test")
	t.Setenv("MINIO_ACCESS_KEY", "")
	t.Setenv("MINIO_SECRET_KEY", "")

	_, _, err := runCLI(t, "etl", "load", "loinc", "--version", "2.77")
	assertError(t, err)
	assertErrorContains(t, err, "MINIO_ACCESS_KEY")
}
