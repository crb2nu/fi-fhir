package connection

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// The tap's guarantees without a database: the ledger and the session sink are
// substituted, so each test can make one of them fail, panic, or refuse and
// observe that the admission result is byte-for-byte what the inner processor
// returned. The PostgreSQL proofs (capture_integration_test.go) repeat the
// important ones over a real MLLP listener.

var errInjected = errors.New("unit test: injected failure")

// fakeLedger is an in-memory captureLedger with the same conditional-update
// semantics as the PostgreSQL store.
type fakeLedger struct {
	mu       sync.Mutex
	now      time.Time
	captures map[string]*Capture
	fillErr  error
	armedErr error
	// blockArmed makes ArmedStreamCaptures wait for its context, like a hung
	// query.
	blockArmed bool
	// countErr fails the count after a successful write; failMarkErr then
	// fails recording that, leaving the row armed one slot behind.
	countErr    error
	failMarkErr error
	fills       int
	// advancedUnder records the version each counted fill read and advanced
	// the row from — the expected version of the update.
	advancedUnder []int64
}

func newFakeLedger(captures ...Capture) *fakeLedger {
	ledger := &fakeLedger{now: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC), captures: map[string]*Capture{}}
	for index := range captures {
		capture := captures[index]
		ledger.captures[capture.ID] = &capture
	}
	return ledger
}

func (l *fakeLedger) Now() time.Time {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.now
}

func (l *fakeLedger) advance(by time.Duration) {
	l.mu.Lock()
	l.now = l.now.Add(by)
	l.mu.Unlock()
}

func (l *fakeLedger) ArmedStreamCaptures(ctx context.Context, tenantID string, now time.Time, _ int) ([]Capture, error) {
	l.mu.Lock()
	blocked := l.blockArmed
	l.mu.Unlock()
	if blocked {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.armedErr != nil {
		return nil, l.armedErr
	}
	var armed []Capture
	for _, capture := range l.captures {
		if capture.TenantID == tenantID && capture.Status == CaptureStatusArmed && capture.Mode == CaptureModeStream &&
			now.Before(capture.ExpiresAt) && capture.Captured < capture.MaxMessages {
			armed = append(armed, *capture)
		}
	}
	return armed, nil
}

func (l *fakeLedger) ExpireCaptures(_ context.Context, tenantID string, now time.Time) (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.armedErr != nil {
		return 0, l.armedErr
	}
	var expired int64
	for _, capture := range l.captures {
		due := capture.Mode == CaptureModeStream && !now.Before(capture.ExpiresAt) ||
			capture.Mode == CaptureModePeek && !now.Add(-peekOrphanGrace).Before(capture.ExpiresAt)
		if capture.TenantID == tenantID && capture.Status == CaptureStatusArmed && due {
			capture.Status = CaptureStatusExpired
			capture.Version++
			expired++
		}
	}
	return expired, nil
}

func (l *fakeLedger) FillCaptureSlot(ctx context.Context, tenantID, captureID string, now time.Time,
	write func(context.Context, int) error) (captureFill, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fills++
	if l.fillErr != nil {
		return captureFill{}, l.fillErr
	}
	capture, ok := l.captures[captureID]
	if !ok || capture.TenantID != tenantID || capture.Status != CaptureStatusArmed || capture.Mode != CaptureModeStream ||
		capture.Captured >= capture.MaxMessages || !now.Before(capture.ExpiresAt) {
		return captureFill{outcome: fillSkipped}, nil
	}
	slot, version := capture.Captured+1, capture.Version
	fail := func(fill captureFill, captured int, code string) (captureFill, error) {
		if l.failMarkErr != nil {
			return fill, l.failMarkErr
		}
		capture.Status = CaptureStatusFailed
		capture.Captured = captured
		capture.Version++
		capture.Problems = []Problem{{Code: code}}
		fill.counted = fill.outcome == fillCountFailed
		fill.finished = true
		return fill, nil
	}
	if err := write(ctx, slot); err != nil {
		return fail(captureFill{slot: slot, outcome: fillWriteFailed, cause: err}, slot-1, CodeSampleWriteFailed)
	}
	if l.countErr != nil {
		return fail(captureFill{slot: slot, outcome: fillCountFailed, cause: l.countErr}, slot, CodeCaptureCountFailed)
	}
	l.advancedUnder = append(l.advancedUnder, version)
	capture.Captured = slot
	capture.Version++
	if slot >= capture.MaxMessages {
		capture.Status = CaptureStatusComplete
		return captureFill{slot: slot, outcome: fillCompleted, counted: true, finished: true}, nil
	}
	return captureFill{slot: slot, outcome: fillWritten, counted: true}, nil
}

func (l *fakeLedger) capture(id string) Capture {
	l.mu.Lock()
	defer l.mu.Unlock()
	return *l.captures[id]
}

// fakeSessions records every sample, and can fail or panic. Like the real
// stores it keeps one sample per caller-named ID: a repeated ID returns the
// first write.
type fakeSessions struct {
	mu       sync.Mutex
	samples  []session.AddSampleRequest
	status   session.SessionStatus
	addErr   error
	addPanic bool
}

func (f *fakeSessions) GetSession(_ context.Context, sessionID string) (*session.Session, error) {
	if sessionID == "sess-missing" {
		return nil, session.ErrNotFound
	}
	status := f.status
	if status == "" {
		status = session.SessionStatusActive
	}
	return &session.Session{ID: sessionID, Status: status}, nil
}

func (f *fakeSessions) AddSample(_ context.Context, sessionID string, request session.AddSampleRequest) (*session.Sample, error) {
	if f.addPanic {
		panic("unit test: the session store panicked")
	}
	if f.addErr != nil {
		return nil, f.addErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, existing := range f.samples {
		if request.ID != "" && existing.ID == request.ID {
			return &session.Sample{ID: existing.ID, SessionID: sessionID, Name: existing.Name, Source: existing.Source}, nil
		}
	}
	f.samples = append(f.samples, request)
	return &session.Sample{ID: request.ID, SessionID: sessionID, Name: request.Name, Source: request.Source}, nil
}

func (f *fakeSessions) added() []session.AddSampleRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]session.AddSampleRequest(nil), f.samples...)
}

