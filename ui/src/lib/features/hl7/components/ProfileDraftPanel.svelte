<script lang="ts">
  import { Badge, Button, EmptyState, Panel, Table, Tabs, Td, Th, Tr } from '$lib/ui/primitives';
  import type { TabItem } from '$lib/ui/primitives';
  import Download from '@lucide/svelte/icons/download';
  import Save from '@lucide/svelte/icons/save';
  import SlidersHorizontal from '@lucide/svelte/icons/sliders-horizontal';
  import ProfileSelector from './ProfileSelector.svelte';
  import ToleranceEditor from './ToleranceEditor.svelte';
  import EventRulesEditor from './EventRulesEditor.svelte';
  import IdentifierEditor from './IdentifierEditor.svelte';
  import TerminologyEditor from './TerminologyEditor.svelte';
  import { profileStore, selectedProfile, profileError, isDirty } from '$lib/features/hl7/profile/profileStore';
  import { toSourceProfileYAML } from '$lib/features/hl7/profile/yaml';
  import { saveProfileYaml } from '$lib/features/hl7/profile/profileYamlApi';
  import type { ProfileFix } from '$lib/features/hl7/profile/types';

  // Props
  export let fixes: readonly ProfileFix[] = [];
  export let onApplyFix: ((fix: ProfileFix) => void) | undefined = undefined;
  export let onProfileChange: ((profileId: string | null) => void) | undefined = undefined;

  // Tab configuration
  const tabs: TabItem[] = [
    { id: 'tolerance', label: 'Tolerance', controls: 'profile-draft-editor' },
    { id: 'events', label: 'Events', controls: 'profile-draft-editor' },
    { id: 'identifiers', label: 'Identifiers', controls: 'profile-draft-editor' },
    { id: 'terminology', label: 'Terminology', controls: 'profile-draft-editor' }
  ];

  let activeTab = 'tolerance';

  function handleTabChange(key: string) {
    activeTab = key;
  }

  // Handle profile change from selector
  function handleProfileChange(profileId: string | null) {
    onProfileChange?.(profileId);
  }

  // Export profile as YAML file
  function exportYaml() {
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

  let yamlSaving = false;
  let yamlError: string | null = null;

  async function saveYamlToApi() {
    if (!$selectedProfile) return;
    yamlSaving = true;
    yamlError = null;
    try {
      const yaml = toSourceProfileYAML($selectedProfile);
      await saveProfileYaml($selectedProfile.id, yaml);
      await profileStore.selectProfile($selectedProfile.id);
    } catch (e) {
      yamlError = e instanceof Error ? e.message : 'Failed to save YAML';
    } finally {
      yamlSaving = false;
    }
  }
</script>

<div class="stack">
  <Panel title="Source profile" titleTag="h3">
    <ProfileSelector onProfileChange={handleProfileChange} />

    {#if $profileError}
      <p class="error" role="alert">{$profileError}</p>
    {/if}
  </Panel>

  {#if fixes.length > 0}
    <Panel title="Suggested fixes" titleTag="h3" flush>
      {#snippet actions()}
        <Badge mono>{fixes.length}</Badge>
      {/snippet}
      {#if !$selectedProfile}
        <p class="fix-hint">Select a profile to apply these fixes.</p>
      {/if}
      <Table label="Suggested fixes" layout="fixed">
        {#snippet head()}
          <tr>
            <Th width="34%">Fix</Th>
            <Th>Change</Th>
            <Th width="84px"><span class="sr-only">Action</span></Th>
          </tr>
        {/snippet}
        {#each fixes as fix (fix.id)}
          <Tr>
            <Td truncate value={fix.title} />
            <Td muted truncate value={fix.description} />
            <Td class="action-cell">
              <Button disabled={!$selectedProfile || !onApplyFix} onclick={() => onApplyFix?.(fix)}>
                Apply
              </Button>
            </Td>
          </Tr>
        {/each}
      </Table>
    </Panel>
  {/if}

  {#if $selectedProfile}
    <Panel flush aria-label="Profile configuration">
      {#snippet header()}
        <h3 class="profile-name" title={$selectedProfile?.name}>{$selectedProfile?.name}</h3>
        <Badge mono>v{$selectedProfile?.version}</Badge>
        <span class="profile-id" title={$selectedProfile?.id}>{$selectedProfile?.id}</span>
        {#if $isDirty}
          <Badge tone="warning" dot>Unsaved changes</Badge>
        {/if}
      {/snippet}
      {#snippet actions()}
        <Button variant="ghost" icon={Save} onclick={saveYamlToApi} loading={yamlSaving}>
          {yamlSaving ? 'Saving YAML…' : 'Save YAML'}
        </Button>
        <Button variant="ghost" icon={Download} onclick={exportYaml}>Export YAML</Button>
      {/snippet}

      {#if yamlError}
        <p class="error error--bar" role="alert">{yamlError}</p>
      {/if}

      <div class="config-tabs">
        <Tabs label="Profile sections" items={tabs} value={activeTab} onchange={handleTabChange} />
      </div>

      <div class="config-body" id="profile-draft-editor" role="tabpanel">
        {#if activeTab === 'tolerance'}
          <ToleranceEditor />
        {:else if activeTab === 'events'}
          <EventRulesEditor />
        {:else if activeTab === 'identifiers'}
          <IdentifierEditor showAdvanced={true} />
        {:else if activeTab === 'terminology'}
          <TerminologyEditor />
        {/if}
      </div>
    </Panel>
  {:else}
    <Panel title="Profile configuration" titleTag="h3" flush>
      <EmptyState
        align="start"
        icon={SlidersHorizontal}
        message="Select a profile to configure tolerance, event rules, identifiers and terminology."
      />
    </Panel>
  {/if}
</div>

<style>
  .stack {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .error {
    margin: var(--space-2) 0 0;
    padding: var(--space-1) var(--space-2);
    border: 1px solid var(--color-danger-border);
    border-radius: var(--radius-sm);
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
    font-size: var(--text-xs);
  }

  .error--bar {
    margin: 0;
    border-width: 0 0 1px;
    border-radius: 0;
    padding: var(--space-2) var(--space-3);
  }

  .fix-hint {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .stack :global(.action-cell) {
    padding-right: var(--space-1);
    text-align: right;
  }

  .profile-name {
    min-width: 0;
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-ui);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .profile-id {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-tertiary);
  }

  .config-tabs {
    display: flex;
    height: 32px;
    padding: 0 var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .config-body {
    min-height: 200px;
    padding: var(--space-3);
  }
</style>
