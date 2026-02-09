<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { workflowDraft } from '../workflowStore';
  import { yamlToDraft } from '../workflowYaml';
  import { generateWorkflow } from '../workflowApi';
  import { ALL_EVENT_TYPES, ACTION_TYPES } from '../workflowTypes';
  import { toasts } from '$lib/ui/toastStore';

  let description = '';
  let generating = false;
  let generatedYaml = '';
  let generatedExplanation = '';
  let generatedWarnings: string[] = [];

  async function handleGenerate() {
    if (!description.trim()) return;
    generating = true;
    generatedYaml = '';
    generatedExplanation = '';
    generatedWarnings = [];

    try {
      const result = await generateWorkflow(description, ALL_EVENT_TYPES, ACTION_TYPES);
      generatedYaml = result.generateWorkflow.yaml;
      generatedExplanation = result.generateWorkflow.explanation;
      generatedWarnings = result.generateWorkflow.warnings;
    } catch {
      toasts.error('Failed to generate workflow');
    } finally {
      generating = false;
    }
  }

  function loadIntoBuilder() {
    try {
      const draft = yamlToDraft(generatedYaml);
      workflowDraft.loadDraft(draft);
      toasts.success('Workflow loaded into builder');
    } catch {
      toasts.error('Failed to parse generated YAML');
    }
  }
</script>

<Panel title="Generate from Description">
  <div class="generator">
    <div class="input-section">
      <textarea
        class="textarea"
        bind:value={description}
        aria-label="Workflow description"
        placeholder="Describe what you want the workflow to do, e.g.&#10;&#10;Route all patient admit and discharge events to a FHIR server, and log critical lab results to a webhook."
        rows="4"
      ></textarea>
      <Button on:click={handleGenerate} loading={generating} disabled={!description.trim()}>
        {generating ? 'Generating...' : 'Generate Workflow'}
      </Button>
    </div>

    {#if generatedYaml}
      <div class="result">
        {#if generatedExplanation}
          <div class="explanation">
            <h4 class="explanation-title">Explanation</h4>
            <div class="explanation-text">{generatedExplanation}</div>
          </div>
        {/if}

        {#if generatedWarnings.length > 0}
          <div class="warnings" role="alert">
            {#each generatedWarnings as warn (warn)}
              <div class="warning-item">{warn}</div>
            {/each}
          </div>
        {/if}

        <div class="yaml-section">
          <div class="yaml-header">
            <h4 class="yaml-title">Generated YAML</h4>
            <Button variant="secondary" on:click={loadIntoBuilder}>
              Load into Builder
            </Button>
          </div>
          <pre class="yaml-output">{generatedYaml}</pre>
        </div>
      </div>
    {/if}
  </div>
</Panel>

<style>
  .generator {
    display: grid;
    gap: 14px;
  }

  .input-section {
    display: grid;
    gap: 10px;
  }

  .textarea {
    padding: 10px 12px;
    border-radius: 10px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    outline: none;
    resize: vertical;
    width: 100%;
    box-sizing: border-box;
    line-height: 1.5;
    transition: var(--transition-all);
  }

  .textarea::placeholder {
    color: var(--color-text-muted);
  }

  .textarea:hover:not(:disabled):not(:focus) {
    border-color: var(--color-border-strong);
  }

  .textarea:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .result {
    display: grid;
    gap: 12px;
  }

  .explanation {
    padding: 12px 16px;
    border-radius: 8px;
    border: 1px solid var(--color-primary-border);
    background: var(--color-primary-muted);
  }

  .explanation-title {
    color: var(--color-primary);
    font-size: 0.85rem;
    font-weight: 700;
    margin: 0 0 8px;
  }

  .explanation-text {
    color: var(--color-text-secondary);
    font-size: 0.9rem;
    line-height: 1.55;
    white-space: pre-wrap;
  }

  .warnings {
    display: grid;
    gap: 4px;
  }

  .warning-item {
    padding: 6px 10px;
    border-radius: 6px;
    background: var(--color-warning-bg);
    border: 1px solid var(--color-warning-border);
    color: var(--color-text-primary);
    font-size: 0.85rem;
  }

  .yaml-section {
    display: grid;
    gap: 8px;
  }

  .yaml-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .yaml-title {
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
    font-weight: 700;
    margin: 0;
  }

  .yaml-output {
    padding: 12px 16px;
    border-radius: 8px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-surface);
    color: var(--color-text-primary);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 0.85rem;
    line-height: 1.5;
    overflow-x: auto;
    white-space: pre;
    margin: 0;
  }
</style>