// recordingObserver counts what the tap reports.
type recordingObserver struct {
	mu       sync.Mutex
	captured map[CaptureMode]int
	failures map[TapFailure]int
}

func newRecordingObserver() *recordingObserver {
	return &recordingObserver{captured: map[CaptureMode]int{}, failures: map[TapFailure]int{}}
}

func (o *recordingObserver) observer() CaptureObserver {
	return CaptureObserver{
		Captured: func(mode CaptureMode, messages int) {
			o.mu.Lock()
			o.captured[mode] += messages
			o.mu.Unlock()
		},
		TapFailed: func(reason TapFailure, _ error) {
			o.mu.Lock()
			o.failures[reason]++
			o.mu.Unlock()
		},
	}
}

func (o *recordingObserver) failuresOf(reason TapFailure) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.failures[reason]
}

// fixedProcessor returns one outcome and counts its calls.
type fixedProcessor struct {
	result integration.ProcessResult
	err    error
	calls  int
}

func (p *fixedProcessor) Process(context.Context, integration.ProcessRequest) (integration.ProcessResult, error) {
	p.calls++
	return p.result, p.err
}

func armedStream(id, sourceID string, maxMessages int, expiresAt time.Time) Capture {
	return Capture{
		TenantID: "tenant-a", ID: id, SessionID: "sess-1", Mode: CaptureModeStream, SourceID: sourceID,
		Status: CaptureStatusArmed, MaxMessages: maxMessages, Version: 1, ExpiresAt: expiresAt,
	}
}

func frame(t *testing.T, mode integration.ExecutionMode, sourceID string) integration.ProcessRequest {
	t.Helper()
	envelope, err := integration.NewRawEnvelope(integration.RawEnvelopeMetadata{
		TenantID: "tenant-a", SourceID: sourceID, Format: events.FormatHL7v2, ContentType: "application/hl7-v2+er7",
		ReceivedAt: time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC), Classification: integration.DataClassificationPHI,
	}, []byte("MSH|^~\\&|S|F|R|F|20260926||ADT^A01|c|P|2.5.1\rPID|1||MRN-SYNTH||Synthetic^Patient\r"))
	if err != nil {
		t.Fatal(err)
	}
	return integration.ProcessRequest{Mode: mode, Envelope: envelope, CorrelationID: "correlation-1"}
}

func acceptedFor(request integration.ProcessRequest) integration.ProcessResult {
	return integration.ProcessResult{
		Mode: request.Mode, TenantID: "tenant-a",
		Receipt: &integration.Receipt{ID: "receipt-1", Status: integration.ReceiptStatusAccepted, CorrelationID: request.CorrelationID},
	}
}

