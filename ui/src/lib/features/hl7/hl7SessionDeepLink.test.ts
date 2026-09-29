/**
 * `/hl7?session=<id>` (.loom/42 E-3): the page reopens the session, lists its
 * runs in the session rail and shows the newest run in the results pane; an
 * id the store does not hold, or a deployment without sessions, renders the
 * rail's honest state instead.
 *
 * Only `fetch` is faked; the real GraphQL client runs.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen } from '@testing-library/svelte';
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

function fakeApi() {
  const operations: string[] = [];
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    if (String(input) !== '/graphql') return new Response('not found', { status: 404 });
    const body = JSON.parse(String(init?.body ?? '{}')) as { query?: string; variables?: Record<string, unknown> };
    const operation = /\b(?:query|mutation|subscription)\s+(\w+)/.exec(body.query ?? '')?.[1] ?? '';
    operations.push(operation);
    switch (operation) {
      case 'IntegrationSessionWorkspace':
        return json({ data: { integrationSession: body.variables?.['id'] === 'session-1' ? WORKSPACE : null } });
      case 'SessionRunHistory':
        return json({ data: { sessionRuns: [RUN] } });
      case 'SessionRunDiagnostics':
        return json({ data: { sessionDiagnostics: [] } });
      case 'SessionRunDetail':
        return json({
          data: { sessionRun: { ...RUN, diagnostics: [], lineage: [], events: [], warnings: [] } }
        });
      default:
        return json({ data: null, errors: [{ message: `unexpected operation ${operation}` }] });
    }
  });
  return { fetchMock, operations };
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
});
