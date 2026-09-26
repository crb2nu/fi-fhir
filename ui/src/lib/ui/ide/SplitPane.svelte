<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import type { SplitOrientation } from './types';

  /**
   * Resizable split pane with drag handle.
   * Emits 'resize' with the new size in pixels.
   */

  export let orientation: SplitOrientation = 'horizontal';
  export let initialSize: number = 280;
  export let minSize: number = 200;
  export let maxSize: number = 480;
  export let storageKey: string | undefined = undefined;

  const dispatch = createEventDispatcher<{ resize: number }>();

  let size = initialSize;
  let dragging = false;
  let startPos = 0;
  let startSize = 0;

  onMount(() => {
    if (storageKey && typeof window !== 'undefined') {
      try {
        const stored = localStorage.getItem(storageKey);
        if (stored !== null) {
          const val = Number(stored);
          if (Number.isFinite(val)) {
            size = Math.min(maxSize, Math.max(minSize, val));
          }
        }
      } catch {
        // Ignore
      }
    }
  });

  function onMouseDown(e: MouseEvent): void {
    e.preventDefault();
    dragging = true;
    startPos = orientation === 'horizontal' ? e.clientX : e.clientY;
    startSize = size;
    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
  }

  function onMouseMove(e: MouseEvent): void {
    if (!dragging) return;
    const current = orientation === 'horizontal' ? e.clientX : e.clientY;
    const delta = current - startPos;
    const newSize = Math.min(maxSize, Math.max(minSize, startSize + delta));
    size = newSize;
  }

  function commitSize(): void {
    if (storageKey && typeof window !== 'undefined') {
      try {
        localStorage.setItem(storageKey, String(size));
      } catch {
        // Ignore
      }
    }
    dispatch('resize', size);
  }

  function onMouseUp(): void {
    if (!dragging) return;
    dragging = false;
    window.removeEventListener('mousemove', onMouseMove);
    window.removeEventListener('mouseup', onMouseUp);
    commitSize();
  }

  /** Keyboard resizing: arrows move the divider 16 px (64 px with Shift). */
  function onKeydown(e: KeyboardEvent): void {
    const grow = orientation === 'horizontal' ? 'ArrowRight' : 'ArrowDown';
    const shrink = orientation === 'horizontal' ? 'ArrowLeft' : 'ArrowUp';
    if (e.key !== grow && e.key !== shrink) return;
    e.preventDefault();
    const step = e.shiftKey ? 64 : 16;
    const delta = e.key === grow ? step : -step;
    size = Math.min(maxSize, Math.max(minSize, size + delta));
    commitSize();
  }

  onDestroy(() => {
    if (dragging) {
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
    }
  });
</script>

<div
  class="split-pane"
  class:horizontal={orientation === 'horizontal'}
  class:vertical={orientation === 'vertical'}
  class:dragging
>
  <div
    class="split-primary"
    style={orientation === 'horizontal'
      ? `width: ${size}px`
      : `height: ${size}px`}
  >
    <slot />
  </div>

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

  <div class="split-secondary">
    <slot name="secondary" />
  </div>
</div>

<style>
  .split-pane {
    display: flex;
    width: 100%;
    height: 100%;
    overflow: hidden;
  }

  .split-pane.horizontal {
    flex-direction: row;
  }

  .split-pane.vertical {
    flex-direction: column;
  }

  .split-primary {
    flex: 0 0 auto;
    overflow: hidden;
  }

  .split-secondary {
    flex: 1 1 0;
    overflow: hidden;
    min-width: 0;
    min-height: 0;
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
