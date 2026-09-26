<!--
  Tabs — underline tabs (WAI-ARIA tabs pattern). Roving tabindex: one tab stop,
  ArrowLeft/Right (and Up/Down) move between enabled tabs, Home/End jump.
  activation="auto" selects on focus (views that switch instantly);
  activation="manual" moves focus only and selects on Enter/Space/click.
  The active tab gets the accent underline; that is the accent's only use here.
-->
<script lang="ts">
  import type { HTMLAttributes } from 'svelte/elements';
  import Icon from './Icon.svelte';
  import type { TabItem } from './types';

  interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'onchange'> {
    items: readonly TabItem[];
    /** id of the selected tab. */
    value?: string | undefined;
    onchange?: ((id: string) => void) | undefined;
    /** Accessible name of the tablist. */
    label?: string | undefined;
    activation?: 'auto' | 'manual';
  }

  let {
    items,
    value = $bindable(),
    onchange,
    label,
    activation = 'auto',
    class: className,
    ...rest
  }: Props = $props();

  const buttons: Record<string, HTMLButtonElement | null> = {};

  // The tab stop is the selected tab, or the first enabled tab when the value
  // matches nothing enabled.
  const tabStop = $derived.by(() => {
    const selected = items.find((item) => item.id === value && !item.disabled);
    return selected?.id ?? items.find((item) => !item.disabled)?.id;
  });

  function select(id: string): void {
    if (id === value) return;
    value = id;
    onchange?.(id);
  }

  function onKeydown(event: KeyboardEvent, fromId: string): void {
    const enabled = items.filter((item) => !item.disabled);
    const current = enabled.findIndex((item) => item.id === fromId);
    if (current < 0 || enabled.length === 0) return;

    let next: number;
    switch (event.key) {
      case 'ArrowRight':
      case 'ArrowDown':
        next = (current + 1) % enabled.length;
        break;
      case 'ArrowLeft':
      case 'ArrowUp':
        next = (current - 1 + enabled.length) % enabled.length;
        break;
      case 'Home':
        next = 0;
        break;
      case 'End':
        next = enabled.length - 1;
        break;
      default:
        return;
    }

    event.preventDefault();
    const target = enabled[next];
    if (!target) return;
    buttons[target.id]?.focus();
    if (activation === 'auto') select(target.id);
  }
</script>

<div
  {...rest}
  class={['ui-tabs', className]}
  role="tablist"
  aria-label={label}
  aria-orientation="horizontal"
>
  {#each items as item (item.id)}
    <button
      bind:this={buttons[item.id]}
      type="button"
      role="tab"
      class="ui-tab"
      class:is-active={item.id === value}
      aria-selected={item.id === value}
      aria-controls={item.controls}
      tabindex={item.id === tabStop ? 0 : -1}
      disabled={item.disabled}
      data-testid={item.testid}
      onclick={() => select(item.id)}
      onkeydown={(event) => onKeydown(event, item.id)}
    >
      {#if item.icon}
        <Icon icon={item.icon} />
      {/if}
      <span class="ui-tab-label">{item.label}</span>
      {#if item.count !== undefined}
        <span class="ui-tab-count">{item.count}</span>
      {/if}
    </button>
  {/each}
</div>

<style>
  .ui-tabs {
    display: flex;
    align-items: stretch;
    gap: var(--space-4);
    min-width: 0;
    min-height: var(--size-control-sm);
    overflow-x: auto;
    scrollbar-width: none;
  }

  .ui-tab {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 0 2px;
    /* Equal top/bottom borders keep the label optically centred. */
    border-top: 2px solid transparent;
    border-bottom: 2px solid transparent;
    background: none;
    color: var(--color-text-tertiary);
    font: inherit;
    font-size: var(--text-ui);
    font-weight: var(--font-medium);
    white-space: nowrap;
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .ui-tab:hover:not(:disabled) {
    color: var(--color-text-primary);
  }

  .ui-tab.is-active {
    color: var(--color-text-primary);
    border-bottom-color: var(--color-primary);
  }

  .ui-tab:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
    border-radius: var(--radius-sm);
  }

  .ui-tab:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  .ui-tab-count {
    font-family: var(--font-mono);
    font-size: var(--text-label);
    font-variant-numeric: tabular-nums;
    color: var(--color-text-tertiary);
    background: var(--color-bg-active);
    border-radius: var(--radius-sm);
    padding: 1px 5px;
  }
</style>
