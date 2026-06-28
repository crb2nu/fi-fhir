import { afterAll, beforeEach, describe, expect, it, vi } from 'vitest';
import type { DocumentNode, DefinitionNode } from 'graphql';
import { graphqlFetch } from '$lib/graphql/client';
import { subscribe } from '$lib/graphql/subscriptions';
import { ParsePreviewDocument } from '$lib/gen/graphql';
import { parseHL7Preview } from '$lib/features/hl7/hl7Preview';
import { subscribeSessionRunEvents } from './api';

vi.mock('$lib/graphql/client', () => ({
  graphqlFetch: vi.fn()
}));

vi.mock('$lib/graphql/subscriptions', () => ({
  subscribe: vi.fn()
}));

const mockFetch = graphqlFetch as unknown as ReturnType<typeof vi.fn>;
const mockSubscribe = subscribe as unknown as ReturnType<typeof vi.fn>;
const env = import.meta.env as Record<string, string | undefined>;
const originalFlag = env.PUBLIC_INTEGRATION_SESSION_ENGINE;

const preview = {
  __typename: 'ParseResult' as const,
  success: true,
  errors: [],
  events: [],
  warnings: [
    {
      __typename: 'ParseWarning' as const,
      phase: 'syntactic',
      code: 'TRAILING_FIELD',
      message: 'Trailing empty field',
      path: 'PID-3',
      explanation: null,
      fixSuggestion: null,
      impact: null,
      severity: 'info',
      fromCache: null
    }
  ]
};

function operationName(document: unknown): string {
  const doc = document as DocumentNode;
  const operation = doc.definitions.find((d: DefinitionNode) => d.kind === 'OperationDefinition');
  return operation && 'name' in operation && operation.name ? operation.name.value : '';
}

beforeEach(() => {
  env.PUBLIC_INTEGRATION_SESSION_ENGINE = undefined;
  mockFetch.mockReset();
  mockSubscribe.mockReset();
});

afterAll(() => {
  env.PUBLIC_INTEGRATION_SESSION_ENGINE = originalFlag;
});

describe('parseHL7Preview session routing', () => {
  it('uses the direct parse preview query by default', async () => {
    mockFetch.mockResolvedValue({ parsePreview: preview });

    const result = await parseHL7Preview({
      source: 'adt_feed',
      data: 'MSH|^~\\\\&|A|B|C|D|20240101||ADT^A01|1|P|2.5'
    });

    expect(result.parsePreview).toBe(preview);
    expect(result.session).toBeUndefined();
    expect(mockFetch).toHaveBeenCalledTimes(1);
    expect(mockFetch).toHaveBeenCalledWith(ParsePreviewDocument, {
      format: 'HL7V2',
      data: 'MSH|^~\\\\&|A|B|C|D|20240101||ADT^A01|1|P|2.5',
      source: 'adt_feed'
    });
  });

  it('creates a session, adds a sample, and runs session preview when enabled', async () => {
    env.PUBLIC_INTEGRATION_SESSION_ENGINE = '1';
    mockFetch.mockImplementation(async (document: unknown) => {
      switch (operationName(document)) {
        case 'CreateIntegrationSession':
          return { createIntegrationSession: { id: 'session-1', source: 'adt_feed', profileId: null, title: 'adt_feed HL7 preview' } };
        case 'AddSessionSample':
          return { addSessionSample: { id: 'sample-1', source: 'adt_feed', checksum: 'sha256:1', rawRetained: false } };
        case 'RunSessionPreview':
          return {
            runSessionPreview: {
              id: 'run-1',
              state: 'completed',
              preview,
              diagnostics: [
                {
                  id: 'diag-1',
                  phase: 'semantic',
                  code: 'PID_NAME',
                  message: 'Patient name missing',
                  path: 'PID-5',
                  severity: 'warning',
                  status: 'open',
                  fixSuggestion: 'Map PID-5'
                }
              ],
              stages: [{ id: 'stage-1', name: 'parse', state: 'completed', startedAt: null, completedAt: null, durationMs: 3 }]
            }
          };
        default:
          throw new Error(`unexpected operation ${operationName(document)}`);
      }
    });

    const result = await parseHL7Preview({
      source: 'adt_feed',
      data: 'MSH|^~\\\\&|A|B|C|D|20240101||ADT^A01|1|P|2.5',
      profileId: 'profile-1'
    });

    expect(result.parsePreview).toBe(preview);
    expect(result.session).toMatchObject({
      mode: 'session',
      id: 'session-1',
      sampleId: 'sample-1',
      runId: 'run-1',
      state: 'completed'
    });
    expect(result.session?.diagnostics).toHaveLength(1);
    expect(mockFetch.mock.calls.map((call) => operationName(call[0]))).toEqual([
      'CreateIntegrationSession',
      'AddSessionSample',
      'RunSessionPreview'
    ]);
    expect(mockFetch.mock.calls[0]?.[1]).toMatchObject({
      input: { source: 'adt_feed', profileId: 'profile-1' }
    });
    expect(mockFetch.mock.calls[1]?.[1]).toMatchObject({
      sessionId: 'session-1',
      input: { source: 'adt_feed', format: 'HL7V2' }
    });
    expect(mockFetch.mock.calls[2]?.[1]).toMatchObject({
      sessionId: 'session-1',
      sampleId: 'sample-1',
      input: { profileId: 'profile-1' }
    });
  });

  it('falls back to direct parse if the session flow is unavailable', async () => {
    env.PUBLIC_INTEGRATION_SESSION_ENGINE = '1';
    mockFetch
      .mockRejectedValueOnce(new Error('session resolver unavailable'))
      .mockResolvedValueOnce({ parsePreview: preview });

    const result = await parseHL7Preview({
      source: 'adt_feed',
      data: 'MSH|^~\\\\&|A|B|C|D|20240101||ADT^A01|1|P|2.5'
    });

    expect(result.parsePreview).toBe(preview);
    expect(result.session).toMatchObject({
      mode: 'fallback',
      state: 'fallback',
      error: 'session resolver unavailable'
    });
    expect(mockFetch.mock.calls.map((call) => operationName(call[0]))).toEqual([
      'CreateIntegrationSession',
      'ParsePreview'
    ]);
  });
});

describe('subscribeSessionRunEvents', () => {
  it('delegates to the GraphQL subscription client', () => {
    const unsubscribe = vi.fn();
    mockSubscribe.mockReturnValue(unsubscribe);
    const onData = vi.fn();
    const onError = vi.fn();

    const result = subscribeSessionRunEvents('session-1', 'run-1', { onData, onError });

    expect(result).toBe(unsubscribe);
    expect(mockSubscribe).toHaveBeenCalledTimes(1);
    expect(operationName(mockSubscribe.mock.calls[0]?.[0])).toBe('SessionRunEvents');
    expect(mockSubscribe.mock.calls[0]?.[1]).toEqual({ sessionId: 'session-1', runId: 'run-1' });
  });
});
