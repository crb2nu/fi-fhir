package matching

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MemoryMPI is an in-memory implementation of the MPI interface.
// Useful for testing and development.
type MemoryMPI struct {
	mu sync.RWMutex

	// records stores patient records by enterprise ID
	records map[string]*MPIRecord

	// identifierIndex maps identifier (type:value:system) to enterprise ID
	identifierIndex map[string]string

	// links stores links between enterprise IDs
	links []MPILink

	// auditLog stores audit events
	auditLog []MPIAuditEvent

	// matcher is used for search
	matcher *Matcher
}

// NewMemoryMPI creates a new in-memory MPI.
func NewMemoryMPI(config MatcherConfig) *MemoryMPI {
	return &MemoryMPI{
		records:         make(map[string]*MPIRecord),
		identifierIndex: make(map[string]string),
		links:           []MPILink{},
		auditLog:        []MPIAuditEvent{},
		matcher:         NewMatcher(config),
	}
}

// Add registers a new patient and returns an enterprise patient ID.
func (m *MemoryMPI) Add(ctx context.Context, patient *Patient) (*MPIRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for existing record by identifier
	if patient.SSN != "" {
		if existingID, found := m.identifierIndex[identifierKey("SS", patient.SSN, "")]; found {
			return m.records[existingID], nil
		}
	}
	if patient.MBI != "" {
		if existingID, found := m.identifierIndex[identifierKey("MC", patient.MBI, "")]; found {
			return m.records[existingID], nil
		}
	}
	if patient.MRN != "" && patient.MRNSystem != "" {
		if existingID, found := m.identifierIndex[identifierKey("MR", patient.MRN, patient.MRNSystem)]; found {
			return m.records[existingID], nil
		}
	}

	// Create new record
	enterpriseID := uuid.New().String()
	now := time.Now()

	record := &MPIRecord{
		EnterpriseID:  enterpriseID,
		Patient:       patient,
		Status:        StatusActive,
		CreatedAt:     now,
		UpdatedAt:     now,
		SourceRecords: []SourceRecord{},
	}

	// Add source records from identifiers
	if patient.SSN != "" {
		record.SourceRecords = append(record.SourceRecords, SourceRecord{
			Source:  "SSA",
			IDType:  "SS",
			IDValue: patient.SSN,
			AddedAt: now,
		})
		m.identifierIndex[identifierKey("SS", patient.SSN, "")] = enterpriseID
	}
	if patient.MBI != "" {
		record.SourceRecords = append(record.SourceRecords, SourceRecord{
			Source:  "CMS",
			IDType:  "MC",
			IDValue: patient.MBI,
			AddedAt: now,
		})
		m.identifierIndex[identifierKey("MC", patient.MBI, "")] = enterpriseID
	}
	if patient.MRN != "" {
		record.SourceRecords = append(record.SourceRecords, SourceRecord{
			Source:   patient.MRNSystem,
			IDType:   "MR",
			IDValue:  patient.MRN,
			IDSystem: patient.MRNSystem,
			AddedAt:  now,
		})
		m.identifierIndex[identifierKey("MR", patient.MRN, patient.MRNSystem)] = enterpriseID
	}

	m.records[enterpriseID] = record

	// Record audit event
	m.auditLog = append(m.auditLog, MPIAuditEvent{
		EventType:    "add",
		EnterpriseID: enterpriseID,
		Actor:        "system",
		Timestamp:    now,
	})

	return record, nil
}

// Get retrieves a patient by enterprise ID.
func (m *MemoryMPI) Get(ctx context.Context, enterpriseID string) (*MPIRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	record, found := m.records[enterpriseID]
	if !found {
		return nil, fmt.Errorf("patient not found: %s", enterpriseID)
	}

	// Follow merge chain
	for record.Status == StatusMerged && record.MergedInto != "" {
		merged, found := m.records[record.MergedInto]
		if !found {
			break
		}
		record = merged
	}

	return record, nil
}

