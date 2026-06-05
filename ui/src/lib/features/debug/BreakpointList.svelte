<script lang="ts">
  /**
   * BreakpointList Component
   *
   * Displays a list of breakpoints with toggle, remove, and add controls.
   */
  import type { Breakpoint, BreakpointType } from './types';

  export let breakpoints: Breakpoint[] = [];
  export let onToggle: ((id: string) => void) | undefined = undefined;
  export let onRemove: ((id: string) => void) | undefined = undefined;
  export let onAdd: ((detail: { type: BreakpointType; name: string }) => void) | undefined = undefined;
  /**
   * Whether a debug session is active. Adding/toggling breakpoints requires one
   * (the operations call the session API). When false the controls are disabled
   * with an explanatory tooltip rather than letting a dead click fire a toast
   * (UX policy B2/D2).
   */
  export let hasSession = true;

  const NO_SESSION_HINT = 'Start a debug session to manage breakpoints';

  let showAddForm = false;

  $: if (!hasSession) {
    showAddForm = false;
  }
  let newType: BreakpointType = 'route';
  let newName = '';

  function handleAdd(): void {
    const trimmed = newName.trim();
    if (!trimmed) return;
    onAdd?.({ type: newType, name: trimmed });
    newName = '';
    showAddForm = false;
  }

  function handleCancel(): void {
    newName = '';
    showAddForm = false;
  }

  const typeLabels: Record<BreakpointType, string> = {
    route: 'Route',
    action: 'Action',
    transform: 'Transform'
  };
</script>

