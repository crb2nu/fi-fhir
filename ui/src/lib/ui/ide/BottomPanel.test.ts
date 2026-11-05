import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import BottomPanel from './BottomPanel.svelte';
import { workflowDraft } from '$lib/features/workflows/workflowStore';
import {
  llmCapabilityState,
  resetLlmCapability,
  type LlmCapabilitySnapshot
} from '$lib/features/copilot/llmCapabilityStore';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';

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

  it('shows no badge on a fresh session (empty default draft)', () => {
    render(BottomPanel);

    expect(screen.queryByTestId('problems-badge')).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/problems$/)).not.toBeInTheDocument();
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

// A dimmed Copilot tab must say why, from the same capability stores the
// Copilot panel reads: the status endpoint's `llm.configured` and the probe.
describe('BottomPanel Copilot dim reason', () => {
  function capability(overrides: Partial<LlmCapabilitySnapshot> = {}): LlmCapabilitySnapshot {
    return {
      enabled: true,
      configured: true,
      providerBaseURLHost: 'litellm.ai.svc.cluster.local:8000',
      defaultModel: 'gemma4-e4b-radeonvii',
      qualityModel: null,
      status: 'available',
      warnings: [],
      features: [],
      ...overrides
    };
  }

  function copilotTab(): HTMLElement {
    return screen.getByRole('tab', { name: 'Copilot' });
  }

  beforeEach(() => {
    resetLlmCapability();
    resetAccessCapabilities();
  });

  afterEach(() => {
    resetLlmCapability();
    resetAccessCapabilities();
  });

  it('carries no reason while the LLM is available or unprobed', () => {
    render(BottomPanel);
    expect(copilotTab()).not.toHaveClass('dimmed');
    expect(copilotTab()).not.toHaveAttribute('title');
    expect(copilotTab()).not.toHaveAttribute('aria-description');

    llmCapabilityState.set({ status: 'available', capability: capability() });
    expect(copilotTab()).not.toHaveAttribute('title');
  });

  it('says no LLM is configured when the status endpoint reports none', async () => {
    setAccessStatus({
      authenticated: true,
      authVia: 'bearer',
      capabilities: { operatorRead: true, llm: { configured: false } }
    });
    render(BottomPanel);
    expect(copilotTab()).toHaveClass('dimmed');
    expect(copilotTab()).toHaveAttribute('title', 'No LLM is configured for this deployment');
    expect(copilotTab()).toHaveAttribute('aria-description', 'No LLM is configured for this deployment');
  });

  it('says no LLM is configured when the probe reports LLM features disabled', () => {
    llmCapabilityState.set({
      status: 'disabled',
      capability: capability({
        enabled: false,
        status: 'disabled',
        warnings: ['LLM features disabled; set FI_FHIR_LLM_ENABLED=true to enable']
      })
    });
    render(BottomPanel);
    expect(copilotTab()).toHaveClass('dimmed');
    expect(copilotTab()).toHaveAttribute('title', 'No LLM is configured for this deployment');
  });

  it('gives the capability warning (permission or provider) when the LLM is unavailable', () => {
    llmCapabilityState.set({
      status: 'unavailable',
      capability: capability({
        status: 'unavailable',
        warnings: ['Your roles do not include graphql:operator']
      })
    });
    render(BottomPanel);
    expect(copilotTab()).toHaveClass('dimmed');
    expect(copilotTab()).toHaveAttribute('title', 'Your roles do not include graphql:operator');
    expect(copilotTab()).toHaveAttribute('aria-description', 'Your roles do not include graphql:operator');
  });

  it('falls back to "not responding" when the unavailable LLM gives no warning', () => {
    llmCapabilityState.set({ status: 'unavailable', capability: capability({ status: 'unavailable' }) });
    render(BottomPanel);
    expect(copilotTab()).toHaveAttribute('title', "The deployment's LLM is not responding");
  });
});
