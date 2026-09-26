<!--
  /profiles — toolbar (Builder · YAML · Revisions, lifecycle actions, the one
  primary "Review & publish") over a split: the profiles table on the left,
  the selected profile's workspace on the right.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import CircleHelp from '@lucide/svelte/icons/circle-help';
  import Copy from '@lucide/svelte/icons/copy';
  import CopyPlus from '@lucide/svelte/icons/copy-plus';
  import Download from '@lucide/svelte/icons/download';
  import FileSliders from '@lucide/svelte/icons/file-sliders';
  import Plus from '@lucide/svelte/icons/plus';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import {
    Badge,
    Button,
    EmptyState,
    Field,
    Icon,
    IconButton,
    Popover,
    Table,
    Tabs,
    Td,
    Textarea,
    Th,
    Toolbar,
    Tr,
    type TabItem
  } from '$lib/ui/primitives';
  import CodeEditor from '$lib/ui/editor/CodeEditor.svelte';
  import ConfirmModal from '$lib/ui/ConfirmModal.svelte';

  import ProfileSelector from '$lib/features/hl7/components/ProfileSelector.svelte';
  import ToleranceEditor from '$lib/features/hl7/components/ToleranceEditor.svelte';
  import EventRulesEditor from '$lib/features/hl7/components/EventRulesEditor.svelte';
  import IdentifierEditor from '$lib/features/hl7/components/IdentifierEditor.svelte';
  import TerminologyEditor from '$lib/features/hl7/components/TerminologyEditor.svelte';
  import ProfileDiffModal from './ProfileDiffModal.svelte';
  import { formatProfileTimestamp } from './profileFormat';

  import {
    selectedProfile,
    originalProfile,
    isDirty as isProfileDirty,
    isLoading as isProfileLoading,
    isSaving as isProfileSaving,
    profileStore
  } from '$lib/features/hl7/profile/profileStore';
  import { fetchProfileYaml, saveProfileYaml } from '$lib/features/hl7/profile/profileYamlApi';
  import { getProfileRevisions } from '$lib/features/hl7/profile/profileApi';
  import { toSourceProfileYAML } from '$lib/features/hl7/profile/yaml';
  import type { ProfileRevision } from '$lib/gen/graphql';

  type View = 'builder' | 'yaml' | 'revisions';
  type BuilderView = 'tolerance' | 'events' | 'identifiers' | 'terminology';

  const views: readonly TabItem[] = [
    { id: 'builder', label: 'Builder' },
    { id: 'yaml', label: 'YAML' },
    { id: 'revisions', label: 'Revisions' }
  ];

  let activeTab: View = 'builder';

  const builderTabs: readonly TabItem[] = [
    { id: 'tolerance', label: 'Tolerance', controls: 'profile-builder' },
    { id: 'events', label: 'Events', controls: 'profile-builder' },
    { id: 'identifiers', label: 'Identifiers', controls: 'profile-builder' },
    { id: 'terminology', label: 'Terminology', controls: 'profile-builder' }
  ];

  let builderTab: BuilderView = 'tolerance';

  let selector: ProfileSelector | undefined;

  let yamlState: 'idle' | 'loading' | 'ready' | 'saving' = 'idle';
  let yamlValue = '';
  let yamlOriginal = '';
  let yamlLoadedAt = '';
  let yamlError: string | null = null;
  let copied = false;
  let showPublishModal = false;
  let showResetConfirm = false;
  let changeSummary = '';

  async function handlePublish() {
    changeSummary = '';
    showPublishModal = true;
  }

  async function handleConfirmPublish() {
    const ok = await profileStore.saveProfile(changeSummary);
    if (ok) {
      showPublishModal = false;
      changeSummary = '';
      if ($selectedProfile) {
        await loadYaml($selectedProfile.id);
        await loadRevisions($selectedProfile.id);
      }
    }
  }

  function handleDiscard() {
    showResetConfirm = true;
  }

  async function handleDiscardConfirm() {
    showPublishModal = false;
    await profileStore.discardChanges();
  }

  type RevisionsState = {
    state: 'idle' | 'loading' | 'ready';
    loadedAt: string;
    revisions: ProfileRevision[];
    error: string | null;
  };

  let revisions: RevisionsState = { state: 'idle', loadedAt: '', revisions: [], error: null };

  $: yamlDirty = (yamlState === 'ready' || yamlState === 'saving') && yamlValue !== yamlOriginal;
  $: lifecycleBlocked = $isProfileLoading || $isProfileDirty || yamlDirty;

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

  function formatLoaded(value: string): string {
    return formatProfileTimestamp(value, { seconds: true });
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

<div class="profiles-page">
  <Toolbar title="Profiles">
    {#snippet tabs()}
      <Tabs
        label="Profile views"
        items={views}
        value={activeTab}
        onchange={(id) => (activeTab = id as View)}
      />
    {/snippet}
    {#snippet actions()}
      <Popover label="About profiles" placement="bottom-end">
        {#snippet trigger(props)}
          <IconButton {...props} icon={CircleHelp} label="About profiles" />
        {/snippet}
        A source profile controls how HL7 is parsed: tolerance, event classification, identifier
        validation and terminology mapping. Builder edits stay local until Review &amp; publish
        records a revision; Save YAML writes the YAML directly.
      </Popover>
      <Button
        variant="ghost"
        icon={Plus}
        onclick={() => selector?.openNew()}
        disabled={lifecycleBlocked}
      >
        New
      </Button>
      <Button
        variant="ghost"
        icon={CopyPlus}
        onclick={() => selector?.openDuplicate()}
        disabled={!$selectedProfile || lifecycleBlocked}
      >
        Duplicate
      </Button>
      <Button
        variant="danger"
        icon={Trash2}
        onclick={() => selector?.openDelete()}
        disabled={!$selectedProfile || lifecycleBlocked}
      >
        Delete
      </Button>
      <Button variant="primary" onclick={handlePublish} disabled={!$isProfileDirty}>
        Review &amp; publish
      </Button>
    {/snippet}
  </Toolbar>

  <div class="split">
    <section class="list" aria-label="Source profiles">
      <ProfileSelector
        bind:this={selector}
        layout="table"
        onProfileChange={handleProfileChange}
        externalDirty={yamlDirty}
      />
    </section>

    <section class="details" aria-label="Selected profile">
      {#if !$selectedProfile}
        <EmptyState icon={FileSliders} message="Select a profile to edit it." />
      {:else}
        <div class="details-head">
          <span class="details-title" title={$selectedProfile.name}>{$selectedProfile.name}</span>
          <span class="details-id text-mono" title={$selectedProfile.id}>{$selectedProfile.id}</span>
          <Badge mono>v{$selectedProfile.version}</Badge>
          {#if $isProfileDirty}
            <Badge tone="warning">Draft</Badge>
            <Badge tone="warning">Builder unsaved</Badge>
          {/if}
          {#if yamlDirty}
            <Badge tone="warning">YAML unsaved</Badge>
          {/if}
          {#if copied}
            <Badge tone="success">Copied</Badge>
          {/if}
          <span class="details-meta text-mono">
            Updated {formatProfileTimestamp($selectedProfile.updatedAt)}
            {#if $selectedProfile.createdBy}· {$selectedProfile.createdBy}{/if}
          </span>
        </div>

        <div class="view">
          {#if activeTab === 'builder'}
            <div class="view-bar">
              <Tabs
                label="Builder sections"
                items={builderTabs}
                value={builderTab}
                onchange={(id) => (builderTab = id as BuilderView)}
              />
              <span class="view-actions">
                <Button variant="ghost" icon={Download} onclick={exportYamlFromBuilder}>
                  Export YAML
                </Button>
                <Button
                  variant="ghost"
                  icon={RotateCcw}
                  onclick={handleDiscard}
                  disabled={!$isProfileDirty}
                >
                  Reset
                </Button>
              </span>
            </div>

            <div id="profile-builder" class="view-body" role="tabpanel" aria-label="{builderTab} rules">
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
          {:else if activeTab === 'yaml'}
            {#if yamlState === 'loading' || yamlState === 'idle'}
              <p class="view-status">Loading YAML</p>
            {:else}
              <div class="view-bar">
                <span class="view-actions view-actions--start">
                  <Button
                    variant="ghost"
                    icon={RefreshCw}
                    onclick={() => loadYaml($selectedProfile!.id)}
                    disabled={yamlState === 'saving'}
                  >
                    Reload
                  </Button>
                  <Button variant="ghost" icon={Copy} onclick={copyYaml} disabled={yamlState === 'saving'}>
                    Copy
                  </Button>
                  <Button
                    variant="ghost"
                    icon={Download}
                    onclick={downloadYaml}
                    disabled={yamlState === 'saving'}
                  >
                    Download
                  </Button>
                  <Button
                    variant="ghost"
                    icon={RotateCcw}
                    onclick={resetToLoaded}
                    disabled={!yamlDirty || yamlState === 'saving'}
                  >
                    Reset
                  </Button>
                </span>
                <span class="view-actions">
                  <Button
                    onclick={saveYaml}
                    loading={yamlState === 'saving'}
                    disabled={!yamlDirty || yamlState === 'saving'}
                  >
                    {yamlState === 'saving' ? 'Saving' : 'Save YAML'}
                  </Button>
                </span>
              </div>

              {#if yamlError}
                <p class="note" role="alert">
                  <Icon icon={CircleAlert} class="note-icon" />
                  <span>{yamlError}</span>
                </p>
              {/if}

              <div class="yaml-editor">
                <CodeEditor
                  language="yaml"
                  value={yamlValue}
                  on:change={(e) => {
                    yamlValue = e.detail;
                  }}
                  readOnly={yamlState === 'saving'}
                  height="100%"
                />
              </div>

              <p class="view-foot text-mono">Loaded {formatLoaded(yamlLoadedAt)}</p>
            {/if}
          {:else if activeTab === 'revisions'}
            {#if revisions.state === 'loading' || revisions.state === 'idle'}
              <p class="view-status">Loading revisions</p>
            {:else}
              <div class="view-bar">
                <span class="view-actions view-actions--start">
                  <Button
                    variant="ghost"
                    icon={RefreshCw}
                    onclick={() => loadRevisions($selectedProfile!.id)}
                  >
                    Reload
                  </Button>
                </span>
                <span class="view-meta text-mono">
                  {revisions.revisions.length}
                  {revisions.revisions.length === 1 ? 'revision' : 'revisions'} · loaded {formatLoaded(
                    revisions.loadedAt
                  )}
                </span>
              </div>

              {#if revisions.error}
                <p class="note" role="alert">
                  <Icon icon={CircleAlert} class="note-icon" />
                  <span>{revisions.error}</span>
                </p>
              {/if}

              {#if revisions.revisions.length === 0}
                {#if !revisions.error}
                  <EmptyState align="start" message="No revisions recorded for this profile." />
                {/if}
              {:else}
                <Table label="Profile revisions" layout="fixed" class="revisions-table">
                  {#snippet head()}
                    <tr>
                      <Th width="96px">Version</Th>
                      <Th width="148px">Created</Th>
                      <Th width="160px">By</Th>
                      <Th>Summary</Th>
                    </tr>
                  {/snippet}
                  {#each revisions.revisions as r (r.version)}
                    <Tr>
                      <Td mono truncate value={r.version} />
                      <Td mono muted value={formatProfileTimestamp(r.createdAt)} />
                      <Td truncate muted={!r.createdBy} value={r.createdBy ?? '—'} />
                      <Td truncate muted={!r.changeSummary} value={r.changeSummary ?? '—'} />
                    </Tr>
                  {/each}
                </Table>
              {/if}
            {/if}
          {/if}
        </div>
      {/if}
    </section>
  </div>
</div>

<ConfirmModal
  bind:open={showPublishModal}
  title="Publish changes"
  message="Publish the draft as a new revision. The summary is recorded in the revision history."
  confirmText="Publish"
  cancelText="Back to editing"
  on:confirm={handleConfirmPublish}
  on:cancel={() => (changeSummary = '')}
  loading={$isProfileSaving}
  confirmDisabled={!changeSummary.trim() || changeSummary.trim().length > 1024}
  closeOnConfirm={false}
>
  <div class="publish-dialog">
    <Field label="Change summary">
      <Textarea
        bind:value={changeSummary}
        maxlength={1024}
        rows={3}
        aria-label="Profile change summary"
        placeholder="e.g. Tolerate a missing MSH-15"
      />
    </Field>

    {#if $originalProfile && $selectedProfile}
      <ProfileDiffModal original={$originalProfile} draft={$selectedProfile} />
    {/if}
  </div>
</ConfirmModal>

<ConfirmModal
  bind:open={showResetConfirm}
  title="Discard builder changes?"
  message="Discard all local builder changes and reload the published profile?"
  confirmText="Discard"
  variant="danger"
  on:confirm={handleDiscardConfirm}
/>

<style>
  .profiles-page {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }

  .split {
    flex: 1 1 auto;
    min-height: 0;
    display: grid;
    grid-template-columns: minmax(440px, 38%) minmax(0, 1fr);
  }

  .list {
    display: flex;
    flex-direction: column;
    min-height: 0;
    min-width: 0;
  }

  .details {
    display: flex;
    flex-direction: column;
    min-height: 0;
    min-width: 0;
    border-left: 1px solid var(--color-border-subtle);
  }

  .details-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 0 0 auto;
    min-width: 0;
    height: 40px;
    padding: 0 var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
  }

  .details-title {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-ui);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .details-id {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--color-text-tertiary);
  }

  .details-meta {
    margin-left: auto;
    flex: 0 0 auto;
    white-space: nowrap;
    color: var(--color-text-tertiary);
  }

  .view {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-height: 0;
  }

  .view-bar {
    display: flex;
    align-items: stretch;
    gap: var(--space-3);
    flex: 0 0 auto;
    min-height: var(--toolbar-height);
    padding: 0 var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .view-actions {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    margin-left: auto;
  }

  .view-actions--start {
    margin-left: 0;
  }

  .view-meta {
    display: flex;
    align-items: center;
    margin-left: auto;
    color: var(--color-text-tertiary);
  }

  .view-body {
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
    padding: var(--space-3);
  }

  .view-status {
    margin: 0;
    padding: var(--space-3);
    font-size: var(--text-ui);
    color: var(--color-text-tertiary);
  }

  .yaml-editor {
    flex: 1 1 auto;
    min-height: 240px;
    position: relative;
  }

  .yaml-editor > :global(*) {
    position: absolute;
    inset: 0;
  }

  .view-foot {
    flex: 0 0 auto;
    margin: 0;
    padding: var(--space-1) var(--space-3);
    border-top: 1px solid var(--color-border-subtle);
    color: var(--color-text-tertiary);
  }

  .details :global(.revisions-table) {
    flex: 1 1 auto;
    min-height: 0;
  }

  .note {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    flex: 0 0 auto;
    margin: var(--space-2) var(--space-3) 0;
    padding: var(--space-2);
    border: 1px solid var(--color-danger-border);
    border-radius: var(--radius-sm);
    background: var(--color-danger-bg);
    color: var(--color-text-primary);
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
  }

  .note :global(.note-icon) {
    color: var(--color-danger-text);
  }

  .publish-dialog {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    margin-top: var(--space-3);
  }
</style>
