/**
 * WebSocket subscription client for GraphQL real-time events.
 *
 * Uses graphql-ws library which implements the graphql-transport-ws protocol.
 * The backend (gqlgen) supports this protocol for subscription handling.
 */
import { createClient, type Client } from 'graphql-ws';
import type { TypedDocumentNode } from '@graphql-typed-document-node/core';
import { print } from 'graphql';
import { browser } from '$app/environment';

/** Singleton WebSocket client instance */
let wsClient: Client | null = null;

/**
 * Gets or creates the WebSocket client.
 * Uses a lazy singleton pattern - the client is only created when first needed.
 *
 * The URL is constructed to work with Vite's proxy configuration which
 * forwards /graphql to the backend and has ws: true enabled.
 */
function getClient(): Client {
  if (!wsClient) {
    // Construct WebSocket URL from current origin
    // In development: ws://localhost:5173/graphql/ws (proxied to backend)
    // In production: wss://your-domain.com/graphql/ws
    const protocol = browser && window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const host = browser ? window.location.host : 'localhost:5173';
    const url = `${protocol}//${host}/graphql/ws`;

    wsClient = createClient({
      url,
      // Retry on connection loss with exponential backoff
      retryAttempts: Infinity,
      shouldRetry: () => true,
      // Connection params can be used for auth tokens if needed
      connectionParams: () => ({})
    });
  }
  return wsClient;
}

/**
 * Subscription callback types for handling real-time events.
 */
export type SubscriptionCallbacks<TData> = {
  /** Called when new data arrives from the subscription */
  onData: (data: TData) => void;
  /** Called when an error occurs (connection or protocol errors) */
  onError?: (error: Error) => void;
  /** Called when the subscription completes */
  onComplete?: () => void;
};

/**
 * Subscribes to a GraphQL subscription and returns an unsubscribe function.
 *
 * @param document - The typed GraphQL subscription document
 * @param variables - Variables to pass to the subscription
 * @param callbacks - Callbacks for handling data, errors, and completion
 * @returns A function to unsubscribe from the subscription
 *
 * @example
 * ```ts
 * import { EventStreamDocument } from '$lib/gen/graphql';
 *
 * const unsubscribe = subscribe(
 *   EventStreamDocument,
 *   { filter: { types: ['LAB_RESULT'] } },
 *   {
 *     onData: (data) => console.log('Event:', data.eventStream),
 *     onError: (err) => console.error('Subscription error:', err)
 *   }
 * );
 *
 * // Later: cleanup
 * unsubscribe();
 * ```
 */
export function subscribe<TData, TVariables>(
  document: TypedDocumentNode<TData, TVariables>,
  variables: TVariables,
  callbacks: SubscriptionCallbacks<TData>
): () => void {
  // Don't create WebSocket connections on the server
  if (!browser) {
    return () => {};
  }

  const client = getClient();

  const unsubscribe = client.subscribe<TData>(
    {
      query: print(document),
      variables: variables as Record<string, unknown>
    },
    {
      next: (result) => {
        if (result.data) {
          callbacks.onData(result.data);
        }
        if (result.errors?.length) {
          const msg = result.errors.map((e) => e.message ?? 'Unknown error').join('; ');
          callbacks.onError?.(new Error(msg));
        }
      },
      error: (err) => {
        // Handle both Error objects and CloseEvent
        if (err instanceof Error) {
          callbacks.onError?.(err);
        } else if (err && typeof err === 'object' && 'code' in err) {
          // CloseEvent from WebSocket
          const closeErr = err as { code: number; reason?: string };
          callbacks.onError?.(new Error(`WebSocket closed: ${closeErr.code} ${closeErr.reason ?? ''}`));
        } else {
          callbacks.onError?.(new Error('Unknown subscription error'));
        }
      },
      complete: () => {
        callbacks.onComplete?.();
      }
    }
  );

  return unsubscribe;
}

/**
 * Disposes the WebSocket client, closing the connection.
 * Useful for cleanup during hot module replacement or app shutdown.
 */
export async function disposeClient(): Promise<void> {
  if (wsClient) {
    await wsClient.dispose();
    wsClient = null;
  }
}
