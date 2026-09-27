/**
 * The /hl7 page's sample intake: the session it captures into, the session's
 * captures, and the polling that brings captured samples into the inbox.
 *
 * There is no subscription (C-2 handoff §3). While any capture of the session
 * is armed the controller polls `connectionCaptures(sessionId)` every 2.5 s —
 * each replica refreshes its armed-capture cache every 2 s — and refetches the
 * session's samples whenever a count changes or a capture finishes. It stops
 * as soon as none is armed, and after three consecutive failed polls (the
 * failure stays on screen with Retry). Timers chain, so polls never overlap.
 *
 * It lives on the page, not in the Samples tab: switching tabs must not stop a
 * capture from arriving. Samples reach the tab-memory inbox only; nothing here
 * touches browser storage.
 */
import { writable, type Readable } from 'svelte/store';
import * as defaultApi from './intakeApi';
import type { BatchPeekResultView, ConnectionCaptureRow } from './intakeApi';
import {
  anyArmed,
  captureSampleSignature,
  intakeSamplesFrom,
  type SessionIntakeSample
} from './intakeState';
import { describeIntakeFailure } from './intakeErrors';

export const POLL_INTERVAL_MS = 2_500;
export const MAX_CONSECUTIVE_POLL_FAILURES = 3;

export interface IntakeControllerState {
  /** The session intake writes into; null until Preview or intake creates one. */
  sessionId: string | null;
  /** The session's peeks and captures, newest first. */
  captures: ConnectionCaptureRow[];
  /** A poll is scheduled or running. */
  polling: boolean;
  /** The last poll's failure, rendered on the capture rows. */
  pollError: string | null;
}

/** The state before the controller has reported anything. */
export const EMPTY_INTAKE: IntakeControllerState = Object.freeze({
  sessionId: null,
  captures: [],
  polling: false,
  pollError: null
}) as IntakeControllerState;

type IntakeApi = Pick<
  typeof defaultApi,
  | 'fetchConnectionCaptures'
  | 'fetchIntakeSessionSamples'
  | 'peekBatchConnection'
  | 'startConnectionCapture'
  | 'cancelConnectionCapture'
>;

export interface IntakeControllerDeps {
  /** The page's current session, or null. */
  currentSession: () => string | null;
  /** Returns the page's session, creating one (and recording it on the page) when there is none. */
  ensureSession: () => Promise<string>;
  /** Adds session samples the inbox does not hold yet. */
  onSamples: (samples: SessionIntakeSample[], options: { activate: boolean }) => void;
  api?: Partial<IntakeApi> | undefined;
  intervalMs?: number | undefined;
  setTimer?: ((callback: () => void, ms: number) => unknown) | undefined;
  clearTimer?: ((handle: unknown) => void) | undefined;
}

export interface StartCaptureRequest {
  sourceId: string;
  maxMessages: number;
  ttlSeconds: number;
  reason: string;
}

export interface PeekRequest {
  connectionId: string;
  objectPath?: string | undefined;
  maxObjects?: number | undefined;
  maxMessages?: number | undefined;
  reason: string;
}

export interface IntakeController {
  state: Readable<IntakeControllerState>;
  /** Reads the session's captures now (and its samples when they changed); polls on while one is armed. */
  refresh: () => Promise<void>;
  startCapture: (request: StartCaptureRequest) => Promise<ConnectionCaptureRow>;
  cancelCapture: (id: string, reason: string) => Promise<ConnectionCaptureRow>;
  /** Lists (no objectPath) or reads one object; read samples go to the inbox and the first becomes active. */
  peek: (request: PeekRequest) => Promise<BatchPeekResultView>;
  dispose: () => void;
}

