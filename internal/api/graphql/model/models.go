package model

import (
	"time"
)

// Event is the interface for all event types in the GraphQL schema.
// All event types must implement this interface to be returned in event queries.
type Event interface {
	IsEvent()
	GetID() string
	GetType() EventType
	GetTimestamp() time.Time
	GetSource() string
	GetSourceFormat() *SourceFormat
	GetCorrelationID() *string
}

// BaseEventFields contains common fields for all events.
// Embed this in concrete event types to get the common fields.
type BaseEventFields struct {
	ID            string        `json:"id"`
	Type          EventType     `json:"type"`
	Timestamp     time.Time     `json:"timestamp"`
	Source        string        `json:"source"`
	SourceFormat  *SourceFormat `json:"sourceFormat,omitempty"`
	CorrelationID *string       `json:"correlationId,omitempty"`
}

// Event interface implementation for BaseEventFields
func (e BaseEventFields) GetID() string              { return e.ID }
func (e BaseEventFields) GetType() EventType         { return e.Type }
func (e BaseEventFields) GetTimestamp() time.Time    { return e.Timestamp }
func (e BaseEventFields) GetSource() string          { return e.Source }
func (e BaseEventFields) GetSourceFormat() *SourceFormat { return e.SourceFormat }
func (e BaseEventFields) GetCorrelationID() *string  { return e.CorrelationID }

// Concrete event types - implement Event interface

type PatientAdmitEvent struct {
	BaseEventFields
	Patient   Patient   `json:"patient"`
	Encounter Encounter `json:"encounter"`
}

func (PatientAdmitEvent) IsEvent() {}

type PatientDischargeEvent struct {
	BaseEventFields
	Patient   Patient   `json:"patient"`
	Encounter Encounter `json:"encounter"`
}

func (PatientDischargeEvent) IsEvent() {}

type LabResultEvent struct {
	BaseEventFields
	Patient          Patient   `json:"patient"`
	Test             LabTest   `json:"test"`
	Result           LabResult `json:"result"`
	IsCritical       bool      `json:"isCritical"`
	OrderingProvider *Provider `json:"orderingProvider,omitempty"`
}

func (LabResultEvent) IsEvent() {}

type VitalSignEvent struct {
	BaseEventFields
	Patient   Patient   `json:"patient"`
	VitalSign VitalSign `json:"vitalSign"`
}

func (VitalSignEvent) IsEvent() {}

type ConditionEvent struct {
	BaseEventFields
	Patient        Patient   `json:"patient"`
	Condition      Condition `json:"condition"`
	ClinicalStatus *string   `json:"clinicalStatus,omitempty"`
	OnsetDate      *string   `json:"onsetDate,omitempty"`
}

func (ConditionEvent) IsEvent() {}

type ProcedureEvent struct {
	BaseEventFields
	Patient       Patient   `json:"patient"`
	Procedure     Procedure `json:"procedure"`
	PerformedDate *string   `json:"performedDate,omitempty"`
}

func (ProcedureEvent) IsEvent() {}

type ImmunizationEvent struct {
	BaseEventFields
	Patient          Patient      `json:"patient"`
	Immunization     Immunization `json:"immunization"`
	AdministeredDate *string      `json:"administeredDate,omitempty"`
}

func (ImmunizationEvent) IsEvent() {}

type AppointmentEvent struct {
	BaseEventFields
	Patient     Patient     `json:"patient"`
	Appointment Appointment `json:"appointment"`
}

func (AppointmentEvent) IsEvent() {}

type DocumentEvent struct {
	BaseEventFields
	Patient      *Patient `json:"patient,omitempty"`
	DocumentType string   `json:"documentType"`
	Title        *string  `json:"title,omitempty"`
}

func (DocumentEvent) IsEvent() {}

// Supporting types

type Patient struct {
	MRN         string       `json:"mrn"`
	Identifiers []Identifier `json:"identifiers"`
	FamilyName  string       `json:"familyName"`
	GivenName   string       `json:"givenName"`
	MiddleName  *string      `json:"middleName,omitempty"`
	DateOfBirth *time.Time   `json:"dateOfBirth,omitempty"`
	Gender      *string      `json:"gender,omitempty"`
	Address     *Address     `json:"address,omitempty"`
	Phone       *string      `json:"phone,omitempty"`
	Email       *string      `json:"email,omitempty"`
}

