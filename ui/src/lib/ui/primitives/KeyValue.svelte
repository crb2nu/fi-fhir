<!--
  KeyValue — a definition grid for record details (the pane beside a Table).
  Keys are 12px tertiary, values 13px (12px mono for ids). Missing values show
  an em dash, not "N/A" or an empty cell.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';
  import type { KeyValueItem } from './types';

  interface Props extends HTMLAttributes<HTMLDListElement> {
    items?: readonly KeyValueItem[];
    /** Two key/value columns side by side on wide panes. */
    columns?: 1 | 2;
    /** Extra <dt>/<dd> pairs with custom markup, appended after `items`. */
    children?: Snippet;
  }

  let { items = [], columns = 1, class: className, children, ...rest }: Props = $props();

  function display(value: KeyValueItem['value']): string {
    return value === null || value === undefined || value === '' ? '—' : String(value);
  }
</script>

<dl {...rest} class={['ui-kv', { 'ui-kv--two': columns === 2 }, className]}>
  {#each items as item (item.key)}
    <dt>{item.key}</dt>
    <dd
      class={{
        'is-mono': item.mono,
        'is-truncate': item.truncate,
        'is-empty': display(item.value) === '—'
      }}
      title={item.truncate ? display(item.value) : undefined}
    >
      {display(item.value)}
    </dd>
  {/each}
  {@render children?.()}
</dl>

<style>
  .ui-kv {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    column-gap: var(--space-4);
    row-gap: 6px;
    margin: 0;
    font-size: var(--text-ui);
    align-items: baseline;
  }

  .ui-kv--two {
    grid-template-columns: max-content minmax(0, 1fr) max-content minmax(0, 1fr);
  }

  .ui-kv :global(dt) {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
    white-space: nowrap;
  }

  .ui-kv :global(dd) {
    margin: 0;
    min-width: 0;
    color: var(--color-text-primary);
    overflow-wrap: anywhere;
  }

  dd.is-mono {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }

  dd.is-truncate {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  dd.is-empty {
    color: var(--color-text-muted);
  }
</style>