// boundTaps is a tap set bound over the fakes, with the registry refreshed.
func boundTaps(t *testing.T, ledger *fakeLedger, sessions *fakeSessions, observer *recordingObserver) *CaptureTaps {
	t.Helper()
	registry := newCaptureRegistry(ledger, "tenant-a", time.Hour, observer.observer())
	if err := registry.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	taps := NewCaptureTaps()
	if err := taps.Bind(CaptureTapBinding{Registry: registry, Sessions: sessions, Observer: observer.observer()}); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	return taps
}

// TestCaptureTap_ReturnsExactlyWhatTheInnerProcessorReturned drives every
// combination of an inner outcome with a tap that captures, fails, panics, or
// is unbound, and requires the decorated result and error to be the inner
// processor's, unchanged.
func TestCaptureTap_ReturnsExactlyWhatTheInnerProcessorReturned(t *testing.T) {
	production := frame(t, integration.ExecutionModeProduction, "adt-east")
	rejected := acceptedFor(production)
	rejected.Receipt = &integration.Receipt{ID: "receipt-2", Status: integration.ReceiptStatusRejected}
	outcomes := map[string]fixedProcessor{
		"accepted":         {result: acceptedFor(production)},
		"rejected receipt": {result: rejected},
		"admission error":  {err: errInjected},
	}
	taps := map[string]func(*testing.T) (*CaptureTaps, *fakeSessions){
		"unbound": func(*testing.T) (*CaptureTaps, *fakeSessions) { return NewCaptureTaps(), &fakeSessions{} },
		"capturing": func(t *testing.T) (*CaptureTaps, *fakeSessions) {
			sessions := &fakeSessions{}
			ledger := newFakeLedger(armedStream("cap-1", "adt-east", 5, time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC)))
			return boundTaps(t, ledger, sessions, newRecordingObserver()), sessions
		},
		"session store failing": func(t *testing.T) (*CaptureTaps, *fakeSessions) {
			sessions := &fakeSessions{addErr: errInjected}
			ledger := newFakeLedger(armedStream("cap-1", "adt-east", 5, time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC)))
			return boundTaps(t, ledger, sessions, newRecordingObserver()), sessions
		},
		"session store panicking": func(t *testing.T) (*CaptureTaps, *fakeSessions) {
			sessions := &fakeSessions{addPanic: true}
			ledger := newFakeLedger(armedStream("cap-1", "adt-east", 5, time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC)))
			return boundTaps(t, ledger, sessions, newRecordingObserver()), sessions
		},
		"ledger failing": func(t *testing.T) (*CaptureTaps, *fakeSessions) {
			ledger := newFakeLedger(armedStream("cap-1", "adt-east", 5, time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC)))
			ledger.fillErr = errInjected
			return boundTaps(t, ledger, &fakeSessions{}, newRecordingObserver()), &fakeSessions{}
		},
	}
	for outcomeName, outcome := range outcomes {
		for tapName, build := range taps {
			t.Run(outcomeName+"/"+tapName, func(t *testing.T) {
				inner := outcome
				tapSet, _ := build(t)
				result, err := tapSet.Wrap(&inner).Process(context.Background(), production)
				if inner.calls != 1 {
					t.Fatalf("the inner processor ran %d times", inner.calls)
				}
				if err != inner.err || !reflect.DeepEqual(result, inner.result) {
					t.Fatalf("decorated outcome = (%+v, %v), want the inner (%+v, %v)", result, err, inner.result, inner.err)
				}
			})
		}
	}
}

