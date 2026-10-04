<!--
  Tabs — underline tabs (WAI-ARIA tabs pattern). Roving tabindex: one tab stop,
  ArrowLeft/Right (and Up/Down) move between enabled tabs, Home/End jump.
  activation="auto" selects on focus (views that switch instantly);
  activation="manual" moves focus only and selects on Enter/Space/click.
  The active tab gets the accent underline; that is the accent's only use here.

  A tab may carry a toned `badge` (the Problems count), a `dirty` dot, and a
  `title`. With `onclose` every tab gets a close button beside it (outside the
  tab, tabindex -1) and Delete closes the focused tab. `onselect` reports every
  activation, including a click on the tab that is already selected (the
  bottom panel reopens on it); `onchange` reports changes only.
-->
<script lang="ts">
  import type { HTMLAttributes } from 'svelte/elements';
  import X from '@lucide/svelte/icons/x';
  import Badge from './Badge.svelte';
  import Icon from './Icon.svelte';
  import type { TabItem } from './types';

  interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'onchange' | 'onselect' | 'onclose'> {
    items: readonly TabItem[];
    /** id of the selected tab. */
    value?: string | undefined;
    onchange?: ((id: string) => void) | undefined;
    /** Every activation (click, Enter/Space, auto-activation), changed or not. */
    onselect?: ((id: string) => void) | undefined;
    /** Renders a close button per tab and binds Delete to it. */
    onclose?: ((id: string) => void) | undefined;
    /** Accessible name of the tablist. */
    label?: string | undefined;
    activation?: 'auto' | 'manual';
    /** Let the caller commit selection after navigation succeeds. */
    controlled?: boolean;
  }

  let {
    items,
    value = $bindable(),
    onchange,
    onselect,
    onclose,
    label,
    activation = 'auto',
    controlled = false,
    class: className,
    ...rest
  }: Props = $props();

  // Tab refs for arrow-key focus. Reactive state so `bind:this` into it is a
  // tracked binding (a plain object triggers binding_property_non_reactive).
  const buttons: Record<string, HTMLButtonElement | null> = $state({});

  // The tab stop is the selected tab, or the first enabled tab when the value
  // matches nothing enabled.
  const tabStop = $derived.by(() => {
    const selected = items.find((item) => item.id === value && !item.disabled);
    return selected?.id ?? items.find((item) => !item.disabled)?.id;
  });

  function select(id: string): void {
    onselect?.(id);
    if (id === value) return;
    if (!controlled) value = id;
    onchange?.(id);
  }

  function onKeydown(event: KeyboardEvent, fromId: string): void {
    if (event.key === 'Delete' && onclose) {
      event.preventDefault();
      onclose(fromId);
      return;
    }
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
    {#snippet tab()}
      <button
        bind:this={buttons[item.id]}
        type="button"
        role="tab"
        class={['ui-tab', item.class]}
        class:is-active={item.id === value}
        aria-selected={item.id === value}
        aria-controls={item.controls}
        tabindex={item.id === tabStop ? 0 : -1}
        disabled={item.disabled}
        title={item.title}
        data-testid={item.testid}
        onclick={() => select(item.id)}
        onkeydown={(event) => onKeydown(event, item.id)}
      >
        {#if item.icon}
          <Icon icon={item.icon} class="ui-tab-icon" />
        {/if}
        <span class="ui-tab-label">{item.label}</span>
        {#if item.badge}
          <Badge
            mono
            tone={item.badge.tone ?? 'neutral'}
            class={['ui-tab-badge', item.badge.class]}
            data-testid={item.badge.testid}
            aria-label={item.badge.label}
          >
            {item.badge.value}
          </Badge>
        {:else if item.count !== undefined}
          <span class="ui-tab-count">{item.count}</span>
        {/if}
        {#if item.dirty}
          <span class="ui-tab-dirty" role="img" aria-label="Unsaved changes"></span>
        {/if}
      </button>
    {/snippet}
    {#if onclose}
      <div class="ui-tab-item" class:is-active={item.id === value} role="presentation">
        {@render tab()}
        <button
          type="button"
          class="ui-tab-close"
          aria-label="Close {item.label}"
          title="Close"
          tabindex="-1"
          onclick={() => onclose(item.id)}
        >
          <Icon icon={X} size={14} />
        </button>
      </div>
    {:else}
      {@render tab()}
    {/if}
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

  .ui-tab-dirty {
    width: 6px;
    height: 6px;
    flex: 0 0 auto;
    border-radius: var(--radius-full);
    background: var(--color-warning);
  }

  .ui-tab-item {
    display: inline-flex;
    align-items: stretch;
    flex: 0 0 auto;
  }

  .ui-tab-close {
    display: inline-flex;
    align-self: center;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    padding: 0;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-text-tertiary);
    cursor: pointer;
    visibility: hidden;
  }

  .ui-tab-item:hover .ui-tab-close,
  .ui-tab-item:focus-within .ui-tab-close,
  .ui-tab-item.is-active .ui-tab-close {
    visibility: visible;
  }

  .ui-tab-close:hover {
    background: var(--color-bg-active);
    color: var(--color-text-primary);
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
