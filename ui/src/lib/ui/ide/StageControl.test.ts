import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';

vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));

const { default: StageControl } = await import('./StageControl.svelte');

// The header's stage control replaced the JourneyProgress band; these carry
// over what that band's tests pinned (current stage, completed stages, links)
// plus the keyboard model of a segmented control.
describe('StageControl', () => {
  it('renders the five stages as links in order', () => {
    render(StageControl, { props: { pathname: '/profiles' } });

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

  it('marks the stage matching the pathname as current and earlier stages done', () => {
    const { container } = render(StageControl, { props: { pathname: '/profiles/draft' } });

    const current = container.querySelector('a[aria-current="step"]');
    expect(current).toHaveTextContent('Normalization');
    expect(current).toHaveClass('is-current');
    expect(container.querySelectorAll('a[data-state="complete"]')).toHaveLength(1);
    expect(container.querySelector('a[data-state="complete"]')).toHaveTextContent('Source Intake');
    // Done stages carry the check glyph; the current and upcoming ones do not.
    expect(container.querySelectorAll('.stage-check')).toHaveLength(1);
    expect(container.querySelectorAll('a[data-state="upcoming"]')).toHaveLength(3);
  });

  it('marks no stage current off the stage routes', () => {
    const { container } = render(StageControl, { props: { pathname: '/operator' } });

    expect(container.querySelector('a[aria-current]')).toBeNull();
    expect(container.querySelectorAll('a[data-state="upcoming"]')).toHaveLength(5);
  });

  it('is one tab stop on the current stage, first stage when none is current', () => {
    const { container, unmount } = render(StageControl, { props: { pathname: '/workflows' } });
    const stops = Array.from(container.querySelectorAll('a[tabindex="0"]'));
    expect(stops).toHaveLength(1);
    expect(stops[0]).toHaveTextContent('Delivery');
    unmount();

    const home = render(StageControl, { props: { pathname: '/' } });
    const homeStops = Array.from(home.container.querySelectorAll('a[tabindex="0"]'));
    expect(homeStops).toHaveLength(1);
    expect(homeStops[0]).toHaveTextContent('Source Intake');
  });

  it('moves focus between segments with the arrow, Home and End keys', async () => {
    const { container } = render(StageControl, { props: { pathname: '/terminology' } });
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