func TestCaptureTap_CapturesOnlyDurablyAcceptedProductionFramesOfTheArmedSource(t *testing.T) {
	ledger := newFakeLedger(armedStream("cap-1", "adt-east", 5, time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC)))
	sessions := &fakeSessions{}
	taps := boundTaps(t, ledger, sessions, newRecordingObserver())

	preview := frame(t, integration.ExecutionModePreview, "adt-east")
	other := frame(t, integration.ExecutionModeProduction, "adt-west")
	armed := frame(t, integration.ExecutionModeProduction, "adt-east")
	rejected := acceptedFor(armed)
	rejected.Receipt.Status = integration.ReceiptStatusRejected
	noReceipt := acceptedFor(armed)
	noReceipt.Receipt = nil
	for name, call := range map[string]struct {
		request integration.ProcessRequest
		inner   fixedProcessor
	}{
		"preview mode":      {preview, fixedProcessor{result: acceptedFor(preview)}},
		"another source":    {other, fixedProcessor{result: acceptedFor(other)}},
		"admission error":   {armed, fixedProcessor{err: errInjected}},
		"rejected receipt":  {armed, fixedProcessor{result: rejected}},
		"no receipt at all": {armed, fixedProcessor{result: noReceipt}},
	} {
		inner := call.inner
		result, err := taps.Wrap(&inner).Process(context.Background(), call.request)
		if err != inner.err || !reflect.DeepEqual(result, inner.result) {
			t.Fatalf("%s: the tap changed the outcome", name)
		}
		if len(sessions.added()) != 0 || ledger.fills != 0 {
			t.Fatalf("%s: captured %d samples with %d fills", name, len(sessions.added()), ledger.fills)
		}
	}

	accepted := fixedProcessor{result: acceptedFor(armed)}
	if _, err := taps.Wrap(&accepted).Process(context.Background(), armed); err != nil {
		t.Fatal(err)
	}
	added := sessions.added()
	if len(added) != 1 {
		t.Fatalf("an accepted production frame of the armed source captured %d samples", len(added))
	}
	want := session.AddSampleRequest{
		ID: "sample_capture_cap-1_1", Name: "capture cap-1 #1", Format: events.FormatHL7v2, Source: "capture:cap-1",
		Raw: string(armed.Envelope.Bytes()), PHIPolicy: session.PHIPolicyRedact, Redaction: session.SampleRedactionCapture,
	}
	if !reflect.DeepEqual(added[0], want) {
		t.Fatalf("sample request = %+v, want %+v", added[0], want)
	}
}

func TestCaptureTap_CompletesAtMaxMessagesUnderTheExpectedVersion(t *testing.T) {
	ledger := newFakeLedger(armedStream("cap-1", "adt-east", 2, time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC)))
	sessions := &fakeSessions{}
	observer := newRecordingObserver()
	taps := boundTaps(t, ledger, sessions, observer)
	request := frame(t, integration.ExecutionModeProduction, "adt-east")
	for range 3 {
		inner := fixedProcessor{result: acceptedFor(request)}
		if _, err := taps.Wrap(&inner).Process(context.Background(), request); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(sessions.added()); got != 2 {
		t.Fatalf("captured %d samples, want maxMessages 2", got)
	}
	if ledger.fills != 2 {
		t.Fatalf("the third frame reached the ledger (%d fills): a completed capture must leave the cache at once", ledger.fills)
	}
	final := ledger.capture("cap-1")
	if final.Status != CaptureStatusComplete || final.Captured != 2 {
		t.Fatalf("capture = %+v, want complete with 2", final)
	}
	// Each count is an expected-version update from the version it read: 1 at
	// arm, then 2; the second completes the capture in the same update.
	if !reflect.DeepEqual(ledger.advancedUnder, []int64{1, 2}) {
		t.Fatalf("advanced under versions %v, want [1 2]", ledger.advancedUnder)
	}
	if observer.captured[CaptureModeStream] != 2 {
		t.Fatalf("captured metric = %d, want 2", observer.captured[CaptureModeStream])
	}
}

func TestCaptureTap_SessionFailureFailsTheCaptureAndIsCounted(t *testing.T) {
	ledger := newFakeLedger(armedStream("cap-1", "adt-east", 5, time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC)))
	sessions := &fakeSessions{addErr: errInjected}
	observer := newRecordingObserver()
	taps := boundTaps(t, ledger, sessions, observer)
	request := frame(t, integration.ExecutionModeProduction, "adt-east")
	for range 2 {
		inner := fixedProcessor{result: acceptedFor(request)}
		if _, err := taps.Wrap(&inner).Process(context.Background(), request); err != nil {
			t.Fatal(err)
		}
	}
	if observer.failuresOf(TapFailureSessionStore) != 1 {
		t.Fatalf("session_store failures = %d, want 1 (the capture stops after the first)", observer.failuresOf(TapFailureSessionStore))
	}
	final := ledger.capture("cap-1")
	if final.Status != CaptureStatusFailed || final.Captured != 0 || len(final.Problems) != 1 || final.Problems[0].Code != CodeSampleWriteFailed {
		t.Fatalf("capture = %+v, want failed with SAMPLE_WRITE_FAILED and nothing counted", final)
	}
	if ledger.fills != 1 {
		t.Fatalf("fills = %d: a failed capture must leave the cache at once", ledger.fills)
	}
}