// Search finds patients matching the given criteria.
func (m *MemoryMPI) Search(ctx context.Context, query *Patient, opts SearchOptions) ([]MPISearchResult, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []MPISearchResult

	for _, record := range m.records {
		// Skip based on status
		if record.Status == StatusMerged && !opts.IncludeMerged {
			continue
		}
		if record.Status == StatusInactive && !opts.IncludeInactive {
			continue
		}

		matchResult := m.matcher.Match(query, record.Patient)

		var score float64
		if matchResult.ProbabilisticScore != nil {
			score = matchResult.ProbabilisticScore.Score
		} else if matchResult.Result == MatchConfirmed {
			score = 1.0
		}

		if score >= opts.MinScore || matchResult.Result == MatchConfirmed || matchResult.Result == MatchProbable {
			results = append(results, MPISearchResult{
				Record:      record,
				MatchResult: matchResult,
				Score:       score,
			})
		}
	}

	// Sort by score descending
	for i := range results {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

	// Apply max results
	if opts.MaxResults > 0 && len(results) > opts.MaxResults {
		results = results[:opts.MaxResults]
	}

	return results, nil
}

// Link marks two patient records as the same person.
func (m *MemoryMPI) Link(ctx context.Context, enterpriseID1, enterpriseID2 string, linkType LinkType) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, found := m.records[enterpriseID1]; !found {
		return fmt.Errorf("patient not found: %s", enterpriseID1)
	}
	if _, found := m.records[enterpriseID2]; !found {
		return fmt.Errorf("patient not found: %s", enterpriseID2)
	}

	// Check if link already exists
	for _, link := range m.links {
		if (link.EnterpriseID1 == enterpriseID1 && link.EnterpriseID2 == enterpriseID2) ||
			(link.EnterpriseID1 == enterpriseID2 && link.EnterpriseID2 == enterpriseID1) {
			return nil // Already linked
		}
	}

	now := time.Now()
	m.links = append(m.links, MPILink{
		EnterpriseID1: enterpriseID1,
		EnterpriseID2: enterpriseID2,
		LinkType:      linkType,
		CreatedAt:     now,
		CreatedBy:     "system",
	})

	m.auditLog = append(m.auditLog, MPIAuditEvent{
		EventType:    "link",
		EnterpriseID: enterpriseID1,
		SecondaryID:  enterpriseID2,
		Actor:        "system",
		Timestamp:    now,
		Details:      map[string]any{"link_type": string(linkType)},
	})

	return nil
}

// Unlink removes a link between two patient records.
func (m *MemoryMPI) Unlink(ctx context.Context, enterpriseID1, enterpriseID2 string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	newLinks := []MPILink{}
	found := false

	for _, link := range m.links {
		if (link.EnterpriseID1 == enterpriseID1 && link.EnterpriseID2 == enterpriseID2) ||
			(link.EnterpriseID1 == enterpriseID2 && link.EnterpriseID2 == enterpriseID1) {
			found = true
			continue
		}
		newLinks = append(newLinks, link)
	}

	if !found {
		return fmt.Errorf("link not found between %s and %s", enterpriseID1, enterpriseID2)
	}

	m.links = newLinks

	m.auditLog = append(m.auditLog, MPIAuditEvent{
		EventType:    "unlink",
		EnterpriseID: enterpriseID1,
		SecondaryID:  enterpriseID2,
		Actor:        "system",
		Timestamp:    time.Now(),
	})

	return nil
}

// GetLinks returns all records linked to the given enterprise ID.
func (m *MemoryMPI) GetLinks(ctx context.Context, enterpriseID string) ([]MPILink, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []MPILink
	for _, link := range m.links {
		if link.EnterpriseID1 == enterpriseID || link.EnterpriseID2 == enterpriseID {
			results = append(results, link)
		}
	}

	return results, nil
}

