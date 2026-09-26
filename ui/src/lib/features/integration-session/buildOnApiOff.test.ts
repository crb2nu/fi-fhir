/**
 * Negative path for Lane R-C: the UI image is built with the session engine on
 * (`VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED=true`, the `ui/Dockerfile`
 * default) while the API still has `FI_FHIR_INTEGRATION_SESSION_ENABLED` unset.
 * That is production between the image rollout and the GitOps env flip, and
 * again after a rollback of the flip.
 *
 * R-B's unit tests pin `resolveIntegrationSessionEngine` and each surface with
 * the neighbouring modules mocked. This suite fakes only `fetch`: the real
 * credential gate reads `/api/auth/status`, the real capability store and
 * engine gate decide, the real GraphQL client and SSE client talk to an
 * in-process stand-in for the API, and the real toast net records anything it
 * would have shown. Each negative case has a positive control (API sessions
 * on) so the assertions can fail for the reason they exist.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import GraphQLCredentialGate from '$lib/graphql/GraphQLCredentialGate.svelte';
import { resetAccessCapabilities } from '$lib/graphql/accessCapabilities';
import { resetObservedStreams } from '$lib/graphql/streamAvailability';
import HL7PreviewPage from '$lib/features/hl7/HL7PreviewPage.svelte';
import DryRunPanel from '$lib/features/workflows/components/DryRunPanel.svelte';
import { toasts } from '$lib/ui/toastStore';

/** The production operator bundle (R-0) — what every production identity holds. */
const PRODUCTION_ROLES = [
  'integration:preview',
  'graphql:operator',
  'clinical:read',
  'integration.operator',
  'integration.delivery.operator',
  'integration.deployment.operator'
];

/** What the API answers for any session operation while its workspace is off. */
const SESSIONS_OFF_ERROR = 'legacy integration execution is unavailable';

/** R-A's `/api/auth/status` body for a trusted-network caller (`deriveAuthStatus`). */
function authStatus(sessionsOn: boolean) {
  return {
    authenticated: true,
    authVia: 'network',
    principal: 'fi-fhir-ide-operator',
    roles: PRODUCTION_ROLES,
    capabilities: {
      operatorRead: true,
      operatorDelivery: true,
      operatorDeployment: true,
      clinicalRead: true,
      integrationSessions: sessionsOn,
      streaming: sessionsOn,
      subscriptions: sessionsOn ? ['integrationSessionEvents', 'sessionRunEvents'] : [],
      llm: { configured: true }
    },
    missingRoles: { operatorRead: [], operatorDelivery: [], operatorDeployment: [], clinicalRead: [] }
  };
}

/** A stateless preview result that passes the client's provenance checks. */
function statelessPreview(correlationId: string) {
  const digest = (character: string) => `sha256:${character.repeat(64)}`;
  return {
    __typename: 'IntegrationPreviewResult',
    mode: 'preview',
    tenantId: 'tenant-a',
    integrationRevision: { artifactId: 'integration-adt', revisionId: 'definition-1', digest: digest('a') },
    artifactRevisions: {
      source: { artifactId: 'source-adt', revisionId: 'source-1', digest: digest('b') },
      profile: { artifactId: 'profile-adt', revisionId: '1', digest: digest('c') },
      workflow: { artifactId: 'workflow-adt', revisionId: 'workflow-1', digest: digest('d') }
    },
    events: [
      {
        tenantId: 'tenant-a',
        id: 'event-1',
        type: 'patient_admit',
        sourceMessageId: 'MSG001',
        correlationId,
        classification: 'phi',
        payload: {
          id: 'event-1',
          type: 'patient_admit',
          timestamp: '2024-01-15T10:30:00Z',
          source: 'adt-east',
          source_format: 'hl7v2',
          source_profile_id: 'profile-adt',
          source_message_id: 'MSG001',
          correlation_id: correlationId,
          patient: { mrn: 'MRN123', family_name: 'DOE', given_name: 'JOHN', gender: 'M' },
          encounter: { class: 'I', location: { facility: 'HOSPITAL', unit: 'ICU', room: '101', bed: 'A' } }
        }
      }
    ],
    diagnostics: [],
    routes: [],
    deliveries: [],
    correlations: {
      tenantId: 'tenant-a',
      correlationId,
      traceId: null,
      sourceMessageId: 'MSG001',
      eventIds: ['event-1'],
      workflowRunId: null
    }
  };
}

type Recorded = { transport: 'graphql' | 'sse'; operation: string; variables: Record<string, unknown> };

function json(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), { status, headers: { 'content-type': 'application/json' } });
}

/**
 * An in-process stand-in for `fi-fhir serve` with the session workspace on or
 * off, answering the way `internal/api/graphql` does: SSE is a 404 while
 * streaming is off, and every session operation is refused with
 * {@link SESSIONS_OFF_ERROR}.
 */
