<script lang="ts">
  import { onMount } from 'svelte';
  import CircleHelp from '@lucide/svelte/icons/circle-help';
  import { IconButton, Popover, Tabs, Toolbar, type TabItem } from '$lib/ui/primitives';
  import AutorouteResolver from '$lib/features/terminology/AutorouteResolver.svelte';
  import MappingBrowser from '$lib/features/terminology/MappingBrowser.svelte';
  import MappingUploader from '$lib/features/terminology/MappingUploader.svelte';
  import MappingEditor from '$lib/features/terminology/MappingEditor.svelte';
  import PendingReviewList from '$lib/features/terminology/PendingReviewList.svelte';
  import TemporalWorkflowList from '$lib/features/terminology/TemporalWorkflowList.svelte';
  import { getPendingAutorouteStats } from '$lib/features/terminology/terminologyApi';
  import type { ListMappingsQuery } from '$lib/gen/graphql';

  type MappingNode = ListMappingsQuery['listMappings']['nodes'][number];

  const PANEL_ID = 'terminology-view';

  let activeTab = 'browse';
  // Undefined until the stats load; the Review tab shows no count until then.
  let reviewCount: number | undefined = undefined;

  $: tabItems = [
    { id: 'browse', label: 'Browse', controls: PANEL_ID },
    { id: 'upload', label: 'Upload', controls: PANEL_ID },
    { id: 'review', label: 'Review', count: reviewCount, controls: PANEL_ID },
    { id: 'resolve', label: 'Resolver', controls: PANEL_ID },
    { id: 'workflows', label: 'Workflows', controls: PANEL_ID }
  ] satisfies TabItem[];

  $: activeLabel = tabItems.find((tab) => tab.id === activeTab)?.label ?? 'Browse';

  async function loadReviewCount() {
    try {
      const stats = await getPendingAutorouteStats();
      reviewCount = stats.pendingCount;
    } catch {
      // The count is optional — silently ignore
    }
  }

  // Load pending review count for the tab
  onMount(() => {
    void loadReviewCount();
  });

  // Trigger refresh when a mapping is approved or uploaded
  let browserKey = 0;
  function refreshBrowser() {
    browserKey++;
  }

  function handleReviewChange() {
    refreshBrowser();
    void loadReviewCount();
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

<div class="terminology-page">
  <Toolbar title="Terminology">
    {#snippet tabs()}
      <Tabs
        label="Terminology views"
        items={tabItems}
        value={activeTab}
        onchange={(id) => (activeTab = id)}
      />
    {/snippet}
    {#snippet actions()}
      <Popover label="About terminology" placement="bottom-end">
        {#snippet trigger(props)}
          <IconButton {...props} icon={CircleHelp} label="About terminology" />
        {/snippet}
        Mappings translate source codes into target terminologies. Resolve codes seen in HL7 intake;
        approved suggestions and CSV uploads appear in Browse.
      </Popover>
    {/snippet}
  </Toolbar>

  <div class="terminology-body" id={PANEL_ID} role="tabpanel" aria-label={activeLabel}>
    {#if activeTab === 'browse'}
      {#key browserKey}
        <MappingBrowser on:refresh={refreshBrowser} on:edit={handleEditMapping} />
      {/key}
    {:else if activeTab === 'upload'}
      <MappingUploader on:uploadComplete={refreshBrowser} />
    {:else if activeTab === 'review'}
      <PendingReviewList on:approve={handleReviewChange} on:refresh={handleReviewChange} />
    {:else if activeTab === 'resolve'}
      <AutorouteResolver on:approved={refreshBrowser} />
    {:else if activeTab === 'workflows'}
      <TemporalWorkflowList workflowType="TerminologyReviewWorkflow" />
    {/if}
  </div>
</div>

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
  .terminology-page {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }

  .terminology-body {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
  }
</style>
