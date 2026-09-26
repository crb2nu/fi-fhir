import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import EventStreamPanel from './EventStreamPanel.svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import { resetObservedStreams } from '$lib/graphql/streamAvailability';

const { subscribeMock, unsubscribeMock } = vi.hoisted(() => ({
  subscribeMock: vi.fn(),
  unsubscribeMock: vi.fn()
}));

vi.mock('$lib/graphql/subscriptions', () => ({
  subscribe: (...args: unknown[]) => subscribeMock(...args)
}));

type Callbacks = { onError?: (error: Error) => void };

describe('EventStreamPanel streaming honesty', () => {
  beforeEach(() => {
    subscribeMock.mockReset();
    unsubscribeMock.mockReset();
    subscribeMock.mockReturnValue(unsubscribeMock);
  });

  afterEach(() => {
    resetAccessCapabilities();
    resetObservedStreams();
  });

  it('renders the honest state and never subscribes when the API has streaming off', () => {
    setAccessStatus({
      authenticated: true,
      authVia: 'network',
      capabilities: { operatorRead: false, streaming: false, subscriptions: [] }
    });
    render(EventStreamPanel);

    const state = screen.getByTestId('streaming-unavailable');
    expect(state).toHaveAttribute('data-stream', 'eventStream');
    expect(state).toHaveTextContent('Live streaming for the event stream is not available on this deployment');
    expect(state).toHaveTextContent('FI_FHIR_INTEGRATION_SESSION_ENABLED');
    expect(state).toHaveTextContent('Events browser');
    expect(subscribeMock).not.toHaveBeenCalled();
    expect(screen.queryByText(/Connecting/)).not.toBeInTheDocument();
  });

  it('says what streams instead when eventStream is not allowlisted', () => {
    setAccessStatus({
      authenticated: true,
      authVia: 'network',
      capabilities: {
        operatorRead: false,
        streaming: true,
        subscriptions: ['integrationSessionEvents', 'sessionRunEvents']
      }
    });
    render(EventStreamPanel);

    const state = screen.getByTestId('streaming-unavailable');
    expect(state).toHaveAttribute('data-reason', 'not-allowlisted');
    expect(state).toHaveTextContent('integrationSessionEvents');
    expect(subscribeMock).not.toHaveBeenCalled();
  });

  it('flips to the honest state when a stream opened on unknown capabilities answers 404', async () => {
    render(EventStreamPanel);
    expect(subscribeMock).toHaveBeenCalledTimes(1);

    const callbacks = subscribeMock.mock.calls[0]?.[2] as Callbacks;
    callbacks.onError?.(new Error('GraphQL stream HTTP 404'));

    const state = await screen.findByTestId('streaming-unavailable');
    expect(state).toHaveAttribute('data-reason', 'streaming-off');
    expect(unsubscribeMock).toHaveBeenCalled();
    expect(screen.queryByText('GraphQL stream HTTP 404')).not.toBeInTheDocument();
  });

  it('keeps an ordinary stream error visible with Reconnect', async () => {
    render(EventStreamPanel);
    const callbacks = subscribeMock.mock.calls[0]?.[2] as Callbacks;
    callbacks.onError?.(new Error('GraphQL stream HTTP 502'));

    expect(await screen.findByText('GraphQL stream HTTP 502')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Reconnect' })).toBeInTheDocument();
    expect(screen.queryByTestId('streaming-unavailable')).not.toBeInTheDocument();
  });
});
