import { describe, expect, it } from 'vitest';
import { GraphQLResponseError } from '$lib/graphql/client';
import { describeIntakeFailure, problemGuidance } from './intakeErrors';

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

describe('problemGuidance', () => {
  it('explains where a peek resolves secrets', () => {
    expect(problemGuidance('SECRET_UNRESOLVABLE')).toContain('FI_FHIR_CONNECTION_SECRET_*');
    expect(problemGuidance('SECRET_UNRESOLVABLE')).toContain('connections/');
  });

  it('has nothing to add for an unknown code', () => {
    expect(problemGuidance('SOMETHING_NEW')).toBe('');
  });
});
