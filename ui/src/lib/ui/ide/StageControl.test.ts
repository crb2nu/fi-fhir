import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import { evidenceOf } from './__fixtures__/journeyEvidence';

vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));

const { default: StageControl } = await import('./StageControl.svelte');

describe('StageControl', () => {
  it('renders the five stages as links in order', () => {
    render(StageControl, { props: { pathname: '/profiles', evidence: null } });

    const nav = screen.getByRole('navigation', { name: 'Stages' });
    const links = Array.from(nav.querySelectorAll('a'));
    expect(links.map((link) => link.getAttribute('href'))).toEqual([
      '/hl7',
      '/profiles',
      '/terminology',
      '/workflows',
      '/events',
    ]);
    expect(links.map((link) => link.querySelector('.stage-label')?.textContent)).toEqual([
      'Source Intake',
      'Normalization',
      'Translation',
      'Delivery',
      'Verification',
    ]);
  });

  it('marks the route stage current and completes nothing before evidence arrives', () => {
    const { container } = render(StageControl, { props: { pathname: '/workflows/draft', evidence: null } });

    const current = container.querySelector('a[aria-current="step"]');
    expect(current).toHaveTextContent('Delivery');
    expect(current).toHaveClass('is-current');
    // Earlier stages are NOT complete because they come first.
    expect(container.querySelectorAll('a[data-state="complete"]')).toHaveLength(0);
    expect(container.querySelectorAll('.stage-check')).toHaveLength(0);
    expect(container.querySelectorAll('a[data-state="pending"]')).toHaveLength(5);
  });

  it('checks the stages whose evidence exists, wherever they sit', () => {
    const evidence = evidenceOf({ verification: 'complete', normalization: 'complete' });
    const { container } = render(StageControl, { props: { pathname: '/hl7', evidence } });

    const complete = Array.from(container.querySelectorAll('a[data-state="complete"]'));
    expect(complete.map((link) => link.getAttribute('data-stage'))).toEqual(['normalization', 'verification']);
    expect(container.querySelectorAll('.stage-check')).toHaveLength(2);
    expect(container.querySelector('a[data-stage="source-intake"]')).toHaveAttribute('data-state', 'incomplete');
  });

  it('shows an unknown stage as not complete and says why in its title and name', () => {
    const evidence = evidenceOf({ translation: 'unknown' });
    const { container } = render(StageControl, { props: { pathname: '/', evidence } });

    const stage = container.querySelector('a[data-stage="translation"]')!;
    expect(stage).toHaveAttribute('data-state', 'unknown');
    expect(stage).not.toHaveClass('is-complete');
    expect(stage.querySelector('.stage-unknown')).not.toBeNull();
    expect(stage).toHaveAttribute('title', 'Stage 3 of 5: Translation — unknown. translation is unknown.');
    expect(screen.getByRole('link', { name: 'Translation, stage 3 of 5, unknown' })).toBe(stage);
  });

  it('marks no stage current off the stage routes', () => {
    const { container } = render(StageControl, { props: { pathname: '/operator', evidence: evidenceOf() } });

    expect(container.querySelector('a[aria-current]')).toBeNull();
    expect(container.querySelectorAll('a[data-state="incomplete"]')).toHaveLength(5);
  });

  it('is one tab stop on the current stage, first stage when none is current', () => {
    const { container, unmount } = render(StageControl, { props: { pathname: '/workflows', evidence: null } });
    const stops = Array.from(container.querySelectorAll('a[tabindex="0"]'));
    expect(stops).toHaveLength(1);
    expect(stops[0]).toHaveTextContent('Delivery');
    unmount();

    const home = render(StageControl, { props: { pathname: '/', evidence: null } });
    const homeStops = Array.from(home.container.querySelectorAll('a[tabindex="0"]'));
    expect(homeStops).toHaveLength(1);
    expect(homeStops[0]).toHaveTextContent('Source Intake');
  });

  it('moves focus between segments with the arrow, Home and End keys', async () => {
    const { container } = render(StageControl, { props: { pathname: '/terminology', evidence: null } });
    const links = Array.from(container.querySelectorAll('a')) as HTMLAnchorElement[];

    links[2]!.focus();
    await fireEvent.keyDown(links[2]!, { key: 'ArrowRight' });
    expect(document.activeElement).toBe(links[3]);

    await fireEvent.keyDown(links[3]!, { key: 'Home' });
    expect(document.activeElement).toBe(links[0]);

    await fireEvent.keyDown(links[0]!, { key: 'ArrowLeft' });
    expect(document.activeElement).toBe(links[4]);

    await fireEvent.keyDown(links[4]!, { key: 'End' });
    expect(document.activeElement).toBe(links[4]);
  });
});
