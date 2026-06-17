import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import BottomPanel from './BottomPanel.svelte';
import { workflowDraft } from '$lib/features/workflows/workflowStore';

// The Problems-tab badge must track the same live signal the ProblemsPanel
// renders (`workflowDiagnostics`), not the now-removed generic diagnosticsStore.
// These tests pin the badge to the workflow-draft validity state.
describe('BottomPanel problems badge', () => {
  beforeEach(() => {
    workflowDraft.reset();
  });

  afterEach(() => {
    workflowDraft.reset();
  });

  it('shows the problem count on the Problems tab when the workflow draft is invalid', () => {
    // Empty name + zero routes => 2 validation errors.
    workflowDraft.loadDraft({
      name: '',
      version: '1.0',
      routes: []
    });

    render(BottomPanel);

    const badge = screen.getByLabelText('2 problems');
    expect(badge).toBeInTheDocument();
    expect(badge).toHaveTextContent('2');
    // All workflow diagnostics are severity 'error' -> danger variant.
    expect(badge).toHaveClass('danger');
  });

  it('hides the badge when the workflow draft is valid', () => {
    workflowDraft.loadDraft({
      name: 'adt-routing',
      version: '1.0',
      routes: [
        {
          _key: 'route-1',
          name: 'Admission route',
          filter: {
            eventTypes: ['PATIENT_ADMIT'],
            sources: [],
            condition: ''
          },
          transforms: [],
          actions: [
            {
              _key: 'action-1',
              type: 'log',
              config: {
                message: 'received'
              }
            }
          ],
          expanded: true
        }
      ]
    });

    render(BottomPanel);

    expect(screen.queryByLabelText(/problems$/)).not.toBeInTheDocument();
  });
});
