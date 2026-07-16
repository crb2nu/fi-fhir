package session

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

var (
	ErrNotFound  = errors.New("integration session: not found")
	ErrInvalid   = errors.New("integration session: invalid")
	ErrImmutable = errors.New("integration session: immutable record")
)

// Store is the restart-safe Integration Session persistence boundary.
type Store interface {
	CreateSession(context.Context, CreateSessionRequest) (*Session, error)
	UpdateSession(context.Context, string, UpdateSessionRequest) (*Session, error)
	ArchiveSession(context.Context, string) (*Session, error)
	GetSession(context.Context, string) (*Session, error)
	ListSessions(context.Context, ListSessionsOptions) ([]Session, error)
	AddSample(context.Context, string, AddSampleRequest) (*Sample, error)
	GetSample(context.Context, string, string) (*Sample, error)
	ListSamples(context.Context, string) ([]Sample, error)
	SaveArtifactDraft(context.Context, string, SaveArtifactDraftRequest) (*ArtifactDraft, error)
	GetArtifactDraft(context.Context, string, string) (*ArtifactDraft, error)
	GetArtifactRevision(context.Context, string, string) (*ArtifactDraft, error)
	ListArtifactDrafts(context.Context, string) ([]ArtifactDraft, error)
	CreateRun(context.Context, string, string, string) (*Run, error)
	UpdateRun(context.Context, Run) (*Run, error)
	GetRun(context.Context, string, string) (*Run, error)
	ListRuns(context.Context, string) ([]Run, error)
	AcceptDecision(context.Context, AcceptDecisionRequest) (*Decision, error)
	ListDecisions(context.Context, string) ([]Decision, error)
	ExportBundle(context.Context, string) (*ExportBundle, error)
	GetExport(context.Context, string, string) (*ExportBundle, error)
	ListExports(context.Context, string) ([]ExportBundle, error)
}

type MemoryStore struct {
	mu        sync.RWMutex
	now       func() time.Time
	sessions  map[string]*Session
	samples   map[string]*Sample
	drafts    map[string]*ArtifactDraft
	runs      map[string]*Run
	decisions map[string]*Decision
	exports   map[string]*ExportBundle
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		now:       func() time.Time { return time.Now().UTC() },
		sessions:  make(map[string]*Session),
		samples:   make(map[string]*Sample),
		drafts:    make(map[string]*ArtifactDraft),
		runs:      make(map[string]*Run),
		decisions: make(map[string]*Decision),
		exports:   make(map[string]*ExportBundle),
	}
}

