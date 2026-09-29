<!--
  StageControl — the five integration stages as a compact segmented control
  in the header. The current stage (the route's) is filled with the accent;
  a stage whose evidence exists carries a 12 px check; a stage whose evidence
  cannot be read here (a missing role, a capability not configured) carries a
  dashed circle and says why in its title. Completion comes from evidence
  (journeyState.ts), never from the stage's position. Every segment is a link.
  One tab stop: ArrowLeft/ArrowRight, Home and End move between segments;
  Enter follows the focused one. Off the stage routes (Home, Operator) no
  segment is current.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import Check from '@lucide/svelte/icons/check';
  import CircleDashed from '@lucide/svelte/icons/circle-dashed';
  import { Icon } from '$lib/ui/primitives';
  import { getJourneyState, stageStateWord, stageTitle, type JourneyEvidence } from './journey';
  import { journeyEvidenceValue } from './journeyState';

  interface Props {
    pathname?: string;
    /** Evidence to render; defaults to the shell's journey evidence store. */
    evidence?: JourneyEvidence | null | undefined;
  }

  let { pathname = '/', evidence }: Props = $props();

  const resolved = $derived(evidence === undefined ? $journeyEvidenceValue : evidence);
  const journey = $derived(getJourneyState(pathname, resolved));
  const tabStop = $derived(journey.stage?.id ?? journey.steps[0]?.id);

  function onKeydown(event: KeyboardEvent, index: number): void {
    const count = journey.steps.length;
    let next: number;
    switch (event.key) {
      case 'ArrowRight':
        next = (index + 1) % count;
        break;
      case 'ArrowLeft':
        next = (index - 1 + count) % count;
        break;
      case 'Home':
        next = 0;
        break;
      case 'End':
        next = count - 1;
        break;
      default:
        return;
    }
    event.preventDefault();
    const list = (event.currentTarget as HTMLElement).closest('ol');
    list?.querySelectorAll<HTMLAnchorElement>('a.stage')[next]?.focus();
  }
</script>

<nav class="stage-control" aria-label="Stages" data-testid="stage-control">
  <ol>
    {#each journey.steps as step, index (step.id)}
      <li>
        <a
          href={resolve(step.route)}
          class="stage"
          class:is-current={step.current}
          class:is-complete={step.state === 'complete'}
          class:is-unknown={step.state === 'unknown'}
          data-state={step.state}
          data-stage={step.id}
          aria-current={step.current ? 'step' : undefined}
          aria-label="{step.label}, stage {step.order} of {journey.totalStages}, {stageStateWord(step.state)}"
          tabindex={step.id === tabStop ? 0 : -1}
          title={stageTitle(step, journey.totalStages)}
          onkeydown={(event) => onKeydown(event, index)}
        >
          {#if step.state === 'complete'}
            <Icon icon={Check} size={12} strokeWidth={2.25} class="stage-check" />
          {:else if step.state === 'unknown'}
            <Icon icon={CircleDashed} size={12} class="stage-unknown" />
          {/if}
          <span class="stage-order" aria-hidden="true">{step.order}</span>
          <span class="stage-label">{step.label}</span>
        </a>
      </li>
    {/each}
  </ol>
</nav>

<style>
  .stage-control {
    flex: 0 0 auto;
    min-width: 0;
  }

  ol {
    display: flex;
    margin: 0;
    padding: 0;
    list-style: none;
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  li + li {
    border-left: 1px solid var(--color-border-subtle);
  }

  .stage {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    height: 22px;
    padding: 0 10px;
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
    font-weight: var(--font-medium);
    line-height: 1;
    text-decoration: none;
    white-space: nowrap;
    transition: var(--transition-colors);
  }

  .stage:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .stage.is-complete {
    color: var(--color-text-secondary);
  }

  .stage.is-unknown :global(.stage-unknown) {
    color: var(--color-text-muted);
  }

  .stage.is-current,
  .stage.is-current:hover {
    background: var(--color-primary);
    color: var(--color-text-inverse);
  }

  .stage:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
  }

  .stage.is-current:focus-visible {
    outline-color: var(--color-text-inverse);
  }

  .stage-order {
    display: none;
    font-family: var(--font-mono);
    font-variant-numeric: tabular-nums;
  }

  /* Narrow windows: numbers for every stage, the name for the current one. */
  @media (max-width: 1180px) {
    .stage:not(.is-current) .stage-order {
      display: inline;
    }

    .stage:not(.is-current) .stage-label {
      position: absolute;
      width: 1px;
      height: 1px;
      overflow: hidden;
      clip: rect(0, 0, 0, 0);
      white-space: nowrap;
    }

    .stage {
      padding: 0 8px;
    }
  }
</style>