// TestCaptureTap_CountFailureFailsTheCaptureWithTheSampleCounted: a sample
// written whose count does not commit finishes the capture failed with
// CAPTURE_COUNT_FAILED and the sample counted, so the row agrees with the
// session; the failure is a ledger failure, and the capture leaves the cache.
func TestCaptureTap_CountFailureFailsTheCaptureWithTheSampleCounted(t *testing.T) {
	ledger := newFakeLedger(armedStream("cap-1", "adt-east", 5, time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC)))
	ledger.countErr = errInjected
	sessions := &fakeSessions{}
	observer := newRecordingObserver()
	taps := boundTaps(t, ledger, sessions, observer)
	request := frame(t, integration.ExecutionModeProduction, "adt-east")
	for range 2 {
		inner := fixedProcessor{result: acceptedFor(request)}
		result, err := taps.Wrap(&inner).Process(context.Background(), request)
		if err != nil || !reflect.DeepEqual(result, inner.result) {
			t.Fatalf("a failing count changed the outcome: (%+v, %v)", result, err)
		}
	}
	final := ledger.capture("cap-1")
	if final.Status != CaptureStatusFailed || final.Captured != 1 || len(final.Problems) != 1 || final.Problems[0].Code != CodeCaptureCountFailed {
		t.Fatalf("capture = %+v, want failed with CAPTURE_COUNT_FAILED and the written sample counted", final)
	}
	if got := len(sessions.added()); got != 1 || observer.failuresOf(TapFailureLedger) != 1 || observer.captured[CaptureModeStream] != 1 {
		t.Fatalf("samples %d, ledger failures %d, captured metric %d; want 1, 1, 1",
			got, observer.failuresOf(TapFailureLedger), observer.captured[CaptureModeStream])
	}
	if ledger.fills != 1 {
		t.Fatalf("fills = %d: a failed capture must leave the cache at once", ledger.fills)
	}
}

// TestCaptureTap_ARetriedSlotIsOneSample: when a count fails and recording
// that fails too, the row stays armed one slot behind while the session
// already holds that slot's sample. The next frame is handed the same slot,
// and its write — under the same derived ID — is the first sample again, not a
// second, so the session never holds more than the row can count.
func TestCaptureTap_ARetriedSlotIsOneSample(t *testing.T) {
	ledger := newFakeLedger(armedStream("cap-1", "adt-east", 2, time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC)))
	ledger.countErr, ledger.failMarkErr = errInjected, errInjected
	sessions := &fakeSessions{}
	observer := newRecordingObserver()
	taps := boundTaps(t, ledger, sessions, observer)
	send := func(controlID string) {
		t.Helper()
		request := frame(t, integration.ExecutionModeProduction, "adt-east")
		request.CorrelationID = controlID
		inner := fixedProcessor{result: acceptedFor(request)}
		if _, err := taps.Wrap(&inner).Process(context.Background(), request); err != nil {
			t.Fatal(err)
		}
	}
	send("first")
	if row := ledger.capture("cap-1"); row.Status != CaptureStatusArmed || row.Captured != 0 {
		t.Fatalf("after an unrecorded count failure the row is %+v, want armed one behind", row)
	}
	// The ledger (count, then the failed record) failed twice; the capture
	// stays in the cache because the row may still be armed.
	if observer.failuresOf(TapFailureLedger) != 2 || observer.captured[CaptureModeStream] != 0 {
		t.Fatalf("ledger failures %d, captured metric %d; want 2, 0",
			observer.failuresOf(TapFailureLedger), observer.captured[CaptureModeStream])
	}
	ledger.mu.Lock()
	ledger.countErr, ledger.failMarkErr = nil, nil
	ledger.mu.Unlock()
	send("second")
	send("third")
	added := sessions.added()
	if len(added) != 2 || added[0].ID != "sample_capture_cap-1_1" || added[1].ID != "sample_capture_cap-1_2" {
		t.Fatalf("session samples = %+v, want slots 1 and 2 once each", added)
	}
	if row := ledger.capture("cap-1"); row.Status != CaptureStatusComplete || row.Captured != 2 {
		t.Fatalf("capture = %+v, want complete with 2", row)
	}
	if observer.captured[CaptureModeStream] != 2 {
		t.Fatalf("captured metric = %d, want each slot counted once", observer.captured[CaptureModeStream])
	}
}

