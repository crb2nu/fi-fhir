package store

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	WorkflowDefinitionStatusDraft    = "draft"
	WorkflowDefinitionStatusArchived = "archived"

	WorkflowApprovalStatusPending  = "pending"
	WorkflowApprovalStatusApproved = "approved"
	WorkflowApprovalStatusRejected = "rejected"

	WorkflowRunStatusSuccess = "success"
	WorkflowRunStatusFailed  = "failed"
)

// Paging defines common list pagination options.
type Paging struct {
	Limit  int
	Offset int
}

// WorkflowDefinitionListFilter filters workflow definitions.
type WorkflowDefinitionListFilter struct {
	Name   *string
	Status *string
}

// WorkflowRunListFilter filters workflow run history.
type WorkflowRunListFilter struct {
	WorkflowName  *string
	Environment   *string
	Status        *string
	FromStartedAt *time.Time
	ToStartedAt   *time.Time
}

// WorkflowApprovalRequestListFilter filters approval requests.
type WorkflowApprovalRequestListFilter struct {
	WorkflowID  *string
	Environment *string
	Status      *string
}

// WorkflowValidationRecord stores validation data for a saved version.
type WorkflowValidationRecord struct {
	Valid    bool
	Errors   []string
	Warnings []string
	Info     []string
}

