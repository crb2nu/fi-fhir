import { afterEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';

vi.mock('$app/environment', () => ({ browser: true }));
vi.mock('./config', () => ({
  PLATFORM_CONFIG: {
    endpoint: 'http://localhost:8080/mcp',
    token: '',
    agentId: 'mapping-studio',
    enabled: false
  }
}));

import { hudEvents, subscribeHudEvents } from './hudEvents';

afterEach(() => {
  vi.useRealTimers();
  hudEvents.set([]);
});

describe('subscribeHudEvents', () => {
  it('produces nothing — not even simulated events — when no platform is configured', () => {
    vi.useFakeTimers();
    const stop = subscribeHudEvents();

    vi.advanceTimersByTime(30_000);

    expect(get(hudEvents)).toEqual([]);
    stop();
  });
});
