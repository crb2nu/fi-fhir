<script lang="ts">
  import { resolve } from '$app/paths';
  import { getJourneyState } from './journey';

  /**
   * Shared 5-stage journey indicator used by the shell and mission control.
   * Compact mode keeps the rail slim; full mode expands the stage cards.
   */

  export let pathname: string = '/';
  export let variant: 'compact' | 'full' = 'compact';
  export let showAction: boolean = true;

  let journey = getJourneyState(pathname);
  let isCompact = variant === 'compact';
  let heading = journey.title;
  let eyebrow = journey.progressLabel;
  let description = journey.description;

  $: journey = getJourneyState(pathname);
  $: isCompact = variant === 'compact';
  $: heading = !journey.stage && !isCompact ? 'Operator journey' : journey.title;
  $: eyebrow = !journey.stage && isCompact ? 'Journey overview' : journey.progressLabel;
  $: description = !journey.stage && isCompact
    ? 'Track the five-stage operator flow without leaving the current workspace.'
    : journey.description;
  // The strip's "Next up" chip already announces intent, so the verb prefix
  // ("Continue to X") only costs horizontal space on small screens.
  $: nextShortLabel = journey.nextAction.label.replace(/^(?:Continue to|Return to|Start)\s+/, '');
</script>

<section class="journey {variant}" aria-label={heading}>
  {#if isCompact}
    <!-- Single-line summary shown below 760px in place of the card grid. -->
    <div class="journey-strip">
      <span class="strip-stage">
        {#if journey.stage}
          <span class="strip-count">{journey.stage.order}/{journey.totalStages}</span>
        {/if}
        <span class="strip-title">{heading}</span>
      </span>
      <a class="strip-next" href={resolve(journey.nextAction.href)}>
        <span class="strip-next-label">Next up</span>
        <span class="strip-next-action">{nextShortLabel}</span>
      </a>
    </div>
  {/if}

  <div class="journey-top">
    <div class="copy">
      <div class="eyebrow">{eyebrow}</div>
      <div class="heading-row">
        <h2>{heading}</h2>
        {#if journey.stage}
          <span class="stage-count">{journey.stage.order}/{journey.totalStages}</span>
        {/if}
      </div>
      <p>{description}</p>
    </div>

    {#if isCompact}
      <a class="next-strip" href={resolve(journey.nextAction.href)}>
        <span class="next-label">Next up</span>
        <span class="next-action">{journey.nextAction.label}</span>
      </a>
    {:else if showAction}
      <a class="primary-action" href={resolve(journey.nextAction.href)}>
        <span>{journey.nextAction.label}</span>
        <small>{journey.nextAction.hint}</small>
      </a>
    {/if}
  </div>

  <div class="stage-grid" aria-label="Journey stages">
    {#each journey.steps as stage (stage.id)}
      <a
        class="stage-card"
        class:current={stage.state === 'current'}
        class:complete={stage.state === 'complete'}
        class:upcoming={stage.state === 'upcoming'}
        href={resolve(stage.route)}
        aria-current={stage.state === 'current' ? 'step' : undefined}
      >
        <div class="stage-card-top">
          <span class="stage-order">{stage.order}</span>
          <span class="stage-state">
            {stage.state === 'current' ? 'Active' : stage.state === 'complete' ? 'Done' : 'Next'}
          </span>
        </div>

        <span class="stage-label">{stage.label}</span>

        {#if !isCompact}
          <span class="stage-summary">{stage.summary}</span>
          <span class="stage-focus">
            {#each stage.focus as focus, index (focus)}
              <span class="focus-chip">{focus}</span>
              {#if index < stage.focus.length - 1}
                <span class="focus-sep" aria-hidden="true"></span>
              {/if}
            {/each}
          </span>
        {/if}
      </a>
    {/each}
  </div>

  {#if !isCompact}
    <div class="journey-footer">
      <span class="footer-label">
        {#if journey.stage}
          {journey.stage.label} is active
        {:else}
          Mission control is ready
        {/if}
      </span>
      <span class="footer-hint">{journey.nextAction.hint}</span>
    </div>
  {/if}
</section>

<style>
  .journey {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-4);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-xl);
    background:
      radial-gradient(circle at top left, rgba(96, 165, 250, 0.16), transparent 42%),
      linear-gradient(180deg, rgba(255, 255, 255, 0.03), transparent),
      var(--color-bg-elevated);
    box-shadow: var(--shadow-sm);
  }

  .journey.compact {
    padding: 12px 14px;
    gap: var(--space-2);
  }

  .journey-top {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
  }

  .copy {
    display: grid;
    gap: var(--space-2);
    min-width: 0;
  }

  .journey.compact .copy {
    gap: 4px;
  }

  .eyebrow {
    color: var(--color-text-muted);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    letter-spacing: var(--tracking-wider);
    text-transform: uppercase;
  }

  .heading-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  h2 {
    margin: 0;
    font-size: var(--text-lg);
    line-height: var(--leading-tight);
  }

  .stage-count {
    padding: 3px 8px;
    border-radius: var(--radius-full);
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
    color: var(--color-text-muted);
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    white-space: nowrap;
  }

  .copy p {
    margin: 0;
    color: var(--color-text-secondary);
    line-height: var(--leading-relaxed);
  }

  .journey.compact .copy p {
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-muted);
  }

  .primary-action {
    display: grid;
    gap: 2px;
    align-content: start;
    min-width: 168px;
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-primary-border);
    background: var(--color-primary-muted);
    color: var(--color-primary);
    text-decoration: none;
    transition: var(--transition-all);
  }

  .primary-action:hover {
    transform: translateY(-1px);
    background: var(--color-primary);
    color: var(--color-text-inverse);
    box-shadow: var(--shadow-md);
  }

  .primary-action small {
    color: inherit;
    font-size: var(--text-xs);
    opacity: 0.82;
    line-height: var(--leading-snug);
  }

  .next-strip {
    display: grid;
    gap: 2px;
    min-width: 168px;
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
    color: var(--color-text-primary);
    text-decoration: none;
    transition: var(--transition-all);
  }

  .next-strip:hover {
    transform: translateY(-1px);
    border-color: var(--color-border-strong);
    background: var(--color-bg-hover);
    box-shadow: var(--shadow-sm);
  }

  .next-label {
    color: var(--color-text-muted);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    letter-spacing: var(--tracking-wider);
    text-transform: uppercase;
  }

  .next-action {
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
  }

  .stage-grid {
    display: grid;
    gap: var(--space-2);
  }

  .journey.compact .stage-grid {
    grid-template-columns: repeat(5, minmax(0, 1fr));
    gap: 10px;
  }

  .journey.full .stage-grid {
    grid-template-columns: repeat(5, minmax(0, 1fr));
  }

  .stage-card {
    display: grid;
    gap: 6px;
    padding: var(--space-3);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
    color: inherit;
    text-decoration: none;
    transition: var(--transition-all);
    min-width: 0;
  }

  .journey.compact .stage-card {
    gap: 8px;
    padding: 12px;
    min-height: 84px;
    align-content: start;
  }

  .stage-card:hover {
    transform: translateY(-1px);
    border-color: var(--color-border-strong);
    background: var(--color-bg-hover);
    box-shadow: var(--shadow-sm);
  }

  .stage-card.current {
    border-color: var(--color-primary-border);
    background: var(--color-primary-muted);
  }

  .stage-card.complete {
    border-color: var(--color-success-border);
    background: var(--color-success-bg);
  }

  .stage-card.upcoming {
    opacity: 0.88;
  }

  .stage-card-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .journey.compact .stage-card-top {
    align-items: center;
  }

  .stage-order {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 24px;
    height: 24px;
    border-radius: 999px;
    background: var(--color-bg-base);
    color: var(--color-text-primary);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    border: 1px solid var(--color-border-subtle);
    flex: 0 0 auto;
  }

  .stage-card.current .stage-order {
    border-color: var(--color-primary-border);
    color: var(--color-primary);
  }

  .stage-card.complete .stage-order {
    border-color: var(--color-success-border);
    color: var(--color-success-text);
  }

  .stage-state {
    color: var(--color-text-muted);
    font-size: var(--text-2xs);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-wide);
    text-transform: uppercase;
    white-space: nowrap;
  }

  .journey.compact .stage-state {
    letter-spacing: 0.08em;
  }

  .stage-label {
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .journey.compact .stage-label {
    font-size: var(--text-xs);
    line-height: 1.35;
  }

  .stage-summary {
    color: var(--color-text-muted);
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
  }

  .stage-focus {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 4px;
  }

  .focus-chip {
    color: var(--color-text-secondary);
    font-size: var(--text-2xs);
    font-weight: var(--font-medium);
  }

  .focus-sep {
    width: 3px;
    height: 3px;
    border-radius: 999px;
    background: var(--color-border-strong);
  }

  .journey-footer {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    padding-top: var(--space-2);
    border-top: 1px solid var(--color-border-subtle);
    color: var(--color-text-muted);
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
  }

  .footer-label {
    font-weight: var(--font-semibold);
    color: var(--color-text-secondary);
  }

  .footer-hint {
    text-align: right;
  }

  .journey-strip {
    display: none;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    min-width: 0;
  }

  .strip-stage {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .strip-count {
    padding: 3px 8px;
    border-radius: var(--radius-full);
    border: 1px solid var(--color-primary-border);
    background: var(--color-primary-muted);
    color: var(--color-primary);
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    white-space: nowrap;
    flex: 0 0 auto;
  }

  .strip-title {
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .strip-next {
    display: inline-flex;
    align-items: baseline;
    gap: 6px;
    padding: 6px 10px;
    border-radius: var(--radius-full);
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
    color: var(--color-text-primary);
    text-decoration: none;
    transition: var(--transition-all);
    min-width: 0;
    flex: 0 1 auto;
  }

  .strip-next:hover {
    border-color: var(--color-border-strong);
    background: var(--color-bg-hover);
  }

  .strip-next-label {
    color: var(--color-text-muted);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    letter-spacing: var(--tracking-wider);
    text-transform: uppercase;
    white-space: nowrap;
  }

  .strip-next-action {
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  @media (max-width: 1180px) {
    .journey.full .stage-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }
  }

  @media (max-width: 980px) {
    .journey-top {
      flex-direction: column;
    }

    .primary-action,
    .next-strip {
      min-width: 0;
      width: 100%;
    }

    /* Compact keeps its five-across row down to 760px; below that the
       strip takes over entirely (see the 759px block). */
    .journey.full .stage-grid {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 759px) {
    .journey.compact {
      padding: 10px 12px;
      gap: 0;
    }

    .journey.compact .journey-top,
    .journey.compact .stage-grid {
      display: none;
    }

    .journey.compact .journey-strip {
      display: flex;
    }
  }
</style>
