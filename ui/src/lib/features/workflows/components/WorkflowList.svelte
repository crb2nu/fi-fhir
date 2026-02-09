<script lang="ts">
  import { onMount } from 'svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Skeleton from '$lib/ui/Skeleton.svelte';
  import { fetchWorkflows } from '../workflowApi';
  import type { ListWorkflowsQuery } from '$lib/gen/graphql';

  type WorkflowItem = ListWorkflowsQuery['workflows'][number];

  let workflows: WorkflowItem[] = [];
  let loading = true;
  let error: string | null = null;

  onMount(async () => {
    try {
      const data = await fetchWorkflows();
      workflows = data.workflows;
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load workflows';
    } finally {
      loading = false;
    }
  });

  function formatTime(ts: string | null): string {
    if (!ts) return 'Never';
    try {
      return new Date(ts).toLocaleString();
    } catch {
      return ts;
    }
  }
</script>

<Panel>
  {#if loading}
    <div class="skeleton-list">
      <Skeleton height="48px" />
      <Skeleton height="48px" />
      <Skeleton height="48px" />
    </div>
  {:else if error}
    <EmptyState icon="error" title="Failed to load workflows" description={error} />
  {:else if workflows.length === 0}
    <EmptyState
      icon="inbox"
      title="No workflows found"
      description="Workflows defined in YAML will appear here. Switch to the Builder tab to create one."
    />
  {:else}
    <div class="workflow-list">
      {#each workflows as wf (wf.name)}
        <div class="workflow-row">
          <div class="workflow-name">{wf.name}</div>
          <div class="workflow-meta">
            <Badge variant={wf.enabled ? 'success' : 'default'} size="sm">
              {wf.enabled ? 'Active' : 'Inactive'}
            </Badge>
            <span class="stat">{wf.routeCount} routes</span>
            <span class="stat">{wf.eventsProcessed} events</span>
            {#if wf.errors > 0}
              <Badge variant="danger" size="sm">{wf.errors} errors</Badge>
            {/if}
          </div>
          <div class="workflow-time muted">{formatTime(wf.lastEventTime)}</div>
        </div>
      {/each}
    </div>
  {/if}
</Panel>

<style>
  .skeleton-list {
    display: grid;
    gap: 8px;
  }

  .workflow-list {
    display: grid;
    gap: 6px;
  }

  .workflow-row {
    display: grid;
    grid-template-columns: 1fr auto auto;
    gap: 12px;
    align-items: center;
    padding: 10px 12px;
    border-radius: 8px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
  }

  .workflow-row:hover {
    background: var(--color-bg-hover);
  }

  .workflow-name {
    font-weight: 700;
    color: var(--color-text-primary);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  }

  .workflow-meta {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .stat {
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
  }

  .workflow-time {
    font-size: 0.85rem;
    white-space: nowrap;
  }

  .muted {
    color: var(--color-text-muted);
  }

  @media (max-width: 640px) {
    .workflow-row {
      grid-template-columns: 1fr;
      gap: 6px;
    }

    .workflow-meta {
      flex-wrap: wrap;
    }
  }
</style>
