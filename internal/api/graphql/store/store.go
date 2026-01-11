package store

import (
	"context"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/crb2nu/fi-fhir/internal/api/graphql/model"
)

// EventStore provides storage and retrieval of healthcare events.
type EventStore interface {
	// SaveEvent stores an event and returns its ID.
	SaveEvent(ctx context.Context, event model.Event) (string, error)

	// GetEvent retrieves a single event by ID.
	GetEvent(ctx context.Context, id string) (model.Event, error)

	// QueryEvents retrieves events matching the filter with pagination.
	QueryEvents(ctx context.Context, filter *model.EventFilter, first int, after *string, orderBy *model.EventOrderBy) (*model.EventConnection, error)

	// GetPatient retrieves a patient by MRN.
	GetPatient(ctx context.Context, mrn string) (*model.Patient, error)

	// QueryPatients retrieves patients matching the filter with pagination.
	QueryPatients(ctx context.Context, filter *model.PatientFilter, first int, after *string) (*model.PatientConnection, error)

	// Subscribe creates a channel for receiving events matching the filter.
	Subscribe(ctx context.Context, filter *model.EventFilter) (<-chan model.Event, error)

	// SubscribePatient creates a channel for events related to a specific patient.
	SubscribePatient(ctx context.Context, mrn string) (<-chan model.Event, error)
}

// MemoryStore is an in-memory implementation of EventStore.
// Suitable for development and testing.
type MemoryStore struct {
	mu          sync.RWMutex
	events      []model.Event
	eventsByID  map[string]model.Event
	patients    map[string]*model.Patient
	nextID      int64
	subscribers []chan model.Event
}

// NewMemoryStore creates a new in-memory event store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		events:     make([]model.Event, 0),
		eventsByID: make(map[string]model.Event),
		patients:   make(map[string]*model.Patient),
	}
}

// SaveEvent stores an event in memory.
func (s *MemoryStore) SaveEvent(ctx context.Context, event model.Event) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Use provided ID or generate one
	id := event.GetID()
	if id == "" {
		s.nextID++
		id = fmt.Sprintf("evt_%d", s.nextID)
	}

	// Store the event
	s.events = append(s.events, event)
	s.eventsByID[id] = event

	// Notify subscribers
	for _, ch := range s.subscribers {
		select {
		case ch <- event:
		default:
			// Skip if subscriber is not ready
		}
	}

	return id, nil
}

// GetEvent retrieves an event by ID.
func (s *MemoryStore) GetEvent(ctx context.Context, id string) (model.Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	event, ok := s.eventsByID[id]
	if !ok {
		return nil, fmt.Errorf("event not found: %s", id)
	}
	return event, nil
}