// Merge combines two patient records into one master.
func (m *MemoryMPI) Merge(ctx context.Context, survivorID, victimID string, reason string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	survivor, found := m.records[survivorID]
	if !found {
		return fmt.Errorf("survivor not found: %s", survivorID)
	}
	victim, found := m.records[victimID]
	if !found {
		return fmt.Errorf("victim not found: %s", victimID)
	}

	if victim.Status == StatusMerged {
		return fmt.Errorf("patient %s already merged", victimID)
	}

	now := time.Now()

	// Move victim's source records to survivor
	survivor.SourceRecords = append(survivor.SourceRecords, victim.SourceRecords...)
	survivor.UpdatedAt = now

	// Mark victim as merged
	victim.Status = StatusMerged
	victim.MergedInto = survivorID
	victim.UpdatedAt = now

	// Update identifier index to point to survivor
	for _, sr := range victim.SourceRecords {
		key := identifierKey(sr.IDType, sr.IDValue, sr.IDSystem)
		m.identifierIndex[key] = survivorID
	}

	// Update any links pointing to victim to point to survivor
	for i := range m.links {
		if m.links[i].EnterpriseID1 == victimID {
			m.links[i].EnterpriseID1 = survivorID
		}
		if m.links[i].EnterpriseID2 == victimID {
			m.links[i].EnterpriseID2 = survivorID
		}
	}

	m.auditLog = append(m.auditLog, MPIAuditEvent{
		EventType:    "merge",
		EnterpriseID: survivorID,
		SecondaryID:  victimID,
		Actor:        "system",
		Timestamp:    now,
		Details:      map[string]any{"reason": reason},
	})

	return nil
}

// GetByIdentifier finds a patient by a specific identifier.
func (m *MemoryMPI) GetByIdentifier(ctx context.Context, idType, idValue, idSystem string) (*MPIRecord, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	key := identifierKey(idType, idValue, idSystem)
	enterpriseID, found := m.identifierIndex[key]
	if !found {
		return nil, fmt.Errorf("patient not found by identifier: %s:%s", idType, idValue)
	}

	record := m.records[enterpriseID]

	// Follow merge chain
	for record.Status == StatusMerged && record.MergedInto != "" {
		merged, found := m.records[record.MergedInto]
		if !found {
			break
		}
		record = merged
	}

	return record, nil
}

// Update modifies an existing patient record.
func (m *MemoryMPI) Update(ctx context.Context, enterpriseID string, patient *Patient) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	record, found := m.records[enterpriseID]
	if !found {
		return fmt.Errorf("patient not found: %s", enterpriseID)
	}

	if record.Status == StatusMerged {
		return fmt.Errorf("cannot update merged patient: %s", enterpriseID)
	}

	record.Patient = patient
	record.UpdatedAt = time.Now()

	m.auditLog = append(m.auditLog, MPIAuditEvent{
		EventType:    "update",
		EnterpriseID: enterpriseID,
		Actor:        "system",
		Timestamp:    time.Now(),
	})

	return nil
}

// GetMetrics returns MPI metrics.
func (m *MemoryMPI) GetMetrics() MPIMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics := MPIMetrics{
		TotalRecords: int64(len(m.records)),
		TotalLinks:   int64(len(m.links)),
	}

	for _, record := range m.records {
		switch record.Status {
		case StatusActive:
			metrics.ActiveRecords++
		case StatusMerged:
			metrics.MergedRecords++
		}
	}

	for _, link := range m.links {
		switch link.LinkType {
		case LinkConfirmed, LinkAutomatic:
			metrics.ConfirmedLinks++
		case LinkPossible:
			metrics.PossibleLinks++
		}
	}

	return metrics
}

// GetAuditLog returns the audit log.
func (m *MemoryMPI) GetAuditLog() []MPIAuditEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]MPIAuditEvent, len(m.auditLog))
	copy(result, m.auditLog)
	return result
}

// identifierKey creates a unique key for an identifier.
func identifierKey(idType, idValue, idSystem string) string {
	return idType + ":" + idValue + ":" + idSystem
}

// Verify MemoryMPI implements MPI interface
var _ MPI = (*MemoryMPI)(nil)
