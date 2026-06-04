<script lang="ts">
  import { resolve } from '$app/paths';
  import Badge from '$lib/ui/Badge.svelte';
  import type { FlowStep } from './authoringFlow';

  export let eyebrow = 'Authoring flow';
  export let title: string;
  export let summary = '';
  export let steps: readonly FlowStep[] = [];
  export let compact = false;
</script>

{#if compact}
  <nav class="flow-compact" aria-label={title}>
    {#each steps as step, index (step.title + step.eyebrow)}
      {#if index > 0}
        <span class="connector" aria-hidden="true">
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none">
            <path d="M6 3l5 5-5 5" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/>
          </svg>
        </span>
      {/if}
      <div class="chip" class:first={index === 0}>
        <span class="chip-index">0{index + 1}</span>
        <div class="chip-body">
          <span class="chip-label">{step.eyebrow}</span>
          {#if step.actions?.length}
            <div class="chip-actions">
              {#each step.actions as action (action.label + (action.href ?? 'button'))}
                {#if action.href}
                  <a class="chip-link" href={resolve(action.href)} aria-label={action.ariaLabel}>{action.label}</a>
                {:else}
                  <button class="chip-link" type="button" aria-label={action.ariaLabel} on:click={() => action.onClick?.()}>{action.label}</button>
                {/if}
                {#if action !== step.actions[step.actions.length - 1]}
                  <span class="chip-sep" aria-hidden="true">&middot;</span>
                {/if}
              {/each}
            </div>
          {/if}
        </div>
        {#if step.metric && step.metric !== step.eyebrow}
          <span class="chip-badge">{step.metric}</span>
        {/if}
      </div>
    {/each}
  </nav>
{:else}
  <section class="flow-rail">
    <div class="header">
      <p class="eyebrow">{eyebrow}</p>
      <h2>{title}</h2>
      {#if summary}
        <p class="summary">{summary}</p>
      {/if}
    </div>

    <div class="steps" aria-label={title}>
      {#each steps as step, index (step.title + step.eyebrow)}
        <article class="step">
          <div class="step-top">
            <span class="index">0{index + 1}</span>
            <div class="badges">
              {#if step.metric}
                <Badge variant="default" pill>{step.metric}</Badge>
              {/if}
              {#if step.status}
                <Badge variant="primary" pill>{step.status}</Badge>
              {/if}
            </div>
          </div>

          <p class="step-eyebrow">{step.eyebrow}</p>
          <h3>{step.title}</h3>
          <p class="description">{step.description}</p>

          {#if step.actions?.length}
            <div class="actions">
              {#each step.actions as action (action.label + (action.href ?? 'button'))}
                {#if action.href}
                  <a
                    class="action {action.variant ?? 'secondary'}"
                    href={resolve(action.href)}
                    aria-label={action.ariaLabel}
                  >
                    {action.label}
                  </a>
                {:else}
                  <button
                    class="action {action.variant ?? 'secondary'}"
                    type="button"
                    aria-label={action.ariaLabel}
                    on:click={() => action.onClick?.()}
                  >
                    {action.label}
                  </button>
                {/if}
              {/each}
            </div>
          {/if}
        </article>
      {/each}
    </div>
  </section>
{/if}

<style>
  .flow-rail {
    display: grid;
    gap: 16px;
    padding: 18px;
    border-radius: 24px;
    border: 1px solid rgba(56, 189, 248, 0.18);
    background:
      radial-gradient(circle at top right, rgba(56, 189, 248, 0.12), transparent 42%),
      linear-gradient(180deg, rgba(15, 23, 42, 0.06), transparent 100%),
      var(--color-bg-elevated);
    box-shadow: var(--shadow-md);
  }

  .header {
    display: grid;
    gap: 8px;
    max-width: 76ch;
  }

  .eyebrow {
    margin: 0;
    color: var(--color-primary);
    font-size: 0.8rem;
    font-weight: 900;
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }

  h2 {
    margin: 0;
    color: var(--color-text-primary);
    font-size: clamp(1.35rem, 1.1rem + 1vw, 1.9rem);
    line-height: 1.15;
  }

  .summary {
    margin: 0;
    color: var(--color-text-secondary);
    line-height: 1.6;
    max-width: 70ch;
  }

  .steps {
    display: grid;
    gap: 12px;
    grid-template-columns: 1fr;
  }

  @media (min-width: 900px) {
    .steps {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }

  .step {
    display: grid;
    gap: 10px;
    padding: 14px;
    border-radius: 20px;
    border: 1px solid var(--color-border-default);
    background:
      linear-gradient(180deg, rgba(255, 255, 255, 0.04), transparent),
      var(--color-bg-surface);
  }

  .step-top {
    display: flex;
    align-items: start;
    justify-content: space-between;
    gap: 10px;
  }

  .index {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    border-radius: 999px;
    border: 1px solid rgba(56, 189, 248, 0.28);
    background: rgba(56, 189, 248, 0.12);
    color: var(--color-primary);
    font-size: 0.82rem;
    font-weight: 900;
    letter-spacing: 0.08em;
    font-variant-numeric: tabular-nums;
  }

  .badges {
    display: flex;
    flex-wrap: wrap;
    justify-content: flex-end;
    gap: 6px;
  }

  .step-eyebrow {
    margin: 0;
    color: var(--color-text-muted);
    font-size: 0.8rem;
    font-weight: 800;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }

  h3 {
    margin: 0;
    color: var(--color-text-primary);
    font-size: 1.02rem;
    line-height: 1.25;
  }

  .description {
    margin: 0;
    color: var(--color-text-secondary);
    line-height: 1.55;
  }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    margin-top: auto;
    padding-top: 4px;
  }

  .action {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-height: 34px;
    padding: 0 12px;
    border-radius: 999px;
    border: 1px solid transparent;
    text-decoration: none;
    color: inherit;
    appearance: none;
    font-size: 0.85rem;
    font-weight: 800;
    transition: var(--transition-all);
    cursor: pointer;
    font: inherit;
  }

  .action.primary {
    color: var(--color-text-inverse);
    background: var(--color-primary);
    border-color: var(--color-primary);
  }

  .action.secondary {
    color: var(--color-text-secondary);
    background: var(--color-bg-elevated);
    border-color: var(--color-border-default);
  }

  .action.ghost {
    color: var(--color-primary);
    background: transparent;
    border-color: transparent;
    padding-inline: 0;
  }

  .action:disabled {
    opacity: 0.55;
    cursor: not-allowed;
    transform: none;
    box-shadow: none;
  }

  .action:hover {
    transform: translateY(-1px);
    box-shadow: var(--shadow-sm);
  }

  .action:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .action.primary:hover {
    box-shadow: var(--shadow-md);
  }

  /* ── Compact mode ─────────────────────────── */

  .flow-compact {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 6px;
    padding: 8px 14px;
    border-radius: 999px;
    border: 1px solid rgba(56, 189, 248, 0.18);
    background:
      linear-gradient(90deg, rgba(56, 189, 248, 0.06), transparent 60%),
      var(--color-bg-elevated);
  }

  .connector {
    display: inline-flex;
    align-items: center;
    color: var(--color-text-muted);
    flex-shrink: 0;
  }

  .chip {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 4px 10px;
    border-radius: 999px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-surface);
    min-height: 32px;
  }

  .chip.first {
    border-color: rgba(56, 189, 248, 0.28);
    background: rgba(56, 189, 248, 0.08);
  }

  .chip-index {
    font-size: 0.72rem;
    font-weight: 900;
    letter-spacing: 0.08em;
    color: var(--color-primary);
    font-variant-numeric: tabular-nums;
  }

  .chip-body {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .chip-label {
    font-size: 0.82rem;
    font-weight: 800;
    color: var(--color-text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    white-space: nowrap;
  }

  .chip-actions {
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  .chip-link {
    font: inherit;
    font-size: 0.8rem;
    font-weight: 700;
    color: var(--color-primary);
    text-decoration: none;
    background: none;
    border: none;
    padding: 0;
    cursor: pointer;
    white-space: nowrap;
  }

  .chip-link:hover {
    text-decoration: underline;
  }

  .chip-link:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
    border-radius: var(--radius-sm);
  }

  .chip-sep {
    color: var(--color-text-muted);
    font-size: 0.75rem;
  }

  .chip-badge {
    font-size: 0.72rem;
    font-weight: 800;
    color: var(--color-text-muted);
    padding: 2px 7px;
    border-radius: 999px;
    border: 1px solid var(--color-border-strong);
    background: var(--color-bg-elevated);
    white-space: nowrap;
  }

  @media (max-width: 640px) {
    .flow-compact {
      flex-direction: column;
      align-items: stretch;
      border-radius: 16px;
      padding: 10px;
    }

    .connector {
      display: none;
    }

    .chip {
      width: 100%;
    }
  }
</style>
