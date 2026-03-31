import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { workflowDraft } from '$lib/features/workflows/workflowStore';
import RuntimeOutputPanel from './RuntimeOutputPanel.svelte';
import { resetRuntimeOutputFeed } from './runtimeOutputStore';
import { WorkflowEventsDocument } from '$lib/gen/graphql';

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
    expect(subscribeMock).toHaveBeenCalledTimes(1);
  });
});
