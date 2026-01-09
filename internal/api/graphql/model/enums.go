package model

import (
	"fmt"
	"io"
	"strconv"
)

// EventType represents the type of healthcare event
type EventType string

const (
	EventTypePatientAdmit          EventType = "PATIENT_ADMIT"
	EventTypePatientDischarge      EventType = "PATIENT_DISCHARGE"
	EventTypePatientTransfer       EventType = "PATIENT_TRANSFER"
	EventTypePatientUpdate         EventType = "PATIENT_UPDATE"
	EventTypeLabResult             EventType = "LAB_RESULT"
	EventTypeLabOrdered            EventType = "LAB_ORDERED"
	EventTypeAppointmentScheduled  EventType = "APPOINTMENT_SCHEDULED"
	EventTypeAppointmentCancelled  EventType = "APPOINTMENT_CANCELLED"
	EventTypeAppointmentNoshow     EventType = "APPOINTMENT_NOSHOW"
	EventTypeClaimSubmitted        EventType = "CLAIM_SUBMITTED"
	EventTypeClaimAdjudicated      EventType = "CLAIM_ADJUDICATED"
	EventTypeVitalSign             EventType = "VITAL_SIGN"
	EventTypeCondition             EventType = "CONDITION"
	EventTypeProcedure             EventType = "PROCEDURE"
	EventTypeImmunization          EventType = "IMMUNIZATION"
	EventTypeDocument              EventType = "DOCUMENT"
)

var AllEventType = []EventType{
	EventTypePatientAdmit,
	EventTypePatientDischarge,
	EventTypePatientTransfer,
	EventTypePatientUpdate,
	EventTypeLabResult,
	EventTypeLabOrdered,
	EventTypeAppointmentScheduled,
	EventTypeAppointmentCancelled,
	EventTypeAppointmentNoshow,
	EventTypeClaimSubmitted,
	EventTypeClaimAdjudicated,
	EventTypeVitalSign,
	EventTypeCondition,
	EventTypeProcedure,
	EventTypeImmunization,
	EventTypeDocument,
}

func (e EventType) IsValid() bool {
	switch e {
	case EventTypePatientAdmit, EventTypePatientDischarge, EventTypePatientTransfer,
		EventTypePatientUpdate, EventTypeLabResult, EventTypeLabOrdered,
		EventTypeAppointmentScheduled, EventTypeAppointmentCancelled, EventTypeAppointmentNoshow,
		EventTypeClaimSubmitted, EventTypeClaimAdjudicated, EventTypeVitalSign,
		EventTypeCondition, EventTypeProcedure, EventTypeImmunization, EventTypeDocument:
		return true
	}
	return false
}

func (e EventType) String() string {
	return string(e)
}

func (e *EventType) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}
	*e = EventType(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid EventType", str)
	}
	return nil
}

func (e EventType) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

// SourceFormat represents the source format of healthcare data
type SourceFormat string

const (
	SourceFormatHL7v2  SourceFormat = "HL7V2"
	SourceFormatFHIR   SourceFormat = "FHIR"
	SourceFormatCSV    SourceFormat = "CSV"
	SourceFormatEDI837 SourceFormat = "EDI_837"
	SourceFormatEDI835 SourceFormat = "EDI_835"
	SourceFormatCDA    SourceFormat = "CDA"
)

var AllSourceFormat = []SourceFormat{
	SourceFormatHL7v2,
	SourceFormatFHIR,
	SourceFormatCSV,
	SourceFormatEDI837,
	SourceFormatEDI835,
	SourceFormatCDA,
}

func (e SourceFormat) IsValid() bool {
	switch e {
	case SourceFormatHL7v2, SourceFormatFHIR, SourceFormatCSV,
		SourceFormatEDI837, SourceFormatEDI835, SourceFormatCDA:
		return true
	}
	return false
}

func (e SourceFormat) String() string {
	return string(e)
}

func (e *SourceFormat) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}
	*e = SourceFormat(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid SourceFormat", str)
	}
	return nil
}

func (e SourceFormat) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

// EventOrderField represents fields for ordering events
type EventOrderField string

const (
	EventOrderFieldTimestamp EventOrderField = "TIMESTAMP"
	EventOrderFieldType      EventOrderField = "TYPE"
	EventOrderFieldSource    EventOrderField = "SOURCE"
)

var AllEventOrderField = []EventOrderField{
	EventOrderFieldTimestamp,
	EventOrderFieldType,
	EventOrderFieldSource,
}

func (e EventOrderField) IsValid() bool {
	switch e {
	case EventOrderFieldTimestamp, EventOrderFieldType, EventOrderFieldSource:
		return true
	}
	return false
}

func (e EventOrderField) String() string {
	return string(e)
}

func (e *EventOrderField) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}
	*e = EventOrderField(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid EventOrderField", str)
	}
	return nil
}

func (e EventOrderField) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}

// OrderDirection represents sort direction
type OrderDirection string

const (
	OrderDirectionAsc  OrderDirection = "ASC"
	OrderDirectionDesc OrderDirection = "DESC"
)

var AllOrderDirection = []OrderDirection{
	OrderDirectionAsc,
	OrderDirectionDesc,
}

func (e OrderDirection) IsValid() bool {
	switch e {
	case OrderDirectionAsc, OrderDirectionDesc:
		return true
	}
	return false
}

func (e OrderDirection) String() string {
	return string(e)
}

func (e *OrderDirection) UnmarshalGQL(v interface{}) error {
	str, ok := v.(string)
	if !ok {
		return fmt.Errorf("enums must be strings")
	}
	*e = OrderDirection(str)
	if !e.IsValid() {
		return fmt.Errorf("%s is not a valid OrderDirection", str)
	}
	return nil
}

func (e OrderDirection) MarshalGQL(w io.Writer) {
	fmt.Fprint(w, strconv.Quote(e.String()))
}