type Identifier struct {
	Value    string  `json:"value"`
	Type     string  `json:"type"`
	System   *string `json:"system,omitempty"`
	Assigner *string `json:"assigner,omitempty"`
}

type Address struct {
	Line1      *string `json:"line1,omitempty"`
	Line2      *string `json:"line2,omitempty"`
	City       *string `json:"city,omitempty"`
	State      *string `json:"state,omitempty"`
	PostalCode *string `json:"postalCode,omitempty"`
	Country    *string `json:"country,omitempty"`
}

type Provider struct {
	NPI              *string `json:"npi,omitempty"`
	ID               *string `json:"id,omitempty"`
	FamilyName       string  `json:"familyName"`
	GivenName        string  `json:"givenName"`
	Specialty        *string `json:"specialty,omitempty"`
	OrganizationName *string `json:"organizationName,omitempty"`
}

type Location struct {
	Facility *string `json:"facility,omitempty"`
	Unit     *string `json:"unit,omitempty"`
	Room     *string `json:"room,omitempty"`
	Bed      *string `json:"bed,omitempty"`
}

type Encounter struct {
	ID                string     `json:"id"`
	Class             string     `json:"class"`
	Status            *string    `json:"status,omitempty"`
	AdmitDateTime     *time.Time `json:"admitDateTime,omitempty"`
	DischargeDateTime *time.Time `json:"dischargeDateTime,omitempty"`
	Location          *Location  `json:"location,omitempty"`
	AttendingProvider *Provider  `json:"attendingProvider,omitempty"`
}

type LabTest struct {
	LoincCode   *string `json:"loincCode,omitempty"`
	LocalCode   *string `json:"localCode,omitempty"`
	Description string  `json:"description"`
	Category    *string `json:"category,omitempty"`
}

type LabResult struct {
	Value          string  `json:"value"`
	Unit           *string `json:"unit,omitempty"`
	ReferenceRange *string `json:"referenceRange,omitempty"`
	Interpretation *string `json:"interpretation,omitempty"`
	Status         *string `json:"status,omitempty"`
}

type VitalSign struct {
	Name           string  `json:"name"`
	LoincCode      *string `json:"loincCode,omitempty"`
	Value          string  `json:"value"`
	Unit           *string `json:"unit,omitempty"`
	Interpretation *string `json:"interpretation,omitempty"`
}

type Condition struct {
	Name       string  `json:"name"`
	Code       *string `json:"code,omitempty"`
	CodeSystem *string `json:"codeSystem,omitempty"`
	Category   *string `json:"category,omitempty"`
}

type Procedure struct {
	Name       string  `json:"name"`
	Code       *string `json:"code,omitempty"`
	CodeSystem *string `json:"codeSystem,omitempty"`
	Status     *string `json:"status,omitempty"`
}

type Immunization struct {
	VaccineName string  `json:"vaccineName"`
	VaccineCode *string `json:"vaccineCode,omitempty"`
	Status      *string `json:"status,omitempty"`
}

type Appointment struct {
	ID        string     `json:"id"`
	Status    string     `json:"status"`
	StartTime time.Time  `json:"startTime"`
	EndTime   *time.Time `json:"endTime,omitempty"`
	Location  *Location  `json:"location,omitempty"`
	Provider  *Provider  `json:"provider,omitempty"`
	Reason    *string    `json:"reason,omitempty"`
}

// Pagination types

type PageInfo struct {
	HasNextPage     bool    `json:"hasNextPage"`
	HasPreviousPage bool    `json:"hasPreviousPage"`
	StartCursor     *string `json:"startCursor,omitempty"`
	EndCursor       *string `json:"endCursor,omitempty"`
}

type EventEdge struct {
	Cursor string `json:"cursor"`
	Node   Event  `json:"node"`
}

type EventConnection struct {
	Edges      []EventEdge `json:"edges"`
	PageInfo   PageInfo    `json:"pageInfo"`
	TotalCount int         `json:"totalCount"`
}

type PatientEdge struct {
	Cursor string  `json:"cursor"`
	Node   Patient `json:"node"`
}

type PatientConnection struct {
	Edges      []PatientEdge `json:"edges"`
	PageInfo   PageInfo      `json:"pageInfo"`
	TotalCount int           `json:"totalCount"`
}

// Workflow and health types

