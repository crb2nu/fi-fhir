<script lang="ts">
  import Button from '$lib/ui/Button.svelte';
  import {
    profileStore,
    profileList,
    selectedProfile,
    isLoading,
    isSaving,
    isDirty
  } from '$lib/features/hl7/profile/profileStore.svelte';
  import { onMount } from 'svelte';

  // Props
  export let onProfileChange: ((profileId: string | null) => void) | undefined = undefined;

  // Local state
  let showNewModal = false;
  let showDeleteConfirm = false;
  let showDuplicateModal = false;
  let newProfileId = '';
  let newProfileName = '';
  let duplicateId = '';
  let duplicateName = '';

  // Load profiles on mount
  onMount(() => {
    profileStore.loadProfiles();
  });

  // Handle selection change
  async function handleSelect(event: Event) {
    const target = event.target as HTMLSelectElement;
    const value = target.value;

    if ($isDirty) {
      if (!confirm('You have unsaved changes. Discard them?')) {
        // Reset the select to the current value
        target.value = $selectedProfile?.id || '';
        return;
      }
    }

    await profileStore.selectProfile(value || null);
    onProfileChange?.(value || null);
  }

  // Create new profile
  async function handleCreateNew() {
    if (!newProfileId.trim() || !newProfileName.trim()) return;

    const id = await profileStore.createNewProfile(newProfileId.trim(), newProfileName.trim());
    if (id) {
      showNewModal = false;
      newProfileId = '';
      newProfileName = '';
      onProfileChange?.(id);
    }
  }

  // Save current profile
  async function handleSave() {
    await profileStore.saveProfile();
  }

  // Delete profile
  async function handleDelete() {
    const success = await profileStore.deleteSelectedProfile();
    if (success) {
      showDeleteConfirm = false;
      onProfileChange?.(null);
    }
  }

  // Duplicate profile
  async function handleDuplicate() {
    if (!duplicateId.trim() || !duplicateName.trim()) return;

    const id = await profileStore.duplicateSelectedProfile(
      duplicateId.trim(),
      duplicateName.trim()
    );
    if (id) {
      showDuplicateModal = false;
      duplicateId = '';
      duplicateName = '';
      await profileStore.selectProfile(id);
      onProfileChange?.(id);
    }
  }

  // Cancel and discard changes
  async function handleDiscard() {
    await profileStore.discardChanges();
  }

  // Generate default ID from name
  function generateId(name: string): string {
    return name
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '_')
      .replace(/^_|_$/g, '');
  }

  $: if (newProfileName && !newProfileId) {
    newProfileId = generateId(newProfileName);
  }

  $: if (duplicateName && !duplicateId) {
    duplicateId = generateId(duplicateName);
  }
</script>

