/**
 * CopilotPanel honest LLM-state tests — the panel must surface the backend's
 * `llmCapability` verdict (disabled/unavailable/degraded) instead of letting
 * doomed actions fire, and must not block when the runtime is available.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';

vi.mock('$lib/graphql/client', () => ({
  graphqlFetch: vi.fn(),
  isErrorToasted: vi.fn(() => true)
}));

import { graphqlFetch } from '$lib/graphql/client';
import { platformState } from '$lib/platform';
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
  platformState.update((s) => ({ ...s, connected: true }));
});

describe('CopilotPanel LLM capability state', () => {
  it('shows no status chip and allows sending when the runtime is available', async () => {
    mockFetch.mockResolvedValue({ llmCapability: capability() });
    render(CopilotPanel);

    await waitFor(() => expect(mockFetch).toHaveBeenCalled());
    expect(screen.queryByRole('status')).toBeNull();

    const textarea = screen.getByRole('textbox');
    await fireEvent.input(textarea, { target: { value: 'PID|1||12345' } });

    const send = screen.getByTitle('Send (Enter)');
    expect(send).not.toBeDisabled();
  });

  it('shows the LLM off chip and blocks sending when the backend reports disabled', async () => {
    mockFetch.mockResolvedValue({
      llmCapability: capability({
        enabled: false,
        configured: false,
        status: 'disabled',
        warnings: ['LLM features are disabled'],
        features: []
      })
    });
    render(CopilotPanel);

    const chip = await screen.findByText('LLM off');
    expect(chip).toHaveAttribute('title', 'LLM features are disabled');

    const textarea = screen.getByRole('textbox');
    await fireEvent.input(textarea, { target: { value: 'explain this segment' } });

    // Send stays disabled and explains why (visible note + button title).
    const send = screen.getByRole('button', { name: 'LLM features are disabled' });
    expect(send).toBeDisabled();
    expect(screen.getByText('LLM features are disabled')).toBeInTheDocument();
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

    const textarea = screen.getByRole('textbox');
    await fireEvent.input(textarea, { target: { value: 'route ADT admits to FHIR' } });

    // Default action is Explain, whose feature row is enabled — sending works.
    expect(screen.getByTitle('Send (Enter)')).not.toBeDisabled();

    // Switching to Generate (feature off) blocks with the backend's reason.
    await fireEvent.click(screen.getByRole('button', { name: 'Generate' }));
    const send = await screen.findByRole('button', { name: 'Workflow copilot module not wired' });
    expect(send).toBeDisabled();
  });

  it('fails open when the capability probe errors (unknown state, no chip, no block)', async () => {
    mockFetch.mockRejectedValue(new Error('probe failed'));
    render(CopilotPanel);

    await waitFor(() => expect(mockFetch).toHaveBeenCalled());

    const textarea = screen.getByRole('textbox');
    await fireEvent.input(textarea, { target: { value: 'some input' } });

    expect(screen.getByTitle('Send (Enter)')).not.toBeDisabled();
    expect(screen.queryByText(/LLM (off|degraded|unavailable)/)).toBeNull();
  });
});
