package resolvers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	enginesession "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

type integrationSessionService struct {
	store     enginesession.Store
	runner    *enginesession.Runner
	simulator *enginesession.WorkflowSimulator
	publisher *enginesession.PublicationService
	hub       *enginesession.Hub
}

func newIntegrationSessionService() *integrationSessionService {
	return newIntegrationSessionServiceWithStore(enginesession.NewMemoryStore())
}

func newIntegrationSessionServiceWithStore(store enginesession.Store) *integrationSessionService {
	if store == nil {
		store = enginesession.NewMemoryStore()
	}
	// A store that can carry the durable fanout log gets a durable hub, so a
	// subscription pinned to one replica sees a run executed on another. The
	// in-memory store has no log and keeps in-process fanout, which is correct
	// for a single-process composition.
	hub := enginesession.NewHub()
	if log, ok := store.(enginesession.StreamLog); ok {
		hub = enginesession.NewDurableHub(log, nil)
	}
	return &integrationSessionService{
		store: store, runner: enginesession.NewRunner(store, hub),
		simulator: enginesession.NewWorkflowSimulator(store), hub: hub,
	}
}

func (s *integrationSessionService) createSession(input model.CreateIntegrationSessionInput) (*model.IntegrationSession, error) {
	req := enginesession.CreateSessionRequest{
		Name: input.Name,
	}
	if input.Description != nil {
		req.Description = *input.Description
	}
	session, err := s.store.CreateSession(context.Background(), req)
	if err != nil {
		return nil, err
	}
	s.publish("session.created", session.ID, "", "integration session created")
	return s.cloneSession(session.ID, true)
}

func (s *integrationSessionService) listSessions(includeArchived bool) ([]model.IntegrationSession, error) {
	sessions, err := s.store.ListSessions(context.Background(), enginesession.ListSessionsOptions{
		IncludeArchived: includeArchived,
	})
	if err != nil {
		return nil, err
	}
	out := make([]model.IntegrationSession, 0, len(sessions))
	for _, session := range sessions {
		gql, err := s.toGraphQLSession(session, true)
		if err != nil {
			return nil, err
		}
		out = append(out, *gql)
	}
	return out, nil
}

func (s *integrationSessionService) getSession(id string) (*model.IntegrationSession, error) {
	return s.cloneSession(id, true)
}

func (s *integrationSessionService) archiveSession(id string) (*model.IntegrationSession, error) {
	session, err := s.store.ArchiveSession(context.Background(), id)
	if err != nil {
		return nil, err
	}
	s.publish("session.archived", id, "", "integration session archived")
	return s.toGraphQLSession(*session, true)
}

func (s *integrationSessionService) addSample(input model.AddSessionSampleInput) (*model.SessionSample, error) {
	if input.Data == "" {
		return nil, fmt.Errorf("sample data is required")
	}
	policy := enginesession.PHIPolicyRedact
	if input.RetainRawPayload != nil && *input.RetainRawPayload {
		policy = enginesession.PHIPolicyRetain
	}
	req := enginesession.AddSampleRequest{
		Name:      input.Name,
		Format:    convertToEventsSourceFormat(input.Format),
		Raw:       input.Data,
		PHIPolicy: policy,
	}
	if input.Source != nil {
		req.Source = *input.Source
	}

	sample, err := s.store.AddSample(context.Background(), input.SessionID, req)
	if err != nil {
		return nil, err
	}
	s.publish("sample.added", input.SessionID, "", "session sample added")
	return s.toGraphQLSample(*sample), nil
}

func (s *integrationSessionService) listSamples(sessionID string) ([]model.SessionSample, error) {
	samples, err := s.store.ListSamples(context.Background(), sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]model.SessionSample, 0, len(samples))
	for _, sample := range samples {
		out = append(out, *s.toGraphQLSample(sample))
	}
	return out, nil
}

func (s *integrationSessionService) saveArtifact(sessionID, kind string, input model.UpdateSessionArtifactInput) (*model.SessionArtifact, error) {
	name := kind
	if input.Name != nil && *input.Name != "" {
		name = *input.Name
	}
	artifactKind := toArtifactKind(kind)
	artifactID := ""
	existing, err := s.store.ListArtifactDrafts(context.Background(), sessionID)
	if err != nil {
		return nil, err
	}
	for _, revision := range existing {
		if revision.Kind == artifactKind {
			artifactID = revision.ID
		}
	}
	draft, err := s.store.SaveArtifactDraft(context.Background(), sessionID, enginesession.SaveArtifactDraftRequest{
		ID:      artifactID,
		Kind:    artifactKind,
		Name:    name,
		Content: json.RawMessage(input.Content),
	})
	if err != nil {
		return nil, err
	}
	s.publish("artifact.updated", sessionID, "", fmt.Sprintf("%s draft updated", kind))
	return toGraphQLArtifact(*draft), nil
}

