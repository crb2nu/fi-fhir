/**
 * The /hl7 page's view of its Integration Session as a piece of work
 * (.loom/42 E-3): what `integrationSession`, `sessionRuns` and
 * `sessionDiagnostics` say about it, the run the sidebar has selected, and
 * the per-run stream while that run is live.
 *
 * Honest states, in order: no session on the page (`idle`); a deep link the
 * page could not open because the session engine is off (`unavailable`); the
 * id is not in this tenant's store (`absent`); the identity may not read it
 * (`forbidden`); a read failed (`error`); otherwise `ready`. Nothing here
 * invents a value: a list is empty only when the server said so.
 *
 * The stream opens only for a run the server reports as `pending` or
 * `running`, only when `sessionRunEvents` is not known to be unavailable, and
 * closes as soon as the run reaches a terminal status.
 */
import { writable, type Readable } from 'svelte/store';
import { canAttemptStream, noteStreamError } from '$lib/graphql/streamAvailability';
import * as defaultApi from './workspaceApi';
import type {
  SessionDiagnosticRow,
  SessionRunDetail,
  SessionRunSummary,
  SessionWorkspace
} from './workspaceApi';

export type WorkspaceStatus =
  | { kind: 'idle' }
  | { kind: 'loading'; sessionId: string }
  | { kind: 'ready'; sessionId: string }
  | { kind: 'unavailable'; sessionId: string }
  | { kind: 'absent'; sessionId: string }
  | { kind: 'forbidden'; sessionId: string }
  | { kind: 'error'; sessionId: string; message: string };

export type RunStreamState =
  | { state: 'idle' }
  | { state: 'connecting' | 'open'; runId: string; events: number }
  | { state: 'closed'; runId: string; events: number }
  | { state: 'unavailable'; runId: string }
  | { state: 'error'; runId: string; message: string };

export interface SessionWorkspaceState {
  status: WorkspaceStatus;
  session: SessionWorkspace | null;
  runs: SessionRunSummary[];
  runsError: string | null;
  selectedRunId: string | null;
  /** The selected run's diagnostics; null until read. */
  diagnostics: SessionDiagnosticRow[] | null;
  diagnosticsLoading: boolean;
  diagnosticsError: string | null;
  stream: RunStreamState;
}

export const LIVE_RUN_STATUSES: readonly string[] = ['pending', 'running'];

export function isLiveRun(run: Pick<SessionRunSummary, 'status'> | null | undefined): boolean {
  return Boolean(run && LIVE_RUN_STATUSES.includes(run.status));
}

const INITIAL: SessionWorkspaceState = Object.freeze({
  status: { kind: 'idle' },
  session: null,
  runs: [],
  runsError: null,
  selectedRunId: null,
  diagnostics: null,
  diagnosticsLoading: false,
  diagnosticsError: null,
  stream: { state: 'idle' }
}) as SessionWorkspaceState;

type WorkspaceApi = Pick<
  typeof defaultApi,
  | 'fetchSessionWorkspace'
  | 'fetchSessionRuns'
  | 'fetchSessionRun'
  | 'fetchRunDiagnostics'
  | 'archiveSession'
  | 'acceptDiagnosticFix'
  | 'exportSessionBundle'
  | 'subscribeRunEvents'
>;

export interface SessionWorkspaceDeps {
  api?: Partial<WorkspaceApi> | undefined;
  /** Whether `sessionRunEvents` may be tried now (defaults to the stream-availability gate). */
  canStream?: (() => boolean) | undefined;
}

export interface SessionWorkspaceController {
  state: Readable<SessionWorkspaceState>;
  /** Reads the session and its runs, and selects the newest run. */
  open: (sessionId: string) => Promise<SessionWorkspaceState>;
  /** A deep link the page cannot open because the session engine is off. */
  markUnavailable: (sessionId: string) => void;
  /** Re-reads the open session and its runs, keeping (or moving to the newest) selection. */
  refresh: (selectNewest?: boolean) => Promise<void>;
  selectRun: (runId: string) => Promise<void>;
  /** The full run (events, warnings, stages) for the results pane; null when absent. */
  runDetail: (runId: string) => Promise<SessionRunDetail | null>;
  archive: () => Promise<void>;
  exportBundle: (reason: string, includeRawPayload: boolean) => ReturnType<WorkspaceApi['exportSessionBundle']>;
  /** Accepts a diagnostic's fix in the open session (or `sessionIdOverride`, the page's own). */
  acceptFix: (diagnosticId: string, sessionIdOverride?: string) => Promise<SessionDiagnosticRow>;
  reset: () => void;
  dispose: () => void;
}

