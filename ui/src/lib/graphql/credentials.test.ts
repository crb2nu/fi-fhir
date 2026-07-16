import { afterEach, describe, expect, it } from 'vitest';
import {
  GraphQLCredentialsUnavailableError,
  requireGraphQLAuthorization,
  setGraphQLCredentialProvider,
  setGraphQLTrustedNetworkAccess
} from './credentials';

afterEach(() => {
  setGraphQLCredentialProvider(null);
  setGraphQLTrustedNetworkAccess(false);
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

  it('allows an explicit trusted-network request without an authorization header', async () => {
    setGraphQLTrustedNetworkAccess(true);

    await expect(requireGraphQLAuthorization()).resolves.toBeNull();
  });
});
