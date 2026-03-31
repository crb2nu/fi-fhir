<script lang="ts">
  import { onMount } from 'svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Tabs from '$lib/ui/Tabs.svelte';
  import type { TabItem } from '$lib/ui/types';
  import AuthoringFlowRail from '$lib/features/shared/AuthoringFlowRail.svelte';
  import type { FlowStep } from '$lib/features/shared/authoringFlow';
  import AutorouteResolver from '$lib/features/terminology/AutorouteResolver.svelte';
  import MappingBrowser from '$lib/features/terminology/MappingBrowser.svelte';
  import MappingUploader from '$lib/features/terminology/MappingUploader.svelte';
  import MappingEditor from '$lib/features/terminology/MappingEditor.svelte';
  import PendingReviewList from '$lib/features/terminology/PendingReviewList.svelte';
  import TemporalWorkflowList from '$lib/features/terminology/TemporalWorkflowList.svelte';
  import { getPendingAutorouteStats } from '$lib/features/terminology/terminologyApi';
  import type { ListMappingsQuery } from '$lib/gen/graphql';

  type MappingNode = ListMappingsQuery['listMappings']['nodes'][number];

  let tabs: TabItem[] = [
    { key: 'browse', label: 'Browse' },
    { key: 'upload', label: 'Upload' },
    { key: 'review', label: 'Review' },
    { key: 'resolve', label: 'Resolver' },
    { key: 'workflows', label: 'Workflows' }
  ];

  let activeTab = 'browse';
  let reviewCount = 0;
  let flowSteps: FlowStep[] = [];

  // Load pending review count for badge
  onMount(async () => {
    try {
      const stats = await getPendingAutorouteStats();
      reviewCount = stats.pendingCount;
      tabs = tabs.map((t) =>
        t.key === 'review' ? { ...t, count: stats.pendingCount } : t
      );
    } catch {
      // Badge is optional — silently ignore
    }
  });

  // Trigger refresh when a mapping is approved or uploaded
  let browserKey = 0;
  function refreshBrowser() {
    browserKey++;
  }

  // Edit modal state
  let editingMapping: MappingNode | null = null;
  let showEditor = false;

  $: flowSteps = [
    {
      eyebrow: 'Upstream source context',
      title: 'Start from the code that appeared in HL7 or the source profile',
      description:
        'Use the HL7 preview and profile workspace first so the mapping work stays tied to the source message and the normalization rules that produced it.',
      metric: 'upstream context',
      status: activeTab === 'resolve' ? 'resolver open' : 'ready',
      actions: [
        { label: 'Open HL7 preview', variant: 'primary', href: '/hl7' },
        { label: 'Open profiles', variant: 'secondary', href: '/profiles' }
      ]
    },
    {
      eyebrow: 'Mapping action',
      title: 'Resolve, upload, or review the candidate code path',
      description:
        'Browse persistent mappings, upload CSVs from downstream teams, or let the resolver suggest a semantic match when the source code has no obvious home.',
      metric: activeTab === 'review' ? 'review queue' : 'mapping lane',
      status: activeTab === 'browse' ? 'browse' : activeTab === 'upload' ? 'upload' : 'resolve',
      actions: [
        { label: 'Browse', variant: 'secondary', onClick: () => { activeTab = 'browse'; } },
        { label: 'Resolver', variant: 'secondary', onClick: () => { activeTab = 'resolve'; } },
        { label: 'Upload CSV', variant: 'primary', onClick: () => { activeTab = 'upload'; } }
      ]
    },
    {
      eyebrow: 'Downstream workflow use',
      title: 'Confirm review queues and workflow handoff',
      description:
        'Once a mapping is approved, keep an eye on the review queue and the workflow lane so code changes don’t drift away from the operational path that consumes them.',
      metric: reviewCount ? `${reviewCount} pending` : 'review queue',
      status: activeTab === 'workflows' ? 'workflow open' : 'monitoring',
      actions: [
        { label: 'Review', variant: 'secondary', onClick: () => { activeTab = 'review'; } },
        { label: 'Workflows', variant: 'primary', onClick: () => { activeTab = 'workflows'; } },
        { label: 'Open workflows', variant: 'ghost', href: '/workflows' }
      ]
    }
  ] satisfies FlowStep[];

  function handleEditMapping(event: CustomEvent<{ mapping: MappingNode }>) {
    editingMapping = event.detail.mapping;
    showEditor = true;
  }

  function handleEditorClose() {
    showEditor = false;
    editingMapping = null;
  }

  function handleEditorSave() {
    showEditor = false;
    editingMapping = null;
    refreshBrowser();
  }
