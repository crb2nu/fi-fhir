<script lang="ts">
  import Button from '$lib/ui/Button.svelte';
  import ConfirmModal from '$lib/ui/ConfirmModal.svelte';
  import {
    profileStore,
    profileList,
    selectedProfile,
    isLoading,
    isSaving,
    isDirty
  } from '$lib/features/hl7/profile/profileStore';
  import { afterUpdate, onMount, tick } from 'svelte';
  import { createDialogFocusController } from '$lib/domain/a11yDialog';

  // Props
  export let onProfileChange: ((profileId: string | null) => void) | undefined = undefined;
  export let externalDirty: boolean = false;

  // Local state
  let activeOnly = true;
  let showNewModal = false;
  let showDeleteConfirm = false;
  let showDuplicateModal = false;
  let showDiscardConfirm = false;
  let pendingProfileId: string | null = null;
  let newProfileId = '';
  let newProfileName = '';
  let duplicateId = '';
  let duplicateName = '';
  let hasUnsavedChanges = false;

  let newModalEl: HTMLDivElement | null = null;
  let duplicateModalEl: HTMLDivElement | null = null;
  let deleteModalEl: HTMLDivElement | null = null;
  let wasNewModalOpen = false;
  let wasDuplicateModalOpen = false;
  let wasDeleteModalOpen = false;
  let newFocusCtl: ReturnType<typeof createDialogFocusController> | null = null;
  let duplicateFocusCtl: ReturnType<typeof createDialogFocusController> | null = null;
  let deleteFocusCtl: ReturnType<typeof createDialogFocusController> | null = null;

  // Load profiles on mount
  onMount(() => {
    profileStore.loadProfiles(activeOnly);
  });

  // Handle selection change
  async function handleSelect(event: Event) {
    const target = event.target as HTMLSelectElement;
    const value = target.value;

    if ($isDirty || externalDirty) {
      // Store the pending selection and show confirm modal
      pendingProfileId = value || null;
      // Reset the select to the current value while modal is shown
      target.value = $selectedProfile?.id || '';
      showDiscardConfirm = true;
      return;
    }

    await profileStore.selectProfile(value || null);
    onProfileChange?.(value || null);
  }

  // Handle confirmed discard of changes
  async function handleDiscardConfirm() {
    showDiscardConfirm = false;
    await profileStore.selectProfile(pendingProfileId);
    onProfileChange?.(pendingProfileId);
    pendingProfileId = null;
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

  $: hasUnsavedChanges = Boolean($isDirty || externalDirty);

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

  afterUpdate(() => {
    if (showNewModal && !wasNewModalOpen) {
      tick().then(() => {
        if (!newModalEl) return;
        newFocusCtl = createDialogFocusController(newModalEl);
        newFocusCtl.focusInitial();
      });
    }
    if (!showNewModal && wasNewModalOpen) {
      newFocusCtl?.restoreFocus();
      newFocusCtl = null;
    }
    wasNewModalOpen = showNewModal;

    if (showDuplicateModal && !wasDuplicateModalOpen) {
      tick().then(() => {
        if (!duplicateModalEl) return;
        duplicateFocusCtl = createDialogFocusController(duplicateModalEl);
        duplicateFocusCtl.focusInitial();
      });
    }
    if (!showDuplicateModal && wasDuplicateModalOpen) {
      duplicateFocusCtl?.restoreFocus();
      duplicateFocusCtl = null;
    }
    wasDuplicateModalOpen = showDuplicateModal;

    if (showDeleteConfirm && !wasDeleteModalOpen) {
      tick().then(() => {
        if (!deleteModalEl) return;
        deleteFocusCtl = createDialogFocusController(deleteModalEl);
        deleteFocusCtl.focusInitial();
      });
    }
    if (!showDeleteConfirm && wasDeleteModalOpen) {
      deleteFocusCtl?.restoreFocus();
      deleteFocusCtl = null;
    }
    wasDeleteModalOpen = showDeleteConfirm;
  });

  function handleWindowKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      if (showDeleteConfirm) showDeleteConfirm = false;
      else if (showDuplicateModal) showDuplicateModal = false;
      else if (showNewModal) showNewModal = false;
      return;
    }
    if (e.key === 'Tab') {
      if (showDeleteConfirm) deleteFocusCtl?.onKeydown(e);
      else if (showDuplicateModal) duplicateFocusCtl?.onKeydown(e);
      else if (showNewModal) newFocusCtl?.onKeydown(e);
    }
  }
</script>

