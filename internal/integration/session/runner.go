package session

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/parser/hl7v2"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
)

type Runner struct {
	store Store
	hub   *Hub
	now   func() time.Time
}

func NewRunner(store Store, hub *Hub) *Runner {
	if hub == nil {
		hub = NewHub()
	}
	return &Runner{
		store: store,
		hub:   hub,
		now:   func() time.Time { return time.Now().UTC() },
	}
}

func (r *Runner) RunHL7v2(ctx context.Context, req RunRequest) (*Run, error) {
	if r.store == nil {
		return nil, fmt.Errorf("%w: runner store is required", ErrInvalid)
	}
	sample, err := r.store.GetSample(ctx, req.SessionID, req.SampleID)
	if err != nil {
		return nil, err
	}
	if sample.Format != events.FormatHL7v2 {
		return nil, fmt.Errorf("%w: sample format %q is not hl7v2", ErrInvalid, sample.Format)
	}
	source := req.Source
	if source == "" {
		source = sample.Source
	}
	if source == "" {
		source = "integration-session"
	}

	run, err := r.store.CreateRun(ctx, req.SessionID, req.SampleID, source)
	if err != nil {
		return nil, err
	}

	started := r.now()
	run.Status = RunStatusRunning
	run.StartedAt = &started
	run, err = r.updateRun(ctx, *run)
	if err != nil {
		return nil, err
	}
	r.publish(StreamEvent{Type: StreamEventRunStarted, SessionID: req.SessionID, RunID: run.ID, Payload: *run})

	if err := r.startStage(ctx, run, "load_sample"); err != nil {
		return nil, err
	}
	if err := r.completeStage(ctx, run, "load_sample", ""); err != nil {
		return nil, err
	}

	if err := r.startStage(ctx, run, "parse_hl7v2"); err != nil {
		return nil, err
	}
	parserConfig := hl7v2.ParserConfig{}
	if req.ProfileRevisionID != "" {
		revision, revisionErr := r.store.GetArtifactRevision(ctx, req.SessionID, req.ProfileRevisionID)
		if revisionErr != nil {
			return r.finishFailed(ctx, run, fmt.Errorf("load profile revision: %w", revisionErr))
		}
		if revision.Kind != ArtifactKindMappingProfile {
			return r.finishFailed(ctx, run, fmt.Errorf("%w: artifact revision is not a mapping profile", ErrInvalid))
		}
		digest := sha256.Sum256(revision.Content)
		if revision.Digest != fmt.Sprintf("sha256:%x", digest) {
			return r.finishFailed(ctx, run, fmt.Errorf("%w: profile revision digest mismatch", ErrImmutable))
		}
		profileRef, revisionErr := processor.NewProfileRevisionReference(revision.ID, revision.Version, revision.Content)
		if revisionErr != nil {
			return r.finishFailed(ctx, run, fmt.Errorf("compile profile revision: %w", revisionErr))
		}
		compiled, timezone, revisionErr := processor.CompileProfileRevision(profileRef, revision.Content)
		if revisionErr != nil {
			return r.finishFailed(ctx, run, fmt.Errorf("compile profile revision: %w", revisionErr))
		}
		parserConfig.DefaultTimezone = timezone
		run.ProfileID = revision.ID
		run.ProfileRevisionID = revision.RevisionID
		run.ProfileRevisionDigest = revision.Digest
		parser := hl7v2.NewParser(source, parserConfig)
		parser.SetProfile(compiled)
		result, parseErr := parser.ParseWithResult(sample.Raw)
		return r.finishParsed(ctx, run, sample, result, parseErr)
	}
	parser := hl7v2.NewParser(source, parserConfig)
	result, parseErr := parser.ParseWithResult(sample.Raw)
	return r.finishParsed(ctx, run, sample, result, parseErr)
}

func (r *Runner) finishParsed(
	ctx context.Context,
	run *Run,
	sample *Sample,
	result *hl7v2.ParseResult,
	parseErr error,
) (*Run, error) {
	if parseErr != nil {
		if err := r.failStage(ctx, run, "parse_hl7v2", parseErr.Error()); err != nil {
			return nil, err
		}
		return r.finishFailed(ctx, run, parseErr)
	}
	if run.ProfileID == "" {
		run.ProfileID = result.ProfileID
	}
	if err := r.completeStage(ctx, run, "parse_hl7v2", ""); err != nil {
		return nil, err
	}

	if err := r.startStage(ctx, run, "normalize_diagnostics"); err != nil {
		return nil, err
	}
	run.Diagnostics = NormalizeDiagnostics(result.Warnings)
	for _, diagnostic := range run.Diagnostics {
		r.publish(StreamEvent{Type: StreamEventDiagnostic, SessionID: run.SessionID, RunID: run.ID, Payload: diagnostic})
	}
	if err := r.completeStage(ctx, run, "normalize_diagnostics", ""); err != nil {
		return nil, err
	}

	if err := r.startStage(ctx, run, "build_lineage"); err != nil {
		return nil, err
	}
	run.Lineage = BuildHL7v2Lineage(sample.Raw, result.Event)
	if err := r.completeStage(ctx, run, "build_lineage", ""); err != nil {
		return nil, err
	}

	parsedEvent, err := parsedEventFrom(result.Event)
	if err != nil {
		return r.finishFailed(ctx, run, err)
	}
	run.Events = []ParsedEvent{parsedEvent}

	finished := r.now()
	run.Status = RunStatusSucceeded
	run.FinishedAt = &finished
	run, err = r.updateRun(ctx, *run)
	if err != nil {
		return nil, err
	}
	r.publish(StreamEvent{Type: StreamEventRunCompleted, SessionID: run.SessionID, RunID: run.ID, Payload: *run})
	return run, nil
}

