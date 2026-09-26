/**
 * CopilotPanel honest LLM-state tests — the panel runs on the API's own LLM
 * and must surface the backend's `llmCapability` verdict (ready / not
 * responding / not configured) instead of letting doomed actions fire. The
 * optional loom platform connection plays no part.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';

vi.mock('$lib/graphql/client', () => ({
  graphqlFetch: vi.fn(),
  isErrorToasted: vi.fn(() => true)
}));

import { graphqlFetch } from '$lib/graphql/client';
import { PLATFORM_CONFIG, platformState } from '$lib/platform';
import { resetLlmCapability, ACTION_FEATURES, type LlmCapabilitySnapshot } from './llmCapabilityStore';
import { clearMessages } from './copilotStore';
import CopilotPanel from './CopilotPanel.svelte';

const mockFetch = graphqlFetch as unknown as ReturnType<typeof vi.fn>;

function capability(overrides: Partial<LlmCapabilitySnapshot> = {}): LlmCapabilitySnapshot {
  return {
    enabled: true,
    configured: true,
    providerBaseURLHost: 'litellm.ai.svc.cluster.local:8000',
    defaultModel: 'gemma4-e4b-radeonvii',
    qualityModel: null,
    status: 'available',
    warnings: [],
    features: Object.values(ACTION_FEATURES).map((name) => ({
      name,
      enabled: true,
      status: 'available',
      reason: null,
      model: null
    })),
    ...overrides
  };
}

beforeEach(() => {
  mockFetch.mockReset();
  resetLlmCapability();
  clearMessages();
});

describe('CopilotPanel runs on the backend LLM', () => {
  it('kill-test: configured-and-healthy LLM with the platform disabled is usable, no platform gate', async () => {
    expect(PLATFORM_CONFIG.enabled).toBe(false);
    expect(get(platformState).connected).toBe(false);
    mockFetch.mockResolvedValue({ llmCapability: capability() });

    render(CopilotPanel);

    // Probed on panel open — not on a platform connection that never comes.
    await waitFor(() => expect(mockFetch).toHaveBeenCalledTimes(1));
    const state = await screen.findByTestId('copilot-llm-state');
    await waitFor(() => expect(state).toHaveAttribute('data-state', 'ready'));
    expect(state).toHaveTextContent('Backend LLM ready');
    expect(state).toHaveTextContent('gemma4-e4b-radeonvii');
    expect(screen.queryByText(/Platform connection required/i)).toBeNull();
    expect(screen.queryByText(/Connect to the platform/i)).toBeNull();

    const textarea = screen.getByRole('textbox');
    expect(textarea).not.toBeDisabled();
    await fireEvent.input(textarea, { target: { value: 'PID|1||12345' } });
    expect(screen.getByTitle('Send (Enter)')).not.toBeDisabled();
    // No status role in the calm, ready state.
    expect(screen.queryByRole('status')).toBeNull();
  });

  it('says no LLM is configured and locks the input when the backend reports disabled', async () => {
    mockFetch.mockResolvedValue({
      llmCapability: capability({
        enabled: false,
        configured: false,
        status: 'disabled',
        warnings: ['LLM features disabled; set FI_FHIR_LLM_ENABLED=true to enable'],
        features: []
      })
    });
    render(CopilotPanel);

    const state = await screen.findByTestId('copilot-llm-state');
    await waitFor(() => expect(state).toHaveAttribute('data-state', 'not-configured'));
    expect(state).toHaveTextContent('No LLM is configured for this deployment');
    expect(state).toHaveTextContent('FI_FHIR_LLM_ENABLED');
    expect(screen.getByRole('textbox')).toBeDisabled();
    expect(screen.getByRole('button', { name: /FI_FHIR_LLM_ENABLED/ })).toBeDisabled();
    expect(screen.queryByText(/Platform connection required/i)).toBeNull();
  });

  it('says the LLM is not responding, with the capability warnings, when configured but unavailable', async () => {
    mockFetch.mockResolvedValue({
      llmCapability: capability({
        status: 'unavailable',
        warnings: ['LLM client unavailable: provider refused the connection'],
        features: []
      })
    });
    render(CopilotPanel);

    const state = await screen.findByTestId('copilot-llm-state');
    await waitFor(() => expect(state).toHaveAttribute('data-state', 'unreachable'));
    expect(state).toHaveTextContent("The deployment's LLM is not responding");
    expect(state).toHaveTextContent('provider refused the connection');
    expect(screen.getByRole('textbox')).toBeDisabled();
  });

  it('shows the degraded chip but only blocks the action whose feature is off', async () => {
    mockFetch.mockResolvedValue({
      llmCapability: capability({
        status: 'degraded',
        warnings: ['Workflow copilot module not wired'],
        features: [
          {
            name: 'generateWorkflow',
            enabled: false,
            status: 'unconfigured',
            reason: 'Workflow copilot module not wired',
            model: null
          },
          { name: 'explainWorkflow', enabled: true, status: 'available', reason: null, model: null },
          { name: 'suggestMappings', enabled: true, status: 'available', reason: null, model: null },
          { name: 'analyzeQuality', enabled: true, status: 'available', reason: null, model: null }
        ]
      })
    });
    render(CopilotPanel);

    await screen.findByText('LLM degraded');
    expect(screen.getByTestId('copilot-llm-state')).toHaveAttribute('data-state', 'ready');

    const textarea = screen.getByRole('textbox');
    await fireEvent.input(textarea, { target: { value: 'route ADT admits to FHIR' } });

    // Default action is Explain, whose feature row is enabled — sending works.
    expect(screen.getByTitle('Send (Enter)')).not.toBeDisabled();

    // Switching to Generate (feature off) blocks with the backend's reason.
    await fireEvent.click(screen.getByRole('button', { name: 'Generate' }));
    const send = await screen.findByRole('button', { name: 'Workflow copilot module not wired' });
    expect(send).toBeDisabled();
  });

  it('fails open when the capability probe errors (unknown state, no block)', async () => {
    mockFetch.mockRejectedValue(new Error('probe failed'));
    render(CopilotPanel);

    await waitFor(() => expect(mockFetch).toHaveBeenCalled());
    const state = screen.getByTestId('copilot-llm-state');
    await waitFor(() => expect(state).toHaveAttribute('data-state', 'unknown'));

    const textarea = screen.getByRole('textbox');
    await fireEvent.input(textarea, { target: { value: 'some input' } });

    expect(screen.getByTitle('Send (Enter)')).not.toBeDisabled();
    expect(screen.queryByText(/LLM degraded/)).toBeNull();
  });
});
