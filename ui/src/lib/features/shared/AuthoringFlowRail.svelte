<script lang="ts">
  import type { FlowStep } from './authoringFlow';

  export let eyebrow = 'Authoring flow';
  export let title: string;
  export let summary = '';
  export let steps: readonly FlowStep[] = [];
</script>

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
              <span class="badge metric">{step.metric}</span>
            {/if}
            {#if step.status}
              <span class="badge status">{step.status}</span>
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
                  href={action.href}
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

  .badge {
    display: inline-flex;
    align-items: center;
    min-height: 24px;
    padding: 0 9px;
    border-radius: 999px;
    border: 1px solid var(--color-border-strong);
    background: var(--color-bg-elevated);
    color: var(--color-text-secondary);
    font-size: 0.78rem;
    font-weight: 800;
  }

  .metric {
    color: var(--color-text-primary);
  }

  .status {
    border-color: rgba(56, 189, 248, 0.28);
    background: rgba(56, 189, 248, 0.1);
    color: var(--color-primary);
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

  .action.primary:hover {
    box-shadow: var(--shadow-glow-primary);
  }
</style>
