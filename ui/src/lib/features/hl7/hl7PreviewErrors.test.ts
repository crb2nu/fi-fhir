/**
 * HL7 intake with the Integration Session engine on (production's shape): a
 * preview that fails must say so on the status line, and a new run must never
 * keep showing the previous run's "Preview complete".
 *
 * Only `fetch` is faked; the real GraphQL and SSE clients run.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import { setGraphQLTrustedNetworkAccess } from '$lib/graphql/credentials';
import { resetObservedStreams } from '$lib/graphql/streamAvailability';
import { toasts } from '$lib/ui/toastStore';
import HL7PreviewPage from './HL7PreviewPage.svelte';

type Recorded = { operation: string; sse: boolean; variables: Record<string, unknown> };

const COMPLETED_RUN = {
  id: 'run-1',
  sessionId: 'session-1',
  sampleId: 'sample-1',
  status: 'completed',
  profileRevisionId: null,
  profileRevisionDigest: null,
  createdAt: '2026-01-01T09:00:00Z',
  completedAt: '2026-01-01T09:00:01Z',
  stages: [],
  diagnostics: [],
  lineage: [],
  events: [],
  warnings: []
};

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'content-type': 'application/json' } });
}

/** An SSE response that opens and then stays silent (no events, never ends). */
function openStream(): Response {
  return new Response(new ReadableStream<Uint8Array>(), {
    status: 200,
    headers: { 'content-type': 'text/event-stream' }
  });
}

/**
 * A stand-in for `fi-fhir serve` with the session engine on. `stream` decides
 * how the SSE subscription answers; `failSampleOnCall` makes the Nth
 * AddStreamingSessionSample a network failure.
 */
function fakeApi(options: { stream: 'open' | 'never'; failSampleOnCall?: number }) {
  const requests: Recorded[] = [];
  let sampleCalls = 0;
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    if (String(input) !== '/graphql') return new Response('not found', { status: 404 });

    const body = JSON.parse(String(init?.body ?? '{}')) as { query?: string; variables?: Record<string, unknown> };
    const operation = /\b(?:query|mutation|subscription)\s+(\w+)/.exec(body.query ?? '')?.[1] ?? '';
    const sse = new Headers(init?.headers).get('accept')?.includes('text/event-stream') ?? false;
    requests.push({ operation, sse, variables: body.variables ?? {} });

    if (sse) {
      if (options.stream === 'open') return openStream();
      // The stream never opens: the request hangs until the client aborts it.
      return new Promise<Response>((_, reject) => {
        init?.signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')));
      });
    }
    switch (operation) {
      case 'CreateStreamingIntegrationSession':
        return json({ data: { createIntegrationSession: { id: 'session-1' } } });
      case 'AddStreamingSessionSample':
        sampleCalls += 1;
        if (sampleCalls === options.failSampleOnCall) throw new TypeError('Failed to fetch');
        return json({ data: { addSessionSample: { id: `sample-${sampleCalls}`, sessionId: 'session-1' } } });
      case 'RunStreamingSessionPreview':
        return json({ data: { runSessionPreview: COMPLETED_RUN } });
      default:
        return json({ data: null, errors: [{ message: `unexpected operation ${operation}` }] });
    }
  });
  return { fetchMock, requests };
}

function progression(): HTMLElement | null {
  return screen.queryByRole('region', { name: 'Server preview progression' });
}

async function pressPreview(): Promise<void> {
  await fireEvent.click(screen.getAllByRole('button', { name: 'Preview' })[0]!);
}

beforeEach(() => {
  vi.stubEnv('VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED', 'true');
  vi.stubEnv('VITE_FI_FHIR_PREVIEW_INTEGRATION_ID', 'adt-east');
  setGraphQLTrustedNetworkAccess(true);
  setAccessStatus({
    authenticated: true,
    authVia: 'network',
    capabilities: {
      operatorRead: false,
      integrationSessions: true,
      streaming: true,
      subscriptions: ['integrationSessionEvents', 'sessionRunEvents']
    }
  });
});

afterEach(() => {
  vi.useRealTimers();
  cleanup();
  toasts.dismissAll();
  setGraphQLTrustedNetworkAccess(false);
  resetAccessCapabilities();
  resetObservedStreams();
  vi.unstubAllEnvs();
  vi.unstubAllGlobals();
});

// The first HL7PreviewPage render compiles the whole page (CodeMirror included).
describe('HL7 intake preview failures with the session engine on', { timeout: 30_000 }, () => {
  it('shows the error under a run that is still connecting when the stream never opens', async () => {
    const api = fakeApi({ stream: 'never' });
    vi.stubGlobal('fetch', api.fetchMock);
    render(HL7PreviewPage);

    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] });
    await pressPreview();

    // The session exists and the panel is listening; the stream has not opened.
    await vi.waitFor(() => expect(progression()).toHaveClass('state-connecting'));
    expect(api.requests.some((request) => request.sse)).toBe(true);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();

    // waitForStreamOpen gives up after 5 s.
    await vi.advanceTimersByTimeAsync(5_000);

    const alert = await vi.waitFor(() => screen.getByRole('alert'));
    expect(alert).toHaveTextContent('Integration Session stream timed out');
    // The honest progression region stays, still in its connecting state.
    expect(progression()).toHaveClass('state-connecting');
    expect(within(progression()!).getByText('Connecting to server diagnostics')).toBeInTheDocument();
    expect(api.requests.map((request) => request.operation)).not.toContain('RunStreamingSessionPreview');
  });

  it('drops the previous "Preview complete" when the next run fails before any session update', async () => {
    const api = fakeApi({ stream: 'open', failSampleOnCall: 2 });
    vi.stubGlobal('fetch', api.fetchMock);
    render(HL7PreviewPage);

    await pressPreview();
    await vi.waitFor(() => expect(progression()).toHaveClass('state-complete'), { timeout: 5_000 });
    expect(progression()).toHaveTextContent('Preview complete');

    // Second run: adding the sample fails at the network, before the run
    // reports anything.
    await pressPreview();
    await vi.waitFor(() => expect(screen.getByText('Failed to fetch')).toBeInTheDocument(), { timeout: 5_000 });

    expect(progression()).not.toBeInTheDocument();
    expect(screen.queryByText('Preview complete')).not.toBeInTheDocument();
    expect(screen.getByText('Preview failed')).toBeInTheDocument();

    // The server session is still reused: one create, and the failed second
    // sample was added to the same session.
    const operations = api.requests.map((request) => request.operation);
    expect(operations.filter((operation) => operation === 'CreateStreamingIntegrationSession')).toHaveLength(1);
    const samples = api.requests.filter((request) => request.operation === 'AddStreamingSessionSample');
    expect(samples).toHaveLength(2);
    expect(samples[1]!.variables).toMatchObject({ input: { sessionId: 'session-1' } });
  });
});
