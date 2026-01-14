// Package projections provides a service layer for accessing event sourcing projections.
package projections

import (
	"context"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/eventsourcing"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/eventsourcing/projections"
)

// Service provides access to all projection read models.
// It wraps the underlying projections and provides GraphQL-friendly query methods.
type Service struct {
	timeline   *projections.PatientTimelineProjection
	statistics *projections.EventStatisticsProjection
	encounters *projections.ActiveEncountersProjection
	runner     *eventsourcing.ProjectionRunner
}

// NewService creates a new projection service with the given event store.
// If store is nil, projections will work in standalone mode (manual Handle calls).
func NewService(store eventsourcing.EventStore) *Service {
	timeline := projections.NewPatientTimelineProjection()
	statistics := projections.NewEventStatisticsProjection()
	encounters := projections.NewActiveEncountersProjection()

	s := &Service{
		timeline:   timeline,
		statistics: statistics,
		encounters: encounters,
	}

	// If a store is provided, set up a projection runner
	if store != nil {
		checkpointStore := eventsourcing.NewMemoryCheckpointStore()
		config := eventsourcing.ProjectionRunnerConfig{
			BatchSize:    100,
			PollInterval: time.Second,
		}
		runner := eventsourcing.NewProjectionRunner(store, checkpointStore, config)
		runner.RegisterProjection(timeline)
		runner.RegisterProjection(statistics)
		runner.RegisterProjection(encounters)
		s.runner = runner
	}

	return s
}

// Start begins running projections in the background.
// This should be called after creating the service if using an event store.
func (s *Service) Start(ctx context.Context) error {
	if s.runner == nil {
		return nil // No runner configured
	}
	go func() {
		_ = s.runner.Run(ctx) // Error is ctx.Canceled when context is done
	}()
	return nil
}

// HandleEvent manually processes an event through all projections.
// Use this for testing or when not using the projection runner.
func (s *Service) HandleEvent(ctx context.Context, event eventsourcing.StoredEvent) error {
	if err := s.timeline.Handle(ctx, event); err != nil {
		return err
	}
	if err := s.statistics.Handle(ctx, event); err != nil {
		return err
	}
	if err := s.encounters.Handle(ctx, event); err != nil {
		return err
	}
	return nil
}

// =============================================================================
// Patient Timeline Queries
// =============================================================================

// GetPatientTimeline returns the timeline for a patient.
func (s *Service) GetPatientTimeline(mrn string, from, to *time.Time, limit int) (*model.PatientTimeline, error) {
	var fromTime, toTime time.Time
	if from != nil {
		fromTime = *from
	}
	if to != nil {
		toTime = *to
	}

	// Get timeline with optional time range
	var events []projections.TimelineEvent
	if fromTime.IsZero() && toTime.IsZero() {
		timeline, ok := s.timeline.GetTimeline(mrn)
		if !ok {
			return nil, nil // Not found
		}
		events = timeline.Events
	} else {
		var ok bool
		events, ok = s.timeline.GetTimelineRange(mrn, fromTime, toTime)
		if !ok {
			return nil, nil
		}
	}

	// Apply limit
	if limit > 0 && len(events) > limit {
		events = events[:limit]
	}

	// Convert to GraphQL model
	result := &model.PatientTimeline{
		MRN:         mrn,
		Events:      make([]model.TimelineEvent, len(events)),
		LastUpdated: time.Now(),
		EventCount:  len(events),
	}

	for i, e := range events {
		source := e.Source
		result.Events[i] = model.TimelineEvent{
			Position:  int(e.Position),
			Timestamp: e.Timestamp,
			EventType: e.EventType,
			Summary:   e.Summary,
			StreamID:  e.StreamID,
			Source:    &source,
		}
	}

	return result, nil
}

// =============================================================================
// Event Statistics Queries
// =============================================================================

