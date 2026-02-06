<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';
  import { workflowDraft } from '../workflowStore';
  import { draftToYaml } from '../workflowYaml';
  import { explainWorkflow } from '../workflowApi';
  import { toasts } from '$lib/ui/toastStore';

  let yamlOutput = '';
  let explanation = '';
  let explaining = false;
  let copied = false;

  $: yamlOutput = draftToYaml($workflowDraft);

  async function handleExplain() {
    explaining = true;
    explanation = '';
    try {
      const result = await explainWorkflow(yamlOutput, 'business');
      explanation = result.explainWorkflow.description;
      if (result.explainWorkflow.routeExplanations.length > 0) {
        explanation +=
          '\n\nRoutes:\n' +
          result.explainWorkflow.routeExplanations
            .map((r) => `- ${r.name}: ${r.description}`)
            .join('\n');
      }
    } catch {
      toasts.error('Failed to explain workflow');
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

  {#if explanation}
    <div class="explanation">
      <h4 class="explanation-title">AI Explanation</h4>
      <div class="explanation-text">{explanation}</div>
    </div>
  {/if}
</Panel>

<style>
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

  .explanation {
    margin-top: 12px;
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
</style>
