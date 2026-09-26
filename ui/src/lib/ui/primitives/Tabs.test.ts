import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import Tabs from './Tabs.svelte';
import type { TabItem } from './types';

const items: TabItem[] = [
  { id: 'browse', label: 'Browse', testid: 'tab-browse' },
  { id: 'live', label: 'Live Stream', count: 3 },
  { id: 'timeline', label: 'Patient Timeline', disabled: true },
  { id: 'stats', label: 'Statistics' }
];

describe('Tabs', () => {
  it('renders a labelled tablist with aria-selected on the active tab', () => {
    render(Tabs, { props: { items, value: 'live', label: 'Event views', 'data-testid': 'views' } });
    expect(screen.getByRole('tablist', { name: 'Event views' })).toHaveAttribute('data-testid', 'views');
    expect(screen.getByRole('tab', { name: /Live Stream/ })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('tab', { name: 'Browse' })).toHaveAttribute('aria-selected', 'false');
    expect(screen.getByTestId('tab-browse')).toBeInTheDocument();
  });

  it('renders counts in a mono badge and disables disabled tabs', () => {
    render(Tabs, { props: { items, value: 'browse' } });
    expect(screen.getByRole('tab', { name: /Live Stream/ }).querySelector('.ui-tab-count')).toHaveTextContent('3');
    expect(screen.getByRole('tab', { name: 'Patient Timeline' })).toBeDisabled();
  });

  it('keeps a single tab stop on the selected tab (roving tabindex)', () => {
    render(Tabs, { props: { items, value: 'stats' } });
    const tabs = screen.getAllByRole('tab');
    expect(tabs.map((tab) => tab.getAttribute('tabindex'))).toEqual(['-1', '-1', '-1', '0']);
  });

  it('falls back to the first enabled tab as the tab stop when nothing matches', () => {
    render(Tabs, { props: { items, value: 'missing' } });
    expect(screen.getByRole('tab', { name: 'Browse' })).toHaveAttribute('tabindex', '0');
  });

  it('selects on click and reports the change', async () => {
    const onchange = vi.fn();
    render(Tabs, { props: { items, value: 'browse', onchange } });
    await fireEvent.click(screen.getByRole('tab', { name: 'Statistics' }));
    expect(onchange).toHaveBeenCalledWith('stats');
    expect(screen.getByRole('tab', { name: 'Statistics' })).toHaveAttribute('aria-selected', 'true');
  });

  it('auto activation: arrows move focus and select, skipping disabled tabs', async () => {
    const onchange = vi.fn();
    render(Tabs, { props: { items, value: 'live', onchange } });
    const live = screen.getByRole('tab', { name: /Live Stream/ });
    live.focus();

    await fireEvent.keyDown(live, { key: 'ArrowRight' });
    const stats = screen.getByRole('tab', { name: 'Statistics' });
    expect(stats).toHaveFocus();
    expect(onchange).toHaveBeenLastCalledWith('stats');

    await fireEvent.keyDown(stats, { key: 'ArrowRight' }); // wraps
    expect(screen.getByRole('tab', { name: 'Browse' })).toHaveFocus();

    await fireEvent.keyDown(screen.getByRole('tab', { name: 'Browse' }), { key: 'End' });
    expect(stats).toHaveFocus();
    await fireEvent.keyDown(stats, { key: 'Home' });
    expect(screen.getByRole('tab', { name: 'Browse' })).toHaveFocus();
    await fireEvent.keyDown(screen.getByRole('tab', { name: 'Browse' }), { key: 'ArrowLeft' });
    expect(stats).toHaveFocus();
  });

  it('manual activation: arrows move focus only; Enter/click selects', async () => {
    const onchange = vi.fn();
    render(Tabs, { props: { items, value: 'browse', onchange, activation: 'manual' } });
    const browse = screen.getByRole('tab', { name: 'Browse' });
    browse.focus();

    await fireEvent.keyDown(browse, { key: 'ArrowRight' });
    const live = screen.getByRole('tab', { name: /Live Stream/ });
    expect(live).toHaveFocus();
    expect(onchange).not.toHaveBeenCalled();
    expect(browse).toHaveAttribute('aria-selected', 'true');

    await fireEvent.click(live); // Enter/Space on a <button> dispatch click
    expect(onchange).toHaveBeenCalledWith('live');
  });
});
