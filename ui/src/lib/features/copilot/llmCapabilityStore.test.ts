import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';

vi.mock('$lib/graphql/client', () => ({ graphqlFetch: vi.fn() }));

import { graphqlFetch } from '$lib/graphql/client';
import { LlmCapabilityDocument } from '$lib/gen/graphql';
import {
  llmCapabilityState,
  refreshLlmCapability,
  resetLlmCapability,
  actionBlockReason,
  ACTION_FEATURES,
  type LlmCapabilitySnapshot,
} from './llmCapabilityStore';

const mockFetch = graphqlFetch as unknown as ReturnType<typeof vi.fn>;

function feature(name: string, overrides: Partial<LlmCapabilitySnapshot['features'][0]> = {}) {
  return { name, enabled: true, status: 'available', reason: null, model: null, ...overrides };
}

function capability(overrides: Partial<LlmCapabilitySnapshot> = {}): LlmCapabilitySnapshot {
  return {
    enabled: true,
    configured: true,
    providerBaseURLHost: 'litellm.ai.svc.cluster.local:8000',
    defaultModel: 'gemma4-e4b-radeonvii',
    qualityModel: 'gemma4-26b-a4b-gptq',
    status: 'available',
    warnings: [],
    features: Object.values(ACTION_FEATURES).map((name) => feature(name)),
    ...overrides,
  };
}

beforeEach(() => {
  mockFetch.mockReset();
  resetLlmCapability();
});

describe('refreshLlmCapability', () => {
  it('probes silently and stores a known status', async () => {
    const cap = capability();
    mockFetch.mockResolvedValue({ llmCapability: cap });

    await refreshLlmCapability();

    expect(mockFetch).toHaveBeenCalledWith(LlmCapabilityDocument, undefined, {
      showErrorToast: false,
    });
    expect(get(llmCapabilityState)).toEqual({ status: 'available', capability: cap });
  });

  it('resets to unknown when the probe fails (fail-open, no throw)', async () => {
    mockFetch.mockResolvedValue({ llmCapability: capability({ status: 'disabled' }) });
    await refreshLlmCapability();
    expect(get(llmCapabilityState).status).toBe('disabled');

    mockFetch.mockRejectedValue(new Error('network down'));
    await expect(refreshLlmCapability()).resolves.toBeUndefined();
    expect(get(llmCapabilityState)).toEqual({ status: 'unknown', capability: null });
  });

  it('maps an unrecognized backend status to unknown but keeps the payload', async () => {
    const cap = capability({ status: 'future-status' });
    mockFetch.mockResolvedValue({ llmCapability: cap });

    await refreshLlmCapability();

    expect(get(llmCapabilityState).status).toBe('unknown');
    expect(get(llmCapabilityState).capability).toEqual(cap);
  });
});

describe('actionBlockReason', () => {
  it('does not block before a probe has resolved (unknown, no capability)', () => {
    expect(actionBlockReason(get(llmCapabilityState), 'generate')).toBeNull();
  });

  it('does not block when the runtime is available', () => {
    const state = { status: 'available' as const, capability: capability() };
    for (const action of ['explain', 'suggest', 'generate', 'review'] as const) {
      expect(actionBlockReason(state, action)).toBeNull();
    }
  });

  it('blocks every action when LLM features are disabled, using the backend warning', () => {
    const state = {
      status: 'disabled' as const,
      capability: capability({ status: 'disabled', warnings: ['LLM features are disabled'] }),
    };
    expect(actionBlockReason(state, 'generate')).toBe('LLM features are disabled');
  });

  it('blocks with a default message when unavailable and no warning is present', () => {
    const state = {
      status: 'unavailable' as const,
      capability: capability({ status: 'unavailable', warnings: [] }),
    };
    expect(actionBlockReason(state, 'review')).toMatch(/not configured or unreachable/);
  });

  it('blocks only the action whose feature row is off when degraded', () => {
    const state = {
      status: 'degraded' as const,
      capability: capability({
        status: 'degraded',
        features: [
          feature('generateWorkflow', {
            enabled: false,
            status: 'unconfigured',
            reason: 'Workflow copilot module not wired',
          }),
          feature('explainWorkflow'),
          feature('suggestMappings'),
          feature('analyzeQuality'),
        ],
      }),
    };
    expect(actionBlockReason(state, 'generate')).toBe('Workflow copilot module not wired');
    expect(actionBlockReason(state, 'explain')).toBeNull();
    expect(actionBlockReason(state, 'suggest')).toBeNull();
    expect(actionBlockReason(state, 'review')).toBeNull();
  });

  it('falls back to a status-derived message when a disabled feature has no reason', () => {
    const state = {
      status: 'degraded' as const,
      capability: capability({
        status: 'degraded',
        features: [feature('analyzeQuality', { enabled: false, status: 'unconfigured', reason: '' })],
      }),
    };
    expect(actionBlockReason(state, 'review')).toBe('The review action is unavailable (unconfigured)');
  });
});
