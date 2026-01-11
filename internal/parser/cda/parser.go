package cda

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/antchfx/xmlquery"
)

// Parser parses CDA/CCDA documents.
type Parser struct {
	source         string
	config         ParserConfig
	sectionParsers map[string]SectionParser
}

// ParserConfig configures parser behavior.
type ParserConfig struct {
	// StrictMode fails on any parsing error (vs. collecting warnings)
	StrictMode bool

	// ExtractNarrative includes human-readable text from sections
	ExtractNarrative bool

	// ValidateTemplates checks template OIDs against known CCDA templates
	ValidateTemplates bool

	// MaxSectionDepth limits nested entry parsing (default: 5)
	MaxSectionDepth int
}

// SectionParser handles parsing of specific section types.
type SectionParser interface {
	TemplateOID() string
	Parse(section *xmlquery.Node, config *ParserConfig) (*Section, error)
}

// ParseResult contains parsed document and any warnings.
type ParseResult struct {
	Document *CDADocument
	Warnings []ParseWarning
}

// ParseWarning represents a non-fatal parsing issue.
type ParseWarning struct {
	Location string
	Message  string
	Code     string
}

// NewParser creates a new CDA parser.
func NewParser(source string, config *ParserConfig) *Parser {
	if config == nil {
		config = &ParserConfig{}
	}
	if config.MaxSectionDepth == 0 {
		config.MaxSectionDepth = 5
	}

	p := &Parser{
		source:         source,
		config:         *config,
		sectionParsers: make(map[string]SectionParser),
	}

	return p
}

// RegisterSectionParser adds a custom section parser.
func (p *Parser) RegisterSectionParser(parser SectionParser) {
	p.sectionParsers[parser.TemplateOID()] = parser
}

// Parse parses a CDA/CCDA document.
func (p *Parser) Parse(xmlData []byte) (*CDADocument, error) {
	result, err := p.ParseWithResult(xmlData)
	if err != nil {
		return nil, err
	}
	return result.Document, nil
}

// ParseWithResult returns document with warnings.
func (p *Parser) ParseWithResult(xmlData []byte) (*ParseResult, error) {
	doc, err := xmlquery.Parse(bytes.NewReader(xmlData))
	if err != nil {
		return nil, fmt.Errorf("failed to parse XML: %w", err)
	}

	result := &ParseResult{
		Document: &CDADocument{
			RawXML: xmlData,
		},
		Warnings: []ParseWarning{},
	}

	// Find ClinicalDocument root
	root := xmlquery.FindOne(doc, "//ClinicalDocument")
	if root == nil {
		// Try with namespace prefix
		root = xmlquery.FindOne(doc, "//*[local-name()='ClinicalDocument']")
	}
	if root == nil {
		return nil, fmt.Errorf("ClinicalDocument element not found")
	}

	// Parse header
	if err := p.parseHeader(root, result); err != nil {
		if p.config.StrictMode {
			return nil, err
		}
		result.Warnings = append(result.Warnings, ParseWarning{
			Location: "header",
			Message:  err.Error(),
			Code:     "HEADER_PARSE_ERROR",
		})
	}

	// Parse body sections
	if err := p.parseBody(root, result); err != nil {
		if p.config.StrictMode {
			return nil, err
		}
		result.Warnings = append(result.Warnings, ParseWarning{
			Location: "body",
			Message:  err.Error(),
			Code:     "BODY_PARSE_ERROR",
		})
	}

	return result, nil
}

