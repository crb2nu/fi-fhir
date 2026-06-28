package resolvers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/hl7v2"
)

type integrationSessionSubscriber struct {
	sessionID string
	runID     *string
	ch        chan *model.IntegrationSessionEvent
}

type integrationSessionService struct {
	mu sync.RWMutex

	sessions    map[string]*model.IntegrationSession
	samples     map[string][]model.SessionSample
	artifacts   map[string]map[string]model.SessionArtifact
	runs        map[string][]model.SessionRun
	runsByID    map[string]model.SessionRun
	diagnostics map[string][]model.SessionDiagnostic

	subscribers map[string]*integrationSessionSubscriber
}

func newIntegrationSessionService() *integrationSessionService {
	return &integrationSessionService{
		sessions:    make(map[string]*model.IntegrationSession),
		samples:     make(map[string][]model.SessionSample),
		artifacts:   make(map[string]map[string]model.SessionArtifact),
		runs:        make(map[string][]model.SessionRun),
		runsByID:    make(map[string]model.SessionRun),
		diagnostics: make(map[string][]model.SessionDiagnostic),
		subscribers: make(map[string]*integrationSessionSubscriber),
	}
}

func (s *integrationSessionService) createSession(input model.CreateIntegrationSessionInput) (*model.IntegrationSession, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("session name is required")
	}

	now := time.Now()
	session := &model.IntegrationSession{
		ID:          uuid.NewString(),
		Name:        input.Name,
		Description: input.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()

	out := s.cloneSession(session.ID, true)
	s.publish("session.created", session.ID, nil, "integration session created")
	return out, nil
}

func (s *integrationSessionService) listSessions(includeArchived bool) []model.IntegrationSession {
	s.mu.RLock()
	ids := make([]string, 0, len(s.sessions))
	for id, session := range s.sessions {
		if includeArchived || !session.Archived {
			ids = append(ids, id)
		}
	}
	s.mu.RUnlock()

	out := make([]model.IntegrationSession, 0, len(ids))
	for _, id := range ids {
		if session := s.cloneSession(id, true); session != nil {
			out = append(out, *session)
		}
	}
	return out
}

func (s *integrationSessionService) getSession(id string) *model.IntegrationSession {
	return s.cloneSession(id, true)
}

func (s *integrationSessionService) archiveSession(id string) (*model.IntegrationSession, error) {
	s.mu.Lock()
	session, ok := s.sessions[id]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("integration session %q not found", id)
	}
	session.Archived = true
	session.UpdatedAt = time.Now()
	s.mu.Unlock()

	out := s.cloneSession(id, true)
	s.publish("session.archived", id, nil, "integration session archived")
	return out, nil
}

func (s *integrationSessionService) addSample(input model.AddSessionSampleInput) (*model.SessionSample, error) {
	if input.Name == "" {
		return nil, fmt.Errorf("sample name is required")
	}
	if input.Data == "" {
		return nil, fmt.Errorf("sample data is required")
	}

	now := time.Now()
	checksum := sha256.Sum256([]byte(input.Data))
	retainRaw := input.RetainRawPayload != nil && *input.RetainRawPayload
	var raw *string
	if retainRaw {
		raw = &input.Data
	}

	sample := model.SessionSample{
		ID:              uuid.NewString(),
		SessionID:       input.SessionID,
		Name:            input.Name,
		Format:          input.Format,
		Source:          input.Source,
		RawPayload:      raw,
		PayloadChecksum: hex.EncodeToString(checksum[:]),
		PayloadRef:      input.PayloadRef,
		CreatedAt:       now,
	}

	s.mu.Lock()
	session, ok := s.sessions[input.SessionID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("integration session %q not found", input.SessionID)
	}
	session.UpdatedAt = now
	s.samples[input.SessionID] = append(s.samples[input.SessionID], sample)
	s.mu.Unlock()

	s.publish("sample.added", input.SessionID, nil, "session sample added")
	return &sample, nil
}

func (s *integrationSessionService) listSamples(sessionID string) []model.SessionSample {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]model.SessionSample(nil), s.samples[sessionID]...)
}

