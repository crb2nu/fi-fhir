package projections

import (
	"context"
	"sync"
	"time"

	"github.com/cblevins/fi-fhir/pkg/eventsourcing"
)

// EventStatistics is the read model for event statistics.
type EventStatistics struct {
	TotalEvents    int64                      `json:"total_events"`
	ByType         map[string]int64           `json:"by_type"`
	BySource       map[string]int64           `json:"by_source"`
	ByHour         map[string]int64           `json:"by_hour"`          // "2024-01-15T10" -> count
	ByTypeAndSource map[string]map[string]int64 `json:"by_type_and_source"` // type -> source -> count
	LastUpdated    time.Time                  `json:"last_updated"`
	LastPosition   int64                      `json:"last_position"`
}

// EventStatisticsProjection aggregates event counts by various dimensions.
type EventStatisticsProjection struct {
	stats *EventStatistics
	mu    sync.RWMutex
}

// NewEventStatisticsProjection creates a new event statistics projection.
func NewEventStatisticsProjection() *EventStatisticsProjection {
	return &EventStatisticsProjection{
		stats: &EventStatistics{
			ByType:          make(map[string]int64),
			BySource:        make(map[string]int64),
			ByHour:          make(map[string]int64),
			ByTypeAndSource: make(map[string]map[string]int64),
		},
	}
}

// Name returns the projection name.
func (p *EventStatisticsProjection) Name() string {
	return "event_statistics"
}

// Handle processes an event and updates statistics.
func (p *EventStatisticsProjection) Handle(ctx context.Context, event eventsourcing.StoredEvent) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Update total count
	p.stats.TotalEvents++

	// Update by type
	p.stats.ByType[event.EventType]++

	// Update by source
	source := event.Metadata["source"]
	if source == "" {
		source = "unknown"
	}
	p.stats.BySource[source]++

	// Update by hour
	hourKey := event.Timestamp.Format("2006-01-02T15")
	p.stats.ByHour[hourKey]++

	// Update by type and source
	if p.stats.ByTypeAndSource[event.EventType] == nil {
		p.stats.ByTypeAndSource[event.EventType] = make(map[string]int64)
	}
	p.stats.ByTypeAndSource[event.EventType][source]++

	// Update metadata
	p.stats.LastUpdated = time.Now()
	p.stats.LastPosition = event.Position

	return nil
}

// GetStatistics returns the current statistics.
func (p *EventStatisticsProjection) GetStatistics() EventStatistics {
	p.mu.RLock()
	defer p.mu.RUnlock()

	// Return a deep copy
	result := EventStatistics{
		TotalEvents:    p.stats.TotalEvents,
		ByType:         make(map[string]int64),
		BySource:       make(map[string]int64),
		ByHour:         make(map[string]int64),
		ByTypeAndSource: make(map[string]map[string]int64),
		LastUpdated:    p.stats.LastUpdated,
		LastPosition:   p.stats.LastPosition,
	}

	for k, v := range p.stats.ByType {
		result.ByType[k] = v
	}
	for k, v := range p.stats.BySource {
		result.BySource[k] = v
	}
	for k, v := range p.stats.ByHour {
		result.ByHour[k] = v
	}
	for eventType, sources := range p.stats.ByTypeAndSource {
		result.ByTypeAndSource[eventType] = make(map[string]int64)
		for source, count := range sources {
			result.ByTypeAndSource[eventType][source] = count
		}
	}

	return result
}

// GetEventCountByType returns the count for a specific event type.
func (p *EventStatisticsProjection) GetEventCountByType(eventType string) int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats.ByType[eventType]
}

// GetEventCountBySource returns the count for a specific source.
func (p *EventStatisticsProjection) GetEventCountBySource(source string) int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.stats.BySource[source]
}

// GetTopEventTypes returns the top N event types by count.
func (p *EventStatisticsProjection) GetTopEventTypes(n int) []TypeCount {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return topN(p.stats.ByType, n)
}

// GetTopSources returns the top N sources by count.
func (p *EventStatisticsProjection) GetTopSources(n int) []TypeCount {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return topN(p.stats.BySource, n)
}

// TypeCount represents a type/source with its count.
type TypeCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// topN returns the top N items from a count map.
func topN(counts map[string]int64, n int) []TypeCount {
	result := make([]TypeCount, 0, len(counts))
	for name, count := range counts {
		result = append(result, TypeCount{Name: name, Count: count})
	}

	// Sort by count descending (simple bubble sort for small N)
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j].Count > result[i].Count {
				result[i], result[j] = result[j], result[i]
			}
		}
	}

	if len(result) > n {
		result = result[:n]
	}

	return result
}

// Clear resets the projection (for testing/rebuilding).
func (p *EventStatisticsProjection) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stats = &EventStatistics{
		ByType:          make(map[string]int64),
		BySource:        make(map[string]int64),
		ByHour:          make(map[string]int64),
		ByTypeAndSource: make(map[string]map[string]int64),
	}
}