// parseHeader extracts header information from the ClinicalDocument.
func (p *Parser) parseHeader(root *xmlquery.Node, result *ParseResult) error {
	doc := result.Document

	// Document ID
	if id := findOne(root, "id"); id != nil {
		doc.ID = getAttr(id, "root")
		if ext := getAttr(id, "extension"); ext != "" {
			doc.ID = ext
		}
	}

	// Set ID (for versioning)
	if setID := findOne(root, "setId"); setID != nil {
		doc.SetID = getAttr(setID, "root")
		if ext := getAttr(setID, "extension"); ext != "" {
			doc.SetID = ext
		}
	}

	// Version number
	if ver := findOne(root, "versionNumber"); ver != nil {
		if v, err := strconv.Atoi(getAttr(ver, "value")); err == nil {
			doc.VersionNumber = v
		}
	}

	// Template IDs
	for _, tmpl := range findAll(root, "templateId") {
		if oid := getAttr(tmpl, "root"); oid != "" {
			doc.TemplateIDs = append(doc.TemplateIDs, oid)
		}
	}

	// Document type code
	if code := findOne(root, "code"); code != nil {
		doc.TypeCode = parseCodedValue(code)
	}

	// Title
	if title := findOne(root, "title"); title != nil {
		doc.Title = strings.TrimSpace(title.InnerText())
	}

	// Effective time
	if effTime := findOne(root, "effectiveTime"); effTime != nil {
		if t := parseTimestamp(getAttr(effTime, "value")); t != nil {
			doc.EffectiveTime = *t
		}
	}

	// Confidentiality
	if conf := findOne(root, "confidentialityCode"); conf != nil {
		doc.ConfidentialityCode = getAttr(conf, "code")
	}

	// Language
	if lang := findOne(root, "languageCode"); lang != nil {
		doc.LanguageCode = getAttr(lang, "code")
	}

	// Patient (recordTarget)
	if rt := findOne(root, "recordTarget"); rt != nil {
		doc.Patient = p.parsePatientRole(rt)
	}

	// Author
	if author := findOne(root, "author"); author != nil {
		doc.Author = p.parseAuthor(author)
	}

	// Custodian
	if custodian := findOne(root, "custodian"); custodian != nil {
		doc.Custodian = p.parseCustodian(custodian)
	}

	// Authenticator
	if auth := findOne(root, "authenticator"); auth != nil {
		doc.Authenticator = p.parseAuthenticator(auth)
	}

	// Informants
	for _, inf := range findAll(root, "informant") {
		if informant := p.parseInformant(inf); informant != nil {
			doc.InformantList = append(doc.InformantList, *informant)
		}
	}

	// Service Event (documentationOf)
	if docOf := findOne(root, "documentationOf"); docOf != nil {
		if se := findOne(docOf, "serviceEvent"); se != nil {
			doc.ServiceEvent = p.parseServiceEvent(se)
		}
	}

	// Encompassing Encounter (componentOf)
	if compOf := findOne(root, "componentOf"); compOf != nil {
		if ee := findOne(compOf, "encompassingEncounter"); ee != nil {
			doc.EncompassingEncounter = p.parseEncompassingEncounter(ee)
		}
	}

	return nil
}

// parseBody extracts sections from the document body.
func (p *Parser) parseBody(root *xmlquery.Node, result *ParseResult) error {
	// Find structuredBody
	component := findOne(root, "component")
	if component == nil {
		return nil // No body is valid for some documents
	}

	structuredBody := findOne(component, "structuredBody")
	if structuredBody == nil {
		return nil
	}

	// Parse each section
	for _, comp := range findAll(structuredBody, "component") {
		section := findOne(comp, "section")
		if section == nil {
			continue
		}

		parsedSection, err := p.parseSection(section, 0)
		if err != nil {
			if p.config.StrictMode {
				return err
			}
			result.Warnings = append(result.Warnings, ParseWarning{
				Location: "section",
				Message:  err.Error(),
				Code:     "SECTION_PARSE_ERROR",
			})
			continue
		}

		if parsedSection != nil {
			result.Document.Sections = append(result.Document.Sections, *parsedSection)
		}
	}

	return nil
}

