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
        placeholder="Describe what you want the workflow to do, e.g.&#10;&#10;Route all patient admit and discharge events to a FHIR server, and log critical lab results to a webhook."
        rows="4"
      ></textarea>
      <Button on:click={handleGenerate} disabled={generating || !description.trim()}>
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
          <div class="warnings">
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
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
    resize: vertical;
    width: 100%;
    box-sizing: border-box;
    line-height: 1.5;
  }

  .textarea:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .result {
    display: grid;
    gap: 12px;
  }

  .explanation {
    padding: 12px 16px;
    border-radius: 8px;
    border: 1px solid rgba(59, 130, 246, 0.2);
    background: rgba(59, 130, 246, 0.05);
  }

  .explanation-title {
    color: rgba(147, 197, 253, 0.9);
    font-size: 0.85rem;
    font-weight: 700;
    margin: 0 0 8px;
  }

  .explanation-text {
    color: rgba(229, 231, 235, 0.85);
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
    background: rgba(245, 158, 11, 0.1);
    border: 1px solid rgba(245, 158, 11, 0.25);
    color: rgba(253, 230, 138, 0.9);
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
    color: rgba(229, 231, 235, 0.8);
    font-size: 0.85rem;
    font-weight: 700;
    margin: 0;
  }

  .yaml-output {
    padding: 12px 16px;
    border-radius: 8px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(0, 0, 0, 0.3);
    color: rgba(229, 231, 235, 0.92);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 0.85rem;
    line-height: 1.5;
    overflow-x: auto;
    white-space: pre;
    margin: 0;
  }
</style>
