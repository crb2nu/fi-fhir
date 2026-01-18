package profile

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type LintOptions struct {
	Format      string
	SamplesPath string
	Verbose     bool
}

type LintReport struct {
	Errors   []string
	Warnings []string

	SampleStats *HL7v2SampleStats
	SampleFiles []string
}

func LintProfileFile(profilePath string, opts LintOptions) (*LintReport, error) {
	if profilePath == "" {
		return nil, fmt.Errorf("profile path is required")
	}
	if _, err := os.Stat(profilePath); err != nil {
		return nil, fmt.Errorf("profile not found: %w", err)
	}

	r := &LintReport{}

	data, err := os.ReadFile(profilePath) //nolint:gosec // G304: user-provided path
	if err != nil {
		return nil, fmt.Errorf("failed to read profile: %w", err)
	}

	var wrapper struct {
		SourceProfile *SourceProfile `yaml:"source_profile"`
	}
	if err := yaml.Unmarshal(data, &wrapper); err != nil {
		return nil, fmt.Errorf("failed to parse profile YAML: %w", err)
	}
	p := wrapper.SourceProfile

	addError := func(format string, args ...any) {
		r.Errors = append(r.Errors, fmt.Sprintf(format, args...))
	}
	addWarning := func(format string, args ...any) {
		r.Warnings = append(r.Warnings, fmt.Sprintf(format, args...))
	}

	if p == nil {
		addError("missing source_profile root element")
		sort.Strings(r.Errors)
		sort.Strings(r.Warnings)
		return r, nil
	}

	if strings.TrimSpace(p.ID) == "" {
		addError("source_profile.id is required")
	}
	if strings.TrimSpace(p.Name) == "" {
		addError("source_profile.name is required")
	}
	if strings.TrimSpace(p.Version) == "" {
		addWarning("source_profile.version is empty (set a SemVer string)")
	}

	if p.HL7v2 != nil {
		lintHL7v2Config(p.HL7v2, addError, addWarning)
	}
	if p.ZSegments != nil {
		lintZSegments(p.ZSegments, addError, addWarning)
	}
	if p.Terminology != nil {
		lintTerminology(p.Terminology, addError, addWarning)
	}
	if p.Identifiers != nil {
		lintIdentifiers(p.Identifiers, addError, addWarning)
	}

	if opts.SamplesPath != "" {
		format := strings.TrimSpace(opts.Format)
		if format == "" {
			format = "hl7v2"
		}
		switch format {
		case "hl7v2", "hl7":
			samples, used, err := ReadHL7v2Samples([]string{opts.SamplesPath}, ReadHL7v2SamplesOptions{})
			if err != nil {
				return nil, err
			}
			stats, err := AnalyzeHL7v2Samples(samples)
			if err != nil {
				return nil, err
			}
			r.SampleStats = stats
			r.SampleFiles = used

			lintProfileAgainstHL7v2Samples(p, stats, addWarning)
		default:
			return nil, fmt.Errorf("unsupported --format for samples: %s (expected hl7v2)", format)
		}
	}

	sort.Strings(r.Errors)
	sort.Strings(r.Warnings)
	return r, nil
}

func lintHL7v2Config(c *HL7v2Config, addError func(string, ...any), addWarning func(string, ...any)) {
	if strings.TrimSpace(c.DefaultVersion) == "" {
		addWarning("hl7v2.default_version is empty (set to most common MSH-12, e.g. 2.5.1)")
	} else {
		validVersions := map[string]bool{
			"2.3": true, "2.3.1": true, "2.4": true, "2.5": true, "2.5.1": true,
			"2.6": true, "2.7": true, "2.7.1": true, "2.8": true,
		}
		if !validVersions[c.DefaultVersion] {
			addError("invalid hl7v2.default_version %q (expected one of: 2.3, 2.3.1, 2.4, 2.5, 2.5.1, 2.6, 2.7, 2.7.1, 2.8)", c.DefaultVersion)
		}
	}

	if strings.TrimSpace(c.Timezone) == "" {
		addWarning("hl7v2.timezone is empty (set an IANA zone like America/New_York)")
	}

	if c.Encoding == nil {
		addWarning("hl7v2.encoding is missing (charset + line ending handling defaults may be surprising)")
	} else {
		if strings.TrimSpace(c.Encoding.CharsetDefault) == "" {
			addWarning("hl7v2.encoding.charset_default is empty (default to UTF-8 is typical)")
		}
		if c.Encoding.LineEndingMode != "" && c.Encoding.LineEndingMode != "strict" && c.Encoding.LineEndingMode != "tolerant" {
			addError("invalid hl7v2.encoding.line_ending_mode %q (expected strict or tolerant)", c.Encoding.LineEndingMode)
		}
	}

	if c.Tolerate != nil {
		for _, seg := range c.Tolerate.MissingSegments {
			seg = strings.TrimSpace(seg)
			if seg == "" {
				continue
			}
			if seg == "MSH" {
				addError("hl7v2.tolerate.missing_segments must not include MSH")
				continue
			}
			if !isValidSegmentID(seg) {
				addError("hl7v2.tolerate.missing_segments contains invalid segment id %q", seg)
			}
		}

		if c.Tolerate.UnknownSegments {
			addWarning("hl7v2.tolerate.unknown_segments=true can hide unexpected segments; consider disabling once stable")
		}
		if c.Tolerate.NonStandardDelimiters {
			addWarning("hl7v2.tolerate.non_standard_delimiters=true allows delimiter drift; prefer pinning once stable")
		}
	}
}