// parseSection parses a single section.
func (p *Parser) parseSection(node *xmlquery.Node, depth int) (*Section, error) {
	if depth > p.config.MaxSectionDepth {
		return nil, fmt.Errorf("max section depth exceeded")
	}

	section := &Section{}

	// Template ID
	if tmpl := findOne(node, "templateId"); tmpl != nil {
		section.TemplateID = getAttr(tmpl, "root")
	}

	// Check for custom section parser
	if parser, ok := p.sectionParsers[section.TemplateID]; ok {
		return parser.Parse(node, &p.config)
	}

	// Code
	if code := findOne(node, "code"); code != nil {
		section.Code = parseCodedValue(code)
	}

	// Title
	if title := findOne(node, "title"); title != nil {
		section.Title = strings.TrimSpace(title.InnerText())
	}

	// Narrative text
	if p.config.ExtractNarrative {
		if text := findOne(node, "text"); text != nil {
			section.Text = text.InnerText()
		}
	}

	// Entries
	for _, entry := range findAll(node, "entry") {
		parsedEntry := p.parseEntry(entry, depth+1)
		if parsedEntry != nil {
			section.Entries = append(section.Entries, *parsedEntry)
		}
	}

	return section, nil
}

// parseEntry parses a structured entry.
func (p *Parser) parseEntry(node *xmlquery.Node, depth int) *Entry {
	if depth > p.config.MaxSectionDepth {
		return nil
	}

	// Find the actual entry content (act, observation, procedure, etc.)
	var entryContent *xmlquery.Node
	var typeCode string

	for _, child := range node.SelectElements("*") {
		name := child.Data
		switch name {
		case "act", "observation", "procedure", "substanceAdministration",
			"supply", "encounter", "organizer":
			entryContent = child
			typeCode = name
		}
	}

	if entryContent == nil {
		return nil
	}

	entry := &Entry{
		TypeCode:  typeCode,
		ClassCode: getAttr(entryContent, "classCode"),
		MoodCode:  getAttr(entryContent, "moodCode"),
	}

	// Template IDs
	for _, tmpl := range findAll(entryContent, "templateId") {
		if oid := getAttr(tmpl, "root"); oid != "" {
			entry.TemplateIDs = append(entry.TemplateIDs, oid)
		}
	}

	// ID
	if id := findOne(entryContent, "id"); id != nil {
		entry.ID = getAttr(id, "root")
		if ext := getAttr(id, "extension"); ext != "" {
			entry.ID = ext
		}
	}

	// Code
	if code := findOne(entryContent, "code"); code != nil {
		entry.Code = parseCodedValue(code)
	}

	// Status
	if status := findOne(entryContent, "statusCode"); status != nil {
		entry.StatusCode = getAttr(status, "code")
	}

	// Effective time
	if effTime := findOne(entryContent, "effectiveTime"); effTime != nil {
		entry.EffectiveTime = parseTimeInterval(effTime)
	}

	// Value
	if value := findOne(entryContent, "value"); value != nil {
		entry.Value = parseEntryValue(value)
	}

	// Text
	if text := findOne(entryContent, "text"); text != nil {
		entry.Text = strings.TrimSpace(text.InnerText())
	}

	// Participants
	for _, part := range findAll(entryContent, "participant") {
		if participant := p.parseParticipant(part); participant != nil {
			entry.Participants = append(entry.Participants, *participant)
		}
	}

	// Entry relationships (nested entries)
	for _, rel := range findAll(entryContent, "entryRelationship") {
		nestedEntry := p.parseEntry(rel, depth+1)
		if nestedEntry != nil {
			entry.EntryRelationships = append(entry.EntryRelationships, *nestedEntry)
		}
	}

	// Author
	if author := findOne(entryContent, "author"); author != nil {
		entry.Author = p.parseAuthor(author)
	}

	// Performer
	if perf := findOne(entryContent, "performer"); perf != nil {
		entry.Performer = p.parsePerformer(perf)
	}

	// For organizers, parse components
	if typeCode == "organizer" {
		for _, comp := range findAll(entryContent, "component") {
			nestedEntry := p.parseEntry(comp, depth+1)
			if nestedEntry != nil {
				entry.EntryRelationships = append(entry.EntryRelationships, *nestedEntry)
			}
		}
	}

	return entry
}

