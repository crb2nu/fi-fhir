package profile

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"
)

// MarshalYAML renders a SourceProfile wrapped in the expected root element:
//
//	source_profile:
//	  ...
//
// This keeps output deterministic (stable map key ordering) for CLI use and golden tests.
func MarshalYAML(p *SourceProfile) ([]byte, error) {
	if p == nil {
		return nil, fmt.Errorf("profile is nil")
	}

	root := &yaml.Node{Kind: yaml.DocumentNode}
	m := &yaml.Node{Kind: yaml.MappingNode}
	root.Content = []*yaml.Node{m}

	m.Content = append(m.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "source_profile", Tag: "!!str"},
		marshalSourceProfileNode(p),
	)

	out, err := yaml.Marshal(root)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal profile YAML: %w", err)
	}
	return out, nil
}

func marshalSourceProfileNode(p *SourceProfile) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}

	addScalar(n, "id", p.ID)
	addScalar(n, "name", p.Name)
	if p.Version != "" {
		addScalar(n, "version", p.Version)
	}

	if p.HL7v2 != nil {
		addNode(n, "hl7v2", marshalHL7v2Node(p.HL7v2))
	}
	if p.EDI != nil {
		addNode(n, "edi", marshalEDINode(p.EDI))
	}
	if p.ZSegments != nil {
		addNode(n, "z_segments", marshalZSegmentsNode(p.ZSegments))
	}
	if p.Identifiers != nil {
		addNode(n, "identifiers", marshalIdentifiersNode(p.Identifiers))
	}
	if p.Terminology != nil {
		addNode(n, "terminology", marshalTerminologyNode(p.Terminology))
	}
	if p.Quality != nil {
		addNode(n, "quality", marshalQualityNode(p.Quality))
	}

	return n
}

func marshalHL7v2Node(c *HL7v2Config) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	if c.DefaultVersion != "" {
		addScalar(n, "default_version", c.DefaultVersion)
	}
	if c.Timezone != "" {
		addScalar(n, "timezone", c.Timezone)
	}
	if c.Encoding != nil {
		addNode(n, "encoding", marshalEncodingNode(c.Encoding))
	}
	if c.Tolerate != nil {
		addNode(n, "tolerate", marshalToleranceNode(c.Tolerate))
	}
	if c.Datatypes != nil {
		addNode(n, "datatypes", marshalDatatypeNode(c.Datatypes))
	}
	if c.EventRules != nil {
		addNode(n, "event_classification", marshalEventRulesNode(c.EventRules))
	}
	return n
}

func marshalEncodingNode(c *EncodingConfig) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	if c.CharsetDefault != "" {
		addScalar(n, "charset_default", c.CharsetDefault)
	}
	addBool(n, "charset_detection", c.CharsetDetection)
	if c.LineEndingMode != "" {
		addScalar(n, "line_ending_mode", c.LineEndingMode)
	}
	return n
}

func marshalToleranceNode(c *ToleranceConfig) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	if len(c.MissingSegments) > 0 {
		addStringSeq(n, "missing_segments", c.MissingSegments)
	}
	addBool(n, "nte_anywhere", c.NTEAnywhere)
	addBool(n, "extra_components", c.ExtraComponents)
	addBool(n, "unknown_segments", c.UnknownSegments)
	addBool(n, "non_standard_delimiters", c.NonStandardDelimiters)
	return n
}

func marshalDatatypeNode(c *DatatypeConfig) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	if c.XCNComponentCount != "" {
		addScalar(n, "xcn_component_count", c.XCNComponentCount)
	}
	if c.CXComponentCount != "" {
		addScalar(n, "cx_component_count", c.CXComponentCount)
	}
	if c.XPNComponentCount != "" {
		addScalar(n, "xpn_component_count", c.XPNComponentCount)
	}
	return n
}

func marshalEventRulesNode(c *EventRulesConfig) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	if c.ADTA01 != nil {
		addNode(n, "adt_a01", marshalEventRuleNode(c.ADTA01))
	}
	if c.ADTA04 != nil {
		addNode(n, "adt_a04", marshalEventRuleNode(c.ADTA04))
	}
	if c.ADTA08 != nil {
		addNode(n, "adt_a08", marshalEventRuleNode(c.ADTA08))
	}
	return n
}

func marshalEventRuleNode(r *EventRule) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	addScalar(n, "default", r.Default)
	if len(r.Rules) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, rule := range r.Rules {
			item := &yaml.Node{Kind: yaml.MappingNode}
			addScalar(item, "condition", rule.Condition)
			addScalar(item, "event", rule.Event)
			seq.Content = append(seq.Content, item)
		}
		addNode(n, "rules", seq)
	}
	return n
}

func marshalEDINode(c *EDIConfig) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	if c.CompanionGuide != "" {
		addScalar(n, "companion_guide", c.CompanionGuide)
	}
	if c.CompanionGuideDir != "" {
		addScalar(n, "companion_guide_dir", c.CompanionGuideDir)
	}
	return n
}

func marshalZSegmentsNode(c *ZSegmentConfig) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	addBool(n, "preserve_raw", c.PreserveRaw)

	if c.Mappings != nil {
		mappingsNode := &yaml.Node{Kind: yaml.MappingNode}
		keys := make([]string, 0, len(c.Mappings))
		for k := range c.Mappings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			addNode(mappingsNode, k, marshalZFieldMappingsNode(c.Mappings[k]))
		}
		addNode(n, "mappings", mappingsNode)
	}

	return n
}

