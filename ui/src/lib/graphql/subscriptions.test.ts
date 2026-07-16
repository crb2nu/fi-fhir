import { afterEach, describe, expect, it, vi } from 'vitest';
import { parse } from 'graphql';
import type { TypedDocumentNode } from '@graphql-typed-document-node/core';
import { setGraphQLCredentialProvider } from './credentials';
import { disposeClient, subscribe } from './subscriptions';

const document = parse('subscription Test { event: integrationSessionEvents(sessionId: "session-1") { id } }') as unknown as TypedDocumentNode<
  { event: { id: string } },
  Record<string, never>
>;

afterEach(() => {
  setGraphQLCredentialProvider(null);
  vi.unstubAllGlobals();
});

describe('GraphQL SSE subscriptions', () => {
  it('authenticates, streams data, and completes without opening a WebSocket', async () => {
    setGraphQLCredentialProvider(() => 'test-token');
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(new TextEncoder().encode(':\n\nevent: next\ndata: {"data":{"event":{"id":"event-1"}}}\n\n'));
        controller.enqueue(new TextEncoder().encode('event: complete\n\n'));
        controller.close();
      }
    });
    const fetchMock = vi.fn().mockResolvedValue(new Response(stream, {
      status: 200,
      headers: { 'content-type': 'text/event-stream' }
    }));
    vi.stubGlobal('fetch', fetchMock);
    const onOpen = vi.fn();
    const onData = vi.fn();
    const onComplete = vi.fn();

    const unsubscribe = subscribe(document, {}, { onOpen, onData, onComplete });

    await vi.waitFor(() => expect(onComplete).toHaveBeenCalledOnce());
    expect(onOpen).toHaveBeenCalledOnce();
    expect(onData).toHaveBeenCalledWith({ event: { id: 'event-1' } });
    expect(fetchMock).toHaveBeenCalledWith(
      '/graphql',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({
          accept: 'text/event-stream',
          Authorization: 'Bearer test-token'
        })
      })
    );
    expect(() => unsubscribe()).not.toThrow();
  });

  it('fails closed before fetch when credentials are unavailable', async () => {
    const fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
    const onError = vi.fn();

    subscribe(document, {}, { onData: vi.fn(), onError });

    await vi.waitFor(() => expect(onError).toHaveBeenCalledWith(expect.objectContaining({
      message: 'GraphQL credentials unavailable'
    })));
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('rejects a successful response that is not an SSE stream', async () => {
    setGraphQLCredentialProvider(() => 'test-token');
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response('{}', {
      status: 200,
      headers: { 'content-type': 'application/json' }
    })));
    const onError = vi.fn();

    subscribe(document, {}, { onData: vi.fn(), onError });

    await vi.waitFor(() => expect(onError).toHaveBeenCalledWith(expect.objectContaining({
      message: 'GraphQL stream response has an unexpected content type'
    })));
  });

  it('keeps credential-gate disposal safe', async () => {
    await expect(disposeClient()).resolves.toBeUndefined();
  });
});