// parsePatientRole parses recordTarget/patientRole.
func (p *Parser) parsePatientRole(node *xmlquery.Node) *PatientRole {
	pr := findOne(node, "patientRole")
	if pr == nil {
		return nil
	}

	role := &PatientRole{}

	// IDs
	for _, id := range findAll(pr, "id") {
		role.IDs = append(role.IDs, parseIdentifier(id))
	}

	// Addresses
	for _, addr := range findAll(pr, "addr") {
		role.Addresses = append(role.Addresses, parseAddress(addr))
	}

	// Telecoms
	for _, tel := range findAll(pr, "telecom") {
		role.Telecoms = append(role.Telecoms, parseTelecom(tel))
	}

	// Patient info
	if patient := findOne(pr, "patient"); patient != nil {
		role.Patient = p.parsePatientInfo(patient)
	}

	// Provider organization
	if org := findOne(pr, "providerOrganization"); org != nil {
		role.Provider = p.parseOrganization(org)
	}

	return role
}

// parsePatientInfo parses patient demographics.
func (p *Parser) parsePatientInfo(node *xmlquery.Node) *PatientInfo {
	info := &PatientInfo{}

	// Names
	for _, name := range findAll(node, "name") {
		info.Names = append(info.Names, parsePersonName(name))
	}

	// Gender
	if gender := findOne(node, "administrativeGenderCode"); gender != nil {
		info.Gender = getAttr(gender, "code")
	}

	// Birth time
	if birth := findOne(node, "birthTime"); birth != nil {
		info.BirthTime = parseTimestamp(getAttr(birth, "value"))
	}

	// Deceased time (SDTC extension)
	if deceased := findOne(node, "sdtc:deceasedTime"); deceased != nil {
		info.DeceasedTime = parseTimestamp(getAttr(deceased, "value"))
	}

	// Marital status
	if marital := findOne(node, "maritalStatusCode"); marital != nil {
		cv := parseCodedValue(marital)
		info.MaritalStatus = &cv
	}

	// Race
	if race := findOne(node, "raceCode"); race != nil {
		cv := parseCodedValue(race)
		info.RaceCode = &cv
	}

	// Ethnicity
	if ethnicity := findOne(node, "ethnicGroupCode"); ethnicity != nil {
		cv := parseCodedValue(ethnicity)
		info.EthnicityCode = &cv
	}

	// Language
	if lang := findOne(node, "languageCommunication/languageCode"); lang != nil {
		info.LanguageCode = getAttr(lang, "code")
	}

	return info
}

// parseAuthor parses author information.
func (p *Parser) parseAuthor(node *xmlquery.Node) *Author {
	author := &Author{}

	// Time
	if t := findOne(node, "time"); t != nil {
		author.Time = parseTimestamp(getAttr(t, "value"))
	}

	// Assigned author
	if aa := findOne(node, "assignedAuthor"); aa != nil {
		author.AssignedAuthor = &AssignedAuthor{}

		// IDs
		for _, id := range findAll(aa, "id") {
			author.AssignedAuthor.IDs = append(author.AssignedAuthor.IDs, parseIdentifier(id))
		}

		// Addresses
		for _, addr := range findAll(aa, "addr") {
			author.AssignedAuthor.Addresses = append(author.AssignedAuthor.Addresses, parseAddress(addr))
		}

		// Telecoms
		for _, tel := range findAll(aa, "telecom") {
			author.AssignedAuthor.Telecoms = append(author.AssignedAuthor.Telecoms, parseTelecom(tel))
		}

		// Person
		if person := findOne(aa, "assignedPerson"); person != nil {
			author.AssignedAuthor.Person = p.parsePersonInfo(person)
		}

		// Organization
		if org := findOne(aa, "representedOrganization"); org != nil {
			author.AssignedAuthor.Organization = p.parseOrganization(org)
		}
	}

	return author
}

// parseCustodian parses custodian information.
func (p *Parser) parseCustodian(node *xmlquery.Node) *Custodian {
	ac := findOne(node, "assignedCustodian")
	if ac == nil {
		return nil
	}

	org := findOne(ac, "representedCustodianOrganization")
	if org == nil {
		return nil
	}

	return &Custodian{
		Organization: p.parseOrganization(org),
	}
}

