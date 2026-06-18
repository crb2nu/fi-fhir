import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import { get } from 'svelte/store';

// Mock the op boundary only — the component must call the REAL GenerateWorkflow
// op via workflowApi, never the copilot simulator (.loom/23 Wave 2 Slice 2b).
vi.mock('../workflowApi', () => ({ generateWorkflow: vi.fn() }));

import GenerateFromDescription from './GenerateFromDescription.svelte';
import { generateWorkflow } from '../workflowApi';
import { workflowDraft } from '../workflowStore';
import { draftToYaml } from '../workflowYaml';
import { ALL_EVENT_TYPES, ACTION_TYPES } from '../workflowTypes';

const mockGenerate = generateWorkflow as unknown as ReturnType<typeof vi.fn>;

// A real, round-trippable draft -> YAML, so "Load into Builder" exercises the
// genuine yamlToDraft parse rather than a mock.
const KNOWN_DRAFT = {
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

beforeEach(() => {
  mockGenerate.mockReset();
  workflowDraft.reset();
});

describe('GenerateFromDescription', () => {
  it('calls the real GenerateWorkflow op with the description and type hints', async () => {
    mockGenerate.mockResolvedValue({
      generateWorkflow: { yaml: 'routes: []', explanation: 'Routes admits', warnings: [] }
    });

    render(GenerateFromDescription);
    await fireEvent.input(screen.getByLabelText('Workflow description'), {
      target: { value: 'route ADT admits to FHIR' }
    });
    await fireEvent.click(screen.getByRole('button', { name: /Generate Workflow/ }));

    expect(mockGenerate).toHaveBeenCalledWith('route ADT admits to FHIR', ALL_EVENT_TYPES, ACTION_TYPES);
    expect(await screen.findByText('Routes admits')).toBeInTheDocument();
    expect(screen.getByText('routes: []')).toBeInTheDocument();
  });

  it('renders generator warnings returned by the op', async () => {
    mockGenerate.mockResolvedValue({
      generateWorkflow: { yaml: 'routes: []', explanation: '', warnings: ['ambiguous trigger'] }
    });

    render(GenerateFromDescription);
    await fireEvent.input(screen.getByLabelText('Workflow description'), {
      target: { value: 'do something' }
    });
    await fireEvent.click(screen.getByRole('button', { name: /Generate Workflow/ }));

    expect(await screen.findByText('ambiguous trigger')).toBeInTheDocument();
  });

  it('loads the generated YAML into the builder draft (not a toast)', async () => {
    const yaml = draftToYaml(KNOWN_DRAFT);
    mockGenerate.mockResolvedValue({
      generateWorkflow: { yaml, explanation: '', warnings: [] }
    });

    render(GenerateFromDescription);
    await fireEvent.input(screen.getByLabelText('Workflow description'), {
      target: { value: 'admissions' }
    });
    await fireEvent.click(screen.getByRole('button', { name: /Generate Workflow/ }));
    await screen.findByText('Load into Builder');

    // Draft is untouched until the user explicitly loads it.
    expect(get(workflowDraft).name).toBe('');

    await fireEvent.click(screen.getByRole('button', { name: 'Load into Builder' }));

    expect(get(workflowDraft).name).toBe('adt-routing');
    expect(get(workflowDraft).routes).toHaveLength(1);
  });
});
