import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { GraphQLResponseError } from '$lib/graphql/client';
import {
  MAX_CONSECUTIVE_POLL_FAILURES,
  POLL_INTERVAL_MS,
  createIntakeController,
  type IntakeController
} from './intakeController';
import type { ConnectionCaptureRow } from './intakeApi';
import { REDACTED_TEXT, capture, sessionSample } from './intakeFixtures';

// The page's session and the API boundary, as the controller sees them.
let sessionId: string | null;
const api = {
  fetchConnectionCaptures: vi.fn(),
  fetchIntakeSessionSamples: vi.fn(),
  peekBatchConnection: vi.fn(),
  startConnectionCapture: vi.fn(),
  cancelConnectionCapture: vi.fn()
};
const onSamples = vi.fn();
const ensureSession = vi.fn(async () => {
  sessionId ??= 'session-1';
  return sessionId;
});

let controller: IntakeController;

function make(): IntakeController {
  controller = createIntakeController({
    currentSession: () => sessionId,
    ensureSession,
    onSamples,
    api
  });
  return controller;
}

/** Queues successive connectionCaptures answers. */
function capturesSequence(...answers: ConnectionCaptureRow[][]): void {
  for (const answer of answers) api.fetchConnectionCaptures.mockResolvedValueOnce(answer);
}

async function tickPoll(): Promise<void> {
  await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS);
}

beforeEach(() => {
  vi.useFakeTimers();
  sessionId = null;
  for (const fn of Object.values(api)) fn.mockReset();
  onSamples.mockReset();
  ensureSession.mockClear();
  api.fetchIntakeSessionSamples.mockResolvedValue([]);
});

afterEach(() => {
  controller?.dispose();
  vi.useRealTimers();
});

describe('intake controller polling', () => {
  it('arms into a session it creates, polls every 2.5 s while armed, pulls samples on a count change, and stops when none is armed', async () => {
    make();
    api.startConnectionCapture.mockResolvedValue(capture());
    capturesSequence(
      [capture()],
      [capture({ captured: 1 })],
      [capture({ captured: 2, status: 'COMPLETE', maxMessages: 2 })]
    );
    api.fetchIntakeSessionSamples.mockResolvedValue([sessionSample()]);

    await controller.startCapture({ sourceId: 'adt-east', maxMessages: 2, ttlSeconds: 60, reason: 'feed check' });
    expect(ensureSession).toHaveBeenCalledTimes(1);
    expect(api.startConnectionCapture).toHaveBeenCalledWith({
      sourceId: 'adt-east',
      sessionId: 'session-1',
      maxMessages: 2,
      ttlSeconds: 60,
      reason: 'feed check'
    });
    expect(get(controller.state)).toMatchObject({ sessionId: 'session-1', polling: true });
    expect(api.fetchConnectionCaptures).not.toHaveBeenCalled();

    await tickPoll();
    expect(api.fetchConnectionCaptures).toHaveBeenCalledTimes(1);
    expect(api.fetchConnectionCaptures).toHaveBeenLastCalledWith('session-1');

    await tickPoll();
    expect(api.fetchConnectionCaptures).toHaveBeenCalledTimes(2);
    expect(onSamples).toHaveBeenLastCalledWith(
      [expect.objectContaining({ sampleId: 'sample_capture_cap-1_1', raw: REDACTED_TEXT, provenance: 'capture:cap-1' })],
      { activate: false }
    );

    await tickPoll();
    expect(api.fetchConnectionCaptures).toHaveBeenCalledTimes(3);
    expect(get(controller.state)).toMatchObject({ polling: false });
    const samplesCalls = api.fetchIntakeSessionSamples.mock.calls.length;

    // Nothing is armed: no further poll, however long the page stays open.
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 10);
    expect(api.fetchConnectionCaptures).toHaveBeenCalledTimes(3);
    expect(api.fetchIntakeSessionSamples).toHaveBeenCalledTimes(samplesCalls);
  });

  it('does not refetch samples while the counts stay the same', async () => {
    sessionId = 'session-1';
    make();
    capturesSequence([capture()], [capture()], [capture()]);
    await controller.refresh();
    await tickPoll();
    await tickPoll();
    expect(api.fetchConnectionCaptures).toHaveBeenCalledTimes(3);
    expect(api.fetchIntakeSessionSamples).toHaveBeenCalledTimes(1);
  });

  it('a refresh with nothing armed reads once and does not poll', async () => {
    sessionId = 'session-1';
    make();
    capturesSequence([capture({ status: 'EXPIRED' })]);
    await controller.refresh();
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 4);
    expect(api.fetchConnectionCaptures).toHaveBeenCalledTimes(1);
    expect(get(controller.state).polling).toBe(false);
  });

  it('a refresh without a session queries nothing', async () => {
    make();
    await controller.refresh();
    expect(api.fetchConnectionCaptures).not.toHaveBeenCalled();
    expect(ensureSession).not.toHaveBeenCalled();
  });

  it('stops after three consecutive failed polls and keeps the failure on screen; Retry resumes', async () => {
    sessionId = 'session-1';
    make();
    capturesSequence([capture()]);
    await controller.refresh();
    api.fetchConnectionCaptures.mockRejectedValue(new GraphQLResponseError('connection sample intake request failed', []));

    for (let attempt = 0; attempt < MAX_CONSECUTIVE_POLL_FAILURES; attempt += 1) await tickPoll();
    expect(api.fetchConnectionCaptures).toHaveBeenCalledTimes(1 + MAX_CONSECUTIVE_POLL_FAILURES);
    expect(get(controller.state)).toMatchObject({ polling: false });
    expect(get(controller.state).pollError).toMatch(/could not complete/);

    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 4);
    expect(api.fetchConnectionCaptures).toHaveBeenCalledTimes(1 + MAX_CONSECUTIVE_POLL_FAILURES);

    api.fetchConnectionCaptures.mockResolvedValueOnce([capture({ status: 'CANCELLED' })]);
    await controller.refresh();
    expect(get(controller.state)).toMatchObject({ pollError: null, polling: false });
  });

  it('cancel records the final state with one more read', async () => {
    sessionId = 'session-1';
    make();
    capturesSequence([capture()]);
    await controller.refresh();
    api.cancelConnectionCapture.mockResolvedValue(capture({ status: 'CANCELLED' }));
    capturesSequence([capture({ status: 'CANCELLED' })]);

    await controller.cancelCapture('cap-1', 'wrong source');
    expect(api.cancelConnectionCapture).toHaveBeenCalledWith('cap-1', 'wrong source');
    expect(get(controller.state)).toMatchObject({ polling: false });
    expect(get(controller.state).captures[0]).toMatchObject({ id: 'cap-1', status: 'CANCELLED' });
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 4);
    expect(api.fetchConnectionCaptures).toHaveBeenCalledTimes(2);
  });

  it('dispose stops polling', async () => {
    sessionId = 'session-1';
    make();
    capturesSequence([capture()]);
    await controller.refresh();
    controller.dispose();
    await vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 4);
    expect(api.fetchConnectionCaptures).toHaveBeenCalledTimes(1);
  });
});

