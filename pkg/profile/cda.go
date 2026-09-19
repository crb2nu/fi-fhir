package profile

import (
	"fmt"
	"strings"
)

// CDAConfig selects the canonical events emitted from a CDA document. Selection
// does not remove sections from the parsed document or redact its raw XML.
type CDAConfig struct {
	// Omitted flags retain document and section event emission.
	EmitDocumentEvents *bool `yaml:"emit_document_events,omitempty" json:"emit_document_events,omitempty"`
	EmitSectionEvents  *bool `yaml:"emit_section_events,omitempty" json:"emit_section_events,omitempty"`

	// A nonempty list emits only sections explicitly enabled in the list.
	// An omitted or empty list leaves all registered section mappers enabled.
	Sections []CDASectionConfig `yaml:"sections,omitempty" json:"sections,omitempty"`
}

// CDASectionConfig selects event emission for an exact section template OID.
type CDASectionConfig struct {
	TemplateID string `yaml:"template_id" json:"template_id"`
	EmitEvents bool   `yaml:"emit_events" json:"emit_events"`
}

// Validate rejects ambiguous section selection. Custom template IDs are allowed
// so callers can register their own section parsers and mappers.
func (c *CDAConfig) Validate() error {
	if c == nil {
		return nil
	}
	seen := make(map[string]bool, len(c.Sections))
	for i, section := range c.Sections {
		if section.TemplateID == "" || strings.TrimSpace(section.TemplateID) != section.TemplateID {
			return fmt.Errorf("source_profile.cda.sections[%d].template_id must be nonempty with no surrounding whitespace", i)
		}
		if seen[section.TemplateID] {
			return fmt.Errorf("source_profile.cda.sections[%d].template_id duplicates %q", i, section.TemplateID)
		}
		seen[section.TemplateID] = true
	}
	return nil
}
