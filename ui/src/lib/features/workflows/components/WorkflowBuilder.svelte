<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';
  import RouteEditor from './RouteEditor.svelte';
  import WorkflowPreview from './WorkflowPreview.svelte';
  import DryRunPanel from './DryRunPanel.svelte';
  import GenerateFromDescription from './GenerateFromDescription.svelte';
  import { workflowDraft, isWorkflowValid } from '../workflowStore';

  let showPreview = false;
  let showDryRun = false;
  let showGenerate = false;
</script>

<div class="builder">
  <Panel>
    <div class="builder-header">
      <div class="name-version">
        <label class="field-label">
          Workflow Name
          <input
            type="text"
            class="input"
            value={$workflowDraft.name}
            placeholder="e.g. adt-routing"
            on:input={(e) =>
              workflowDraft.update((d) => ({
                ...d,
                name: (e.target as HTMLInputElement).value
              }))}
          />
        </label>
        <label class="field-label version-field">
          Version
          <input
            type="text"
            class="input"
            value={$workflowDraft.version}
            placeholder="1.0"
            on:input={(e) =>
              workflowDraft.update((d) => ({
                ...d,
                version: (e.target as HTMLInputElement).value
              }))}
          />
        </label>
      </div>
    </div>
  </Panel>

  <div class="routes">
    {#each $workflowDraft.routes as route (route._key)}
      <RouteEditor
        {route}
        on:toggleExpand={() => workflowDraft.toggleRouteExpanded(route._key)}
        on:remove={() => workflowDraft.removeRoute(route._key)}
        on:updateName={(e) =>
          workflowDraft.updateRoute(route._key, { name: e.detail })}
        on:updateFilter={(e) =>
          workflowDraft.updateRoute(route._key, { filter: e.detail })}
        on:addTransform={() => workflowDraft.addTransform(route._key)}
        on:removeTransform={(e) =>
          workflowDraft.removeTransform(route._key, e.detail.transformKey)}
        on:changeTransform={(e) =>
          workflowDraft.updateTransform(route._key, e.detail.transformKey, e.detail.transform)}
        on:moveTransform={(e) =>
          workflowDraft.moveTransform(route._key, e.detail.transformKey, e.detail.direction)}
        on:addAction={() => workflowDraft.addAction(route._key)}
        on:removeAction={(e) =>
          workflowDraft.removeAction(route._key, e.detail.actionKey)}
        on:changeAction={(e) =>
          workflowDraft.updateAction(route._key, e.detail.actionKey, e.detail.action)}
        on:moveAction={(e) =>
          workflowDraft.moveAction(route._key, e.detail.actionKey, e.detail.direction)}
        on:moveRoute={(e) => workflowDraft.moveRoute(route._key, e.detail)}
      />
    {/each}

    <Button variant="secondary" on:click={() => workflowDraft.addRoute()}>
      + Add Route
    </Button>
  </div>

  <div class="toolbar">
    <Button
      on:click={() => { showPreview = !showPreview; showDryRun = false; showGenerate = false; }}
      disabled={!$isWorkflowValid}
    >
      {showPreview ? 'Hide Preview' : 'Preview YAML'}
    </Button>
    <Button
      variant="secondary"
      on:click={() => { showDryRun = !showDryRun; showPreview = false; showGenerate = false; }}
      disabled={!$isWorkflowValid}
    >
      {showDryRun ? 'Hide Dry Run' : 'Dry Run'}
    </Button>
    <Button
      variant="secondary"
      on:click={() => { showGenerate = !showGenerate; showPreview = false; showDryRun = false; }}
    >
      {showGenerate ? 'Hide Generator' : 'Generate with AI'}
    </Button>
    <div class="spacer"></div>
    <Button variant="secondary" on:click={() => workflowDraft.reset()}>
      Reset
    </Button>
  </div>

  {#if showPreview}
    <WorkflowPreview />
  {/if}

  {#if showDryRun}
    <DryRunPanel />
  {/if}

  {#if showGenerate}
    <GenerateFromDescription />
  {/if}
</div>

<style>
  .builder {
    display: grid;
    gap: 14px;
  }

  .builder-header {
    display: grid;
    gap: 12px;
  }

  .name-version {
    display: grid;
    grid-template-columns: 1fr 120px;
    gap: 12px;
  }

  .field-label {
    display: grid;
    gap: 4px;
    color: rgba(229, 231, 235, 0.8);
    font-size: 0.85rem;
    font-weight: 600;
  }

  .input {
    padding: 8px 12px;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
    width: 100%;
    box-sizing: border-box;
  }

  .input:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .routes {
    display: grid;
    gap: 10px;
  }

  .toolbar {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    align-items: center;
  }

  .spacer {
    flex: 1;
  }
</style>
