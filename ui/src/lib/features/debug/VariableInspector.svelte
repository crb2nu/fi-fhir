<script lang="ts">
  /**
   * VariableInspector Component
   *
   * Expandable tree view of current debug step variables.
   * Renders key-value pairs with color-coded types.
   */

  export let variables: Record<string, unknown> = {};

  type ValueType = 'string' | 'number' | 'boolean' | 'null' | 'object' | 'array';

  function getType(value: unknown): ValueType {
    if (value === null || value === undefined) return 'null';
    if (Array.isArray(value)) return 'array';
    if (typeof value === 'object') return 'object';
    if (typeof value === 'number') return 'number';
    if (typeof value === 'boolean') return 'boolean';
    return 'string';
  }

  function formatValue(value: unknown): string {
    if (value === null || value === undefined) return 'null';
    if (typeof value === 'string') return `"${value}"`;
    if (typeof value === 'boolean') return value ? 'true' : 'false';
    if (typeof value === 'number') return String(value);
    return '';
  }

  function getCollectionLabel(value: unknown, type: ValueType): string {
    if (type === 'array') {
      return `Array(${Array.isArray(value) ? value.length : 0})`;
    }
    return 'Object';
  }

  function asEntries(value: unknown): [string, unknown][] {
    if (typeof value !== 'object' || value === null) return [];
    return Object.entries(value as Record<string, unknown>);
  }

  $: entries = Object.entries(variables);
  $: hasEntries = entries.length > 0;

  let expandedKeys: Record<string, boolean> = {};

  function toggleExpand(key: string): void {
    expandedKeys = { ...expandedKeys, [key]: !expandedKeys[key] };
  }
</script>

<div class="variable-inspector">
  {#if !hasEntries}
    <div class="empty">No variables</div>
  {:else}
    <ul class="var-list">
      {#each entries as [key, value] (key)}
        {@const type = getType(value)}
        <li class="var-entry">
          {#if type === 'object' || type === 'array'}
            <button
              class="var-row expandable"
              on:click={() => toggleExpand(key)}
              aria-expanded={!!expandedKeys[key]}
            >
              <span class="expand-icon" class:expanded={expandedKeys[key]}>
                <svg viewBox="0 0 12 12" fill="currentColor" aria-hidden="true">
                  <path d="M4 2l4 4-4 4" />
                </svg>
              </span>
              <span class="var-key">{key}</span>
              <span class="var-type {type}">
                {getCollectionLabel(value, type)}
              </span>
            </button>
            {#if expandedKeys[key]}
              <div class="nested">
                {#each asEntries(value) as [nk, nv] (nk)}
                  <div class="var-row nested-row">
                    <span class="var-key">{nk}</span>
                    <span class="var-value {getType(nv)}">{formatValue(nv)}</span>
                  </div>
                {/each}
              </div>
            {/if}
          {:else}
            <div class="var-row">
              <span class="var-key">{key}</span>
              <span class="var-value {type}">{formatValue(value)}</span>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .variable-inspector {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    overflow: auto;
  }

  .empty {
    padding: var(--space-4);
    color: var(--color-text-muted);
    text-align: center;
    font-family: var(--font-sans);
    font-style: italic;
  }

  .var-list {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .var-entry {
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .var-entry:last-child {
    border-bottom: none;
  }

  .var-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-3);
    min-height: 28px;
  }

  .var-row.expandable {
    width: 100%;
    background: none;
    border: none;
    cursor: pointer;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: inherit;
    text-align: left;
    transition: var(--transition-colors);
  }

  .var-row.expandable:hover {
    background: var(--color-bg-hover);
  }

  .var-row.expandable:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .expand-icon {
    display: flex;
    width: 12px;
    height: 12px;
    color: var(--color-text-muted);
    transition: transform var(--duration-fast) var(--ease-out);
    flex-shrink: 0;
  }

  .expand-icon.expanded {
    transform: rotate(90deg);
  }

  .expand-icon svg {
    width: 100%;
    height: 100%;
  }

  .nested {
    padding-left: var(--space-6);
    border-left: 1px solid var(--color-border-subtle);
    margin-left: var(--space-4);
  }

  .nested-row {
    padding: var(--space-1) var(--space-2);
  }

  .var-key {
    color: var(--color-text-secondary);
    white-space: nowrap;
    flex-shrink: 0;
  }

  .var-key::after {
    content: ':';
    color: var(--color-text-muted);
    margin-right: var(--space-1);
  }

  .var-value {
    word-break: break-all;
  }

  .var-type {
    font-size: var(--text-2xs);
    padding: 1px var(--space-1);
    border-radius: var(--radius-sm);
    white-space: nowrap;
  }

  /* Color-coded types */
  .var-value.string,
  .var-type.object {
    color: rgba(134, 239, 172, 0.95);
  }

  .var-value.number {
    color: rgba(147, 197, 253, 0.95);
  }

  .var-value.boolean {
    color: rgba(253, 186, 116, 0.95);
  }

  .var-value.null {
    color: rgba(156, 163, 175, 0.8);
    font-style: italic;
  }

  .var-type.array {
    color: rgba(192, 132, 252, 0.95);
  }
</style>
