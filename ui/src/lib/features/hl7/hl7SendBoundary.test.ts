/**
 * Lane R-E: HL7 intake sends CR-terminated segments whatever line endings the
 * editor holds. CodeMirror joins lines with LF, pasted and loaded files carry
 * LF or CRLF, and the preview kernel parses with strict validation, which
 * rejects any LF ("strict validation requires CR segment terminators") unless
 * a source profile opts into tolerant line endings.
 *
 * Only `fetch` is faked: the real GraphQL and SSE clients build each request,
 * so the assertions read the variables that went over the wire.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { tick } from 'svelte';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { EditorView } from '@codemirror/view';
import { getHL7Value } from '$lib/domain/hl7Access';
import { parseHL7Path } from '$lib/domain/hl7Path';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import { setGraphQLTrustedNetworkAccess } from '$lib/graphql/credentials';
import { resetObservedStreams } from '$lib/graphql/streamAvailability';
import { toasts } from '$lib/ui/toastStore';
import HL7PreviewPage from './HL7PreviewPage.svelte';
import { parseHL7Preview } from './hl7Preview';
import { createHL7PreviewStore } from './hl7PreviewStore';
import { submitHL7Message } from './hl7Submit';

/** A synthetic ADT^A01 in the executable A01 subset; every identifier is a placeholder. */
const SEGMENTS = [
  'MSH|^~\\&|TEST_APP|TEST_FACILITY|FI_FHIR|TEST_FACILITY|20260101090000||ADT^A01|NEWLINE-TEST-001|T|2.5.1',
  'EVN|A01|20260101090000',
  'PID|1||SYNTHETIC-0001^^^TEST^MR||SYNTHETIC^PATIENT||20000101|U',
  'PV1|1|I|TEST_WARD^TEST_ROOM^TEST_BED'
];
const CR_MESSAGE = SEGMENTS.join('\r');
const LF_MESSAGE = SEGMENTS.join('\n');
const NON_CR_FORMS = [
  ['LF', LF_MESSAGE],
  ['CRLF', SEGMENTS.join('\r\n')],
  ['mixed CR, LF and CRLF', `${SEGMENTS[0]}\r${SEGMENTS[1]}\n${SEGMENTS[2]}\r\n${SEGMENTS[3]}`]
] as const;

type Recorded = { operation: string; sse: boolean; variables: Record<string, unknown> };

function json(body: unknown): Response {
  return new Response(JSON.stringify(body), { status: 200, headers: { 'content-type': 'application/json' } });
}

/** A stateless preview result that passes the client's provenance checks. */
function statelessPreview(correlationId: string) {
  const digest = (character: string) => `sha256:${character.repeat(64)}`;
  return {
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
        sourceMessageId: 'NEWLINE-TEST-001',
        correlationId,
        classification: 'phi',
        payload: {
          id: 'event-1',
          type: 'patient_admit',
          timestamp: '2026-01-01T09:00:00Z',
          source: 'adt-east',
          source_format: 'hl7v2',
          source_profile_id: 'profile-adt',
          source_message_id: 'NEWLINE-TEST-001',
          correlation_id: correlationId,
          patient: { mrn: 'SYNTHETIC-0001', family_name: 'SYNTHETIC', given_name: 'PATIENT', gender: 'U' },
          encounter: { class: 'I', location: { facility: null, unit: 'TEST_WARD', room: 'TEST_ROOM', bed: 'TEST_BED' } }
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
      sourceMessageId: 'NEWLINE-TEST-001',
      eventIds: ['event-1'],
      workflowRunId: null
    }
  };
}

const SESSION_RUN = {
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

/** An in-process stand-in for `fi-fhir serve` that records every GraphQL request. */
function fakeApi() {
  const requests: Recorded[] = [];
  const fetchMock = vi.fn(async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    if (String(input) !== '/graphql') return new Response('not found', { status: 404 });

    const body = JSON.parse(String(init?.body ?? '{}')) as { query?: string; variables?: Record<string, unknown> };
    const operation = /\b(?:query|mutation|subscription)\s+(\w+)/.exec(body.query ?? '')?.[1] ?? '';
    const sse = new Headers(init?.headers).get('accept')?.includes('text/event-stream') ?? false;
    requests.push({ operation, sse, variables: body.variables ?? {} });

    if (sse) return new Response('', { status: 200, headers: { 'content-type': 'text/event-stream' } });
    const correlationId = (body.variables?.input as { correlationId?: string } | undefined)?.correlationId ?? '';
    switch (operation) {
      case 'PreviewIntegrationMessage':
        return json({ data: { previewIntegrationMessage: statelessPreview(correlationId) } });
      case 'CreateStreamingIntegrationSession':
        return json({ data: { createIntegrationSession: { id: 'session-1' } } });
      case 'AddStreamingSessionSample':
        return json({ data: { addSessionSample: { id: 'sample-1', sessionId: 'session-1' } } });
      case 'RunStreamingSessionPreview':
        return json({ data: { runSessionPreview: SESSION_RUN } });
      case 'SubmitMessage':
        return json({
          data: { submitMessage: { success: true, eventId: 'event-1', errors: [], warnings: [], workflowResults: [] } }
        });
      default:
        return json({ data: null, errors: [{ message: `unexpected operation ${operation}` }] });
    }
  });

  /** The `input.data` of every request for `operation`. */
  function sentData(operation: string): unknown[] {
    return requests
      .filter((request) => request.operation === operation)
      .map((request) => (request.variables.input as { data?: unknown } | undefined)?.data);
  }

  return { fetchMock, requests, sentData };
}

