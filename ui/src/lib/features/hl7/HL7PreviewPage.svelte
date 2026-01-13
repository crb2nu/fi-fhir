<script lang="ts">
  import { createHL7PreviewStore } from '$lib/features/hl7/hl7PreviewStore';
  import { parseHL7Preview } from '$lib/features/hl7/hl7Preview';
  import Button from '$lib/ui/Button.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Tabs from '$lib/ui/Tabs.svelte';
  import TextArea from '$lib/ui/TextArea.svelte';
  import WarningList from '$lib/ui/WarningList.svelte';
  import HL7Inspector from '$lib/features/hl7/components/HL7Inspector.svelte';
  import { parseHL7Path } from '$lib/domain/hl7Path';
  import type { HL7PathLocation } from '$lib/domain/hl7Path';

  const store = createHL7PreviewStore();
  const { state, warningsByPhase, events, hl7 } = store;

  let activeTab: 'events' | 'warnings' | 'inspector' = 'warnings';
  let selectedPath: string | null = null;
  let selectedLocation: HL7PathLocation | null = null;

  const tabs = [
    { key: 'warnings', label: 'Warnings' },
    { key: 'events', label: 'Events' },
    { key: 'inspector', label: 'Inspector' }
  ] as const;

  async function run() {
    state.update((s) => ({ ...s, loading: true, error: null, result: null }));
    selectedPath = null;
    selectedLocation = null;
    const snapshot = getSnapshot();

    try {
      const result = await parseHL7Preview({ source: snapshot.source, data: snapshot.data });
      state.update((s) => ({ ...s, loading: false, result }));
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      state.update((s) => ({ ...s, loading: false, error: msg }));
    }
  }

  function onSelectWarning(
    e: CustomEvent<{ phase: string; code: string; message: string; path?: string | null }>
  ) {
    selectedPath = e.detail.path ?? null;
    selectedLocation = parseHL7Path(selectedPath);
    activeTab = selectedLocation ? 'inspector' : 'warnings';
  }

  function getSnapshot() {
    let snapshot: { source: string; data: string } | null = null;
    state.subscribe((s) => (snapshot = { source: s.source, data: s.data }))();
    if (!snapshot) {
      return { source: 'ui', data: '' };
    }
    return snapshot;
  }
</script>

<h1>HL7 Preview & Triage</h1>
<p class="sub">
  Paste sample HL7v2 messages, preview semantic extraction, and review warnings by parsing phase.
</p>

<div class="grid">
  <Panel title="Sample HL7v2">
    <div class="row">
      <label class="label">
        Source
        <input
          class="input"
          type="text"
          bind:value={$state.source}
          placeholder="epic_adt_hosp_a"
          disabled={$state.loading}
        />
      </label>
      <div class="actions">
        <Button on:click={run} disabled={$state.loading || !$state.data.trim()}>
          {#if $state.loading}Running…{:else}Preview{/if}
        </Button>
      </div>
    </div>

    <TextArea bind:value={$state.data} rows={12} disabled={$state.loading} />

    {#if $state.error}
      <div class="error">{$state.error}</div>
    {/if}
  </Panel>

  <Panel title="Results" tone={$state.error ? 'error' : 'default'}>
    {#if !$state.result}
      <div class="empty">Run a preview to see warnings and extracted events.</div>
    {:else}
      <div class="meta">
        <div class="pill {$state.result.parsePreview.success ? 'ok' : 'bad'}">
          {$state.result.parsePreview.success ? 'success' : 'failed'}
        </div>
        <div class="pill">events: {$events.length}</div>
        <div class="pill">warnings: {$state.result.parsePreview.warnings.length}</div>
      </div>

      {#if $state.result.parsePreview.errors.length}
        <Panel title="Parse errors" tone="error">
          <ul class="errors">
            {#each $state.result.parsePreview.errors as err (err)}
              <li>{err}</li>
            {/each}
          </ul>
        </Panel>
      {/if}

      <div class="tabs">
        <Tabs tabs={tabs} active={activeTab} onChange={(k) => (activeTab = k as typeof activeTab)} />
      </div>

      {#if activeTab === 'warnings'}
        <WarningList groups={$warningsByPhase} {selectedPath} on:select={onSelectWarning} />
      {:else if activeTab === 'events'}
        <pre class="json">{JSON.stringify($events, null, 2)}</pre>
      {:else}
        <HL7Inspector message={$hl7} selected={selectedLocation} />
      {/if}
    {/if}
  </Panel>
</div>

<style>
  h1 {
    color: #f9fafb;
    margin: 0 0 8px;
  }

  .sub {
    color: rgba(229, 231, 235, 0.86);
    line-height: 1.55;
    margin: 0 0 16px;
  }

  .grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: 14px;
  }

  @media (min-width: 980px) {
    .grid {
      grid-template-columns: 1.1fr 0.9fr;
      align-items: start;
    }
  }

  .row {
    display: flex;
    gap: 12px;
    align-items: flex-end;
    justify-content: space-between;
    margin-bottom: 10px;
  }

  .label {
    display: grid;
    gap: 6px;
    color: rgba(229, 231, 235, 0.8);
    font-size: 0.9rem;
    min-width: 260px;
    flex: 1;
  }

  .input {
    padding: 10px 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
  }

  .input:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    flex: 0;
  }

  .error {
    margin-top: 10px;
    padding: 10px 12px;
    border-radius: 12px;
    border: 1px solid rgba(239, 68, 68, 0.45);
    background: rgba(239, 68, 68, 0.08);
    color: rgba(254, 226, 226, 0.9);
  }

  .empty {
    color: rgba(229, 231, 235, 0.7);
  }

  .meta {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    margin-bottom: 12px;
  }

  .pill {
    padding: 4px 10px;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.04);
    color: rgba(229, 231, 235, 0.86);
    font-weight: 650;
    font-size: 0.85rem;
  }

  .pill.ok {
    border-color: rgba(16, 185, 129, 0.35);
    background: rgba(16, 185, 129, 0.12);
  }

  .pill.bad {
    border-color: rgba(239, 68, 68, 0.35);
    background: rgba(239, 68, 68, 0.12);
  }

  .tabs {
    margin: 12px 0;
  }

  .json {
    margin: 0;
    padding: 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
    color: rgba(229, 231, 235, 0.9);
    overflow: auto;
    max-height: 520px;
  }

  .errors {
    margin: 0;
    padding-left: 18px;
    color: rgba(254, 226, 226, 0.9);
  }
</style>
