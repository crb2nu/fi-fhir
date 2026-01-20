<script lang="ts">
  import { onMount } from 'svelte';
  import Button from '$lib/ui/Button.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import TextArea from '$lib/ui/TextArea.svelte';
  import { profileStore, profileList, selectedProfile } from '$lib/features/hl7/profile/profileStore';
  import { fetchProfileYaml, saveProfileYaml } from '$lib/features/hl7/profile/profileYamlApi';

  let activeOnly = true;
  let yamlState: 'idle' | 'loading' | 'ready' | 'saving' = 'idle';
  let yamlValue = '';
  let yamlOriginal = '';
  let yamlLoadedAt = '';
  let yamlError: string | null = null;
  let copied = false;

  function isDirty(): boolean {
    return (yamlState === 'ready' || yamlState === 'saving') && yamlValue !== yamlOriginal;
  }

  async function loadProfiles(): Promise<void> {
    await profileStore.loadProfiles(activeOnly);
  }

  async function loadYaml(profileId: string): Promise<void> {
    yamlState = 'loading';
    yamlError = null;
    copied = false;
    try {
      const content = await fetchProfileYaml(profileId);
      const ts = new Date().toISOString();
      yamlLoadedAt = ts;
      yamlValue = content;
      yamlOriginal = content;
      yamlState = 'ready';
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      yamlError = msg;
      yamlState = 'ready';
    }
  }

  async function select(id: string | null): Promise<void> {
    await profileStore.selectProfile(id);
    if (id) {
      await loadYaml(id);
    } else {
      yamlState = 'idle';
      yamlError = null;
      yamlValue = '';
      yamlOriginal = '';
      yamlLoadedAt = '';
    }
  }

  function downloadYaml(): void {
    if (!$selectedProfile) return;
    if (yamlState !== 'ready' && yamlState !== 'saving') return;
    const blob = new Blob([yamlValue], { type: 'text/yaml' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${$selectedProfile.id}.yaml`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
  }

  async function copyYaml(): Promise<void> {
    if (yamlState !== 'ready' && yamlState !== 'saving') return;
    const text = yamlValue;
    copied = false;

    try {
      await navigator.clipboard.writeText(text);
      copied = true;
      setTimeout(() => (copied = false), 1200);
      return;
    } catch {
      // Fallback below.
    }

    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.left = '-9999px';
    document.body.appendChild(ta);
    ta.focus();
    ta.select();
    try {
      document.execCommand('copy');
      copied = true;
      setTimeout(() => (copied = false), 1200);
    } finally {
      document.body.removeChild(ta);
    }
  }

  async function save(): Promise<void> {
    if (!$selectedProfile) return;
    if (yamlState !== 'ready') return;
    if (!isDirty()) return;

    yamlState = 'saving';
    yamlError = null;
    try {
      await saveProfileYaml($selectedProfile.id, yamlValue);
      await profileStore.selectProfile($selectedProfile.id);
      await loadProfiles();
      await loadYaml($selectedProfile.id);
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      yamlError = msg;
      yamlState = 'ready';
    }
  }

  function resetToLoaded(): void {
    if (yamlState !== 'ready') return;
    yamlValue = yamlOriginal;
  }

  onMount(() => {
    void loadProfiles();
    const handler = (e: BeforeUnloadEvent) => {
      if (!isDirty()) return;
      e.preventDefault();
      e.returnValue = '';
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  });
</script>

<h1>Profiles</h1>
<p class="sub">
  View and edit raw Source Profile YAML. This writes directly to the backend via <span class="mono">/api/profiles/:id/yaml</span>.
  Use carefully: invalid YAML will be rejected.
</p>

<div class="grid">
  <Panel title="Select">
    <div class="row">
      <label class="label">
        <input type="checkbox" bind:checked={activeOnly} on:change={loadProfiles} />
        Active only
      </label>
      <div class="actions">
        <Button variant="secondary" on:click={loadProfiles}>Refresh list</Button>
      </div>
    </div>

    <div class="row">
      <select class="select" value={$selectedProfile?.id ?? ''} on:change={(e) => select((e.currentTarget as HTMLSelectElement).value || null)}>
        <option value="">Select a profile…</option>
        {#each $profileList as p (p.id)}
          <option value={p.id}>{p.name} (v{p.version})</option>
        {/each}
      </select>
    </div>

    {#if $selectedProfile}
      <div class="meta">
        <span class="pill">{$selectedProfile.id}</span>
        <span class="pill muted">v{$selectedProfile.version}</span>
        {#if isDirty()}
          <span class="pill warn">unsaved</span>
        {/if}
        {#if copied}
          <span class="pill ok">copied</span>
        {/if}
      </div>
    {/if}
  </Panel>

  <Panel title="YAML">
    {#if !$selectedProfile}
      <div class="empty">Select a profile to load its YAML.</div>
    {:else if yamlState === 'loading' || yamlState === 'idle'}
      <div class="empty">Loading…</div>
    {:else}
      {#if yamlError}
        <div class="error">{yamlError}</div>
      {/if}

      <div class="toolbar">
        <div class="left">
          <Button variant="secondary" on:click={() => loadYaml($selectedProfile!.id)} disabled={yamlState === 'saving'}>
            Reload
          </Button>
          <Button variant="secondary" on:click={copyYaml} disabled={yamlState === 'saving'}>
            Copy
          </Button>
          <Button variant="secondary" on:click={downloadYaml} disabled={yamlState === 'saving'}>
            Download
          </Button>
          <Button variant="secondary" on:click={resetToLoaded} disabled={!isDirty() || yamlState === 'saving'}>
            Reset
          </Button>
        </div>
        <div class="right">
          <Button on:click={save} disabled={!isDirty() || yamlState === 'saving'}>
            {yamlState === 'saving' ? 'Saving…' : 'Save'}
          </Button>
        </div>
      </div>

      <TextArea bind:value={yamlValue} rows={24} disabled={yamlState === 'saving'} />

      <div class="footer">
        <span class="muted">
          Loaded {yamlLoadedAt ? new Date(yamlLoadedAt).toLocaleString() : '-'}
        </span>
      </div>
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
    max-width: 80ch;
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
    color: rgba(229, 231, 235, 0.9);
  }

  .grid {
    display: grid;
    gap: 14px;
    grid-template-columns: 1fr;
  }

  @media (min-width: 980px) {
    .grid {
      grid-template-columns: 0.55fr 1.45fr;
      align-items: start;
    }
  }

  .row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    flex-wrap: wrap;
  }

  .label {
    display: flex;
    align-items: center;
    gap: 8px;
    color: rgba(229, 231, 235, 0.8);
    font-weight: 700;
    font-size: 0.9rem;
  }

  .actions {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
  }

  .select {
    width: 100%;
    padding: 10px 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
  }

  .select:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .meta {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
    margin-top: 12px;
  }

  .pill {
    padding: 3px 10px;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.04);
    color: rgba(229, 231, 235, 0.86);
    font-weight: 800;
    font-size: 0.85rem;
  }

  .pill.muted {
    color: rgba(229, 231, 235, 0.6);
  }

  .pill.warn {
    border-color: rgba(245, 158, 11, 0.35);
    background: rgba(245, 158, 11, 0.12);
    color: rgba(253, 230, 138, 0.95);
  }

  .pill.ok {
    border-color: rgba(16, 185, 129, 0.35);
    background: rgba(16, 185, 129, 0.12);
  }

  .empty {
    color: rgba(229, 231, 235, 0.7);
    line-height: 1.5;
  }

  .error {
    padding: 10px 12px;
    border-radius: 12px;
    border: 1px solid rgba(239, 68, 68, 0.35);
    background: rgba(239, 68, 68, 0.12);
    color: rgba(254, 202, 202, 0.95);
    font-weight: 700;
    margin-bottom: 10px;
  }

  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    flex-wrap: wrap;
    margin-bottom: 10px;
  }

  .left,
  .right {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
  }

  .footer {
    margin-top: 10px;
    display: flex;
    justify-content: flex-end;
  }

  .muted {
    color: rgba(229, 231, 235, 0.6);
    font-size: 0.85rem;
  }
</style>