func (s *integrationSessionService) listArtifacts(sessionID string) ([]model.SessionArtifact, error) {
	drafts, err := s.store.ListArtifactDrafts(context.Background(), sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]model.SessionArtifact, 0, len(drafts))
	for _, draft := range drafts {
		out = append(out, *toGraphQLArtifact(draft))
	}
	return out, nil
}

func (s *integrationSessionService) runPreview(input model.RunSessionPreviewInput) (*model.SessionRun, error) {
	sampleID, cleanup, err := s.ensurePreviewSample(input)
	if err != nil {
		return nil, err
	}
	_ = cleanup

	sample, err := s.store.GetSample(context.Background(), input.SessionID, sampleID)
	if err != nil {
		return nil, err
	}
	if sample.Format != events.FormatHL7v2 {
		run, err := s.createUnsupportedFormatRun(input.SessionID, sampleID, sample.Format, sample.Source)
		if err != nil {
			return nil, err
		}
		return s.toGraphQLRun(*run)
	}

	source := ""
	if input.Source != nil {
		source = *input.Source
	}
	profileRevisionID := ""
	artifacts, err := s.store.ListArtifactDrafts(context.Background(), input.SessionID)
	if err != nil {
		return nil, err
	}
	for _, revision := range artifacts {
		if revision.Kind == enginesession.ArtifactKindMappingProfile {
			profileRevisionID = revision.RevisionID
		}
	}
	run, err := s.runner.RunHL7v2(context.Background(), enginesession.RunRequest{
		SessionID:         input.SessionID,
		SampleID:          sampleID,
		Source:            source,
		ProfileRevisionID: profileRevisionID,
	})
	if err != nil {
		if run != nil {
			return s.toGraphQLRun(*run)
		}
		return nil, err
	}
	return s.toGraphQLRun(*run)
}

func (s *integrationSessionService) ensurePreviewSample(input model.RunSessionPreviewInput) (string, func(), error) {
	if input.SampleID != nil && *input.SampleID != "" {
		return *input.SampleID, func() {}, nil
	}
	if input.Data == nil || *input.Data == "" {
		return "", nil, fmt.Errorf("run preview requires sampleId or data")
	}
	format := model.SourceFormatHL7v2
	if input.Format != nil {
		format = *input.Format
	}
	source := "integration-session"
	if input.Source != nil && *input.Source != "" {
		source = *input.Source
	}
	sample, err := s.store.AddSample(context.Background(), input.SessionID, enginesession.AddSampleRequest{
		Name:      "Ad hoc preview",
		Format:    convertToEventsSourceFormat(format),
		Source:    source,
		Raw:       *input.Data,
		PHIPolicy: enginesession.PHIPolicyRedact,
	})
	if err != nil {
		return "", nil, err
	}
	return sample.ID, func() {}, nil
}

func (s *integrationSessionService) createUnsupportedFormatRun(sessionID, sampleID string, format events.SourceFormat, source string) (*enginesession.Run, error) {
	run, err := s.store.CreateRun(context.Background(), sessionID, sampleID, source)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	msg := fmt.Sprintf("session preview currently supports HL7v2 only; got %s", format)
	run.Status = enginesession.RunStatusFailed
	run.StartedAt = &now
	run.FinishedAt = &now
	run.Error = msg
	run.Stages = []enginesession.RunStage{{
		Name:       "parse_hl7v2",
		Status:     enginesession.StageStatusFailed,
		StartedAt:  now,
		FinishedAt: &now,
		Error:      msg,
	}}
	run.Diagnostics = []enginesession.Diagnostic{{
		ID:        "diag_001",
		Severity:  "error",
		Phase:     "syntactic",
		Code:      "UNSUPPORTED_FORMAT",
		Message:   msg,
		Source:    "integration_session_runner",
		CreatedAt: now,
	}}
	updated, err := s.store.UpdateRun(context.Background(), *run)
	if err != nil {
		return nil, err
	}
	s.publish("run.failed", sessionID, updated.ID, "session preview run failed")
	return updated, nil
}

func (s *integrationSessionService) listRuns(sessionID string) ([]model.SessionRun, error) {
	runs, err := s.store.ListRuns(context.Background(), sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]model.SessionRun, 0, len(runs))
	for _, run := range runs {
		gql, err := s.toGraphQLRun(run)
		if err != nil {
			return nil, err
		}
		out = append(out, *gql)
	}
	return out, nil
}

