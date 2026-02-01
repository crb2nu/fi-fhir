<script lang="ts">
  /**
   * Panel Component
   *
   * Container component with optional title, tone variants,
   * collapsible functionality, and header actions.
   */

  export let title: string | null = null;
  export let tone: 'default' | 'error' | 'success' | 'warning' | 'info' = 'default';
  export let collapsible = false;
  export let collapsed = false;
  export let padding: 'none' | 'sm' | 'md' | 'lg' = 'md';
</script>

<section class="panel {tone}" class:collapsible class:collapsed>
  {#if title || $$slots.actions}
    <header class="header" class:clickable={collapsible}>
      {#if collapsible}
        <button
          type="button"
          class="collapse-trigger"
          aria-expanded={!collapsed}
          on:click={() => (collapsed = !collapsed)}
        >
          <span class="collapse-icon" class:rotated={!collapsed}>
            <svg viewBox="0 0 20 20" fill="currentColor">
              <path
                fill-rule="evenodd"
                d="M7.293 14.707a1 1 0 010-1.414L10.586 10 7.293 6.707a1 1 0 011.414-1.414l4 4a1 1 0 010 1.414l-4 4a1 1 0 01-1.414 0z"
                clip-rule="evenodd"
              />
            </svg>
          </span>
          {#if title}
            <span class="title">{title}</span>
          {/if}
        </button>
      {:else if title}
        <span class="title">{title}</span>
      {/if}

      {#if $$slots.actions}
        <div class="actions">
          <slot name="actions" />
        </div>
      {/if}
    </header>
  {/if}

  {#if !collapsed}
    <div class="body padding-{padding}">
      <slot />
    </div>
  {/if}
</section>

<style>
  .panel {
    border-radius: var(--panel-radius);
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-elevated);
    overflow: hidden;
  }

  /* Tone variants */
  .panel.error {
    border-color: var(--color-danger-border);
  }

  .panel.success {
    border-color: var(--color-success-border);
  }

  .panel.warning {
    border-color: var(--color-warning-border);
  }

  .panel.info {
    border-color: var(--color-info-border);
  }

  /* Header */
  .header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-3) var(--space-4);
    border-bottom: 1px solid var(--color-border-subtle);
    min-height: 48px;
  }

  .collapsed .header {
    border-bottom: none;
  }

  .header.clickable {
    padding-left: 0;
  }

  .title {
    font-size: var(--text-sm);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
    letter-spacing: var(--tracking-tight);
  }

  /* Collapse trigger */
  .collapse-trigger {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-2) var(--space-3);
    margin: 0;
    background: none;
    border: none;
    border-radius: var(--radius-md);
    color: inherit;
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .collapse-trigger:hover {
    background: var(--color-bg-hover);
  }

  .collapse-trigger:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .collapse-icon {
    display: flex;
    width: 16px;
    height: 16px;
    color: var(--color-text-muted);
    transition: transform var(--duration-normal) var(--ease-out);
  }

  .collapse-icon.rotated {
    transform: rotate(90deg);
  }

  .collapse-icon svg {
    width: 100%;
    height: 100%;
  }

  /* Actions slot */
  .actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  /* Body */
  .body {
    color: var(--color-text-secondary);
    animation: slideInUp var(--duration-fast) var(--ease-out);
  }

  .body.padding-none {
    padding: 0;
  }

  .body.padding-sm {
    padding: var(--space-2) var(--space-3);
  }

  .body.padding-md {
    padding: var(--space-3) var(--space-4);
  }

  .body.padding-lg {
    padding: var(--space-4) var(--space-5);
  }

  /* Tone-specific header backgrounds */
  .panel.error .header {
    background: var(--color-danger-bg);
  }

  .panel.success .header {
    background: var(--color-success-bg);
  }

  .panel.warning .header {
    background: var(--color-warning-bg);
  }

  .panel.info .header {
    background: var(--color-info-bg);
  }

  @keyframes slideInUp {
    from {
      opacity: 0;
      transform: translateY(-4px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