export function createIntakeController(deps: IntakeControllerDeps): IntakeController {
  const api: IntakeApi = { ...defaultApi, ...(deps.api ?? {}) };
  const intervalMs = deps.intervalMs ?? POLL_INTERVAL_MS;
  const setTimer = deps.setTimer ?? ((callback: () => void, ms: number) => setTimeout(callback, ms));
  const clearTimer = deps.clearTimer ?? ((handle: unknown) => clearTimeout(handle as ReturnType<typeof setTimeout>));

  let current: IntakeControllerState = {
    sessionId: deps.currentSession(),
    captures: [],
    polling: false,
    pollError: null
  };
  const store = writable<IntakeControllerState>(current);
  let timer: unknown = null;
  let failures = 0;
  let sampleSignature = '';
  let disposed = false;
  let inFlight: Promise<void> | null = null;

  function set(patch: Partial<IntakeControllerState>): void {
    current = { ...current, ...patch };
    store.set(current);
  }

  function cancelTimer(): void {
    if (timer !== null) clearTimer(timer);
    timer = null;
  }

  function schedule(): void {
    cancelTimer();
    if (disposed) return;
    timer = setTimer(() => {
      timer = null;
      void poll();
    }, intervalMs);
    set({ polling: true });
  }

  function upsert(capture: ConnectionCaptureRow): ConnectionCaptureRow[] {
    const rest = current.captures.filter((existing) => existing.id !== capture.id);
    return [capture, ...rest];
  }

  async function pullSamples(sessionId: string, captures: readonly ConnectionCaptureRow[]): Promise<void> {
    const samples = await api.fetchIntakeSessionSamples(sessionId);
    if (disposed || current.sessionId !== sessionId) return;
    const intake = intakeSamplesFrom(samples, captures);
    if (intake.length > 0) deps.onSamples(intake, { activate: false });
  }

  async function poll(): Promise<void> {
    const sessionId = deps.currentSession();
    if (sessionId !== current.sessionId) {
      // A different session: its captures are not this one's.
      sampleSignature = '';
      set({ sessionId, captures: [], pollError: null });
    }
    if (!sessionId || disposed) {
      set({ polling: false });
      return;
    }
    try {
      const captures = await api.fetchConnectionCaptures(sessionId);
      if (disposed || current.sessionId !== sessionId) return;
      const signature = captureSampleSignature(captures);
      if (signature !== sampleSignature) {
        await pullSamples(sessionId, captures);
        sampleSignature = signature;
      }
      failures = 0;
      set({ captures, pollError: null });
      if (anyArmed(captures)) schedule();
      else set({ polling: false });
    } catch (error) {
      if (disposed) return;
      failures += 1;
      const message = describeIntakeFailure(error).message;
      if (failures < MAX_CONSECUTIVE_POLL_FAILURES && anyArmed(current.captures)) {
        set({ pollError: message });
        schedule();
      } else {
        set({ pollError: message, polling: false });
      }
    }
  }

  async function refresh(): Promise<void> {
    cancelTimer();
    failures = 0;
    if (!inFlight) {
      inFlight = poll().finally(() => {
        inFlight = null;
      });
    }
    await inFlight;
  }

  async function startCapture(request: StartCaptureRequest): Promise<ConnectionCaptureRow> {
    const sessionId = await deps.ensureSession();
    if (current.sessionId !== sessionId) {
      sampleSignature = '';
      set({ sessionId, captures: [] });
    }
    const capture = await api.startConnectionCapture({
      sourceId: request.sourceId,
      sessionId,
      maxMessages: request.maxMessages,
      ttlSeconds: request.ttlSeconds,
      reason: request.reason
    });
    failures = 0;
    set({ captures: upsert(capture), pollError: null });
    schedule();
    return capture;
  }

  async function cancelCapture(id: string, reason: string): Promise<ConnectionCaptureRow> {
    const capture = await api.cancelConnectionCapture(id, reason);
    set({ captures: upsert(capture) });
    // One more read: the final counts, and any sample that landed meanwhile.
    await refresh();
    return capture;
  }

  async function peek(request: PeekRequest): Promise<BatchPeekResultView> {
    const sessionId = await deps.ensureSession();
    if (current.sessionId !== sessionId) {
      sampleSignature = '';
      set({ sessionId, captures: [] });
    }
    const result = await api.peekBatchConnection({
      connectionId: request.connectionId,
      sessionId,
      objectPath: request.objectPath ?? null,
      maxObjects: request.maxObjects ?? null,
      maxMessages: request.maxMessages ?? null,
      reason: request.reason
    });
    set({ captures: upsert(result.capture) });
    const samples = intakeSamplesFrom(result.samples, [result.capture]);
    if (samples.length > 0) deps.onSamples(samples, { activate: true });
    return result;
  }

  function dispose(): void {
    disposed = true;
    cancelTimer();
    set({ polling: false });
  }

  return {
    state: { subscribe: store.subscribe },
    refresh,
    startCapture,
    cancelCapture,
    peek,
    dispose
  };
}
