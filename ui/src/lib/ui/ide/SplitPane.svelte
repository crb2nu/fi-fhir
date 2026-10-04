<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import type { SplitOrientation } from './types';

  /** Resizable panes. Only a deliberate resize changes the stored preference. */
  export let orientation: SplitOrientation = 'horizontal';
  export let initialSize: number = 280;
  export let minSize: number = 200;
  export let maxSize: number = 480;
  export let storageKey: string | undefined = undefined;
  /** Space reserved for the secondary pane along the split's axis. */
  export let minSecondarySize: number = 200;
  /** Stack horizontal panes when their minimum widths no longer fit. */
  export let stackWhenNarrow = false;
  export let stackMinPaneHeight = 280;

  const HANDLE_SIZE = 4;
  const dispatch = createEventDispatcher<{ resize: number }>();

  let container: HTMLDivElement;
  let observer: ResizeObserver | undefined;
  let width: number | null = null;
  let height: number | null = null;
  let preferredSize = initialSize;
  let dragging = false;
  let resized = false;
  let startPos = 0;
  let startSize = 0;

  $: axisSize = orientation === 'horizontal' ? width : height;
  $: available = axisSize === null ? null : Math.max(0, axisSize - HANDLE_SIZE);
  // A static split too small for both minima shares the remaining space in
  // their proportions, so neither pane disappears. Opt-in stacking avoids
  // that compromise for layouts such as the HL7 editor and results.
  $: availableMax = available === null ? maxSize
    : available < minSize + minSecondarySize
      ? available * minSize / Math.max(1, minSize + minSecondarySize)
      : available - minSecondarySize;
  $: effectiveMax = Math.min(maxSize, availableMax);
  $: effectiveMin = Math.min(minSize, effectiveMax);
  $: size = Math.min(effectiveMax, Math.max(effectiveMin, preferredSize));
  $: stacked = stackWhenNarrow && orientation === 'horizontal' && width !== null &&
    width < minSize + minSecondarySize + HANDLE_SIZE;
  $: if (stacked && dragging) stopDragging();

  onMount(() => {
    if (storageKey) {
      try {
        const stored = localStorage.getItem(storageKey);
        if (stored !== null) {
          const val = Number(stored);
          if (Number.isFinite(val)) preferredSize = Math.min(maxSize, Math.max(minSize, val));
        }
      } catch {
        // A blocked storage API must not prevent resizing.
      }
    }
    const bounds = container.getBoundingClientRect();
    width = bounds.width;
    height = bounds.height;
    observer = new ResizeObserver((entries) => {
      const entry = entries.find((candidate) => candidate.target === container);
      if (!entry) return;
      width = entry.contentRect.width;
      height = entry.contentRect.height;
    });
    observer.observe(container);
  });

  function onMouseDown(e: MouseEvent): void {
    if (stacked || e.button !== 0) return;
    e.preventDefault();
    dragging = true;
    resized = false;
    startPos = orientation === 'horizontal' ? e.clientX : e.clientY;
    startSize = size;
    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
  }

  function onMouseMove(e: MouseEvent): void {
    if (!dragging) return;
    const current = orientation === 'horizontal' ? e.clientX : e.clientY;
    const next = Math.min(effectiveMax, Math.max(effectiveMin, startSize + current - startPos));
    if (next === size) return;
    preferredSize = next;
    resized = true;
  }

  function commitSize(): void {
    if (storageKey) {
      try {
        localStorage.setItem(storageKey, String(preferredSize));
      } catch {
        // A blocked storage API must not prevent resizing.
      }
    }
    dispatch('resize', preferredSize);
  }

  function stopDragging(): void {
    if (!dragging) return;
    dragging = false;
    window.removeEventListener('mousemove', onMouseMove);
    window.removeEventListener('mouseup', onMouseUp);
  }

  function onMouseUp(): void {
    if (!dragging) return;
    stopDragging();
    if (resized) commitSize();
  }

  /** Keyboard resizing: arrows move the divider 16 px (64 px with Shift). */
  function onKeydown(e: KeyboardEvent): void {
    if (stacked) return;
    const grow = orientation === 'horizontal' ? 'ArrowRight' : 'ArrowDown';
    const shrink = orientation === 'horizontal' ? 'ArrowLeft' : 'ArrowUp';
    if (e.key !== grow && e.key !== shrink) return;
    e.preventDefault();
    const step = e.shiftKey ? 64 : 16;
    const next = Math.min(effectiveMax, Math.max(effectiveMin, size + (e.key === grow ? step : -step)));
    if (next === size) return;
    preferredSize = next;
    commitSize();
  }

  onDestroy(() => {
    observer?.disconnect();
    stopDragging();
  });
</script>

<div
  bind:this={container}
  class="split-pane"
  class:horizontal={orientation === 'horizontal'}
  class:vertical={orientation === 'vertical'}
  class:stacked
  class:dragging
  style:--stack-min-pane-height={`${stackMinPaneHeight}px`}
>
  <div
    class="split-primary"
    style={stacked ? undefined : orientation === 'horizontal'
      ? `width: ${size}px`
      : `height: ${size}px`}
  >
    <slot />
  </div>

  {#if !stacked}
    <button
      type="button"
      class="split-handle"
      class:handle-horizontal={orientation === 'horizontal'}
      class:handle-vertical={orientation === 'vertical'}
      aria-label={orientation === 'horizontal' ? 'Resize workspace columns' : 'Resize workspace rows'}
      title="Drag or use the arrow keys to resize"
      on:mousedown={onMouseDown}
      on:keydown={onKeydown}
    ></button>
  {/if}

  <div class="split-secondary">
    <slot name="secondary" />
  </div>
</div>

<style>
  .split-pane {
    display: flex;
    width: 100%;
    height: 100%;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
  }

  .split-pane.horizontal {
    flex-direction: row;
  }

  .split-pane.vertical,
  .split-pane.stacked {
    flex-direction: column;
  }

  .split-primary {
    flex: 0 0 auto;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
  }

  .split-secondary {
    flex: 1 1 0;
    overflow: hidden;
    min-width: 0;
    min-height: 0;
  }

  .split-pane.stacked {
    overflow-y: auto;
  }

  .stacked > .split-primary,
  .stacked > .split-secondary {
    flex: 1 0 var(--stack-min-pane-height);
    width: 100%;
    min-height: var(--stack-min-pane-height);
    overflow: auto;
  }

  .stacked > .split-secondary {
    border-top: 1px solid var(--color-border-subtle);
  }

  /* 4 px hit area with a 1 px rule; the accent shows on hover, focus and drag. */
  .split-handle {
    position: relative;
    flex: 0 0 4px;
    padding: 0;
    border: none;
    background: transparent;
    cursor: col-resize;
    z-index: 1;
  }

  .split-handle::after {
    content: '';
    position: absolute;
    inset: 0 auto 0 50%;
    width: 1px;
    background: var(--color-border-subtle);
    transform: translateX(-50%);
  }

  .handle-vertical::after {
    inset: 50% 0 auto 0;
    width: auto;
    height: 1px;
    transform: translateY(-50%);
  }

  .split-handle:hover,
  .split-handle:focus-visible,
  .dragging .split-handle {
    background: var(--ide-split-handle-hover, var(--color-primary));
    transition: background var(--duration-fast) var(--ease-out) 80ms;
  }

  .split-handle:focus-visible {
    outline: none;
  }

  .handle-horizontal {
    cursor: col-resize;
  }

  .handle-vertical {
    cursor: row-resize;
  }

  /* Prevent text selection during drag */
  .dragging {
    user-select: none;
    -webkit-user-select: none;
  }
</style>
