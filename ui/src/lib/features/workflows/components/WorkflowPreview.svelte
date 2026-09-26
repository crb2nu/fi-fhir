<script lang="ts">
  import Check from '@lucide/svelte/icons/check';
  import Copy from '@lucide/svelte/icons/copy';
  import Download from '@lucide/svelte/icons/download';
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
  import { Button, Icon, Panel } from '$lib/ui/primitives';
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

<Panel title="YAML preview" flush>
  {#snippet actions()}
    <Button variant="ghost" icon={copied ? Check : Copy} onclick={handleCopy}>
      {copied ? 'Copied' : 'Copy'}
    </Button>
    <Button variant="ghost" icon={Download} onclick={handleDownload}>Download</Button>
    <Button variant="ghost" onclick={handleExplain} disabled={explaining}>
      {explaining ? 'Explaining...' : 'Explain with AI'}
    </Button>
  {/snippet}

  <pre class="yaml-output">{yamlOutput}</pre>

  {#if explanation || explainSummary}
    <section class="explanation" aria-labelledby="workflow-explanation-title">
      <h4 id="workflow-explanation-title" class="explanation-title">AI explanation</h4>
      {#if explainSummary}
        <p class="explanation-summary">{explainSummary}</p>
      {/if}
      <div class="explanation-text">{explanation}</div>
      {#if explainWarnings.length > 0}
        <div class="explanation-warnings" role="alert">
          <span class="warnings-label">Warnings</span>
          <ul class="warnings-list">
            {#each explainWarnings as warning (warning)}
              <li><Icon icon={TriangleAlert} /><span>{warning}</span></li>
            {/each}
          </ul>
        </div>
      {/if}
      {#if explainDiagram.trim()}
        <div class="explanation-diagram">
          <MermaidDiagram source={explainDiagram} />
        </div>
      {/if}
    </section>
  {/if}
</Panel>

<style>
  .yaml-output {
    max-height: 360px;
    margin: 0;
    padding: var(--space-3);
    overflow: auto;
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    line-height: var(--leading-snug);
    white-space: pre;
  }

  .explanation {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    border-top: 1px solid var(--color-border-subtle);
  }

  .explanation-title,
  .warnings-label {
    margin: 0;
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .explanation-summary {
    margin: 0;
    font-size: var(--text-ui);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .explanation-text {
    font-size: var(--text-ui);
    line-height: var(--leading-ui);
    color: var(--color-text-secondary);
    white-space: pre-wrap;
  }

  .explanation-warnings {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--color-warning-border);
    border-radius: var(--radius-sm);
    background: var(--color-warning-bg);
  }

  .warnings-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: var(--text-xs);
    color: var(--color-text-primary);
  }

  .warnings-list li {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
  }

  .warnings-list :global(.ui-icon) {
    color: var(--color-warning-text);
  }

  .explanation-diagram {
    padding-top: var(--space-2);
  }
</style>
