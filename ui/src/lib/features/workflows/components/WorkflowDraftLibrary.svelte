<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { get } from 'svelte/store';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Save from '@lucide/svelte/icons/save';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import {
    Badge,
    Button,
    Field,
    Icon,
    IconButton,
    Input,
    Panel,
    Table,
    Td,
    Th,
    Tr
  } from '$lib/ui/primitives';
  import CodeEditor from '$lib/ui/editor/CodeEditor.svelte';
  import { workflowDraft, workflowSavedDrafts, type SavedWorkflowDraft } from '../workflowStore';
  import { yamlToDraft, draftToYaml } from '../workflowYaml';
  import { evaluateImportYaml } from '../importYamlValidation';
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
    // Validation issues (parse errors, structural problems) are persistent state
    // until the user fixes the YAML, so they live inline in the `.issues` list
    // below — not in a transient toast that would duplicate them (.loom/22 B1/B4).
    const { issues, parsedName } = evaluateImportYaml(importYaml);
    importIssues = issues;
    parsedDraftName = parsedName;

    if (issues.length === 0) {
      toasts.success('YAML validation passed');
      return true;
    }
    return false;
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

  let open = true;

  function formatSavedAt(ts: string): string {
    const date = new Date(ts);
    if (Number.isNaN(date.getTime())) return ts;
    const pad = (n: number) => String(n).padStart(2, '0');
    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(
      date.getHours()
    )}:${pad(date.getMinutes())}`;
  }
</script>

<Panel aria-label="Draft library" flush class={open ? 'draft-library' : 'draft-library is-collapsed'}>
  {#snippet header()}
    <button
      type="button"
      class="collapse-toggle"
      aria-expanded={open}
      aria-controls="draft-library-body"
      on:click={() => (open = !open)}
    >
      <Icon icon={open ? ChevronDown : ChevronRight} size={14} />
      <span class="panel-title">Draft library</span>
    </button>
  {/snippet}
  {#snippet actions()}
    <Badge mono title="Saved drafts">{savedDrafts.length}</Badge>
  {/snippet}

  {#if open}
    <div id="draft-library-body" class="library">
      <div class="column">
        <Field label="Save current draft as">
          <div class="inline-control">
            <Input bind:value={saveName} placeholder={$workflowDraft.name || 'e.g. adt-routing-v1'} />
            <Button icon={Save} onclick={saveCurrentDraft}>Save draft</Button>
          </div>
        </Field>

        <section class="block" aria-labelledby="saved-drafts-title">
          <h4 id="saved-drafts-title" class="block-title">Saved drafts</h4>
          {#if savedDrafts.length === 0}
            <p class="note">No saved drafts.</p>
          {:else}
            <div class="table-frame">
              <Table label="Saved drafts" layout="fixed" class="drafts-table">
                {#snippet head()}
                  <tr>
                    <Th>Name</Th>
                    <Th width="128px">Saved</Th>
                    <Th width={pushToServerEnabled ? '196px' : '96px'}
                      ><span class="sr-only">Actions</span></Th
                    >
                  </tr>
                {/snippet}
                {#each savedDrafts as item (item.id)}
                  <Tr>
                    <Td mono truncate value={item.name} />
                    <Td mono muted value={formatSavedAt(item.savedAt)} />
                    <Td>
                      <span class="row-actions">
                        <Button variant="ghost" onclick={() => loadSnapshot(item.id)}>Load</Button>
                        {#if pushToServerEnabled}
                          <Button variant="ghost" onclick={() => pushSnapshotToServer(item.id)}>
                            Push to server
                          </Button>
                        {/if}
                        <IconButton
                          icon={Trash2}
                          label={`Delete draft ${item.name}`}
                          onclick={() => deleteSnapshot(item.id)}
                        />
                      </span>
                    </Td>
                  </Tr>
                {/each}
              </Table>
            </div>
          {/if}
        </section>
      </div>

      <section class="column" aria-labelledby="import-yaml-title">
        <h4 id="import-yaml-title" class="block-title">Import workflow YAML</h4>
        <div class="code-frame">
          <CodeEditor
            language="yaml"
            value={importYaml}
            on:change={(e) => {
              importYaml = e.detail;
            }}
            placeholder="name: adt-routing"
            height="200px"
          />
        </div>
        <div class="button-row">
          <Button onclick={loadYamlIntoBuilder}>Load into builder</Button>
          <Button variant="ghost" onclick={validateImportYaml}>Validate YAML</Button>
          <Button variant="ghost" onclick={useCurrentAsImportSource}>Use current draft</Button>
          {#if promoteImportEnabled}
            <Button variant="ghost" onclick={validateAndPromoteImportYaml}>
              Validate and push to server
            </Button>
          {/if}
        </div>
        {#if parsedDraftName}
          <p class="note">Parsed workflow <span class="text-mono">{parsedDraftName}</span></p>
        {/if}
        {#if importIssues.length > 0}
          <ul class="issues" role="alert">
            {#each importIssues as issue, idx (idx)}
              <li><Icon icon={CircleAlert} /><span>{issue}</span></li>
            {/each}
          </ul>
        {/if}
      </section>
    </div>
  {/if}
</Panel>

<style>
  :global(.draft-library.is-collapsed) :global(.ui-panel-header) {
    border-bottom: 0;
  }

  .collapse-toggle {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    height: 100%;
    margin-left: calc(-1 * var(--space-1));
    padding: 0 var(--space-1);
    background: none;
    border: 0;
    color: var(--color-text-tertiary);
    font: inherit;
    cursor: pointer;
  }

  .collapse-toggle:hover {
    color: var(--color-text-primary);
  }

  .collapse-toggle:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
    border-radius: var(--radius-sm);
  }

  .panel-title {
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
  }

  .library {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-4);
    padding: var(--panel-padding);
  }

  .column {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
  }

  .inline-control {
    display: flex;
    gap: var(--space-2);
    min-width: 0;
  }

  .block {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .block-title {
    margin: 0;
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .table-frame,
  .code-frame {
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .library :global(.drafts-table) {
    max-height: 240px;
  }

  .row-actions {
    display: inline-flex;
    align-items: center;
    gap: 2px;
  }

  .button-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .note {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .issues {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--color-danger-border);
    border-radius: var(--radius-sm);
    background: var(--color-danger-bg);
    list-style: none;
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }

  .issues li {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
  }

  @media (max-width: 960px) {
    .library {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
