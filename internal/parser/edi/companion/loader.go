package companion

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadGuide loads a companion guide from a file (YAML or JSON).
// The format is detected from the file extension.
func LoadGuide(path string) (*CompanionGuide, error) {
	f, err := os.Open(path) // #nosec G304 - path is validated by caller
	if err != nil {
		return nil, fmt.Errorf("failed to open guide file: %w", err)
	}
	defer func() { _ = f.Close() }()

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		return LoadGuideFromYAML(f)
	case ".json":
		return LoadGuideFromJSON(f)
	default:
		return nil, fmt.Errorf("unsupported file extension: %s (use .yaml, .yml, or .json)", ext)
	}
}

// LoadGuideFromYAML loads a companion guide from YAML.
func LoadGuideFromYAML(r io.Reader) (*CompanionGuide, error) {
	var guide CompanionGuide
	decoder := yaml.NewDecoder(r)
	if err := decoder.Decode(&guide); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	if err := validateGuide(&guide); err != nil {
		return nil, err
	}

	applyDefaults(&guide)
	return &guide, nil
}

// LoadGuideFromJSON loads a companion guide from JSON.
func LoadGuideFromJSON(r io.Reader) (*CompanionGuide, error) {
	var guide CompanionGuide
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&guide); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	if err := validateGuide(&guide); err != nil {
		return nil, err
	}

	applyDefaults(&guide)
	return &guide, nil
}

// LoadGuidesFromDirectory loads all companion guides from a directory.
// Files with .yaml, .yml, or .json extensions are processed.
func LoadGuidesFromDirectory(dir string) ([]*CompanionGuide, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var guides []*CompanionGuide
	var errors []string

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		guide, err := LoadGuide(path)
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", entry.Name(), err))
			continue
		}
		guides = append(guides, guide)
	}

	if len(errors) > 0 {
		return guides, fmt.Errorf("some guides failed to load: %s", strings.Join(errors, "; "))
	}

	return guides, nil
}

// SaveGuideToYAML saves a companion guide to YAML format.
func SaveGuideToYAML(guide *CompanionGuide, w io.Writer) error {
	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	return encoder.Encode(guide)
}

// SaveGuideToJSON saves a companion guide to JSON format.
func SaveGuideToJSON(guide *CompanionGuide, w io.Writer, pretty bool) error {
	encoder := json.NewEncoder(w)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(guide)
}

// SaveGuide saves a companion guide to a file.
// The format is determined by the file extension.
func SaveGuide(guide *CompanionGuide, path string) error {
	f, err := os.Create(path) // #nosec G304 - path is validated by caller
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer func() { _ = f.Close() }()

	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		return SaveGuideToYAML(guide, f)
	case ".json":
		return SaveGuideToJSON(guide, f, true)
	default:
		return fmt.Errorf("unsupported file extension: %s", ext)
	}
}

// validateGuide validates that a companion guide has required fields.
func validateGuide(guide *CompanionGuide) error {
	if guide.ID == "" {
		return fmt.Errorf("guide ID is required")
	}
	if guide.Name == "" {
		return fmt.Errorf("guide name is required")
	}
	if len(guide.TransactionTypes) == 0 {
		return fmt.Errorf("at least one transaction type is required")
	}

	// Validate validation rules
	for i, rule := range guide.Validations {
		if rule.ID == "" {
			return fmt.Errorf("validation rule %d: ID is required", i)
		}
		if rule.Path == "" {
			return fmt.Errorf("validation rule %s: path is required", rule.ID)
		}
		if rule.Type == "" {
			return fmt.Errorf("validation rule %s: type is required", rule.ID)
		}

		// Validate path syntax
		if _, err := ParsePath(rule.Path); err != nil {
			return fmt.Errorf("validation rule %s: invalid path: %w", rule.ID, err)
		}
	}

	// Validate overrides
	for i, override := range guide.Overrides {
		if override.Path == "" {
			return fmt.Errorf("override %d: path is required", i)
		}
		if override.Requirement == "" {
			return fmt.Errorf("override for %s: requirement is required", override.Path)
		}

		// Validate path syntax
		if _, err := ParsePath(override.Path); err != nil {
			return fmt.Errorf("override %d: invalid path: %w", i, err)
		}
	}

	// Validate code restrictions
	for i, restriction := range guide.CodeRestrictions {
		if restriction.Path == "" {
			return fmt.Errorf("code restriction %d: path is required", i)
		}
		if len(restriction.Values) == 0 {
			return fmt.Errorf("code restriction for %s: values are required", restriction.Path)
		}

		// Validate path syntax
		if _, err := ParsePath(restriction.Path); err != nil {
			return fmt.Errorf("code restriction %d: invalid path: %w", i, err)
		}
	}

	return nil
}

// applyDefaults applies default values to a companion guide.
func applyDefaults(guide *CompanionGuide) {
	// Set default severity for validation rules
	for i := range guide.Validations {
		if guide.Validations[i].Severity == "" {
			guide.Validations[i].Severity = SeverityError
		}
	}

	// Set default severity for code restrictions
	for i := range guide.CodeRestrictions {
		if guide.CodeRestrictions[i].Severity == "" {
			guide.CodeRestrictions[i].Severity = SeverityError
		}
	}
}

// ParseGuideFromString parses a companion guide from a string.
// It attempts YAML first, then JSON if YAML fails.
func ParseGuideFromString(content string) (*CompanionGuide, error) {
	// Try YAML first (YAML is a superset of JSON)
	guide, err := LoadGuideFromYAML(strings.NewReader(content))
	if err == nil {
		return guide, nil
	}

	// Try JSON
	guide, err = LoadGuideFromJSON(strings.NewReader(content))
	if err == nil {
		return guide, nil
	}

	return nil, fmt.Errorf("failed to parse as YAML or JSON: %w", err)
}
