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

// runProgress is one run's in-flight state: the record as it will be written,
// and the envelopes recorded so far in publish order.
//
// Nothing here touches the store. Before 2026-09-27 every stage transition was
// its own UpdateRun (a SELECT ... FOR UPDATE plus an UPDATE in its own
// transaction) and every publish was its own committed INSERT, so a run whose
// parse takes well under a millisecond performed ~25 sequential round trips and
// its wall-clock was the sum of their commit latencies: ~3 s each on a
// saturated CI runner, 3–15 s per run on a busy docker host. The run's
// durable record and the stream's content are the same either way, because the
// SSE projection re-reads the run at delivery time and the relay polls the log
// on a 250 ms tick, so subscribers already saw a run's envelopes as one burst.
type runProgress struct {
	run    *Run
	events []StreamEvent
}

// record buffers one envelope. At is stamped now, not at publish time, so the
// envelope timeline still reflects when each stage happened.
func (p *runProgress) record(now time.Time, eventType StreamEventType, payload any) {
	p.events = append(p.events, StreamEvent{
		Type:      eventType,
		SessionID: p.run.SessionID,
		RunID:     p.run.ID,
		Payload:   payload,
		At:        now,
	})
}

// RunHL7v2 executes one preview run for an HL7v2 sample.
//
// Persistence contract: CreateRun claims the identifier (status pending), the
// stages run in memory, and one UpdateRun writes the terminal record with its
// stages, diagnostics, lineage and events. The durable record therefore goes
// pending → succeeded|failed without a persisted "running" state. The stream
// envelopes are published as one batch after that write succeeds; if it fails,
// nothing is published, so the stream never describes a run the record does
// not hold. ErrImmutable for terminal runs is the store's contract and is
// unchanged.
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

	created, err := r.store.CreateRun(ctx, req.SessionID, req.SampleID, source)
	if err != nil {
		return nil, err
	}
	progress := &runProgress{run: created}
	run := progress.run

	started := r.now()
	run.Status = RunStatusRunning
	run.StartedAt = &started
	progress.record(started, StreamEventRunStarted, *cloneRun(run))

	r.startStage(progress, "load_sample")
	r.completeStage(progress, "load_sample", "")

	r.startStage(progress, "parse_hl7v2")
	parserConfig := hl7v2.ParserConfig{}
	if req.ProfileRevisionID != "" {
		revision, revisionErr := r.store.GetArtifactRevision(ctx, req.SessionID, req.ProfileRevisionID)
		if revisionErr != nil {
			return r.finishFailed(ctx, progress, fmt.Errorf("load profile revision: %w", revisionErr))
		}
		if revision.Kind != ArtifactKindMappingProfile {
			return r.finishFailed(ctx, progress, fmt.Errorf("%w: artifact revision is not a mapping profile", ErrInvalid))
		}
		digest := sha256.Sum256(revision.Content)
		if revision.Digest != fmt.Sprintf("sha256:%x", digest) {
			return r.finishFailed(ctx, progress, fmt.Errorf("%w: profile revision digest mismatch", ErrImmutable))
		}
		profileRef, revisionErr := processor.NewProfileRevisionReference(revision.ID, revision.Version, revision.Content)
		if revisionErr != nil {
			return r.finishFailed(ctx, progress, fmt.Errorf("compile profile revision: %w", revisionErr))
		}
		compiled, timezone, revisionErr := processor.CompileProfileRevision(profileRef, revision.Content)
		if revisionErr != nil {
			return r.finishFailed(ctx, progress, fmt.Errorf("compile profile revision: %w", revisionErr))
		}
		parserConfig.DefaultTimezone = timezone
		run.ProfileID = revision.ID
		run.ProfileRevisionID = revision.RevisionID
		run.ProfileRevisionDigest = revision.Digest
		parser := hl7v2.NewParser(source, parserConfig)
		parser.SetProfile(compiled)
		result, parseErr := parser.ParseWithResult(sample.Raw)
		return r.finishParsed(ctx, progress, sample, result, parseErr)
	}
	parser := hl7v2.NewParser(source, parserConfig)
	result, parseErr := parser.ParseWithResult(sample.Raw)
	return r.finishParsed(ctx, progress, sample, result, parseErr)
}

func (r *Runner) finishParsed(
	ctx context.Context,
	progress *runProgress,
	sample *Sample,
	result *hl7v2.ParseResult,
	parseErr error,
) (*Run, error) {
	run := progress.run
	if parseErr != nil {
		r.completeStage(progress, "parse_hl7v2", parseErr.Error())
		return r.finishFailed(ctx, progress, parseErr)
	}
	if run.ProfileID == "" {
		run.ProfileID = result.ProfileID
	}
	r.completeStage(progress, "parse_hl7v2", "")

	r.startStage(progress, "normalize_diagnostics")
	run.Diagnostics = NormalizeDiagnostics(result.Warnings)
	for _, diagnostic := range run.Diagnostics {
		progress.record(r.now(), StreamEventDiagnostic, diagnostic)
	}
	r.completeStage(progress, "normalize_diagnostics", "")

	r.startStage(progress, "build_lineage")
	run.Lineage = BuildHL7v2Lineage(sample.Raw, result.Event)
	r.completeStage(progress, "build_lineage", "")

	parsedEvent, err := parsedEventFrom(result.Event)
	if err != nil {
		return r.finishFailed(ctx, progress, err)
	}
	run.Events = []ParsedEvent{parsedEvent}

	finished := r.now()
	run.Status = RunStatusSucceeded
	run.FinishedAt = &finished
	return r.commit(ctx, progress, StreamEventRunCompleted)
}

func (r *Runner) startStage(progress *runProgress, name string) {
	run := progress.run
	now := r.now()
	run.Stages = append(run.Stages, RunStage{
		Name:      name,
		Status:    StageStatusRunning,
		StartedAt: now,
	})
	progress.record(now, StreamEventStageStarted, run.Stages[len(run.Stages)-1])
}

func (r *Runner) completeStage(progress *runProgress, name, errMsg string) {
	run := progress.run
	finished := r.now()
	for i := len(run.Stages) - 1; i >= 0; i-- {
		if run.Stages[i].Name != name {
			continue
		}
		run.Stages[i].FinishedAt = &finished
		if errMsg == "" {
			run.Stages[i].Status = StageStatusSucceeded
		} else {
			run.Stages[i].Status = StageStatusFailed
			run.Stages[i].Error = errMsg
		}
		break
	}
	progress.record(finished, StreamEventStageCompleted, run.Stages[len(run.Stages)-1])
}

func (r *Runner) finishFailed(ctx context.Context, progress *runProgress, err error) (*Run, error) {
	run := progress.run
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
	updated, commitErr := r.commit(ctx, progress, StreamEventRunFailed)
	if commitErr != nil {
		return nil, commitErr
	}
	return updated, err
}

// commit writes the terminal record in one UpdateRun, records the terminal
// envelope with the record as written, and publishes every envelope of the
// run in order as one batch.
func (r *Runner) commit(ctx context.Context, progress *runProgress, terminal StreamEventType) (*Run, error) {
	updated, err := r.store.UpdateRun(ctx, *progress.run)
	if err != nil {
		return nil, err
	}
	progress.run = updated
	progress.record(r.now(), terminal, *cloneRun(updated))
	if r.hub != nil {
		r.hub.PublishAll(progress.events)
	}
	return updated, nil
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
