package session

import (
	"context"
	"crypto/rand"
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
	ErrNotFound = errors.New("integration session: not found")
	ErrInvalid  = errors.New("integration session: invalid")
)

type MemoryStore struct {
	mu       sync.RWMutex
	now      func() time.Time
	sessions map[string]*Session
	samples  map[string]*Sample
	drafts   map[string]*ArtifactDraft
	runs     map[string]*Run
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		now:      func() time.Time { return time.Now().UTC() },
		sessions: make(map[string]*Session),
		samples:  make(map[string]*Sample),
		drafts:   make(map[string]*ArtifactDraft),
		runs:     make(map[string]*Run),
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
		policy = PHIPolicyRetain
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
	if req.ID != "" {
		draft, ok := s.drafts[req.ID]
		if !ok || draft.SessionID != sessionID {
			return nil, ErrNotFound
		}
		draft.Kind = req.Kind
		draft.Name = strings.TrimSpace(req.Name)
		draft.Content = cloneRaw(req.Content)
		draft.Version++
		draft.UpdatedAt = now
		return cloneDraft(draft), nil
	}

	draft := &ArtifactDraft{
		ID:        newID("draft"),
		SessionID: sessionID,
		Kind:      req.Kind,
		Name:      strings.TrimSpace(req.Name),
		Content:   cloneRaw(req.Content),
		Version:   1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.drafts[draft.ID] = draft
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

func (s *MemoryStore) ListArtifactDrafts(_ context.Context, sessionID string) ([]ArtifactDraft, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if _, ok := s.sessions[sessionID]; !ok {
		return nil, ErrNotFound
	}
	out := make([]ArtifactDraft, 0)
	for _, draft := range s.drafts {
		if draft.SessionID == sessionID {
			out = append(out, *cloneDraft(draft))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
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
	run.CreatedAt = existing.CreatedAt
	run.UpdatedAt = s.now()
	copied := cloneRun(&run)
	s.runs[run.ID] = copied
	return cloneRun(copied), nil
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
	return &ExportBundle{
		Session:    *session,
		Samples:    samples,
		Drafts:     drafts,
		Runs:       runs,
		ExportedAt: s.now(),
	}, nil
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
