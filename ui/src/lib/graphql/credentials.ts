export type GraphQLCredentialProvider = () =>
  | string
  | null
  | undefined
  | Promise<string | null | undefined>;

/** Raised before a request is sent when no runtime credential is available. */
export class GraphQLCredentialsUnavailableError extends Error {
  constructor() {
    super('GraphQL credentials unavailable');
    this.name = 'GraphQLCredentialsUnavailableError';
  }
}

let credentialProvider: GraphQLCredentialProvider | null = null;

/**
 * Installs the runtime access-token provider owned by the authentication shell.
 *
 * Tokens must not come from a PUBLIC_* build variable. The provider is invoked
 * for every HTTP request so it can refresh or revoke credentials without
 * rebuilding the UI. WebSocket transport is disabled in preview mode.
 */
export function setGraphQLCredentialProvider(provider: GraphQLCredentialProvider | null): void {
  credentialProvider = provider;
}

/** Returns a validated Bearer authorization value or fails closed. */
export async function requireGraphQLAuthorization(): Promise<string> {
  if (!credentialProvider) {
    throw new GraphQLCredentialsUnavailableError();
  }

  const token = (await credentialProvider())?.trim() ?? '';
  if (!token || /[\r\n]/.test(token)) {
    throw new GraphQLCredentialsUnavailableError();
  }

  return `Bearer ${token}`;
}

export function isGraphQLCredentialsUnavailable(
  error: unknown
): error is GraphQLCredentialsUnavailableError {
  return error instanceof GraphQLCredentialsUnavailableError;
}
