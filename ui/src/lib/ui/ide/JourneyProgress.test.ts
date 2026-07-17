import { describe, expect, it, vi } from 'vitest';
import { render, screen, within } from '@testing-library/svelte';

vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));

const { default: JourneyProgress } = await import('./JourneyProgress.svelte');

// The compact banner renders two surfaces for the same journey state: the
// five-across card grid (>=760px) and a single-line strip (<760px). jsdom
// applies no media queries, so these tests pin the content of both surfaces;
// which one is visible at a given width is CSS-only.
describe('JourneyProgress', () => {
  it('marks the stage matching the pathname as current in the card grid', () => {
    const { container } = render(JourneyProgress, {
      props: { pathname: '/profiles', variant: 'compact' }
    });

    const current = container.querySelector('.stage-card[aria-current="step"]');
    expect(current).not.toBeNull();
    expect(current).toHaveTextContent('Normalization');
    expect(container.querySelectorAll('.stage-card')).toHaveLength(5);
    expect(container.querySelectorAll('.stage-card.complete')).toHaveLength(1);
  });

  it('renders the small-screen strip with stage count, title, and next action', () => {
    const { container } = render(JourneyProgress, {
      props: { pathname: '/profiles', variant: 'compact' }
    });

    const strip = container.querySelector('.journey-strip');
    expect(strip).not.toBeNull();

    const scoped = within(strip as HTMLElement);
    expect(scoped.getByText('2/5')).toBeInTheDocument();
    expect(scoped.getByText('Normalization')).toBeInTheDocument();

    // The strip drops the "Continue to" verb prefix to fit one line at 375px.
    const next = scoped.getByRole('link', { name: /Next up/ });
    expect(next).toHaveTextContent('Translation');
    expect(next).not.toHaveTextContent('Continue to');
    expect(next).toHaveAttribute('href', '/terminology');
  });

  it('omits the stage count on the strip for mission control', () => {
    const { container } = render(JourneyProgress, {
      props: { pathname: '/', variant: 'compact' }
    });

    const strip = container.querySelector('.journey-strip');
    expect(strip).not.toBeNull();

    const scoped = within(strip as HTMLElement);
    expect(scoped.getByText('Mission control')).toBeInTheDocument();
    expect((strip as HTMLElement).querySelector('.strip-count')).toBeNull();
  });

  it('does not render the strip for the full variant', () => {
    const { container } = render(JourneyProgress, {
      props: { pathname: '/profiles', variant: 'full' }
    });

    expect(container.querySelector('.journey-strip')).toBeNull();
    expect(screen.getByRole('heading', { name: 'Normalization' })).toBeInTheDocument();
  });
});
