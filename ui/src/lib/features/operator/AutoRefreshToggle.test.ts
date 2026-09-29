import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import AutoRefreshToggle from './AutoRefreshToggle.svelte';

describe('AutoRefreshToggle', () => {
  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('is off by default and refreshes every 15 s only while on', async () => {
    const onrefresh = vi.fn();
    const { unmount } = render(AutoRefreshToggle, { onrefresh, subject: 'messages' });
    const toggle = screen.getByTestId('auto-refresh');
    expect(toggle).toHaveAttribute('data-state', 'off');
    expect(toggle).toHaveAttribute('aria-pressed', 'false');

    vi.advanceTimersByTime(60_000);
    expect(onrefresh).not.toHaveBeenCalled();

    await fireEvent.click(toggle);
    expect(toggle).toHaveAttribute('data-state', 'on');
    vi.advanceTimersByTime(15_000);
    expect(onrefresh).toHaveBeenCalledTimes(1);
    vi.advanceTimersByTime(30_000);
    expect(onrefresh).toHaveBeenCalledTimes(3);

    await fireEvent.click(toggle);
    vi.advanceTimersByTime(60_000);
    expect(onrefresh).toHaveBeenCalledTimes(3);

    await fireEvent.click(toggle);
    unmount();
    vi.advanceTimersByTime(60_000);
    expect(onrefresh).toHaveBeenCalledTimes(3);
  });
});
