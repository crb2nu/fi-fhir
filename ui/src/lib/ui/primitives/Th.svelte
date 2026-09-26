<!--
  Th — sticky 28px header cell, 11px uppercase. `numeric` right-aligns (use it
  for count columns). Pass `onsort` only when the API can sort by this column;
  the header then becomes a button and exposes aria-sort.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLThAttributes } from 'svelte/elements';
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import ChevronsUpDown from '@lucide/svelte/icons/chevrons-up-down';
  import Icon from './Icon.svelte';
  import type { SortDirection } from './types';

  interface Props extends HTMLThAttributes {
    numeric?: boolean;
    /** Column width, e.g. "120px" or "20%". */
    width?: string | undefined;
    sort?: SortDirection | undefined;
    onsort?: (() => void) | undefined;
    children?: Snippet;
  }

  let {
    numeric = false,
    width,
    sort,
    onsort,
    scope = 'col',
    class: className,
    children,
    ...rest
  }: Props = $props();

  const sortIcon = $derived(
    sort === 'ascending' ? ArrowUp : sort === 'descending' ? ArrowDown : ChevronsUpDown
  );
</script>

<th
  {...rest}
  {scope}
  class={['ui-th', { 'is-numeric': numeric, 'is-sortable': Boolean(onsort) }, className]}
  style:width
  aria-sort={onsort ? (sort ?? 'none') : undefined}
>
  {#if onsort}
    <button type="button" class="ui-th-sort" onclick={onsort}>
      {@render children?.()}
      <Icon icon={sortIcon} size={12} class={sort && sort !== 'none' ? 'is-sorted' : ''} />
    </button>
  {:else}
    {@render children?.()}
  {/if}
</th>

<style>
  .ui-th {
    position: sticky;
    top: 0;
    z-index: 1;
    height: var(--table-header-height);
    padding: 0 var(--table-cell-padding-x);
    background: var(--color-bg-elevated);
    border-bottom: 1px solid var(--color-border-default);
    text-align: left;
    vertical-align: middle;
    font-size: var(--text-label);
    font-weight: var(--font-medium);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    white-space: nowrap;
    color: var(--color-text-tertiary);
  }

  .ui-th.is-numeric {
    text-align: right;
  }

  .ui-th.is-sortable {
    padding: 0;
  }

  .ui-th-sort {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    width: 100%;
    height: 100%;
    padding: 0 var(--table-cell-padding-x);
    background: none;
    border: 0;
    color: inherit;
    font: inherit;
    letter-spacing: inherit;
    text-transform: inherit;
    cursor: pointer;
  }

  .ui-th.is-numeric .ui-th-sort {
    justify-content: flex-end;
  }

  .ui-th-sort:hover {
    color: var(--color-text-primary);
  }

  .ui-th-sort:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
  }

  .ui-th-sort :global(.ui-icon) {
    opacity: 0.5;
  }

  .ui-th-sort :global(.ui-icon.is-sorted) {
    opacity: 1;
  }
</style>
