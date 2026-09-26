<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { Icon } from '$lib/ui/primitives';
  import type { IDEView } from './types';
  import { VIEW_ICONS } from './viewIcons';

  /**
   * Left activity bar: one 16 px icon per view, 40 px wide. The label is the
   * accessible name and the native tooltip; the active view gets a 2 px
   * accent bar on its left edge. Order: Home, the five stages in stage
   * order, then Operator. Labels match the route toolbars.
   */

  export let activeView: IDEView = 'hl7';

  const dispatch = createEventDispatcher<{ change: IDEView }>();

  type ViewEntry = {
    view: IDEView;
    /** Domain label; the button's accessible name and tooltip. */
    label: string;
    /** Stage name, added to the tooltip for the five stage views. */
    stage?: string;
  };

  const views: ViewEntry[] = [
    { view: 'system', label: 'Home' },
    { view: 'hl7', label: 'HL7 / Intake', stage: 'Source Intake' },
    { view: 'profiles', label: 'Profiles', stage: 'Normalization' },
    { view: 'terminology', label: 'Terminology', stage: 'Translation' },
    { view: 'workflows', label: 'Workflows', stage: 'Delivery' },
    { view: 'events', label: 'Events', stage: 'Verification' },
    { view: 'operator', label: 'Operator' },
  ];

  function onSelect(view: IDEView): void {
    dispatch('change', view);
  }
</script>

<nav class="activity-bar" aria-label="Activity bar">
  {#each views as entry (entry.view)}
    <button
      type="button"
      class="activity-btn"
      class:active={entry.view === activeView}
      aria-label={entry.label}
      aria-current={entry.view === activeView ? 'true' : undefined}
      title={entry.stage ? `${entry.label} (${entry.stage})` : entry.label}
      on:click={() => onSelect(entry.view)}
    >
      <Icon icon={VIEW_ICONS[entry.view]} />
    </button>
  {/each}
</nav>

<style>
  .activity-bar {
    display: flex;
    flex-direction: column;
    align-items: stretch;
    width: 40px;
    min-width: 40px;
    padding: var(--space-1) 0;
    gap: 2px;
    background: var(--ide-activity-bar-bg, var(--color-bg-elevated));
    border-right: 1px solid var(--color-border-subtle);
    overflow-y: auto;
    scrollbar-width: none;
  }

  .activity-btn {
    position: relative;
    display: flex;
    align-items: center;
    justify-content: center;
    width: 40px;
    height: 36px;
    padding: 0;
    border: none;
    background: transparent;
    color: var(--color-text-tertiary);
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .activity-btn:hover {
    color: var(--color-text-primary);
  }

  .activity-btn.active {
    color: var(--color-text-primary);
  }

  .activity-btn.active::before {
    content: '';
    position: absolute;
    left: 0;
    top: 6px;
    bottom: 6px;
    width: 2px;
    background: var(--color-primary);
  }

  .activity-btn:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -4px;
    border-radius: var(--radius-sm);
  }
</style>
