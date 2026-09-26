<!--
  Popover — a small anchored surface (contextual help, a status chip's detail,
  a filter menu). The `trigger` snippet receives the aria/click props to spread
  on its button. Escape closes and returns focus to the trigger; a pointer-down
  outside closes. Positioned `fixed` from the trigger's rect so it escapes
  overflow:hidden chrome (status bar, toolbars). Radius 6px + shadow-lg: this
  is the one place elevation is allowed besides dialogs.

    <Popover label="About this view">
      {#snippet trigger(props)}
        <IconButton {...props} icon={CircleHelp} label="About this view" />
      {/snippet}
      Events are read from the store; the list refreshes every 10 s.
    </Popover>
-->
<script lang="ts">
  import { tick, type Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';
  import type { PopoverPlacement, PopoverTriggerProps } from './types';

  interface Props extends HTMLAttributes<HTMLDivElement> {
    open?: boolean;
    /** Accessible name of the popover surface. */
    label: string;
    placement?: PopoverPlacement;
    trigger: Snippet<[PopoverTriggerProps]>;
    children?: Snippet;
  }

  let {
    open = $bindable(false),
    label,
    placement = 'bottom-start',
    trigger,
    class: className,
    children,
    ...rest
  }: Props = $props();

  const uid = $props.id();
  const panelId = `${uid}-popover`;

  let anchorEl: HTMLSpanElement | undefined = $state();
  let panelEl: HTMLDivElement | undefined = $state();
  let top = $state(0);
  let left = $state(0);

  const GAP = 4;
  const MARGIN = 8;

  function toggle(): void {
    open = !open;
  }

  function focusTrigger(): void {
    anchorEl?.querySelector<HTMLElement>('button, [href], [tabindex]:not([tabindex="-1"])')?.focus();
  }

  function close(restoreFocus: boolean): void {
    open = false;
    if (restoreFocus) void tick().then(focusTrigger);
  }

  function place(): void {
    if (!anchorEl || !panelEl) return;
    const anchor = anchorEl.getBoundingClientRect();
    const panel = panelEl.getBoundingClientRect();
    const above = placement.startsWith('top');
    const alignEnd = placement.endsWith('end');

    const rawTop = above ? anchor.top - panel.height - GAP : anchor.bottom + GAP;
    const rawLeft = alignEnd ? anchor.right - panel.width : anchor.left;
    const maxLeft = window.innerWidth - panel.width - MARGIN;
    const maxTop = window.innerHeight - panel.height - MARGIN;

    left = Math.max(MARGIN, Math.min(rawLeft, maxLeft));
    top = Math.max(MARGIN, Math.min(rawTop, maxTop));
  }

  $effect(() => {
    if (!open || !panelEl) return;

    place();
    panelEl.focus({ preventScroll: true });

    const onKeydown = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      event.stopPropagation();
      close(true);
    };
    const onPointerdown = (event: Event) => {
      const target = event.target as Node | null;
      if (target && (panelEl?.contains(target) || anchorEl?.contains(target))) return;
      close(false);
    };
    const onReflow = () => place();

    document.addEventListener('keydown', onKeydown);
    document.addEventListener('pointerdown', onPointerdown, true);
    window.addEventListener('resize', onReflow);
    window.addEventListener('scroll', onReflow, true);
    return () => {
      document.removeEventListener('keydown', onKeydown);
      document.removeEventListener('pointerdown', onPointerdown, true);
      window.removeEventListener('resize', onReflow);
      window.removeEventListener('scroll', onReflow, true);
    };
  });
</script>

<span class="ui-popover-anchor" bind:this={anchorEl}>
  {@render trigger({
    'aria-expanded': open,
    'aria-controls': panelId,
    'aria-haspopup': 'dialog',
    onclick: toggle
  })}
</span>

{#if open}
  <div
    {...rest}
    bind:this={panelEl}
    id={panelId}
    class={['ui-popover', className]}
    role="dialog"
    aria-label={label}
    tabindex="-1"
    style:top="{top}px"
    style:left="{left}px"
  >
    {@render children?.()}
  </div>
{/if}

<style>
  .ui-popover-anchor {
    display: inline-flex;
  }

  .ui-popover {
    position: fixed;
    z-index: var(--z-popover);
    min-width: 180px;
    max-width: 320px;
    padding: var(--space-3);
    background: var(--color-bg-overlay);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-lg);
    color: var(--color-text-secondary);
    font-size: var(--text-ui);
    line-height: var(--leading-ui);
    animation: fadeIn var(--duration-fast) var(--ease-out);
  }

  .ui-popover:focus {
    outline: none;
  }
</style>
