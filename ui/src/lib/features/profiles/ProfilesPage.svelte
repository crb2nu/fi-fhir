<script lang="ts">
  import { resolve } from '$app/paths';
  import { onMount } from 'svelte';
  import Tabs from '$lib/ui/Tabs.svelte';
  import type { TabItem } from '$lib/ui/types';
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';
  import CodeEditor from '$lib/ui/editor/CodeEditor.svelte';
  import AuthoringFlowRail from '$lib/features/shared/AuthoringFlowRail.svelte';
  import type { FlowStep } from '$lib/features/shared/authoringFlow';

  import ProfileSelector from '$lib/features/hl7/components/ProfileSelector.svelte';
  import ToleranceEditor from '$lib/features/hl7/components/ToleranceEditor.svelte';
  import EventRulesEditor from '$lib/features/hl7/components/EventRulesEditor.svelte';
  import IdentifierEditor from '$lib/features/hl7/components/IdentifierEditor.svelte';
  import TerminologyEditor from '$lib/features/hl7/components/TerminologyEditor.svelte';

  import { selectedProfile, isDirty as isProfileDirty } from '$lib/features/hl7/profile/profileStore';
  import { fetchProfileYaml, saveProfileYaml } from '$lib/features/hl7/profile/profileYamlApi';
  import { getProfileRevisions } from '$lib/features/hl7/profile/profileApi';
  import { toSourceProfileYAML } from '$lib/features/hl7/profile/yaml';
  import type { ProfileRevision } from '$lib/gen/graphql';

  const tabs: readonly TabItem[] = [
    { key: 'builder', label: 'Builder' },
    { key: 'yaml', label: 'YAML' },
    { key: 'revisions', label: 'Revisions' }
  ];

  let activeTab: 'builder' | 'yaml' | 'revisions' = 'builder';

  const builderTabs: readonly TabItem[] = [
    { key: 'tolerance', label: 'Tolerance' },
    { key: 'events', label: 'Events' },
    { key: 'identifiers', label: 'Identifiers' },
    { key: 'terminology', label: 'Terminology' }
  ];

  let builderTab: 'tolerance' | 'events' | 'identifiers' | 'terminology' = 'tolerance';

  let yamlState: 'idle' | 'loading' | 'ready' | 'saving' = 'idle';
  let yamlValue = '';
  let yamlOriginal = '';
  let yamlLoadedAt = '';
  let yamlError: string | null = null;
  let copied = false;

  type RevisionsState = {
    state: 'idle' | 'loading' | 'ready';
    loadedAt: string;
    revisions: ProfileRevision[];
    error: string | null;
  };

  let revisions: RevisionsState = { state: 'idle', loadedAt: '', revisions: [], error: null };

  $: yamlDirty = (yamlState === 'ready' || yamlState === 'saving') && yamlValue !== yamlOriginal;
  $: flowSteps = [
    {
      eyebrow: 'Normalize source feeds',
      title: 'Choose the profile that controls HL7 parsing',
      description: $selectedProfile
        ? `${$selectedProfile.name} v${$selectedProfile.version} defines tolerance, event rules, identifier validation, and terminology mapping before the message becomes a semantic event.`
        : 'Pick a Source Profile to see how it shapes raw HL7 normalization, warnings, and downstream event mapping.',
      metric: $selectedProfile ? $selectedProfile.id : 'No profile selected',
      status: $isProfileDirty ? 'builder unsaved' : 'ready',
      actions: [
        { label: 'Open HL7 preview', variant: 'primary', href: resolve('/hl7') },
        { label: 'Review terminology', variant: 'secondary', href: resolve('/terminology') }
      ]
    },
    {
      eyebrow: 'Tune the workspace',
      title: 'Edit builder rules or the source YAML',
      description:
        'Use the builder for tolerance, event, identifier, and terminology changes, then cross-check the raw YAML and revision history so the profile stays explainable.',
      metric: yamlDirty ? 'yaml unsaved' : 'yaml ready',
      status: yamlState === 'saving' ? 'saving' : 'loaded',
      actions: [
        { label: 'Builder', variant: 'secondary', onClick: () => { activeTab = 'builder'; } },
        { label: 'YAML', variant: 'secondary', onClick: () => { activeTab = 'yaml'; } },
        { label: 'Revisions', variant: 'ghost', onClick: () => { activeTab = 'revisions'; } }
      ]
    },
    {
      eyebrow: 'Downstream mapping',
      title: 'Validate what changes for semantic events and workflows',
      description:
        'Once the profile looks right, check terminology mapping and workflow usage so the same normalization rules keep producing the semantic shape that downstream tools expect.',
      metric: revisions.revisions.length ? `${revisions.revisions.length} revisions` : 'No revisions loaded',
      status: revisions.error ? 'revision error' : 'workflow ready',
      actions: [
        { label: 'Terminology mapping', variant: 'primary', href: resolve('/terminology') },
        { label: 'Workflow builder', variant: 'secondary', href: resolve('/workflows') }
      ]
    }
  ] satisfies FlowStep[];

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

  async function loadRevisions(profileId: string): Promise<void> {
    revisions = { ...revisions, state: 'loading', error: null };
    try {
      const rs = (await getProfileRevisions(profileId)) as ProfileRevision[];
      revisions = {
        state: 'ready',
        loadedAt: new Date().toISOString(),
        revisions: rs,
        error: null
      };
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      revisions = {
        state: 'ready',
        loadedAt: new Date().toISOString(),
        revisions: [],
        error: msg
      };
    }
  }

  async function handleProfileChange(profileId: string | null): Promise<void> {
    if (!profileId) {
      yamlState = 'idle';
      yamlError = null;
      yamlValue = '';
      yamlOriginal = '';
      yamlLoadedAt = '';
      revisions = { state: 'idle', loadedAt: '', revisions: [], error: null };
      return;
    }

    await Promise.all([loadYaml(profileId), loadRevisions(profileId)]);
  }

  function exportYamlFromBuilder(): void {
    if (!$selectedProfile) return;
    const yaml = toSourceProfileYAML($selectedProfile);
    const blob = new Blob([yaml], { type: 'text/yaml' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${$selectedProfile.id}.yaml`;
    document.body.appendChild(a);
    a.click();
    document.body.removeChild(a);
    URL.revokeObjectURL(url);
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

  async function saveYaml(): Promise<void> {
    if (!$selectedProfile) return;
    if (yamlState !== 'ready') return;
    if (!yamlDirty) return;

    yamlState = 'saving';
    yamlError = null;
    try {
      await saveProfileYaml($selectedProfile.id, yamlValue);
      await loadYaml($selectedProfile.id);
      await loadRevisions($selectedProfile.id);
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
    const handler = (e: BeforeUnloadEvent) => {
      if (!($isProfileDirty || yamlDirty)) return;
      e.preventDefault();
      e.returnValue = '';
    };
    window.addEventListener('beforeunload', handler);
    return () => window.removeEventListener('beforeunload', handler);
  });
</script>

<h1>Profiles</h1>
<p class="sub">
  Source Profiles shape how raw HL7 gets normalized, which warnings stay recoverable, and how
  identifiers and terminology flow into semantic events.
</p>

<div class="flow-shell">
  <AuthoringFlowRail
    eyebrow="Normalization flow"
    title="From profile edits to downstream mapping"
    summary="Keep the source profile, its YAML, and the downstream consumers visible together so the changes you make in one place are easy to confirm in the others."
    steps={flowSteps}
  />
</div>

<div class="grid">
  <Panel title="Selected Source Profile">
    <ProfileSelector onProfileChange={handleProfileChange} externalDirty={yamlDirty} />

    {#if $selectedProfile}
      <div class="meta">
        <span class="pill">{$selectedProfile.id}</span>
        <span class="pill muted">v{$selectedProfile.version}</span>
        {#if $isProfileDirty}
          <span class="pill warn">builder unsaved</span>
        {/if}
        {#if yamlDirty}
          <span class="pill warn">yaml unsaved</span>
        {/if}
        {#if copied}
          <span class="pill ok">copied</span>
        {/if}
      </div>

      <div class="context-links">
        <a class="context-link" href={resolve('/hl7')}>Open HL7 preview</a>
        <a class="context-link" href={resolve('/terminology')}>Open terminology mappings</a>
        <a class="context-link" href={resolve('/workflows')}>Check workflows</a>
      </div>

      <p class="context-copy">
        This profile controls tolerance, event rules, identifier validation, and terminology
        mapping before the message leaves the HL7 preview lane.
      </p>
    {/if}
  </Panel>

  <Panel title="Normalization workspace">
    <div class="tabs">
      <Tabs {tabs} active={activeTab} onChange={(k) => (activeTab = k as typeof activeTab)} />
    </div>

    {#if activeTab === 'builder'}
      {#if !$selectedProfile}
        <div class="empty">Select a profile to edit.</div>
      {:else}
        <div class="toolbar">
          <div class="left">
            <Button variant="secondary" on:click={exportYamlFromBuilder}>Export YAML</Button>
          </div>
          <div class="right">
            <span class="hint">Builder edits and YAML save back to the same Source Profile record.</span>
          </div>
        </div>

        <div class="tabs">
          <Tabs
            tabs={builderTabs}
            active={builderTab}
            onChange={(k) => (builderTab = k as typeof builderTab)}
          />
        </div>

        <div class="builder">
          {#if builderTab === 'tolerance'}
            <ToleranceEditor />
          {:else if builderTab === 'events'}
            <EventRulesEditor />
          {:else if builderTab === 'identifiers'}
            <IdentifierEditor showAdvanced={true} />
          {:else if builderTab === 'terminology'}
            <TerminologyEditor />
          {/if}
        </div>
      {/if}
    {:else if activeTab === 'yaml'}
      {#if !$selectedProfile}
        <div class="empty">Select a profile to inspect its YAML and revision trail.</div>
      {:else if yamlState === 'loading' || yamlState === 'idle'}
        <div class="empty">Loading…</div>
      {:else}
        {#if yamlError}
          <div class="error">{yamlError}</div>
        {/if}

        <div class="toolbar">
          <div class="left">
            <Button
              variant="secondary"
              on:click={() => loadYaml($selectedProfile!.id)}
              disabled={yamlState === 'saving'}
            >
              Reload
            </Button>
            <Button variant="secondary" on:click={copyYaml} disabled={yamlState === 'saving'}>
              Copy
            </Button>
            <Button variant="secondary" on:click={downloadYaml} disabled={yamlState === 'saving'}>
              Download
            </Button>
            <Button
              variant="secondary"
              on:click={resetToLoaded}
              disabled={!yamlDirty || yamlState === 'saving'}
            >
              Reset
            </Button>
          </div>
          <div class="right">
            <Button on:click={saveYaml} disabled={!yamlDirty || yamlState === 'saving'}>
              {yamlState === 'saving' ? 'Saving…' : 'Save YAML'}
            </Button>
          </div>
        </div>

        <CodeEditor
          language="yaml"
          value={yamlValue}
          on:change={(e) => { yamlValue = e.detail; }}
          readOnly={yamlState === 'saving'}
          height="480px"
        />

        <div class="footer">
          <span class="muted">
            Loaded {yamlLoadedAt ? new Date(yamlLoadedAt).toLocaleString() : '-'}
          </span>
        </div>
      {/if}
    {:else if activeTab === 'revisions'}
      {#if !$selectedProfile}
        <div class="empty">Select a profile to review how edits changed the normalization contract.</div>
      {:else if revisions.state === 'loading' || revisions.state === 'idle'}
        <div class="empty">Loading…</div>
      {:else}
        {#if revisions.error}
          <div class="error">{revisions.error}</div>
        {/if}

        <div class="toolbar">
          <div class="left">
            <Button
              variant="secondary"
              on:click={() => loadRevisions($selectedProfile!.id)}
            >
              Reload
            </Button>
          </div>
          <div class="right">
            <span class="muted">
              Loaded {revisions.loadedAt ? new Date(revisions.loadedAt).toLocaleString() : '-'}
            </span>
          </div>
        </div>

        {#if revisions.revisions.length === 0}
          <div class="empty">No revisions found.</div>
        {:else}
          <div class="rev-table">
            <div class="rev-head">
              <div>Version</div>
              <div>Created</div>
              <div>By</div>
              <div>Summary</div>
            </div>
            {#each revisions.revisions as r (r.version)}
              <div class="rev-row">
                <div class="mono">{r.version}</div>
                <div class="muted">{new Date(r.createdAt).toLocaleString()}</div>
                <div class="muted">{r.createdBy ?? '-'}</div>
                <div class="muted">{r.changeSummary ?? '-'}</div>
              </div>
            {/each}
          </div>
        {/if}
      {/if}
    {/if}
  </Panel>
</div>

<style>
  h1 {
    color: var(--color-text-primary);
    margin: 0 0 8px;
  }

  .sub {
    color: var(--color-text-secondary);
    line-height: 1.55;
    margin: 0 0 16px;
    max-width: 86ch;
  }

  .flow-shell {
    margin-bottom: 14px;
  }

  .mono {
    font-family: var(--font-mono);
    color: var(--color-text-primary);
  }

  .grid {
    display: grid;
    gap: 14px;
    grid-template-columns: 1fr;
  }

  @media (min-width: 980px) {
    .grid {
      grid-template-columns: 0.6fr 1.4fr;
      align-items: start;
    }
  }

  .tabs {
    margin-bottom: 12px;
  }

  .context-links {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    margin-top: 12px;
  }

  .context-link {
    display: inline-flex;
    align-items: center;
    min-height: 32px;
    padding: 0 10px;
    border-radius: 999px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-surface);
    color: var(--color-text-secondary);
    text-decoration: none;
    font-size: 0.84rem;
    font-weight: 800;
  }

  .context-link:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .context-copy {
    margin: 12px 0 0;
    color: var(--color-text-secondary);
    line-height: 1.55;
  }

  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    flex-wrap: wrap;
    margin-bottom: 12px;
  }

  .left,
  .right {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
    align-items: center;
  }

  .builder {
    min-height: 200px;
  }

  .hint {
    color: var(--color-text-muted);
    font-size: 0.85rem;
    font-weight: 700;
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
    border: 1px solid var(--color-border-strong);
    background: var(--color-bg-surface);
    color: var(--color-text-secondary);
    font-weight: 800;
    font-size: 0.85rem;
  }

  .pill.muted {
    color: var(--color-text-muted);
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
    color: var(--color-text-tertiary);
    line-height: 1.5;
    padding: 6px 0;
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

  .footer {
    margin-top: 10px;
    display: flex;
    justify-content: flex-end;
  }

  .muted {
    color: var(--color-text-muted);
    font-size: 0.85rem;
  }

  .rev-table {
    display: grid;
    gap: 8px;
  }

  .rev-head,
  .rev-row {
    display: grid;
    grid-template-columns: 140px 200px 140px 1fr;
    gap: 10px;
    align-items: baseline;
  }

  .rev-head {
    color: var(--color-text-tertiary);
    font-weight: 800;
    font-size: 0.9rem;
    padding-bottom: 6px;
    border-bottom: 1px solid var(--color-border-default);
  }

  .rev-row {
    padding: 8px 10px;
    border-radius: 12px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-elevated);
  }

  @media (max-width: 720px) {
    .rev-head,
    .rev-row {
      grid-template-columns: 1fr;
    }
  }
</style>
