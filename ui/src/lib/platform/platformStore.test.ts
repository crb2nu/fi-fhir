import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';

// Must mock before importing the module
vi.mock('$app/environment', () => ({ browser: true }));
vi.mock('./config', () => ({
  PLATFORM_CONFIG: {
    endpoint: 'http://localhost:8080/mcp',
    token: '',
    agentId: 'mapping-studio',
    enabled: false,
  },
}));

import { platformState, initializePlatform, teardownPlatform } from './platformStore';

describe('platformStore', () => {
  beforeEach(() => {
    teardownPlatform();
  });

  it('starts in disconnected state', () => {
    const state = get(platformState);
    expect(state.connected).toBe(false);
    expect(state.connecting).toBe(false);
    expect(state.sessionId).toBeNull();
  });

  it('has correct agentId', () => {
    const state = get(platformState);
    expect(state.agentId).toBe('mapping-studio');
  });

  it('does nothing when platform is disabled', async () => {
    await initializePlatform();
    const state = get(platformState);
    // When disabled, initializePlatform returns early
    expect(state.connected).toBe(false);
    expect(state.connecting).toBe(false);
  });

  it('resets state on teardown', async () => {
    await teardownPlatform();
    const state = get(platformState);
    expect(state.connected).toBe(false);
    expect(state.sessionId).toBeNull();
    expect(state.error).toBeNull();
  });
});