<div class="selector-row">
  <div class="select-wrapper">
    <select
      class="select"
      value={$selectedProfile?.id || ''}
      on:change={handleSelect}
      disabled={$isLoading}
    >
      <option value="">Select a profile...</option>
      {#each $profileList as profile (profile.id)}
        <option value={profile.id}>{profile.name} (v{profile.version})</option>
      {/each}
    </select>
    {#if $isLoading}
      <span class="loading-indicator">Loading...</span>
    {/if}
  </div>

  <div class="actions">
    <Button variant="secondary" on:click={() => (showNewModal = true)} disabled={$isLoading}>
      + New
    </Button>

    {#if $selectedProfile}
      <Button
        variant="secondary"
        on:click={() => {
          duplicateId = '';
          duplicateName = $selectedProfile?.name + ' (Copy)';
          showDuplicateModal = true;
        }}
        disabled={$isLoading}
      >
        Duplicate
      </Button>

      <Button
        on:click={handleSave}
        disabled={$isLoading || $isSaving || !$isDirty}
      >
        {$isSaving ? 'Saving...' : 'Save'}
      </Button>

      {#if $isDirty}
        <Button variant="secondary" on:click={handleDiscard} disabled={$isLoading || $isSaving}>
          Discard
        </Button>
      {/if}

      <Button variant="danger" on:click={() => (showDeleteConfirm = true)} disabled={$isLoading}>
        Delete
      </Button>
    {/if}
  </div>

  {#if $isDirty}
    <div class="dirty-indicator">Unsaved changes</div>
  {/if}
</div>

<!-- New Profile Modal -->
{#if showNewModal}
  <div class="modal-overlay" on:click={() => (showNewModal = false)} role="button" tabindex="-1">
    <div class="modal" on:click|stopPropagation role="dialog" aria-modal="true">
      <h3 class="modal-title">Create New Profile</h3>
      <div class="modal-body">
        <label class="label">
          Profile Name
          <input
            class="input"
            type="text"
            bind:value={newProfileName}
            placeholder="e.g., Epic ADT"
          />
        </label>
        <label class="label">
          Profile ID
          <input
            class="input mono"
            type="text"
            bind:value={newProfileId}
            placeholder="e.g., epic_adt"
          />
          <span class="hint">Used to reference this profile in API calls</span>
        </label>
      </div>
      <div class="modal-actions">
        <Button variant="secondary" on:click={() => (showNewModal = false)}>Cancel</Button>
        <Button on:click={handleCreateNew} disabled={!newProfileId.trim() || !newProfileName.trim()}>
          Create
        </Button>
      </div>
    </div>
  </div>
{/if}

<!-- Duplicate Modal -->
{#if showDuplicateModal}
  <div
    class="modal-overlay"
    on:click={() => (showDuplicateModal = false)}
    role="button"
    tabindex="-1"
  >
    <div class="modal" on:click|stopPropagation role="dialog" aria-modal="true">
      <h3 class="modal-title">Duplicate Profile</h3>
      <div class="modal-body">
        <label class="label">
          New Profile Name
          <input
            class="input"
            type="text"
            bind:value={duplicateName}
            placeholder="e.g., Epic ADT v2"
          />
        </label>
        <label class="label">
          New Profile ID
          <input class="input mono" type="text" bind:value={duplicateId} placeholder="e.g., epic_adt_v2" />
        </label>
      </div>
      <div class="modal-actions">
        <Button variant="secondary" on:click={() => (showDuplicateModal = false)}>Cancel</Button>
        <Button on:click={handleDuplicate} disabled={!duplicateId.trim() || !duplicateName.trim()}>
          Duplicate
        </Button>
      </div>
    </div>
  </div>
{/if}

<!-- Delete Confirmation Modal -->
{#if showDeleteConfirm}
  <div
    class="modal-overlay"
    on:click={() => (showDeleteConfirm = false)}
    role="button"
    tabindex="-1"
  >
    <div class="modal" on:click|stopPropagation role="dialog" aria-modal="true">
      <h3 class="modal-title">Delete Profile</h3>
      <div class="modal-body">
        <p>
          Are you sure you want to delete <strong>{$selectedProfile?.name}</strong>? This action
          cannot be undone.
        </p>
      </div>
      <div class="modal-actions">
        <Button variant="secondary" on:click={() => (showDeleteConfirm = false)}>Cancel</Button>
        <Button variant="danger" on:click={handleDelete}>Delete</Button>
      </div>
    </div>
  </div>
{/if}

<style>
  .selector-row {
    display: flex;
    gap: 10px;
    align-items: center;
    flex-wrap: wrap;
  }

  .select-wrapper {
    position: relative;
    flex: 1;
    min-width: 200px;
    max-width: 400px;
  }

  .select {
    width: 100%;
    padding: 10px 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
    font-size: 0.95rem;
  }

  .select:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .select:disabled {
    opacity: 0.6;
  }

  .loading-indicator {
    position: absolute;
    right: 40px;
    top: 50%;
    transform: translateY(-50%);
    font-size: 0.8rem;
    color: rgba(229, 231, 235, 0.6);
  }

  .actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }

  .dirty-indicator {
    padding: 6px 10px;
    border-radius: 8px;
    background: rgba(245, 158, 11, 0.15);
    border: 1px solid rgba(245, 158, 11, 0.3);
    color: rgba(245, 158, 11, 0.9);
    font-size: 0.85rem;
    font-weight: 600;
  }

  .modal-overlay {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }

  .modal {
    background: #1f2937;
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 16px;
    padding: 24px;
    min-width: 360px;
    max-width: 480px;
  }

  .modal-title {
    margin: 0 0 16px;
    font-size: 1.1rem;
    font-weight: 800;
    color: #f3f4f6;
  }

  .modal-body {
    display: grid;
    gap: 14px;
    margin-bottom: 20px;
  }

  .modal-body p {
    color: rgba(229, 231, 235, 0.85);
    line-height: 1.5;
  }

  .modal-actions {
    display: flex;
    gap: 10px;
    justify-content: flex-end;
  }

  .label {
    display: grid;
    gap: 6px;
    color: rgba(229, 231, 235, 0.8);
    font-size: 0.9rem;
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

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono',
      'Courier New', monospace;
  }

  .hint {
    font-size: 0.8rem;
    color: rgba(229, 231, 235, 0.55);
  }
</style>
