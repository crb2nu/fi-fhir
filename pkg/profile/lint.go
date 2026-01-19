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

	// Samples overrides SamplesPath when provided. This is primarily for CLIs that support stdin ("-").
	Samples     []string
	SampleFiles []string

	// MaxFiles limits how many files will be read when SamplesPath is a directory.
	// If <= 0, defaults to ReadHL7v2SamplesOptions default (200).
	MaxFiles int
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

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse profile YAML AST: %w", err)
	}

	addError := func(format string, args ...any) {
		r.Errors = append(r.Errors, fmt.Sprintf(format, args...))
	}
	addWarning := func(format string, args ...any) {
		r.Warnings = append(r.Warnings, fmt.Sprintf(format, args...))
	}

	lintUnknownKeys(&doc, addWarning)

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

	if len(opts.Samples) > 0 || opts.SamplesPath != "" {
		format := strings.TrimSpace(opts.Format)
		if format == "" {
			format = "hl7v2"
		}
		switch format {
		case "hl7v2", "hl7":
			samples := opts.Samples
			used := opts.SampleFiles
			if len(samples) == 0 && opts.SamplesPath != "" {
				var err error
				samples, used, err = ReadHL7v2Samples([]string{opts.SamplesPath}, ReadHL7v2SamplesOptions{
					MaxFiles: opts.MaxFiles,
				})
				if err != nil {
					return nil, err
				}
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

func lintUnknownKeys(doc *yaml.Node, addWarning func(string, ...any)) {
	if doc == nil || doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return
	}
	root := doc.Content[0]
	if root == nil || root.Kind != yaml.MappingNode {
		return
	}

	sourceProfileNode := mappingValue(root, "source_profile")
	if sourceProfileNode == nil || sourceProfileNode.Kind != yaml.MappingNode {
		return
	}

	allowedSourceProfile := map[string]bool{
		"id": true, "name": true, "version": true,
		"hl7v2": true, "edi": true, "z_segments": true,
		"identifiers": true, "terminology": true, "quality": true,
	}
	warnUnknownKeysInMapping(sourceProfileNode, "source_profile", allowedSourceProfile, addWarning)

	if n := mappingValue(sourceProfileNode, "hl7v2"); n != nil && n.Kind == yaml.MappingNode {
		allowed := map[string]bool{
			"default_version": true, "timezone": true,
			"encoding": true, "tolerate": true, "datatypes": true, "event_classification": true,
		}
		warnUnknownKeysInMapping(n, "source_profile.hl7v2", allowed, addWarning)

		if enc := mappingValue(n, "encoding"); enc != nil && enc.Kind == yaml.MappingNode {
			warnUnknownKeysInMapping(enc, "source_profile.hl7v2.encoding", map[string]bool{
				"charset_default": true, "charset_detection": true, "line_ending_mode": true,
			}, addWarning)
		}
		if tol := mappingValue(n, "tolerate"); tol != nil && tol.Kind == yaml.MappingNode {
			warnUnknownKeysInMapping(tol, "source_profile.hl7v2.tolerate", map[string]bool{
				"missing_segments": true, "nte_anywhere": true, "extra_components": true, "unknown_segments": true, "non_standard_delimiters": true,
			}, addWarning)
		}
		if dt := mappingValue(n, "datatypes"); dt != nil && dt.Kind == yaml.MappingNode {
			warnUnknownKeysInMapping(dt, "source_profile.hl7v2.datatypes", map[string]bool{
				"xcn_component_count": true, "cx_component_count": true, "xpn_component_count": true,
			}, addWarning)
		}
		if ec := mappingValue(n, "event_classification"); ec != nil && ec.Kind == yaml.MappingNode {
			warnUnknownKeysInMapping(ec, "source_profile.hl7v2.event_classification", map[string]bool{
				"adt_a01": true, "adt_a04": true, "adt_a08": true,
			}, addWarning)
			warnUnknownKeysInEventRuleMapping(ec, "source_profile.hl7v2.event_classification", addWarning)
		}
	}

	if n := mappingValue(sourceProfileNode, "edi"); n != nil && n.Kind == yaml.MappingNode {
		warnUnknownKeysInMapping(n, "source_profile.edi", map[string]bool{
			"companion_guide": true, "companion_guide_dir": true,
		}, addWarning)
	}

	if n := mappingValue(sourceProfileNode, "z_segments"); n != nil && n.Kind == yaml.MappingNode {
		warnUnknownKeysInMapping(n, "source_profile.z_segments", map[string]bool{
			"preserve_raw": true, "mappings": true,
		}, addWarning)
		if m := mappingValue(n, "mappings"); m != nil && m.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(m.Content); i += 2 {
				segKey := scalarString(m.Content[i])
				seq := m.Content[i+1]
				if segKey == "" || seq == nil || seq.Kind != yaml.SequenceNode {
					continue
				}
				for j, item := range seq.Content {
					if item == nil || item.Kind != yaml.MappingNode {
						continue
					}
					warnUnknownKeysInMapping(item, fmt.Sprintf("source_profile.z_segments.mappings.%s[%d]", segKey, j), map[string]bool{
						"field": true, "target": true, "type": true,
					}, addWarning)
				}
			}
		}
	}

	if n := mappingValue(sourceProfileNode, "identifiers"); n != nil && n.Kind == yaml.MappingNode {
		warnUnknownKeysInMapping(n, "source_profile.identifiers", map[string]bool{
			"assigning_authority_map": true, "primary_id_preference": true, "validation": true, "normalization": true,
		}, addWarning)

		if pref := mappingValue(n, "primary_id_preference"); pref != nil && pref.Kind == yaml.SequenceNode {
			for i, item := range pref.Content {
				if item == nil || item.Kind != yaml.MappingNode {
					continue
				}
				warnUnknownKeysInMapping(item, fmt.Sprintf("source_profile.identifiers.primary_id_preference[%d]", i), map[string]bool{
					"type": true, "assigner_contains": true, "assigner_equals": true,
				}, addWarning)
			}
		}

		if val := mappingValue(n, "validation"); val != nil && val.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(val.Content); i += 2 {
				k := scalarString(val.Content[i])
				item := val.Content[i+1]
				if k == "" || item == nil || item.Kind != yaml.MappingNode {
					continue
				}
				warnUnknownKeysInMapping(item, "source_profile.identifiers.validation."+k, map[string]bool{
					"enabled": true, "on_invalid": true,
				}, addWarning)
			}
		}

		if norm := mappingValue(n, "normalization"); norm != nil && norm.Kind == yaml.MappingNode {
			warnUnknownKeysInMapping(norm, "source_profile.identifiers.normalization", map[string]bool{
				"ssn": true, "phone": true, "mrn": true,
			}, addWarning)
			if ssn := mappingValue(norm, "ssn"); ssn != nil && ssn.Kind == yaml.MappingNode {
				warnUnknownKeysInMapping(ssn, "source_profile.identifiers.normalization.ssn", map[string]bool{
					"strip_dashes": true, "reject_patterns": true,
				}, addWarning)
			}
			if phone := mappingValue(norm, "phone"); phone != nil && phone.Kind == yaml.MappingNode {
				warnUnknownKeysInMapping(phone, "source_profile.identifiers.normalization.phone", map[string]bool{
					"strip_country_code": true, "normalize_to_digits": true,
				}, addWarning)
			}
			if mrn := mappingValue(norm, "mrn"); mrn != nil && mrn.Kind == yaml.MappingNode {
				warnUnknownKeysInMapping(mrn, "source_profile.identifiers.normalization.mrn", map[string]bool{
					"strip_leading_zeros": true, "uppercase": true,
				}, addWarning)
			}
		}
	}

	if n := mappingValue(sourceProfileNode, "terminology"); n != nil && n.Kind == yaml.MappingNode {
		warnUnknownKeysInMapping(n, "source_profile.terminology", map[string]bool{
			"strict_validation": true, "unknown_code_behavior": true, "versions": true, "mappings": true,
		}, addWarning)
		if m := mappingValue(n, "mappings"); m != nil && m.Kind == yaml.SequenceNode {
			for i, item := range m.Content {
				if item == nil || item.Kind != yaml.MappingNode {
					continue
				}
				warnUnknownKeysInMapping(item, fmt.Sprintf("source_profile.terminology.mappings[%d]", i), map[string]bool{
					"source_system": true, "target_system": true, "file": true,
				}, addWarning)
			}
		}
	}

	if n := mappingValue(sourceProfileNode, "quality"); n != nil && n.Kind == yaml.MappingNode {
		warnUnknownKeysInMapping(n, "source_profile.quality", map[string]bool{
			"metrics": true, "alerts": true,
		}, addWarning)
		if a := mappingValue(n, "alerts"); a != nil && a.Kind == yaml.MappingNode {
			for i := 0; i+1 < len(a.Content); i += 2 {
				k := scalarString(a.Content[i])
				item := a.Content[i+1]
				if k == "" || item == nil || item.Kind != yaml.MappingNode {
					continue
				}
				warnUnknownKeysInMapping(item, "source_profile.quality.alerts."+k, map[string]bool{
					"threshold": true, "severity": true,
				}, addWarning)
			}
		}
	}
}