// parseAuthenticator parses authenticator information.
func (p *Parser) parseAuthenticator(node *xmlquery.Node) *Authenticator {
	auth := &Authenticator{}

	// Time
	if t := findOne(node, "time"); t != nil {
		auth.Time = parseTimestamp(getAttr(t, "value"))
	}

	// Signature code
	if sig := findOne(node, "signatureCode"); sig != nil {
		auth.SignatureCode = getAttr(sig, "code")
	}

	// Assigned entity
	if ae := findOne(node, "assignedEntity"); ae != nil {
		auth.AssignedEntity = p.parseAssignedEntity(ae)
	}

	return auth
}

// parseInformant parses informant information.
func (p *Parser) parseInformant(node *xmlquery.Node) *Informant {
	informant := &Informant{}

	if ae := findOne(node, "assignedEntity"); ae != nil {
		informant.AssignedEntity = p.parseAssignedEntity(ae)
	}

	if re := findOne(node, "relatedEntity"); re != nil {
		informant.RelatedEntity = &RelatedEntity{
			ClassCode: getAttr(re, "classCode"),
		}
		if code := findOne(re, "code"); code != nil {
			cv := parseCodedValue(code)
			informant.RelatedEntity.Code = &cv
		}
		if person := findOne(re, "relatedPerson"); person != nil {
			informant.RelatedEntity.Person = p.parsePersonInfo(person)
		}
	}

	if informant.AssignedEntity == nil && informant.RelatedEntity == nil {
		return nil
	}

	return informant
}

// parseAssignedEntity parses an assignedEntity element.
func (p *Parser) parseAssignedEntity(node *xmlquery.Node) *AssignedEntity {
	ae := &AssignedEntity{}

	// IDs
	for _, id := range findAll(node, "id") {
		ae.IDs = append(ae.IDs, parseIdentifier(id))
	}

	// Addresses
	for _, addr := range findAll(node, "addr") {
		ae.Addresses = append(ae.Addresses, parseAddress(addr))
	}

	// Telecoms
	for _, tel := range findAll(node, "telecom") {
		ae.Telecoms = append(ae.Telecoms, parseTelecom(tel))
	}

	// Person
	if person := findOne(node, "assignedPerson"); person != nil {
		ae.Person = p.parsePersonInfo(person)
	}

	// Organization
	if org := findOne(node, "representedOrganization"); org != nil {
		ae.Organization = p.parseOrganization(org)
	}

	return ae
}

// parsePersonInfo parses a person element (names only).
func (p *Parser) parsePersonInfo(node *xmlquery.Node) *PersonInfo {
	info := &PersonInfo{}

	for _, name := range findAll(node, "name") {
		info.Names = append(info.Names, parsePersonName(name))
	}

	return info
}

// parseOrganization parses an organization element.
func (p *Parser) parseOrganization(node *xmlquery.Node) *Organization {
	org := &Organization{}

	// IDs
	for _, id := range findAll(node, "id") {
		org.IDs = append(org.IDs, parseIdentifier(id))
	}

	// Names
	for _, name := range findAll(node, "name") {
		org.Names = append(org.Names, strings.TrimSpace(name.InnerText()))
	}

	// Addresses
	for _, addr := range findAll(node, "addr") {
		org.Addresses = append(org.Addresses, parseAddress(addr))
	}

	// Telecoms
	for _, tel := range findAll(node, "telecom") {
		org.Telecoms = append(org.Telecoms, parseTelecom(tel))
	}

	return org
}

// parseParticipant parses a participant element.
func (p *Parser) parseParticipant(node *xmlquery.Node) *Participant {
	part := &Participant{
		TypeCode: getAttr(node, "typeCode"),
	}

	if t := findOne(node, "time"); t != nil {
		part.Time = parseTimeInterval(t)
	}

	if pr := findOne(node, "participantRole"); pr != nil {
		part.ParticipantRole = &ParticipantRole{
			ClassCode: getAttr(pr, "classCode"),
		}

		// IDs
		for _, id := range findAll(pr, "id") {
			part.ParticipantRole.IDs = append(part.ParticipantRole.IDs, parseIdentifier(id))
		}

		// Addresses
		for _, addr := range findAll(pr, "addr") {
			part.ParticipantRole.Addresses = append(part.ParticipantRole.Addresses, parseAddress(addr))
		}

		// Telecoms
		for _, tel := range findAll(pr, "telecom") {
			part.ParticipantRole.Telecoms = append(part.ParticipantRole.Telecoms, parseTelecom(tel))
		}

		// Playing entity
		if pe := findOne(pr, "playingEntity"); pe != nil {
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
		}
	}

	return part
}