/** R-A's `/api/auth/status` contract with the session workspace on or off. */
function authStatus(integrationSessions: boolean) {
  return {
    authenticated: true,
    authVia: 'network',
    capabilities: {
      operatorRead: false,
      integrationSessions,
      streaming: integrationSessions,
      subscriptions: integrationSessions ? ['integrationSessionEvents', 'sessionRunEvents'] : []
    }
  };
}

let api: ReturnType<typeof fakeApi>;

beforeEach(() => {
  api = fakeApi();
  vi.stubGlobal('fetch', api.fetchMock);
  vi.stubEnv('VITE_FI_FHIR_PREVIEW_INTEGRATION_ID', 'adt-east');
  vi.stubEnv('VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED', 'false');
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
});

describe('built-in HL7 sample', () => {
  it('is CR-terminated and parses to the executable A01 subset in the UI HL7 helpers', () => {
    const store = createHL7PreviewStore();
    const data = get(store.state).data;

    // Real carriage returns: not the two characters backslash and "r", not LF.
    expect(data).not.toContain('\\r');
    expect(data).not.toContain('\n');
    expect(data.split('\r')).toHaveLength(4);

    const message = get(store.hl7);
    expect(message.segments.map((segment) => segment.id)).toEqual(['MSH', 'EVN', 'PID', 'PV1']);
    // One escape backslash: `^~\&`, not `^~\\&`, which the kernel rejects as a
    // malformed delimiter declaration.
    expect(message.delimiters).toEqual({ field: '|', component: '^', repetition: '~', escape: '\\', subcomponent: '&' });
    expect(getHL7Value(message, parseHL7Path('MSH-2'))).toBe('^~\\&');
    expect(getHL7Value(message, parseHL7Path('MSH-9'))).toBe('ADT^A01');
    expect(getHL7Value(message, parseHL7Path('PID-5'))).toBe('DOE^JOHN');
    expect(getHL7Value(message, parseHL7Path('PV1-3'))).toBe('ICU^101^A^HOSPITAL');
  });
});

describe('stateless preview (previewIntegrationMessage)', () => {
  it.each(NON_CR_FORMS)('sends %s editor text as CR-separated segments', async (_form, editorText) => {
    await parseHL7Preview({ data: editorText, correlationId: 'correlation-123' });

    expect(api.sentData('PreviewIntegrationMessage')).toEqual([CR_MESSAGE]);
  });

  it('sends CR-separated text unchanged', async () => {
    await parseHL7Preview({ data: CR_MESSAGE, correlationId: 'correlation-123' });

    expect(api.sentData('PreviewIntegrationMessage')).toEqual([CR_MESSAGE]);
  });
});

describe('Integration Session run', () => {
  it.each(NON_CR_FORMS)('adds %s editor text to the session as CR-separated segments', async (_form, editorText) => {
    vi.stubEnv('VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED', 'true');
    setAccessStatus(authStatus(true));

    await parseHL7Preview({ data: editorText, onSessionUpdate: () => {} });

    // The session engine carried the run, and its only payload is the sample.
    expect(api.requests.map((request) => request.operation)).toEqual([
      'CreateStreamingIntegrationSession',
      'AddStreamingSessionSample',
      'StreamIntegrationSessionEvents',
      'RunStreamingSessionPreview'
    ]);
    expect(api.sentData('AddStreamingSessionSample')).toEqual([CR_MESSAGE]);
    expect(api.sentData('RunStreamingSessionPreview')).toEqual([null]);
  });
});

describe('process (submitMessage)', () => {
  it.each(NON_CR_FORMS)('submits %s editor text as CR-separated segments', async (_form, editorText) => {
    await submitHL7Message({ source: 'ui_preview', data: editorText, correlationId: 'correlation-123' });

    expect(api.sentData('SubmitMessage')).toEqual([CR_MESSAGE]);
  });
});

// The first HL7PreviewPage render compiles the whole page (CodeMirror included).
describe('HL7 intake page', { timeout: 30_000 }, () => {
  function editorView(): EditorView {
    const view = EditorView.findFromDOM(screen.getByTestId('code-editor'));
    if (!view) throw new Error('HL7 intake editor is not mounted');
    return view;
  }

  async function pressPreview(): Promise<void> {
    await fireEvent.click(screen.getAllByRole('button', { name: 'Preview' })[0]!);
    await waitFor(() => expect(api.sentData('PreviewIntegrationMessage')).toHaveLength(1));
  }

  it('previews the built-in sample as it stands, with CR-separated segments', async () => {
    render(HL7PreviewPage);

    await pressPreview();

    const [sent] = api.sentData('PreviewIntegrationMessage');
    expect(sent).toBe(get(createHL7PreviewStore().state).data);
    expect(String(sent).split('\r').map((segment) => segment.slice(0, 3))).toEqual(['MSH', 'EVN', 'PID', 'PV1']);
  });

  it('previews text typed into the editor (LF-joined) as CR-separated segments', async () => {
    render(HL7PreviewPage);
    const view = editorView();
    view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: LF_MESSAGE } });
    await tick();

    await pressPreview();

    expect(api.sentData('PreviewIntegrationMessage')).toEqual([CR_MESSAGE]);
  });
});
