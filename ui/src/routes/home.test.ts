import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';

vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));

// The default on-demand tab mounts RecentEventsFeed, which queries events on
// mount; the health section's AlertsPanel fetches alerts. Mock both data
// boundaries so the page renders deterministically.
vi.mock('$lib/features/events/eventsApi', () => ({
  queryEvents: vi.fn().mockResolvedValue({ events: { edges: [] } })
}));
vi.mock('$lib/features/observability/observabilityStore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/features/observability/observabilityStore')>();
  return { ...actual, fetchAlerts: vi.fn().mockResolvedValue(undefined) };
});

const { default: HomePage } = await import('./+page.svelte');

describe('home dashboard (status console)', () => {
  it('leads with the adaptive hero and recommended move', () => {
    render(HomePage);

    expect(
      screen.getByRole('heading', { name: 'Build the interface from source to destination' })
    ).toBeInTheDocument();
    expect(screen.getByText('Recommended move')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Start Source Intake' })).toBeInTheDocument();
  });

  it('defaults the on-demand panel to live data (Recent events)', () => {
    render(HomePage);

    const eventsTab = screen.getByRole('tab', { name: 'Recent events' });
    expect(eventsTab).toHaveAttribute('aria-selected', 'true');
  });

  it('does not render fabricated signals', () => {
    render(HomePage);

    // The hardcoded stage-health "Clean" row and the data-less Active
    // investigations column were removed — placeholders must not read
    // as live telemetry.
    expect(screen.queryByText('Clean')).toBeNull();
    expect(screen.queryByText('Active investigations')).toBeNull();
    expect(screen.queryByText('Five-stage guided workspace')).toBeNull();
  });

  it('keeps navigation cards reachable behind the Operator surfaces tab', async () => {
    render(HomePage);

    await fireEvent.click(screen.getByRole('tab', { name: 'Operator surfaces' }));

    expect(screen.getByRole('link', { name: /Source intake lab/ })).toHaveAttribute('href', '/hl7');
    expect(screen.getByRole('link', { name: /Workflow workbench/ })).toHaveAttribute(
      'href',
      '/workflows'
    );
  });
});
