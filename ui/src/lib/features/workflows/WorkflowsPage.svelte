<script lang="ts">
  import { Tabs, Toolbar } from '$lib/ui/primitives';
  import type { TabItem } from '$lib/ui/primitives';
  import WorkflowList from './components/WorkflowList.svelte';
  import WorkflowBuilder from './components/WorkflowBuilder.svelte';
  import WorkflowMonitor from './components/WorkflowMonitor.svelte';

  const VIEW_PANEL_ID = 'workflows-view';

  const tabItems: readonly TabItem[] = [
    { id: 'list', label: 'Inventory', controls: VIEW_PANEL_ID },
    { id: 'builder', label: 'Design', controls: VIEW_PANEL_ID },
    { id: 'monitor', label: 'Verification', controls: VIEW_PANEL_ID }
  ];

  let activeTab = 'builder';
  let builderSelection:
    | {
        workflowId: string;
        name: string;
        description: string | null;
        versionId: string | null;
        versionNumber: number | null;
      }
    | null = null;
  let monitorWorkflowSelection: string | null = null;

  $: activeLabel = tabItems.find((item) => item.id === activeTab)?.label ?? 'Workflows';

  function handleOpenBuilder(
    event: CustomEvent<{
      workflowId: string;
      name: string;
      description: string | null;
      versionId: string | null;
      versionNumber: number | null;
    }>
  ) {
    builderSelection = event.detail;
    activeTab = 'builder';
  }

  function handleOpenMonitor(event: CustomEvent<{ workflowName: string }>) {
    monitorWorkflowSelection = event.detail.workflowName;
    activeTab = 'monitor';
  }
</script>

<section class="page">
  <Toolbar title="Workflows">
    {#snippet tabs()}
      <Tabs
        label="Workflow views"
        items={tabItems}
        value={activeTab}
        onchange={(id) => (activeTab = id)}
      />
    {/snippet}
  </Toolbar>

  <div
    id={VIEW_PANEL_ID}
    class="view"
    class:is-fill={activeTab === 'list'}
    role="tabpanel"
    aria-label={activeLabel}
  >
    {#if activeTab === 'list'}
      <WorkflowList on:openBuilder={handleOpenBuilder} on:openMonitor={handleOpenMonitor} />
    {:else if activeTab === 'builder'}
      <WorkflowBuilder managedSelection={builderSelection} />
    {:else if activeTab === 'monitor'}
      <WorkflowMonitor initialWorkflowName={monitorWorkflowSelection} />
    {/if}
  </div>
</section>

<style>
  .page {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
    min-width: 0;
  }

  .view {
    flex: 1 1 auto;
    min-height: 0;
    min-width: 0;
    overflow: auto;
    padding: var(--space-3);
  }

  /* Inventory owns its edges: the table and the details pane scroll on their own. */
  .view.is-fill {
    display: flex;
    flex-direction: column;
    overflow: hidden;
    padding: 0;
  }
</style>