// parsePerformer parses a performer element.
func (p *Parser) parsePerformer(node *xmlquery.Node) *Performer {
	perf := &Performer{
		TypeCode: getAttr(node, "typeCode"),
	}

	if t := findOne(node, "time"); t != nil {
		perf.Time = parseTimeInterval(t)
	}

	if ae := findOne(node, "assignedEntity"); ae != nil {
		perf.AssignedEntity = p.parseAssignedEntity(ae)
	}

	return perf
}

// parseServiceEvent parses a serviceEvent element.
func (p *Parser) parseServiceEvent(node *xmlquery.Node) *ServiceEvent {
	se := &ServiceEvent{
		ClassCode: getAttr(node, "classCode"),
	}

	if code := findOne(node, "code"); code != nil {
		cv := parseCodedValue(code)
		se.Code = &cv
	}

	if effTime := findOne(node, "effectiveTime"); effTime != nil {
		se.EffectiveTime = parseTimeInterval(effTime)
	}

	for _, perf := range findAll(node, "performer") {
		if performer := p.parsePerformer(perf); performer != nil {
			se.Performers = append(se.Performers, *performer)
		}
	}

	return se
}

// parseEncompassingEncounter parses an encompassingEncounter element.
func (p *Parser) parseEncompassingEncounter(node *xmlquery.Node) *EncompassingEncounter {
	ee := &EncompassingEncounter{}

	// IDs
	for _, id := range findAll(node, "id") {
		ee.IDs = append(ee.IDs, parseIdentifier(id))
	}

	// Code
	if code := findOne(node, "code"); code != nil {
		cv := parseCodedValue(code)
		ee.Code = &cv
	}

	// Effective time
	if effTime := findOne(node, "effectiveTime"); effTime != nil {
		ee.EffectiveTime = parseTimeInterval(effTime)
	}

	// Discharge disposition
	if dd := findOne(node, "dischargeDispositionCode"); dd != nil {
		cv := parseCodedValue(dd)
		ee.DischargeDisposition = &cv
	}

	// Location
	if loc := findOne(node, "location"); loc != nil {
		ee.Location = &Location{}
		if hcf := findOne(loc, "healthCareFacility"); hcf != nil {
			ee.Location.HealthCareFacility = &HealthCareFacility{}

			for _, id := range findAll(hcf, "id") {
				ee.Location.HealthCareFacility.IDs = append(
					ee.Location.HealthCareFacility.IDs,
					parseIdentifier(id),
				)
			}

			if code := findOne(hcf, "code"); code != nil {
				cv := parseCodedValue(code)
				ee.Location.HealthCareFacility.Code = &cv
			}

			if place := findOne(hcf, "location"); place != nil {
				ee.Location.HealthCareFacility.Location = &Place{}
				if name := findOne(place, "name"); name != nil {
					ee.Location.HealthCareFacility.Location.Name = strings.TrimSpace(name.InnerText())
				}
				if addr := findOne(place, "addr"); addr != nil {
					a := parseAddress(addr)
					ee.Location.HealthCareFacility.Location.Address = &a
				}
			}

			if org := findOne(hcf, "serviceProviderOrganization"); org != nil {
				ee.Location.HealthCareFacility.Organization = p.parseOrganization(org)
			}
		}
	}

	return ee
}

// Helper functions for XML navigation

func findOne(node *xmlquery.Node, path string) *xmlquery.Node {
	// Try local name first (namespace-agnostic)
	result := xmlquery.FindOne(node, fmt.Sprintf("*[local-name()='%s']", path))
	if result != nil {
		return result
	}
	// Fall back to direct name
	return xmlquery.FindOne(node, path)
}

