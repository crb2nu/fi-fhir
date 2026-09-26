<!--
  ProfileSelector — picks the source profile the builder edits and owns the
  profile lifecycle dialogs (new, duplicate, delete, save revision, discard
  guard).

  layout="select" (default): a compact row — Select, Active only, actions.
    Used by the HL7 intake profile draft panel.
  layout="table": a filters row and a profiles Table for /profiles. New,
    Duplicate and Delete are driven by the page toolbar through the exported
    openNew/openDuplicate/openDelete; publishing is the page's job.

  Every selection goes through requestSelect, which asks to discard unsaved
  builder or YAML changes (`externalDirty`) before switching.
-->
<script lang="ts">
  import Plus from '@lucide/svelte/icons/plus';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import ConfirmModal from '$lib/ui/ConfirmModal.svelte';
  import {
    Badge,
    Button,
    EmptyState,
    Field,
    Icon,
    Input,
    Select,
    Table,
    Td,
    Textarea,
    Th,
    Tr
  } from '$lib/ui/primitives';
  import {
    profileStore,
    profileList,
    selectedProfile,
    isLoading,
    isSaving,
    isDirty,
    profileError,
    type ProfileSummary
  } from '$lib/features/hl7/profile/profileStore';
  import { afterUpdate, onMount, tick } from 'svelte';
  import { createDialogFocusController } from '$lib/domain/a11yDialog';
  import { formatProfileTimestamp } from '$lib/features/profiles/profileFormat';

  // Props
  export let onProfileChange: ((profileId: string | null) => void) | undefined = undefined;
  export let externalDirty: boolean = false;
  export let layout: 'select' | 'table' = 'select';

  // The list query returns timestamps; the store keeps them only when it maps them.
  type ProfileRow = ProfileSummary & { updatedAt?: string | null };

  // Local state
  let activeOnly = true;
  let showNewModal = false;
  let showDeleteConfirm = false;
  let showDuplicateModal = false;
  let showDiscardConfirm = false;
  let showSaveConfirm = false;
  let saveChangeSummary = '';
  let pendingProfileId: string | null = null;
  let newProfileId = '';
  let newProfileName = '';
  let duplicateId = '';
  let duplicateName = '';
  let hasUnsavedChanges = false;
  let selectValue = '';

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

  function refresh() {
    profileStore.loadProfiles(activeOnly);
  }

  // Keep the compact select in step with the store.
  $: selectValue = $selectedProfile?.id ?? '';

  $: rows = $profileList as ProfileRow[];
  $: hasUpdated = rows.some((row) => Boolean(row.updatedAt));

  /**
   * Switch to another profile, asking first when the builder or YAML has
   * unsaved changes. The one path every selection control goes through.
   */
  async function requestSelect(profileId: string | null) {
    if (profileId === ($selectedProfile?.id ?? null)) return;

    if ($isDirty || externalDirty) {
      // Store the pending selection and show confirm modal
      pendingProfileId = profileId;
      showDiscardConfirm = true;
      return;
    }

    await profileStore.selectProfile(profileId);
    onProfileChange?.(profileId);
  }

  // Handle selection change from the compact select
  async function handleSelect(event: Event) {
    const value = (event.currentTarget as HTMLSelectElement).value;
    if ($isDirty || externalDirty) {
      // Put the select back on the current profile while the modal is shown.
      await tick();
      selectValue = $selectedProfile?.id ?? '';
    }
    await requestSelect(value || null);
  }

  // Handle confirmed discard of changes
  async function handleDiscardConfirm() {
    showDiscardConfirm = false;
    await profileStore.selectProfile(pendingProfileId);
    onProfileChange?.(pendingProfileId);
    pendingProfileId = null;
  }

  /** Open the New profile dialog (page toolbar entry point). */
  export function openNew() {
    if ($isLoading || hasUnsavedChanges) return;
    showNewModal = true;
  }

  /** Open the Duplicate dialog for the selected profile. */
  export function openDuplicate() {
    if (!$selectedProfile || $isLoading || hasUnsavedChanges) return;
    duplicateId = '';
    duplicateName = $selectedProfile.name + ' (Copy)';
    showDuplicateModal = true;
  }

  /** Open the Delete confirmation for the selected profile. */
  export function openDelete() {
    if (!$selectedProfile || $isLoading || hasUnsavedChanges) return;
    showDeleteConfirm = true;
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
    saveChangeSummary = '';
    showSaveConfirm = true;
  }

  async function handleSaveConfirm() {
    if (await profileStore.saveProfile(saveChangeSummary)) {
      showSaveConfirm = false;
      saveChangeSummary = '';
    }
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

{#if layout === 'table'}
  <div class="profile-table">
    <div class="filters">
      <label class="check">
        <input
          type="checkbox"
          bind:checked={activeOnly}
          on:change={refresh}
          disabled={$isLoading}
        />
        Active only
      </label>
      <Button variant="ghost" icon={RefreshCw} onclick={refresh} disabled={$isLoading}>
        Refresh
      </Button>
      <span class="count text-mono">
        {#if $isLoading && rows.length === 0}
          Loading
        {:else}
          {rows.length} {rows.length === 1 ? 'profile' : 'profiles'}
        {/if}
      </span>
    </div>

    {#if $profileError}
      <p class="note note--danger" role="alert">
        <Icon icon={CircleAlert} class="note-icon" />
        <span>{$profileError}</span>
      </p>
    {/if}

    {#if rows.length === 0}
      {#if !$isLoading}
        <EmptyState
          align="start"
          message={activeOnly
            ? 'No active profiles. Clear Active only to list inactive ones.'
            : 'No source profiles yet.'}
        />
      {/if}
    {:else}
      <Table label="Source profiles" layout="fixed" class="profile-table-grid">
        {#snippet head()}
          <tr>
            <Th>Name</Th>
            <Th width="144px">Id</Th>
            <Th width="64px">Version</Th>
            <Th width="80px">Status</Th>
            {#if hasUpdated}
              <Th width="96px">Updated</Th>
            {/if}
          </tr>
        {/snippet}
        {#each rows as profile (profile.id)}
          <Tr
            selectable
            selected={profile.id === $selectedProfile?.id}
            onselect={() => requestSelect(profile.id)}
          >
            <Td truncate value={profile.name} />
            <Td mono truncate value={profile.id} />
            <Td mono truncate value={profile.version} />
            <Td>
              {#if profile.isActive}
                <Badge tone="success" dot>Active</Badge>
              {:else}
                <Badge dot>Inactive</Badge>
              {/if}
            </Td>
            {#if hasUpdated}
              <Td
                mono
                muted
                title={formatProfileTimestamp(profile.updatedAt)}
                value={formatProfileTimestamp(profile.updatedAt).slice(0, 10)}
              />
            {/if}
          </Tr>
        {/each}
      </Table>
    {/if}
  </div>
{:else}
  <div class="selector-row">
    <div class="select-wrapper">
      <Select
        aria-label="Source profile"
        bind:value={selectValue}
        onchange={handleSelect}
        disabled={$isLoading}
      >
        <option value="">Select a profile</option>
        {#each $profileList as profile (profile.id)}
          <option value={profile.id}>{profile.name} (v{profile.version})</option>
        {/each}
      </Select>
    </div>

    <label class="check">
      <input
        type="checkbox"
        bind:checked={activeOnly}
        on:change={refresh}
        disabled={$isLoading}
      />
      Active only
    </label>

    <div class="actions">
      <Button variant="ghost" icon={RefreshCw} onclick={refresh} disabled={$isLoading}>
        Refresh
      </Button>

      <Button icon={Plus} onclick={openNew} disabled={$isLoading || hasUnsavedChanges}>
        New
      </Button>

      {#if $selectedProfile}
        <Button onclick={openDuplicate} disabled={$isLoading || hasUnsavedChanges}>
          Duplicate
        </Button>

        <Button onclick={handleSave} disabled={$isLoading || $isSaving || !$isDirty}>
          {$isSaving ? 'Saving' : 'Save'}
        </Button>

        {#if $isDirty}
          <Button variant="ghost" onclick={handleDiscard} disabled={$isLoading || $isSaving}>
            Discard
          </Button>
        {/if}

        <Button variant="danger" onclick={openDelete} disabled={$isLoading || hasUnsavedChanges}>
          Delete
        </Button>
      {/if}
    </div>

    {#if $isLoading}
      <span class="status">Loading</span>
    {/if}
    {#if $isDirty}
      <Badge tone="warning">Unsaved changes</Badge>
    {/if}
  </div>
{/if}

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
      <h3 id="new-profile-modal-title" class="modal-title">New profile</h3>
      <div class="modal-body">
        <Field label="Profile name">
          <Input bind:value={newProfileName} placeholder="e.g. Epic ADT" />
        </Field>
        <Field label="Profile id" hint="Used to reference this profile in API calls.">
          <Input mono bind:value={newProfileId} placeholder="e.g. epic_adt" />
        </Field>
      </div>
      <div class="modal-actions">
        <Button size="md" onclick={() => (showNewModal = false)}>Cancel</Button>
        <Button
          variant="primary"
          size="md"
          onclick={handleCreateNew}
          disabled={!newProfileId.trim() || !newProfileName.trim()}
        >
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
      <h3 id="duplicate-profile-modal-title" class="modal-title">Duplicate profile</h3>
      <div class="modal-body">
        <Field label="New profile name">
          <Input bind:value={duplicateName} placeholder="e.g. Epic ADT v2" />
        </Field>
        <Field label="New profile id">
          <Input mono bind:value={duplicateId} placeholder="e.g. epic_adt_v2" />
        </Field>
      </div>
      <div class="modal-actions">
        <Button size="md" onclick={() => (showDuplicateModal = false)}>Cancel</Button>
        <Button
          variant="primary"
          size="md"
          onclick={handleDuplicate}
          disabled={!duplicateId.trim() || !duplicateName.trim()}
        >
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
      <h3 id="delete-profile-modal-title" class="modal-title">Delete profile</h3>
      <div class="modal-body">
        <p class="modal-text">
          Delete <strong>{$selectedProfile?.name}</strong>
          (<span class="text-mono">{$selectedProfile?.id}</span>)? This cannot be undone.
        </p>
      </div>
      <div class="modal-actions">
        <Button size="md" onclick={() => (showDeleteConfirm = false)}>Cancel</Button>
        <Button variant="danger" size="md" onclick={handleDelete}>Delete</Button>
      </div>
    </div>
  </div>
{/if}

<!-- Save Revision Modal -->
<ConfirmModal
  bind:open={showSaveConfirm}
  title="Save profile revision"
  message="Summarize this revision for reviewers and the audit trail."
  confirmText="Save"
  loading={$isSaving}
  confirmDisabled={!saveChangeSummary.trim() || saveChangeSummary.trim().length > 1024}
  closeOnConfirm={false}
  on:confirm={handleSaveConfirm}
  on:cancel={() => (saveChangeSummary = '')}
>
  <div class="change-summary">
    <Field label="Change summary">
      <Textarea
        bind:value={saveChangeSummary}
        maxlength={1024}
        rows={4}
        aria-label="Profile change summary"
        placeholder="Describe the parsing or mapping change"
      />
    </Field>
  </div>
</ConfirmModal>

<!-- Discard Changes Confirmation Modal -->
<ConfirmModal
  bind:open={showDiscardConfirm}
  title="Discard changes?"
  message="You have unsaved changes. Discard them and switch profiles?"
  confirmText="Discard"
  variant="danger"
  on:confirm={handleDiscardConfirm}
/>

<style>
  /* ── Table layout (/profiles) ─────────────────────────────────────────── */
  .profile-table {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-height: 0;
    min-width: 0;
  }

  .filters {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex: 0 0 auto;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .count {
    margin-left: auto;
    color: var(--color-text-tertiary);
  }

  .profile-table :global(.profile-table-grid) {
    flex: 1 1 auto;
    min-height: 0;
  }

  .note {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    margin: var(--space-2) var(--space-3) 0;
    padding: var(--space-2);
    border-radius: var(--radius-sm);
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-primary);
  }

  .note--danger {
    border: 1px solid var(--color-danger-border);
    background: var(--color-danger-bg);
  }

  .note--danger :global(.note-icon) {
    color: var(--color-danger-text);
  }

  /* ── Shared controls ──────────────────────────────────────────────────── */
  .check {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-ui);
    color: var(--color-text-secondary);
    white-space: nowrap;
    user-select: none;
    cursor: pointer;
  }

  .check input {
    margin: 0;
    accent-color: var(--color-primary);
  }

  /* ── Compact select layout (HL7 intake) ───────────────────────────────── */
  .selector-row {
    display: flex;
    gap: var(--space-2) var(--space-3);
    align-items: center;
    flex-wrap: wrap;
  }

  .select-wrapper {
    flex: 1 1 220px;
    min-width: 200px;
    max-width: 360px;
  }

  .actions {
    display: flex;
    gap: var(--space-1);
    flex-wrap: wrap;
  }

  .status {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  /* ── Dialogs ──────────────────────────────────────────────────────────── */
  .modal-overlay {
    position: fixed;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-4);
    z-index: var(--z-modal);
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
    width: 100%;
    max-width: var(--modal-width-sm);
    background: var(--color-bg-overlay);
    border: 1px solid var(--color-border-default);
    border-radius: var(--modal-radius);
    box-shadow: var(--shadow-xl);
    outline: none;
  }

  .modal-title {
    margin: 0;
    padding: var(--space-4) var(--space-4) 0;
    font-size: var(--text-title);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .modal-body {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4) var(--space-4);
  }

  .modal-text {
    margin: 0;
    font-size: var(--text-ui);
    line-height: var(--leading-ui);
    color: var(--color-text-secondary);
  }

  .modal-text strong {
    color: var(--color-text-primary);
    font-weight: var(--font-semibold);
  }

  .modal-actions {
    display: flex;
    gap: var(--space-2);
    justify-content: flex-end;
    padding: var(--space-3) var(--space-4);
    border-top: 1px solid var(--color-border-subtle);
  }

  .change-summary {
    margin-top: var(--space-3);
  }
</style>
