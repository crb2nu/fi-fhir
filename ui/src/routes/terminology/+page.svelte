<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import Tabs, { type TabItem } from '$lib/ui/Tabs.svelte';
  import AutorouteResolver from '$lib/features/terminology/AutorouteResolver.svelte';
  import MappingBrowser from '$lib/features/terminology/MappingBrowser.svelte';
  import MappingUploader from '$lib/features/terminology/MappingUploader.svelte';
  import MappingEditor from '$lib/features/terminology/MappingEditor.svelte';
  import PendingReviewList from '$lib/features/terminology/PendingReviewList.svelte';
  import TemporalWorkflowList from '$lib/features/terminology/TemporalWorkflowList.svelte';
  import type { ListMappingsQuery } from '$lib/gen/graphql';

  type MappingNode = ListMappingsQuery['listMappings']['nodes'][number];

  const tabs: readonly TabItem[] = [
    { key: 'browse', label: 'Browse' },
    { key: 'upload', label: 'Upload' },
    { key: 'review', label: 'Review' },
    { key: 'resolve', label: 'Resolver' },
    { key: 'workflows', label: 'Workflows' }
  ];

  let activeTab = 'browse';

  // Trigger refresh when a mapping is approved or uploaded
  let browserKey = 0;
  function refreshBrowser() {
    browserKey++;
  }

  // Edit modal state
  let editingMapping: MappingNode | null = null;
  let showEditor = false;

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
  Manage custom code mappings between source systems and standard terminologies (LOINC, SNOMED CT, ICD-10, etc.).
</p>

<div class="tabs-wrapper">
  <Tabs {tabs} active={activeTab} onChange={(key) => (activeTab = key)} />
</div>

<Panel>
  {#if activeTab === 'browse'}
    <div class="tab-content">
      <p class="description">
        Browse and manage existing code mappings. Filter by source system, target system, equivalence, or date.
      </p>
      {#key browserKey}
        <MappingBrowser on:refresh={refreshBrowser} on:edit={handleEditMapping} />
      {/key}
    </div>
  {:else if activeTab === 'upload'}
    <div class="tab-content">
      <p class="description">
        Upload custom mappings from a CSV file. The CSV should have columns: source_system,
        source_code, source_display, target_system, target_code, target_display, equivalence.
      </p>
      <MappingUploader on:uploaded={refreshBrowser} />
    </div>
  {:else if activeTab === 'review'}
    <div class="tab-content">
      <p class="description">
        Review and approve LLM-suggested mappings. High-confidence suggestions can be bulk approved.
      </p>
      <PendingReviewList on:approve={refreshBrowser} on:refresh={refreshBrowser} />
    </div>
  {:else if activeTab === 'resolve'}
    <div class="tab-content">
      <p class="description">
        Enter a source code to find or suggest a mapping. The resolver first checks persistent
        mappings, then falls back to LLM-powered semantic search if no match is found.
      </p>
      <AutorouteResolver on:approved={refreshBrowser} />
    </div>
  {:else if activeTab === 'workflows'}
    <div class="tab-content">
      <p class="description">
        Monitor Temporal workflows for terminology review processes. View status, cancel running workflows, or signal decisions.
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

  .tab-content {
    padding: 8px 0;
  }

  .description {
    color: rgba(229, 231, 235, 0.7);
    font-size: 0.9rem;
    line-height: 1.55;
    margin: 0 0 16px;
  }
</style>
