<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Tabs, { type TabItem } from '$lib/ui/Tabs.svelte';
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
    { key: 'tolerance', label: 'Tolerance' },
    { key: 'events', label: 'Events' },
    { key: 'identifiers', label: 'Identifiers' },
    { key: 'terminology', label: 'Terminology' }
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
  <!-- Profile Selector -->
  <Panel title="Source Profile">
    <ProfileSelector onProfileChange={handleProfileChange} />

    {#if $profileError}
      <div class="error-banner">
        {$profileError}
      </div>
    {/if}
  </Panel>

  <!-- Suggested Fixes (if available) -->
  {#if fixes.length > 0}
    <Panel title="Suggested Fixes ({fixes.length})">
      {#if !$selectedProfile}
        <div class="fix-hint">
          Select a profile above to apply these fixes.
        </div>
      {/if}
      <div class="fixes">
        {#each fixes as fix (fix.id)}
          <div class="fix">
            <div class="fix-text">
              <div class="fix-title">{fix.title}</div>
              <div class="fix-desc">{fix.description}</div>
            </div>
            <div class="fix-action">
              <Button
                variant="secondary"
                disabled={!$selectedProfile || !onApplyFix}
                on:click={() => onApplyFix?.(fix)}
              >
                Apply
              </Button>
            </div>
          </div>
        {/each}
      </div>
    </Panel>
  {/if}

  <!-- Profile Configuration (only shown when a profile is selected) -->
  {#if $selectedProfile}
    <Panel title="Profile Configuration">
      <div class="profile-header">
        <div class="profile-info">
          <span class="profile-name">{$selectedProfile.name}</span>
          <span class="profile-version">v{$selectedProfile.version}</span>
          <span class="profile-id mono">{$selectedProfile.id}</span>
        </div>
        <div class="profile-actions">
          {#if $isDirty}
            <span class="unsaved-badge">Unsaved changes</span>
          {/if}
          <Button variant="secondary" on:click={saveYamlToApi} disabled={yamlSaving}>
            {yamlSaving ? 'Saving YAML…' : 'Save YAML'}
          </Button>
          <Button variant="secondary" on:click={exportYaml}>Export YAML</Button>
        </div>
      </div>

      {#if yamlError}
        <div class="error-banner">
          {yamlError}
        </div>
      {/if}

      <div class="tabs-container">
        <Tabs {tabs} active={activeTab} onChange={handleTabChange} />
      </div>

      <div class="tab-content">
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
    <Panel title="Profile Configuration">
      <div class="no-profile">
        <p>Select a profile above to configure parsing options.</p>
        <p class="hint">
          Profiles control how HL7 messages are parsed, including tolerance for missing segments,
          event classification rules, identifier validation, and terminology mapping.
        </p>
      </div>
    </Panel>
  {/if}
</div>

<style>
  .stack {
    display: grid;
    gap: 12px;
  }

  .error-banner {
    margin-top: 12px;
    padding: 10px 14px;
    border-radius: 10px;
    background: rgba(239, 68, 68, 0.12);
    border: 1px solid rgba(239, 68, 68, 0.3);
    color: rgba(239, 68, 68, 0.95);
    font-size: 0.9rem;
  }

  .fix-hint {
    margin-bottom: 12px;
    padding: 10px 14px;
    border-radius: 10px;
    background: rgba(59, 130, 246, 0.08);
    border: 1px solid rgba(59, 130, 246, 0.2);
    color: rgba(147, 197, 253, 0.9);
    font-size: 0.9rem;
  }

  .fixes {
    display: grid;
    gap: 10px;
  }

  .fix {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 12px;
    align-items: start;
    padding: 10px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
  }

  .fix-title {
    font-weight: 800;
    color: rgba(243, 244, 246, 0.95);
  }

  .fix-desc {
    margin-top: 4px;
    color: rgba(229, 231, 235, 0.78);
    line-height: 1.4;
    font-size: 0.9rem;
  }

  .profile-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 16px;
    flex-wrap: wrap;
    gap: 10px;
  }

  .profile-info {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }

  .profile-name {
    font-weight: 800;
    color: rgba(243, 244, 246, 0.95);
    font-size: 1.05rem;
  }

  .profile-version {
    padding: 3px 8px;
    border-radius: 6px;
    background: rgba(59, 130, 246, 0.15);
    border: 1px solid rgba(59, 130, 246, 0.25);
    color: rgba(59, 130, 246, 0.95);
    font-size: 0.8rem;
    font-weight: 600;
  }

  .profile-id {
    color: rgba(229, 231, 235, 0.5);
    font-size: 0.85rem;
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono',
      'Courier New', monospace;
  }

  .profile-actions {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
  }

  .unsaved-badge {
    padding: 4px 10px;
    border-radius: 6px;
    background: rgba(245, 158, 11, 0.15);
    border: 1px solid rgba(245, 158, 11, 0.3);
    color: rgba(245, 158, 11, 0.9);
    font-size: 0.8rem;
    font-weight: 600;
  }

  .tabs-container {
    margin-bottom: 16px;
  }

  .tab-content {
    min-height: 200px;
  }

  .no-profile {
    padding: 24px;
    text-align: center;
    border-radius: 12px;
    border: 1px dashed rgba(255, 255, 255, 0.15);
  }

  .no-profile p {
    margin: 0;
    color: rgba(229, 231, 235, 0.7);
  }

  .no-profile .hint {
    margin-top: 10px;
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.5);
    max-width: 400px;
    margin-left: auto;
    margin-right: auto;
    line-height: 1.5;
  }
</style>
