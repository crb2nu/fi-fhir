package cda

import (
	"time"
)

// CDADocument represents a parsed CDA/CCDA document.
type CDADocument struct {
	// Header information
	ID                  string
	SetID               string
	VersionNumber       int
	TemplateIDs         []string
	TypeCode            CodedValue
	Title               string
	EffectiveTime       time.Time
	ConfidentialityCode string
	LanguageCode        string

	// Participants
	Patient       *PatientRole
	Author        *Author
	Custodian     *Custodian
	Authenticator *Authenticator
	InformantList []Informant

	// Encounter context
	ServiceEvent          *ServiceEvent
	EncompassingEncounter *EncompassingEncounter

	// Body sections
	Sections []Section

	// Raw XML for audit
	RawXML []byte
}

// DocumentType returns the primary template ID identifying the document type.
func (d *CDADocument) DocumentType() string {
	if len(d.TemplateIDs) > 0 {
		return d.TemplateIDs[0]
	}
	return ""
}

// FindSection returns a section by template ID.
func (d *CDADocument) FindSection(templateID string) *Section {
	for i := range d.Sections {
		if d.Sections[i].TemplateID == templateID {
			return &d.Sections[i]
		}
	}
	return nil
}

// Section represents a CDA section within the document body.
type Section struct {
	TemplateID string
	Code       CodedValue
	Title      string
	Text       string  // Narrative text (human-readable)
	Entries    []Entry // Structured entries
}

// Entry represents a structured entry in a section.
type Entry struct {
	TypeCode           string // e.g., "observation", "act", "procedure", "substanceAdministration"
	ClassCode          string
	MoodCode           string
	TemplateIDs        []string
	ID                 string
	Code               CodedValue
	StatusCode         string
	EffectiveTime      *TimeInterval
	Value              *EntryValue
	Text               string
	Participants       []Participant
	EntryRelationships []Entry // Nested entries
	Author             *Author
	Performer          *Performer
}

// EntryValue represents a value in an entry (can be various types).
type EntryValue struct {
	Type         string // CD, PQ, ST, INT, REAL, TS, IVL_TS, etc.
	Code         string
	CodeSystem   string
	DisplayName  string
	Value        string
	Unit         string
	Low          string
	High         string
	NullFlavor   string
	OriginalText string
}

// CodedValue represents a coded concept (from code systems).
type CodedValue struct {
	Code           string
	CodeSystem     string // OID
	CodeSystemName string
	DisplayName    string
	OriginalText   string
	NullFlavor     string
	Translations   []CodedValue // Translated codes
}

// IsNull returns true if this coded value has a null flavor.
func (cv CodedValue) IsNull() bool {
	return cv.NullFlavor != ""
}

// TimeInterval represents effectiveTime with low/high bounds.
type TimeInterval struct {
	Low        *time.Time
	High       *time.Time
	Value      *time.Time // Point in time
	NullFlavor string
}

// Identifier represents an II (Instance Identifier) data type.
type Identifier struct {
	Root            string // OID or UUID
	Extension       string // The actual identifier value
	AssigningAuthority string
}

// PatientRole contains patient information from recordTarget.
type PatientRole struct {
	IDs       []Identifier
	Addresses []Address
	Telecoms  []Telecom
	Patient   *PatientInfo
	Provider  *Organization
}

// PatientInfo contains patient demographics.
type PatientInfo struct {
	Names           []PersonName
	Gender          string
	BirthTime       *time.Time
	DeceasedTime    *time.Time
	MaritalStatus   *CodedValue
	RaceCode        *CodedValue
	EthnicityCode   *CodedValue
	LanguageCode    string
	ReligiousAffiliation *CodedValue
}

// PersonName represents a human name.
type PersonName struct {
	Use        string // L (legal), P (pseudonym), etc.
	Family     string
	Given      []string
	Prefix     []string
	Suffix     []string
	ValidTime  *TimeInterval
}

// FullName returns a formatted full name.
func (pn PersonName) FullName() string {
	name := ""
	for _, p := range pn.Prefix {
		name += p + " "
	}
	for _, g := range pn.Given {
		name += g + " "
	}
	name += pn.Family
	for _, s := range pn.Suffix {
		name += " " + s
	}
	return name
}

// Address represents a postal address.
type Address struct {
	Use            string // H (home), WP (work), etc.
	StreetAddress  []string
	City           string
	State          string
	PostalCode     string
	Country        string
	UseablePeriod  *TimeInterval
}

// Telecom represents a telecommunication address.
type Telecom struct {
	Use   string // HP (home phone), WP (work phone), MC (mobile), etc.
	Value string // tel:, mailto:, etc.
}

// Author represents the document or entry author.
type Author struct {
	Time         *time.Time
	AssignedAuthor *AssignedAuthor
}

// AssignedAuthor represents the author details.
type AssignedAuthor struct {
	IDs          []Identifier
	Addresses    []Address
	Telecoms     []Telecom
	Person       *PersonInfo
	Organization *Organization
}

// PersonInfo represents a person (author, performer, etc.).
type PersonInfo struct {
	Names []PersonName
}

// Organization represents an organization.
type Organization struct {
	IDs       []Identifier
	Names     []string
	Addresses []Address
	Telecoms  []Telecom
}

// Custodian represents the organization maintaining the document.
type Custodian struct {
	Organization *Organization
}

// Authenticator represents a document authenticator.
type Authenticator struct {
	Time              *time.Time
	SignatureCode     string
	AssignedEntity    *AssignedEntity
}

// AssignedEntity represents an assigned person/organization.
type AssignedEntity struct {
	IDs          []Identifier
	Addresses    []Address
	Telecoms     []Telecom
	Person       *PersonInfo
	Organization *Organization
}

// Informant represents an information source.
type Informant struct {
	AssignedEntity    *AssignedEntity
	RelatedEntity     *RelatedEntity
}

// RelatedEntity represents a related person (family member, etc.).
type RelatedEntity struct {
	ClassCode string
	Code      *CodedValue
	Person    *PersonInfo
}

// Participant represents a participant in an entry.
type Participant struct {
	TypeCode       string
	Time           *TimeInterval
	ParticipantRole *ParticipantRole
}

// ParticipantRole represents the role of a participant.
type ParticipantRole struct {
	ClassCode    string
	IDs          []Identifier
	Addresses    []Address
	Telecoms     []Telecom
	PlayingEntity *PlayingEntity
}

// PlayingEntity represents the entity playing the role.
type PlayingEntity struct {
	ClassCode string
	Code      *CodedValue
	Names     []string
}

// Performer represents a performer of an entry.
type Performer struct {
	TypeCode       string
	Time           *TimeInterval
	AssignedEntity *AssignedEntity
}

// ServiceEvent represents the main clinical act being documented.
type ServiceEvent struct {
	ClassCode     string
	Code          *CodedValue
	EffectiveTime *TimeInterval
	Performers    []Performer
}

// EncompassingEncounter represents the encounter context.
type EncompassingEncounter struct {
	IDs              []Identifier
	Code             *CodedValue
	EffectiveTime    *TimeInterval
	DischargeDisposition *CodedValue
	Location         *Location
}

// Location represents a healthcare facility location.
type Location struct {
	HealthCareFacility *HealthCareFacility
}

// HealthCareFacility represents a healthcare facility.
type HealthCareFacility struct {
	IDs          []Identifier
	Code         *CodedValue
	Location     *Place
	Organization *Organization
}

// Place represents a physical place.
type Place struct {
	Name    string
	Address *Address
}
