/**
 * LLM capability store — honest availability state for the Copilot.
 *
 * Probes the backend's read-only `llmCapability` query (`.loom/23` Slice 3f)
 * so the panel can show a real disabled/unavailable/degraded state instead of
 * inferring health from operation failures. The probe is silent and fails
 * OPEN: 'unknown' (never probed, or probe failed) does not block any action —
 * a real dispatch will surface its own error honestly. Only a definitive
 * backend answer (top-level disabled/unavailable, or the action's own feature
 * row disabled) blocks dispatch.
 */
import { writable } from 'svelte/store';
import { graphqlFetch } from '$lib/graphql/client';
import { LlmCapabilityDocument, type LlmCapabilityQuery } from '$lib/gen/graphql';
import type { CopilotAction } from './copilotStore';

export type LlmCapabilitySnapshot = LlmCapabilityQuery['llmCapability'];

/** Backend status vocabulary plus 'unknown' for the unprobed/probe-failed state. */
export type LlmStatus = 'unknown' | 'available' | 'degraded' | 'disabled' | 'unavailable';

export interface LlmCapabilityState {
  status: LlmStatus;
  capability: LlmCapabilitySnapshot | null;
}

const initialState: LlmCapabilityState = { status: 'unknown', capability: null };

export const llmCapabilityState = writable<LlmCapabilityState>(initialState);

const KNOWN_STATUSES: ReadonlySet<string> = new Set([
  'available',
  'degraded',
  'disabled',
  'unavailable'
]);

/** Copilot action → backend feature-row name (`LLMFeatureCapability.name`). */
export const ACTION_FEATURES: Record<CopilotAction, string> = {
  explain: 'explainWorkflow',
  suggest: 'suggestMappings',
  generate: 'generateWorkflow',
  review: 'analyzeQuality'
};

/**
 * Probes `llmCapability`. No toast on failure — a background probe blip must
 * not spend toast budget; the state just resets to 'unknown' (fail-open).
 */
export async function refreshLlmCapability(): Promise<void> {
  try {
    const res = await graphqlFetch(LlmCapabilityDocument, undefined, { showErrorToast: false });
    const capability = res.llmCapability;
    const status = KNOWN_STATUSES.has(capability.status)
      ? (capability.status as LlmStatus)
      : 'unknown';
    llmCapabilityState.set({ status, capability });
  } catch {
    llmCapabilityState.set({ status: 'unknown', capability: null });
  }
}

/** Resets to the unprobed state (used on disconnect and in tests). */
export function resetLlmCapability(): void {
  llmCapabilityState.set(initialState);
}

/**
 * Returns the reason `action` cannot run right now, or null when it can.
 * 'unknown' and 'degraded' fail open; a degraded runtime may still serve the
 * selected action unless its own feature row says otherwise.
 */
export function actionBlockReason(
  state: LlmCapabilityState,
  action: CopilotAction
): string | null {
  const cap = state.capability;
  if (!cap) return null;
  if (state.status === 'disabled') {
    return cap.warnings[0] ?? 'LLM features are disabled on this server';
  }
  if (state.status === 'unavailable') {
    return cap.warnings[0] ?? 'LLM provider is not configured or unreachable';
  }
  const feature = cap.features.find((f) => f.name === ACTION_FEATURES[action]);
  if (feature && !feature.enabled) {
    return feature.reason?.trim() || `The ${action} action is unavailable (${feature.status})`;
  }
  return null;
}
