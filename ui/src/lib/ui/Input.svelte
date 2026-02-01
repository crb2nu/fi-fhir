<script lang="ts">
  /**
   * Input Component
   *
   * Standardized text input with label, error states, and optional icon.
   * Designed for consistent form styling across the application.
   */

  export let id: string | undefined = undefined;
  export let name: string | undefined = undefined;
  export let type: 'text' | 'email' | 'password' | 'number' | 'search' | 'url' | 'tel' | 'date' = 'text';
  export let value: string = '';
  export let placeholder: string = '';
  export let label: string | undefined = undefined;
  export let hint: string | undefined = undefined;
  export let error: string | undefined = undefined;
  export let disabled = false;
  export let readonly = false;
  export let required = false;
  export let size: 'sm' | 'md' | 'lg' = 'md';
  export let fullWidth = true;

  // Generate unique ID if not provided
  const inputId = id ?? `input-${Math.random().toString(36).slice(2, 9)}`;
</script>

<div class="input-wrapper" class:full-width={fullWidth} class:has-error={!!error}>
  {#if label}
    <label class="label" for={inputId}>
      {label}
      {#if required}
        <span class="required" aria-hidden="true">*</span>
      {/if}
    </label>
  {/if}

  <div class="input-container">
    {#if $$slots.prefix}
      <span class="addon prefix">
        <slot name="prefix" />
      </span>
    {/if}

    <input
      {type}
      id={inputId}
      {name}
      bind:value
      {placeholder}
      {disabled}
      {readonly}
      {required}
      class="input {size}"
      class:has-prefix={$$slots.prefix}
      class:has-suffix={$$slots.suffix}
      aria-invalid={!!error}
      aria-describedby={error ? `${inputId}-error` : hint ? `${inputId}-hint` : undefined}
      on:input
      on:change
      on:focus
      on:blur
      on:keydown
      on:keyup
      {...$$restProps}
    />

    {#if $$slots.suffix}
      <span class="addon suffix">
        <slot name="suffix" />
      </span>
    {/if}
  </div>

  {#if error}
    <p class="message error-message" id="{inputId}-error" role="alert">
      {error}
    </p>
  {:else if hint}
    <p class="message hint-message" id="{inputId}-hint">
      {hint}
    </p>
  {/if}
</div>

<style>
  .input-wrapper {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .input-wrapper.full-width {
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

  .input-container {
    position: relative;
    display: flex;
    align-items: stretch;
  }

  .input {
    flex: 1;
    width: 100%;
    padding: 0 var(--input-padding-x);
    border-radius: var(--radius-lg);
    border: var(--input-border-width) solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-size: var(--text-sm);
    line-height: var(--leading-normal);
    outline: none;
    transition: var(--transition-all);
  }

  /* Size variants */
  .input.sm {
    height: var(--input-height-sm);
    font-size: var(--text-xs);
  }

  .input.md {
    height: var(--input-height-md);
  }

  .input.lg {
    height: var(--input-height-lg);
    font-size: var(--text-base);
  }

  /* With addons */
  .input.has-prefix {
    border-top-left-radius: 0;
    border-bottom-left-radius: 0;
    border-left: none;
  }

  .input.has-suffix {
    border-top-right-radius: 0;
    border-bottom-right-radius: 0;
    border-right: none;
  }

  /* States */
  .input::placeholder {
    color: var(--color-text-muted);
  }

  .input:hover:not(:disabled):not(:focus) {
    border-color: var(--color-border-strong);
  }

  .input:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .input:disabled {
    opacity: 0.5;
    cursor: not-allowed;
    background: var(--color-bg-elevated);
  }

  .input:read-only {
    background: var(--color-bg-elevated);
  }

  /* Error state */
  .has-error .input {
    border-color: var(--color-danger-border);
  }

  .has-error .input:focus {
    border-color: var(--color-danger);
    box-shadow: var(--shadow-focus-danger);
  }

  /* Addon styling */
  .addon {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 0 var(--space-3);
    background: var(--color-bg-elevated);
    border: var(--input-border-width) solid var(--color-border-default);
    color: var(--color-text-tertiary);
    font-size: var(--text-sm);
  }

  .addon.prefix {
    border-right: none;
    border-radius: var(--radius-lg) 0 0 var(--radius-lg);
  }

  .addon.suffix {
    border-left: none;
    border-radius: 0 var(--radius-lg) var(--radius-lg) 0;
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