</script>

<h1>Terminology Mapping</h1>
<p class="sub">
  Map codes only after you know which source profile and HL7 message produced them, then confirm
  the downstream review and workflow path stays aligned.
</p>

<div class="flow-shell">
  <AuthoringFlowRail
    eyebrow="Terminology flow"
    title="From source code to workflow-ready mapping"
    summary="Keep the source message, profile rules, mapping decisions, and review workflows in the same mental model so the terminology work stays connected to the rest of the authoring flow."
    steps={flowSteps}
  />
</div>

<div class="tabs-wrapper">
  <Tabs {tabs} active={activeTab} onChange={(key) => (activeTab = key)} />
</div>

<Panel>
  {#if activeTab === 'browse'}
    <div class="tab-content">
      <p class="description">
        Browse and manage existing code mappings. Filter by source system, target system, equivalence,
        or date when you need to confirm which downstream workflow will use the mapping.
      </p>
      {#key browserKey}
        <MappingBrowser on:refresh={refreshBrowser} on:edit={handleEditMapping} />
      {/key}
    </div>
  {:else if activeTab === 'upload'}
    <div class="tab-content">
      <p class="description">
        Upload custom mappings from a CSV file. The CSV should have columns: source_system,
        source_code, source_display, target_system, target_code, target_display, equivalence, so
        the mapping can be traced back to the source context that produced it.
      </p>
      <MappingUploader on:uploaded={refreshBrowser} />
    </div>
  {:else if activeTab === 'review'}
    <div class="tab-content">
      <p class="description">
        Review and approve LLM-suggested mappings. High-confidence suggestions can be bulk approved,
        which keeps the upstream source context moving without stalling the downstream workflow lane.
      </p>
      <PendingReviewList on:approve={refreshBrowser} on:refresh={refreshBrowser} />
    </div>
  {:else if activeTab === 'resolve'}
    <div class="tab-content">
      <p class="description">
        Enter a source code to find or suggest a mapping. The resolver first checks persistent
        mappings, then falls back to LLM-powered semantic search if no match is found, so you can
        work directly from the source payload you saw in HL7 preview.
      </p>
      <AutorouteResolver on:approved={refreshBrowser} />
    </div>
  {:else if activeTab === 'workflows'}
    <div class="tab-content">
      <p class="description">
        Monitor Temporal workflows for terminology review processes. View status, cancel running
        workflows, or signal decisions that confirm the mapping is ready to use downstream.
      </p>
      <TemporalWorkflowList workflowType="TerminologyReviewWorkflow" />
    </div>
  {/if}
</Panel>

<!-- Mapping Editor Modal -->
{#if editingMapping}
  <MappingEditor
    mapping={editingMapping}
    bind:open={showEditor}
    on:close={handleEditorClose}
    on:save={handleEditorSave}
  />
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
    max-width: 84ch;
  }

  .flow-shell {
    margin-bottom: 14px;
  }

  .tabs-wrapper {
    margin-bottom: 16px;
  }

  .tab-content {
    padding: 8px 0;
  }

  .description {
    color: var(--color-text-tertiary);
    font-size: 0.9rem;
    line-height: 1.55;
    margin: 0 0 16px;
  }
</style>
