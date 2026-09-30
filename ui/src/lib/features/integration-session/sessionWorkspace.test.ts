import { afterEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { resetObservedStreams } from '$lib/graphql/streamAvailability';
import { createSessionWorkspace, isLiveRun } from './sessionWorkspace';
import type { RunStreamCallbacks, SessionDiagnosticRow, SessionRunSummary, SessionWorkspace } from './workspaceApi';

function run(id: string, createdAt: string, status = 'completed'): SessionRunSummary {
  return {
    id,
    sessionId: 's-1',
    sampleId: 'sample-1',
    status,
    profileRevisionId: null,
    profileRevisionDigest: null,
    createdAt,
    completedAt: status === 'completed' ? createdAt : null,
    stages: []
  };
}

function diagnostic(id: string, runId: string, accepted = false): SessionDiagnosticRow {
  return {
    id,
    sessionId: 's-1',
    runId,
    sampleId: 'sample-1',
    severity: 'warning',
    code: 'MISSING_PV1',
    message: 'PV1 is missing',
    path: 'PV1',
    fixSuggestion: 'Review the source profile or sample payload for this warning.',
    accepted,
    acceptedAt: accepted ? '2026-01-01T00:00:00Z' : null,
    lineage: []
  };
}

const SESSION: SessionWorkspace = {
  id: 's-1',
  name: 'HL7 source profile workspace',
  description: null,
  archived: false,
  createdAt: '2026-01-01T09:00:00Z',
  updatedAt: '2026-01-01T09:05:00Z',
  samples: [],
  currentProfileDraft: null,
  currentWorkflowDraft: null,
  workflowSimulations: [],
  publications: []
};

function fakeApi(runs: SessionRunSummary[] = [run('r-old', '2026-01-01T09:00:00Z'), run('r-new', '2026-01-01T09:04:00Z')]) {
  const streams: Array<{ runId: string; callbacks: RunStreamCallbacks; closed: boolean }> = [];
  const api = {
    fetchSessionWorkspace: vi.fn(async (id: string) =>
      id === SESSION.id ? ({ kind: 'found', session: SESSION } as const) : ({ kind: 'absent' } as const)
    ),
    // The real wrapper sorts newest first; the fake returns them that way.
    fetchSessionRuns: vi.fn(async () => [...runs].sort((a, b) => b.createdAt.localeCompare(a.createdAt))),
    fetchSessionRun: vi.fn(async () => null),
    fetchRunDiagnostics: vi.fn(async (_sessionId: string, runId: string) => [diagnostic(`d-${runId}`, runId)]),
    archiveSession: vi.fn(async () => ({ archived: true, updatedAt: '2026-01-01T10:00:00Z' })),
    acceptDiagnosticFix: vi.fn(async (_sessionId: string, diagnosticId: string) => ({
      ...diagnostic(diagnosticId, 'r-new', true)
    })),
    exportSessionBundle: vi.fn(),
    subscribeRunEvents: vi.fn((_sessionId: string, runId: string, callbacks: RunStreamCallbacks) => {
      const entry = { runId, callbacks, closed: false };
      streams.push(entry);
      return () => {
        entry.closed = true;
      };
    })
  };
  return { api, streams };
}

afterEach(() => {
  resetObservedStreams();
});

describe('createSessionWorkspace', () => {
  it('opens a session, lists its runs newest first and reads the newest run\'s diagnostics', async () => {
    const { api } = fakeApi();
    const workspace = createSessionWorkspace({ api, canStream: () => true });
    const state = await workspace.open('s-1');
    expect(state.status).toEqual({ kind: 'ready', sessionId: 's-1' });
    expect(state.session?.name).toBe('HL7 source profile workspace');
    expect(state.runs.map((entry) => entry.id)).toEqual(['r-new', 'r-old']);
    expect(state.selectedRunId).toBe('r-new');
    expect(state.diagnostics?.map((entry) => entry.id)).toEqual(['d-r-new']);
    expect(api.fetchRunDiagnostics).toHaveBeenCalledWith('s-1', 'r-new');
    // Terminal runs never open a stream.
    expect(api.subscribeRunEvents).not.toHaveBeenCalled();
    expect(state.stream).toEqual({ state: 'idle' });
  });

  it('reports absent, forbidden and failed reads as their own states', async () => {
    const { api } = fakeApi();
    const workspace = createSessionWorkspace({ api });
    expect((await workspace.open('missing')).status).toEqual({ kind: 'absent', sessionId: 'missing' });

    api.fetchSessionWorkspace.mockResolvedValueOnce({ kind: 'forbidden' } as never);
    expect((await workspace.open('s-1')).status).toEqual({ kind: 'forbidden', sessionId: 's-1' });

    api.fetchSessionWorkspace.mockRejectedValueOnce(new Error('GraphQL request failed'));
    expect((await workspace.open('s-1')).status).toEqual({
      kind: 'error',
      sessionId: 's-1',
      message: 'GraphQL request failed'
    });
    expect(api.fetchSessionRuns).not.toHaveBeenCalled();

    workspace.markUnavailable('s-9');
    expect(get(workspace.state).status).toEqual({ kind: 'unavailable', sessionId: 's-9' });
  });

  it('streams a live run until it finishes, then reads its final diagnostics', async () => {
    const { api, streams } = fakeApi([run('r-live', '2026-01-01T09:04:00Z', 'running')]);
    const workspace = createSessionWorkspace({ api, canStream: () => true });
    await workspace.open('s-1');
    expect(streams).toHaveLength(1);
    expect(streams[0]!.runId).toBe('r-live');
    expect(get(workspace.state).stream).toMatchObject({ state: 'connecting', runId: 'r-live' });

    streams[0]!.callbacks.onOpen?.();
    streams[0]!.callbacks.onEvent({
      id: 'e1',
      type: 'run.stage',
      sessionId: 's-1',
      runId: 'r-live',
      message: 'stage',
      timestamp: '2026-01-01T09:04:01Z',
      run: run('r-live', '2026-01-01T09:04:00Z', 'running')
    });
    expect(get(workspace.state).stream).toEqual({ state: 'open', runId: 'r-live', events: 1 });

    api.fetchRunDiagnostics.mockClear();
    streams[0]!.callbacks.onEvent({
      id: 'e2',
      type: 'run.completed',
      sessionId: 's-1',
      runId: 'r-live',
      message: 'done',
      timestamp: '2026-01-01T09:04:02Z',
      run: run('r-live', '2026-01-01T09:04:00Z', 'completed')
    });
    expect(streams[0]!.closed).toBe(true);
    expect(get(workspace.state).stream).toEqual({ state: 'closed', runId: 'r-live', events: 2 });
    expect(get(workspace.state).runs[0]!.status).toBe('completed');
    await vi.waitFor(() => expect(api.fetchRunDiagnostics).toHaveBeenCalledWith('s-1', 'r-live'));
  });

  it('does not open a stream the deployment cannot serve', async () => {
    const { api } = fakeApi([run('r-live', '2026-01-01T09:04:00Z', 'pending')]);
    const workspace = createSessionWorkspace({ api, canStream: () => false });
    await workspace.open('s-1');
    expect(api.subscribeRunEvents).not.toHaveBeenCalled();
    expect(get(workspace.state).stream).toEqual({ state: 'unavailable', runId: 'r-live' });
  });

  it('marks the stream unavailable on a refusal and keeps an ordinary failure as an error', async () => {
    const { api, streams } = fakeApi([run('r-live', '2026-01-01T09:04:00Z', 'running')]);
    const workspace = createSessionWorkspace({ api, canStream: () => true });
    await workspace.open('s-1');
    streams[0]!.callbacks.onError?.(new Error('network down'));
    expect(get(workspace.state).stream).toEqual({ state: 'error', runId: 'r-live', message: 'network down' });

    await workspace.selectRun('r-live');
    streams[1]!.callbacks.onError?.(new Error('GraphQL stream HTTP 404'));
    expect(get(workspace.state).stream).toEqual({ state: 'unavailable', runId: 'r-live' });
  });

  it('closes the stream when another run is selected, and ignores the old run\'s events', async () => {
    const { api, streams } = fakeApi([
      run('r-live', '2026-01-01T09:04:00Z', 'running'),
      run('r-done', '2026-01-01T09:00:00Z')
    ]);
    const workspace = createSessionWorkspace({ api, canStream: () => true });
    await workspace.open('s-1');
    await workspace.selectRun('r-done');
    expect(streams[0]!.closed).toBe(true);
    streams[0]!.callbacks.onOpen?.();
    expect(get(workspace.state).stream).toEqual({ state: 'idle' });
    expect(get(workspace.state).diagnostics?.[0]?.runId).toBe('r-done');
  });

  it('drops a slower answer from a session that is no longer open', async () => {
    const { api } = fakeApi();
    let release: (value: { kind: 'found'; session: SessionWorkspace }) => void = () => {};
    api.fetchSessionWorkspace.mockImplementationOnce(
      () => new Promise((resolve) => (release = resolve)) as never
    );
    const workspace = createSessionWorkspace({ api });
    const first = workspace.open('s-1');
    await workspace.open('missing');
    release({ kind: 'found', session: SESSION });
    await first;
    expect(get(workspace.state).status).toEqual({ kind: 'absent', sessionId: 'missing' });
  });

  it('archives, accepts a fix in place, and refreshes onto the newest run', async () => {
    const runs = [run('r-old', '2026-01-01T09:00:00Z')];
    const { api } = fakeApi(runs);
    const workspace = createSessionWorkspace({ api });
    await workspace.open('s-1');

    await workspace.archive();
    expect(api.archiveSession).toHaveBeenCalledWith('s-1');
    expect(get(workspace.state).session?.archived).toBe(true);

    const accepted = await workspace.acceptFix('d-r-old');
    expect(accepted.accepted).toBe(true);
    expect(api.acceptDiagnosticFix).toHaveBeenCalledWith('s-1', 'd-r-old');
    expect(get(workspace.state).diagnostics?.[0]?.accepted).toBe(true);

    runs.push(run('r-next', '2026-01-01T09:10:00Z'));
    await workspace.refresh(true);
    expect(get(workspace.state).selectedRunId).toBe('r-next');
  });

  it('knows which statuses are live', () => {
    expect(isLiveRun({ status: 'pending' })).toBe(true);
    expect(isLiveRun({ status: 'running' })).toBe(true);
    expect(isLiveRun({ status: 'completed' })).toBe(false);
    expect(isLiveRun({ status: 'failed' })).toBe(false);
    expect(isLiveRun(null)).toBe(false);
  });
});
