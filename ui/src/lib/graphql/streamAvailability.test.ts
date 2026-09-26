import { afterEach, describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { parseAuthStatus, resetAccessCapabilities, setAccessStatus } from './accessCapabilities';
import {
  canAttemptStream,
  classifyStreamError,
  noteStreamError,
  resetObservedStreams,
  resolveStreamStatus,
  streamStatus
} from './streamAvailability';

function status(capabilities: Record<string, unknown>) {
  return {
    authenticated: true,
    authVia: 'network',
    capabilities: { operatorRead: false, ...capabilities }
  };
}

afterEach(() => {
  resetAccessCapabilities();
  resetObservedStreams();
});

describe('classifyStreamError', () => {
  it('reads a 404 from the SSE transport as streaming off', () => {
    expect(classifyStreamError(new Error('GraphQL stream HTTP 404'))).toBe('streaming-off');
  });

  it('reads the transport refusal as a root that is not allowlisted', () => {
    expect(classifyStreamError(new Error('GraphQL stream operation forbidden'))).toBe(
      'not-allowlisted'
    );
  });

  it('leaves ordinary failures alone so a reconnect can still help', () => {
    expect(classifyStreamError(new Error('GraphQL stream HTTP 502'))).toBeNull();
    expect(classifyStreamError(new Error('Failed to fetch'))).toBeNull();
    expect(classifyStreamError(undefined)).toBeNull();
  });
});

describe('resolveStreamStatus', () => {
  it('is unknown while capabilities are unknown (old two-key status)', () => {
    const state = parseAuthStatus({ authenticated: true, authVia: 'network' });
    expect(resolveStreamStatus(state, {}, 'eventStream')).toEqual({ availability: 'unknown' });
  });

  it('is unavailable for every root when the API has streaming off', () => {
    const state = parseAuthStatus(status({ streaming: false, subscriptions: [] }));
    for (const root of ['eventStream', 'integrationSessionEvents'] as const) {
      expect(resolveStreamStatus(state, {}, root)).toEqual({
        availability: 'unavailable',
        reason: 'streaming-off'
      });
    }
  });

  it('keys on subscription membership when streaming is on', () => {
    const state = parseAuthStatus(
      status({ streaming: true, subscriptions: ['integrationSessionEvents', 'sessionRunEvents'] })
    );
    expect(resolveStreamStatus(state, {}, 'integrationSessionEvents')).toEqual({
      availability: 'available'
    });
    expect(resolveStreamStatus(state, {}, 'workflowEvents')).toEqual({
      availability: 'unavailable',
      reason: 'not-allowlisted'
    });
  });

  it('is unknown when streaming is on but the list was not reported', () => {
    const state = parseAuthStatus(status({ streaming: true }));
    expect(resolveStreamStatus(state, {}, 'eventStream')).toEqual({ availability: 'unknown' });
  });

  it('lets an observed failure override a stale "available" status', () => {
    const state = parseAuthStatus(
      status({ streaming: true, subscriptions: ['integrationSessionEvents'] })
    );
    expect(
      resolveStreamStatus(state, { integrationSessionEvents: 'streaming-off' }, 'integrationSessionEvents')
    ).toEqual({ availability: 'unavailable', reason: 'streaming-off' });
  });
});

describe('streamStatus store', () => {
  it('flips a root to unavailable when a stream answers 404', () => {
    const eventStream = streamStatus('eventStream');
    expect(get(eventStream)).toEqual({ availability: 'unknown' });
    expect(canAttemptStream('eventStream')).toBe(true);

    expect(noteStreamError('eventStream', new Error('GraphQL stream HTTP 404'))).toBe(true);

    expect(get(eventStream)).toEqual({ availability: 'unavailable', reason: 'streaming-off' });
    expect(canAttemptStream('eventStream')).toBe(false);
    // Other roots are untouched by one root's failure.
    expect(canAttemptStream('workflowEvents')).toBe(true);
  });

  it('does not mark a root for an ordinary error', () => {
    expect(noteStreamError('eventStream', new Error('Failed to fetch'))).toBe(false);
    expect(canAttemptStream('eventStream')).toBe(true);
  });

  it('follows the capabilities the gate records', () => {
    setAccessStatus(status({ streaming: false, subscriptions: [] }));
    expect(canAttemptStream('debugStepEvent')).toBe(false);
  });
});
