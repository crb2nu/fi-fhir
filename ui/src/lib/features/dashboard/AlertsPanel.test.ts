/**
 * AlertsPanel tests — the dashboard alert list must render the shared
 * observability store (not hardcoded fiction) and label simulated data.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';

vi.mock('$lib/features/observability/observabilityStore', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('$lib/features/observability/observabilityStore')>();
  return { ...actual, fetchAlerts: vi.fn().mockResolvedValue(undefined) };
});

import {
  observabilityState,
  isSimulated,
  fetchAlerts,
  type Alert,
} from '$lib/features/observability/observabilityStore';
import AlertsPanel from './AlertsPanel.svelte';

function alert(overrides: Partial<Alert> = {}): Alert {
  return {
    id: 'a1',
    name: 'HighErrorRate',
    severity: 'critical',
    state: 'firing',
    summary: 'ORM-routing error rate above 2% for 5 minutes',
    description: undefined,
    startsAt: 0,
    labels: {},
    ...overrides,
  };
}

beforeEach(() => {
  vi.mocked(fetchAlerts).mockClear();
  observabilityState.update((s) => ({ ...s, alerts: [] }));
  isSimulated.set(false);
});

describe('AlertsPanel', () => {
  it('fetches on mount and renders firing alerts from the store', () => {
    observabilityState.update((s) => ({
      ...s,
      alerts: [alert(), alert({ id: 'a2', state: 'resolved', name: 'ResolvedAlert' })],
    }));

    render(AlertsPanel);

    expect(fetchAlerts).toHaveBeenCalled();
    expect(screen.getByText('HighErrorRate')).toBeInTheDocument();
    expect(screen.getByText(/ORM-routing error rate/)).toBeInTheDocument();
    // Non-firing alerts stay out of the "Active Alerts" list.
    expect(screen.queryByText('ResolvedAlert')).toBeNull();
  });

  it('labels simulated data with a Demo data tag', () => {
    observabilityState.update((s) => ({ ...s, alerts: [alert()] }));
    isSimulated.set(true);

    render(AlertsPanel);

    expect(screen.getByText('Demo data')).toBeInTheDocument();
  });

  it('shows an honest empty state with no demo tag on live-but-quiet data', () => {
    render(AlertsPanel);

    expect(screen.getByText('No active alerts.')).toBeInTheDocument();
    expect(screen.queryByText('Demo data')).toBeNull();
  });
});
