<script lang="ts">
  /**
   * Tooltip Component
   *
   * Hover tooltip for displaying additional information.
   * Positions automatically based on available space.
   */

  export let content: string;
  export let position: 'top' | 'bottom' | 'left' | 'right' = 'top';
  export let delay = 200;

  let visible = false;
  let timeoutId: ReturnType<typeof setTimeout> | null = null;

  function show() {
    timeoutId = setTimeout(() => {
      visible = true;
    }, delay);
  }

  function hide() {
    if (timeoutId) {
      clearTimeout(timeoutId);
      timeoutId = null;
    }
    visible = false;
  }
</script>

<div
  class="tooltip-wrapper"
  role="presentation"
  on:mouseenter={show}
  on:mouseleave={hide}
  on:focus={show}
  on:blur={hide}
>
  <slot />

  {#if visible && content}
    <div
      class="tooltip {position}"
      role="tooltip"
      aria-hidden={!visible}
    >
      <span class="tooltip-content">{content}</span>
      <span class="tooltip-arrow"></span>
    </div>
  {/if}
</div>

<style>
  .tooltip-wrapper {
    position: relative;
    display: inline-flex;
  }

  .tooltip {
    position: absolute;
    z-index: var(--z-tooltip);
    padding: var(--space-2) var(--space-3);
    background: var(--color-bg-base);
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-lg);
    pointer-events: none;
    animation: tooltipIn var(--duration-fast) var(--ease-out);
  }

  .tooltip-content {
    font-size: var(--text-xs);
    font-weight: var(--font-medium);
    color: var(--color-text-primary);
    white-space: nowrap;
    line-height: var(--leading-tight);
  }

  .tooltip-arrow {
    position: absolute;
    width: 8px;
    height: 8px;
    background: var(--color-bg-base);
    border: 1px solid var(--color-border-strong);
    transform: rotate(45deg);
  }

  /* Position: Top */
  .tooltip.top {
    bottom: calc(100% + 8px);
    left: 50%;
    transform: translateX(-50%);
  }

  .tooltip.top .tooltip-arrow {
    bottom: -5px;
    left: 50%;
    transform: translateX(-50%) rotate(45deg);
    border-top: none;
    border-left: none;
  }

  /* Position: Bottom */
  .tooltip.bottom {
    top: calc(100% + 8px);
    left: 50%;
    transform: translateX(-50%);
  }

  .tooltip.bottom .tooltip-arrow {
    top: -5px;
    left: 50%;
    transform: translateX(-50%) rotate(45deg);
    border-bottom: none;
    border-right: none;
  }

  /* Position: Left */
  .tooltip.left {
    right: calc(100% + 8px);
    top: 50%;
    transform: translateY(-50%);
  }

  .tooltip.left .tooltip-arrow {
    right: -5px;
    top: 50%;
    transform: translateY(-50%) rotate(45deg);
    border-left: none;
    border-bottom: none;
  }

  /* Position: Right */
  .tooltip.right {
    left: calc(100% + 8px);
    top: 50%;
    transform: translateY(-50%);
  }

  .tooltip.right .tooltip-arrow {
    left: -5px;
    top: 50%;
    transform: translateY(-50%) rotate(45deg);
    border-right: none;
    border-top: none;
  }

  @keyframes tooltipIn {
    from {
      opacity: 0;
      transform: translateX(-50%) translateY(2px);
    }
    to {
      opacity: 1;
      transform: translateX(-50%) translateY(0);
    }
  }

  .tooltip.bottom {
    animation-name: tooltipInBottom;
  }

  @keyframes tooltipInBottom {
    from {
      opacity: 0;
      transform: translateX(-50%) translateY(-2px);
    }
    to {
      opacity: 1;
      transform: translateX(-50%) translateY(0);
    }
  }

  .tooltip.left {
    animation-name: tooltipInLeft;
  }

  @keyframes tooltipInLeft {
    from {
      opacity: 0;
      transform: translateY(-50%) translateX(2px);
    }
    to {
      opacity: 1;
      transform: translateY(-50%) translateX(0);
    }
  }

  .tooltip.right {
    animation-name: tooltipInRight;
  }

  @keyframes tooltipInRight {
    from {
      opacity: 0;
      transform: translateY(-50%) translateX(-2px);
    }
    to {
      opacity: 1;
      transform: translateY(-50%) translateX(0);
    }
  }
</style>
