import { describe, expect, it, vi } from 'vitest';
import type { TypedDocumentNode } from '@graphql-typed-document-node/core';
import {
  GraphQLSubscriptionsUnavailableError,
  disposeClient,
  subscribe
} from './subscriptions';

const document = {
  kind: 'Document',
  definitions: []
} as unknown as TypedDocumentNode<{ event: string }, Record<string, never>>;

describe('GraphQL subscription containment', () => {
  it('fails locally without creating a transport or retry loop', async () => {
    const onData = vi.fn();
    const onError = vi.fn();

    const unsubscribe = subscribe(document, {}, { onData, onError });
    expect(unsubscribe).toBeTypeOf('function');

    await vi.waitFor(() => {
      expect(onError).toHaveBeenCalledWith(expect.any(GraphQLSubscriptionsUnavailableError));
    });
    expect(onData).not.toHaveBeenCalled();
    expect(() => unsubscribe()).not.toThrow();
  });

  it('keeps credential-gate disposal safe when no transport exists', async () => {
    await expect(disposeClient()).resolves.toBeUndefined();
  });
});