func warnUnknownKeysInEventRuleMapping(ec *yaml.Node, base string, addWarning func(string, ...any)) {
	for i := 0; i+1 < len(ec.Content); i += 2 {
		k := scalarString(ec.Content[i])
		v := ec.Content[i+1]
		if k == "" || v == nil || v.Kind != yaml.MappingNode {
			continue
		}
		warnUnknownKeysInMapping(v, base+"."+k, map[string]bool{
			"default": true, "rules": true,
		}, addWarning)
		rules := mappingValue(v, "rules")
		if rules == nil || rules.Kind != yaml.SequenceNode {
			continue
		}
		for j, item := range rules.Content {
			if item == nil || item.Kind != yaml.MappingNode {
				continue
			}
			warnUnknownKeysInMapping(item, fmt.Sprintf("%s.%s.rules[%d]", base, k, j), map[string]bool{
				"condition": true, "event": true,
			}, addWarning)
		}
	}
}

func warnUnknownKeysInMapping(n *yaml.Node, path string, allowed map[string]bool, addWarning func(string, ...any)) {
	if n == nil || n.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		key := scalarString(n.Content[i])
		if key == "" {
			continue
		}
		if !allowed[key] {
			addWarning("unknown key %q at %s", key, path)
		}
	}
}

func mappingValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		k := scalarString(m.Content[i])
		if k == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func scalarString(n *yaml.Node) string {
	if n == nil || n.Kind != yaml.ScalarNode {
		return ""
	}
	return n.Value
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