func (s *MemoryStore) CreateSession(_ context.Context, req CreateSessionRequest) (*Session, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("%w: session name is required", ErrInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	session := &Session{
		ID:          newID("sess"),
		Name:        strings.TrimSpace(req.Name),
		Description: req.Description,
		Status:      SessionStatusActive,
		Tags:        cloneStrings(req.Tags),
		Metadata:    cloneMap(req.Metadata),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.sessions[session.ID] = session
	return cloneSession(session), nil
}

func (s *MemoryStore) UpdateSession(_ context.Context, sessionID string, req UpdateSessionRequest) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	if req.Name != nil {
		if strings.TrimSpace(*req.Name) == "" {
			return nil, fmt.Errorf("%w: session name is required", ErrInvalid)
		}
		session.Name = strings.TrimSpace(*req.Name)
	}
	if req.Description != nil {
		session.Description = *req.Description
	}
	if req.Tags != nil {
		session.Tags = cloneStrings(req.Tags)
	}
	if req.Metadata != nil {
		session.Metadata = cloneMap(req.Metadata)
	}
	session.UpdatedAt = s.now()
	return cloneSession(session), nil
}

func (s *MemoryStore) ArchiveSession(_ context.Context, sessionID string) (*Session, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	now := s.now()
	session.Status = SessionStatusArchived
	session.ArchivedAt = &now
	session.UpdatedAt = now
	return cloneSession(session), nil
}

func (s *MemoryStore) GetSession(_ context.Context, sessionID string) (*Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[sessionID]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneSession(session), nil
}

func (s *MemoryStore) ListSessions(_ context.Context, opts ListSessionsOptions) ([]Session, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Session, 0, len(s.sessions))
	for _, session := range s.sessions {
		if session.Status == SessionStatusArchived && !opts.IncludeArchived {
			continue
		}
		out = append(out, *cloneSession(session))
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *MemoryStore) AddSample(_ context.Context, sessionID string, req AddSampleRequest) (*Sample, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("%w: sample name is required", ErrInvalid)
	}
	if req.Format == "" {
		return nil, fmt.Errorf("%w: sample format is required", ErrInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return nil, ErrNotFound
	}
	policy := req.PHIPolicy
	if policy == "" {
		policy = PHIPolicyRedact
	}
	if policy != PHIPolicyRetain && policy != PHIPolicyRedact {
		return nil, fmt.Errorf("%w: unsupported PHI policy %q", ErrInvalid, policy)
	}
	raw := req.Raw
	redacted := false
	if policy == PHIPolicyRedact {
		raw = redactSample(req.Format, raw)
		redacted = raw != req.Raw
	}
	now := s.now()
	sample := &Sample{
		ID:          newID("sample"),
		SessionID:   sessionID,
		Name:        strings.TrimSpace(req.Name),
		Format:      req.Format,
		Source:      req.Source,
		Raw:         raw,
		PHIPolicy:   policy,
		PHIRedacted: redacted,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.samples[sample.ID] = sample
	return cloneSample(sample), nil
}

func (s *MemoryStore) GetSample(_ context.Context, sessionID, sampleID string) (*Sample, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sample, ok := s.samples[sampleID]
	if !ok || sample.SessionID != sessionID {
		return nil, ErrNotFound
	}
	return cloneSample(sample), nil
}

func (s *MemoryStore) ListSamples(_ context.Context, sessionID string) ([]Sample, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return nil, ErrNotFound
	}
	out := make([]Sample, 0)
	for _, sample := range s.samples {
		if sample.SessionID == sessionID {
			out = append(out, *cloneSample(sample))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *MemoryStore) SaveArtifactDraft(_ context.Context, sessionID string, req SaveArtifactDraftRequest) (*ArtifactDraft, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("%w: draft name is required", ErrInvalid)
	}
	if req.Kind == "" {
		return nil, fmt.Errorf("%w: draft kind is required", ErrInvalid)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return nil, ErrNotFound
	}

	now := s.now()
	artifactID := req.ID
	version := 1
	createdAt := now
	if artifactID != "" {
		latest, ok := s.drafts[artifactID]
		if !ok || latest.SessionID != sessionID {
			return nil, ErrNotFound
		}
		if latest.Kind != req.Kind {
			return nil, fmt.Errorf("%w: artifact kind cannot change", ErrImmutable)
		}
		version = latest.Version + 1
		createdAt = latest.CreatedAt
	} else {
		artifactID = newID("artifact")
	}
	digest := sha256.Sum256(req.Content)
	draft := &ArtifactDraft{
		ID:         artifactID,
		RevisionID: newID("revision"),
		SessionID:  sessionID,
		Kind:       req.Kind,
		Name:       strings.TrimSpace(req.Name),
		Content:    cloneRaw(req.Content),
		Version:    version,
		Digest:     "sha256:" + hex.EncodeToString(digest[:]),
		CreatedAt:  createdAt,
		UpdatedAt:  now,
	}
	s.drafts[draft.ID] = draft
	s.drafts[draft.RevisionID] = draft
	return cloneDraft(draft), nil
}

func (s *MemoryStore) GetArtifactDraft(_ context.Context, sessionID, draftID string) (*ArtifactDraft, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	draft, ok := s.drafts[draftID]
	if !ok || draft.SessionID != sessionID {
		return nil, ErrNotFound
	}
	return cloneDraft(draft), nil
}

func (s *MemoryStore) GetArtifactRevision(ctx context.Context, sessionID, revisionID string) (*ArtifactDraft, error) {
	return s.GetArtifactDraft(ctx, sessionID, revisionID)
}

func (s *MemoryStore) ListArtifactDrafts(_ context.Context, sessionID string) ([]ArtifactDraft, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return nil, ErrNotFound
	}
	out := make([]ArtifactDraft, 0)
	for key, draft := range s.drafts {
		if key == draft.RevisionID && draft.SessionID == sessionID {
			out = append(out, *cloneDraft(draft))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].Version < out[j].Version
		}
		return out[i].UpdatedAt.Before(out[j].UpdatedAt)
	})
	return out, nil
}

