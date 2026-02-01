<script lang="ts">
  /**
   * FilterChip Component
   *
   * Displays an active filter with label and remove button.
   * Used in filter bars to show currently applied filters.
   */

  import { createEventDispatcher } from 'svelte';

  export let label: string;
  export let value: string;

  const dispatch = createEventDispatcher<{ remove: void }>();
</script>

<span class="filter-chip">
  <span class="chip-label">{label}:</span>
  <span class="chip-value">{value}</span>
  <button
    class="chip-remove"
    type="button"
    aria-label="Remove filter: {label}"
    on:click|stopPropagation={() => dispatch('remove')}
  >
    <svg viewBox="0 0 16 16" fill="currentColor">
      <path d="M4.646 4.646a.5.5 0 0 1 .708 0L8 7.293l2.646-2.647a.5.5 0 0 1 .708.708L8.707 8l2.647 2.646a.5.5 0 0 1-.708.708L8 8.707l-2.646 2.647a.5.5 0 0 1-.708-.708L7.293 8 4.646 5.354a.5.5 0 0 1 0-.708z"/>
    </svg>
  </button>
</span>

<style>
  .filter-chip {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-1) var(--space-1) var(--space-1) var(--space-2);
    background: var(--color-primary-muted);
    border: 1px solid var(--color-primary-border);
    border-radius: var(--radius-full);
    font-size: var(--text-xs);
    animation: slideInUp var(--duration-fast) var(--ease-out);
  }

  .chip-label {
    color: var(--color-text-tertiary);
    font-weight: var(--font-medium);
  }

  .chip-value {
    color: var(--color-text-primary);
    font-weight: var(--font-semibold);
    max-width: 120px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .chip-remove {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    padding: 0;
    margin: 0;
    background: transparent;
    border: none;
    border-radius: var(--radius-full);
    color: var(--color-text-muted);
    cursor: pointer;
    transition: var(--transition-all);
  }

  .chip-remove:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .chip-remove:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .chip-remove svg {
    width: 12px;
    height: 12px;
  }

  @keyframes slideInUp {
    from {
      opacity: 0;
      transform: translateY(4px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }
</style>
