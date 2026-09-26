<!--
  Table — lists of records. The wrapper is the scroll container so the header
  (Th) stays sticky; give the Table a height (or put it in a flex column) for
  that to matter. Rows are Tr, cells Th/Td. 30px rows, 11px uppercase headers,
  hover on every Tr, selection via <Tr selectable selected>.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';

  interface Props extends HTMLAttributes<HTMLDivElement> {
    /** Accessible name for the table (or pass `caption`). */
    label?: string | undefined;
    /** Visually hidden caption, read by screen readers. */
    caption?: string | undefined;
    /** Header row(s): <tr><Th>…</Th></tr>. */
    head?: Snippet | undefined;
    /** Body rows: <Tr>…</Tr>. */
    children?: Snippet;
    /** `fixed` honours Th widths exactly and truncates predictably. */
    layout?: 'auto' | 'fixed';
  }

  let {
    label,
    caption,
    head,
    children,
    layout = 'auto',
    class: className,
    ...rest
  }: Props = $props();
</script>

<div {...rest} class={['ui-table-wrap', className]}>
  <table class="ui-table" class:is-fixed={layout === 'fixed'} aria-label={label}>
    {#if caption}
      <caption class="sr-only">{caption}</caption>
    {/if}
    {#if head}
      <thead>{@render head()}</thead>
    {/if}
    <tbody>{@render children?.()}</tbody>
  </table>
</div>

<style>
  .ui-table-wrap {
    min-width: 0;
    min-height: 0;
    overflow: auto;
  }

  .ui-table {
    width: 100%;
    border-collapse: separate;
    border-spacing: 0;
    font-size: var(--text-ui);
    color: var(--color-text-primary);
  }

  .ui-table.is-fixed {
    table-layout: fixed;
  }
</style>
