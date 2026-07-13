/**
 * GraphQL subscriptions are intentionally unavailable during the authenticated
 * preview phase. The backend exposes only bounded POST operations; opening a
 * WebSocket would bypass that transport boundary.
 */
import type { TypedDocumentNode } from '@graphql-typed-document-node/core';
import { browser } from '$app/environment';

export class GraphQLSubscriptionsUnavailableError extends Error {
  constructor() {
    super('GraphQL subscriptions are unavailable in authenticated preview mode');
    this.name = 'GraphQLSubscriptionsUnavailableError';
  }
}

export type SubscriptionCallbacks<TData> = {
  onData: (data: TData) => void;
  onError?: (error: Error) => void;
  onComplete?: () => void;
};

/**
 * Fails locally without resolving credentials, opening a socket, or retrying.
 * Consumers receive the normal subscription error callback and can render an
 * explicit unavailable state while preview remains POST-only.
 */
export function subscribe<TData, TVariables>(
  _document: TypedDocumentNode<TData, TVariables>,
  _variables: TVariables,
  callbacks: SubscriptionCallbacks<TData>
): () => void {
  if (browser) {
    queueMicrotask(() => {
      callbacks.onError?.(new GraphQLSubscriptionsUnavailableError());
    });
  }
  return () => {};
}

/** Kept as a stable lifecycle hook for the credential gate. */
export function disposeClient(): Promise<void> {
  return Promise.resolve();
}
