<script lang="ts">
  /**
   * Skeleton Component
   *
   * Loading placeholder with shimmer animation.
   * Use to indicate content loading states.
   */

  export let variant: 'text' | 'circular' | 'rectangular' = 'text';
  export let width: string | undefined = undefined;
  export let height: string | undefined = undefined;
  export let lines = 1;
  export let animate = true;
</script>

{#if variant === 'text' && lines > 1}
  <div class="skeleton-group" style:width>
    {#each Array.from({ length: lines }, (_, idx) => idx) as i (i)}
      <div
        class="skeleton text"
        class:animate
        class:last={i === lines - 1}
        style:width={i === lines - 1 ? '70%' : undefined}
        style:height
        aria-hidden="true"
      ></div>
    {/each}
  </div>
{:else}
  <div
    class="skeleton {variant}"
    class:animate
    style:width
    style:height
    aria-hidden="true"
  ></div>
{/if}

<style>
  .skeleton {
    background: var(--color-bg-surface);
    border-radius: var(--radius-md);
  }

  .skeleton.animate {
    background: linear-gradient(
      90deg,
      var(--color-bg-surface) 25%,
      var(--color-bg-hover) 50%,
      var(--color-bg-surface) 75%
    );
    background-size: 200% 100%;
    animation: shimmer 1.5s infinite;
  }

  /* Variant: Text */
  .skeleton.text {
    height: 14px;
    width: 100%;
  }

  .skeleton.text.last {
    width: 70%;
  }

  /* Variant: Circular */
  .skeleton.circular {
    width: 40px;
    height: 40px;
    border-radius: var(--radius-full);
  }

  /* Variant: Rectangular */
  .skeleton.rectangular {
    width: 100%;
    height: 100px;
    border-radius: var(--radius-lg);
  }

  /* Group for multiple text lines */
  .skeleton-group {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  @keyframes shimmer {
    0% {
      background-position: -200% 0;
    }
    100% {
      background-position: 200% 0;
    }
  }
</style>
