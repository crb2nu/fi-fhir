<script lang="ts" context="module">
  export type SelectOption = {
    value: string;
    label: string;
    disabled?: boolean;
  };
</script>

<script lang="ts">
  /**
   * Select Component
   *
   * Dropdown select matching Input styling for consistent forms.
   * Supports option groups, placeholder, and error states.
   */

  export let id: string | undefined = undefined;
  export let name: string | undefined = undefined;
  export let value: string = '';
  export let options: SelectOption[] = [];
  export let placeholder: string | undefined = undefined;
  export let label: string | undefined = undefined;
  export let hint: string | undefined = undefined;
  export let error: string | undefined = undefined;
  export let disabled = false;
  export let required = false;
  export let size: 'sm' | 'md' | 'lg' = 'md';
  export let fullWidth = true;

  // Generate unique ID if not provided
  const selectId = id ?? `select-${Math.random().toString(36).slice(2, 9)}`;
</script>

<div class="select-wrapper" class:full-width={fullWidth} class:has-error={!!error}>
  {#if label}
    <label class="label" for={selectId}>
      {label}
      {#if required}
        <span class="required" aria-hidden="true">*</span>
      {/if}
    </label>
  {/if}

  <div class="select-container">
    <select
      id={selectId}
      {name}
      bind:value
      {disabled}
      {required}
      class="select {size}"
      aria-invalid={!!error}
      aria-describedby={error ? `${selectId}-error` : hint ? `${selectId}-hint` : undefined}
      on:change
      on:focus
      on:blur
      {...$$restProps}
    >
      {#if placeholder}
        <option value="" disabled selected={!value}>{placeholder}</option>
      {/if}
      {#each options as option (option.value)}
        <option value={option.value} disabled={option.disabled}>
          {option.label}
        </option>
      {/each}
    </select>

    <span class="chevron" aria-hidden="true">
      <svg viewBox="0 0 20 20" fill="currentColor">
        <path
          fill-rule="evenodd"
          d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z"
          clip-rule="evenodd"
        />
      </svg>
    </span>
  </div>

  {#if error}
    <p class="message error-message" id="{selectId}-error" role="alert">
      {error}
    </p>
  {:else if hint}
    <p class="message hint-message" id="{selectId}-hint">
      {hint}
    </p>
  {/if}
</div>

<style>
  .select-wrapper {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .select-wrapper.full-width {
    width: 100%;
  }

  .label {
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-tertiary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wide);
  }

  .required {
    color: var(--color-danger);
    margin-left: 2px;
  }

  .select-container {
    position: relative;
    display: flex;
    align-items: center;
  }

  .select {
    width: 100%;
    padding: 0 var(--space-10) 0 var(--input-padding-x);
    border-radius: var(--radius-lg);
    border: var(--input-border-width) solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-size: var(--text-sm);
    line-height: var(--leading-normal);
    outline: none;
    cursor: pointer;
    appearance: none;
    transition: var(--transition-all);
  }

  /* Size variants */
  .select.sm {
    height: var(--input-height-sm);
    font-size: var(--text-xs);
  }

  .select.md {
    height: var(--input-height-md);
  }

  .select.lg {
    height: var(--input-height-lg);
    font-size: var(--text-base);
  }

  /* States */
  .select:hover:not(:disabled):not(:focus) {
    border-color: var(--color-border-strong);
  }

  .select:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .select:disabled {
    opacity: 0.5;
    cursor: not-allowed;
    background: var(--color-bg-elevated);
  }

  /* Error state */
  .has-error .select {
    border-color: var(--color-danger-border);
  }

  .has-error .select:focus {
    border-color: var(--color-danger);
    box-shadow: var(--shadow-focus-danger);
  }

  /* Chevron icon */
  .chevron {
    position: absolute;
    right: var(--space-3);
    pointer-events: none;
    color: var(--color-text-muted);
    transition: var(--transition-transform);
  }

  .chevron svg {
    width: 16px;
    height: 16px;
  }

  .select:focus + .chevron {
    color: var(--color-text-secondary);
  }

  /* Option styling (limited browser support) */
  .select option {
    background: var(--color-bg-base);
    color: var(--color-text-primary);
    padding: var(--space-2);
  }

  .select option:disabled {
    color: var(--color-text-muted);
  }

  /* Messages */
  .message {
    font-size: var(--text-xs);
    margin-top: var(--space-1);
  }

  .hint-message {
    color: var(--color-text-muted);
  }

  .error-message {
    color: var(--color-danger-text);
  }
</style>