<svelte:window on:keydown={handleWindowKeydown} />

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

  <label class="filter">
    <input
      type="checkbox"
      bind:checked={activeOnly}
      on:change={() => profileStore.loadProfiles(activeOnly)}
      disabled={$isLoading}
    />
    Active only
  </label>

  <div class="actions">
    <Button variant="secondary" on:click={() => profileStore.loadProfiles(activeOnly)} disabled={$isLoading}>
      Refresh
    </Button>

    <Button
      variant="secondary"
      on:click={() => (showNewModal = true)}
      disabled={$isLoading || hasUnsavedChanges}
    >
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
        disabled={$isLoading || hasUnsavedChanges}
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

      <Button
        variant="danger"
        on:click={() => (showDeleteConfirm = true)}
        disabled={$isLoading || hasUnsavedChanges}
      >
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
  <div class="modal-overlay">
    <button
      type="button"
      class="modal-backdrop"
      tabindex="-1"
      aria-label="Close dialog"
      on:click={() => (showNewModal = false)}
    ></button>
    <div
      class="modal"
      bind:this={newModalEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby="new-profile-modal-title"
      tabindex="-1"
    >
      <h3 id="new-profile-modal-title" class="modal-title">Create New Profile</h3>
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
  <div class="modal-overlay">
    <button
      type="button"
      class="modal-backdrop"
      tabindex="-1"
      aria-label="Close dialog"
      on:click={() => (showDuplicateModal = false)}
    ></button>
    <div
      class="modal"
      bind:this={duplicateModalEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby="duplicate-profile-modal-title"
      tabindex="-1"
    >
      <h3 id="duplicate-profile-modal-title" class="modal-title">Duplicate Profile</h3>
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
  <div class="modal-overlay">
    <button
      type="button"
      class="modal-backdrop"
      tabindex="-1"
      aria-label="Close dialog"
      on:click={() => (showDeleteConfirm = false)}
    ></button>
    <div
      class="modal"
      bind:this={deleteModalEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby="delete-profile-modal-title"
      tabindex="-1"
    >
      <h3 id="delete-profile-modal-title" class="modal-title">Delete Profile</h3>
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

<!-- Discard Changes Confirmation Modal -->
<ConfirmModal
  bind:open={showDiscardConfirm}
  title="Discard Changes?"
  message="You have unsaved changes. Discard them and switch profiles?"
  confirmText="Discard"
  variant="danger"
  on:confirm={handleDiscardConfirm}
/>

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

	  .filter {
	    display: inline-flex;
	    align-items: center;
	    gap: 8px;
	    color: var(--color-text-secondary);
	    font-weight: 700;
	    font-size: 0.9rem;
	    user-select: none;
	  }

	  .select {
	    width: 100%;
	    padding: 10px 12px;
	    border-radius: var(--radius-xl);
	    border: 1px solid var(--color-border-default);
	    background: var(--color-bg-input);
	    color: var(--color-text-primary);
	    outline: none;
	    font-size: 0.95rem;
	  }

	  .select:focus {
	    border-color: var(--color-border-focus);
	    box-shadow: var(--shadow-focus);
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
	    color: var(--color-text-muted);
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
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }

	  .modal-backdrop {
    position: absolute;
    inset: 0;
    border: 0;
    padding: 0;
	    background: var(--modal-backdrop);
	    cursor: default;
	  }

	  .modal {
	    position: relative;
	    z-index: 1;
	    background: var(--color-bg-base);
	    border: 1px solid var(--color-border-default);
	    border-radius: var(--modal-radius);
	    padding: 24px;
	    min-width: 360px;
	    max-width: 480px;
	  }

	  .modal-title {
    margin: 0 0 16px;
    font-size: 1.1rem;
    font-weight: 800;
	    color: var(--color-text-primary);
	  }

  .modal-body {
    display: grid;
    gap: 14px;
    margin-bottom: 20px;
  }

	  .modal-body p {
	    color: var(--color-text-secondary);
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
	    color: var(--color-text-secondary);
	    font-size: 0.9rem;
	  }

	  .input {
	    padding: 10px 12px;
	    border-radius: var(--radius-xl);
	    border: 1px solid var(--color-border-default);
	    background: var(--color-bg-input);
	    color: var(--color-text-primary);
	    outline: none;
	  }

	  .input:focus {
	    border-color: var(--color-border-focus);
	    box-shadow: var(--shadow-focus);
	  }

	  .mono {
	    font-family: var(--font-mono);
	  }

	  .hint {
	    font-size: 0.8rem;
	    color: var(--color-text-muted);
	  }
</style>
