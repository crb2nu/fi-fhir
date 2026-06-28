import { graphqlFetch } from '$lib/graphql/client';
import { subscribe as wsSubscribe } from '$lib/graphql/subscriptions';
import type { ParsePreviewQuery } from '$lib/gen/graphql';
import {
  AddSessionSampleDocument,
  CreateIntegrationSessionDocument,
  RunSessionPreviewDocument,
  SessionRunEventsDocument,
  type SessionRunEventsSubscription,
  type SessionRunEventsSubscriptionVariables
} from './documents';
import type { SubscriptionCallbacks } from '$lib/graphql/subscriptions';
import type { IntegrationSessionPreviewMeta, SessionBackedPreviewResult } from './types';

export type SessionBackedHL7PreviewInput = {
  source: string;
  data: string;
  profileId?: string | null;
  sessionId?: string | null;
};

export function isIntegrationSessionEngineEnabled(): boolean {
  return import.meta.env.PUBLIC_INTEGRATION_SESSION_ENGINE === '1';
}

function messageFromError(err: unknown): string {
  return err instanceof Error ? err.message : String(err);
}

function emptyPreview(): ParsePreviewQuery['parsePreview'] {
  return {
    __typename: 'ParseResult',
    success: false,
    errors: ['Session preview did not return a parse result'],
    events: [],
    warnings: []
  };
}

export function buildFallbackSessionMeta(error: unknown): IntegrationSessionPreviewMeta {
  return {
    mode: 'fallback',
    id: null,
    sampleId: null,
    runId: null,
    state: 'fallback',
    diagnostics: [],
    stages: [],
    error: messageFromError(error)
  };
}

export async function runSessionBackedHL7Preview(
  input: SessionBackedHL7PreviewInput
): Promise<SessionBackedPreviewResult> {
  const source = input.source || 'ui';
  let sessionId = input.sessionId ?? null;

  if (!sessionId) {
    const created = await graphqlFetch(
      CreateIntegrationSessionDocument,
      {
        input: {
          name: `${source} HL7 preview`,
          description: input.profileId ? `Profile ${input.profileId}` : null
        }
      },
      { showErrorToast: false }
    );
    sessionId = created.createIntegrationSession.id;
  }

  const sample = await graphqlFetch(
    AddSessionSampleDocument,
    {
      input: {
        sessionId,
        name: `${source} sample`,
        source,
        format: 'HL7V2',
        data: input.data,
        retainRawPayload: true
      }
    },
    { showErrorToast: false }
  );

  const run = await graphqlFetch(
    RunSessionPreviewDocument,
    {
      input: {
        sessionId,
        sampleId: sample.addSessionSample.id,
        source
      }
    },
    { showErrorToast: false }
  );

  const preview: ParsePreviewQuery['parsePreview'] = {
    __typename: 'ParseResult',
    success: run.runSessionPreview.status === 'completed',
    errors: run.runSessionPreview.status === 'completed' ? [] : ['Session preview failed'],
    events: run.runSessionPreview.events ?? [],
    warnings: run.runSessionPreview.warnings ?? []
  };

  return {
    parsePreview: preview,
    session: {
      mode: 'session',
      id: sessionId,
      sampleId: sample.addSessionSample.id,
      runId: run.runSessionPreview.id,
      state: run.runSessionPreview.status,
      diagnostics: run.runSessionPreview.diagnostics ?? [],
      stages: run.runSessionPreview.stages ?? [],
      error: null
    }
  };
}

export function subscribeSessionRunEvents(
  sessionId: string,
  runId: string | null,
  callbacks: {
    onData: (event: SessionRunEventsSubscription['sessionRunEvents']) => void;
    onError?: (err: Error) => void;
  }
): () => void {
  const subscriptionCallbacks: SubscriptionCallbacks<SessionRunEventsSubscription> = {
    onData: (data) => {
      callbacks.onData(data.sessionRunEvents);
    }
  };

  if (callbacks.onError) {
    subscriptionCallbacks.onError = callbacks.onError;
  }

  return wsSubscribe<SessionRunEventsSubscription, SessionRunEventsSubscriptionVariables>(
    SessionRunEventsDocument,
    { sessionId, runId },
    subscriptionCallbacks
  );
}