type WorkflowStatus struct {
	Name            string     `json:"name"`
	Enabled         bool       `json:"enabled"`
	RouteCount      int        `json:"routeCount"`
	EventsProcessed int        `json:"eventsProcessed"`
	LastEventTime   *time.Time `json:"lastEventTime,omitempty"`
	Errors          int        `json:"errors"`
}

type ComponentHealth struct {
	Name    string  `json:"name"`
	Status  string  `json:"status"`
	Message *string `json:"message,omitempty"`
}

type HealthStatus struct {
	Status     string            `json:"status"`
	Version    string            `json:"version"`
	Uptime     int               `json:"uptime"`
	Components []ComponentHealth `json:"components"`
}

type ParseWarning struct {
	Phase   string  `json:"phase"`
	Code    string  `json:"code"`
	Message string  `json:"message"`
	Path    *string `json:"path,omitempty"`
}

type ParseResult struct {
	Success  bool           `json:"success"`
	Events   []Event        `json:"events"`
	Warnings []ParseWarning `json:"warnings"`
	Errors   []string       `json:"errors"`
}

// Mutation result types

type WorkflowResult struct {
	WorkflowName    string   `json:"workflowName"`
	RoutesMatched   int      `json:"routesMatched"`
	ActionsExecuted int      `json:"actionsExecuted"`
	Errors          []string `json:"errors"`
	Duration        int      `json:"duration"`
}

type SubmitResult struct {
	Success         bool             `json:"success"`
	EventID         *string          `json:"eventId,omitempty"`
	Warnings        []ParseWarning   `json:"warnings"`
	Errors          []string         `json:"errors"`
	WorkflowResults []WorkflowResult `json:"workflowResults"`
}

type FhirSubscription struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Criteria  string    `json:"criteria"`
	Server    string    `json:"server"`
	Endpoint  string    `json:"endpoint"`
	CreatedAt time.Time `json:"createdAt"`
}

// Subscription notification type

type WorkflowEventNotification struct {
	Event           Event    `json:"event"`
	Workflow        string   `json:"workflow"`
	RoutesMatched   []string `json:"routesMatched"`
	ActionsExecuted []string `json:"actionsExecuted"`
	Duration        int      `json:"duration"`
}

// =============================================================================
// Event Sourcing / Projection Types
// =============================================================================

// TimelineEvent represents a single event in a patient's timeline projection.
type TimelineEvent struct {
	Position  int       `json:"position"`
	Timestamp time.Time `json:"timestamp"`
	EventType string    `json:"eventType"`
	Summary   string    `json:"summary"`
	StreamID  string    `json:"streamId"`
	Source    *string   `json:"source,omitempty"`
}

// PatientTimeline is a projection showing chronological events for a patient.
type PatientTimeline struct {
	MRN         string          `json:"mrn"`
	Events      []TimelineEvent `json:"events"`
	LastUpdated time.Time       `json:"lastUpdated"`
	EventCount  int             `json:"eventCount"`
}

// EventTypeCount represents a count of events by type for statistics.
type EventTypeCount struct {
	EventType string `json:"eventType"`
	Count     int    `json:"count"`
}

// SourceCount represents a count of events by source for statistics.
type SourceCount struct {
	Source string `json:"source"`
	Count  int    `json:"count"`
}

// EventStatistics provides aggregate statistics across all events.
type EventStatistics struct {
	TotalEvents int              `json:"totalEvents"`
	ByType      []EventTypeCount `json:"byType"`
	BySource    []SourceCount    `json:"bySource"`
}

// ActiveEncounter represents a currently active patient encounter (census view).
type ActiveEncounter struct {
	ID          string    `json:"id"`
	PatientMRN  string    `json:"patientMrn"`
	PatientName *string   `json:"patientName,omitempty"`
	Class       string    `json:"class"`
	Location    *string   `json:"location,omitempty"`
	Unit        *string   `json:"unit,omitempty"`
	Room        *string   `json:"room,omitempty"`
	Bed         *string   `json:"bed,omitempty"`
	AdmitTime   time.Time `json:"admitTime"`
	Provider    *string   `json:"provider,omitempty"`
	LastUpdated time.Time `json:"lastUpdated"`
}

// ProjectionStatus shows the current state of a projection.
type ProjectionStatus struct {
	Name         string `json:"name"`
	Checkpoint   int    `json:"checkpoint"`
	LastPosition int    `json:"lastPosition"`
	Behind       int    `json:"behind"`
	Status       string `json:"status"`
}
