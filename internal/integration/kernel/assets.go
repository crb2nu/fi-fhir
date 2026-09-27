package kernel

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"
)

// The built-in samples and profiles are embedded from this package's
// testdata. Everything here is synthetic: the samples are the six demo
// messages the IDE ships (ui/src/lib/features/hl7/samples/demoSamples.ts,
// held byte-identical by TestSamplesMatchIDEDemoSamples) and the profiles are
// the adt-http golden profile family (held equal to
// testdata/golden/integration/adt-http/*.json by
// TestBuiltInProfilesMatchGoldenJSON).
//
//go:embed testdata/samples testdata/profiles
var assets embed.FS

// Sample is one built-in demo message.
type Sample struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`
	Format string `json:"format"`
	Text   string `json:"text"`
}

// Profile is one built-in example Source Profile.
type Profile struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	YAML        string `json:"yaml"`
}

type sampleEntry struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`
	Format string `json:"format"`
	File   string `json:"file"`
}

type profileEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	File        string `json:"file"`
}

// Samples returns the built-in demo messages in the IDE's order.
func Samples() ([]Sample, error) {
	var entries []sampleEntry
	if err := readManifest("testdata/samples", &entries); err != nil {
		return nil, err
	}
	samples := make([]Sample, 0, len(entries))
	for _, entry := range entries {
		text, err := assets.ReadFile(path.Join("testdata/samples", entry.File))
		if err != nil {
			return nil, fmt.Errorf("sample %s: %w", entry.ID, err)
		}
		samples = append(samples, Sample{
			ID:     entry.ID,
			Name:   entry.Name,
			Source: entry.Source,
			Format: entry.Format,
			Text:   string(text),
		})
	}
	return samples, nil
}

// Profiles returns the built-in example Source Profiles.
func Profiles() ([]Profile, error) {
	var entries []profileEntry
	if err := readManifest("testdata/profiles", &entries); err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0, len(entries))
	for _, entry := range entries {
		text, err := assets.ReadFile(path.Join("testdata/profiles", entry.File))
		if err != nil {
			return nil, fmt.Errorf("profile %s: %w", entry.ID, err)
		}
		profiles = append(profiles, Profile{
			ID:          entry.ID,
			Name:        entry.Name,
			Description: entry.Description,
			YAML:        string(text),
		})
	}
	return profiles, nil
}

func readManifest(dir string, into any) error {
	raw, err := assets.ReadFile(path.Join(dir, "manifest.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return fmt.Errorf("%s/manifest.json: %w", dir, err)
	}
	return nil
}