function message(error: unknown): string {
  return error instanceof Error && error.message ? error.message : 'The request failed.';
}

export function createSessionWorkspace(deps: SessionWorkspaceDeps = {}): SessionWorkspaceController {
  const api: WorkspaceApi = { ...defaultApi, ...(deps.api ?? {}) };
  const canStream = deps.canStream ?? (() => canAttemptStream('sessionRunEvents'));

  let current: SessionWorkspaceState = INITIAL;
  const store = writable<SessionWorkspaceState>(current);
  let unsubscribeStream: (() => void) | null = null;
  // Every read carries the generation it started in; a newer open() or a
  // reset() makes older answers land nowhere.
  let generation = 0;
  let disposed = false;

  function set(patch: Partial<SessionWorkspaceState>): void {
    current = { ...current, ...patch };
    store.set(current);
  }

  function sessionId(): string | null {
    return current.status.kind === 'idle' ? null : current.status.sessionId;
  }

  function closeStream(): void {
    unsubscribeStream?.();
    unsubscribeStream = null;
  }

  function upsertRun(run: SessionRunSummary): void {
    const runs = current.runs.some((existing) => existing.id === run.id)
      ? current.runs.map((existing) => (existing.id === run.id ? run : existing))
      : [run, ...current.runs];
    set({ runs });
  }

  function openStream(id: string, runId: string): void {
    closeStream();
    if (!canStream()) {
      set({ stream: { state: 'unavailable', runId } });
      return;
    }
    let events = 0;
    const mine = generation;
    set({ stream: { state: 'connecting', runId, events } });
    unsubscribeStream = api.subscribeRunEvents(id, runId, {
      onOpen: () => {
        if (mine !== generation || current.selectedRunId !== runId) return;
        set({ stream: { state: 'open', runId, events } });
      },
      onEvent: (event) => {
        if (mine !== generation || current.selectedRunId !== runId) return;
        events += 1;
        if (event.run) upsertRun(event.run);
        const live = event.run ? isLiveRun(event.run) : true;
        if (live) {
          set({ stream: { state: 'open', runId, events } });
          return;
        }
        // The run finished: stop listening, and read its final diagnostics.
        closeStream();
        set({ stream: { state: 'closed', runId, events } });
        void loadDiagnostics(id, runId);
      },
      onError: (error) => {
        if (mine !== generation || current.selectedRunId !== runId) return;
        closeStream();
        set({
          stream: noteStreamError('sessionRunEvents', error)
            ? { state: 'unavailable', runId }
            : { state: 'error', runId, message: message(error) }
        });
      },
      onComplete: () => {
        if (mine !== generation || current.selectedRunId !== runId) return;
        unsubscribeStream = null;
        if (current.stream.state === 'open' || current.stream.state === 'connecting') {
          set({ stream: { state: 'closed', runId, events } });
        }
      }
    });
  }

  async function loadDiagnostics(id: string, runId: string): Promise<void> {
    const mine = generation;
    set({ diagnosticsLoading: true, diagnosticsError: null });
    try {
      const diagnostics = await api.fetchRunDiagnostics(id, runId);
      if (mine !== generation || current.selectedRunId !== runId) return;
      set({ diagnostics, diagnosticsLoading: false });
    } catch (error) {
      if (mine !== generation || current.selectedRunId !== runId) return;
      set({ diagnostics: null, diagnosticsLoading: false, diagnosticsError: message(error) });
    }
  }

  async function selectRun(runId: string): Promise<void> {
    const id = sessionId();
    if (!id || disposed) return;
    closeStream();
    set({ selectedRunId: runId, diagnostics: null, diagnosticsError: null, stream: { state: 'idle' } });
    const run = current.runs.find((candidate) => candidate.id === runId);
    if (isLiveRun(run)) openStream(id, runId);
    await loadDiagnostics(id, runId);
  }

  async function readRuns(id: string, mine: number, selectNewest: boolean): Promise<void> {
    try {
      const runs = await api.fetchSessionRuns(id);
      if (mine !== generation) return;
      set({ runs, runsError: null });
      const keep = !selectNewest && current.selectedRunId && runs.some((run) => run.id === current.selectedRunId);
      const next = keep ? current.selectedRunId : (runs[0]?.id ?? null);
      if (next && next !== current.selectedRunId) await selectRun(next);
      else if (next && selectNewest) await selectRun(next);
    } catch (error) {
      if (mine !== generation) return;
      set({ runsError: message(error) });
    }
  }

  async function open(id: string): Promise<SessionWorkspaceState> {
    closeStream();
    generation += 1;
    const mine = generation;
    current = { ...INITIAL, status: { kind: 'loading', sessionId: id } };
    store.set(current);
    try {
      const lookup = await api.fetchSessionWorkspace(id);
      if (mine !== generation) return current;
      if (lookup.kind !== 'found') {
        set({ status: { kind: lookup.kind, sessionId: id } });
        return current;
      }
      set({ status: { kind: 'ready', sessionId: id }, session: lookup.session });
    } catch (error) {
      if (mine !== generation) return current;
      set({ status: { kind: 'error', sessionId: id, message: message(error) } });
      return current;
    }
    await readRuns(id, mine, true);
    return current;
  }

  async function refresh(selectNewest = false): Promise<void> {
    const id = sessionId();
    if (!id || current.status.kind !== 'ready') return;
    const mine = generation;
    try {
      const lookup = await api.fetchSessionWorkspace(id);
      if (mine !== generation) return;
      if (lookup.kind === 'found') set({ session: lookup.session });
      else set({ status: { kind: lookup.kind, sessionId: id }, session: null });
    } catch (error) {
      if (mine !== generation) return;
      set({ runsError: message(error) });
      return;
    }
    if (current.status.kind === 'ready') await readRuns(id, mine, selectNewest);
  }

  async function archive(): Promise<void> {
    const id = sessionId();
    if (!id || !current.session) throw new Error('No session is open.');
    const result = await api.archiveSession(id);
    if (current.session && sessionId() === id) {
      set({ session: { ...current.session, archived: result.archived, updatedAt: result.updatedAt } });
    }
  }

  async function exportBundle(reason: string, includeRawPayload: boolean) {
    const id = sessionId();
    if (!id || !current.session) throw new Error('No session is open.');
    return api.exportSessionBundle({ sessionId: id, reason, includeRawPayload });
  }

  async function acceptFix(diagnosticId: string, sessionIdOverride?: string): Promise<SessionDiagnosticRow> {
    const id = sessionIdOverride ?? sessionId();
    if (!id) throw new Error('No session is open.');
    const accepted = await api.acceptDiagnosticFix(id, diagnosticId);
    if (current.diagnostics && sessionId() === id) {
      set({
        diagnostics: current.diagnostics.map((diagnostic) =>
          diagnostic.id === accepted.id ? { ...diagnostic, ...accepted } : diagnostic
        )
      });
    }
    return accepted;
  }

  function markUnavailable(id: string): void {
    closeStream();
    generation += 1;
    current = { ...INITIAL, status: { kind: 'unavailable', sessionId: id } };
    store.set(current);
  }

  function reset(): void {
    closeStream();
    generation += 1;
    current = INITIAL;
    store.set(current);
  }

  return {
    state: { subscribe: store.subscribe },
    open,
    markUnavailable,
    refresh,
    selectRun,
    runDetail: (runId) => api.fetchSessionRun(runId),
    archive,
    exportBundle,
    acceptFix,
    reset,
    dispose: () => {
      disposed = true;
      closeStream();
      generation += 1;
    }
  };
}
