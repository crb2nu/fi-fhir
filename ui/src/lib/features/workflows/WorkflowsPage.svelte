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
  <WorkflowList />
{:else if activeTab === 'builder'}
  <WorkflowBuilder />
{:else if activeTab === 'monitor'}
  <WorkflowMonitor />
{/if}

<style>
  h1 {
    color: #f9fafb;
    margin: 0 0 8px;
  }

  .sub {
    color: rgba(229, 231, 235, 0.86);
    line-height: 1.55;
    margin: 0 0 16px;
    max-width: 70ch;
  }

  .tabs-wrapper {
    margin-bottom: 16px;
  }
</style>
