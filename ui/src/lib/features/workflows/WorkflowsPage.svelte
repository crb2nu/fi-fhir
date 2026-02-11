<script lang="ts">
  import Tabs from '$lib/ui/Tabs.svelte';
  import type { TabItem } from '$lib/ui/types';
  import WorkflowList from './components/WorkflowList.svelte';
  import WorkflowBuilder from './components/WorkflowBuilder.svelte';
  import WorkflowMonitor from './components/WorkflowMonitor.svelte';

  const tabs: readonly TabItem[] = [
    { key: 'list', label: 'Workflows' },
    { key: 'builder', label: 'Builder' },
    { key: 'monitor', label: 'Monitor' }
  ];

  let activeTab = 'list';
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

<h1>Workflow Builder</h1>
<p class="sub">
  Design event routing workflows visually. Configure filters, transforms, and actions — then preview
  the generated YAML or dry-run against sample events.
</p>

<div class="tabs-wrapper">
  <Tabs {tabs} active={activeTab} onChange={(key) => (activeTab = key)} />
</div>

{#if activeTab === 'list'}
  <WorkflowList on:openBuilder={handleOpenBuilder} on:openMonitor={handleOpenMonitor} />
{:else if activeTab === 'builder'}
  <WorkflowBuilder managedSelection={builderSelection} />
{:else if activeTab === 'monitor'}
  <WorkflowMonitor initialWorkflowName={monitorWorkflowSelection} />
{/if}

<style>
  h1 {
    color: var(--color-text-primary);
    margin: 0 0 8px;
  }

  .sub {
    color: var(--color-text-secondary);
    line-height: 1.55;
    margin: 0 0 16px;
    max-width: 70ch;
  }

  .tabs-wrapper {
    margin-bottom: 16px;
  }
</style>
