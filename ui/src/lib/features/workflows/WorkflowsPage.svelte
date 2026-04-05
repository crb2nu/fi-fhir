<script lang="ts">
  import Tabs from '$lib/ui/Tabs.svelte';
  import type { TabItem } from '$lib/ui/types';
  import PageHeader from '$lib/ui/PageHeader.svelte';
  import AuthoringFlowRail from '$lib/features/shared/AuthoringFlowRail.svelte';
  import type { FlowStep } from '$lib/features/shared/authoringFlow';
  import WorkflowList from './components/WorkflowList.svelte';
  import WorkflowBuilder from './components/WorkflowBuilder.svelte';
  import WorkflowMonitor from './components/WorkflowMonitor.svelte';

  const tabs: readonly TabItem[] = [
    { key: 'list', label: 'Inventory' },
    { key: 'builder', label: 'Design' },
    { key: 'monitor', label: 'Verification' }
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

  const flowSteps: FlowStep[] = [
    {
      eyebrow: 'Source mapping',
      title: 'Map the source feed',
      description: 'Normalize HL7, flatfile, or EDI inputs before they reach the destination workflow.',
      actions: [
        { label: 'HL7 preview', variant: 'primary', href: '/hl7' },
        { label: 'Profiles', variant: 'secondary', href: '/profiles' }
      ]
    },
    {
      eyebrow: 'Shape destination',
      title: 'Design routes and transforms',
      description: 'Inventory keeps the catalog, Design edits routes and transforms.',
      actions: [
        { label: 'Inventory', variant: 'secondary', onClick: () => { activeTab = 'list'; } },
        { label: 'Design', variant: 'secondary', onClick: () => { activeTab = 'builder'; } }
      ]
    },
    {
      eyebrow: 'Verify handoff',
      title: 'Confirm runtime output',
      description: 'Watch runtime output and downstream events to confirm what actually executed.',
      actions: [
        { label: 'Verification', variant: 'primary', onClick: () => { activeTab = 'monitor'; } }
      ]
    }
  ];

  function handleOpenMonitor(event: CustomEvent<{ workflowName: string }>) {
    monitorWorkflowSelection = event.detail.workflowName;
    activeTab = 'monitor';
  }
</script>

<section class="page">
  <PageHeader title="Workflows" subtitle="Design destinations and verify the handoff." />

  <div class="flow-shell">
    <AuthoringFlowRail
      compact
      title="Source to destination handoff"
      steps={flowSteps}
    />
  </div>

  <div class="workspace-frame">
    <Tabs {tabs} active={activeTab} onChange={(key) => (activeTab = key)} />

    <div class="workspace">
      {#if activeTab === 'list'}
        <WorkflowList on:openBuilder={handleOpenBuilder} on:openMonitor={handleOpenMonitor} />
      {:else if activeTab === 'builder'}
        <WorkflowBuilder managedSelection={builderSelection} />
      {:else if activeTab === 'monitor'}
        <WorkflowMonitor initialWorkflowName={monitorWorkflowSelection} />
      {/if}
    </div>
  </div>
</section>

<style>
  .page {
    display: grid;
    gap: 16px;
  }

  .flow-shell {
    margin-bottom: 2px;
  }

  .workspace-frame {
    display: grid;
    gap: 16px;
    padding: 20px;
    border-radius: 24px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
    box-shadow: var(--shadow-sm);
  }

  .workspace {
    min-width: 0;
  }
</style>