<div class="breakpoint-list">
  <div class="bp-header">
    <span class="bp-title">Breakpoints</span>
    <button
      class="bp-add-btn"
      title={hasSession ? 'Add breakpoint' : NO_SESSION_HINT}
      aria-label="Add breakpoint"
      disabled={!hasSession}
      on:click={() => { if (!hasSession) return; showAddForm = !showAddForm; }}
    >
      <svg viewBox="0 0 16 16" fill="currentColor" aria-hidden="true">
        <path d="M8 2v12M2 8h12" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" fill="none" />
      </svg>
    </button>
  </div>

  {#if showAddForm}
    <div class="bp-add-form" role="form" aria-label="Add breakpoint form">
      <select
        class="bp-type-select"
        bind:value={newType}
        aria-label="Breakpoint type"
      >
        <option value="route">Route</option>
        <option value="action">Action</option>
        <option value="transform">Transform</option>
      </select>
      <input
        class="bp-name-input"
        type="text"
        placeholder="Name..."
        bind:value={newName}
        aria-label="Breakpoint name"
        on:keydown={(e) => { if (e.key === 'Enter') handleAdd(); if (e.key === 'Escape') handleCancel(); }}
      />
      <button class="bp-form-btn confirm" on:click={handleAdd} aria-label="Confirm add">
        <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
          <path d="M3 8l4 4 6-8" />
        </svg>
      </button>
      <button class="bp-form-btn cancel" on:click={handleCancel} aria-label="Cancel add">
        <svg viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true">
          <path d="M4 4l8 8M12 4l-8 8" />
        </svg>
      </button>
    </div>
  {/if}

  {#if breakpoints.length === 0}
    <div class="bp-empty">No breakpoints set</div>
  {:else}
    <ul class="bp-items" role="list">
      {#each breakpoints as bp (bp.id)}
        <li class="bp-item" class:disabled={!bp.enabled}>
          <label class="bp-toggle" title={hasSession ? (bp.enabled ? 'Disable' : 'Enable') : NO_SESSION_HINT}>
            <input
              type="checkbox"
              checked={bp.enabled}
              disabled={!hasSession}
              on:change={() => onToggle?.(bp.id)}
              aria-label="Toggle {bp.name}"
            />
            <span class="bp-checkbox" class:checked={bp.enabled}></span>
          </label>
          <span class="bp-type-badge {bp.type}">{typeLabels[bp.type]}</span>
          <span class="bp-name">{bp.name}</span>
          <button
            class="bp-remove-btn"
            title="Remove breakpoint"
            aria-label="Remove {bp.name}"
            on:click={() => onRemove?.(bp.id)}
          >
            <svg viewBox="0 0 12 12" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" aria-hidden="true">
              <path d="M3 3l6 6M9 3l-6 6" />
            </svg>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .breakpoint-list {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .bp-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .bp-title {
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wider);
  }

  .bp-add-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    padding: 0;
    border: 1px solid transparent;
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--color-text-muted);
    cursor: pointer;
    transition: var(--transition-all);
  }

  .bp-add-btn:hover:not(:disabled) {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
    border-color: var(--color-border-default);
  }

  .bp-add-btn:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  .bp-add-btn:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .bp-add-btn svg {
    width: 14px;
    height: 14px;
  }

  /* Add form */
  .bp-add-form {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
  }

  .bp-type-select {
    padding: var(--space-1) var(--space-1);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-size: var(--text-2xs);
    font-family: inherit;
  }

  .bp-name-input {
    flex: 1;
    min-width: 0;
    padding: var(--space-1) var(--space-2);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-size: var(--text-xs);
    font-family: inherit;
  }

  .bp-name-input:focus {
    outline: none;
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .bp-form-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 22px;
    height: 22px;
    padding: 0;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .bp-form-btn svg {
    width: 12px;
    height: 12px;
  }

  .bp-form-btn.confirm {
    color: var(--color-success);
  }

  .bp-form-btn.confirm:hover {
    background: var(--color-success-bg);
  }

  .bp-form-btn.cancel {
    color: var(--color-danger);
  }

  .bp-form-btn.cancel:hover {
    background: var(--color-danger-bg);
  }

  /* List */
  .bp-items {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .bp-item {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-3);
    min-height: 32px;
    border-bottom: 1px solid var(--color-border-subtle);
    transition: var(--transition-colors);
  }

  .bp-item:last-child {
    border-bottom: none;
  }

  .bp-item:hover {
    background: var(--color-bg-hover);
  }

  .bp-item.disabled {
    opacity: 0.5;
  }

  /* Checkbox */
  .bp-toggle {
    display: flex;
    cursor: pointer;
    flex-shrink: 0;
  }

  .bp-toggle:has(input:disabled) {
    cursor: not-allowed;
  }

  .bp-toggle:has(input:disabled) .bp-checkbox {
    opacity: 0.45;
  }

  .bp-toggle input {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
  }

  .bp-checkbox {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 14px;
    height: 14px;
    border: 1.5px solid var(--color-border-strong);
    border-radius: 3px;
    background: transparent;
    transition: var(--transition-all);
  }

  .bp-checkbox.checked {
    background: var(--color-primary);
    border-color: var(--color-primary);
  }

  .bp-checkbox.checked::after {
    content: '';
    display: block;
    width: 4px;
    height: 7px;
    border: solid white;
    border-width: 0 1.5px 1.5px 0;
    transform: rotate(45deg) translateY(-1px);
  }

  /* Type badge */
  .bp-type-badge {
    font-size: var(--text-2xs);
    font-weight: var(--font-medium);
    padding: 1px var(--space-1);
    border-radius: var(--radius-sm);
    white-space: nowrap;
    flex-shrink: 0;
  }

  .bp-type-badge.route {
    color: var(--color-primary);
    background: var(--color-primary-muted);
  }

  .bp-type-badge.action {
    color: var(--color-success-text);
    background: var(--color-success-bg);
  }

  .bp-type-badge.transform {
    color: var(--color-warning-text);
    background: var(--color-warning-bg);
  }

  /* Name */
  .bp-name {
    flex: 1;
    font-size: var(--text-xs);
    font-family: var(--font-mono);
    color: var(--color-text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  /* Remove button */
  .bp-remove-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    padding: 0;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-text-muted);
    cursor: pointer;
    opacity: 0;
    transition: var(--transition-all);
    flex-shrink: 0;
  }

  .bp-remove-btn svg {
    width: 10px;
    height: 10px;
  }

  .bp-item:hover .bp-remove-btn {
    opacity: 1;
  }

  .bp-remove-btn:hover {
    background: var(--color-danger-bg);
    color: var(--color-danger);
  }

  .bp-empty {
    padding: var(--space-4) var(--space-3);
    color: var(--color-text-muted);
    font-size: var(--text-xs);
    text-align: center;
    font-style: italic;
  }
</style>
