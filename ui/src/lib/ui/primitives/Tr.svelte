<!--
  Tr — a body row. With `selectable` the row is focusable, Enter/Space call
  `onselect`, ArrowUp/ArrowDown/Home/End move focus between selectable rows,
  and `selected` draws the accent left border with a muted accent fill (the
  details pane to the right shows the selected record).
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';

  interface Props extends HTMLAttributes<HTMLTableRowElement> {
    selectable?: boolean;
    selected?: boolean;
    onselect?: (() => void) | undefined;
    children?: Snippet;
  }

  let {
    selectable = false,
    selected = false,
    onselect,
    onclick,
    onkeydown,
    class: className,
    children,
    ...rest
  }: Props = $props();

  type RowEvent<E extends Event> = E & { currentTarget: EventTarget & HTMLTableRowElement };

  function handleClick(event: RowEvent<MouseEvent>): void {
    onclick?.(event);
    if (selectable && !event.defaultPrevented) onselect?.();
  }

  function siblingRows(row: HTMLTableRowElement): HTMLTableRowElement[] {
    const body = row.parentElement;
    if (!body) return [row];
    return Array.from(body.querySelectorAll<HTMLTableRowElement>(':scope > tr[data-selectable]'));
  }

  function handleKeydown(event: RowEvent<KeyboardEvent>): void {
    onkeydown?.(event);
    if (!selectable || event.defaultPrevented) return;
    const row = event.currentTarget;
    if (event.target !== row) return; // leave keys inside cell controls alone

    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      onselect?.();
      return;
    }

    const rows = siblingRows(row);
    const index = rows.indexOf(row);
    let next: HTMLTableRowElement | undefined;
    if (event.key === 'ArrowDown') next = rows[index + 1];
    else if (event.key === 'ArrowUp') next = rows[index - 1];
    else if (event.key === 'Home') next = rows[0];
    else if (event.key === 'End') next = rows[rows.length - 1];
    else return;

    event.preventDefault();
    next?.focus();
  }
</script>

<tr
  tabindex={selectable ? 0 : undefined}
  {...rest}
  class={['ui-tr', { 'is-selectable': selectable, 'is-selected': selected }, className]}
  aria-selected={selectable ? selected : undefined}
  data-selectable={selectable ? '' : undefined}
  onclick={handleClick}
  onkeydown={handleKeydown}
>
  {@render children?.()}
</tr>

<style>
  .ui-tr.is-selectable {
    cursor: pointer;
  }

  .ui-tr:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
  }

  .ui-tr:hover > :global(td) {
    background: var(--color-bg-hover);
  }

  .ui-tr.is-selected > :global(td) {
    background: var(--color-primary-muted);
  }

  .ui-tr.is-selected > :global(td:first-child) {
    box-shadow: inset 2px 0 0 var(--color-primary);
  }
</style>
