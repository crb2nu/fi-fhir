import { afterEach, describe, expect, it } from 'vitest';
import {
  GraphQLCredentialsUnavailableError,
  requireGraphQLAuthorization,
  setGraphQLCredentialProvider
} from './credentials';

afterEach(() => {
  setGraphQLCredentialProvider(null);
});

describe('GraphQL runtime credentials', () => {
  it('fails closed when no provider is installed', async () => {
    await expect(requireGraphQLAuthorization()).rejects.toBeInstanceOf(
      GraphQLCredentialsUnavailableError
    );
  });

  it('resolves a fresh token for every request', async () => {
    let generation = 0;
    setGraphQLCredentialProvider(async () => `token-${++generation}`);

    await expect(requireGraphQLAuthorization()).resolves.toBe('Bearer token-1');
    await expect(requireGraphQLAuthorization()).resolves.toBe('Bearer token-2');
  });

  it.each([null, undefined, '', '   ', 'token\nheader'])('rejects unusable token %j', async (token) => {
    setGraphQLCredentialProvider(() => token);

    await expect(requireGraphQLAuthorization()).rejects.toBeInstanceOf(
      GraphQLCredentialsUnavailableError
    );
  });
});
