import { afterEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import {
  accessCapabilities,
  resetAccessCapabilities,
  setAccessStatus
} from '$lib/graphql/accessCapabilities';
import { GraphQLResponseError } from '$lib/graphql/client';
import { setGraphQLTrustedNetworkAccess } from '$lib/graphql/credentials';
import { describeIntakeFailure, problemGuidance } from './intakeErrors';
import { intakeEntry } from './intakeState';

function gqlError(message: string, extensions?: Record<string, unknown>): GraphQLResponseError {
  return new GraphQLResponseError(message, [extensions ? { message, extensions } : { message }]);
}

describe('describeIntakeFailure', () => {
  it('names the fix for SOURCE_UNAVAILABLE from extensions.code', () => {
    const failure = describeIntakeFailure(
      gqlError('capture source unavailable: no mounted or compiled MLLP or HTTP source has this id', {
        code: 'SOURCE_UNAVAILABLE'
      })
    );
    expect(failure.kind).toBe('source-unavailable');
    expect(failure.message).toContain('FI_FHIR_MLLP_SOURCE_CONFIG_PATH');
    expect(failure.message).toContain('compile a stream source connection');
    expect(failure.message).toContain('Browse objects');
  });

  it.each([
    ['a capture is already armed for this source', 'already-armed', /Cancel it from its capture row/],
    ['connection sample intake unavailable', 'session', /FI_FHIR_INTEGRATION_SESSION_ENABLED/],
    ['integration session not found', 'session', /Reload the page/],
    ['integration session is archived', 'session', /archived/],
    ['peek requires a compiled batch source connection', 'other', /Compile it in Connections/],
    ['invalid connection sample intake request', 'other', /1–1024 bytes/],
    ['connection capture is already finished', 'other', /already finished/]
  ] as const)('%s', (message, kind, pattern) => {
    const failure = describeIntakeFailure(gqlError(message));
    expect(failure.kind).toBe(kind);
    expect(failure.message).toMatch(pattern);
  });

  it('falls back to the catalog wording, and never renders the word forbidden', () => {
    expect(describeIntakeFailure(gqlError('connection not found')).message).toMatch(/not available in this tenant/);
    const forbidden = describeIntakeFailure(gqlError('GraphQL operation forbidden')).message;
    expect(forbidden).toContain('integration.operator');
    expect(forbidden.toLowerCase()).not.toContain('forbidden');
  });
});

describe('the legacy-execution refusal', () => {
  function status(sessionsOn: boolean) {
    return {
      authenticated: true,
      authVia: 'network',
      capabilities: { operatorRead: true, integrationSessions: sessionsOn, connectionsRead: true }
    };
  }

  afterEach(() => {
    resetAccessCapabilities();
    setGraphQLTrustedNetworkAccess(false);
    vi.unstubAllGlobals();
  });

  it('re-probes the status once for a burst, and intake re-derives to hidden', async () => {
    setGraphQLTrustedNetworkAccess(true);
    setAccessStatus(status(true));
    expect(intakeEntry(get(accessCapabilities), true)).toEqual({ visible: true, disabledReason: null });
    const fetchMock = vi.fn().mockResolvedValue({ ok: true, json: async () => status(false) } as Response);
    vi.stubGlobal('fetch', fetchMock);

    const refusal = gqlError('legacy integration execution is unavailable');
    for (let attempt = 0; attempt < 3; attempt += 1) {
      // The honest state is unchanged until the new status arrives.
      expect(describeIntakeFailure(refusal).kind).toBe('other');
    }

    await vi.waitFor(() => expect(intakeEntry(get(accessCapabilities), true)).toEqual({ visible: false }));
    expect(fetchMock).toHaveBeenCalledTimes(1);
    expect(fetchMock).toHaveBeenCalledWith('/api/auth/status', expect.objectContaining({ cache: 'no-store' }));
  });

  it('does not probe for any other refusal', () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    setGraphQLTrustedNetworkAccess(true);
    describeIntakeFailure(gqlError('connection not found'));
    describeIntakeFailure(gqlError('a capture is already armed for this source'));
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe('problemGuidance', () => {
  it('explains where a peek resolves secrets', () => {
    expect(problemGuidance('SECRET_UNRESOLVABLE')).toContain('FI_FHIR_CONNECTION_SECRET_*');
    expect(problemGuidance('SECRET_UNRESOLVABLE')).toContain('connections/');
  });

  it('has nothing to add for an unknown code', () => {
    expect(problemGuidance('SOMETHING_NEW')).toBe('');
  });
});
