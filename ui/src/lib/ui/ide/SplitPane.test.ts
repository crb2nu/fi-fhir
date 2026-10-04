import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import SplitPaneHarness from './__fixtures__/SplitPaneHarness.svelte';

const STORAGE_KEY = 'split-pane-test';
let observers: ResizeObserverMock[];

class ResizeObserverMock {
  target: Element | null = null;
  disconnect = vi.fn();

  constructor(private callback: ResizeObserverCallback) {
    observers.push(this);
  }

  observe(target: Element): void {
    this.target = target;
  }

  resize(width: number, height: number): void {
    this.callback([
      { target: this.target, contentRect: { width, height } } as ResizeObserverEntry
    ], this as unknown as ResizeObserver);
  }
}

async function resize(width: number, height = 800): Promise<void> {
  observers[0]!.resize(width, height);
  await tick();
}

function primary(): HTMLElement {
  return screen.getByLabelText('Primary draft').closest('.split-primary') as HTMLElement;
}

function split(): HTMLElement {
  return primary().parentElement!;
}

beforeEach(() => {
  observers = [];
  localStorage.removeItem(STORAGE_KEY);
  vi.stubGlobal('ResizeObserver', ResizeObserverMock);
});

afterEach(() => {
  cleanup();
  localStorage.removeItem(STORAGE_KEY);
  vi.unstubAllGlobals();
});

