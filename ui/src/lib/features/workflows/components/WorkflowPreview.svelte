<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';
  import MermaidDiagram from '$lib/ui/MermaidDiagram.svelte';
  import { workflowDraft } from '../workflowStore';
  import { draftToYaml } from '../workflowYaml';
  import { explainWorkflow } from '../workflowApi';
  import { toasts } from '$lib/ui/toastStore';
  import { isErrorToasted } from '$lib/graphql/client';

  let yamlOutput = '';
  let explanation = '';
  let explainSummary = '';
  let explainWarnings: string[] = [];
  let explainDiagram = '';
  let explaining = false;
  let copied = false;

  $: yamlOutput = draftToYaml($workflowDraft);

  async function handleExplain() {
    explaining = true;
    explanation = '';
    explainSummary = '';
    explainWarnings = [];
    explainDiagram = '';
    try {
      const result = await explainWorkflow(yamlOutput, 'business');
      explainSummary = result.explainWorkflow.summary;
      explainWarnings = result.explainWorkflow.warnings;
      explainDiagram = result.explainWorkflow.diagram ?? '';
      explanation = result.explainWorkflow.description;
      if (result.explainWorkflow.routeExplanations.length > 0) {
        explanation +=
          '\n\nRoutes:\n' +
          result.explainWorkflow.routeExplanations
            .map((r) => `- ${r.name}: ${r.description}`)
            .join('\n');
      }
    } catch (e) {
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(e)) {
        toasts.error('Failed to explain workflow');
      }
    } finally {
      explaining = false;
    }
  }

  function handleCopy() {
    navigator.clipboard.writeText(yamlOutput);
    copied = true;
    setTimeout(() => (copied = false), 2000);
  }

  function handleDownload() {
    const blob = new Blob([yamlOutput], { type: 'text/yaml' });
    const url = URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `${$workflowDraft.name || 'workflow'}.yaml`;
    a.click();
    URL.revokeObjectURL(url);
  }
</script>

<Panel title="YAML Preview">
  <svelte:fragment slot="actions">
    <Button variant="secondary" on:click={handleCopy}>
      {copied ? 'Copied!' : 'Copy'}
    </Button>
    <Button variant="secondary" on:click={handleDownload}>
      Download
    </Button>
    <Button variant="secondary" on:click={handleExplain} disabled={explaining}>
      {explaining ? 'Explaining...' : 'Explain with AI'}
    </Button>
  </svelte:fragment>

  <pre class="yaml-output">{yamlOutput}</pre>

  {#if explanation || explainSummary}
    <div class="explanation">
      <h4 class="explanation-title">AI Explanation</h4>
      {#if explainSummary}
        <p class="explanation-summary">{explainSummary}</p>
      {/if}
      <div class="explanation-text">{explanation}</div>
      {#if explainWarnings.length > 0}
        <div class="explanation-warnings" role="alert">
          <span class="warnings-label">Warnings</span>
          <ul class="warnings-list">
            {#each explainWarnings as warning (warning)}
              <li>{warning}</li>
            {/each}
          </ul>
        </div>
      {/if}
      {#if explainDiagram.trim()}
        <div class="explanation-diagram">
          <MermaidDiagram source={explainDiagram} />
        </div>
      {/if}
    </div>
  {/if}
</Panel>

<style>
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

  .explanation {
    margin-top: 12px;
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

  .explanation-summary {
    color: var(--color-text-primary);
    font-size: 0.9rem;
    font-weight: 600;
    line-height: 1.4;
    margin: 0 0 8px;
  }

  .explanation-text {
    color: var(--color-text-secondary);
    font-size: 0.9rem;
    line-height: 1.55;
    white-space: pre-wrap;
  }

  .explanation-warnings {
    margin-top: 12px;
    padding: 8px 12px;
    border-radius: 6px;
    border: 1px solid var(--color-warning-border);
    background: var(--color-warning-bg);
  }

  .warnings-label {
    display: block;
    color: var(--color-warning);
    font-size: 0.75rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    margin-bottom: 4px;
  }

  .warnings-list {
    margin: 0;
    padding-left: 18px;
    color: var(--color-text-secondary);
    font-size: 0.85rem;
    line-height: 1.5;
  }

  .explanation-diagram {
    margin-top: 12px;
  }
</style>