func (r *Runner) startStage(ctx context.Context, run *Run, name string) error {
	run.Stages = append(run.Stages, RunStage{
		Name:      name,
		Status:    StageStatusRunning,
		StartedAt: r.now(),
	})
	updated, err := r.updateRun(ctx, *run)
	if err != nil {
		return err
	}
	*run = *updated
	r.publish(StreamEvent{Type: StreamEventStageStarted, SessionID: run.SessionID, RunID: run.ID, Payload: run.Stages[len(run.Stages)-1]})
	return nil
}

func (r *Runner) completeStage(ctx context.Context, run *Run, name, errMsg string) error {
	for i := len(run.Stages) - 1; i >= 0; i-- {
		if run.Stages[i].Name != name {
			continue
		}
		finished := r.now()
		run.Stages[i].FinishedAt = &finished
		if errMsg == "" {
			run.Stages[i].Status = StageStatusSucceeded
		} else {
			run.Stages[i].Status = StageStatusFailed
			run.Stages[i].Error = errMsg
		}
		break
	}
	updated, err := r.updateRun(ctx, *run)
	if err != nil {
		return err
	}
	*run = *updated
	r.publish(StreamEvent{Type: StreamEventStageCompleted, SessionID: run.SessionID, RunID: run.ID, Payload: run.Stages[len(run.Stages)-1]})
	return nil
}

func (r *Runner) failStage(ctx context.Context, run *Run, name, errMsg string) error {
	return r.completeStage(ctx, run, name, errMsg)
}

func (r *Runner) finishFailed(ctx context.Context, run *Run, err error) (*Run, error) {
	finished := r.now()
	run.Status = RunStatusFailed
	run.Error = err.Error()
	run.FinishedAt = &finished
	run.Diagnostics = append(run.Diagnostics, Diagnostic{
		ID:        "diag_error",
		Severity:  "error",
		Phase:     "syntactic",
		Code:      "PARSE_FAILED",
		Message:   err.Error(),
		Source:    "hl7v2_parser",
		CreatedAt: finished,
	})
	updated, updateErr := r.updateRun(ctx, *run)
	if updateErr != nil {
		return nil, updateErr
	}
	r.publish(StreamEvent{Type: StreamEventRunFailed, SessionID: updated.SessionID, RunID: updated.ID, Payload: *updated})
	return updated, err
}

func (r *Runner) updateRun(ctx context.Context, run Run) (*Run, error) {
	return r.store.UpdateRun(ctx, run)
}

func (r *Runner) publish(event StreamEvent) {
	if r.hub != nil {
		r.hub.Publish(event)
	}
}

func parsedEventFrom(event any) (ParsedEvent, error) {
	payload, err := json.Marshal(event)
	if err != nil {
		return ParsedEvent{}, fmt.Errorf("marshal parsed event: %w", err)
	}
	meta, ok := eventMeta(event)
	if !ok {
		return ParsedEvent{Payload: payload}, nil
	}
	return ParsedEvent{
		ID:              meta.ID,
		Type:            string(meta.Type),
		SourceMessageID: meta.SourceMessageID,
		Payload:         payload,
	}, nil
}

func eventMeta(event any) (events.EventMeta, bool) {
	switch e := event.(type) {
	case *events.PatientAdmitEvent:
		return e.EventMeta, true
	case *events.PatientDischargeEvent:
		return e.EventMeta, true
	case *events.LabResultEvent:
		return e.EventMeta, true
	case *events.AppointmentEvent:
		return e.EventMeta, true
	case *events.ImmunizationEvent:
		return e.EventMeta, true
	case *events.MedicationRequestEvent:
		return e.EventMeta, true
	case *events.DocumentEvent:
		return e.EventMeta, true
	case *events.FinancialTransactionEvent:
		return e.EventMeta, true
	default:
		return events.EventMeta{}, false
	}
}
