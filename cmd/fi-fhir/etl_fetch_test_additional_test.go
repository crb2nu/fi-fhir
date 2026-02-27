package main

import (
	"os"
	"path/filepath"
	"testing"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/etl"
)

func TestETL_FetchTest_WritesToOutputDir(t *testing.T) {
	origSources := sourcesProvider
	src := &stubSource{
		name: "stubsrc",
		versions: []etl.VersionInfo{
			{Version: "v1", IsLatest: true},
		},
		payload: []byte("hello"),
	}
	sourcesProvider = func() map[string]etl.Source { return map[string]etl.Source{"stubsrc": src} }
	defer func() { sourcesProvider = origSources }()

	outDir := t.TempDir()
	stdout, _, err := runCLI(t, "etl", "fetch-test", "stubsrc", "--output", outDir)
	assertNoError(t, err)
	assertContains(t, stdout, "Fetching test data: stubsrc")

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("failed to read output dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected at least one file/dir in output dir")
	}

	// spot-check that a file with the payload exists somewhere under outDir.
	foundPayload := false
	_ = filepath.WalkDir(outDir, func(path string, d os.DirEntry, werr error) error {
		if werr != nil {
			return werr
		}
		if d.IsDir() {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr == nil && string(b) == "hello" {
			foundPayload = true
		}
		return nil
	})
	if !foundPayload {
		t.Fatalf("expected to find downloaded payload in output dir")
	}
}