describe('intake controller peek', () => {
  it('lists without objectPath and adds nothing; reading adds the samples and activates the first', async () => {
    make();
    const peekRow = capture({ id: 'peek-1', mode: 'PEEK', status: 'COMPLETE', sourceId: 'adt-batch-src' });
    api.peekBatchConnection.mockResolvedValueOnce({
      objects: [{ path: 'adt/one.hl7', size: 120, version: 'v1', modifiedAt: null }],
      samples: [],
      capture: peekRow,
      problems: []
    });
    const listed = await controller.peek({ connectionId: 'adt-batch', maxObjects: 10, reason: 'browse' });
    expect(api.peekBatchConnection).toHaveBeenLastCalledWith({
      connectionId: 'adt-batch',
      sessionId: 'session-1',
      objectPath: null,
      maxObjects: 10,
      maxMessages: null,
      reason: 'browse'
    });
    expect(listed.objects).toHaveLength(1);
    expect(onSamples).not.toHaveBeenCalled();

    api.peekBatchConnection.mockResolvedValueOnce({
      objects: [],
      samples: [sessionSample({ id: 'peeked-1', source: 'peek:peek-2' })],
      capture: { ...peekRow, id: 'peek-2' },
      problems: []
    });
    await controller.peek({ connectionId: 'adt-batch', objectPath: 'adt/one.hl7', maxMessages: 3, reason: 'browse' });
    expect(onSamples).toHaveBeenCalledWith(
      [expect.objectContaining({ sampleId: 'peeked-1', sourceId: 'adt-batch-src', provenance: 'peek:peek-2' })],
      { activate: true }
    );
    // A peek is finished when it returns: nothing to poll.
    expect(get(controller.state).polling).toBe(false);
  });
});
