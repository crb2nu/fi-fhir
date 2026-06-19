import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { get } from 'svelte/store';
import WorkflowPreview from './WorkflowPreview.svelte';
import { explainWorkflow } from '../workflowApi';
import { workflowDraft } from '../workflowStore';
import { toastList, toasts } from '$lib/ui/toastStore';
import { isErrorToasted } from '$lib/graphql/client';

vi.mock('../workflowApi', () => ({
  explainWorkflow: vi.fn()
}));

vi.mock('$lib/graphql/client', () => ({
  isErrorToasted: vi.fn(() => false)
}));

describe('WorkflowPreview', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(isErrorToasted).mockReturnValue(false);
    workflowDraft.loadDraft({
      name: 'critical-lab-routing',
      version: '1.0',
      routes: [
        {
          _key: 'route-1',
          name: 'critical_labs',
          filter: {
            eventTypes: ['LAB_RESULT'],
            sources: [],
            condition: 'event.isCritical == true'
          },
          transforms: [],
          actions: [
            {
              _key: 'action-1',
              type: 'webhook',
              config: { url: 'https://alerts.example.com' }
            }
          ],
          expanded: false
        }
      ]
    });
    toasts.dismissAll();
  });

  afterEach(() => {
    cleanup();
  });

  it('calls explainWorkflow with the YAML preview and business audience, then renders route explanations', async () => {
    vi.mocked(explainWorkflow).mockResolvedValue({
      explainWorkflow: {
        summary: 'Critical lab alert routing',
        description: 'Routes critical lab events to an alert webhook.',
        diagram: null,
        routeExplanations: [
          {
            name: 'critical_labs',
            trigger: 'LAB_RESULT events where event.isCritical is true',
            actions: ['webhook'],
            description: 'Sends critical lab results to the alerts endpoint.'
          }
        ],
        warnings: []
      }
    } as Awaited<ReturnType<typeof explainWorkflow>>);

    render(WorkflowPreview);

    await fireEvent.click(screen.getByRole('button', { name: /Explain with AI/ }));

    await waitFor(() => {
      expect(explainWorkflow).toHaveBeenCalledTimes(1);
    });
    const [yamlOutput, audience] = vi.mocked(explainWorkflow).mock.calls[0]!;
    expect(yamlOutput).toContain('name: critical-lab-routing');
    expect(yamlOutput).toContain('name: critical_labs');
    expect(yamlOutput).toContain('event_type: LAB_RESULT');
    expect(audience).toBe('business');
    expect(screen.getByText(/Routes critical lab events to an alert webhook/)).toBeInTheDocument();
    expect(screen.getByText(/critical_labs: Sends critical lab results/)).toBeInTheDocument();
  });

  it('shows a loading label while the explanation is in flight', async () => {
    let resolveExplanation: (value: Awaited<ReturnType<typeof explainWorkflow>>) => void = () => {};
    vi.mocked(explainWorkflow).mockReturnValue(
      new Promise((resolve) => {
        resolveExplanation = resolve;
      })
    );

    render(WorkflowPreview);

    await fireEvent.click(screen.getByRole('button', { name: /Explain with AI/ }));

    expect(screen.getByRole('button', { name: 'Explaining...' })).toBeInTheDocument();

    resolveExplanation({
      explainWorkflow: {
        summary: '',
        description: 'done',
        diagram: null,
        routeExplanations: [],
        warnings: []
      }
    } as Awaited<ReturnType<typeof explainWorkflow>>);
    expect(await screen.findByText('done')).toBeInTheDocument();
  });

  it('does not add a duplicate local toast when explainWorkflow reports an already-toasted failure', async () => {
    vi.mocked(isErrorToasted).mockReturnValue(true);
    vi.mocked(explainWorkflow).mockRejectedValue(new Error('LLM unavailable'));

    render(WorkflowPreview);

    await fireEvent.click(screen.getByRole('button', { name: /Explain with AI/ }));

    await waitFor(() => {
      expect(explainWorkflow).toHaveBeenCalled();
    });
    expect(get(toastList).some((toast) => toast.message === 'Failed to explain workflow')).toBe(false);
  });
});
