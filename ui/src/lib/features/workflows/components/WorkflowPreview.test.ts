import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';

// Mock the op boundary only — "Explain with AI" must call the REAL
// ExplainWorkflow query via workflowApi (.loom/23 Wave 2 Slice 2b).
vi.mock('../workflowApi', () => ({ explainWorkflow: vi.fn() }));

import WorkflowPreview from './WorkflowPreview.svelte';
import { explainWorkflow } from '../workflowApi';
import { workflowDraft } from '../workflowStore';

const mockExplain = explainWorkflow as unknown as ReturnType<typeof vi.fn>;

beforeEach(() => {
  mockExplain.mockReset();
  workflowDraft.loadDraft({
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
  });
});

describe('WorkflowPreview "Explain with AI"', () => {
  it('calls the real ExplainWorkflow query with the draft YAML for a business audience', async () => {
    mockExplain.mockResolvedValue({
      explainWorkflow: {
        summary: 'Routes admissions',
        description: 'This workflow forwards admit events to FHIR.',
        routeExplanations: [{ name: 'Admission route', trigger: 'PATIENT_ADMIT', actions: ['log'], description: 'logs the event' }],
        diagram: null,
        warnings: []
      }
    });

    render(WorkflowPreview);
    await fireEvent.click(screen.getByRole('button', { name: /Explain with AI/ }));

    expect(mockExplain).toHaveBeenCalledTimes(1);
    const [yamlArg, audienceArg] = mockExplain.mock.calls[0] ?? [];
    expect(yamlArg).toContain('adt-routing'); // real draftToYaml output
    expect(audienceArg).toBe('business');

    expect(await screen.findByText(/forwards admit events to FHIR/)).toBeInTheDocument();
    // Route explanations are appended to the rendered description.
    expect(screen.getByText(/Admission route: logs the event/)).toBeInTheDocument();
  });

  it('shows a loading label while the explanation is in flight', async () => {
    let resolveFn: (v: unknown) => void = () => {};
    mockExplain.mockReturnValue(new Promise((r) => { resolveFn = r; }));

    render(WorkflowPreview);
    await fireEvent.click(screen.getByRole('button', { name: 'Explain with AI' }));

    expect(screen.getByRole('button', { name: 'Explaining...' })).toBeInTheDocument();

    resolveFn({
      explainWorkflow: { summary: '', description: 'done', routeExplanations: [], diagram: null, warnings: [] }
    });
    expect(await screen.findByText('done')).toBeInTheDocument();
  });
});
