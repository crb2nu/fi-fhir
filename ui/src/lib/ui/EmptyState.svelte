<script lang="ts">
  /**
   * EmptyState Component
   *
   * Consistent empty state display with icon, title, description,
   * and optional action button.
   */

  export let icon: 'search' | 'file' | 'folder' | 'data' | 'upload' | 'error' | 'inbox' = 'inbox';
  export let title: string;
  export let description: string | undefined = undefined;
  export let compact = false;
</script>

<div class="empty-state" class:compact>
  <div class="icon-container">
    {#if icon === 'search'}
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <circle cx="11" cy="11" r="8" />
        <path d="M21 21l-4.35-4.35" />
      </svg>
    {:else if icon === 'file'}
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M14 2H6a2 2 0 00-2 2v16a2 2 0 002 2h12a2 2 0 002-2V8z" />
        <polyline points="14,2 14,8 20,8" />
      </svg>
    {:else if icon === 'folder'}
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M22 19a2 2 0 01-2 2H4a2 2 0 01-2-2V5a2 2 0 012-2h5l2 3h9a2 2 0 012 2z" />
      </svg>
    {:else if icon === 'data'}
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M4 7h16M4 12h16M4 17h10" />
      </svg>
    {:else if icon === 'upload'}
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <path d="M21 15v4a2 2 0 01-2 2H5a2 2 0 01-2-2v-4" />
        <polyline points="17,8 12,3 7,8" />
        <line x1="12" y1="3" x2="12" y2="15" />
      </svg>
    {:else if icon === 'error'}
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <circle cx="12" cy="12" r="10" />
        <line x1="12" y1="8" x2="12" y2="12" />
        <line x1="12" y1="16" x2="12.01" y2="16" />
      </svg>
    {:else}
      <!-- inbox / default -->
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5">
        <polyline points="22,12 16,12 14,15 10,15 8,12 2,12" />
        <path d="M5.45 5.11L2 12v6a2 2 0 002 2h16a2 2 0 002-2v-6l-3.45-6.89A2 2 0 0016.76 4H7.24a2 2 0 00-1.79 1.11z" />
      </svg>
    {/if}
  </div>

  <h3 class="title">{title}</h3>

  {#if description}
    <p class="description">{description}</p>
  {/if}

  {#if $$slots.default}
    <div class="action">
      <slot />
    </div>
  {/if}
</div>

<style>
  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    text-align: center;
    padding: var(--space-12) var(--space-6);
    animation: fadeIn var(--duration-normal) var(--ease-out);
  }

  .empty-state.compact {
    padding: var(--space-8) var(--space-4);
  }

  .icon-container {
    width: 64px;
    height: 64px;
    margin-bottom: var(--space-4);
    color: var(--color-text-muted);
    opacity: 0.6;
  }

  .compact .icon-container {
    width: 48px;
    height: 48px;
    margin-bottom: var(--space-3);
  }

  .icon-container svg {
    width: 100%;
    height: 100%;
  }

  .title {
    font-size: var(--text-base);
    font-weight: var(--font-semibold);
    color: var(--color-text-secondary);
    margin-bottom: var(--space-2);
  }

  .compact .title {
    font-size: var(--text-sm);
  }

  .description {
    font-size: var(--text-sm);
    color: var(--color-text-muted);
    max-width: 300px;
    line-height: var(--leading-relaxed);
  }

  .compact .description {
    font-size: var(--text-xs);
    max-width: 240px;
  }

  .action {
    margin-top: var(--space-4);
  }

  .compact .action {
    margin-top: var(--space-3);
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(4px); }
    to { opacity: 1; transform: translateY(0); }
  }
</style>
