<script lang="ts">
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
  import { Button, Icon, Panel, Textarea } from '$lib/ui/primitives';
  import { workflowDraft } from '../workflowStore';
  import { yamlToDraft } from '../workflowYaml';
  import { generateWorkflow } from '../workflowApi';
  import { ALL_EVENT_TYPES, ACTION_TYPES } from '../workflowTypes';
  import { toasts } from '$lib/ui/toastStore';
  import { isErrorToasted } from '$lib/graphql/client';

  const DESCRIPTION_PLACEHOLDER =
    'Describe what the workflow should do, e.g.\n\nRoute patient admit and discharge events to a FHIR server, and send critical lab results to a webhook.';

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
    } catch (e) {
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(e)) {
        toasts.error('Failed to generate workflow');
      }
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

<Panel title="Generate from description">
  <div class="generator">
    <Textarea
      bind:value={description}
      aria-label="Workflow description"
      placeholder={DESCRIPTION_PLACEHOLDER}
      rows={4}
    />
    <div class="button-row">
      <Button onclick={handleGenerate} loading={generating} disabled={!description.trim()}>
        {generating ? 'Generating...' : 'Generate workflow'}
      </Button>
    </div>

    {#if generatedYaml}
      {#if generatedExplanation}
        <section class="block" aria-labelledby="generated-explanation-title">
          <h4 id="generated-explanation-title" class="block-title">Explanation</h4>
          <div class="explanation-text">{generatedExplanation}</div>
        </section>
      {/if}

      {#if generatedWarnings.length > 0}
        <ul class="warnings" role="alert">
          {#each generatedWarnings as warn (warn)}
            <li><Icon icon={TriangleAlert} /><span>{warn}</span></li>
          {/each}
        </ul>
      {/if}

      <section class="block" aria-labelledby="generated-yaml-title">
        <div class="block-head">
          <h4 id="generated-yaml-title" class="block-title">Generated YAML</h4>
          <Button variant="ghost" onclick={loadIntoBuilder}>Load into builder</Button>
        </div>
        <pre class="yaml-output">{generatedYaml}</pre>
      </section>
    {/if}
  </div>
</Panel>

<style>
  .generator {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .button-row {
    display: flex;
    gap: var(--space-2);
  }

  .block {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .block-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
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

  .explanation-text {
    font-size: var(--text-ui);
    line-height: var(--leading-ui);
    color: var(--color-text-secondary);
    white-space: pre-wrap;
  }

  .warnings {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--color-warning-border);
    border-radius: var(--radius-sm);
    background: var(--color-warning-bg);
    list-style: none;
    font-size: var(--text-xs);
    color: var(--color-text-primary);
  }

  .warnings li {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
  }

  .warnings :global(.ui-icon) {
    color: var(--color-warning-text);
  }

  .yaml-output {
    max-height: 320px;
    margin: 0;
    padding: var(--space-3);
    overflow: auto;
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    line-height: var(--leading-snug);
    white-space: pre;
  }
</style>