func findAll(node *xmlquery.Node, path string) []*xmlquery.Node {
	// Try local name first (namespace-agnostic)
	results := xmlquery.Find(node, fmt.Sprintf("*[local-name()='%s']", path))
	if len(results) > 0 {
		return results
	}
	// Fall back to direct name
	return xmlquery.Find(node, path)
}

func getAttr(node *xmlquery.Node, name string) string {
	if node == nil {
		return ""
	}
	return node.SelectAttr(name)
}

func parseCodedValue(node *xmlquery.Node) CodedValue {
	cv := CodedValue{
		Code:           getAttr(node, "code"),
		CodeSystem:     getAttr(node, "codeSystem"),
		CodeSystemName: getAttr(node, "codeSystemName"),
		DisplayName:    getAttr(node, "displayName"),
		NullFlavor:     getAttr(node, "nullFlavor"),
	}

	// Original text
	if ot := findOne(node, "originalText"); ot != nil {
		cv.OriginalText = strings.TrimSpace(ot.InnerText())
	}

	// Translations
	for _, trans := range findAll(node, "translation") {
		cv.Translations = append(cv.Translations, parseCodedValue(trans))
	}

	return cv
}

func parseIdentifier(node *xmlquery.Node) Identifier {
	return Identifier{
		Root:               getAttr(node, "root"),
		Extension:          getAttr(node, "extension"),
		AssigningAuthority: getAttr(node, "assigningAuthorityName"),
	}
}

func parseAddress(node *xmlquery.Node) Address {
	addr := Address{
		Use: getAttr(node, "use"),
	}

	for _, street := range findAll(node, "streetAddressLine") {
		addr.StreetAddress = append(addr.StreetAddress, strings.TrimSpace(street.InnerText()))
	}

	if city := findOne(node, "city"); city != nil {
		addr.City = strings.TrimSpace(city.InnerText())
	}
	if state := findOne(node, "state"); state != nil {
		addr.State = strings.TrimSpace(state.InnerText())
	}
	if zip := findOne(node, "postalCode"); zip != nil {
		addr.PostalCode = strings.TrimSpace(zip.InnerText())
	}
	if country := findOne(node, "country"); country != nil {
		addr.Country = strings.TrimSpace(country.InnerText())
	}

	return addr
}

func parseTelecom(node *xmlquery.Node) Telecom {
	return Telecom{
		Use:   getAttr(node, "use"),
		Value: getAttr(node, "value"),
	}
}

func parsePersonName(node *xmlquery.Node) PersonName {
	name := PersonName{
		Use: getAttr(node, "use"),
	}

	if family := findOne(node, "family"); family != nil {
		name.Family = strings.TrimSpace(family.InnerText())
	}

	for _, given := range findAll(node, "given") {
		name.Given = append(name.Given, strings.TrimSpace(given.InnerText()))
	}

	for _, prefix := range findAll(node, "prefix") {
		name.Prefix = append(name.Prefix, strings.TrimSpace(prefix.InnerText()))
	}

	for _, suffix := range findAll(node, "suffix") {
		name.Suffix = append(name.Suffix, strings.TrimSpace(suffix.InnerText()))
	}

	return name
}

func parseTimeInterval(node *xmlquery.Node) *TimeInterval {
	ti := &TimeInterval{
		NullFlavor: getAttr(node, "nullFlavor"),
	}

	// Point in time
	if val := getAttr(node, "value"); val != "" {
		ti.Value = parseTimestamp(val)
	}

	// Interval bounds
	if low := findOne(node, "low"); low != nil {
		ti.Low = parseTimestamp(getAttr(low, "value"))
	}
	if high := findOne(node, "high"); high != nil {
		ti.High = parseTimestamp(getAttr(high, "value"))
	}

	return ti
}