func (s *integrationSessionService) simulateWorkflow(input model.SimulateSessionWorkflowInput) (*model.SessionWorkflowSimulation, error) {
	var baseline *enginesession.WorkflowSimulation
	if input.BaselineSimulationID != nil && *input.BaselineSimulationID != "" {
		var err error
		baseline, err = s.store.GetWorkflowSimulation(context.Background(), input.SessionID, *input.BaselineSimulationID)
		if err != nil {
			return nil, err
		}
	}
	record, err := s.simulator.Simulate(context.Background(), enginesession.SimulateWorkflowRequest{
		SessionID: input.SessionID, WorkflowRevisionID: input.WorkflowRevisionID,
		SourceRunIDs: append([]string(nil), input.SourceRunIds...),
	})
	if err != nil {
		return nil, err
	}
	result := toGraphQLWorkflowSimulation(*record)
	if baseline != nil {
		delta, err := enginesession.CompareWorkflowSimulations(*baseline, *record)
		if err != nil {
			return nil, err
		}
		result.Delta = toGraphQLWorkflowSimulationDelta(delta)
	}
	s.publish("workflow.simulated", input.SessionID, "", "session workflow simulated")
	return result, nil
}

func (s *integrationSessionService) listWorkflowSimulations(sessionID string) ([]model.SessionWorkflowSimulation, error) {
	records, err := s.store.ListWorkflowSimulations(context.Background(), sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]model.SessionWorkflowSimulation, len(records))
	for index := range records {
		out[index] = *toGraphQLWorkflowSimulation(records[index])
	}
	return out, nil
}

func (s *integrationSessionService) listPublications(sessionID string) ([]model.SessionPublication, error) {
	records, err := s.store.ListPublications(context.Background(), sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]model.SessionPublication, len(records))
	for index := range records {
		out[index] = *toGraphQLPublication(records[index])
	}
	return out, nil
}

func (s *integrationSessionService) publishSession(ctx context.Context, input model.PublishIntegrationSessionInput) (*model.SessionPublication, error) {
	if s.publisher == nil {
		return nil, enginesession.ErrPublicationUnavailable
	}
	security, authenticated := requestsecurity.SecurityContextFromContext(ctx)
	if !authenticated {
		return nil, requestsecurity.ErrMissingCredentials
	}
	record, err := s.publisher.Publish(ctx, enginesession.PublishRequest{
		SessionID: input.SessionID, ProfileRevisionID: input.ProfileRevisionID,
		WorkflowSimulationID: input.WorkflowSimulationID, DefinitionID: input.DefinitionID,
		DefinitionRevisionID: input.DefinitionRevisionID, PublishedBy: security.Principal.ID,
		Reason: input.Reason,
	})
	if err != nil {
		return nil, err
	}
	s.publish("session.published", input.SessionID, "", "integration session publication signed")
	return toGraphQLPublication(*record), nil
}

func (s *integrationSessionService) approvePublication(ctx context.Context, input model.PromoteSessionPublicationInput) (*model.SessionDeploymentSnapshot, error) {
	return s.promotePublication(ctx, input, false)
}

func (s *integrationSessionService) deployPublication(ctx context.Context, input model.PromoteSessionPublicationInput) (*model.SessionDeploymentSnapshot, error) {
	return s.promotePublication(ctx, input, true)
}

func (s *integrationSessionService) promotePublication(ctx context.Context, input model.PromoteSessionPublicationInput, deploy bool) (*model.SessionDeploymentSnapshot, error) {
	if s.publisher == nil {
		return nil, enginesession.ErrPublicationUnavailable
	}
	security, authenticated := requestsecurity.SecurityContextFromContext(ctx)
	if !authenticated {
		return nil, requestsecurity.ErrMissingCredentials
	}
	req := enginesession.PromotePublicationRequest{
		SessionID: input.SessionID, PublicationID: input.PublicationID,
		ExpectedVersion: int64(input.ExpectedVersion), Actor: security.Principal, Reason: input.Reason,
	}
	var (
		snapshot lifecycle.Snapshot
		err      error
	)
	if deploy {
		snapshot, err = s.publisher.Deploy(ctx, req)
	} else {
		snapshot, err = s.publisher.Approve(ctx, req)
	}
	if err != nil {
		return nil, err
	}
	eventType := "session.publication.approved"
	message := "session publication approved"
	if deploy {
		eventType = "session.publication.deployed"
		message = "session publication deployed"
	}
	s.publish(eventType, input.SessionID, "", message)
	return toGraphQLDeploymentSnapshot(snapshot), nil
}