func marshalZFieldMappingsNode(m []ZFieldMapping) *yaml.Node {
	seq := &yaml.Node{Kind: yaml.SequenceNode}
	for _, fm := range m {
		item := &yaml.Node{Kind: yaml.MappingNode}
		addInt(item, "field", fm.Field)
		addScalar(item, "target", fm.Target)
		if fm.Type != "" {
			addScalar(item, "type", fm.Type)
		}
		seq.Content = append(seq.Content, item)
	}
	return seq
}

func marshalIdentifiersNode(c *IdentifierConfig) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}

	if c.AssigningAuthorityMap != nil && len(c.AssigningAuthorityMap) > 0 {
		mn := &yaml.Node{Kind: yaml.MappingNode}
		keys := make([]string, 0, len(c.AssigningAuthorityMap))
		for k := range c.AssigningAuthorityMap {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			addScalar(mn, k, c.AssigningAuthorityMap[k])
		}
		addNode(n, "assigning_authority_map", mn)
	}

	if len(c.PrimaryIDPreference) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, r := range c.PrimaryIDPreference {
			item := &yaml.Node{Kind: yaml.MappingNode}
			addScalar(item, "type", r.Type)
			if r.AssignerContains != "" {
				addScalar(item, "assigner_contains", r.AssignerContains)
			}
			if r.AssignerEquals != "" {
				addScalar(item, "assigner_equals", r.AssignerEquals)
			}
			seq.Content = append(seq.Content, item)
		}
		addNode(n, "primary_id_preference", seq)
	}

	if c.Validation != nil && len(c.Validation) > 0 {
		mn := &yaml.Node{Kind: yaml.MappingNode}
		keys := make([]string, 0, len(c.Validation))
		for k := range c.Validation {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := c.Validation[k]
			if v == nil {
				continue
			}
			item := &yaml.Node{Kind: yaml.MappingNode}
			addBool(item, "enabled", v.Enabled)
			if v.OnInvalid != "" {
				addScalar(item, "on_invalid", v.OnInvalid)
			}
			addNode(mn, k, item)
		}
		addNode(n, "validation", mn)
	}

	if c.Normalization != nil {
		addNode(n, "normalization", marshalNormalizationNode(c.Normalization))
	}

	return n
}

func marshalNormalizationNode(c *NormalizationConfig) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	if c.SSN != nil {
		ssn := &yaml.Node{Kind: yaml.MappingNode}
		addBool(ssn, "strip_dashes", c.SSN.StripDashes)
		if len(c.SSN.RejectPatterns) > 0 {
			addStringSeq(ssn, "reject_patterns", c.SSN.RejectPatterns)
		}
		addNode(n, "ssn", ssn)
	}
	if c.Phone != nil {
		ph := &yaml.Node{Kind: yaml.MappingNode}
		addBool(ph, "strip_country_code", c.Phone.StripCountryCode)
		addBool(ph, "normalize_to_digits", c.Phone.NormalizeToDigits)
		addNode(n, "phone", ph)
	}
	if c.MRN != nil {
		mrn := &yaml.Node{Kind: yaml.MappingNode}
		addBool(mrn, "strip_leading_zeros", c.MRN.StripLeadingZeros)
		addBool(mrn, "uppercase", c.MRN.Uppercase)
		addNode(n, "mrn", mrn)
	}
	return n
}

func marshalTerminologyNode(c *TerminologyConfig) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	addBool(n, "strict_validation", c.StrictValidation)
	if c.UnknownCodeBehavior != "" {
		addScalar(n, "unknown_code_behavior", c.UnknownCodeBehavior)
	}
	if c.Versions != nil && len(c.Versions) > 0 {
		mn := &yaml.Node{Kind: yaml.MappingNode}
		keys := make([]string, 0, len(c.Versions))
		for k := range c.Versions {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			addScalar(mn, k, c.Versions[k])
		}
		addNode(n, "versions", mn)
	}
	if len(c.Mappings) > 0 {
		seq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, m := range c.Mappings {
			item := &yaml.Node{Kind: yaml.MappingNode}
			addScalar(item, "source_system", m.SourceSystem)
			addScalar(item, "target_system", m.TargetSystem)
			addScalar(item, "file", m.File)
			seq.Content = append(seq.Content, item)
		}
		addNode(n, "mappings", seq)
	}
	return n
}

func marshalQualityNode(c *QualityConfig) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	if len(c.Metrics) > 0 {
		addStringSeq(n, "metrics", c.Metrics)
	}
	if c.Alerts != nil && len(c.Alerts) > 0 {
		mn := &yaml.Node{Kind: yaml.MappingNode}
		keys := make([]string, 0, len(c.Alerts))
		for k := range c.Alerts {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			a := c.Alerts[k]
			if a == nil {
				continue
			}
			item := &yaml.Node{Kind: yaml.MappingNode}
			addFloat(item, "threshold", a.Threshold)
			addScalar(item, "severity", a.Severity)
			addNode(mn, k, item)
		}
		addNode(n, "alerts", mn)
	}
	return n
}

func addScalar(parent *yaml.Node, key, value string) {
	addNode(parent, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value})
}

func addBool(parent *yaml.Node, key string, value bool) {
	v := "false"
	if value {
		v = "true"
	}
	addNode(parent, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: v})
}

func addInt(parent *yaml.Node, key string, value int) {
	addNode(parent, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: fmt.Sprintf("%d", value)})
}

func addFloat(parent *yaml.Node, key string, value float64) {
	addNode(parent, key, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: fmt.Sprintf("%g", value)})
}

func addStringSeq(parent *yaml.Node, key string, values []string) {
	seq := &yaml.Node{Kind: yaml.SequenceNode}
	for _, v := range values {
		seq.Content = append(seq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v})
	}
	addNode(parent, key, seq)
}

func addNode(parent *yaml.Node, key string, value *yaml.Node) {
	parent.Content = append(parent.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		value,
	)
}