// WorkflowDefinitionRecord stores managed workflow metadata.
type WorkflowDefinitionRecord struct {
	ID          string
	Name        string
	Description string
	Status      string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// WorkflowVersionRecord stores immutable workflow YAML versions.
type WorkflowVersionRecord struct {
	ID            string
	WorkflowID    string
	VersionNumber int
	Yaml          string
	Validation    WorkflowValidationRecord
	CreatedBy     string
	CreatedAt     time.Time
	Notes         string
}

// WorkflowReleaseRecord points an environment at an immutable workflow version.
type WorkflowReleaseRecord struct {
	ID                    string
	WorkflowID            string
	Environment           string
	VersionID             string
	PublishedBy           string
	PublishedAt           time.Time
	RollbackFromReleaseID *string
}

// WorkflowRunRecord stores execution metadata for a trigger.
type WorkflowRunRecord struct {
	ID              string
	WorkflowID      string
	WorkflowName    string
	Environment     string
	VersionID       *string
	EventID         *string
	RoutesMatched   int
	ActionsExecuted int
	Errors          []string
	DurationMs      int
	StartedAt       time.Time
	Status          string
}

// WorkflowApprovalRequestRecord tracks governance state for promotions.
type WorkflowApprovalRequestRecord struct {
	ID              string
	WorkflowID      string
	TargetVersionID string
	Environment     string
	Status          string
	RequestedBy     string
	ReviewedBy      *string
	ReviewedAt      *time.Time
	Comment         *string
}

// WorkflowAuditLogRecord stores lifecycle audit events.
type WorkflowAuditLogRecord struct {
	ID         string
	WorkflowID string
	EventType  string
	Actor      string
	OccurredAt time.Time
	Metadata   map[string]any
}

// WorkflowLifecycleStore defines persistence for workflow lifecycle management.
type WorkflowLifecycleStore interface {
	CreateWorkflowDefinition(ctx context.Context, def *WorkflowDefinitionRecord) (*WorkflowDefinitionRecord, error)
	UpdateWorkflowDefinition(ctx context.Context, def *WorkflowDefinitionRecord) (*WorkflowDefinitionRecord, error)
	ArchiveWorkflowDefinition(ctx context.Context, workflowID string) (*WorkflowDefinitionRecord, error)
	GetWorkflowDefinitionByID(ctx context.Context, workflowID string) (*WorkflowDefinitionRecord, error)
	GetWorkflowDefinitionByName(ctx context.Context, name string) (*WorkflowDefinitionRecord, error)
	ListWorkflowDefinitions(ctx context.Context, filter WorkflowDefinitionListFilter, paging Paging) ([]*WorkflowDefinitionRecord, error)

	SaveWorkflowVersion(ctx context.Context, version *WorkflowVersionRecord) (*WorkflowVersionRecord, error)
	GetWorkflowVersion(ctx context.Context, versionID string) (*WorkflowVersionRecord, error)
	GetWorkflowVersionForWorkflow(ctx context.Context, workflowID, versionID string) (*WorkflowVersionRecord, error)
	ListWorkflowVersions(ctx context.Context, workflowID string, paging Paging) ([]*WorkflowVersionRecord, error)
	GetLatestWorkflowVersion(ctx context.Context, workflowID string) (*WorkflowVersionRecord, error)

	PublishWorkflowVersion(ctx context.Context, release *WorkflowReleaseRecord) (*WorkflowReleaseRecord, error)
	GetWorkflowRelease(ctx context.Context, releaseID string) (*WorkflowReleaseRecord, error)
	GetPublishedWorkflowRelease(ctx context.Context, workflowID, environment string) (*WorkflowReleaseRecord, error)
	ListWorkflowReleases(ctx context.Context, workflowID string) ([]*WorkflowReleaseRecord, error)

	CreateWorkflowRun(ctx context.Context, run *WorkflowRunRecord) (*WorkflowRunRecord, error)
	GetWorkflowRun(ctx context.Context, runID string) (*WorkflowRunRecord, error)
	ListWorkflowRuns(ctx context.Context, filter WorkflowRunListFilter, paging Paging) ([]*WorkflowRunRecord, error)

	CreateWorkflowApprovalRequest(ctx context.Context, req *WorkflowApprovalRequestRecord) (*WorkflowApprovalRequestRecord, error)
	UpdateWorkflowApprovalRequest(ctx context.Context, req *WorkflowApprovalRequestRecord) (*WorkflowApprovalRequestRecord, error)
	GetWorkflowApprovalRequest(ctx context.Context, approvalID string) (*WorkflowApprovalRequestRecord, error)
	ListWorkflowApprovalRequests(ctx context.Context, filter WorkflowApprovalRequestListFilter, paging Paging) ([]*WorkflowApprovalRequestRecord, error)

	CreateWorkflowAuditLog(ctx context.Context, entry *WorkflowAuditLogRecord) (*WorkflowAuditLogRecord, error)
}

// MemoryWorkflowLifecycleStore is an in-memory workflow lifecycle store.
type MemoryWorkflowLifecycleStore struct {
	mu sync.RWMutex

	definitionsByID   map[string]*WorkflowDefinitionRecord
	definitionByName  map[string]string // lowercase name -> id
	versionsByID      map[string]*WorkflowVersionRecord
	versionIDsByDefID map[string][]string
	releasesByID      map[string]*WorkflowReleaseRecord
	publishedByDefEnv map[string]map[string]string // def id -> env(lowercase) -> release id
	runsByID          map[string]*WorkflowRunRecord
	runIDs            []string
	approvalsByID     map[string]*WorkflowApprovalRequestRecord
	approvalIDs       []string
	auditByID         map[string]*WorkflowAuditLogRecord
	auditIDs          []string

	nextDefinitionID int64
	nextVersionID    int64
	nextReleaseID    int64
	nextRunID        int64
	nextApprovalID   int64
	nextAuditID      int64
}

// NewMemoryWorkflowLifecycleStore creates an in-memory lifecycle store.
func NewMemoryWorkflowLifecycleStore() *MemoryWorkflowLifecycleStore {
	return &MemoryWorkflowLifecycleStore{
		definitionsByID:   make(map[string]*WorkflowDefinitionRecord),
		definitionByName:  make(map[string]string),
		versionsByID:      make(map[string]*WorkflowVersionRecord),
		versionIDsByDefID: make(map[string][]string),
		releasesByID:      make(map[string]*WorkflowReleaseRecord),
		publishedByDefEnv: make(map[string]map[string]string),
		runsByID:          make(map[string]*WorkflowRunRecord),
		approvalsByID:     make(map[string]*WorkflowApprovalRequestRecord),
		auditByID:         make(map[string]*WorkflowAuditLogRecord),
	}
}

func normalizePaging(paging Paging) (limit, offset int) {
	limit = paging.Limit
	offset = paging.Offset
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

func pagedSlice[T any](items []T, paging Paging) []T {
	limit, offset := normalizePaging(paging)
	if offset >= len(items) {
		return []T{}
	}
	end := offset + limit
	if end > len(items) {
		end = len(items)
	}
	out := make([]T, end-offset)
	copy(out, items[offset:end])
	return out
}

func strEqFoldPtr(ptr *string, value string) bool {
	if ptr == nil || *ptr == "" {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(*ptr), value)
}

func envKey(environment string) string {
	return strings.ToLower(strings.TrimSpace(environment))
}

func cloneStringPtr(in *string) *string {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func cloneTimePtr(in *time.Time) *time.Time {
	if in == nil {
		return nil
	}
	v := *in
	return &v
}

func cloneMap(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneValidation(v WorkflowValidationRecord) WorkflowValidationRecord {
	out := WorkflowValidationRecord{
		Valid:    v.Valid,
		Errors:   append([]string(nil), v.Errors...),
		Warnings: append([]string(nil), v.Warnings...),
		Info:     append([]string(nil), v.Info...),
	}
	return out
}

func cloneDefinition(def *WorkflowDefinitionRecord) *WorkflowDefinitionRecord {
	if def == nil {
		return nil
	}
	cp := *def
	return &cp
}

func cloneVersion(v *WorkflowVersionRecord) *WorkflowVersionRecord {
	if v == nil {
		return nil
	}
	cp := *v
	cp.Validation = cloneValidation(v.Validation)
	return &cp
}

func cloneRelease(r *WorkflowReleaseRecord) *WorkflowReleaseRecord {
	if r == nil {
		return nil
	}
	cp := *r
	cp.RollbackFromReleaseID = cloneStringPtr(r.RollbackFromReleaseID)
	return &cp
}

func cloneRun(r *WorkflowRunRecord) *WorkflowRunRecord {
	if r == nil {
		return nil
	}
	cp := *r
	cp.VersionID = cloneStringPtr(r.VersionID)
	cp.EventID = cloneStringPtr(r.EventID)
	cp.Errors = append([]string(nil), r.Errors...)
	return &cp
}

func cloneApproval(a *WorkflowApprovalRequestRecord) *WorkflowApprovalRequestRecord {
	if a == nil {
		return nil
	}
	cp := *a
	cp.ReviewedBy = cloneStringPtr(a.ReviewedBy)
	cp.ReviewedAt = cloneTimePtr(a.ReviewedAt)
	cp.Comment = cloneStringPtr(a.Comment)
	return &cp
}

func cloneAudit(a *WorkflowAuditLogRecord) *WorkflowAuditLogRecord {
	if a == nil {
		return nil
	}
	cp := *a
	cp.Metadata = cloneMap(a.Metadata)
	return &cp
}

// CreateWorkflowDefinition creates a new managed workflow definition.
func (s *MemoryWorkflowLifecycleStore) CreateWorkflowDefinition(_ context.Context, def *WorkflowDefinitionRecord) (*WorkflowDefinitionRecord, error) {
	if def == nil {
		return nil, fmt.Errorf("workflow definition is required")
	}
	name := strings.TrimSpace(def.Name)
	if name == "" {
		return nil, fmt.Errorf("workflow definition name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if existingID, ok := s.definitionByName[strings.ToLower(name)]; ok && existingID != "" {
		return nil, fmt.Errorf("workflow definition name already exists: %s", name)
	}

	s.nextDefinitionID++
	id := fmt.Sprintf("wf_%d", s.nextDefinitionID)
	now := time.Now().UTC()

	record := &WorkflowDefinitionRecord{
		ID:          id,
		Name:        name,
		Description: strings.TrimSpace(def.Description),
		Status:      WorkflowDefinitionStatusDraft,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if status := strings.TrimSpace(def.Status); status != "" {
		record.Status = strings.ToLower(status)
	}

	s.definitionsByID[id] = record
	s.definitionByName[strings.ToLower(name)] = id
	return cloneDefinition(record), nil
}

// UpdateWorkflowDefinition updates mutable workflow definition metadata.
func (s *MemoryWorkflowLifecycleStore) UpdateWorkflowDefinition(_ context.Context, def *WorkflowDefinitionRecord) (*WorkflowDefinitionRecord, error) {
	if def == nil {
		return nil, fmt.Errorf("workflow definition is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.definitionsByID[def.ID]
	if !ok {
		return nil, fmt.Errorf("workflow definition not found: %s", def.ID)
	}

	if name := strings.TrimSpace(def.Name); name != "" && !strings.EqualFold(name, current.Name) {
		if existingID, exists := s.definitionByName[strings.ToLower(name)]; exists && existingID != current.ID {
			return nil, fmt.Errorf("workflow definition name already exists: %s", name)
		}
		delete(s.definitionByName, strings.ToLower(current.Name))
		current.Name = name
		s.definitionByName[strings.ToLower(name)] = current.ID
	}

	current.Description = strings.TrimSpace(def.Description)

	if status := strings.TrimSpace(def.Status); status != "" {
		current.Status = strings.ToLower(status)
	}

	current.UpdatedAt = time.Now().UTC()
	return cloneDefinition(current), nil
}

// ArchiveWorkflowDefinition archives a definition while keeping its history.
func (s *MemoryWorkflowLifecycleStore) ArchiveWorkflowDefinition(_ context.Context, workflowID string) (*WorkflowDefinitionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.definitionsByID[workflowID]
	if !ok {
		return nil, fmt.Errorf("workflow definition not found: %s", workflowID)
	}
	current.Status = WorkflowDefinitionStatusArchived
	current.UpdatedAt = time.Now().UTC()
	return cloneDefinition(current), nil
}

// GetWorkflowDefinitionByID returns a definition by ID.
func (s *MemoryWorkflowLifecycleStore) GetWorkflowDefinitionByID(_ context.Context, workflowID string) (*WorkflowDefinitionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return cloneDefinition(s.definitionsByID[workflowID]), nil
}

// GetWorkflowDefinitionByName returns a definition by exact name (case-insensitive).
func (s *MemoryWorkflowLifecycleStore) GetWorkflowDefinitionByName(_ context.Context, name string) (*WorkflowDefinitionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	id := s.definitionByName[strings.ToLower(strings.TrimSpace(name))]
	return cloneDefinition(s.definitionsByID[id]), nil
}

// ListWorkflowDefinitions returns workflow definitions with filtering and paging.
func (s *MemoryWorkflowLifecycleStore) ListWorkflowDefinitions(_ context.Context, filter WorkflowDefinitionListFilter, paging Paging) ([]*WorkflowDefinitionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	filterName := ""
	if filter.Name != nil {
		filterName = strings.ToLower(strings.TrimSpace(*filter.Name))
	}

	items := make([]*WorkflowDefinitionRecord, 0, len(s.definitionsByID))
	for _, def := range s.definitionsByID {
		if filterName != "" && !strings.Contains(strings.ToLower(def.Name), filterName) {
			continue
		}
		if !strEqFoldPtr(filter.Status, def.Status) {
			continue
		}
		items = append(items, cloneDefinition(def))
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].UpdatedAt.Equal(items[j].UpdatedAt) {
			return items[i].Name < items[j].Name
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})

	return pagedSlice(items, paging), nil
}

// SaveWorkflowVersion persists an immutable workflow version.
func (s *MemoryWorkflowLifecycleStore) SaveWorkflowVersion(_ context.Context, version *WorkflowVersionRecord) (*WorkflowVersionRecord, error) {
	if version == nil {
		return nil, fmt.Errorf("workflow version is required")
	}
	if strings.TrimSpace(version.WorkflowID) == "" {
		return nil, fmt.Errorf("workflow ID is required")
	}
	if strings.TrimSpace(version.Yaml) == "" {
		return nil, fmt.Errorf("workflow yaml is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.definitionsByID[version.WorkflowID]; !ok {
		return nil, fmt.Errorf("workflow definition not found: %s", version.WorkflowID)
	}

	s.nextVersionID++
	id := fmt.Sprintf("wfv_%d", s.nextVersionID)
	versionIDs := s.versionIDsByDefID[version.WorkflowID]
	versionNumber := 1
	if len(versionIDs) > 0 {
		latest := s.versionsByID[versionIDs[len(versionIDs)-1]]
		versionNumber = latest.VersionNumber + 1
	}

	now := time.Now().UTC()
	record := &WorkflowVersionRecord{
		ID:            id,
		WorkflowID:    version.WorkflowID,
		VersionNumber: versionNumber,
		Yaml:          version.Yaml,
		Validation:    cloneValidation(version.Validation),
		CreatedBy:     strings.TrimSpace(version.CreatedBy),
		CreatedAt:     now,
		Notes:         strings.TrimSpace(version.Notes),
	}

	s.versionsByID[id] = record
	s.versionIDsByDefID[version.WorkflowID] = append(versionIDs, id)
	return cloneVersion(record), nil
}

// GetWorkflowVersion returns a workflow version by ID.
func (s *MemoryWorkflowLifecycleStore) GetWorkflowVersion(_ context.Context, versionID string) (*WorkflowVersionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneVersion(s.versionsByID[versionID]), nil
}

// GetWorkflowVersionForWorkflow returns a version only when both immutable IDs match.
func (s *MemoryWorkflowLifecycleStore) GetWorkflowVersionForWorkflow(
	_ context.Context,
	workflowID string,
	versionID string,
) (*WorkflowVersionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	version := s.versionsByID[versionID]
	if version == nil || version.WorkflowID != workflowID {
		return nil, nil
	}
	return cloneVersion(version), nil
}

// ListWorkflowVersions returns workflow versions for a definition.
func (s *MemoryWorkflowLifecycleStore) ListWorkflowVersions(_ context.Context, workflowID string, paging Paging) ([]*WorkflowVersionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versionIDs := s.versionIDsByDefID[workflowID]
	items := make([]*WorkflowVersionRecord, 0, len(versionIDs))
	for i := len(versionIDs) - 1; i >= 0; i-- {
		if v := s.versionsByID[versionIDs[i]]; v != nil {
			items = append(items, cloneVersion(v))
		}
	}
	return pagedSlice(items, paging), nil
}

// GetLatestWorkflowVersion returns the most recent version for a definition.
func (s *MemoryWorkflowLifecycleStore) GetLatestWorkflowVersion(_ context.Context, workflowID string) (*WorkflowVersionRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	versionIDs := s.versionIDsByDefID[workflowID]
	if len(versionIDs) == 0 {
		return nil, nil
	}
	return cloneVersion(s.versionsByID[versionIDs[len(versionIDs)-1]]), nil
}

// PublishWorkflowVersion sets an environment pointer to an immutable version.
func (s *MemoryWorkflowLifecycleStore) PublishWorkflowVersion(_ context.Context, release *WorkflowReleaseRecord) (*WorkflowReleaseRecord, error) {
	if release == nil {
		return nil, fmt.Errorf("workflow release is required")
	}
	if strings.TrimSpace(release.WorkflowID) == "" {
		return nil, fmt.Errorf("workflow ID is required")
	}
	if strings.TrimSpace(release.VersionID) == "" {
		return nil, fmt.Errorf("version ID is required")
	}
	environment := strings.TrimSpace(release.Environment)
	if environment == "" {
		return nil, fmt.Errorf("environment is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.definitionsByID[release.WorkflowID]; !ok {
		return nil, fmt.Errorf("workflow definition not found: %s", release.WorkflowID)
	}
	version, ok := s.versionsByID[release.VersionID]
	if !ok {
		return nil, fmt.Errorf("workflow version not found: %s", release.VersionID)
	}
	if version.WorkflowID != release.WorkflowID {
		return nil, fmt.Errorf("workflow version %s does not belong to workflow %s", release.VersionID, release.WorkflowID)
	}

	s.nextReleaseID++
	id := fmt.Sprintf("wfr_%d", s.nextReleaseID)
	now := time.Now().UTC()
	record := &WorkflowReleaseRecord{
		ID:                    id,
		WorkflowID:            release.WorkflowID,
		Environment:           environment,
		VersionID:             release.VersionID,
		PublishedBy:           strings.TrimSpace(release.PublishedBy),
		PublishedAt:           now,
		RollbackFromReleaseID: cloneStringPtr(release.RollbackFromReleaseID),
	}
	if release.PublishedAt.Unix() > 0 {
		record.PublishedAt = release.PublishedAt.UTC()
	}

	s.releasesByID[id] = record
	if _, ok := s.publishedByDefEnv[release.WorkflowID]; !ok {
		s.publishedByDefEnv[release.WorkflowID] = make(map[string]string)
	}
	s.publishedByDefEnv[release.WorkflowID][envKey(environment)] = id

	return cloneRelease(record), nil
}

// GetWorkflowRelease gets a release by ID.
func (s *MemoryWorkflowLifecycleStore) GetWorkflowRelease(_ context.Context, releaseID string) (*WorkflowReleaseRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneRelease(s.releasesByID[releaseID]), nil
}

// GetPublishedWorkflowRelease gets the latest published release for workflow/environment.
func (s *MemoryWorkflowLifecycleStore) GetPublishedWorkflowRelease(_ context.Context, workflowID, environment string) (*WorkflowReleaseRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	releaseID := s.publishedByDefEnv[workflowID][envKey(environment)]
	return cloneRelease(s.releasesByID[releaseID]), nil
}

// ListWorkflowReleases lists all releases for a workflow.
func (s *MemoryWorkflowLifecycleStore) ListWorkflowReleases(_ context.Context, workflowID string) ([]*WorkflowReleaseRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]*WorkflowReleaseRecord, 0)
	for _, release := range s.releasesByID {
		if release.WorkflowID != workflowID {
			continue
		}
		items = append(items, cloneRelease(release))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].PublishedAt.After(items[j].PublishedAt)
	})
	return items, nil
}

// CreateWorkflowRun stores an execution record.
func (s *MemoryWorkflowLifecycleStore) CreateWorkflowRun(_ context.Context, run *WorkflowRunRecord) (*WorkflowRunRecord, error) {
	if run == nil {
		return nil, fmt.Errorf("workflow run is required")
	}
	if strings.TrimSpace(run.WorkflowName) == "" {
		return nil, fmt.Errorf("workflow name is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextRunID++
	id := fmt.Sprintf("wfrun_%d", s.nextRunID)
	now := time.Now().UTC()
	record := &WorkflowRunRecord{
		ID:              id,
		WorkflowID:      strings.TrimSpace(run.WorkflowID),
		WorkflowName:    strings.TrimSpace(run.WorkflowName),
		Environment:     strings.TrimSpace(run.Environment),
		VersionID:       cloneStringPtr(run.VersionID),
		EventID:         cloneStringPtr(run.EventID),
		RoutesMatched:   run.RoutesMatched,
		ActionsExecuted: run.ActionsExecuted,
		Errors:          append([]string(nil), run.Errors...),
		DurationMs:      run.DurationMs,
		StartedAt:       now,
		Status:          strings.TrimSpace(run.Status),
	}
	if record.Environment == "" {
		record.Environment = "production"
	}
	if run.StartedAt.Unix() > 0 {
		record.StartedAt = run.StartedAt.UTC()
	}
	if record.Status == "" {
		record.Status = WorkflowRunStatusSuccess
		if len(record.Errors) > 0 {
			record.Status = WorkflowRunStatusFailed
		}
	}

	s.runsByID[id] = record
	s.runIDs = append(s.runIDs, id)
	return cloneRun(record), nil
}

// GetWorkflowRun returns a workflow run by ID.
func (s *MemoryWorkflowLifecycleStore) GetWorkflowRun(_ context.Context, runID string) (*WorkflowRunRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneRun(s.runsByID[runID]), nil
}

// ListWorkflowRuns lists workflow runs with filter + paging.
func (s *MemoryWorkflowLifecycleStore) ListWorkflowRuns(_ context.Context, filter WorkflowRunListFilter, paging Paging) ([]*WorkflowRunRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]*WorkflowRunRecord, 0, len(s.runIDs))
	for i := len(s.runIDs) - 1; i >= 0; i-- {
		run := s.runsByID[s.runIDs[i]]
		if run == nil {
			continue
		}
		if !strEqFoldPtr(filter.WorkflowName, run.WorkflowName) {
			continue
		}
		if !strEqFoldPtr(filter.Environment, run.Environment) {
			continue
		}
		if !strEqFoldPtr(filter.Status, run.Status) {
			continue
		}
		if filter.FromStartedAt != nil && run.StartedAt.Before(*filter.FromStartedAt) {
			continue
		}
		if filter.ToStartedAt != nil && run.StartedAt.After(*filter.ToStartedAt) {
			continue
		}
		items = append(items, cloneRun(run))
	}

	return pagedSlice(items, paging), nil
}

// CreateWorkflowApprovalRequest creates a new approval request.
func (s *MemoryWorkflowLifecycleStore) CreateWorkflowApprovalRequest(_ context.Context, req *WorkflowApprovalRequestRecord) (*WorkflowApprovalRequestRecord, error) {
	if req == nil {
		return nil, fmt.Errorf("workflow approval request is required")
	}
	if strings.TrimSpace(req.WorkflowID) == "" {
		return nil, fmt.Errorf("workflow ID is required")
	}
	if strings.TrimSpace(req.TargetVersionID) == "" {
		return nil, fmt.Errorf("target version ID is required")
	}
	if strings.TrimSpace(req.Environment) == "" {
		return nil, fmt.Errorf("environment is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextApprovalID++
	id := fmt.Sprintf("wfapr_%d", s.nextApprovalID)
	record := &WorkflowApprovalRequestRecord{
		ID:              id,
		WorkflowID:      strings.TrimSpace(req.WorkflowID),
		TargetVersionID: strings.TrimSpace(req.TargetVersionID),
		Environment:     strings.TrimSpace(req.Environment),
		Status:          WorkflowApprovalStatusPending,
		RequestedBy:     strings.TrimSpace(req.RequestedBy),
		Comment:         cloneStringPtr(req.Comment),
	}
	if status := strings.TrimSpace(req.Status); status != "" {
		record.Status = strings.ToLower(status)
	}
	if req.ReviewedBy != nil {
		record.ReviewedBy = cloneStringPtr(req.ReviewedBy)
	}
	if req.ReviewedAt != nil {
		record.ReviewedAt = cloneTimePtr(req.ReviewedAt)
	}

	s.approvalsByID[id] = record
	s.approvalIDs = append(s.approvalIDs, id)
	return cloneApproval(record), nil
}

// UpdateWorkflowApprovalRequest updates an approval request status.
func (s *MemoryWorkflowLifecycleStore) UpdateWorkflowApprovalRequest(_ context.Context, req *WorkflowApprovalRequestRecord) (*WorkflowApprovalRequestRecord, error) {
	if req == nil {
		return nil, fmt.Errorf("workflow approval request is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.approvalsByID[req.ID]
	if !ok {
		return nil, fmt.Errorf("workflow approval request not found: %s", req.ID)
	}

	if status := strings.TrimSpace(req.Status); status != "" {
		current.Status = strings.ToLower(status)
	}
	if req.ReviewedBy != nil {
		current.ReviewedBy = cloneStringPtr(req.ReviewedBy)
	}
	if req.ReviewedAt != nil {
		current.ReviewedAt = cloneTimePtr(req.ReviewedAt)
	}
	if req.Comment != nil {
		current.Comment = cloneStringPtr(req.Comment)
	}

	return cloneApproval(current), nil
}

// GetWorkflowApprovalRequest gets a request by ID.
func (s *MemoryWorkflowLifecycleStore) GetWorkflowApprovalRequest(_ context.Context, approvalID string) (*WorkflowApprovalRequestRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneApproval(s.approvalsByID[approvalID]), nil
}

// ListWorkflowApprovalRequests lists approval requests with filter + paging.
func (s *MemoryWorkflowLifecycleStore) ListWorkflowApprovalRequests(_ context.Context, filter WorkflowApprovalRequestListFilter, paging Paging) ([]*WorkflowApprovalRequestRecord, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]*WorkflowApprovalRequestRecord, 0, len(s.approvalIDs))
	for i := len(s.approvalIDs) - 1; i >= 0; i-- {
		req := s.approvalsByID[s.approvalIDs[i]]
		if req == nil {
			continue
		}
		if filter.WorkflowID != nil && *filter.WorkflowID != "" && req.WorkflowID != *filter.WorkflowID {
			continue
		}
		if !strEqFoldPtr(filter.Environment, req.Environment) {
			continue
		}
		if !strEqFoldPtr(filter.Status, req.Status) {
			continue
		}
		items = append(items, cloneApproval(req))
	}
	return pagedSlice(items, paging), nil
}

// CreateWorkflowAuditLog stores a lifecycle audit event.
func (s *MemoryWorkflowLifecycleStore) CreateWorkflowAuditLog(_ context.Context, entry *WorkflowAuditLogRecord) (*WorkflowAuditLogRecord, error) {
	if entry == nil {
		return nil, fmt.Errorf("workflow audit entry is required")
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.nextAuditID++
	id := fmt.Sprintf("wfaudit_%d", s.nextAuditID)
	now := time.Now().UTC()
	record := &WorkflowAuditLogRecord{
		ID:         id,
		WorkflowID: strings.TrimSpace(entry.WorkflowID),
		EventType:  strings.TrimSpace(entry.EventType),
		Actor:      strings.TrimSpace(entry.Actor),
		OccurredAt: now,
		Metadata:   cloneMap(entry.Metadata),
	}
	if entry.OccurredAt.Unix() > 0 {
		record.OccurredAt = entry.OccurredAt.UTC()
	}

	s.auditByID[id] = record
	s.auditIDs = append(s.auditIDs, id)
	return cloneAudit(record), nil
}
