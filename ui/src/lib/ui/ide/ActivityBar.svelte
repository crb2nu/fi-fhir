<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { IDEView } from './types';

  /**
   * Left activity bar with 6 navigation icons.
   * 48px wide, renders vertically stacked icon buttons.
   */

  export let activeView: IDEView = 'hl7';

  const dispatch = createEventDispatcher<{ change: IDEView }>();

  type ViewEntry = {
    view: IDEView;
    label: string;
    /** SVG path data for icon (24x24 viewBox) */
    icon: string;
  };

  const views: ViewEntry[] = [
    {
      view: 'hl7',
      label: 'HL7 Messages',
      icon: 'M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8l-6-6z M14 2v6h6 M16 13H8 M16 17H8 M10 9H8',
    },
    {
      view: 'workflows',
      label: 'Workflows',
      icon: 'M6 3v12 M18 9a3 3 0 1 0 0-6 3 3 0 0 0 0 6z M6 21a3 3 0 1 0 0-6 3 3 0 0 0 0 6z M18 9a9 9 0 0 1-9 9',
    },
    {
      view: 'events',
      label: 'Events',
      icon: 'M13 2L3 14h9l-1 8 10-12h-9l1-8z',
    },
    {
      view: 'profiles',
      label: 'Profiles',
      icon: 'M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2 M12 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8z',
    },
    {
      view: 'terminology',
      label: 'Terminology',
      icon: 'M4 19.5A2.5 2.5 0 0 1 6.5 17H20 M4 19.5A2.5 2.5 0 0 0 6.5 22H20V2H6.5A2.5 2.5 0 0 0 4 4.5v15z',
    },
    {
      view: 'system',
      label: 'System',
      icon: 'M12.22 2h-.44a2 2 0 0 0-2 2v.18a2 2 0 0 1-1 1.73l-.43.25a2 2 0 0 1-2 0l-.15-.08a2 2 0 0 0-2.73.73l-.22.38a2 2 0 0 0 .73 2.73l.15.1a2 2 0 0 1 1 1.72v.51a2 2 0 0 1-1 1.74l-.15.09a2 2 0 0 0-.73 2.73l.22.38a2 2 0 0 0 2.73.73l.15-.08a2 2 0 0 1 2 0l.43.25a2 2 0 0 1 1 1.73V20a2 2 0 0 0 2 2h.44a2 2 0 0 0 2-2v-.18a2 2 0 0 1 1-1.73l.43-.25a2 2 0 0 1 2 0l.15.08a2 2 0 0 0 2.73-.73l.22-.39a2 2 0 0 0-.73-2.73l-.15-.08a2 2 0 0 1-1-1.74v-.5a2 2 0 0 1 1-1.74l.15-.09a2 2 0 0 0 .73-2.73l-.22-.38a2 2 0 0 0-2.73-.73l-.15.08a2 2 0 0 1-2 0l-.43-.25a2 2 0 0 1-1-1.73V4a2 2 0 0 0-2-2z M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8z',
    },
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
      title={entry.label}
      on:click={() => onSelect(entry.view)}
    >
      <svg
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="2"
        stroke-linecap="round"
        stroke-linejoin="round"
        aria-hidden="true"
      >
        {#each entry.icon.split(' M') as segment, i (`${entry.view}-${i}`)}
          <path d={i === 0 ? segment : `M${segment}`} />
        {/each}
      </svg>
    </button>
  {/each}
</nav>

<style>
  .activity-bar {
    display: flex;
    flex-direction: column;
    align-items: center;
    width: var(--ide-activity-bar-width, 48px);
    min-width: var(--ide-activity-bar-width, 48px);
    background: var(--ide-activity-bar-bg, var(--color-bg-elevated));
    border-right: 1px solid var(--color-border-subtle);
    padding: var(--space-2) 0;
    gap: var(--space-1);
    overflow-y: auto;
  }

  .activity-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    padding: 6px;
    border: none;
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--color-text-tertiary);
    cursor: pointer;
    transition: var(--transition-all);
    position: relative;
  }

  .activity-btn:hover {
    color: var(--color-text-primary);
    background: var(--color-bg-hover);
  }

  .activity-btn.active {
    color: var(--color-primary);
    background: var(--color-primary-muted);
  }

  .activity-btn.active::before {
    content: '';
    position: absolute;
    left: -6px;
    top: 6px;
    bottom: 6px;
    width: 3px;
    border-radius: 0 2px 2px 0;
    background: var(--color-primary);
  }

  .activity-btn:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .activity-btn svg {
    width: 20px;
    height: 20px;
  }
</style>
