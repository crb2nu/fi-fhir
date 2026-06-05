/**
 * Tests for AlertBadge — focused on the non-color severity cue (WCAG 1.4.1).
 *
 * Alert severity must be conveyed by a visible text label, not the colored
 * marker alone. The previous design rendered an aria-hidden colored dot with
 * no accompanying severity text.
 */
import { describe, it, expect, afterEach, vi } from 'vitest';
import { render, cleanup, fireEvent } from '@testing-library/svelte';

// Stub the unconditional onMount fetch so seeded alerts survive the render.
vi.mock('./observabilityStore', async (importActual) => {
  const actual = await importActual<typeof import('./observabilityStore')>();
  return { ...actual, fetchAlerts: vi.fn().mockResolvedValue(undefined) };
});

import AlertBadge from './AlertBadge.svelte';
import { observabilityState, type Alert } from './observabilityStore';

const alerts: Alert[] = [
  { id: 'a1', name: 'Disk almost full', severity: 'critical', state: 'firing', summary: 'Node-1 disk at 95%', startsAt: 1_700_000_000_000, labels: {} },
  { id: 'a2', name: 'High latency', severity: 'warning', state: 'firing', summary: 'p99 above SLO', startsAt: 1_700_000_000_000, labels: {} },
  { id: 'a3', name: 'Deploy started', severity: 'info', state: 'pending', summary: 'Rollout in progress', startsAt: 1_700_000_000_000, labels: {} }
];

function seed(rows: Alert[] = alerts): void {
  observabilityState.set({
    metrics: null,
    logs: [],
    alerts: rows,
    isLoadingMetrics: false,
    isLoadingLogs: false,
    logFilter: {},
    error: null
  } as Parameters<typeof observabilityState.set>[0]);
}

afterEach(() => {
  cleanup();
  seed([]);
});

describe('AlertBadge severity cue', () => {
  it('renders a visible severity text label for each alert in the dropdown', async () => {
    seed();
    const { container, getByText } = render(AlertBadge);

    await fireEvent.click(container.querySelector('.badge-trigger') as HTMLElement);

    expect(getByText('Critical')).toBeInTheDocument();
    expect(getByText('Warning')).toBeInTheDocument();
    expect(getByText('Info')).toBeInTheDocument();
  });

  it('applies a per-severity class so the cue is not color-only', async () => {
    seed();
    const { container } = render(AlertBadge);

    await fireEvent.click(container.querySelector('.badge-trigger') as HTMLElement);

    expect(container.querySelector('.severity-tag.severity-critical')).not.toBeNull();
    expect(container.querySelector('.severity-tag.severity-warning')).not.toBeNull();
    expect(container.querySelector('.severity-tag.severity-info')).not.toBeNull();
  });

  it('no longer renders an aria-hidden color-only severity dot', async () => {
    seed();
    const { container } = render(AlertBadge);

    await fireEvent.click(container.querySelector('.badge-trigger') as HTMLElement);

    expect(container.querySelector('.severity-dot')).toBeNull();
  });
});
