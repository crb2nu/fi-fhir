<!--
  Td — 30px body cell, single line.
  - `mono`: identifiers, hashes, code (12px monospace).
  - `numeric`: counts and durations — right-aligned, tabular, monospace.
  - `truncate`: ellipsis instead of wrapping; the full text goes in `title`
    (pass `value` and it is used for both the text and the title).
  - `muted`: secondary information (timestamps, sources).
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLTdAttributes } from 'svelte/elements';

  interface Props extends HTMLTdAttributes {
    mono?: boolean;
    numeric?: boolean;
    truncate?: boolean;
    muted?: boolean;
    /** Plain-text content; also the tooltip when `truncate` is set. */
    value?: string | number | null | undefined;
    children?: Snippet;
  }

  let {
    mono = false,
    numeric = false,
    truncate = false,
    muted = false,
    value,
    title,
    class: className,
    children,
    ...rest
  }: Props = $props();

  const text = $derived(value === null || value === undefined ? undefined : String(value));
  const tooltip = $derived(title ?? (truncate ? text : undefined));
</script>

<td
  {...rest}
  class={[
    'ui-td',
    {
      'is-mono': mono || numeric,
      'is-numeric': numeric,
      'is-truncate': truncate,
      'is-muted': muted
    },
    className
  ]}
  title={tooltip}
>
  {#if truncate}
    <span class="ui-td-trunc">
      {#if children}{@render children()}{:else}{text ?? ''}{/if}
    </span>
  {:else if children}
    {@render children()}
  {:else}
    {text ?? ''}
  {/if}
</td>

<style>
  .ui-td {
    height: var(--table-row-height);
    padding: 0 var(--table-cell-padding-x);
    border-bottom: 1px solid var(--color-border-subtle);
    vertical-align: middle;
    white-space: nowrap;
    transition: background-color var(--duration-fast) var(--ease-out);
  }

  .ui-td.is-mono {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }

  .ui-td.is-numeric {
    text-align: right;
    font-variant-numeric: tabular-nums;
  }

  .ui-td.is-muted {
    color: var(--color-text-tertiary);
  }

  /* max-width: 0 lets the cell shrink to its share of the row, so the span can
     ellipsize under table-layout: auto as well as fixed. */
  .ui-td.is-truncate {
    max-width: 0;
  }

  .ui-td-trunc {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