func lintZSegments(z *ZSegmentConfig, addError func(string, ...any), addWarning func(string, ...any)) {
	if z.Mappings == nil {
		addWarning("z_segments.mappings is missing (Z-segments will only be preserved raw)")
		return
	}

	for segID, mappings := range z.Mappings {
		if len(segID) < 1 || segID[0] != 'Z' {
			addError("invalid z_segments.mappings key %q (must start with 'Z')", segID)
			continue
		}
		if !isValidSegmentID(segID) {
			addWarning("z_segments.mappings key %q is not a typical 3-character segment id", segID)
		}
		for i, m := range mappings {
			if m.Field < 1 {
				addError("%s mapping %d: field must be >= 1", segID, i)
			}
			if strings.TrimSpace(m.Target) == "" {
				addError("%s mapping %d: target is required", segID, i)
			}
			if m.Type != "" {
				validTypes := map[string]bool{
					"string": true, "integer": true, "float": true, "boolean": true, "date": true, "datetime": true,
				}
				if !validTypes[m.Type] {
					addError("%s mapping %d: invalid type %q (expected string, integer, float, boolean, date, datetime)", segID, i, m.Type)
				}
			}
		}
	}
}

func lintTerminology(t *TerminologyConfig, addError func(string, ...any), addWarning func(string, ...any)) {
	if t.UnknownCodeBehavior != "" {
		switch t.UnknownCodeBehavior {
		case "pass", "warn", "error":
		default:
			addError("terminology.unknown_code_behavior must be one of pass|warn|error (got %q)", t.UnknownCodeBehavior)
		}
	}

	for i, m := range t.Mappings {
		if strings.TrimSpace(m.SourceSystem) == "" {
			addError("terminology.mappings[%d].source_system is required", i)
		}
		if strings.TrimSpace(m.TargetSystem) == "" {
			addError("terminology.mappings[%d].target_system is required", i)
		}
		if strings.TrimSpace(m.File) == "" {
			addError("terminology.mappings[%d].file is required", i)
		}
	}

	if t.Versions != nil && len(t.Versions) == 0 {
		addWarning("terminology.versions is present but empty")
	}
}

func lintIdentifiers(i *IdentifierConfig, addError func(string, ...any), _ func(string, ...any)) {
	if i.Validation == nil {
		return
	}
	for k, v := range i.Validation {
		if v == nil {
			continue
		}
		switch v.OnInvalid {
		case "", "pass", "warn", "error":
		default:
			addError("identifiers.validation.%s.on_invalid must be one of pass|warn|error (got %q)", k, v.OnInvalid)
		}
	}
}

func lintProfileAgainstHL7v2Samples(p *SourceProfile, stats *HL7v2SampleStats, addWarning func(string, ...any)) {
	if stats == nil || stats.MessageCount == 0 {
		return
	}

	if p.HL7v2 == nil {
		addWarning("samples contain HL7v2 but profile.hl7v2 is missing")
		return
	}

	commonVersion, _ := mostCommonString(stats.Versions)
	if p.HL7v2.DefaultVersion != "" && commonVersion != "" && p.HL7v2.DefaultVersion != commonVersion {
		addWarning("hl7v2.default_version=%q differs from samples most common MSH-12=%q", p.HL7v2.DefaultVersion, commonVersion)
	}

	if p.HL7v2.Encoding != nil {
		commonCharset, _ := mostCommonString(stats.CharSets)
		if p.HL7v2.Encoding.CharsetDefault != "" && commonCharset != "" && p.HL7v2.Encoding.CharsetDefault != commonCharset {
			addWarning("hl7v2.encoding.charset_default=%q differs from samples most common MSH-18=%q", p.HL7v2.Encoding.CharsetDefault, commonCharset)
		}
	}

	if p.HL7v2.Tolerate != nil && !p.HL7v2.Tolerate.NonStandardDelimiters && (len(stats.FieldSeparators) > 1 || len(stats.EncodingChars) > 1) {
		addWarning("samples show delimiter variation but hl7v2.tolerate.non_standard_delimiters=false")
	}

	if len(stats.ZSegments) > 0 {
		zIDs := make([]string, 0, len(stats.ZSegments))
		for id := range stats.ZSegments {
			zIDs = append(zIDs, id)
		}
		sort.Strings(zIDs)

		if p.ZSegments == nil || p.ZSegments.Mappings == nil {
			addWarning("samples contain Z-segments (%s) but profile.z_segments.mappings is missing", strings.Join(zIDs, ", "))
			return
		}

		var missing []string
		for _, zid := range zIDs {
			if _, ok := p.ZSegments.Mappings[zid]; !ok {
				missing = append(missing, zid)
			}
		}
		if len(missing) > 0 {
			addWarning("profile is missing z_segments.mappings entries for: %s", strings.Join(missing, ", "))
		}
	}
}

func isValidSegmentID(id string) bool {
	id = strings.ToUpper(strings.TrimSpace(id))
	if len(id) != 3 {
		return false
	}
	for _, c := range id {
		if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			return false
		}
	}
	return true
}
