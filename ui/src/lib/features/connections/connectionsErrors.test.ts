import { describe, expect, it } from 'vitest';
import { GraphQLResponseError } from '$lib/graphql/client';
import {
  SPEC_REJECTED_MESSAGE,
  describeConnectionFailure,
  describeRefusal,
  specRejectionProblems
} from './connectionsErrors';

describe('describeConnectionFailure', () => {
  it.each([
    ['connection version conflict', /saved first\. Reload/, true],
    ['connection is archived', /archived and accepts no change/, true],
    ['connection already exists', /already exists\. Choose another ID/, false],
    ['connection not found', /not available in this tenant/, true],
    ['invalid connection catalog request', /rejected as invalid/, false],
    ['connection catalog unavailable', /FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true/, false],
    ['engine runtime unavailable', /Only fi-fhir serve reports what it mounted/, false],
    ['authentication required', /no longer authenticated/, false],
    ['connection catalog request failed', /could not complete that request/, false]
  ])('maps "%s"', (message, expected, staleView) => {
    const failure = describeConnectionFailure(new Error(message));
    expect(failure.message).toMatch(expected);
    expect(failure.staleView).toBe(staleView);
  });

  it('names the roles to grant and never renders the word "forbidden"', () => {
    for (const message of ['connection catalog action forbidden', 'GraphQL operation forbidden', 'something forbidden']) {
      const failure = describeConnectionFailure(new Error(message));
      expect(failure.message).not.toMatch(/forbidden/i);
      expect(failure.message).toContain('integration.operator');
      expect(failure.message).toContain('integration.deployment.operator');
    }
  });

  it('passes an unknown message through and falls back when there is none', () => {
    expect(describeConnectionFailure(new Error('GraphQL HTTP 502')).message).toBe('GraphQL HTTP 502');
    expect(describeConnectionFailure(undefined).message).toMatch(/could not complete/);
  });
});

describe('specRejectionProblems', () => {
  const problems = [
    { code: 'SECRET_VALUE_FORBIDDEN', path: 'tls.password', message: 'a spec never carries a secret value' },
    { code: 'UNKNOWN_FIELD', path: 'timeouts.linger', message: 'is not a field of this connection kind' }
  ];

  it('returns every refused path from extensions.problems, whatever its code', () => {
    const error = new GraphQLResponseError(SPEC_REJECTED_MESSAGE, [
      { message: SPEC_REJECTED_MESSAGE, extensions: { code: 'SECRET_VALUE_FORBIDDEN', problems } }
    ]);
    expect(specRejectionProblems(error)).toEqual(problems);
    expect(describeRefusal(error, 2)).toMatch(/Nothing was saved/);
  });

  it('reads extensions.problems even under another message', () => {
    const error = new GraphQLResponseError('connection spec rejected', [
      { message: 'connection spec rejected', extensions: { code: 'UNKNOWN_FIELD', problems: [problems[1]] } }
    ]);
    expect(specRejectionProblems(error)).toEqual([problems[1]]);
    expect(describeRefusal(error, 1)).toBe(
      'Nothing was saved: the catalog refused 1 field of the spec. They are marked on the form.'
    );
  });

  it('is null for any other failure', () => {
    expect(specRejectionProblems(new Error(SPEC_REJECTED_MESSAGE))).toBeNull();
    expect(
      specRejectionProblems(new GraphQLResponseError('connection version conflict', [{ message: 'connection version conflict' }]))
    ).toBeNull();
  });
});
