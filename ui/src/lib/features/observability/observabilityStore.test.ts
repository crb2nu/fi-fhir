/**
 * Tests for observabilityStore: presentation helpers and the honest alert source.
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
  fetchMetrics,
  fetchLogs,
  alertSource,
  observabilityState,
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

const realAlert: Alert = {
  id: 'real-1',
  name: 'Real alert',
  severity: 'info',
  state: 'firing',
  summary: 's',
  startsAt: 0,
  labels: {},
};

describe('alert source (no simulated fallback)', () => {
  beforeEach(() => {
    alertSource.set('unconfigured');
    observabilityState.update((s) => ({ ...s, alerts: [], metrics: null, logs: [] }));
    mockClient.isConnected.mockReset();
    mockClient.callTool.mockReset();
  });

  it('is unconfigured with no alerts when the platform is not connected, and calls nothing', async () => {
    mockClient.isConnected.mockReturnValue(false);
    await fetchAlerts();
    expect(get(alertSource)).toBe('unconfigured');
    expect(get(observabilityState).alerts).toEqual([]);
    expect(mockClient.callTool).not.toHaveBeenCalled();
  });

  it('is live and carries exactly the backend alerts when Alertmanager answers', async () => {
    mockClient.isConnected.mockReturnValue(true);
    mockClient.callTool.mockResolvedValue([realAlert] satisfies Alert[]);
    await fetchAlerts();
    expect(get(alertSource)).toBe('live');
    expect(get(observabilityState).alerts).toEqual([realAlert]);
  });

  it('is unavailable with no alerts when the connected platform fails the query', async () => {
    mockClient.isConnected.mockReturnValue(true);
    mockClient.callTool.mockRejectedValue(new Error('alertmanager unreachable'));
    await fetchAlerts();
    expect(get(alertSource)).toBe('unavailable');
    expect(get(observabilityState).alerts).toEqual([]);
  });

  it('drops a previous live list when the platform disconnects', async () => {
    mockClient.isConnected.mockReturnValue(true);
    mockClient.callTool.mockResolvedValue([realAlert] satisfies Alert[]);
    await fetchAlerts();
    expect(get(observabilityState).alerts).toHaveLength(1);

    mockClient.isConnected.mockReturnValue(false);
    await fetchAlerts();
    expect(get(alertSource)).toBe('unconfigured');
    expect(get(observabilityState).alerts).toEqual([]);
  });

  it('never invents metrics or logs without a platform', async () => {
    mockClient.isConnected.mockReturnValue(false);
    await fetchMetrics();
    await fetchLogs();
    const state = get(observabilityState);
    expect(state.metrics).toBeNull();
    expect(state.logs).toEqual([]);
    expect(mockClient.callTool).not.toHaveBeenCalled();
  });
});
