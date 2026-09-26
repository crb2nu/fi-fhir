/**
 * AlertsPanel tests — the home alert list renders the shared observability
 * store and nothing else: no source means the honest unconfigured state, never
 * demo alerts.
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
  alertSource,
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
  alertSource.set('unconfigured');
});

describe('AlertsPanel', () => {
  it('fetches on mount and renders firing alerts from a live source', () => {
    alertSource.set('live');
    observabilityState.update((s) => ({
      ...s,
      alerts: [alert(), alert({ id: 'a2', state: 'resolved', name: 'ResolvedAlert' })],
    }));

    render(AlertsPanel);

    expect(fetchAlerts).toHaveBeenCalled();
    expect(screen.getByRole('table', { name: 'Firing alerts' })).toBeInTheDocument();
    expect(screen.getByText('HighErrorRate')).toBeInTheDocument();
    expect(screen.getByText(/ORM-routing error rate/)).toBeInTheDocument();
    expect(screen.getByText('Critical')).toBeInTheDocument();
    // Non-firing alerts stay out of the list.
    expect(screen.queryByText('ResolvedAlert')).toBeNull();
  });

  it('renders the unconfigured empty state', () => {
    render(AlertsPanel);

    expect(screen.getByText('No alert source configured.')).toBeInTheDocument();
    expect(screen.getByTestId('alerts-panel')).toHaveAttribute('data-source', 'unconfigured');
    expect(screen.queryByRole('table')).toBeNull();
    expect(screen.queryByText(/demo data/i)).toBeNull();
  });

  it('never shows alerts the store holds while there is no source', () => {
    observabilityState.update((s) => ({ ...s, alerts: [alert()] }));

    render(AlertsPanel);

    expect(screen.queryByText('HighErrorRate')).toBeNull();
    expect(screen.getByText('No alert source configured.')).toBeInTheDocument();
  });

  it('shows an honest empty state on a live-but-quiet source', () => {
    alertSource.set('live');
    render(AlertsPanel);

    expect(screen.getByText('No active alerts.')).toBeInTheDocument();
    expect(screen.queryByText(/demo data/i)).toBeNull();
  });

  it('says so when the connected source fails, with a retry', () => {
    alertSource.set('unavailable');
    render(AlertsPanel);

    expect(screen.getByText('The alert source did not answer.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument();
  });
});
