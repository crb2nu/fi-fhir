<script lang="ts">
  /**
   * EventLineage Component
   *
   * Left-to-right flow visualization showing event processing stages.
   * Each node is color-coded by status with connecting lines between them.
   */
  import type { EventLineageNode, EventLineageStage } from './types';

  export let nodes: EventLineageNode[] = [];

  const stageIcons: Record<EventLineageStage, string> = {
    source: 'S',
    parse: 'P',
    events: 'E',
    workflow: 'W',
    actions: 'A'
  };

  function statusClass(status: EventLineageNode['status']): string {
    return `node-${status}`;
  }
</script>

<div class="event-lineage" role="figure" aria-label="Event lineage flow">
  {#if nodes.length === 0}
    <div class="lineage-empty">No lineage data</div>
  {:else}
    <div class="lineage-flow">
      {#each nodes as node, i (node.stage)}
        {#if i > 0}
          <div class="connector" aria-hidden="true">
            <svg viewBox="0 0 24 8" preserveAspectRatio="none">
              <line x1="0" y1="4" x2="20" y2="4" stroke="currentColor" stroke-width="1.5" />
              <polygon points="18,1 24,4 18,7" fill="currentColor" />
            </svg>
          </div>
        {/if}
        <div class="lineage-node {statusClass(node.status)}">
          <div class="node-icon {statusClass(node.status)}">
            {stageIcons[node.stage]}
          </div>
          <div class="node-content">
            <span class="node-label">{node.label}</span>
            <span class="node-detail">{node.detail}</span>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .event-lineage {
    overflow-x: auto;
    padding: var(--space-3);
  }

  .lineage-empty {
    padding: var(--space-4);
    color: var(--color-text-muted);
    text-align: center;
    font-size: var(--text-xs);
    font-style: italic;
  }

  .lineage-flow {
    display: flex;
    align-items: center;
    gap: 0;
    min-width: min-content;
  }

  /* Node */
  .lineage-node {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-3);
    min-width: 100px;
    flex-shrink: 0;
  }

  .node-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border-radius: var(--radius-full);
    font-size: var(--text-sm);
    font-weight: var(--font-bold);
    flex-shrink: 0;
    transition: var(--transition-all);
  }

  .node-icon.node-success {
    background: var(--color-success-bg);
    color: var(--color-success);
    border: 2px solid var(--color-success-border);
  }

  .node-icon.node-warning {
    background: var(--color-warning-bg);
    color: var(--color-warning);
    border: 2px solid var(--color-warning-border);
  }

  .node-icon.node-error {
    background: var(--color-danger-bg);
    color: var(--color-danger);
    border: 2px solid var(--color-danger-border);
  }

  .node-icon.node-pending {
    background: var(--color-bg-surface);
    color: var(--color-text-muted);
    border: 2px solid var(--color-border-default);
  }

  .node-content {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
    text-align: center;
  }

  .node-label {
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
    white-space: nowrap;
  }

  .node-detail {
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
    white-space: nowrap;
  }

  /* Connector */
  .connector {
    display: flex;
    align-items: center;
    color: var(--color-border-strong);
    flex-shrink: 0;
  }

  .connector svg {
    width: 24px;
    height: 8px;
  }
</style>
