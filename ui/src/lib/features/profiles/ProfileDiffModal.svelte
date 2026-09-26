<!--
  ProfileDiffModal — the YAML line diff between the published profile and the
  local draft, shown inside the publish dialog. Compact mono list: additions
  on a success tint, removals on a danger tint, context lines muted. Long
  unchanged runs fold into one "N unchanged lines" row.
-->
<script lang="ts">
  import { toSourceProfileYAML } from '../hl7/profile/yaml';
  import type { SourceProfile } from '$lib/gen/graphql';
  import { collapseUnchanged, lineDiff } from './profileDiff';

  export let original: SourceProfile;
  export let draft: SourceProfile;

  $: originalYaml = toSourceProfileYAML(original);
  $: draftYaml = toSourceProfileYAML(draft);

  $: diff = lineDiff(originalYaml, draftYaml);
  $: rows = collapseUnchanged(diff);
  $: added = diff.filter((line) => line.type === 'added').length;
  $: removed = diff.filter((line) => line.type === 'removed').length;
</script>

<section class="diff" aria-labelledby="profile-diff-title">
  <div class="diff-head">
    <h4 id="profile-diff-title" class="diff-title">Changes against v{original.version}</h4>
    <span class="diff-counts text-mono">
      <span class="count-added">+{added}</span>
      <span class="count-removed">−{removed}</span>
    </span>
  </div>

  <div class="diff-body" role="list">
    {#each rows as line, i (i)}
      {#if line.type === 'skip'}
        <div class="line line--skip" role="listitem">
          <span class="prefix" aria-hidden="true">&nbsp;</span>
          <span class="content">{line.count} unchanged lines</span>
        </div>
      {:else}
        <div class="line line--{line.type}" role="listitem">
          <span class="prefix" aria-hidden="true">
            {#if line.type === 'added'}+{:else if line.type === 'removed'}-{:else}&nbsp;{/if}
          </span>
          <span class="sr-only">
            {#if line.type === 'added'}Added:{:else if line.type === 'removed'}Removed:{/if}
          </span>
          <span class="content">{line.text || ''}</span>
        </div>
      {/if}
    {/each}
  </div>
</section>

<style>
  .diff {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 0;
  }

  .diff-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .diff-title {
    margin: 0;
    font-size: var(--text-label);
    font-weight: var(--font-medium);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .diff-counts {
    display: inline-flex;
    gap: var(--space-2);
    margin-left: auto;
  }

  .count-added {
    color: var(--color-success-text);
  }

  .count-removed {
    color: var(--color-danger-text);
  }

  .diff-body {
    max-height: 280px;
    overflow: auto;
    padding: var(--space-1) 0;
    background: var(--color-bg-input);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    line-height: 1.5;
  }

  .line {
    display: flex;
    padding: 0 var(--space-2);
    white-space: pre-wrap;
    word-break: break-all;
  }

  .line--added {
    background: var(--color-success-bg);
    color: var(--color-success-text);
  }

  .line--removed {
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
  }

  .line--same {
    color: var(--color-text-tertiary);
  }

  .line--skip {
    color: var(--color-text-muted);
    font-family: var(--font-ui);
    font-size: var(--text-xs);
    border-block: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
  }

  .line--skip:first-child {
    border-top: 0;
  }

  .line--skip:last-child {
    border-bottom: 0;
  }

  .prefix {
    flex: 0 0 16px;
    user-select: none;
    opacity: 0.7;
  }

  .content {
    min-width: 0;
  }
</style>
