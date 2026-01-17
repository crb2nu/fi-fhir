<script lang="ts">
  import { createHL7PreviewStore } from '$lib/features/hl7/hl7PreviewStore';
  import { parseHL7Preview } from '$lib/features/hl7/hl7Preview';
  import Button from '$lib/ui/Button.svelte';
  import JsonViewer from '$lib/ui/JsonViewer.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Tabs from '$lib/ui/Tabs.svelte';
  import TextArea from '$lib/ui/TextArea.svelte';
  import WarningList from '$lib/ui/WarningList.svelte';
  import HL7Inspector from '$lib/features/hl7/components/HL7Inspector.svelte';
  import { parseHL7Path } from '$lib/domain/hl7Path';
  import type { HL7PathLocation } from '$lib/domain/hl7Path';
  import SampleInbox from '$lib/features/hl7/components/SampleInbox.svelte';
  import { createHL7SampleStore } from '$lib/features/hl7/samples/sampleStore';
  import type { HL7Sample } from '$lib/features/hl7/samples/types';
  import { onMount } from 'svelte';
  import ProfileDraftPanel from '$lib/features/hl7/components/ProfileDraftPanel.svelte';
  import { suggestFixes } from '$lib/features/hl7/profile/fixes';
  import { profileStore, selectedProfile } from '$lib/features/hl7/profile/profileStore';
  import type { ProfileFix } from '$lib/features/hl7/profile/types';
  import type { NewHL7Sample } from '$lib/features/hl7/samples/types';

  const store = createHL7PreviewStore();

  let fileInputEl: HTMLInputElement | null = null;
  let dragDepth = 0;
  let isDragging = false;

  // Track the profile ID and version used for the last parse
  let lastUsedProfileId: string | null = null;
  let lastUsedProfileVersion: string | null = null;

  // Detect if profile has changed since last parse
  $: profileChanged =
    $state.result &&
    $selectedProfile &&
    (lastUsedProfileId !== $selectedProfile.id ||
      lastUsedProfileVersion !== $selectedProfile.version);
  const { state, warningsByPhase, events, hl7 } = store;
  const samplesStore = createHL7SampleStore();
  const { samples, activeId, activeSample } = samplesStore;

  let activeTab: 'samples' | 'warnings' | 'events' | 'inspector' | 'profile' = 'warnings';
  let selectedPath: string | null = null;
  let selectedLocation: HL7PathLocation | null = null;

  const tabs = [
    { key: 'samples', label: 'Samples' },
    { key: 'warnings', label: 'Warnings' },
    { key: 'events', label: 'Events' },
    { key: 'inspector', label: 'Inspector' },
    { key: 'profile', label: 'Profile draft' }
  ] as const;

  async function run() {
    state.update((s) => ({ ...s, loading: true, error: null, result: null }));
    selectedPath = null;
    selectedLocation = null;
    const snapshot = getSnapshot();
    const profileId = $selectedProfile?.id ?? null;

    try {
      const result = await parseHL7Preview({
        source: snapshot.source,
        data: snapshot.data,
        profileId
      });
      lastUsedProfileId = profileId;
      lastUsedProfileVersion = $selectedProfile?.version ?? null;
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

  function loadSample(sample: HL7Sample) {
    state.update((s) => ({ ...s, source: sample.source, data: sample.raw }));
  }

  function baseName(filename: string): string {
    return filename.replace(/\.[^.]+$/, '');
  }

  async function filesToInputs(files: File[]): Promise<NewHL7Sample[]> {
    const inputs: NewHL7Sample[] = [];
    for (const f of files) {
      const raw = await f.text();
      const inferred = baseName(f.name);
      inputs.push({
        name: inferred,
        source: inferred || ($state.source || 'ui_preview'),
        raw
      });
    }
    return inputs;
  }

  async function importFiles(files: File[], activate: 'first' | 'last' = 'first') {
    if (!files.length) return;
    const inputs = await filesToInputs(files);
    samplesStore.addMany(inputs, activate);
  }

  async function loadFromFile(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const files = input.files ? Array.from(input.files) : [];
    if (!files.length) return;

    if (files.length === 1) {
      const file = files[0]!;
      const text = await file.text();
      const inferredSource = baseName(file.name);
      state.update((s) => ({
        ...s,
        data: text,
        source: s.source && s.source !== 'ui_preview' ? s.source : inferredSource
      }));
    } else {
      await importFiles(files, 'first');
    }

    input.value = '';
  }

  function onDragEnter(e: DragEvent) {
    if ($state.loading) return;
    if (!e.dataTransfer?.types?.includes('Files')) return;
    dragDepth += 1;
    isDragging = true;
  }

  function onDragLeave() {
    dragDepth = Math.max(0, dragDepth - 1);
    if (dragDepth === 0) isDragging = false;
  }

  async function onDropFiles(e: DragEvent) {
    if ($state.loading) return;
    const files = e.dataTransfer?.files ? Array.from(e.dataTransfer.files) : [];
    if (!files.length) return;
    e.preventDefault();
    dragDepth = 0;
    isDragging = false;
    await importFiles(files, 'first');
  }

  // Generate fixes based on warnings and current profile
  $: fixes = suggestFixes($state.result?.parsePreview.warnings ?? [], $selectedProfile);

  // Apply a suggested fix to the current profile
  function applyFix(fix: ProfileFix) {
    if (!$selectedProfile) {
      console.warn('Cannot apply fix: no profile selected');
      return;
    }
    // Apply the changes to the local state
    profileStore.updateLocal(fix.changes);
    // Switch to the profile tab to show the changes
    activeTab = 'profile';
  }

  onMount(() => {
    const unsub = activeSample.subscribe((s) => {
      if (s) loadSample(s);
    });
    return () => unsub();
  });
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
        <input
          class="file-input"
          type="file"
          multiple
          accept=".hl7,.txt,.msg,.dat,text/plain"
          bind:this={fileInputEl}
          on:change={loadFromFile}
          disabled={$state.loading}
        />
        <Button
          variant="secondary"
          on:click={() => fileInputEl?.click()}
          disabled={$state.loading}
        >
          Load file
        </Button>
        <Button on:click={run} disabled={$state.loading || !$state.data.trim()}>
          {#if $state.loading}Running…{:else}Preview{/if}
        </Button>
      </div>
    </div>

    <div
      class="drop-target"
      class:dragging={isDragging}
      on:dragenter={onDragEnter}
      on:dragleave={onDragLeave}
      on:dragover|preventDefault
      on:drop={onDropFiles}
      role="region"
      aria-label="HL7 input. Drag and drop HL7 files to import."
    >
      <TextArea bind:value={$state.data} rows={12} disabled={$state.loading} />
      {#if isDragging}
        <div class="drop-hint">Drop files to import into Samples</div>
      {/if}
    </div>

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
        {#if lastUsedProfileId}
          <div class="pill profile">profile: {lastUsedProfileId}</div>
        {:else}
          <div class="pill muted">no profile</div>
        {/if}
        {#if profileChanged}
          <button class="pill stale" on:click={run} disabled={$state.loading}>
            Profile changed - Re-test
          </button>
        {/if}
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

      {#if activeTab === 'samples'}
        <SampleInbox
          samples={$samples}
          activeId={$activeId}
          disabled={$state.loading}
          currentRaw={$state.data}
          on:importFiles={async (e) => importFiles(e.detail.files, 'first')}
          on:saveCurrent={(e) => {
            const n = e.detail.name;
            const input = n ? { name: n, source: $state.source, raw: $state.data } : { source: $state.source, raw: $state.data };
            samplesStore.add(input);
          }}
          on:select={(e) => {
            samplesStore.setActive(e.detail.id);
            const s = $samples.find((x) => x.id === e.detail.id);
            if (s) loadSample(s);
          }}
          on:remove={(e) => samplesStore.remove(e.detail.id)}
          on:clear={() => samplesStore.clear()}
          on:loadExamples={() => samplesStore.loadDemoSamples()}
        />
      {:else if activeTab === 'warnings'}
        <WarningList groups={$warningsByPhase} {selectedPath} on:select={onSelectWarning} />
      {:else if activeTab === 'events'}
        <JsonViewer data={$events} />
      {:else if activeTab === 'inspector'}
        <HL7Inspector message={$hl7} selected={selectedLocation} />
      {:else}
        <ProfileDraftPanel
          fixes={fixes}
          onApplyFix={applyFix}
        />
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

  .file-input {
    display: none;
  }

  .drop-target {
    position: relative;
  }

  .drop-target.dragging {
    outline: 2px dashed rgba(59, 130, 246, 0.7);
    outline-offset: 8px;
    border-radius: 12px;
  }

  .drop-hint {
    position: absolute;
    inset: 10px;
    border-radius: 12px;
    background: rgba(15, 23, 42, 0.75);
    border: 1px solid rgba(59, 130, 246, 0.35);
    color: rgba(219, 234, 254, 0.95);
    display: grid;
    place-items: center;
    font-weight: 800;
    pointer-events: none;
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

  .pill.profile {
    border-color: rgba(59, 130, 246, 0.35);
    background: rgba(59, 130, 246, 0.12);
    color: rgba(147, 197, 253, 0.95);
  }

  .pill.muted {
    color: rgba(229, 231, 235, 0.5);
    border-color: rgba(255, 255, 255, 0.08);
  }

  .pill.stale {
    border-color: rgba(245, 158, 11, 0.45);
    background: rgba(245, 158, 11, 0.15);
    color: rgba(253, 230, 138, 0.95);
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .pill.stale:hover:not(:disabled) {
    background: rgba(245, 158, 11, 0.25);
  }

  .pill.stale:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .tabs {
    margin: 12px 0;
  }

  .errors {
    margin: 0;
    padding-left: 18px;
    color: rgba(254, 226, 226, 0.9);
  }
</style>
