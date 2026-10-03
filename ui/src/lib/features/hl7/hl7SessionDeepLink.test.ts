/**
 * `/hl7?session=<id>` (.loom/42 E-3): the page reopens the session, lists its
 * runs in the session rail and shows the newest run in the results pane; an
 * id the store does not hold, or a deployment without sessions, renders the
 * rail's honest state instead.
 *
 * Only `fetch` is faked; the real GraphQL client runs.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import { setGraphQLTrustedNetworkAccess } from '$lib/graphql/credentials';
import { resetObservedStreams } from '$lib/graphql/streamAvailability';
import { toasts } from '$lib/ui/toastStore';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { EditorView } from '@codemirror/view';
import { problemsDiagnostics } from '$lib/ui/ide/panels/workflowProblemsStore';
import HL7PreviewPage from './HL7PreviewPage.svelte';

const navigation = vi.hoisted(() => ({
  callback: null as ((navigation: { to: { url: URL } }) => void) | null
}));
vi.mock('$app/navigation', () => ({
  afterNavigate: (callback: typeof navigation.callback) => { navigation.callback = callback; },
  replaceState: (url: URL) => history.replaceState(null, '', url)
}));

async function navigate(path: string): Promise<void> {
  history.pushState(null, '', path);
  navigation.callback?.({ to: { url: new URL(window.location.href) } });
  await tick();
}

function editorView(): EditorView {
  const view = EditorView.findFromDOM(screen.getByTestId('code-editor'));
  if (!view) throw new Error('HL7 editor is not mounted');
  return view;
}

function deferred() {
  let resolve: () => void = () => {};
  const promise = new Promise<void>((done) => { resolve = done; });
  return { promise, resolve };
}

const RUN = {
  id: 'run-7',
  sessionId: 'session-1',
  sampleId: 'sample-1',
  status: 'completed',
  profileRevisionId: null,
  profileRevisionDigest: null,
  createdAt: '2026-01-01T09:04:00Z',
  completedAt: '2026-01-01T09:04:01Z',
  stages: []
};

const WORKSPACE = {
  id: 'session-1',
  name: 'HL7 source profile workspace',
  description: null,
  archived: false,
  createdAt: '2026-01-01T09:00:00Z',
  updatedAt: '2026-01-01T09:04:01Z',
  samples: [],
  currentProfileDraft: null,
  currentWorkflowDraft: null,
  workflowSimulations: [],
  publications: []
};

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'content-type': 'application/json' } });
}

/** An SSE response that opens and then stays silent. */
function openStream(): Response {
  return new Response(new ReadableStream<Uint8Array>(), {
    status: 200,
    headers: { 'content-type': 'text/event-stream' }
  });
}

const WARNING = {
  phase: 'semantic',
  code: 'MISSING_PV1_3',
  message: 'PV1-3 assigned location is empty',
  path: 'PV1.3',
  explanation: null,
  fixSuggestion: null,
  impact: null,
  severity: 'warning',
  fromCache: null
};

const DIAGNOSTIC = {
  id: 'diag_001',
  sessionId: 'session-1',
  runId: 'run-7',
  sampleId: 'sample-1',
  severity: 'warning',
  code: 'MISSING_PV1_3',
  message: 'PV1-3 assigned location is empty',
  path: 'PV1.3',
  fixSuggestion: 'Review the source profile or sample payload for this warning.',
  accepted: false,
  acceptedAt: null,
  lineage: []
};