// QueryEvents retrieves events with filtering and pagination.
func (s *MemoryStore) QueryEvents(ctx context.Context, filter *model.EventFilter, first int, after *string, orderBy *model.EventOrderBy) (*model.EventConnection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Filter events
	filtered := make([]model.Event, 0)
	for _, event := range s.events {
		if matchesFilter(event, filter) {
			filtered = append(filtered, event)
		}
	}

	// Sort events
	sortEvents(filtered, orderBy)

	// Apply cursor-based pagination
	startIdx := 0
	if after != nil && *after != "" {
		afterID, err := decodeCursor(*after)
		if err == nil {
			for i, e := range filtered {
				if e.GetID() == afterID {
					startIdx = i + 1
					break
				}
			}
		}
	}

	// Apply limit
	endIdx := startIdx + first
	if endIdx > len(filtered) {
		endIdx = len(filtered)
	}

	// Build edges
	edges := make([]model.EventEdge, 0, endIdx-startIdx)
	for i := startIdx; i < endIdx; i++ {
		edges = append(edges, model.EventEdge{
			Cursor: encodeCursor(filtered[i].GetID()),
			Node:   filtered[i],
		})
	}

	// Build page info
	var startCursor, endCursor *string
	if len(edges) > 0 {
		startCursor = &edges[0].Cursor
		endCursor = &edges[len(edges)-1].Cursor
	}

	return &model.EventConnection{
		Edges: edges,
		PageInfo: model.PageInfo{
			HasNextPage:     endIdx < len(filtered),
			HasPreviousPage: startIdx > 0,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(filtered),
	}, nil
}

// GetPatient retrieves a patient by MRN.
func (s *MemoryStore) GetPatient(ctx context.Context, mrn string) (*model.Patient, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	patient, ok := s.patients[mrn]
	if !ok {
		return nil, fmt.Errorf("patient not found: %s", mrn)
	}
	return patient, nil
}

// QueryPatients retrieves patients with filtering and pagination.
func (s *MemoryStore) QueryPatients(ctx context.Context, filter *model.PatientFilter, first int, after *string) (*model.PatientConnection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Filter patients
	filtered := make([]*model.Patient, 0)
	for _, patient := range s.patients {
		if matchesPatientFilter(patient, filter) {
			filtered = append(filtered, patient)
		}
	}

	// Sort by MRN for consistency
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].MRN < filtered[j].MRN
	})

	// Apply cursor-based pagination
	startIdx := 0
	if after != nil && *after != "" {
		afterID, err := decodeCursor(*after)
		if err == nil {
			for i, p := range filtered {
				if p.MRN == afterID {
					startIdx = i + 1
					break
				}
			}
		}
	}

	// Apply limit
	endIdx := startIdx + first
	if endIdx > len(filtered) {
		endIdx = len(filtered)
	}

	// Build edges
	edges := make([]model.PatientEdge, 0, endIdx-startIdx)
	for i := startIdx; i < endIdx; i++ {
		edges = append(edges, model.PatientEdge{
			Cursor: encodeCursor(filtered[i].MRN),
			Node:   *filtered[i],
		})
	}

	// Build page info
	var startCursor, endCursor *string
	if len(edges) > 0 {
		startCursor = &edges[0].Cursor
		endCursor = &edges[len(edges)-1].Cursor
	}

	return &model.PatientConnection{
		Edges: edges,
		PageInfo: model.PageInfo{
			HasNextPage:     endIdx < len(filtered),
			HasPreviousPage: startIdx > 0,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: len(filtered),
	}, nil
}