func TestCaptureTap_ContainsAPanicAndCountsIt(t *testing.T) {
	ledger := newFakeLedger(armedStream("cap-1", "adt-east", 5, time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC)))
	observer := newRecordingObserver()
	taps := boundTaps(t, ledger, &fakeSessions{addPanic: true}, observer)
	request := frame(t, integration.ExecutionModeProduction, "adt-east")
	inner := fixedProcessor{result: acceptedFor(request)}
	result, err := taps.Wrap(&inner).Process(context.Background(), request)
	if err != nil || !reflect.DeepEqual(result, inner.result) {
		t.Fatalf("a panicking tap changed the outcome: (%+v, %v)", result, err)
	}
	if observer.failuresOf(TapFailurePanic) != 1 {
		t.Fatalf("panic failures = %d, want 1", observer.failuresOf(TapFailurePanic))
	}
}

func TestCaptureRegistry_TTLIsEnforcedFromMemoryAndOnRefresh(t *testing.T) {
	ledger := newFakeLedger(armedStream("cap-1", "adt-east", 5, time.Date(2026, 9, 26, 12, 0, 1, 0, time.UTC)))
	sessions := &fakeSessions{}
	taps := boundTaps(t, ledger, sessions, newRecordingObserver())
	ledger.advance(2 * time.Second)

	// Before any refresh: the cache still holds the capture, and the tap is
	// inert anyway, because the TTL is checked from memory.
	request := frame(t, integration.ExecutionModeProduction, "adt-east")
	inner := fixedProcessor{result: acceptedFor(request)}
	if _, err := taps.Wrap(&inner).Process(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(sessions.added()) != 0 || ledger.fills != 0 {
		t.Fatal("an expired capture captured before the refresh")
	}
	registry := taps.binding.Load().Registry
	if err := registry.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := ledger.capture("cap-1").Status; got != CaptureStatusExpired {
		t.Fatalf("status after refresh = %s, want expired", got)
	}
	if _, ok := registry.armedFor("tenant-a", "adt-east", ledger.Now()); ok {
		t.Fatal("the cache still holds an expired capture after a refresh")
	}
}

func TestCaptureRegistry_RunReportsRefreshFailuresAndStops(t *testing.T) {
	ledger := newFakeLedger()
	ledger.armedErr = errInjected
	observer := newRecordingObserver()
	registry := newCaptureRegistry(ledger, "tenant-a", time.Millisecond, observer.observer())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- registry.Run(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for observer.failuresOf(TapFailureRefresh) < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run returned %v; a refresh failure must never stop the process", err)
	}
	if observer.failuresOf(TapFailureRefresh) < 2 {
		t.Fatal("refresh failures were not reported")
	}
}

// TestCaptureRegistry_RunBoundsEachRefresh: a refresh whose query hangs is cut
// off at the interval and reported, and the next one still runs, rather than
// one hung query freezing the cache for the life of the process.
func TestCaptureRegistry_RunBoundsEachRefresh(t *testing.T) {
	ledger := newFakeLedger()
	ledger.blockArmed = true
	observer := newRecordingObserver()
	registry := newCaptureRegistry(ledger, "tenant-a", 5*time.Millisecond, observer.observer())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- registry.Run(ctx) }()
	deadline := time.Now().Add(5 * time.Second)
	for observer.failuresOf(TapFailureRefresh) < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if observer.failuresOf(TapFailureRefresh) < 2 {
		t.Fatal("a hung refresh was never cut off: Run is stuck in its first refresh")
	}
}

// TestCaptureRegistry_PeekRowsExpireOnlyWhenOrphaned: a refresh expires a
// stream capture at its TTL, but leaves a peek's row alone until
// peekOrphanGrace past it, because the peek itself finishes that row.
func TestCaptureRegistry_PeekRowsExpireOnlyWhenOrphaned(t *testing.T) {
	expiresAt := time.Date(2026, 9, 26, 12, 0, 1, 0, time.UTC)
	peek := Capture{
		TenantID: "tenant-a", ID: "peek-1", SessionID: "sess-1", Mode: CaptureModePeek,
		Status: CaptureStatusArmed, MaxMessages: 5, Version: 1, ExpiresAt: expiresAt,
	}
	ledger := newFakeLedger(armedStream("cap-1", "adt-east", 5, expiresAt), peek)
	registry := newCaptureRegistry(ledger, "tenant-a", time.Hour, CaptureObserver{})
	ledger.advance(2 * time.Second)
	if err := registry.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stream, row := ledger.capture("cap-1"), ledger.capture("peek-1"); stream.Status != CaptureStatusExpired || row.Status != CaptureStatusArmed {
		t.Fatalf("past the TTL: stream %s, peek %s; want expired and still armed", stream.Status, row.Status)
	}
	ledger.advance(peekOrphanGrace)
	if err := registry.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if row := ledger.capture("peek-1"); row.Status != CaptureStatusExpired {
		t.Fatalf("an orphaned peek row is %s past its grace, want expired", row.Status)
	}
}

