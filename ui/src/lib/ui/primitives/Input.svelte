<!--
  Input — 28px (sm, default) or 32px (md, dialogs). Inside a Field it is
  labelled/described automatically. `mono` for ids, paths and expressions.
-->
<script lang="ts">
  import type { HTMLInputAttributes } from 'svelte/elements';
  import { getFieldContext, joinIds } from './field-context';
  import type { ControlSize } from './types';

  interface Props extends Omit<HTMLInputAttributes, 'size'> {
    value?: HTMLInputAttributes['value'];
    size?: ControlSize;
    invalid?: boolean;
    mono?: boolean;
  }

  let {
    value = $bindable(),
    size = 'sm',
    invalid = false,
    mono = false,
    type = 'text',
    id,
    required,
    class: className,
    'aria-describedby': describedBy,
    ...rest
  }: Props = $props();

  const field = getFieldContext();
  const isInvalid = $derived(invalid || Boolean(field?.invalid));
</script>

<input
  {...rest}
  bind:value
  {type}
  id={id ?? field?.id}
  required={required ?? field?.required}
  class={['ui-input', `ui-input--${size}`, { 'is-mono': mono, 'is-invalid': isInvalid }, className]}
  aria-invalid={isInvalid ? 'true' : undefined}
  aria-describedby={joinIds(describedBy, field?.describedBy)}
/>

<style>
  .ui-input {
    width: 100%;
    min-width: 0;
    height: var(--size-control-sm);
    padding: 0 var(--space-2);
    background: var(--color-bg-input);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    color: var(--color-text-primary);
    font: inherit;
    font-size: var(--text-ui);
    transition: var(--transition-colors);
  }

  .ui-input--md {
    height: var(--size-control-md);
  }

  .ui-input.is-mono {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }

  .ui-input::placeholder {
    color: var(--color-text-muted);
  }

  .ui-input:hover:not(:disabled) {
    border-color: var(--color-border-strong);
  }

  .ui-input:focus,
  .ui-input:focus-visible {
    outline: none;
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .ui-input.is-invalid {
    border-color: var(--color-danger-border);
  }

  .ui-input.is-invalid:focus {
    border-color: var(--color-danger);
    box-shadow: var(--shadow-focus-danger);
  }

  .ui-input:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
</style>
