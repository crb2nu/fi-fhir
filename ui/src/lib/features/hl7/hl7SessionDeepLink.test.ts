/**
 * `/hl7?session=<id>` (.loom/42 E-3): the page reopens the session, lists its
 * runs in the session rail and shows the newest run in the results pane; an
 * id the store does not hold, or a deployment without sessions, renders the
 * rail's honest state instead.
 *
 * Only `fetch` is faked; the real GraphQL client runs.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import { setGraphQLTrustedNetworkAccess } from '$lib/graphql/credentials';
import { resetObservedStreams } from '$lib/graphql/streamAvailability';
import { toasts } from '$lib/ui/toastStore';
import HL7PreviewPage from './HL7PreviewPage.svelte';

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

function fakeApi(options: { workspaceGate?: Promise<void>; withWarning?: boolean } = {}) {
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
      case 'IntegrationSessionWorkspace':
        await options.workspaceGate;
        return json({ data: { integrationSession: body.variables?.['id'] === 'session-1' ? WORKSPACE : null } });
      case 'CreateStreamingIntegrationSession':
        return json({ data: { createIntegrationSession: { id: 'session-new' } } });
      case 'AddStreamingSessionSample':
        return json({ data: { addSessionSample: { id: 'sample-2', sessionId: 'session-1' } } });
      case 'RunStreamingSessionPreview':
        return json({
          data: {
            runSessionPreview: { ...RUN, id: 'run-8', diagnostics: [], lineage: [], events: [], warnings: [] }
          }
        });
      case 'SessionRunHistory':
        return json({ data: { sessionRuns: [RUN] } });
      case 'SessionRunDiagnostics':
        return json({ data: { sessionDiagnostics: [] } });
      case 'SessionRunDetail':
        return json({
          data: {
            sessionRun: {
              ...RUN,
              diagnostics: options.withWarning ? [DIAGNOSTIC] : [],
              lineage: [],
              events: [],
              warnings: options.withWarning ? [WARNING] : []
            }
          }
        });
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
});
