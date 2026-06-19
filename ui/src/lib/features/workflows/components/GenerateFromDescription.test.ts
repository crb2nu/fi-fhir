import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { get } from 'svelte/store';
import GenerateFromDescription from './GenerateFromDescription.svelte';
import { generateWorkflow } from '../workflowApi';
import { ACTION_TYPES, ALL_EVENT_TYPES } from '../workflowTypes';
import { workflowDraft } from '../workflowStore';
import { draftToYaml } from '../workflowYaml';
import { toastList, toasts } from '$lib/ui/toastStore';
import { isErrorToasted } from '$lib/graphql/client';

vi.mock('../workflowApi', () => ({
  generateWorkflow: vi.fn()
}));

vi.mock('$lib/graphql/client', () => ({
  isErrorToasted: vi.fn(() => false)
}));

const knownDraft = {
  name: 'adt-routing',
  version: '1.0',
  routes: [
    {
      _key: 'route-1',
      name: 'Admission route',
      filter: { eventTypes: ['PATIENT_ADMIT'], sources: [], condition: '' },
      transforms: [],
      actions: [{ _key: 'action-1', type: 'log', config: { message: 'received' } }],
      expanded: true
    }
  ]
};

describe('GenerateFromDescription', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(isErrorToasted).mockReturnValue(false);
    workflowDraft.reset();
    toasts.dismissAll();
  });

  afterEach(() => {
    cleanup();
  });

  it('calls generateWorkflow with the current description and supported workflow types', async () => {
    vi.mocked(generateWorkflow).mockResolvedValue({
      generateWorkflow: {
        yaml: draftToYaml(knownDraft),
        explanation: 'Routes patient admits to the configured FHIR server.',
        warnings: ['FHIR credentials still need to be configured.']
      }
    } as Awaited<ReturnType<typeof generateWorkflow>>);

    render(GenerateFromDescription);

    await fireEvent.input(screen.getByLabelText('Workflow description'), {
      target: { value: 'Route admits to FHIR' }
    });
    await fireEvent.click(screen.getByRole('button', { name: /Generate Workflow/ }));

    await waitFor(() => {
      expect(generateWorkflow).toHaveBeenCalledWith(
        'Route admits to FHIR',
        ALL_EVENT_TYPES,
        ACTION_TYPES
      );
    });
    expect(screen.getByText('Routes patient admits to the configured FHIR server.')).toBeInTheDocument();
    expect(screen.getByText('FHIR credentials still need to be configured.')).toBeInTheDocument();
    expect(screen.getByText((_, element) =>
      Boolean(element?.matches('pre') && element.textContent?.includes('name: adt-routing'))
    )).toBeInTheDocument();
    expect(get(workflowDraft).name).toBe('');
  });

  it('loads valid generated YAML into the builder draft only after the user asks', async () => {
    vi.mocked(generateWorkflow).mockResolvedValue({
      generateWorkflow: {
        yaml: draftToYaml(knownDraft),
        explanation: '',
        warnings: []
      }
    } as Awaited<ReturnType<typeof generateWorkflow>>);

    render(GenerateFromDescription);

    await fireEvent.input(screen.getByLabelText('Workflow description'), {
      target: { value: 'admissions' }
    });
    await fireEvent.click(screen.getByRole('button', { name: /Generate Workflow/ }));
    await screen.findByText('Load into Builder');

    expect(get(workflowDraft).name).toBe('');

    await fireEvent.click(screen.getByRole('button', { name: 'Load into Builder' }));

    await waitFor(() => {
      expect(get(workflowDraft).name).toBe('adt-routing');
    });
    const draft = get(workflowDraft);
    expect(draft.routes).toHaveLength(1);
    expect(draft.routes[0]!.name).toBe('Admission route');
    expect(draft.routes[0]!.filter.eventTypes).toEqual(['PATIENT_ADMIT']);
    expect(get(toastList).some((toast) => toast.message === 'Workflow loaded into builder')).toBe(true);
  });

  it('shows parse failure only when invalid generated YAML is loaded and leaves the draft unchanged', async () => {
    vi.mocked(generateWorkflow).mockResolvedValue({
      generateWorkflow: {
        yaml: 'not: [valid: yaml',
        explanation: 'This response is malformed.',
        warnings: []
      }
    } as Awaited<ReturnType<typeof generateWorkflow>>);
    const originalDraft = get(workflowDraft);

    render(GenerateFromDescription);

    await fireEvent.input(screen.getByLabelText('Workflow description'), {
      target: { value: 'Generate malformed yaml for the kill-test' }
    });
    await fireEvent.click(screen.getByRole('button', { name: /Generate Workflow/ }));

    await screen.findByText('not: [valid: yaml');
    expect(get(toastList).some((toast) => toast.message === 'Failed to parse generated YAML')).toBe(false);
    expect(get(workflowDraft)).toEqual(originalDraft);

    await fireEvent.click(screen.getByRole('button', { name: 'Load into Builder' }));

    await waitFor(() => {
      expect(get(toastList).some((toast) => toast.message === 'Failed to parse generated YAML')).toBe(true);
    });
    expect(get(workflowDraft)).toEqual(originalDraft);
  });

  it('does not add a duplicate local toast for already-toasted GraphQL errors', async () => {
    vi.mocked(isErrorToasted).mockReturnValue(true);
    vi.mocked(generateWorkflow).mockRejectedValue(new Error('LLM unavailable'));

    render(GenerateFromDescription);

    await fireEvent.input(screen.getByLabelText('Workflow description'), {
      target: { value: 'Route lab results to email' }
    });
    await fireEvent.click(screen.getByRole('button', { name: /Generate Workflow/ }));

    await waitFor(() => {
      expect(generateWorkflow).toHaveBeenCalled();
    });
    expect(get(toastList).some((toast) => toast.message === 'Failed to generate workflow')).toBe(false);
  });
});
