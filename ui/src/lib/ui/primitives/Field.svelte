<!--
  Field — label (11px) + one control + hint or error text. The control inside
  (Input/Select/Textarea) picks up id, aria-describedby, aria-invalid and
  required from context. Errors replace the hint; they are one short sentence.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';
  import { setFieldContext } from './field-context';

  interface Props extends HTMLAttributes<HTMLDivElement> {
    label: string;
    hint?: string | undefined;
    error?: string | null | undefined;
    required?: boolean;
    /** Control id; generated when omitted. */
    id?: string | undefined;
    children?: Snippet;
  }

  let {
    label,
    hint,
    error,
    required = false,
    id,
    class: className,
    children,
    ...rest
  }: Props = $props();

  const uid = $props.id();
  const controlId = $derived(id ?? `${uid}-control`);
  const messageId = $derived(`${controlId}-message`);
  const hasError = $derived(Boolean(error));
  const message = $derived(error || hint);

  setFieldContext({
    get id() {
      return controlId;
    },
    get describedBy() {
      return message ? messageId : undefined;
    },
    get invalid() {
      return hasError;
    },
    get required() {
      return required;
    }
  });
</script>

<div {...rest} class={['ui-field', { 'is-invalid': hasError }, className]}>
  <label class="ui-field-label" for={controlId}>
    {label}
    {#if required}
      <span class="ui-field-required" aria-hidden="true">*</span>
    {/if}
  </label>
  {@render children?.()}
  {#if message}
    <p id={messageId} class={hasError ? 'ui-field-error' : 'ui-field-hint'}>{message}</p>
  {/if}
</div>

<style>
  .ui-field {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    min-width: 0;
  }

  .ui-field-label {
    font-size: var(--text-label);
    font-weight: var(--font-medium);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .ui-field-required {
    color: var(--color-danger-text);
    margin-left: 2px;
  }

  .ui-field-hint,
  .ui-field-error {
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
  }

  .ui-field-hint {
    color: var(--color-text-tertiary);
  }

  .ui-field-error {
    color: var(--color-danger-text);
  }
</style>