function fakeApi(sessionsOn: boolean) {
  const requests: Recorded[] = [];
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const url = String(input);
    if (url === '/api/auth/status') return json(authStatus(sessionsOn));
    if (url !== '/graphql') return new Response('not found', { status: 404 });

    const body = JSON.parse(String(init?.body ?? '{}')) as { query?: string; variables?: Record<string, unknown> };
    const operation = /^\s*(?:query|mutation|subscription)\s+(\w+)/.exec(body.query ?? '')?.[1] ?? '';
    const sse = new Headers(init?.headers).get('accept')?.includes('text/event-stream') ?? false;
    requests.push({ transport: sse ? 'sse' : 'graphql', operation, variables: body.variables ?? {} });

    if (sse) {
      return sessionsOn
        ? new Response('', { status: 200, headers: { 'content-type': 'text/event-stream' } })
        : new Response('Integration Session streaming is unavailable\n', {
            status: 404,
            headers: { 'content-type': 'text/plain; charset=utf-8' }
          });
    }
    if (operation === 'Health') return json({ data: { health: { status: 'healthy', version: 'test' } } });
    if (operation === 'PreviewIntegrationMessage') {
      const input = body.variables?.input as { correlationId: string };
      return json({ data: { previewIntegrationMessage: statelessPreview(input.correlationId) } });
    }
    if (sessionsOn && operation === 'ListWorkflowSimulationSessions') {
      return json({ data: { integrationSessions: [] } });
    }
    // Session operations with the workspace off, and anything this flow should
    // never reach: the API's refusal, which the GraphQL client would toast.
    return json({ data: null, errors: [{ message: SESSIONS_OFF_ERROR }] });
  });
  return { fetchMock, requests };
}

/** Every error toast the global net raised during the test, even auto-dismissed ones. */
function recordErrorToasts(): { messages: string[]; stop: () => void } {
  const messages: string[] = [];
  const stop = toasts.subscribe(($state) => {
    for (const toast of $state.toasts) {
      if (toast.variant === 'error' && !messages.includes(toast.message)) messages.push(toast.message);
    }
  });
  return { messages, stop };
}

async function signInFromTrustedNetwork(): Promise<void> {
  render(GraphQLCredentialGate);
  await screen.findByText('Trusted network access active');
}

/** The authoring-flow rail and the editor toolbar both offer Preview; both call run(). */
async function pressPreview(): Promise<void> {
  await fireEvent.click(screen.getAllByRole('button', { name: 'Preview' })[0]!);
}

function sessionStreamNotice(): HTMLElement | undefined {
  return screen
    .queryAllByTestId('streaming-unavailable')
    .find((element) => element.getAttribute('data-stream') === 'integrationSessionEvents');
}

// The first HL7PreviewPage render compiles the whole page (CodeMirror included).
describe('UI built with the session engine on, API with sessions off', { timeout: 30_000 }, () => {
  let errorToasts: ReturnType<typeof recordErrorToasts>;

  beforeEach(() => {
    vi.stubEnv('VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED', 'true');
    vi.stubEnv('VITE_FI_FHIR_PREVIEW_INTEGRATION_ID', 'adt-east');
    errorToasts = recordErrorToasts();
  });

  afterEach(() => {
    cleanup();
    errorToasts.stop();
    toasts.dismissAll();
    resetAccessCapabilities();
    resetObservedStreams();
    vi.unstubAllEnvs();
    vi.unstubAllGlobals();
  });

  it('HL7 intake previews on the stateless path and says why the session stream is absent', async () => {
    const api = fakeApi(false);
    vi.stubGlobal('fetch', api.fetchMock);

    await signInFromTrustedNetwork();
    render(HL7PreviewPage);

    const notice = sessionStreamNotice();
    expect(notice).toBeDefined();
    expect(notice).toHaveAttribute('data-reason', 'streaming-off');
    expect(notice).toHaveTextContent('Live streaming for Integration Session runs is not available');
    expect(notice).toHaveTextContent('stateless path');

    await pressPreview();
    await screen.findByText('events: 1', undefined, { timeout: 5_000 });

    // (a) Exactly the stateless mutation after the gate's health probe: no
    // session mutation, no SSE subscription.
    expect(api.requests.map((request) => `${request.transport} ${request.operation}`)).toEqual([
      'graphql Health',
      'graphql PreviewIntegrationMessage'
    ]);
    expect(api.requests[1]!.variables).toMatchObject({ input: { integrationId: 'adt-east' } });

    // (b) The honest state stays, and nothing was toasted or shown as an error.
    expect(sessionStreamNotice()).toHaveAttribute('data-reason', 'streaming-off');
    expect(errorToasts.messages).toEqual([]);
    expect(screen.queryByText(new RegExp(SESSIONS_OFF_ERROR))).not.toBeInTheDocument();
  });

  it('control: with the API sessions on, the same click takes the session engine', async () => {
    const api = fakeApi(true);
    vi.stubGlobal('fetch', api.fetchMock);

    await signInFromTrustedNetwork();
    render(HL7PreviewPage);
    expect(sessionStreamNotice()).toBeUndefined();

    await pressPreview();
    await waitFor(() => expect(api.requests.length).toBeGreaterThan(1));

    const operations = api.requests.map((request) => request.operation);
    expect(operations[1]).toBe('CreateStreamingIntegrationSession');
    expect(operations).not.toContain('PreviewIntegrationMessage');
  });

  it('workflow dry run neither offers nor loads Integration Sessions', async () => {
    const api = fakeApi(false);
    vi.stubGlobal('fetch', api.fetchMock);

    await signInFromTrustedNetwork();
    render(DryRunPanel);

    expect(screen.getByRole('tab', { name: 'Presets' })).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: 'Session' })).not.toBeInTheDocument();
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(api.requests.map((request) => request.operation)).toEqual(['Health']);
    expect(errorToasts.messages).toEqual([]);
  });

  it('control: with the API sessions on, dry run offers and loads the Session source', async () => {
    const api = fakeApi(true);
    vi.stubGlobal('fetch', api.fetchMock);

    await signInFromTrustedNetwork();
    render(DryRunPanel);

    expect(await screen.findByRole('tab', { name: 'Session' })).toBeInTheDocument();
    await waitFor(() =>
      expect(api.requests.map((request) => request.operation)).toContain('ListWorkflowSimulationSessions')
    );
  });
});