// TestStartCapture_OnlyASourceTheTapCanSee: a source an enabled MLLP or HTTP
// adapter of this replica admits is armed without asking the catalog; any
// other source — a disabled adapter's, a batch runner's, an unknown one — must
// be named by a compiled MLLP or HTTP source connection, so the catalog is
// asked. The store behind this service fails every query, which is how the
// two paths tell apart; the PostgreSQL proof covers the refusal itself.
func TestStartCapture_OnlyASourceTheTapCanSee(t *testing.T) {
	db := sql.OpenDB(unreachableConnector{})
	t.Cleanup(func() { _ = db.Close() })
	store, err := NewPostgresStore(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	runtime := describedRuntime()
	runtime.Adapters[0].SourceID = "adt-west"
	runtime.Adapters[1].SourceID = "adt-east"
	runtime.Adapters[2].Enabled, runtime.Adapters[2].SourceID = true, "batch-east"
	service, err := NewService(store, nil, runtime, "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.EnableSampleIntake(IntakeConfig{Sessions: &fakeSessions{}}); err != nil {
		t.Fatal(err)
	}
	disabled := describedRuntime()
	disabled.Adapters[1].Enabled, disabled.Adapters[1].SourceID = false, "adt-east"
	for sourceID, wantCatalog := range map[string]bool{"adt-east": false, "adt-west": false, "batch-east": true, "adt-nowhere": true} {
		_, err := service.StartCapture(callerContext("tenant-a", ReadRole), StartCaptureRequest{
			SourceID: sourceID, SessionID: "sess-1", Reason: "shape the profile",
		})
		askedCatalog := err != nil && strings.Contains(err.Error(), "look up compiled stream sources")
		armed := err != nil && strings.Contains(err.Error(), "begin capture insert")
		if askedCatalog != wantCatalog || armed == wantCatalog {
			t.Errorf("%s: %v; want catalog lookup %v", sourceID, err, wantCatalog)
		}
	}
	disabledService, err := NewService(store, nil, disabled, "tenant-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := disabledService.EnableSampleIntake(IntakeConfig{Sessions: &fakeSessions{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := disabledService.StartCapture(callerContext("tenant-a", ReadRole), StartCaptureRequest{
		SourceID: "adt-east", SessionID: "sess-1", Reason: "shape the profile",
	}); err == nil || !strings.Contains(err.Error(), "look up compiled stream sources") {
		t.Fatalf("a disabled adapter's source skipped the catalog: %v", err)
	}
}

func TestCaptureRegistry_OtherTenantIsNeverArmed(t *testing.T) {
	ledger := newFakeLedger(armedStream("cap-1", "adt-east", 5, time.Date(2026, 9, 26, 13, 0, 0, 0, time.UTC)))
	registry := newCaptureRegistry(ledger, "tenant-a", time.Hour, CaptureObserver{})
	if err := registry.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, ok := registry.armedFor("tenant-b", "adt-east", ledger.Now()); ok {
		t.Fatal("a frame of another tenant matched this tenant's capture")
	}
}

