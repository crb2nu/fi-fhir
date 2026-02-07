package cda

import (
	"github.com/antchfx/xmlquery"
)

// MedicationsSectionParser parses the Medications section of a CDA/CCDA document.
// It extracts substanceAdministration entries with drug codes, dosing, routes, and status.
type MedicationsSectionParser struct{}

// TemplateOID returns the Medications section template identifier.
func (p *MedicationsSectionParser) TemplateOID() string {
	return TemplateSectionMedications
}

// Parse extracts medication entries from a Medications section XML node.
func (p *MedicationsSectionParser) Parse(sectionNode *xmlquery.Node, config *ParserConfig) (*Section, error) {
	section := &Section{
		TemplateID: TemplateSectionMedications,
	}

	// Code
	if code := findOne(sectionNode, "code"); code != nil {
		section.Code = parseCodedValue(code)
	}

	// Title
	if title := findOne(sectionNode, "title"); title != nil {
		section.Title = title.InnerText()
	}

	// Parse each entry
	for _, entryNode := range findAll(sectionNode, "entry") {
		entry := parseMedicationEntry(entryNode, config)
		if entry != nil {
			section.Entries = append(section.Entries, *entry)
		}
	}

	return section, nil
}

// parseMedicationEntry parses a single medication substanceAdministration entry.
func parseMedicationEntry(entryNode *xmlquery.Node, config *ParserConfig) *Entry {
	// Find the substanceAdministration element
	sa := findOne(entryNode, "substanceAdministration")
	if sa == nil {
		return nil
	}

	entry := &Entry{
		TypeCode:  "substanceAdministration",
		ClassCode: getAttr(sa, "classCode"),
		MoodCode:  getAttr(sa, "moodCode"),
	}

	// Template IDs
	for _, tmpl := range findAll(sa, "templateId") {
		if oid := getAttr(tmpl, "root"); oid != "" {
			entry.TemplateIDs = append(entry.TemplateIDs, oid)
		}
	}

	// ID
	if id := findOne(sa, "id"); id != nil {
		entry.ID = getAttr(id, "root")
		if ext := getAttr(id, "extension"); ext != "" {
			entry.ID = ext
		}
	}

	// Status code
	if status := findOne(sa, "statusCode"); status != nil {
		entry.StatusCode = getAttr(status, "code")
	}

	// Effective time (dosing period)
	if effTime := findOne(sa, "effectiveTime"); effTime != nil {
		entry.EffectiveTime = parseTimeInterval(effTime)
	}

	// Route code
	if routeCode := findOne(sa, "routeCode"); routeCode != nil {
		entry.Code = parseCodedValue(routeCode)
	}

	// Dose quantity — store in Value as PQ type
	if doseQty := findOne(sa, "doseQuantity"); doseQty != nil {
		entry.Value = &EntryValue{
			Type:  "PQ",
			Value: getAttr(doseQty, "value"),
			Unit:  getAttr(doseQty, "unit"),
		}
	}

	// Drug code from consumable/manufacturedProduct/manufacturedMaterial/code
	consumable := findOne(sa, "consumable")
	if consumable != nil {
		mp := findOne(consumable, "manufacturedProduct")
		if mp != nil {
			mm := findOne(mp, "manufacturedMaterial")
			if mm != nil {
				if code := findOne(mm, "code"); code != nil {
					// Drug code takes priority over route code
					entry.Code = parseCodedValue(code)
				}
			}
		}
	}

	// Nested entry relationships (supply orders, indications, instructions)
	for _, rel := range findAll(sa, "entryRelationship") {
		nestedEntry := parseMedicationRelationship(rel)
		if nestedEntry != nil {
			entry.EntryRelationships = append(entry.EntryRelationships, *nestedEntry)
		}
	}

	return entry
}

// parseMedicationRelationship parses nested entryRelationship elements within a medication.
func parseMedicationRelationship(relNode *xmlquery.Node) *Entry {
	// Try supply
	if supply := findOne(relNode, "supply"); supply != nil {
		entry := &Entry{
			TypeCode: "supply",
		}
		if id := findOne(supply, "id"); id != nil {
			entry.ID = getAttr(id, "root")
		}
		if status := findOne(supply, "statusCode"); status != nil {
			entry.StatusCode = getAttr(status, "code")
		}
		if effTime := findOne(supply, "effectiveTime"); effTime != nil {
			entry.EffectiveTime = parseTimeInterval(effTime)
		}
		return entry
	}

	// Try observation (indication)
	if obs := findOne(relNode, "observation"); obs != nil {
		entry := &Entry{
			TypeCode: "observation",
		}
		if code := findOne(obs, "code"); code != nil {
			entry.Code = parseCodedValue(code)
		}
		if value := findOne(obs, "value"); value != nil {
			entry.Value = parseEntryValue(value)
		}
		return entry
	}

	// Try act (instructions)
	if act := findOne(relNode, "act"); act != nil {
		entry := &Entry{
			TypeCode: "act",
		}
		if code := findOne(act, "code"); code != nil {
			entry.Code = parseCodedValue(code)
		}
		if text := findOne(act, "text"); text != nil {
			entry.Text = text.InnerText()
		}
		return entry
	}

	return nil
}
