import { beforeEach, describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import ProblemsPanel from './ProblemsPanel.svelte';
import {
  resetWorkflowBuilderOpened,
  workflowDraft
} from '$lib/features/workflows/workflowStore';

describe('ProblemsPanel', () => {
  beforeEach(() => {
    workflowDraft.reset();
    resetWorkflowBuilderOpened();
  });

  it('explains where problems come from on a fresh session instead of listing the empty draft', () => {
    render(ProblemsPanel);

    const empty = screen.getByTestId('problems-empty');
    expect(empty).toHaveTextContent('No problems');
    expect(empty).toHaveTextContent(/workflow draft you edit in the\s+Workflows builder/);
    expect(empty).toHaveTextContent('Integration Session runs');
    expect(screen.queryByText('Workflow draft needs attention')).not.toBeInTheDocument();
  });

  it('renders structured diagnostics when the workflow draft is invalid', () => {
    workflowDraft.loadDraft({
      name: '',
      version: '1.0',
      routes: []
    });

    render(ProblemsPanel);

    expect(screen.getByText('Workflow draft needs attention')).toBeInTheDocument();
    expect(screen.getByText('2 errors')).toBeInTheDocument();
    expect(screen.getByText('Fix the listed issues before you trust the destination behavior.')).toBeInTheDocument();
    expect(screen.getAllByText('Workflow')).toHaveLength(2);
    expect(screen.getByText('At least one route is required')).toBeInTheDocument();
  });

  it('renders a structured empty state when the workflow draft is valid', () => {
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

    render(ProblemsPanel);

    expect(screen.getByText('Ready for runtime verification')).toBeInTheDocument();
    expect(screen.getByText('No blocking problems detected.')).toBeInTheDocument();
    expect(screen.getByText('1 route')).toBeInTheDocument();
    expect(screen.getByText('1 action')).toBeInTheDocument();
  });
});
