package model

import "time"

// EventFilter is the input type for filtering events
type EventFilter struct {
	Types         []EventType `json:"types,omitempty"`
	Sources       []string    `json:"sources,omitempty"`
	PatientMrn    *string     `json:"patientMrn,omitempty"`
	FromTimestamp *time.Time  `json:"fromTimestamp,omitempty"`
	ToTimestamp   *time.Time  `json:"toTimestamp,omitempty"`
	CorrelationID *string     `json:"correlationId,omitempty"`
}

// EventOrderBy specifies ordering for event queries
type EventOrderBy struct {
	Field     EventOrderField `json:"field"`
	Direction OrderDirection  `json:"direction"`
}

// PatientFilter is the input type for filtering patients
type PatientFilter struct {
	MRN         *string    `json:"mrn,omitempty"`
	FamilyName  *string    `json:"familyName,omitempty"`
	GivenName   *string    `json:"givenName,omitempty"`
	DateOfBirth *time.Time `json:"dateOfBirth,omitempty"`
}

// SubmitMessageInput is the input for submitting raw messages
type SubmitMessageInput struct {
	Format        SourceFormat `json:"format"`
	Data          string       `json:"data"`
	Source        string       `json:"source"`
	CorrelationID *string      `json:"correlationId,omitempty"`
}

// SubmitEventInput is the input for submitting pre-parsed events
type SubmitEventInput struct {
	Type          EventType              `json:"type"`
	Data          map[string]interface{} `json:"data"`
	Source        string                 `json:"source"`
	CorrelationID *string                `json:"correlationId,omitempty"`
}

// CreateSubscriptionInput is the input for creating FHIR subscriptions
type CreateSubscriptionInput struct {
	Name     string `json:"name"`
	Server   string `json:"server"`
	Criteria string `json:"criteria"`
	Endpoint string `json:"endpoint"`
}
