<script lang="ts">
  import { toSourceProfileYAML } from '../hl7/profile/yaml';
  import type { SourceProfile } from '$lib/gen/graphql';
  import { lineDiff } from './profileDiff';

  export let original: SourceProfile;
  export let draft: SourceProfile;

  $: originalYaml = toSourceProfileYAML(original);
  $: draftYaml = toSourceProfileYAML(draft);

  $: diff = lineDiff(originalYaml, draftYaml);
</script>

<div class="diff-container">
  <div class="diff-header">
    <h3>Review Profile Changes</h3>
    <p>Comparing live version (v{original.version}) with your local draft.</p>
  </div>

  <div class="diff-body mono">
    {#each diff as line, i (i)}
      <div class="line {line.type}">
        <span class="prefix">
          {#if line.type === 'added'}+{:else if line.type === 'removed'}-{:else}&nbsp;{/if}
        </span>
        <span class="content">{line.text || ''}</span>
      </div>
    {/each}
  </div>

</div>

<style>
  .diff-container {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    max-height: 80vh;
  }

  .diff-header h3 {
    margin: 0 0 4px;
    color: var(--color-text-primary);
  }

  .diff-header p {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--color-text-secondary);
  }

  .diff-body {
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-lg);
    padding: var(--space-2) 0;
    overflow-y: auto;
    font-size: var(--text-xs);
    line-height: 1.4;
  }

  .line {
    display: flex;
    padding: 0 var(--space-3);
    white-space: pre-wrap;
    word-break: break-all;
  }

  .line.added {
    background: rgba(16, 185, 129, 0.1);
    color: var(--palette-emerald-400);
  }

  .line.removed {
    background: rgba(239, 68, 68, 0.1);
    color: var(--color-danger-soft);
    text-decoration: line-through;
  }

  .line.same {
    color: var(--color-text-tertiary);
  }

  .prefix {
    width: 20px;
    user-select: none;
    opacity: 0.5;
  }

  .mono { font-family: var(--font-mono); }
</style>
