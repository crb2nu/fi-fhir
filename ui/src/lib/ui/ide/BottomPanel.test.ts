import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import BottomPanel from './BottomPanel.svelte';
import {
  markWorkflowBuilderOpened,
  resetWorkflowBuilderOpened,
  workflowDraft
} from '$lib/features/workflows/workflowStore';

// The Problems-tab badge must track the same live signal the ProblemsPanel
// renders (`workflowDiagnostics`), not the now-removed generic diagnosticsStore.
// These tests pin the badge to the workflow-draft validity state.
describe('BottomPanel problems badge', () => {
  beforeEach(() => {
    workflowDraft.reset();
    resetWorkflowBuilderOpened();
  });

  afterEach(() => {
    workflowDraft.reset();
    resetWorkflowBuilderOpened();
  });

  it('shows no badge on a fresh session (never-opened builder, empty default draft)', () => {
    render(BottomPanel);

    expect(screen.queryByTestId('problems-badge')).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/problems$/)).not.toBeInTheDocument();
  });

  it('counts the empty draft once the builder has been opened', () => {
    markWorkflowBuilderOpened();
    render(BottomPanel);

    expect(screen.getByTestId('problems-badge')).toHaveTextContent(/^\s*\d+\s*$/);
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
    expect(badge).toHaveAttribute('data-testid', 'problems-badge');
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

// The toggle chevron must match its aria-label: an open bottom drawer collapses
// downward (down chevron), a closed one expands upward (up chevron).
describe('BottomPanel toggle chevron direction', () => {
  function chevron(): string | null {
    const svg = screen.getByRole('button', { name: /panel$/ }).querySelector('svg');
    if (svg?.classList.contains('lucide-chevron-down')) return 'down';
    if (svg?.classList.contains('lucide-chevron-up')) return 'up';
    return null;
  }

  it('shows a down chevron when open (click to collapse)', () => {
    render(BottomPanel, { open: true });
    expect(screen.getByLabelText('Hide panel')).toBeInTheDocument();
    expect(chevron()).toBe('down');
  });

  it('shows an up chevron when closed (click to expand)', () => {
    render(BottomPanel, { open: false });
    expect(screen.getByLabelText('Show panel')).toBeInTheDocument();
    expect(chevron()).toBe('up');
  });
});

describe('BottomPanel tabs', () => {
  it('labels the Copilot tab in words only (no sparkle glyph)', () => {
    render(BottomPanel);
    const copilot = screen.getByRole('tab', { name: 'Copilot' });
    expect(copilot.textContent?.trim()).toBe('Copilot');
  });

  it('is one tab stop and moves focus with the arrow keys', async () => {
    render(BottomPanel, { activeTab: 'problems' });
    const tabs = screen.getAllByRole('tab');
    expect(tabs.filter((tab) => tab.getAttribute('tabindex') === '0')).toHaveLength(1);
    expect(screen.getByRole('tab', { name: /^Problems/ })).toHaveAttribute('tabindex', '0');

    tabs[1]!.focus();
    await fireEvent.keyDown(tabs[1]!, { key: 'ArrowRight' });
    expect(document.activeElement).toBe(tabs[2]);
    await fireEvent.keyDown(tabs[2]!, { key: 'End' });
    expect(document.activeElement).toBe(tabs[4]);
  });
});