func (s *integrationSessionService) saveArtifact(sessionID, kind string, input model.UpdateSessionArtifactInput) (*model.SessionArtifact, error) {
	now := time.Now()
	name := kind
	if input.Name != nil && *input.Name != "" {
		name = *input.Name
	}

	s.mu.Lock()
	session, ok := s.sessions[sessionID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("integration session %q not found", sessionID)
	}

	byKind := s.artifacts[sessionID]
	if byKind == nil {
		byKind = make(map[string]model.SessionArtifact)
		s.artifacts[sessionID] = byKind
	}

	artifact := byKind[kind]
	if artifact.ID == "" {
		artifact.ID = uuid.NewString()
		artifact.SessionID = sessionID
		artifact.Kind = kind
		artifact.CreatedAt = now
	}
	artifact.Name = name
	artifact.Content = input.Content
	artifact.UpdatedAt = now
	byKind[kind] = artifact
	session.UpdatedAt = now
	s.mu.Unlock()

	s.publish("artifact.updated", sessionID, nil, fmt.Sprintf("%s draft updated", kind))
	return &artifact, nil
}

func (s *integrationSessionService) listArtifacts(sessionID string) []model.SessionArtifact {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]model.SessionArtifact, 0, len(s.artifacts[sessionID]))
	for _, artifact := range s.artifacts[sessionID] {
		out = append(out, artifact)
	}
	return out
}

func (s *integrationSessionService) runPreview(input model.RunSessionPreviewInput) (*model.SessionRun, error) {
	payload, format, source, sampleID, err := s.resolvePreviewInput(input)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	run := model.SessionRun{
		ID:        uuid.NewString(),
		SessionID: input.SessionID,
		SampleID:  sampleID,
		Status:    "running",
		CreatedAt: now,
		Stages: []model.RunStage{{
			ID:        uuid.NewString(),
			Name:      "parse",
			Status:    "running",
			StartedAt: now,
		}},
		Diagnostics: []model.SessionDiagnostic{},
		Events:      []model.Event{},
		Warnings:    []model.ParseWarning{},
	}

	start := time.Now()
	if format != model.SourceFormatHL7v2 {
		msg := fmt.Sprintf("session preview currently supports HL7v2 only; got %s", format)
		run.Diagnostics = append(run.Diagnostics, newSessionDiagnostic(input.SessionID, run.ID, sampleID, "error", "UNSUPPORTED_FORMAT", msg, nil))
		run.Status = "failed"
	} else {
		parser := hl7v2.NewParser(source, hl7v2.ParserConfig{})
		parseResult, parseErr := parser.ParseWithResult(payload)
		if parseErr != nil {
			run.Diagnostics = append(run.Diagnostics, newSessionDiagnostic(input.SessionID, run.ID, sampleID, "error", "PARSE_ERROR", parseErr.Error(), nil))
			run.Status = "failed"
		} else {
			if parseResult.Event != nil {
				if event := convertToGraphQLEvent(parseResult.Event, source, format, nil); event != nil {
					run.Events = append(run.Events, event)
				}
			}
			for _, warning := range parseResult.Warnings {
				path := warning.Path
				gqlWarning := model.ParseWarning{
					Phase:   warning.Phase,
					Code:    warning.Code,
					Message: warning.Message,
					Path:    strPtr(path),
				}
				run.Warnings = append(run.Warnings, gqlWarning)

				severity := warning.Severity
				if severity == "" {
					severity = "warning"
				}
				run.Diagnostics = append(run.Diagnostics, newSessionDiagnostic(input.SessionID, run.ID, sampleID, severity, warning.Code, warning.Message, strPtr(path)))
			}
			run.Status = "completed"
		}
	}

	completed := time.Now()
	summary := fmt.Sprintf("%d event(s), %d diagnostic(s)", len(run.Events), len(run.Diagnostics))
	run.CompletedAt = &completed
	run.Stages[0].Status = run.Status
	run.Stages[0].CompletedAt = &completed
	run.Stages[0].DurationMs = int(time.Since(start).Milliseconds())
	run.Stages[0].Summary = &summary

	s.mu.Lock()
	session, ok := s.sessions[input.SessionID]
	if !ok {
		s.mu.Unlock()
		return nil, fmt.Errorf("integration session %q not found", input.SessionID)
	}
	session.UpdatedAt = completed
	s.runs[input.SessionID] = append(s.runs[input.SessionID], run)
	s.runsByID[run.ID] = run
	s.diagnostics[input.SessionID] = append(s.diagnostics[input.SessionID], run.Diagnostics...)
	s.mu.Unlock()

	s.publish("run.completed", input.SessionID, &run.ID, "session preview run completed")
	return &run, nil
}

