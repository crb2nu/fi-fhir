<!--
  Select — native <select> at 28px with a 14px chevron. Pass `options`, or
  <option> children for groups. Inside a Field it is labelled automatically.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLSelectAttributes } from 'svelte/elements';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import Icon from './Icon.svelte';
  import { getFieldContext, joinIds } from './field-context';
  import type { ControlSize, SelectOption } from './types';

  interface Props extends Omit<HTMLSelectAttributes, 'size'> {
    value?: HTMLSelectAttributes['value'];
    options?: readonly SelectOption[] | undefined;
    /** Leading disabled option shown while nothing is selected. */
    placeholder?: string | undefined;
    size?: ControlSize;
    invalid?: boolean;
    mono?: boolean;
    children?: Snippet;
  }

  let {
    value = $bindable(),
    options,
    placeholder,
    size = 'sm',
    invalid = false,
    mono = false,
    id,
    required,
    class: className,
    'aria-describedby': describedBy,
    children,
    ...rest
  }: Props = $props();

  const field = getFieldContext();
  const isInvalid = $derived(invalid || Boolean(field?.invalid));
</script>

<span class={['ui-select', `ui-select--${size}`, className]}>
  <select
    {...rest}
    bind:value
    id={id ?? field?.id}
    required={required ?? field?.required}
    class={{ 'is-mono': mono, 'is-invalid': isInvalid }}
    aria-invalid={isInvalid ? 'true' : undefined}
    aria-describedby={joinIds(describedBy, field?.describedBy)}
  >
    {#if placeholder}
      <option value="" disabled selected={value === undefined || value === ''}>{placeholder}</option>
    {/if}
    {#if options}
      {#each options as option (option.value)}
        <option value={option.value} disabled={option.disabled}>{option.label}</option>
      {/each}
    {/if}
    {@render children?.()}
  </select>
  <Icon icon={ChevronDown} size={14} class="ui-select-chevron" />
</span>

<style>
  .ui-select {
    position: relative;
    display: inline-flex;
    width: 100%;
    min-width: 0;
  }

  select {
    width: 100%;
    min-width: 0;
    height: var(--size-control-sm);
    padding: 0 26px 0 var(--space-2);
    background: var(--color-bg-input);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    color: var(--color-text-primary);
    font: inherit;
    font-size: var(--text-ui);
    appearance: none;
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .ui-select--md select {
    height: var(--size-control-md);
  }

  select.is-mono {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }

  select:hover:not(:disabled) {
    border-color: var(--color-border-strong);
  }

  select:focus,
  select:focus-visible {
    outline: none;
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  select.is-invalid {
    border-color: var(--color-danger-border);
  }

  select:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .ui-select :global(.ui-select-chevron) {
    position: absolute;
    right: 8px;
    top: 50%;
    transform: translateY(-50%);
    color: var(--color-text-tertiary);
    pointer-events: none;
  }
</style>
