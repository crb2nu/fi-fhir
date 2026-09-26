<!--
  Textarea — multi-line input, vertical resize only. `mono` for payloads and
  expressions. Inside a Field it is labelled automatically.
-->
<script lang="ts">
  import type { HTMLTextareaAttributes } from 'svelte/elements';
  import { getFieldContext, joinIds } from './field-context';

  interface Props extends HTMLTextareaAttributes {
    value?: HTMLTextareaAttributes['value'];
    invalid?: boolean;
    mono?: boolean;
  }

  let {
    value = $bindable(),
    invalid = false,
    mono = false,
    rows = 3,
    id,
    required,
    class: className,
    'aria-describedby': describedBy,
    ...rest
  }: Props = $props();

  const field = getFieldContext();
  const isInvalid = $derived(invalid || Boolean(field?.invalid));
</script>

<textarea
  {...rest}
  bind:value
  {rows}
  id={id ?? field?.id}
  required={required ?? field?.required}
  class={['ui-textarea', { 'is-mono': mono, 'is-invalid': isInvalid }, className]}
  aria-invalid={isInvalid ? 'true' : undefined}
  aria-describedby={joinIds(describedBy, field?.describedBy)}
></textarea>

<style>
  .ui-textarea {
    width: 100%;
    min-width: 0;
    min-height: var(--size-control-md);
    padding: 6px var(--space-2);
    background: var(--color-bg-input);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    color: var(--color-text-primary);
    font: inherit;
    font-size: var(--text-ui);
    line-height: var(--leading-ui);
    resize: vertical;
    transition: var(--transition-colors);
  }

  .ui-textarea.is-mono {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }

  .ui-textarea::placeholder {
    color: var(--color-text-muted);
  }

  .ui-textarea:hover:not(:disabled) {
    border-color: var(--color-border-strong);
  }

  .ui-textarea:focus,
  .ui-textarea:focus-visible {
    outline: none;
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .ui-textarea.is-invalid {
    border-color: var(--color-danger-border);
  }

  .ui-textarea:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
