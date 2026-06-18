package profile

import (
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestHL7v2TemplatesLoadAndLint(t *testing.T) {
	templateDir := filepath.Join("..", "..", "profiles", "templates", "hl7v2")

	var paths []string
	if err := filepath.WalkDir(templateDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".yaml") {
			return nil
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		t.Fatalf("walk template dir: %v", err)
	}

	if len(paths) == 0 {
		t.Fatal("expected at least one HL7v2 template")
	}

	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			report, err := LintProfileFile(path, LintOptions{})
			if err != nil {
				t.Fatalf("lint profile: %v", err)
			}
			if len(report.Errors) != 0 {
				t.Fatalf("lint errors: %v", report.Errors)
			}

			registry := NewRegistry()
			prof, err := registry.LoadFromFile(path)
			if err != nil {
				t.Fatalf("load profile: %v", err)
			}
			if prof.HL7v2 == nil {
				t.Fatal("expected hl7v2 config")
			}
			if prof.HL7v2.Encoding == nil {
				t.Fatal("expected encoding config")
			}
			if prof.HL7v2.Tolerate == nil {
				t.Fatal("expected tolerance config")
			}
			if prof.Identifiers == nil {
				t.Fatal("expected identifier config")
			}
			if prof.ZSegments == nil {
				t.Fatal("expected z-segment config")
			}
		})
	}
}