// TestSampleIntakeRefusesBeforeTheStore: every refusal below happens at the
// authorization, configuration, or request boundary — the store behind this
// service fails any query, so reaching it would change the error.
func TestSampleIntakeRefusesBeforeTheStore(t *testing.T) {
	service := unitService(t)
	operator := callerContext("tenant-a", ReadRole)
	peek := func(ctx context.Context, request PeekRequest) error {
		_, err := service.PeekBatch(ctx, request)
		return err
	}
	start := func(ctx context.Context, request StartCaptureRequest) error {
		_, err := service.StartCapture(ctx, request)
		return err
	}
	validPeek := PeekRequest{ConnectionID: "adt-drop", SessionID: "sess-1", Reason: "shape the profile"}
	validStart := StartCaptureRequest{SourceID: "adt-east", SessionID: "sess-1", Reason: "shape the profile"}

	if err := start(operator, validStart); !errors.Is(err, ErrSessionsUnavailable) {
		t.Fatalf("start without sample intake = %v, want ErrSessionsUnavailable", err)
	}
	if _, err := service.ListCaptures(operator, "sess-1"); !errors.Is(err, ErrSessionsUnavailable) {
		t.Fatalf("list without sample intake = %v", err)
	}
	sessions := &fakeSessions{}
	if err := service.EnableSampleIntake(IntakeConfig{Sessions: sessions}); err != nil {
		t.Fatal(err)
	}
	if err := service.EnableSampleIntake(IntakeConfig{}); !errors.Is(err, ErrSessionsUnavailable) {
		t.Fatalf("enable without sessions = %v", err)
	}

	withPeek := func(mutate func(*PeekRequest)) PeekRequest {
		request := validPeek
		mutate(&request)
		return request
	}
	withStart := func(mutate func(*StartCaptureRequest)) StartCaptureRequest {
		request := validStart
		mutate(&request)
		return request
	}
	cases := []struct {
		name string
		call func() error
		want error
	}{
		{"peek unauthenticated", func() error { return peek(context.Background(), validPeek) }, ErrUnauthenticated},
		{"peek without the read role", func() error { return peek(callerContext("tenant-a"), validPeek) }, ErrForbidden},
		{"peek from another tenant", func() error { return peek(callerContext("tenant-b", ReadRole), validPeek) }, ErrForbidden},
		{"peek without a reason", func() error { return peek(operator, withPeek(func(r *PeekRequest) { r.Reason = " " })) }, ErrInvalidRequest},
		{"peek reason too long", func() error {
			return peek(operator, withPeek(func(r *PeekRequest) { r.Reason = strings.Repeat("r", MaxReasonBytes+1) }))
		}, ErrInvalidRequest},
		{"peek 51 objects", func() error { return peek(operator, withPeek(func(r *PeekRequest) { r.MaxObjects = 51 })) }, ErrInvalidRequest},
		{"peek 51 messages", func() error { return peek(operator, withPeek(func(r *PeekRequest) { r.MaxMessages = 51 })) }, ErrInvalidRequest},
		{"peek negative messages", func() error { return peek(operator, withPeek(func(r *PeekRequest) { r.MaxMessages = -1 })) }, ErrInvalidRequest},
		{"peek object path with a control character", func() error {
			return peek(operator, withPeek(func(r *PeekRequest) { r.ObjectPath = "incoming/a\x00.hl7" }))
		}, ErrInvalidRequest},
		{"peek a malformed connection ID", func() error {
			return peek(operator, withPeek(func(r *PeekRequest) { r.ConnectionID = "../etc" }))
		}, ErrInvalidRequest},
		{"peek into a missing session", func() error {
			return peek(operator, withPeek(func(r *PeekRequest) { r.SessionID = "sess-missing" }))
		}, ErrSessionNotFound},
		{"start without the read role", func() error { return start(callerContext("tenant-a", WriteRole), validStart) }, ErrForbidden},
		{"start 101 messages", func() error {
			return start(operator, withStart(func(r *StartCaptureRequest) { r.MaxMessages = MaxCaptureMessages + 1 }))
		}, ErrInvalidRequest},
		{"start a 901 second TTL", func() error {
			return start(operator, withStart(func(r *StartCaptureRequest) { r.TTLSeconds = MaxCaptureTTLSeconds + 1 }))
		}, ErrInvalidRequest},
		{"start with a blank source", func() error { return start(operator, withStart(func(r *StartCaptureRequest) { r.SourceID = "" })) }, ErrInvalidRequest},
		{"start into a missing session", func() error {
			return start(operator, withStart(func(r *StartCaptureRequest) { r.SessionID = "sess-missing" }))
		}, ErrSessionNotFound},
		{"cancel without a reason", func() error { _, err := service.CancelCapture(operator, "cap-1", ""); return err }, ErrInvalidRequest},
		{"cancel a malformed ID", func() error { _, err := service.CancelCapture(operator, "a b", "stop"); return err }, ErrCaptureNotFound},
		{"list a malformed session", func() error { _, err := service.ListCaptures(operator, " "); return err }, ErrInvalidRequest},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			if err := test.call(); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}

	sessions.status = session.SessionStatusArchived
	if err := start(operator, validStart); !errors.Is(err, ErrSessionArchived) {
		t.Fatalf("start into an archived session = %v, want ErrSessionArchived", err)
	}
	if err := peek(operator, validPeek); !errors.Is(err, ErrSessionArchived) {
		t.Fatalf("peek into an archived session = %v, want ErrSessionArchived", err)
	}
	if len(sessions.added()) != 0 {
		t.Fatal("a refused request wrote a sample")
	}
}