func (s *MemoryStore) CreateRun(_ context.Context, sessionID, sampleID, source string) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return nil, ErrNotFound
	}
	sample, ok := s.samples[sampleID]
	if !ok || sample.SessionID != sessionID {
		return nil, ErrNotFound
	}
	now := s.now()
	run := &Run{
		ID:        newID("run"),
		SessionID: sessionID,
		SampleID:  sampleID,
		Status:    RunStatusPending,
		Source:    source,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.runs[run.ID] = run
	return cloneRun(run), nil
}

func (s *MemoryStore) UpdateRun(_ context.Context, run Run) (*Run, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	existing, ok := s.runs[run.ID]
	if !ok {
		return nil, ErrNotFound
	}
	if terminalRun(existing.Status) {
		return nil, ErrImmutable
	}
	if run.SessionID != existing.SessionID || run.SampleID != existing.SampleID || run.Source != existing.Source {
		return nil, ErrImmutable
	}
	run.CreatedAt = existing.CreatedAt
	run.UpdatedAt = s.now()
	copied := cloneRun(&run)
	s.runs[run.ID] = copied
	return cloneRun(copied), nil
}

func (s *MemoryStore) AcceptDecision(_ context.Context, req AcceptDecisionRequest) (*Decision, error) {
	if strings.TrimSpace(req.AcceptedBy) == "" {
		return nil, fmt.Errorf("%w: accepted by is required", ErrInvalid)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	run, ok := s.runs[req.RunID]
	if !ok || run.SessionID != req.SessionID || !runHasDiagnostic(run, req.DiagnosticID) {
		return nil, ErrNotFound
	}
	key := req.RunID + "\x00" + req.DiagnosticID
	if existing, ok := s.decisions[key]; ok {
		result := *existing
		return &result, nil
	}
	decision := &Decision{
		ID: newID("decision"), SessionID: req.SessionID, RunID: req.RunID,
		DiagnosticID: req.DiagnosticID, AcceptedBy: strings.TrimSpace(req.AcceptedBy),
		Reason: strings.TrimSpace(req.Reason), AcceptedAt: s.now(),
	}
	s.decisions[key] = decision
	result := *decision
	return &result, nil
}

func (s *MemoryStore) ListDecisions(_ context.Context, sessionID string) ([]Decision, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return nil, ErrNotFound
	}
	out := make([]Decision, 0)
	for _, decision := range s.decisions {
		if decision.SessionID == sessionID {
			out = append(out, *decision)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AcceptedAt.Before(out[j].AcceptedAt) })
	return out, nil
}

func (s *MemoryStore) GetRun(_ context.Context, sessionID, runID string) (*Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	run, ok := s.runs[runID]
	if !ok || run.SessionID != sessionID {
		return nil, ErrNotFound
	}
	return cloneRun(run), nil
}

