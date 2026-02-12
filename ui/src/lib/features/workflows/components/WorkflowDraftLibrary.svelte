<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { get } from 'svelte/store';
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { workflowDraft, workflowSavedDrafts, type SavedWorkflowDraft } from '../workflowStore';
  import { yamlToDraft, draftToYaml } from '../workflowYaml';
  import { validateWorkflowDraft } from '../workflowTypes';
  import { toasts } from '$lib/ui/toastStore';

  export let pushToServerEnabled = false;
  export let promoteImportEnabled = false;

  const dispatch = createEventDispatcher<{
    pushSnapshot: { snapshotId: string };
    promoteImportYaml: { yaml: string; draftName: string };
  }>();

  let saveName = '';
  let importYaml = '';
  let importIssues: string[] = [];
  let parsedDraftName = '';
  let savedDrafts: SavedWorkflowDraft[] = [];

  $: savedDrafts = $workflowSavedDrafts;

  function saveCurrentDraft() {
    const saved = workflowSavedDrafts.saveCurrent(saveName);
    saveName = '';
    toasts.success(`Saved draft: ${saved.name}`);
  }

  function loadSnapshot(id: string) {
    const loaded = workflowSavedDrafts.loadIntoBuilder(id);
    if (!loaded) {
      toasts.error('Saved draft not found');
      return;
    }
    toasts.success(`Loaded draft: ${loaded.name}`);
  }

  function deleteSnapshot(id: string) {
    workflowSavedDrafts.deleteSnapshot(id);
  }

  function pushSnapshotToServer(id: string) {
    dispatch('pushSnapshot', { snapshotId: id });
  }

  function validateAndPromoteImportYaml() {
    const valid = validateImportYaml();
    if (!valid) return;

    try {
      const draft = yamlToDraft(importYaml);
      dispatch('promoteImportYaml', {
        yaml: importYaml.trim(),
        draftName: draft.name.trim()
      });
    } catch (err) {
      toasts.error(err instanceof Error ? err.message : 'Failed to prepare YAML promotion');
    }
  }

  function validateImportYaml(): boolean {
    importIssues = [];
    parsedDraftName = '';

    const yaml = importYaml.trim();
    if (!yaml) {
      importIssues = ['YAML input is required'];
      return false;
    }

    try {
      const draft = yamlToDraft(yaml);
      importIssues = validateWorkflowDraft(draft);
      parsedDraftName = draft.name || '(unnamed)';
      if (importIssues.length === 0) {
        toasts.success('YAML validation passed');
        return true;
      }
      toasts.error(`YAML has ${importIssues.length} validation issue${importIssues.length === 1 ? '' : 's'}`);
      return false;
    } catch (err) {
      importIssues = [err instanceof Error ? err.message : 'Invalid YAML'];
      toasts.error('Failed to parse YAML');
      return false;
    }
  }

  function loadYamlIntoBuilder() {
    const valid = validateImportYaml();
    if (!valid) return;

    try {
      const draft = yamlToDraft(importYaml);
      workflowDraft.loadDraft(draft);
      toasts.success(`Loaded YAML into builder: ${draft.name || '(unnamed)'}`);
    } catch (err) {
      toasts.error(err instanceof Error ? err.message : 'Failed to load YAML');
    }
  }

  function useCurrentAsImportSource() {
    importYaml = draftToYaml(get(workflowDraft));
  }

  function formatSavedAt(ts: string): string {
    return new Date(ts).toLocaleString();
  }
</script>