func parseEntryValue(node *xmlquery.Node) *EntryValue {
	ev := &EntryValue{
		Type:       getAttr(node, "xsi:type"),
		NullFlavor: getAttr(node, "nullFlavor"),
	}

	// If no xsi:type, try to infer from content
	if ev.Type == "" {
		ev.Type = inferValueType(node)
	}

	switch ev.Type {
	case "CD", "CE", "CV":
		ev.Code = getAttr(node, "code")
		ev.CodeSystem = getAttr(node, "codeSystem")
		ev.DisplayName = getAttr(node, "displayName")
		if ot := findOne(node, "originalText"); ot != nil {
			ev.OriginalText = strings.TrimSpace(ot.InnerText())
		}
	case "PQ":
		ev.Value = getAttr(node, "value")
		ev.Unit = getAttr(node, "unit")
	case "ST", "ED":
		ev.Value = strings.TrimSpace(node.InnerText())
	case "INT":
		ev.Value = getAttr(node, "value")
	case "REAL":
		ev.Value = getAttr(node, "value")
	case "TS":
		ev.Value = getAttr(node, "value")
	case "IVL_PQ":
		if low := findOne(node, "low"); low != nil {
			ev.Low = getAttr(low, "value")
			ev.Unit = getAttr(low, "unit")
		}
		if high := findOne(node, "high"); high != nil {
			ev.High = getAttr(high, "value")
			if ev.Unit == "" {
				ev.Unit = getAttr(high, "unit")
			}
		}
	case "IVL_TS":
		if low := findOne(node, "low"); low != nil {
			ev.Low = getAttr(low, "value")
		}
		if high := findOne(node, "high"); high != nil {
			ev.High = getAttr(high, "value")
		}
	default:
		// Best effort: try to extract any value
		if val := getAttr(node, "value"); val != "" {
			ev.Value = val
		} else if code := getAttr(node, "code"); code != "" {
			ev.Code = code
			ev.CodeSystem = getAttr(node, "codeSystem")
			ev.DisplayName = getAttr(node, "displayName")
		} else {
			ev.Value = strings.TrimSpace(node.InnerText())
		}
	}

	return ev
}

func inferValueType(node *xmlquery.Node) string {
	if getAttr(node, "code") != "" {
		return "CD"
	}
	if getAttr(node, "unit") != "" {
		return "PQ"
	}
	if findOne(node, "low") != nil || findOne(node, "high") != nil {
		if getAttr(findOne(node, "low"), "unit") != "" {
			return "IVL_PQ"
		}
		return "IVL_TS"
	}
	if getAttr(node, "value") != "" {
		return "ST"
	}
	return "ST"
}

func parseTimestamp(value string) *time.Time {
	if value == "" {
		return nil
	}

	// CDA timestamps can be in various formats:
	// YYYYMMDD, YYYYMMDDHHmmss, YYYYMMDDHHMMSS+0000, YYYYMMDDHHMMSS-0500, etc.

	// Check for timezone suffix (±HHMM or ±HH:MM)
	var tzOffset string
	baseValue := value

	if len(value) > 14 {
		// Look for timezone at position 14 (after YYYYMMDDHHMMSS)
		if value[14] == '+' || value[14] == '-' {
			baseValue = value[:14]
			tzOffset = value[14:]
		}
	}

	// Base formats without timezone
	baseFormats := []string{
		"20060102150405",
		"200601021504",
		"2006010215",
		"20060102",
		"200601",
		"2006",
	}

	for _, format := range baseFormats {
		if len(baseValue) >= len(format) {
			if t, err := time.Parse(format, baseValue[:len(format)]); err == nil {
				// Apply timezone offset if present
				if tzOffset != "" {
					// Parse timezone offset (±HHMM format)
					if len(tzOffset) >= 5 {
						sign := 1
						if tzOffset[0] == '-' {
							sign = -1
						}
						hours, herr := strconv.Atoi(tzOffset[1:3])
						mins, merr := strconv.Atoi(tzOffset[3:5])
						if herr == nil && merr == nil {
							offset := sign * (hours*3600 + mins*60)
							loc := time.FixedZone("", offset)
							t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), loc)
						}
					}
				}
				return &t
			}
		}
	}

	return nil
}
