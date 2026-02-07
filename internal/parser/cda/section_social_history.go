package cda

import (
	"strings"

	"github.com/antchfx/xmlquery"
)

// SocialHistorySectionParser parses the Social History section of a CDA/CCDA document.
// It handles Social History Observations, Smoking Status (LOINC 72166-2), and Tobacco Use entries.
type SocialHistorySectionParser struct{}

// TemplateOID returns the Social History section template identifier.
func (p *SocialHistorySectionParser) TemplateOID() string {
	return TemplateSectionSocialHistory
}

// Parse extracts social history observations from a Social History section XML node.
func (p *SocialHistorySectionParser) Parse(sectionNode *xmlquery.Node, config *ParserConfig) (*Section, error) {
	section := &Section{
		TemplateID: TemplateSectionSocialHistory,
	}

	// Code
	if code := findOne(sectionNode, "code"); code != nil {
		section.Code = parseCodedValue(code)
	}

	// Title
	if title := findOne(sectionNode, "title"); title != nil {
		section.Title = strings.TrimSpace(title.InnerText())
	}

	// Parse each entry
	for _, entryNode := range findAll(sectionNode, "entry") {
		entry := parseSocialHistoryEntry(entryNode)
		if entry != nil {
			section.Entries = append(section.Entries, *entry)
		}
	}

	return section, nil
}

// parseSocialHistoryEntry parses a single social history observation entry.
func parseSocialHistoryEntry(entryNode *xmlquery.Node) *Entry {
	obs := findOne(entryNode, "observation")
	if obs == nil {
		return nil
	}

	entry := &Entry{
		TypeCode:  "observation",
		ClassCode: getAttr(obs, "classCode"),
		MoodCode:  getAttr(obs, "moodCode"),
	}

	// Template IDs
	for _, tmpl := range findAll(obs, "templateId") {
		if oid := getAttr(tmpl, "root"); oid != "" {
			entry.TemplateIDs = append(entry.TemplateIDs, oid)
		}
	}

	// ID
	if id := findOne(obs, "id"); id != nil {
		entry.ID = getAttr(id, "root")
		if ext := getAttr(id, "extension"); ext != "" {
			entry.ID = ext
		}
	}

	// Code (observation type — e.g., LOINC 72166-2 for smoking status)
	if code := findOne(obs, "code"); code != nil {
		entry.Code = parseCodedValue(code)
	}

	// Status
	if status := findOne(obs, "statusCode"); status != nil {
		entry.StatusCode = getAttr(status, "code")
	}

	// Effective time
	if effTime := findOne(obs, "effectiveTime"); effTime != nil {
		entry.EffectiveTime = parseTimeInterval(effTime)
	}

	// Value (coded or text observation value)
	if value := findOne(obs, "value"); value != nil {
		entry.Value = parseEntryValue(value)
	}

	// Infer category from template IDs and code
	entry.Text = inferSocialHistoryCategory(entry)

	return entry
}

// inferSocialHistoryCategory determines the category based on template OIDs and LOINC codes.
func inferSocialHistoryCategory(entry *Entry) string {
	// Check template IDs for specific subtypes
	for _, tmplID := range entry.TemplateIDs {
		switch tmplID {
		case TemplateEntrySmokingStatus:
			return "smoking-status"
		case TemplateEntryTobaccoUse:
			return "tobacco-use"
		}
	}

	// Check LOINC codes for known social history categories
	if entry.Code.CodeSystem == CodeSystemLOINC {
		switch entry.Code.Code {
		case "72166-2": // Tobacco smoking status
			return "smoking-status"
		case "11367-0": // History of tobacco use
			return "tobacco-use"
		case "74013-4": // Alcoholic drinks per day
			return "alcohol-use"
		case "11331-6": // History of alcohol use
			return "alcohol-use"
		case "74204-9": // Drug use status
			return "drug-use"
		case "76689-9": // Sex assigned at birth
			return "sex-assigned-at-birth"
		}
	}

	return "social-history"
}