<Panel title="Draft Library" collapsible>
  <div class="library">
    <div class="save-row">
      <label class="field">
        <span class="label">Save Current Draft As</span>
        <input
          type="text"
          class="input"
          bind:value={saveName}
          placeholder={$workflowDraft.name || 'e.g. adt-routing-v1'}
        />
      </label>
      <Button size="sm" on:click={saveCurrentDraft}>
        Save Draft
      </Button>
    </div>

    <div class="saved">
      <div class="section-title">Saved Drafts</div>
      {#if savedDrafts.length === 0}
        <div class="empty">No saved drafts yet.</div>
      {:else}
        <div class="saved-list">
          {#each savedDrafts as item (item.id)}
            <div class="saved-item">
              <div class="saved-main">
                <div class="saved-name">{item.name}</div>
                <div class="saved-meta">{formatSavedAt(item.savedAt)}</div>
              </div>
              <div class="saved-actions">
                <Button variant="secondary" size="sm" on:click={() => loadSnapshot(item.id)}>
                  Load
                </Button>
                {#if pushToServerEnabled}
                  <Button variant="secondary" size="sm" on:click={() => pushSnapshotToServer(item.id)}>
                    Push to Server
                  </Button>
                {/if}
                <Button variant="danger" size="sm" on:click={() => deleteSnapshot(item.id)}>
                  Delete
                </Button>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>

    <div class="import">
      <div class="section-title">Import Workflow YAML</div>
      <textarea
        class="yaml-input mono"
        rows="10"
        bind:value={importYaml}
        placeholder="name: adt-routing
version: &quot;1.0&quot;
routes:
  - name: admits
    filter:
      event_type: PATIENT_ADMIT
    actions:
      - type: log
        level: info
        message: Admit received"
      ></textarea>
      <div class="import-actions">
        <Button variant="secondary" size="sm" on:click={useCurrentAsImportSource}>
          Load Current Draft YAML
        </Button>
        <Button variant="secondary" size="sm" on:click={validateImportYaml}>
          Validate YAML
        </Button>
        <Button size="sm" on:click={loadYamlIntoBuilder}>
          Load into Builder
        </Button>
        {#if promoteImportEnabled}
          <Button variant="secondary" size="sm" on:click={validateAndPromoteImportYaml}>
            Validate + Push to Server
          </Button>
        {/if}
      </div>
      {#if parsedDraftName}
        <div class="parsed-name">Parsed workflow: <span class="mono">{parsedDraftName}</span></div>
      {/if}
      {#if importIssues.length > 0}
        <div class="issues" role="alert">
          {#each importIssues as issue, idx (idx)}
            <div class="issue">{issue}</div>
          {/each}
        </div>
      {/if}
    </div>
  </div>
</Panel>

<style>
  .library {
    display: grid;
    gap: 14px;
  }

  .save-row {
    display: flex;
    align-items: flex-end;
    gap: 10px;
    flex-wrap: wrap;
  }

  .field {
    display: grid;
    gap: 4px;
    flex: 1;
    min-width: 240px;
  }

  .label {
    color: var(--color-text-tertiary);
    font-size: 0.8rem;
    font-weight: 700;
  }

  .input {
    padding: 8px 12px;
    border-radius: 10px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    outline: none;
    width: 100%;
    box-sizing: border-box;
    transition: var(--transition-all);
  }

  .input:hover:not(:disabled):not(:focus) {
    border-color: var(--color-border-strong);
  }

  .input:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .section-title {
    color: var(--color-text-tertiary);
    font-size: 0.8rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin-bottom: 6px;
  }

  .saved-list {
    display: grid;
    gap: 6px;
  }

  .saved-item {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 10px;
    border: 1px solid var(--color-border-subtle);
    border-radius: 10px;
    background: var(--color-bg-surface);
    padding: 8px 10px;
  }

  .saved-main {
    min-width: 0;
  }

  .saved-name {
    color: var(--color-text-primary);
    font-weight: 650;
  }

  .saved-meta {
    color: var(--color-text-muted);
    font-size: 0.8rem;
  }

  .saved-actions {
    display: flex;
    gap: 8px;
    align-items: center;
    flex-wrap: wrap;
  }

  .empty {
    color: var(--color-text-muted);
    font-size: 0.9rem;
    padding: 6px 0;
  }

  .yaml-input {
    width: 100%;
    padding: 10px 12px;
    border-radius: 10px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    outline: none;
    resize: vertical;
    box-sizing: border-box;
    transition: var(--transition-all);
  }

  .yaml-input:hover:not(:disabled):not(:focus) {
    border-color: var(--color-border-strong);
  }

  .yaml-input:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .import-actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    margin-top: 8px;
  }

  .issues {
    margin-top: 8px;
    display: grid;
    gap: 4px;
    border: 1px solid var(--color-danger-border);
    background: var(--color-danger-bg);
    border-radius: 10px;
    padding: 8px 10px;
  }

  .issue {
    color: var(--color-danger-text);
    font-size: 0.85rem;
  }

  .parsed-name {
    margin-top: 8px;
    color: var(--color-text-secondary);
    font-size: 0.85rem;
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  }
</style>
