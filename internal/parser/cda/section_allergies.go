package cda

import (
	"strings"

	"github.com/antchfx/xmlquery"
)

// AllergiesSectionParser parses the Allergies and Adverse Reactions section of a CDA/CCDA document.
// It handles the nested CDA structure: entry > act (Allergy Concern) > entryRelationship > observation
// (Allergy Intolerance) with participant (allergen) and nested reactions/severity.
type AllergiesSectionParser struct{}

// TemplateOID returns the Allergies section template identifier.
func (p *AllergiesSectionParser) TemplateOID() string {
	return TemplateSectionAllergies
}

// Parse extracts allergy entries from an Allergies section XML node.
func (p *AllergiesSectionParser) Parse(sectionNode *xmlquery.Node, config *ParserConfig) (*Section, error) {
	section := &Section{
		TemplateID: TemplateSectionAllergies,
	}

	// Code
	if code := findOne(sectionNode, "code"); code != nil {
		section.Code = parseCodedValue(code)
	}

	// Title
	if title := findOne(sectionNode, "title"); title != nil {
		section.Title = strings.TrimSpace(title.InnerText())
	}

	// Parse each entry (Allergy Concern Acts)
	for _, entryNode := range findAll(sectionNode, "entry") {
		entries := parseAllergyConcernAct(entryNode)
		section.Entries = append(section.Entries, entries...)
	}

	return section, nil
}

// parseAllergyConcernAct parses an Allergy Concern Act and its nested observations.
func parseAllergyConcernAct(entryNode *xmlquery.Node) []Entry {
	act := findOne(entryNode, "act")
	if act == nil {
		return nil
	}

	actEntry := Entry{
		TypeCode:  "act",
		ClassCode: getAttr(act, "classCode"),
		MoodCode:  getAttr(act, "moodCode"),
	}

	// Template IDs
	for _, tmpl := range findAll(act, "templateId") {
		if oid := getAttr(tmpl, "root"); oid != "" {
			actEntry.TemplateIDs = append(actEntry.TemplateIDs, oid)
		}
	}

	// ID
	if id := findOne(act, "id"); id != nil {
		actEntry.ID = getAttr(id, "root")
		if ext := getAttr(id, "extension"); ext != "" {
			actEntry.ID = ext
		}
	}

	// Status (active vs completed)
	if status := findOne(act, "statusCode"); status != nil {
		actEntry.StatusCode = getAttr(status, "code")
	}

	// Effective time
	if effTime := findOne(act, "effectiveTime"); effTime != nil {
		actEntry.EffectiveTime = parseTimeInterval(effTime)
	}

	// Parse nested allergy intolerance observations
	for _, rel := range findAll(act, "entryRelationship") {
		obs := parseAllergyObservation(rel)
		if obs != nil {
			actEntry.EntryRelationships = append(actEntry.EntryRelationships, *obs)
		}
	}

	return []Entry{actEntry}
}

// parseAllergyObservation parses an Allergy Intolerance Observation.
func parseAllergyObservation(relNode *xmlquery.Node) *Entry {
	obs := findOne(relNode, "observation")
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

	// Code (allergy type code)
	if code := findOne(obs, "code"); code != nil {
		entry.Code = parseCodedValue(code)
	}

	// Status
	if status := findOne(obs, "statusCode"); status != nil {
		entry.StatusCode = getAttr(status, "code")
	}

	// Effective time (onset)
	if effTime := findOne(obs, "effectiveTime"); effTime != nil {
		entry.EffectiveTime = parseTimeInterval(effTime)
	}

	// Value (the allergy substance — sometimes coded here instead of participant)
	if value := findOne(obs, "value"); value != nil {
		entry.Value = parseEntryValue(value)
	}

	// Negation indicator ("no known allergies")
	negation := getAttr(obs, "negationInd")
	if negation == "true" {
		entry.Text = "negated"
	}

	// Participant (allergen substance)
	for _, part := range findAll(obs, "participant") {
		participant := parseAllergyParticipant(part)
		if participant != nil {
			entry.Participants = append(entry.Participants, *participant)
		}
	}

	// Nested entry relationships (reactions, severity)
	for _, rel := range findAll(obs, "entryRelationship") {
		nestedEntry := parseAllergyNestedEntry(rel)
		if nestedEntry != nil {
			entry.EntryRelationships = append(entry.EntryRelationships, *nestedEntry)
		}
	}

	return entry
}

// parseAllergyParticipant parses the participant/participantRole/playingEntity/code
// which identifies the allergen substance.
func parseAllergyParticipant(partNode *xmlquery.Node) *Participant {
	part := &Participant{
		TypeCode: getAttr(partNode, "typeCode"),
	}

	pr := findOne(partNode, "participantRole")
	if pr == nil {
		return nil
	}

	part.ParticipantRole = &ParticipantRole{
		ClassCode: getAttr(pr, "classCode"),
	}

	pe := findOne(pr, "playingEntity")
	if pe == nil {
		return part
	}

	part.ParticipantRole.PlayingEntity = &PlayingEntity{
		ClassCode: getAttr(pe, "classCode"),
	}

	if code := findOne(pe, "code"); code != nil {
		cv := parseCodedValue(code)
		part.ParticipantRole.PlayingEntity.Code = &cv
	}

	for _, name := range findAll(pe, "name") {
		part.ParticipantRole.PlayingEntity.Names = append(
			part.ParticipantRole.PlayingEntity.Names,
			strings.TrimSpace(name.InnerText()),
		)
	}

	return part
}

// parseAllergyNestedEntry parses reaction (MFST) and severity observations within an allergy.
func parseAllergyNestedEntry(relNode *xmlquery.Node) *Entry {
	typeCode := getAttr(relNode, "typeCode")

	obs := findOne(relNode, "observation")
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

	// Code
	if code := findOne(obs, "code"); code != nil {
		entry.Code = parseCodedValue(code)
	}

	// Value (reaction manifestation or severity)
	if value := findOne(obs, "value"); value != nil {
		entry.Value = parseEntryValue(value)
	}

	// Mark entry relationship type for downstream mapping
	if typeCode == "MFST" || typeCode == "SUBJ" {
		entry.Text = typeCode
	}

	return entry
}