func (s *integrationSessionService) resolvePreviewInput(input model.RunSessionPreviewInput) (string, model.SourceFormat, string, *string, error) {
	source := "integration-session"
	if input.Source != nil && *input.Source != "" {
		source = *input.Source
	}
	format := model.SourceFormatHL7v2
	if input.Format != nil {
		format = *input.Format
	}
	if input.Data != nil && *input.Data != "" {
		return *input.Data, format, source, nil, nil
	}
	if input.SampleID == nil {
		return "", "", "", nil, fmt.Errorf("run preview requires sampleId or data")
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.sessions[input.SessionID]; !ok {
		return "", "", "", nil, fmt.Errorf("integration session %q not found", input.SessionID)
	}
	for _, sample := range s.samples[input.SessionID] {
		if sample.ID == *input.SampleID {
			if sample.Source != nil && *sample.Source != "" {
				source = *sample.Source
			}
			if input.Format == nil {
				format = sample.Format
			}
			if sample.RawPayload == nil {
				return "", "", "", nil, fmt.Errorf("sample %q does not retain raw payload; provide data for preview", sample.ID)
			}
			return *sample.RawPayload, format, source, &sample.ID, nil
		}
	}
	return "", "", "", nil, fmt.Errorf("session sample %q not found", *input.SampleID)
}

func newSessionDiagnostic(sessionID, runID string, sampleID *string, severity, code, message string, path *string) model.SessionDiagnostic {
	if severity == "" {
		severity = "warning"
	}
	lineage := []model.LineageLink{}
	if path != nil && *path != "" {
		lineage = append(lineage, model.LineageLink{SourcePath: *path})
	}
	fix := "Review the source profile or sample payload for this warning."
	return model.SessionDiagnostic{
		ID:            uuid.NewString(),
		SessionID:     sessionID,
		RunID:         &runID,
		SampleID:      sampleID,
		Severity:      severity,
		Code:          code,
		Message:       message,
		Path:          path,
		FixSuggestion: &fix,
		Lineage:       lineage,
	}
}

func (s *integrationSessionService) listRuns(sessionID string) []model.SessionRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneRuns(s.runs[sessionID])
}

func (s *integrationSessionService) getRun(id string) *model.SessionRun {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runsByID[id]
	if !ok {
		return nil
	}
	cloned := cloneRun(run)
	return &cloned
}

func (s *integrationSessionService) listDiagnostics(sessionID string, runID *string) []model.SessionDiagnostic {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var out []model.SessionDiagnostic
	for _, diagnostic := range s.diagnostics[sessionID] {
		if runID == nil || (diagnostic.RunID != nil && *diagnostic.RunID == *runID) {
			out = append(out, diagnostic)
		}
	}
	return out
}

func (s *integrationSessionService) acceptDiagnostic(input model.AcceptDiagnosticFixInput) (*model.SessionDiagnostic, error) {
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	diags := s.diagnostics[input.SessionID]
	for i := range diags {
		if diags[i].ID == input.DiagnosticID {
			diags[i].Accepted = true
			diags[i].AcceptedAt = &now
			s.diagnostics[input.SessionID] = diags
			s.updateRunDiagnosticLocked(diags[i])
			if session := s.sessions[input.SessionID]; session != nil {
				session.UpdatedAt = now
			}
			out := diags[i]
			go s.publish("diagnostic.accepted", input.SessionID, out.RunID, "diagnostic fix accepted")
			return &out, nil
		}
	}
	return nil, fmt.Errorf("session diagnostic %q not found", input.DiagnosticID)
}