func (s *MemoryStore) ListRuns(_ context.Context, sessionID string) ([]Run, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return nil, ErrNotFound
	}
	out := make([]Run, 0)
	for _, run := range s.runs {
		if run.SessionID == sessionID {
			out = append(out, *cloneRun(run))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *MemoryStore) ExportBundle(ctx context.Context, sessionID string) (*ExportBundle, error) {
	session, err := s.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	samples, err := s.ListSamples(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	drafts, err := s.ListArtifactDrafts(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	runs, err := s.ListRuns(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	decisions, err := s.ListDecisions(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	for i := range samples {
		if samples[i].PHIPolicy == PHIPolicyRetain {
			samples[i].Raw = ""
		}
	}
	exportID := newID("export")
	bundle := &ExportBundle{
		ID: exportID, Session: *session, Samples: samples, Drafts: drafts,
		Runs: runs, Decisions: decisions, ExportedAt: s.now(),
	}
	s.mu.Lock()
	s.exports[exportID] = cloneBundle(bundle)
	s.mu.Unlock()
	return bundle, nil
}

func (s *MemoryStore) GetExport(_ context.Context, sessionID, exportID string) (*ExportBundle, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	bundle, ok := s.exports[exportID]
	if !ok || bundle.Session.ID != sessionID {
		return nil, ErrNotFound
	}
	return cloneBundle(bundle), nil
}

func (s *MemoryStore) ListExports(_ context.Context, sessionID string) ([]ExportBundle, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.sessions[sessionID]; !ok {
		return nil, ErrNotFound
	}
	out := make([]ExportBundle, 0)
	for _, bundle := range s.exports {
		if bundle.Session.ID == sessionID {
			out = append(out, *cloneBundle(bundle))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExportedAt.Before(out[j].ExportedAt) })
	return out, nil
}

func terminalRun(status RunStatus) bool {
	return status == RunStatusSucceeded || status == RunStatusFailed
}

func runHasDiagnostic(run *Run, diagnosticID string) bool {
	for _, diagnostic := range run.Diagnostics {
		if diagnostic.ID == diagnosticID {
			return true
		}
	}
	return false
}

func redactSample(format events.SourceFormat, raw string) string {
	if format != events.FormatHL7v2 {
		return "[redacted]"
	}
	return redactHL7v2(raw)
}

func redactHL7v2(raw string) string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, "PID|") {
			continue
		}
		fields := strings.Split(line, "|")
		for _, idx := range []int{3, 5, 7, 11, 13, 19} {
			if idx < len(fields) && fields[idx] != "" {
				fields[idx] = "REDACTED"
			}
		}
		lines[i] = strings.Join(fields, "|")
	}
	return strings.Join(lines, "\n")
}

func newID(prefix string) string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UTC().UnixNano())
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

func cloneSession(in *Session) *Session {
	if in == nil {
		return nil
	}
	out := *in
	out.Tags = cloneStrings(in.Tags)
	out.Metadata = cloneMap(in.Metadata)
	if in.ArchivedAt != nil {
		t := *in.ArchivedAt
		out.ArchivedAt = &t
	}
	return &out
}

func cloneSample(in *Sample) *Sample {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneDraft(in *ArtifactDraft) *ArtifactDraft {
	if in == nil {
		return nil
	}
	out := *in
	out.Content = cloneRaw(in.Content)
	return &out
}

func cloneRun(in *Run) *Run {
	if in == nil {
		return nil
	}
	out := *in
	out.Stages = append([]RunStage(nil), in.Stages...)
	out.Diagnostics = append([]Diagnostic(nil), in.Diagnostics...)
	out.Lineage = append([]LineageLink(nil), in.Lineage...)
	out.Events = make([]ParsedEvent, len(in.Events))
	for i := range in.Events {
		out.Events[i] = in.Events[i]
		out.Events[i].Payload = cloneRaw(in.Events[i].Payload)
	}
	if in.StartedAt != nil {
		t := *in.StartedAt
		out.StartedAt = &t
	}
	if in.FinishedAt != nil {
		t := *in.FinishedAt
		out.FinishedAt = &t
	}
	return &out
}

func cloneRaw(in []byte) []byte {
	if in == nil {
		return nil
	}
	return append([]byte(nil), in...)
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	return append([]string(nil), in...)
}

func cloneMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneBundle(in *ExportBundle) *ExportBundle {
	if in == nil {
		return nil
	}
	out := *in
	out.Session = *cloneSession(&in.Session)
	out.Samples = append([]Sample(nil), in.Samples...)
	out.Drafts = make([]ArtifactDraft, len(in.Drafts))
	for i := range in.Drafts {
		out.Drafts[i] = *cloneDraft(&in.Drafts[i])
	}
	out.Runs = make([]Run, len(in.Runs))
	for i := range in.Runs {
		out.Runs[i] = *cloneRun(&in.Runs[i])
	}
	out.Decisions = append([]Decision(nil), in.Decisions...)
	return &out
}
