<!--
  StageControl — the five integration stages as a compact segmented control
  in the header (replaces the journey band). The current stage is filled with
  the accent, earlier stages carry a 12 px check, and every segment is a link.
  One tab stop: ArrowLeft/ArrowRight, Home and End move between segments;
  Enter follows the focused one. Off the stage routes (Dashboard, Operations)
  no segment is current.
-->
<script lang="ts">
  import { resolve } from '$app/paths';
  import Check from '@lucide/svelte/icons/check';
  import { Icon } from '$lib/ui/primitives';
  import { getJourneyState } from './journey';

  interface Props {
    pathname?: string;
  }

  let { pathname = '/' }: Props = $props();

  const journey = $derived(getJourneyState(pathname));
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
          class:is-current={step.state === 'current'}
          class:is-complete={step.state === 'complete'}
          data-state={step.state}
          aria-current={step.state === 'current' ? 'step' : undefined}
          tabindex={step.id === tabStop ? 0 : -1}
          title="Stage {step.order} of {journey.totalStages}: {step.label}"
          onkeydown={(event) => onKeydown(event, index)}
        >
          {#if step.state === 'complete'}
            <Icon icon={Check} size={12} strokeWidth={2.25} class="stage-check" />
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
