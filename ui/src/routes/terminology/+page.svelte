<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import AutorouteResolver from '$lib/features/terminology/AutorouteResolver.svelte';
  import MappingBrowser from '$lib/features/terminology/MappingBrowser.svelte';
  import MappingUploader from '$lib/features/terminology/MappingUploader.svelte';

  let activeTab: 'resolve' | 'browse' | 'upload' = 'resolve';

  // Trigger refresh when a mapping is approved or uploaded
  let browserKey = 0;
  function refreshBrowser() {
    browserKey++;
  }
</script>

<h1>Terminology Mapping</h1>
<p class="sub">
  Manage custom code mappings between source systems and standard terminologies (LOINC, SNOMED CT, ICD-10, etc.).
</p>

<div class="tabs">
  <button class:active={activeTab === 'resolve'} on:click={() => (activeTab = 'resolve')}>
    Autoroute Resolver
  </button>
  <button class:active={activeTab === 'browse'} on:click={() => (activeTab = 'browse')}>
    Browse Mappings
  </button>
  <button class:active={activeTab === 'upload'} on:click={() => (activeTab = 'upload')}>
    Upload CSV
  </button>
</div>

<Panel>
  {#if activeTab === 'resolve'}
    <div class="tab-content">
      <p class="description">
        Enter a source code to find or suggest a mapping. The resolver first checks persistent
        mappings, then falls back to LLM-powered semantic search if no match is found.
      </p>
      <AutorouteResolver
        on:approved={refreshBrowser}
      />
    </div>
  {:else if activeTab === 'browse'}
    <div class="tab-content">
      <p class="description">
        Browse and manage existing code mappings. Filter by source system, target system, or profile.
      </p>
      {#key browserKey}
        <MappingBrowser on:refresh={refreshBrowser} />
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
  {/if}
</Panel>

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

  .tabs {
    display: flex;
    gap: 8px;
    margin-bottom: 16px;
  }

  .tabs button {
    padding: 8px 16px;
    border-radius: 8px;
    border: 1px solid rgba(75, 85, 99, 0.4);
    background: rgba(31, 41, 55, 0.6);
    color: rgba(229, 231, 235, 0.8);
    cursor: pointer;
    font-weight: 500;
    transition: all 0.15s ease;
  }

  .tabs button:hover {
    background: rgba(55, 65, 81, 0.7);
    color: rgba(229, 231, 235, 0.95);
  }

  .tabs button.active {
    background: rgba(59, 130, 246, 0.15);
    border-color: rgba(59, 130, 246, 0.4);
    color: rgba(219, 234, 254, 0.95);
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
