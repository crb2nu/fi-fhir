import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { workflowDraft } from '$lib/features/workflows/workflowStore';
import RuntimeOutputPanel from './RuntimeOutputPanel.svelte';
import { resetRuntimeOutputFeed } from './runtimeOutputStore';
import { WorkflowEventsDocument } from '$lib/gen/graphql';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import { resetObservedStreams } from '$lib/graphql/streamAvailability';

const { subscribeMock } = vi.hoisted(() => ({
  subscribeMock: vi.fn()
}));

vi.mock('$lib/graphql/subscriptions', () => ({
  subscribe: (...args: unknown[]) => subscribeMock(...args)
}));

describe('RuntimeOutputPanel', () => {
  beforeEach(() => {
    workflowDraft.reset();
    resetRuntimeOutputFeed();
    subscribeMock.mockReset();
  });

  afterEach(() => {
    resetAccessCapabilities();
    resetObservedStreams();
  });

  it('renders the honest state and never subscribes when the event stream is unavailable', () => {
    setAccessStatus({
      authenticated: true,
      authVia: 'network',
      capabilities: { operatorRead: false, streaming: false, subscriptions: [] }
    });

    render(RuntimeOutputPanel);

    const state = screen.getByTestId('streaming-unavailable');
    expect(state).toHaveAttribute('data-stream', 'eventStream');
    expect(state).toHaveTextContent('Live streaming for runtime output is not available');
    expect(subscribeMock).not.toHaveBeenCalled();
  });

  it('names the workflow feed when a named draft cannot stream workflowEvents', () => {
    workflowDraft.update((draft) => ({ ...draft, name: 'adt-routing' }));
    setAccessStatus({
      authenticated: true,
      authVia: 'network',
      capabilities: {
        operatorRead: false,
        streaming: true,
        subscriptions: ['integrationSessionEvents', 'sessionRunEvents']
      }
    });

    render(RuntimeOutputPanel);

    const state = screen.getByTestId('streaming-unavailable');
    expect(state).toHaveAttribute('data-stream', 'workflowEvents');
    expect(state).toHaveTextContent('workflow adt-routing output');
    expect(subscribeMock).not.toHaveBeenCalled();
  });

  it('flips to the honest state when the stream answers 404 instead of reporting a disconnect', async () => {
    subscribeMock.mockImplementation((_document, _variables, callbacks) => {
      callbacks.onError(new Error('GraphQL stream HTTP 404'));
      return vi.fn();
    });

    render(RuntimeOutputPanel);

    const state = await screen.findByTestId('streaming-unavailable');
    expect(state).toHaveAttribute('data-reason', 'streaming-off');
    expect(screen.queryByText('Disconnected')).not.toBeInTheDocument();
  });

  it('renders live workflow output entries with severity and timestamps', async () => {
    workflowDraft.update((draft) => ({ ...draft, name: 'adt-routing' }));

    subscribeMock.mockImplementation((document, variables, callbacks) => {
      expect(document).toBe(WorkflowEventsDocument);
      expect(variables).toEqual({ workflowName: 'adt-routing' });

      callbacks.onData({
        workflowEvents: {
          workflow: 'adt-routing',
          routesMatched: [],
          actionsExecuted: ['notify'],
          duration: 42,
          event: {
            id: 'event-1',
            type: 'PATIENT_ADMIT',
            timestamp: '2026-03-31T10:30:00.000Z',
            source: 'workflow-engine'
          }
        }
      });

      return vi.fn();
    });

    render(RuntimeOutputPanel);

    expect(await screen.findByText('warning')).toBeInTheDocument();
    expect(screen.getByText('adt-routing · Patient Admit')).toBeInTheDocument();
    expect(screen.getByText(/No routes matched/)).toBeInTheDocument();
    expect(screen.getByText('workflow-engine', { selector: '.source' })).toBeInTheDocument();
    expect(screen.getByRole('list', { name: 'Runtime output entries' })).toBeInTheDocument();
    expect(screen.getByText('Workflow feed')).toBeInTheDocument();
    expect(screen.getByText('Live')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Clear feed' })).toBeInTheDocument();
    expect(subscribeMock).toHaveBeenCalledTimes(1);
  });
});