describe('SplitPane container sizing', () => {
  it('reserves results space for a large saved width and restores the preference after shrinking and growing', async () => {
    localStorage.setItem(STORAGE_KEY, '1100');
    render(SplitPaneHarness, { settings: {
      initialSize: 600, minSize: 360, maxSize: 1200, minSecondarySize: 360, storageKey: STORAGE_KEY
    } });
    await resize(1000);
    expect(primary().style.width).toBe('636px');
    expect(localStorage.getItem(STORAGE_KEY)).toBe('1100');

    await resize(820);
    expect(primary().style.width).toBe('456px');
    await resize(1600);
    expect(primary().style.width).toBe('1100px');
    expect(localStorage.getItem(STORAGE_KEY)).toBe('1100');
  });

  it('stacks below both minimum widths, retains pane contents, and restores columns without changing the preference', async () => {
    localStorage.setItem(STORAGE_KEY, '900');
    render(SplitPaneHarness, { settings: {
      minSize: 360, maxSize: 1200, minSecondarySize: 360, stackWhenNarrow: true,
      stackMinPaneHeight: 320, storageKey: STORAGE_KEY
    } });
    await resize(1200);
    const draft = screen.getByLabelText('Primary draft');
    await fireEvent.input(draft, { target: { value: 'Unsaved editor text' } });
    await resize(723, 400);

    expect(split()).toHaveClass('stacked');
    expect(split().style.getPropertyValue('--stack-min-pane-height')).toBe('320px');
    expect(primary().style.width).toBe('');
    expect(screen.getByRole('region', { name: 'Secondary results' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Resize workspace columns' })).not.toBeInTheDocument();
    expect(screen.getByLabelText('Primary draft')).toBe(draft);
    expect(draft).toHaveValue('Unsaved editor text');

    await resize(724);
    expect(split()).not.toHaveClass('stacked');
    expect(primary().style.width).toBe('360px');
    await resize(1600);
    expect(primary().style.width).toBe('900px');
    expect(localStorage.getItem(STORAGE_KEY)).toBe('900');
  });

  it('persists a deliberate drag from the displayed width and restores that new preference after another shrink', async () => {
    localStorage.setItem(STORAGE_KEY, '1100');
    render(SplitPaneHarness, { settings: {
      minSize: 360, maxSize: 1200, minSecondarySize: 360, storageKey: STORAGE_KEY
    } });
    await resize(1000);
    const handle = screen.getByRole('button', { name: 'Resize workspace columns' });
    await fireEvent.mouseDown(handle, { clientX: 636, button: 0 });
    await fireEvent.mouseMove(window, { clientX: 536 });
    expect(primary().style.width).toBe('536px');
    expect(localStorage.getItem(STORAGE_KEY)).toBe('1100');
    await fireEvent.mouseUp(window);
    expect(localStorage.getItem(STORAGE_KEY)).toBe('536');

    await resize(800);
    expect(primary().style.width).toBe('436px');
    await resize(1600);
    expect(primary().style.width).toBe('536px');
  });

  it('keeps arrows bounded by the secondary reservation and persists only actual changes', async () => {
    localStorage.setItem(STORAGE_KEY, '1100');
    render(SplitPaneHarness, { settings: {
      minSize: 360, maxSize: 1200, minSecondarySize: 360, storageKey: STORAGE_KEY
    } });
    await resize(1000);
    const handle = screen.getByRole('button', { name: 'Resize workspace columns' });
    await fireEvent.keyDown(handle, { key: 'ArrowRight' });
    expect(primary().style.width).toBe('636px');
    expect(localStorage.getItem(STORAGE_KEY)).toBe('1100');
    await fireEvent.keyDown(handle, { key: 'ArrowLeft' });
    expect(primary().style.width).toBe('620px');
    expect(localStorage.getItem(STORAGE_KEY)).toBe('620');
    await fireEvent.keyDown(handle, { key: 'ArrowLeft', shiftKey: true });
    expect(primary().style.width).toBe('556px');
    expect(localStorage.getItem(STORAGE_KEY)).toBe('556');
  });

  it('uses height and up/down keys for a vertical split, sharing space when neither minimum fits', async () => {
    localStorage.setItem(STORAGE_KEY, '600');
    render(SplitPaneHarness, { settings: {
      orientation: 'vertical', minSize: 200, maxSize: 1000, minSecondarySize: 200,
      stackWhenNarrow: true, storageKey: STORAGE_KEY
    } });
    await resize(200, 700);
    expect(primary().style.height).toBe('496px');
    expect(split()).not.toHaveClass('stacked');
    const handle = screen.getByRole('button', { name: 'Resize workspace rows' });
    await fireEvent.keyDown(handle, { key: 'ArrowUp', shiftKey: true });
    expect(primary().style.height).toBe('432px');
    expect(localStorage.getItem(STORAGE_KEY)).toBe('432');

    await resize(200, 204);
    expect(primary().style.height).toBe('100px');
    expect(localStorage.getItem(STORAGE_KEY)).toBe('432');
    await resize(200, 900);
    expect(primary().style.height).toBe('432px');
  });

  it('shares a too-narrow static split instead of hiding either pane', async () => {
    render(SplitPaneHarness, { settings: { minSize: 300, minSecondarySize: 200 } });
    await resize(254);
    expect(split()).not.toHaveClass('stacked');
    expect(primary().style.width).toBe('150px');
    expect(screen.getByRole('button', { name: 'Resize workspace columns' })).toBeInTheDocument();
  });

  it('cancels a drag when stacking and disconnects its observer and global handlers on unmount', async () => {
    const { unmount } = render(SplitPaneHarness, { settings: {
      initialSize: 600, minSize: 360, maxSize: 1200, minSecondarySize: 360,
      stackWhenNarrow: true, storageKey: STORAGE_KEY
    } });
    await resize(1200);
    await fireEvent.mouseDown(screen.getByRole('button', { name: 'Resize workspace columns' }), { clientX: 600 });
    await resize(600);
    await fireEvent.mouseMove(window, { clientX: 400 });
    await fireEvent.mouseUp(window);
    expect(split()).not.toHaveClass('dragging');
    expect(localStorage.getItem(STORAGE_KEY)).toBeNull();

    await resize(1200);
    await fireEvent.mouseDown(screen.getByRole('button', { name: 'Resize workspace columns' }), { clientX: 600 });
    unmount();
    expect(observers[0]!.disconnect).toHaveBeenCalledOnce();
    await fireEvent.mouseMove(window, { clientX: 450 });
    await fireEvent.mouseUp(window);
    expect(localStorage.getItem(STORAGE_KEY)).toBeNull();
  });
});