func (s *integrationSessionService) updateRunDiagnosticLocked(diagnostic model.SessionDiagnostic) {
	if diagnostic.RunID == nil {
		return
	}
	run, ok := s.runsByID[*diagnostic.RunID]
	if !ok {
		return
	}
	for i := range run.Diagnostics {
		if run.Diagnostics[i].ID == diagnostic.ID {
			run.Diagnostics[i] = diagnostic
		}
	}
	s.runsByID[run.ID] = run
	runs := s.runs[run.SessionID]
	for i := range runs {
		if runs[i].ID == run.ID {
			runs[i] = run
		}
	}
	s.runs[run.SessionID] = runs
}

func (s *integrationSessionService) exportBundle(input model.ExportIntegrationBundleInput) (*model.IntegrationBundle, error) {
	session := s.cloneSession(input.SessionID, true)
	if session == nil {
		return nil, fmt.Errorf("integration session %q not found", input.SessionID)
	}
	if input.IncludeRawPayload == nil || !*input.IncludeRawPayload {
		for i := range session.Samples {
			session.Samples[i].RawPayload = nil
		}
	}

	return &model.IntegrationBundle{
		SessionID:   input.SessionID,
		ExportedAt:  time.Now(),
		Session:     *session,
		Samples:     append([]model.SessionSample(nil), session.Samples...),
		Artifacts:   append([]model.SessionArtifact(nil), session.Artifacts...),
		Runs:        cloneRuns(session.Runs),
		Diagnostics: append([]model.SessionDiagnostic(nil), session.Diagnostics...),
	}, nil
}

func (s *integrationSessionService) subscribe(ctx context.Context, sessionID string, runID *string) (<-chan *model.IntegrationSessionEvent, error) {
	s.mu.RLock()
	_, ok := s.sessions[sessionID]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("integration session %q not found", sessionID)
	}

	id := uuid.NewString()
	ch := make(chan *model.IntegrationSessionEvent, 16)

	s.mu.Lock()
	s.subscribers[id] = &integrationSessionSubscriber{
		sessionID: sessionID,
		runID:     runID,
		ch:        ch,
	}
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		delete(s.subscribers, id)
		close(ch)
		s.mu.Unlock()
	}()

	return ch, nil
}

func (s *integrationSessionService) publish(eventType, sessionID string, runID *string, message string) {
	event := &model.IntegrationSessionEvent{
		ID:        uuid.NewString(),
		Type:      eventType,
		SessionID: sessionID,
		RunID:     runID,
		Message:   message,
		Timestamp: time.Now(),
		Session:   s.cloneSession(sessionID, true),
	}
	if runID != nil {
		event.Run = s.getRun(*runID)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, sub := range s.subscribers {
		if sub.sessionID != sessionID {
			continue
		}
		if sub.runID != nil && (runID == nil || *sub.runID != *runID) {
			continue
		}
		select {
		case sub.ch <- event:
		default:
		}
	}
}

func (s *integrationSessionService) cloneSession(id string, includeChildren bool) *model.IntegrationSession {
	s.mu.RLock()
	defer s.mu.RUnlock()

	session, ok := s.sessions[id]
	if !ok {
		return nil
	}

	out := *session
	if !includeChildren {
		return &out
	}
	out.Samples = append([]model.SessionSample(nil), s.samples[id]...)
	out.Artifacts = make([]model.SessionArtifact, 0, len(s.artifacts[id]))
	for _, artifact := range s.artifacts[id] {
		out.Artifacts = append(out.Artifacts, artifact)
		artifactCopy := artifact
		switch artifact.Kind {
		case "profile":
			out.CurrentProfileDraft = &artifactCopy
		case "workflow":
			out.CurrentWorkflowDraft = &artifactCopy
		}
	}
	out.Runs = cloneRuns(s.runs[id])
	out.Diagnostics = append([]model.SessionDiagnostic(nil), s.diagnostics[id]...)
	return &out
}

func cloneRuns(in []model.SessionRun) []model.SessionRun {
	out := make([]model.SessionRun, len(in))
	for i, run := range in {
		out[i] = cloneRun(run)
	}
	return out
}

func cloneRun(run model.SessionRun) model.SessionRun {
	run.Stages = append([]model.RunStage(nil), run.Stages...)
	run.Diagnostics = append([]model.SessionDiagnostic(nil), run.Diagnostics...)
	run.Events = append([]model.Event(nil), run.Events...)
	run.Warnings = append([]model.ParseWarning(nil), run.Warnings...)
	return run
}