func (s *integrationSessionService) getRun(id string) (*model.SessionRun, error) {
	sessions, err := s.store.ListSessions(context.Background(), enginesession.ListSessionsOptions{IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	for _, session := range sessions {
		run, err := s.store.GetRun(context.Background(), session.ID, id)
		if err == nil {
			return s.toGraphQLRun(*run)
		}
	}
	return nil, nil
}

func (s *integrationSessionService) listDiagnostics(sessionID string, runID *string) ([]model.SessionDiagnostic, error) {
	runs, err := s.store.ListRuns(context.Background(), sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]model.SessionDiagnostic, 0)
	for _, run := range runs {
		if runID != nil && run.ID != *runID {
			continue
		}
		for _, diagnostic := range run.Diagnostics {
			out = append(out, s.toGraphQLDiagnostic(run, diagnostic))
		}
	}
	return out, nil
}

func (s *integrationSessionService) acceptDiagnostic(ctx context.Context, input model.AcceptDiagnosticFixInput) (*model.SessionDiagnostic, error) {
	runs, err := s.store.ListRuns(ctx, input.SessionID)
	if err != nil {
		return nil, err
	}
	for _, run := range runs {
		for _, diagnostic := range run.Diagnostics {
			if diagnostic.ID != input.DiagnosticID {
				continue
			}
			acceptedBy := "integration-engineer"
			if security, authenticated := requestsecurity.SecurityContextFromContext(ctx); authenticated {
				acceptedBy = security.Principal.ID
			} else if input.AcceptedBy != nil && *input.AcceptedBy != "" {
				acceptedBy = *input.AcceptedBy
			}
			if _, err := s.store.AcceptDecision(ctx, enginesession.AcceptDecisionRequest{
				SessionID: input.SessionID, RunID: run.ID, DiagnosticID: diagnostic.ID,
				AcceptedBy: acceptedBy,
			}); err != nil {
				return nil, err
			}
			out := s.toGraphQLDiagnostic(run, diagnostic)
			s.publish("diagnostic.accepted", input.SessionID, run.ID, "diagnostic fix accepted")
			return &out, nil
		}
	}
	return nil, fmt.Errorf("session diagnostic %q not found", input.DiagnosticID)
}

// ErrExportUnauthenticated means no verified caller identity reached the export
// mutation, so the disclosure could not be attributed.
var ErrExportUnauthenticated = fmt.Errorf("integration bundle export requires a verified caller identity")

// ErrExportRawPayloadForbidden names the missing decision, not the inventory:
// it says which grant is required, never whether the session or its raw
// payloads exist.
var ErrExportRawPayloadForbidden = fmt.Errorf(
	"integration bundle export with raw payloads requires the %s grant", enginesession.PHIExportRole)

func (s *integrationSessionService) exportBundle(
	ctx context.Context,
	input model.ExportIntegrationBundleInput,
) (*model.IntegrationBundle, error) {
	security, authenticated := requestsecurity.SecurityContextFromContext(ctx)
	if !authenticated {
		return nil, ErrExportUnauthenticated
	}
	includeRaw := input.IncludeRawPayload != nil && *input.IncludeRawPayload
	// The raw-payload decision runs before the store is touched, so a refused
	// disclosure assembles no bundle and writes no export row.
	if includeRaw && !enginesession.HasRole(security.Principal.Roles, enginesession.PHIExportRole) {
		return nil, ErrExportRawPayloadForbidden
	}
	bundle, err := s.store.ExportBundle(ctx, enginesession.ExportRequest{
		SessionID:         input.SessionID,
		Principal:         security.Principal,
		Reason:            input.Reason,
		IncludeRawPayload: includeRaw,
	})
	if err != nil {
		return nil, err
	}
	session, err := s.toGraphQLSession(bundle.Session, true)
	if err != nil {
		return nil, err
	}
	if !includeRaw {
		for i := range session.Samples {
			session.Samples[i].RawPayload = nil
		}
	}
	return &model.IntegrationBundle{
		SessionID:           input.SessionID,
		ExportedAt:          bundle.ExportedAt,
		Session:             *session,
		Samples:             append([]model.SessionSample(nil), session.Samples...),
		Artifacts:           append([]model.SessionArtifact(nil), session.Artifacts...),
		Runs:                cloneRuns(session.Runs),
		WorkflowSimulations: cloneWorkflowSimulations(session.WorkflowSimulations),
		Publications:        clonePublications(session.Publications),
		Diagnostics:         append([]model.SessionDiagnostic(nil), session.Diagnostics...),
	}, nil
}

func (s *integrationSessionService) subscribe(ctx context.Context, sessionID string, runID *string) (<-chan *model.IntegrationSessionEvent, error) {
	if _, err := s.store.GetSession(context.Background(), sessionID); err != nil {
		return nil, err
	}
	source := s.hub.Subscribe(ctx, sessionID)
	out := make(chan *model.IntegrationSessionEvent, 16)
	go func() {
		defer close(out)
		for event := range source {
			if runID != nil && event.RunID != *runID {
				continue
			}
			gql := s.toGraphQLEvent(event)
			select {
			case out <- gql:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}

func (s *integrationSessionService) publish(eventType, sessionID, runID, message string) {
	s.hub.Publish(enginesession.StreamEvent{
		Type:      enginesession.StreamEventType(eventType),
		SessionID: sessionID,
		RunID:     runID,
		Payload:   message,
	})
}

func (s *integrationSessionService) cloneSession(id string, includeChildren bool) (*model.IntegrationSession, error) {
	session, err := s.store.GetSession(context.Background(), id)
	if err != nil {
		return nil, err
	}
	return s.toGraphQLSession(*session, includeChildren)
}

func (s *integrationSessionService) toGraphQLSession(session enginesession.Session, includeChildren bool) (*model.IntegrationSession, error) {
	description := strPtrEmpty(session.Description)
	out := &model.IntegrationSession{
		ID:                  session.ID,
		Name:                session.Name,
		Description:         description,
		Archived:            session.Status == enginesession.SessionStatusArchived,
		CreatedAt:           session.CreatedAt,
		UpdatedAt:           session.UpdatedAt,
		Samples:             []model.SessionSample{},
		Artifacts:           []model.SessionArtifact{},
		Runs:                []model.SessionRun{},
		Diagnostics:         []model.SessionDiagnostic{},
		WorkflowSimulations: []model.SessionWorkflowSimulation{},
		Publications:        []model.SessionPublication{},
	}
	if !includeChildren {
		return out, nil
	}

	samples, err := s.listSamples(session.ID)
	if err != nil {
		return nil, err
	}
	out.Samples = samples

	artifacts, err := s.listArtifacts(session.ID)
	if err != nil {
		return nil, err
	}
	out.Artifacts = artifacts
	for i := range out.Artifacts {
		artifact := out.Artifacts[i]
		switch artifact.Kind {
		case string(enginesession.ArtifactKindMappingProfile):
			out.CurrentProfileDraft = &artifact
		case string(enginesession.ArtifactKindWorkflowDraft):
			out.CurrentWorkflowDraft = &artifact
		}
	}

	runs, err := s.listRuns(session.ID)
	if err != nil {
		return nil, err
	}
	out.Runs = runs

	simulations, err := s.listWorkflowSimulations(session.ID)
	if err != nil {
		return nil, err
	}
	out.WorkflowSimulations = simulations

	publications, err := s.listPublications(session.ID)
	if err != nil {
		return nil, err
	}
	out.Publications = publications

	diagnostics, err := s.listDiagnostics(session.ID, nil)
	if err != nil {
		return nil, err
	}
	out.Diagnostics = diagnostics
	return out, nil
}

func (s *integrationSessionService) toGraphQLSample(sample enginesession.Sample) *model.SessionSample {
	checksum := sha256.Sum256([]byte(sample.Raw))
	var raw *string
	if sample.PHIPolicy == enginesession.PHIPolicyRetain {
		raw = &sample.Raw
	}
	source := strPtrEmpty(sample.Source)
	return &model.SessionSample{
		ID:              sample.ID,
		SessionID:       sample.SessionID,
		Name:            sample.Name,
		Format:          toGraphQLSourceFormat(sample.Format),
		Source:          source,
		RawPayload:      raw,
		PayloadChecksum: hex.EncodeToString(checksum[:]),
		CreatedAt:       sample.CreatedAt,
	}
}

func toGraphQLArtifact(draft enginesession.ArtifactDraft) *model.SessionArtifact {
	return &model.SessionArtifact{
		ID: draft.ID, RevisionID: draft.RevisionID, SessionID: draft.SessionID,
		Kind: string(draft.Kind), Name: draft.Name, Content: string(draft.Content),
		Version: draft.Version, Digest: draft.Digest,
		CreatedAt: draft.CreatedAt, UpdatedAt: draft.UpdatedAt,
	}
}

func toGraphQLWorkflowSimulation(record enginesession.WorkflowSimulation) *model.SessionWorkflowSimulation {
	eventsOut := make([]model.SessionWorkflowEventTrace, len(record.Events))
	for eventIndex, event := range record.Events {
		routes := make([]model.SessionWorkflowRouteTrace, len(event.Routes))
		for routeIndex, route := range event.Routes {
			transforms := make([]model.SessionWorkflowTransformTrace, len(route.Transforms))
			for transformIndex, transform := range route.Transforms {
				transforms[transformIndex] = model.SessionWorkflowTransformTrace{Index: transform.Index, Type: transform.Type, Status: transform.Status}
			}
			actions := make([]model.SessionWorkflowActionTrace, len(route.Actions))
			for actionIndex, action := range route.Actions {
				actions[actionIndex] = model.SessionWorkflowActionTrace{
					ID: action.ID, Type: action.Type,
					DestinationArtifactID: strPtrEmpty(action.DestinationArtifactID),
				}
			}
			routes[routeIndex] = model.SessionWorkflowRouteTrace{
				Name: route.Name, Matched: route.Matched, SkipReason: strPtrEmpty(route.SkipReason),
				DiagnosticCodes: append([]string(nil), route.DiagnosticCodes...),
				Transforms:      transforms, Actions: actions,
			}
		}
		eventsOut[eventIndex] = model.SessionWorkflowEventTrace{
			RunID: event.RunID, EventID: event.EventID, EventType: event.EventType, Routes: routes,
		}
	}
	return &model.SessionWorkflowSimulation{
		ID: record.ID, SessionID: record.SessionID, WorkflowArtifactID: record.WorkflowArtifactID,
		WorkflowRevisionID: record.WorkflowRevisionID, WorkflowRevisionDigest: record.WorkflowRevisionDigest,
		SourceRunIds: append([]string(nil), record.SourceRunIDs...), Events: eventsOut, CreatedAt: record.CreatedAt,
	}
}

func toGraphQLWorkflowSimulationDelta(delta enginesession.WorkflowSimulationDelta) *model.SessionWorkflowSimulationDelta {
	return &model.SessionWorkflowSimulationDelta{
		BaselineSimulationID: delta.BaselineSimulationID, CandidateSimulationID: delta.CandidateSimulationID,
		AddedEvents: append([]string(nil), delta.AddedEvents...), RemovedEvents: append([]string(nil), delta.RemovedEvents...),
		AddedMatchedRoutes: append([]string(nil), delta.AddedMatchedRoutes...), RemovedMatchedRoutes: append([]string(nil), delta.RemovedMatchedRoutes...),
		AddedTransforms: append([]string(nil), delta.AddedTransforms...), RemovedTransforms: append([]string(nil), delta.RemovedTransforms...),
		AddedActions: append([]string(nil), delta.AddedActions...), RemovedActions: append([]string(nil), delta.RemovedActions...),
	}
}

func toGraphQLPublication(record enginesession.Publication) *model.SessionPublication {
	return &model.SessionPublication{
		ID: record.ID, SessionID: record.SessionID, Version: record.Version,
		SessionProfile: &model.IntegrationArtifactRevision{
			ArtifactID: record.ProfileArtifactID, RevisionID: record.ProfileRevisionID, Digest: record.ProfileRevisionDigest,
		},
		SessionWorkflow: &model.IntegrationArtifactRevision{
			ArtifactID: record.WorkflowArtifactID, RevisionID: record.WorkflowRevisionID, Digest: record.WorkflowRevisionDigest,
		},
		WorkflowSimulationID: record.WorkflowSimulationID,
		DefinitionRevision:   toGraphQLIntegrationArtifactRevision(record.DefinitionRevision),
		DefinitionVersion:    int(record.DefinitionVersion),
		ProductionProfile:    toGraphQLIntegrationArtifactRevision(record.ProductionProfile),
		ProductionWorkflow:   toGraphQLIntegrationArtifactRevision(record.ProductionWorkflow),
		SourceRunIds:         append([]string(nil), record.SourceRunIDs...),
		ManifestDigest:       record.ManifestDigest, SignatureAlgorithm: record.SignatureAlgorithm,
		SigningKeyID: record.SigningKeyID, PublishedBy: record.PublishedBy, Reason: record.Reason, CreatedAt: record.CreatedAt,
	}
}

func toGraphQLIntegrationArtifactRevision(ref integration.ArtifactRevisionRef) *model.IntegrationArtifactRevision {
	return &model.IntegrationArtifactRevision{ArtifactID: ref.ArtifactID, RevisionID: ref.RevisionID, Digest: ref.Digest}
}

func toGraphQLDeploymentSnapshot(snapshot lifecycle.Snapshot) *model.SessionDeploymentSnapshot {
	return &model.SessionDeploymentSnapshot{
		DefinitionRevision: toGraphQLIntegrationArtifactRevision(snapshot.DefinitionRevision),
		State:              string(snapshot.State), Version: int(snapshot.Version), ReleaseID: strPtrEmpty(snapshot.ReleaseID), Health: string(snapshot.Health),
	}
}

func (s *integrationSessionService) toGraphQLRun(run enginesession.Run) (*model.SessionRun, error) {
	stages := make([]model.RunStage, 0, len(run.Stages))
	for _, stage := range run.Stages {
		stages = append(stages, toGraphQLStage(stage))
	}
	diagnostics := make([]model.SessionDiagnostic, 0, len(run.Diagnostics))
	lineage := make([]model.LineageLink, 0, len(run.Lineage))
	warnings := make([]model.ParseWarning, 0, len(run.Diagnostics))
	for _, diagnostic := range run.Diagnostics {
		diagnostics = append(diagnostics, s.toGraphQLDiagnostic(run, diagnostic))
		if diagnostic.Severity != "error" {
			path := strPtrEmpty(diagnostic.Path)
			warnings = append(warnings, model.ParseWarning{
				Phase:    diagnostic.Phase,
				Code:     diagnostic.Code,
				Message:  diagnostic.Message,
				Path:     path,
				Severity: strPtrEmpty(diagnostic.Severity),
			})
		}
	}
	for _, link := range run.Lineage {
		lineage = append(lineage, toGraphQLLineageLink(link))
	}
	eventsOut := make([]model.Event, 0, len(run.Events))
	for _, parsed := range run.Events {
		event, err := parsedEventToGraphQL(parsed, run.Source)
		if err != nil {
			return nil, err
		}
		if event != nil {
			eventsOut = append(eventsOut, event)
		}
	}
	sampleID := strPtrEmpty(run.SampleID)
	profileRevisionID := strPtrEmpty(run.ProfileRevisionID)
	profileRevisionDigest := strPtrEmpty(run.ProfileRevisionDigest)
	return &model.SessionRun{
		ID:                    run.ID,
		SessionID:             run.SessionID,
		SampleID:              sampleID,
		Status:                toGraphQLRunStatus(run.Status),
		ProfileRevisionID:     profileRevisionID,
		ProfileRevisionDigest: profileRevisionDigest,
		CreatedAt:             run.CreatedAt,
		CompletedAt:           run.FinishedAt,
		Stages:                stages,
		Diagnostics:           diagnostics,
		Lineage:               lineage,
		Events:                eventsOut,
		Warnings:              warnings,
	}, nil
}

func toGraphQLStage(stage enginesession.RunStage) model.RunStage {
	duration := 0
	if stage.FinishedAt != nil {
		duration = int(stage.FinishedAt.Sub(stage.StartedAt).Milliseconds())
	}
	summary := strPtrEmpty(stage.Error)
	return model.RunStage{
		ID:          stage.Name,
		Name:        stage.Name,
		Status:      string(stage.Status),
		StartedAt:   stage.StartedAt,
		CompletedAt: stage.FinishedAt,
		DurationMs:  duration,
		Summary:     summary,
	}
}

func (s *integrationSessionService) toGraphQLDiagnostic(run enginesession.Run, diagnostic enginesession.Diagnostic) model.SessionDiagnostic {
	runID := run.ID
	sampleID := run.SampleID
	path := strPtrEmpty(diagnostic.Path)
	fix := "Review the source profile or sample payload for this warning."
	acceptedAt, accepted := s.acceptedDecision(run.ID, diagnostic.ID, run.SessionID)
	var acceptedAtPtr *time.Time
	if accepted {
		acceptedAtPtr = &acceptedAt
	}
	lineage := make([]model.LineageLink, 0)
	for _, link := range run.Lineage {
		if diagnostic.Path != "" && link.SourcePath != diagnostic.Path {
			continue
		}
		lineage = append(lineage, toGraphQLLineageLink(link))
	}
	if len(lineage) == 0 && diagnostic.Path != "" {
		lineage = append(lineage, model.LineageLink{SourcePath: diagnostic.Path})
	}
	return model.SessionDiagnostic{
		ID:            diagnostic.ID,
		SessionID:     run.SessionID,
		RunID:         &runID,
		SampleID:      &sampleID,
		Severity:      diagnostic.Severity,
		Code:          diagnostic.Code,
		Message:       diagnostic.Message,
		Path:          path,
		FixSuggestion: &fix,
		Accepted:      accepted,
		AcceptedAt:    acceptedAtPtr,
		Lineage:       lineage,
	}
}

func (s *integrationSessionService) acceptedDecision(runID, diagnosticID, sessionID string) (time.Time, bool) {
	decisions, err := s.store.ListDecisions(context.Background(), sessionID)
	if err != nil {
		return time.Time{}, false
	}
	for _, decision := range decisions {
		if decision.RunID == runID && decision.DiagnosticID == diagnosticID {
			return decision.AcceptedAt, true
		}
	}
	return time.Time{}, false
}

func (s *integrationSessionService) toGraphQLEvent(event enginesession.StreamEvent) *model.IntegrationSessionEvent {
	runID := strPtrEmpty(event.RunID)
	gql := &model.IntegrationSessionEvent{
		ID:        event.ID,
		Type:      string(event.Type),
		SessionID: event.SessionID,
		RunID:     runID,
		Message:   streamEventMessage(event.Type),
		Timestamp: event.At,
	}
	if session, err := s.cloneSession(event.SessionID, false); err == nil {
		gql.Session = session
	}
	if event.RunID != "" {
		if run, err := s.getRun(event.RunID); err == nil {
			gql.Run = run
		}
	}
	return gql
}

func toGraphQLLineageLink(link enginesession.LineageLink) model.LineageLink {
	return model.LineageLink{
		SourcePath: link.SourcePath,
		TargetPath: strPtrEmpty(link.TargetPath),
	}
}

func streamEventMessage(eventType enginesession.StreamEventType) string {
	switch eventType {
	case enginesession.StreamEventRunStarted:
		return "session preview started"
	case enginesession.StreamEventStageStarted:
		return "session preview stage started"
	case enginesession.StreamEventStageCompleted:
		return "session preview stage completed"
	case enginesession.StreamEventDiagnostic:
		return "session diagnostic reported"
	case enginesession.StreamEventRunCompleted:
		return "session preview completed"
	case enginesession.StreamEventRunFailed:
		return "session preview failed"
	default:
		return strings.ReplaceAll(string(eventType), ".", " ")
	}
}

func parsedEventToGraphQL(parsed enginesession.ParsedEvent, source string) (model.Event, error) {
	if len(parsed.Payload) == 0 {
		return nil, nil
	}
	switch events.EventType(parsed.Type) {
	case events.EventPatientAdmit:
		var event events.PatientAdmitEvent
		if err := json.Unmarshal(parsed.Payload, &event); err != nil {
			return nil, err
		}
		return convertToGraphQLEvent(&event, source, model.SourceFormatHL7v2, nil), nil
	case events.EventPatientDischarge:
		var event events.PatientDischargeEvent
		if err := json.Unmarshal(parsed.Payload, &event); err != nil {
			return nil, err
		}
		return convertToGraphQLEvent(&event, source, model.SourceFormatHL7v2, nil), nil
	case events.EventLabResult:
		var event events.LabResultEvent
		if err := json.Unmarshal(parsed.Payload, &event); err != nil {
			return nil, err
		}
		return convertToGraphQLEvent(&event, source, model.SourceFormatHL7v2, nil), nil
	case events.EventAppointmentScheduled, events.EventAppointmentCancelled, events.EventAppointmentNoShow:
		var event events.AppointmentEvent
		if err := json.Unmarshal(parsed.Payload, &event); err != nil {
			return nil, err
		}
		return convertToGraphQLEvent(&event, source, model.SourceFormatHL7v2, nil), nil
	case events.EventDocument:
		var event events.DocumentEvent
		if err := json.Unmarshal(parsed.Payload, &event); err != nil {
			return nil, err
		}
		return convertToGraphQLEvent(&event, source, model.SourceFormatHL7v2, nil), nil
	default:
		return nil, nil
	}
}

func toArtifactKind(kind string) enginesession.ArtifactKind {
	switch kind {
	case "profile":
		return enginesession.ArtifactKindMappingProfile
	case "workflow":
		return enginesession.ArtifactKindWorkflowDraft
	default:
		return enginesession.ArtifactKind(kind)
	}
}

func toGraphQLSourceFormat(format events.SourceFormat) model.SourceFormat {
	switch format {
	case events.FormatHL7v2:
		return model.SourceFormatHL7v2
	case events.FormatFHIR:
		return model.SourceFormatFHIR
	case events.FormatCSV:
		return model.SourceFormatCSV
	case events.FormatEDI837:
		return model.SourceFormatEDI837
	case events.FormatEDI835:
		return model.SourceFormatEDI835
	case events.FormatCDA:
		return model.SourceFormatCDA
	default:
		return model.SourceFormatHL7v2
	}
}

func toGraphQLRunStatus(status enginesession.RunStatus) string {
	switch status {
	case enginesession.RunStatusSucceeded:
		return "completed"
	default:
		return string(status)
	}
}

func cloneRuns(in []model.SessionRun) []model.SessionRun {
	out := make([]model.SessionRun, len(in))
	for i, run := range in {
		out[i] = run
		out[i].Stages = append([]model.RunStage(nil), run.Stages...)
		out[i].Diagnostics = append([]model.SessionDiagnostic(nil), run.Diagnostics...)
		out[i].Lineage = append([]model.LineageLink(nil), run.Lineage...)
		out[i].Events = append([]model.Event(nil), run.Events...)
		out[i].Warnings = append([]model.ParseWarning(nil), run.Warnings...)
	}
	return out
}

func cloneWorkflowSimulations(in []model.SessionWorkflowSimulation) []model.SessionWorkflowSimulation {
	out := make([]model.SessionWorkflowSimulation, len(in))
	for simulationIndex := range in {
		out[simulationIndex] = in[simulationIndex]
		out[simulationIndex].SourceRunIds = append([]string(nil), in[simulationIndex].SourceRunIds...)
		out[simulationIndex].Events = make([]model.SessionWorkflowEventTrace, len(in[simulationIndex].Events))
		for eventIndex := range in[simulationIndex].Events {
			out[simulationIndex].Events[eventIndex] = in[simulationIndex].Events[eventIndex]
			out[simulationIndex].Events[eventIndex].Routes = make([]model.SessionWorkflowRouteTrace, len(in[simulationIndex].Events[eventIndex].Routes))
			for routeIndex := range in[simulationIndex].Events[eventIndex].Routes {
				route := in[simulationIndex].Events[eventIndex].Routes[routeIndex]
				out[simulationIndex].Events[eventIndex].Routes[routeIndex] = route
				out[simulationIndex].Events[eventIndex].Routes[routeIndex].DiagnosticCodes = append([]string(nil), route.DiagnosticCodes...)
				out[simulationIndex].Events[eventIndex].Routes[routeIndex].Transforms = append([]model.SessionWorkflowTransformTrace(nil), route.Transforms...)
				out[simulationIndex].Events[eventIndex].Routes[routeIndex].Actions = append([]model.SessionWorkflowActionTrace(nil), route.Actions...)
			}
		}
		out[simulationIndex].Delta = nil
	}
	return out
}

func clonePublications(in []model.SessionPublication) []model.SessionPublication {
	out := make([]model.SessionPublication, len(in))
	copy(out, in)
	for index := range out {
		out[index].SourceRunIds = append([]string(nil), out[index].SourceRunIds...)
	}
	return out
}
