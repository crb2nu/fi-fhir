/**
 * Tests for observabilityStore presentation helpers.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';

const { mockClient } = vi.hoisted(() => ({
  mockClient: { isConnected: vi.fn(), callTool: vi.fn() },
}));

vi.mock('$lib/platform', () => ({
  getPlatformClient: () => mockClient,
  platformState: {
    subscribe: (run: (v: { connected: boolean }) => void) => {
      run({ connected: false });
      return () => {};
    },
  },
}));

import {
  severityLabel,
  fetchAlerts,
  isSimulated,
  type Alert,
} from './observabilityStore';

describe('severityLabel', () => {
  it('returns a human-readable label for each severity', () => {
    expect(severityLabel('critical')).toBe('Critical');
    expect(severityLabel('warning')).toBe('Warning');
    expect(severityLabel('info')).toBe('Info');
  });

  it('provides a non-color text cue for every Alert severity value (WCAG 1.4.1)', () => {
    const severities: Array<Alert['severity']> = ['critical', 'warning', 'info'];
    for (const severity of severities) {
      const label = severityLabel(severity);
      expect(label.length).toBeGreaterThan(0);
      // Label must be distinct, non-empty text — not a color or class token.
      expect(label).not.toMatch(/^#|rgb|var\(/);
    }
  });

  it('falls back to "Info" for an unexpected value', () => {
    expect(severityLabel('unknown' as Alert['severity'])).toBe('Info');
  });
});

describe('isSimulated flag', () => {
  beforeEach(() => {
    isSimulated.set(false);
    mockClient.isConnected.mockReset();
    mockClient.callTool.mockReset();
  });

  it('is true after a fetch falls back to mock data (platform disconnected)', async () => {
    mockClient.isConnected.mockReturnValue(false);
    await fetchAlerts();
    expect(get(isSimulated)).toBe(true);
  });

  it('is false after a fetch returns real backend data', async () => {
    mockClient.isConnected.mockReturnValue(true);
    mockClient.callTool.mockResolvedValue([
      { id: 'real-1', name: 'Real alert', severity: 'info', state: 'firing', summary: 's', startsAt: 0, labels: {} },
    ] satisfies Alert[]);
    await fetchAlerts();
    expect(get(isSimulated)).toBe(false);
  });

  it('flips back to true when a later fetch loses the backend connection', async () => {
    mockClient.isConnected.mockReturnValue(true);
    mockClient.callTool.mockResolvedValue([
      { id: 'real-1', name: 'Real alert', severity: 'info', state: 'firing', summary: 's', startsAt: 0, labels: {} },
    ] satisfies Alert[]);
    await fetchAlerts();
    expect(get(isSimulated)).toBe(false);

    mockClient.isConnected.mockReturnValue(false);
    await fetchAlerts();
    expect(get(isSimulated)).toBe(true);
  });
});