function fakeApi(options: {
  workspaceGate?: Promise<void>;
  workspaceGates?: Record<string, Promise<void>>;
  detailGates?: Record<string, Promise<void>>;
  previewGate?: Promise<void>;
  runDetailFailure?: 'absent' | 'error';
  withWarning?: boolean;
} = {}) {
  const operations: string[] = [];
  const variables: Record<string, Record<string, unknown>[]> = {};
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    if (String(input) !== '/graphql') return new Response('not found', { status: 404 });
    const body = JSON.parse(String(init?.body ?? '{}')) as { query?: string; variables?: Record<string, unknown> };
    const operation = /\b(?:query|mutation|subscription)\s+(\w+)/.exec(body.query ?? '')?.[1] ?? '';
    operations.push(operation);
    (variables[operation] ??= []).push(body.variables ?? {});
    if (new Headers(init?.headers).get('accept')?.includes('text/event-stream')) return openStream();
    switch (operation) {
      case 'IntegrationSessionWorkspace': {
        const id = String(body.variables?.['id']);
        await (options.workspaceGates?.[id] ?? options.workspaceGate);
        return json({ data: { integrationSession: ['session-1', 'session-2'].includes(id) ? { ...WORKSPACE, id } : null } });
      }
      case 'CreateStreamingIntegrationSession':
        return json({ data: { createIntegrationSession: { id: 'session-new' } } });
      case 'AddStreamingSessionSample': {
        const input = body.variables?.['input'] as { sessionId: string };
        return json({ data: { addSessionSample: { id: 'sample-2', sessionId: input.sessionId } } });
      }
      case 'RunStreamingSessionPreview': {
        await options.previewGate;
        const input = body.variables?.['input'] as { sessionId: string };
        return json({
          data: {
            runSessionPreview: { ...RUN, id: 'run-8', sessionId: input.sessionId, diagnostics: [], lineage: [], events: [], warnings: [] }
          }
        });
      }
      case 'SessionRunHistory':
        return json({ data: { sessionRuns: [body.variables?.['sessionId'] === 'session-2'
          ? { ...RUN, id: 'run-2', sessionId: 'session-2' }
          : RUN] } });
      case 'SessionRunDiagnostics':
        return json({ data: { sessionDiagnostics: [] } });
      case 'SessionRunDetail': {
        const id = String(body.variables?.['id']);
        await options.detailGates?.[id];
        if (options.runDetailFailure === 'absent') return json({ data: { sessionRun: null } });
        if (options.runDetailFailure === 'error') return json({ errors: [{ message: 'Run read failed' }] });
        return json({
          data: {
            sessionRun: {
              ...RUN,
              id,
              sessionId: id === 'run-2' ? 'session-2' : 'session-1',
              diagnostics: options.withWarning ? [DIAGNOSTIC] : [],
              lineage: [],
              events: [],
              warnings: options.withWarning ? [WARNING] : []
            }
          }
        });
      }
      case 'AcceptSessionDiagnosticFix':
        return json({
          data: { acceptDiagnosticFix: { ...DIAGNOSTIC, accepted: true, acceptedAt: '2026-01-01T10:00:00Z' } }
        });
      default:
        return json({ data: null, errors: [{ message: `unexpected operation ${operation}` }] });
    }
  });
  return { fetchMock, operations, variables };
}

function status(integrationSessions: boolean) {
  setAccessStatus({
    authenticated: true,
    authVia: 'network',
    roles: ['integration:preview', 'graphql:operator'],
    capabilities: {
      operatorRead: false,
      integrationSessions,
      streaming: integrationSessions,
      subscriptions: integrationSessions ? ['integrationSessionEvents', 'sessionRunEvents'] : [],
      controlPlane: false,
      connectionCatalog: false
    }
  });
}

beforeEach(() => {
  navigation.callback = null;
  vi.stubEnv('VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED', 'true');
  setGraphQLTrustedNetworkAccess(true);
});

afterEach(() => {
  cleanup();
  toasts.dismissAll();
  setGraphQLTrustedNetworkAccess(false);
  resetAccessCapabilities();
  resetObservedStreams();
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
  history.replaceState(null, '', '/');
});