// GetEventStatistics returns aggregate event statistics.
func (s *Service) GetEventStatistics() (*model.EventStatistics, error) {
	stats := s.statistics.GetStatistics()

	// Convert to GraphQL model
	result := &model.EventStatistics{
		TotalEvents: int(stats.TotalEvents),
		ByType:      make([]model.EventTypeCount, 0, len(stats.ByType)),
		BySource:    make([]model.SourceCount, 0, len(stats.BySource)),
	}

	for eventType, count := range stats.ByType {
		result.ByType = append(result.ByType, model.EventTypeCount{
			EventType: eventType,
			Count:     int(count),
		})
	}

	for source, count := range stats.BySource {
		result.BySource = append(result.BySource, model.SourceCount{
			Source: source,
			Count:  int(count),
		})
	}

	return result, nil
}

// =============================================================================
// Active Encounters Queries
// =============================================================================

// GetActiveEncounters returns all active encounters with optional filters.
func (s *Service) GetActiveEncounters(location, unit, class *string) ([]model.ActiveEncounter, error) {
	var encounters []projections.ActiveEncounter

	// Apply filters
	if location != nil && *location != "" {
		encounters = s.encounters.GetEncountersByLocation(*location)
	} else if class != nil && *class != "" {
		encounters = s.encounters.GetEncountersByClass(*class)
	} else {
		encounters = s.encounters.GetAllEncounters()
	}

	// Filter by unit if specified
	if unit != nil && *unit != "" {
		filtered := make([]projections.ActiveEncounter, 0)
		for _, enc := range encounters {
			if enc.Unit == *unit {
				filtered = append(filtered, enc)
			}
		}
		encounters = filtered
	}

	// Convert to GraphQL model
	result := make([]model.ActiveEncounter, len(encounters))
	for i, enc := range encounters {
		result[i] = s.convertActiveEncounter(enc)
	}

	return result, nil
}

// GetActiveEncounter returns a specific active encounter by ID.
func (s *Service) GetActiveEncounter(id string) (*model.ActiveEncounter, error) {
	enc, ok := s.encounters.GetEncounter(id)
	if !ok {
		return nil, nil
	}

	result := s.convertActiveEncounter(*enc)
	return &result, nil
}

// GetActiveEncounterByPatient returns the active encounter for a patient.
func (s *Service) GetActiveEncounterByPatient(mrn string) (*model.ActiveEncounter, error) {
	enc, ok := s.encounters.GetEncounterByPatient(mrn)
	if !ok {
		return nil, nil
	}

	result := s.convertActiveEncounter(*enc)
	return &result, nil
}

func (s *Service) convertActiveEncounter(enc projections.ActiveEncounter) model.ActiveEncounter {
	result := model.ActiveEncounter{
		ID:          enc.ID,
		PatientMRN:  enc.PatientMRN,
		Class:       enc.Class,
		AdmitTime:   enc.AdmitTime,
		LastUpdated: enc.LastUpdated,
	}

	if enc.PatientName != "" {
		result.PatientName = &enc.PatientName
	}
	if enc.Location != "" {
		result.Location = &enc.Location
	}
	if enc.Unit != "" {
		result.Unit = &enc.Unit
	}
	if enc.Room != "" {
		result.Room = &enc.Room
	}
	if enc.Bed != "" {
		result.Bed = &enc.Bed
	}
	if enc.Provider != "" {
		result.Provider = &enc.Provider
	}

	return result
}

// =============================================================================
// Projection Status Queries
// =============================================================================

// GetProjectionStatus returns the status of all projections.
func (s *Service) GetProjectionStatus() ([]model.ProjectionStatus, error) {
	// Since we're using in-memory projections, we report basic status
	// A more sophisticated implementation would track checkpoint positions

	statuses := []model.ProjectionStatus{
		{
			Name:         "patient_timeline",
			Checkpoint:   0, // Would come from checkpoint store in production
			LastPosition: 0,
			Behind:       0,
			Status:       "running",
		},
		{
			Name:         "event_statistics",
			Checkpoint:   0,
			LastPosition: 0,
			Behind:       0,
			Status:       "running",
		},
		{
			Name:         "active_encounters",
			Checkpoint:   0,
			LastPosition: 0,
			Behind:       0,
			Status:       "running",
		},
	}

	return statuses, nil
}
