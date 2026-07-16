import type { TypedDocumentNode } from '@graphql-typed-document-node/core';
import { print } from 'graphql';
import { requireGraphQLAuthorization } from './credentials';

const MAX_STREAM_EVENT_BYTES = 1 << 20;

export type SubscriptionCallbacks<TData> = {
  onOpen?: () => void;
  onData: (data: TData) => void;
  onError?: (error: Error) => void;
  onComplete?: () => void;
};

type GraphQLStreamEnvelope<TData> = {
  data?: TData;
  errors?: Array<{ message?: string }>;
};

/**
 * Opens one authenticated GraphQL SSE subscription over the same bounded POST
 * endpoint used by mutations. The backend allowlists Integration Session
 * subscription roots; WebSocket transport remains closed.
 */
export function subscribe<TData, TVariables>(
  document: TypedDocumentNode<TData, TVariables>,
  variables: TVariables,
  callbacks: SubscriptionCallbacks<TData>
): () => void {
  const controller = new AbortController();
  let closed = false;

  void openStream(document, variables, callbacks, controller.signal).catch((cause: unknown) => {
    if (closed || isAbortError(cause)) return;
    callbacks.onError?.(cause instanceof Error ? cause : new Error(String(cause)));
  });

  return () => {
    closed = true;
    controller.abort();
  };
}

async function openStream<TData, TVariables>(
  document: TypedDocumentNode<TData, TVariables>,
  variables: TVariables,
  callbacks: SubscriptionCallbacks<TData>,
  signal: AbortSignal
): Promise<void> {
  const authorization = await requireGraphQLAuthorization();
  const headers: Record<string, string> = {
    accept: 'text/event-stream',
    'content-type': 'application/json'
  };
  if (authorization) headers.Authorization = authorization;

  const response = await fetch('/graphql', {
    method: 'POST',
    headers,
    body: JSON.stringify({ query: print(document), variables }),
    signal
  });
  if (!response.ok) {
    throw new Error(`GraphQL stream HTTP ${response.status}`);
  }
  if (!response.body) {
    throw new Error('GraphQL stream response is not readable');
  }
  if (!response.headers.get('content-type')?.toLowerCase().startsWith('text/event-stream')) {
    throw new Error('GraphQL stream response has an unexpected content type');
  }

  callbacks.onOpen?.();
  const reader = response.body.getReader();
  const decoder = new TextDecoder();
  let buffer = '';
  let completed = false;

  while (!completed) {
    const chunk = await reader.read();
    buffer += decoder.decode(chunk.value, { stream: !chunk.done });
    if (buffer.length > MAX_STREAM_EVENT_BYTES && !buffer.includes('\n\n')) {
      throw new Error('GraphQL stream event exceeds the client limit');
    }

    const parsed = drainSSEBuffer(buffer);
    buffer = parsed.remainder;
    for (const event of parsed.events) {
      if (event.type === 'complete') {
        completed = true;
        break;
      }
      if (event.type !== 'next' || !event.data) continue;
      const envelope = JSON.parse(event.data) as GraphQLStreamEnvelope<TData>;
      if (envelope.errors?.length) {
        throw new Error(envelope.errors.map((entry) => entry.message || 'Unknown error').join('; '));
      }
      if (envelope.data) callbacks.onData(envelope.data);
    }
    if (chunk.done) break;
  }

  callbacks.onComplete?.();
}

type ParsedSSEEvent = { type: string; data: string };

function drainSSEBuffer(buffer: string): {
  events: ParsedSSEEvent[];
  remainder: string;
} {
  const normalized = buffer.replaceAll('\r\n', '\n');
  const blocks = normalized.split('\n\n');
  const remainder = blocks.pop() ?? '';
  const events: ParsedSSEEvent[] = [];

  for (const block of blocks) {
    if (block.length > MAX_STREAM_EVENT_BYTES) {
      throw new Error('GraphQL stream event exceeds the client limit');
    }
    let type = 'message';
    const data: string[] = [];
    for (const line of block.split('\n')) {
      if (line.startsWith('event:')) type = line.slice(6).trim();
      if (line.startsWith('data:')) data.push(line.slice(5).trimStart());
    }
    events.push({ type, data: data.join('\n') });
  }
  return { events, remainder };
}

function isAbortError(cause: unknown): boolean {
  return cause instanceof DOMException && cause.name === 'AbortError';
}

/** Kept as a stable lifecycle hook for the credential gate. */
export function disposeClient(): Promise<void> {
  return Promise.resolve();
}