// Subscribe creates a channel for receiving events.
func (s *MemoryStore) Subscribe(ctx context.Context, filter *model.EventFilter) (<-chan model.Event, error) {
	ch := make(chan model.Event, 100)

	s.mu.Lock()
	s.subscribers = append(s.subscribers, ch)
	s.mu.Unlock()

	// Clean up on context cancellation
	go func() {
		<-ctx.Done()
		s.mu.Lock()
		for i, sub := range s.subscribers {
			if sub == ch {
				s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
		close(ch)
	}()

	// Create filtered channel
	filtered := make(chan model.Event, 100)
	go func() {
		for event := range ch {
			if matchesFilter(event, filter) {
				select {
				case filtered <- event:
				case <-ctx.Done():
					return
				}
			}
		}
		close(filtered)
	}()

	return filtered, nil
}

// SubscribePatient creates a channel for patient-specific events.
func (s *MemoryStore) SubscribePatient(ctx context.Context, mrn string) (<-chan model.Event, error) {
	filter := &model.EventFilter{
		PatientMrn: &mrn,
	}
	return s.Subscribe(ctx, filter)
}

// SavePatient stores or updates a patient record.
func (s *MemoryStore) SavePatient(patient *model.Patient) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.patients[patient.MRN] = patient
}

// Helper functions

func matchesFilter(event model.Event, filter *model.EventFilter) bool {
	if filter == nil {
		return true
	}

	// Filter by event types
	if len(filter.Types) > 0 {
		typeMatch := false
		for _, t := range filter.Types {
			if event.GetType() == t {
				typeMatch = true
				break
			}
		}
		if !typeMatch {
			return false
		}
	}

	// Filter by source
	if len(filter.Sources) > 0 {
		sourceMatch := false
		for _, src := range filter.Sources {
			if event.GetSource() == src {
				sourceMatch = true
				break
			}
		}
		if !sourceMatch {
			return false
		}
	}

	// Filter by timestamp range
	if filter.FromTimestamp != nil && event.GetTimestamp().Before(*filter.FromTimestamp) {
		return false
	}
	if filter.ToTimestamp != nil && event.GetTimestamp().After(*filter.ToTimestamp) {
		return false
	}

	// Filter by correlation ID
	if filter.CorrelationID != nil {
		corrID := event.GetCorrelationID()
		if corrID == nil || *corrID != *filter.CorrelationID {
			return false
		}
	}

	// Filter by patient MRN - need to check the specific event type
	if filter.PatientMrn != nil {
		patientMRN := getEventPatientMRN(event)
		if patientMRN != *filter.PatientMrn {
			return false
		}
	}

	return true
}

func getEventPatientMRN(event model.Event) string {
	switch e := event.(type) {
	case *model.PatientAdmitEvent:
		return e.Patient.MRN
	case *model.PatientDischargeEvent:
		return e.Patient.MRN
	case *model.LabResultEvent:
		return e.Patient.MRN
	case *model.VitalSignEvent:
		return e.Patient.MRN
	case *model.ConditionEvent:
		return e.Patient.MRN
	case *model.ProcedureEvent:
		return e.Patient.MRN
	case *model.ImmunizationEvent:
		return e.Patient.MRN
	case *model.AppointmentEvent:
		return e.Patient.MRN
	case *model.DocumentEvent:
		if e.Patient != nil {
			return e.Patient.MRN
		}
	}
	return ""
}

func matchesPatientFilter(patient *model.Patient, filter *model.PatientFilter) bool {
	if filter == nil {
		return true
	}

	if filter.MRN != nil && !strings.Contains(strings.ToLower(patient.MRN), strings.ToLower(*filter.MRN)) {
		return false
	}

	if filter.FamilyName != nil && !strings.Contains(strings.ToLower(patient.FamilyName), strings.ToLower(*filter.FamilyName)) {
		return false
	}

	if filter.GivenName != nil && !strings.Contains(strings.ToLower(patient.GivenName), strings.ToLower(*filter.GivenName)) {
		return false
	}

	if filter.DateOfBirth != nil && patient.DateOfBirth != nil {
		if !sameDay(*filter.DateOfBirth, *patient.DateOfBirth) {
			return false
		}
	}

	return true
}

func sameDay(a, b time.Time) bool {
	return a.Year() == b.Year() && a.Month() == b.Month() && a.Day() == b.Day()
}

func sortEvents(events []model.Event, orderBy *model.EventOrderBy) {
	if orderBy == nil {
		// Default: sort by timestamp descending
		sort.Slice(events, func(i, j int) bool {
			return events[i].GetTimestamp().After(events[j].GetTimestamp())
		})
		return
	}

	asc := orderBy.Direction == model.OrderDirectionAsc

	sort.Slice(events, func(i, j int) bool {
		var less bool
		switch orderBy.Field {
		case model.EventOrderFieldTimestamp:
			less = events[i].GetTimestamp().Before(events[j].GetTimestamp())
		case model.EventOrderFieldType:
			less = string(events[i].GetType()) < string(events[j].GetType())
		case model.EventOrderFieldSource:
			less = events[i].GetSource() < events[j].GetSource()
		default:
			less = events[i].GetTimestamp().Before(events[j].GetTimestamp())
		}

		if asc {
			return less
		}
		return !less
	})
}

func encodeCursor(id string) string {
	return base64.StdEncoding.EncodeToString([]byte(id))
}

func decodeCursor(cursor string) (string, error) {
	data, err := base64.StdEncoding.DecodeString(cursor)
	if err != nil {
		return "", err
	}
	return string(data), nil
}