describe('HL7 intake deep link', { timeout: 30_000 }, () => {
  it('reopens the session: the rail lists its run and the results show it', async () => {
    status(true);
    const api = fakeApi();
    vi.stubGlobal('fetch', api.fetchMock);
    history.replaceState(null, '', '/hl7?session=session-1');
    render(HL7PreviewPage);

    const rail = await screen.findByTestId('hl7-session-sidebar');
    expect(rail).toHaveAttribute('data-session-id', 'session-1');
    const run = await screen.findByTestId('hl7-session-run');
    expect(run).toHaveAttribute('data-run-id', 'run-7');
    await vi.waitFor(() =>
      expect(screen.getByRole('region', { name: 'Server preview progression' })).toHaveClass('state-complete')
    );
    expect(await screen.findByText('In results')).toBeInTheDocument();
    expect(api.operations).toEqual(
      expect.arrayContaining(['IntegrationSessionWorkspace', 'SessionRunHistory', 'SessionRunDiagnostics', 'SessionRunDetail'])
    );
    // Reading a session creates nothing.
    expect(api.operations).not.toContain('CreateStreamingIntegrationSession');
  });

  it('says so when the id is not in the store', async () => {
    status(true);
    const api = fakeApi();
    vi.stubGlobal('fetch', api.fetchMock);
    history.replaceState(null, '', '/hl7?session=gone');
    render(HL7PreviewPage);

    expect(await screen.findByTestId('hl7-session-absent')).toHaveTextContent('Session gone is not in');
    expect(api.operations).not.toContain('SessionRunHistory');
  });

  it('queries nothing when the deployment has no session workspace', async () => {
    status(false);
    const api = fakeApi();
    vi.stubGlobal('fetch', api.fetchMock);
    history.replaceState(null, '', '/hl7?session=session-1');
    render(HL7PreviewPage);

    expect(await screen.findByTestId('hl7-session-unavailable')).toHaveTextContent(
      'The API has no Integration Session workspace on this deployment'
    );
    expect(api.operations).not.toContain('IntegrationSessionWorkspace');
  });

  it('a Preview pressed while the link is still opening runs in the linked session', async () => {
    status(true);
    let release: () => void = () => {};
    const gate = new Promise<void>((resolve) => (release = resolve));
    const api = fakeApi({ workspaceGate: gate });
    vi.stubGlobal('fetch', api.fetchMock);
    history.replaceState(null, '', '/hl7?session=session-1');
    render(HL7PreviewPage);

    await vi.waitFor(() => expect(api.operations).toContain('IntegrationSessionWorkspace'));
    await fireEvent.click(screen.getAllByRole('button', { name: 'Preview' })[0]!);
    release();

    await vi.waitFor(() => expect(api.operations).toContain('RunStreamingSessionPreview'));
    expect(api.operations).not.toContain('CreateStreamingIntegrationSession');
    expect(api.variables['AddStreamingSessionSample']?.[0]).toMatchObject({ input: { sessionId: 'session-1' } });
  });

  it('accepts a warning\'s fix from the Warnings list and marks it accepted', async () => {
    status(true);
    const api = fakeApi({ withWarning: true });
    vi.stubGlobal('fetch', api.fetchMock);
    history.replaceState(null, '', '/hl7?session=session-1');
    render(HL7PreviewPage);

    const accept = await screen.findByRole('button', { name: /Accept fix/ }, { timeout: 5_000 });
    // The sidebar lists the run's diagnostics too; take the one in the Warnings list.
    const inWarnings = screen
      .getAllByRole('button', { name: /Accept fix/ })
      .find((button) => !button.closest('[data-testid="hl7-session-rail"]'));
    expect(inWarnings ?? accept).toBeDefined();
    await fireEvent.click(inWarnings ?? accept);

    await vi.waitFor(() => expect(screen.getByTestId('warning-fix-accepted')).toBeInTheDocument());
    expect(api.variables['AcceptSessionDiagnosticFix']?.[0]).toMatchObject({
      input: { sessionId: 'session-1', diagnosticId: 'diag_001' }
    });
  });

  it('switches the rail and results on same-route navigation without discarding samples or editor edits', async () => {
    status(true);
    const api = fakeApi({ withWarning: true });
    vi.stubGlobal('fetch', api.fetchMock);
    history.replaceState(null, '', '/hl7?session=session-1');
    render(HL7PreviewPage);
    await screen.findByText('In results');
    await fireEvent.click(screen.getByRole('tab', { name: /^Samples/ }));
    await fireEvent.click(screen.getByRole('button', { name: 'Load examples' }));
    const sampleRows = within(screen.getByRole('table', { name: 'Saved samples' })).getAllByRole('row').length;
    const edited = 'MSH|^~\\&|EDITED|TEST|FI_FHIR|TEST|20260101090000||ADT^A01|EDITED-001|T|2.5.1';
    const editor = editorView();
    editor.dispatch({ changes: { from: 0, to: editor.state.doc.length, insert: edited } });
    await fireEvent.input(screen.getByPlaceholderText('epic_adt_hosp_a'), { target: { value: 'edited_source' } });
    await navigate('/hl7?session=session-2');

    await vi.waitFor(() => {
      expect(screen.getByTestId('hl7-session-sidebar')).toHaveAttribute('data-session-id', 'session-2');
      expect(screen.getByTestId('hl7-session-run')).toHaveAttribute('data-run-id', 'run-2');
      expect(screen.getByText('In results')).toBeInTheDocument();
      expect(get(problemsDiagnostics).issues[0]?.id).toBe('run-2:diag_001');
    });
    expect(editorView()).toBe(editor);
    expect(editorView().state.doc.toString()).toBe(edited);
    expect(screen.getByPlaceholderText('epic_adt_hosp_a')).toHaveValue('edited_source');
    expect(within(screen.getByRole('table', { name: 'Saved samples' })).getAllByRole('row')).toHaveLength(sampleRows);
    expect(api.operations).not.toContain('CreateStreamingIntegrationSession');
  });

  it('clears old results for a missing target and does not preview into the old or a replacement session', async () => {
    status(true);
    const api = fakeApi({ withWarning: true });
    vi.stubGlobal('fetch', api.fetchMock);
    history.replaceState(null, '', '/hl7?session=session-1');
    render(HL7PreviewPage);
    await screen.findByText('In results');
    await navigate('/hl7?session=gone');

    await screen.findByTestId('hl7-session-absent');
    expect(screen.queryByRole('region', { name: 'Server preview progression' })).not.toBeInTheDocument();
    expect(get(problemsDiagnostics).sessionCount).toBe(0);
    await fireEvent.click(screen.getAllByRole('button', { name: 'Preview' })[0]!);
    expect(await screen.findByText(/Session gone could not be opened/)).toBeInTheDocument();
    expect(api.operations).not.toContain('CreateStreamingIntegrationSession');
    expect(api.operations).not.toContain('AddStreamingSessionSample');
  });

  it('a Preview pressed while the replacement session opens writes into that target and preserves its own result', async () => {
    status(true);
    const gate = deferred();
    const api = fakeApi({ workspaceGates: { 'session-2': gate.promise } });
    vi.stubGlobal('fetch', api.fetchMock);
    history.replaceState(null, '', '/hl7?session=session-1');
    render(HL7PreviewPage);
    await screen.findByText('In results');
    await navigate('/hl7?session=session-2');
    await fireEvent.click(screen.getAllByRole('button', { name: 'Preview' })[0]!);
    gate.resolve();

    await vi.waitFor(() => expect(api.operations).toContain('RunStreamingSessionPreview'));
    expect(api.variables['AddStreamingSessionSample']?.[0]).toMatchObject({ input: { sessionId: 'session-2' } });
    expect(api.variables['RunStreamingSessionPreview']?.[0]).toMatchObject({ input: { sessionId: 'session-2' } });
    await vi.waitFor(() => expect(screen.getByText('run-8')).toBeInTheDocument());
    await navigate('/hl7?session=session-2');
    expect(screen.getByText('run-8')).toBeInTheDocument();
    expect(api.variables['SessionRunDetail']).toEqual([{ id: 'run-7' }]);
  });

  it('ignores an older session open that finishes after the replacement is ready', async () => {
    status(true);
    const gate = deferred();
    const api = fakeApi({ workspaceGates: { 'session-1': gate.promise } });
    vi.stubGlobal('fetch', api.fetchMock);
    history.replaceState(null, '', '/hl7?session=session-1');
    render(HL7PreviewPage);
    await vi.waitFor(() => expect(api.operations).toContain('IntegrationSessionWorkspace'));
    await fireEvent.click(screen.getAllByRole('button', { name: 'Preview' })[0]!);
    await navigate('/hl7?session=session-2');
    await screen.findByText('In results');
    gate.resolve();
    await tick();
    await vi.waitFor(() => expect(screen.getByTestId('hl7-session-run')).toHaveAttribute('data-run-id', 'run-2'));
    expect(api.variables['SessionRunDetail']).toEqual([{ id: 'run-2' }]);
    expect(api.operations).not.toContain('AddStreamingSessionSample');
    expect(new URL(window.location.href).searchParams.get('session')).toBe('session-2');
  });

  it('ignores an old run detail that completes after another session is displayed', async () => {
    status(true);
    const gate = deferred();
    const api = fakeApi({ detailGates: { 'run-7': gate.promise }, withWarning: true });
    vi.stubGlobal('fetch', api.fetchMock);
    history.replaceState(null, '', '/hl7?session=session-1');
    render(HL7PreviewPage);
    await vi.waitFor(() => expect(api.operations).toContain('SessionRunDetail'));
    await navigate('/hl7?session=session-2');
    await screen.findByText('In results');
    gate.resolve();
    await tick();
    await vi.waitFor(() => expect(get(problemsDiagnostics).issues[0]?.id).toBe('run-2:diag_001'));
    expect(screen.getByTestId('hl7-session-run')).toHaveAttribute('data-run-id', 'run-2');
    expect(screen.getByText('In results')).toBeInTheDocument();
  });

  it('ignores an old Preview completion and resets the workspace when the selector is removed', async () => {
    status(true);
    const gate = deferred();
    const api = fakeApi({ previewGate: gate.promise });
    vi.stubGlobal('fetch', api.fetchMock);
    history.replaceState(null, '', '/hl7?session=session-1');
    render(HL7PreviewPage);
    await screen.findByText('In results');
    await fireEvent.click(screen.getAllByRole('button', { name: 'Preview' })[0]!);
    await vi.waitFor(() => expect(api.operations).toContain('RunStreamingSessionPreview'));
    await navigate('/hl7?session=session-2');
    await screen.findByText('In results');
    gate.resolve();
    await tick();
    await vi.waitFor(() => expect(screen.getByText('In results')).toBeInTheDocument());
    expect(screen.queryByText('run-8')).not.toBeInTheDocument();
    expect(new URL(window.location.href).searchParams.get('session')).toBe('session-2');

    await navigate('/hl7');
    expect(screen.queryByTestId('hl7-session-sidebar')).not.toBeInTheDocument();
    expect(screen.queryByRole('region', { name: 'Server preview progression' })).not.toBeInTheDocument();
    expect(get(problemsDiagnostics).sessionCount).toBe(0);
  });


  it('clears the previous result when the next target is unavailable on the deployment', async () => {
    status(true);
    const api = fakeApi({ withWarning: true });
    vi.stubGlobal('fetch', api.fetchMock);
    history.replaceState(null, '', '/hl7?session=session-1');
    render(HL7PreviewPage);
    await screen.findByText('In results');
    status(false);
    await tick();
    await navigate('/hl7?session=session-2');

    expect(await screen.findByTestId('hl7-session-unavailable')).toBeInTheDocument();
    expect(screen.queryByRole('region', { name: 'Server preview progression' })).not.toBeInTheDocument();
    expect(get(problemsDiagnostics).sessionCount).toBe(0);
    expect(api.variables['IntegrationSessionWorkspace']).toEqual([{ id: 'session-1' }]);
  });


  it.each(['absent', 'error'] as const)('unlocks Preview after Show in results supersedes it and returns %s', async (failure) => {
    status(true);
    const gate = deferred();
    const options: Parameters<typeof fakeApi>[0] = { previewGate: gate.promise };
    const api = fakeApi(options);
    vi.stubGlobal('fetch', api.fetchMock);
    history.replaceState(null, '', '/hl7?session=session-1');
    render(HL7PreviewPage);
    await screen.findByText('In results');
    await fireEvent.click(screen.getAllByRole('button', { name: 'Preview' })[0]!);
    await vi.waitFor(() => expect(api.operations).toContain('RunStreamingSessionPreview'));
    options.runDetailFailure = failure;
    await fireEvent.click(screen.getByTestId('hl7-session-show-run'));

    expect(await screen.findByText(failure === 'absent' ? /Run run-7 is no longer/ : /Run run-7 could not be read/)).toBeInTheDocument();
    expect(screen.getAllByRole('button', { name: 'Preview' })[0]).toBeEnabled();
    expect(screen.queryByRole('region', { name: 'Server preview progression' })).not.toBeInTheDocument();
    gate.resolve();
    await tick();
    expect(screen.queryByText('run-8')).not.toBeInTheDocument();
  });

});
