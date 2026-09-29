<script lang="ts">
  import { resolve } from '$app/paths';
  import { Badge, Icon, Panel } from '$lib/ui/primitives';
  import { getSidebarContext, getSidebarViewLinks, type SidebarView } from './sidebar/sidebarContent';
  import { stageTitle, type JourneyEvidence } from './journey';
  import { journeyEvidenceValue } from './journeyState';
  import type { IDEView } from './types';
  import { VIEW_ICONS } from './viewIcons';

  /**
   * Contextual right sidebar (Cmd/Ctrl+B): what the current view is for (on
   * a stage route, its "n/5" badge and the stage's evidence sentence), the
   * view list, and related views. Flat `Panel` sections with 11 px labels.
   */

  export let open: boolean = false;
  export let width: number = 280;
  export let pathname: string = '/';
  /** Evidence to read; defaults to the shell's journey evidence store. */
  export let evidence: JourneyEvidence | null | undefined = undefined;

  let context = getSidebarContext(pathname, null);
  const viewLinks = getSidebarViewLinks();

  const sidebarToIDEView: Record<SidebarView, IDEView> = {
    home: 'system',
    hl7: 'hl7',
    profiles: 'profiles',
    terminology: 'terminology',
    workflows: 'workflows',
    events: 'events',
    connections: 'connections',
    operator: 'operator',
  };

  $: context = getSidebarContext(pathname, evidence === undefined ? $journeyEvidenceValue : evidence);
  $: stageStep = context.journey.stage
    ? (context.journey.steps.find((step) => step.id === context.journey.stage?.id) ?? null)
    : null;
</script>

<aside
  class="sidebar"
  class:open
  style="--sidebar-w: {width}px"
  aria-hidden={!open}
  aria-label="Workbench sidebar"
>
  {#if open}
    <div class="sidebar-content">
      <Panel title={context.journey.stage ? 'Stage' : 'View'} titleTag="h2">
        {#snippet actions()}
          {#if stageStep}
            <Badge
              mono
              tone={stageStep.state === 'complete' ? 'success' : 'neutral'}
              title={stageTitle(stageStep, context.journey.totalStages)}
              data-testid="sidebar-stage-badge"
              data-state={stageStep.state}
            >
              {stageStep.order}/{context.journey.totalStages}
            </Badge>
          {/if}
        {/snippet}
        <p class="context-title">{context.title}</p>
        <p class="context-description">{context.description}</p>
        {#if stageStep?.reason}
          <p class="context-evidence" data-testid="sidebar-stage-evidence">{stageStep.reason}</p>
        {/if}
      </Panel>

      <Panel title="Views" titleTag="h2" flush>
        <nav class="link-list" aria-label="Views">
          {#each viewLinks as link (link.view)}
            <a
              class="nav-link"
              class:active={link.view === context.view}
              aria-current={link.view === context.view ? 'page' : undefined}
              href={resolve(link.href)}
            >
              <Icon icon={VIEW_ICONS[sidebarToIDEView[link.view]]} size={14} />
              <span class="nav-label">{link.label}</span>
              <span class="nav-route">{link.href}</span>
            </a>
          {/each}
        </nav>
      </Panel>

      <Panel title="Related" titleTag="h2" flush>
        <ul class="link-list">
          {#each context.actions as action (action.label)}
            <li>
              <a class="related-link" href={resolve(action.href)}>
                <span class="nav-label">{action.label}</span>
                <span class="related-hint">{action.hint}</span>
              </a>
            </li>
          {/each}
        </ul>
      </Panel>
    </div>
  {/if}
</aside>

<style>
  .sidebar {
    width: 0;
    min-width: 0;
    overflow: hidden;
    background: var(--ide-sidebar-bg, var(--color-bg-elevated));
    border-left: 1px solid var(--color-border-subtle);
    transition:
      width var(--duration-slow) var(--ease-in-out),
      min-width var(--duration-slow) var(--ease-in-out);
  }

  .sidebar.open {
    width: var(--sidebar-w, 280px);
    min-width: var(--sidebar-w, 280px);
  }

  .sidebar-content {
    display: grid;
    align-content: start;
    gap: var(--space-3);
    width: var(--sidebar-w, 280px);
    height: 100%;
    padding: var(--space-3);
    overflow: auto;
  }

  .context-title {
    margin: 0;
    line-height: var(--leading-snug);
    color: var(--color-text-primary);
    font-size: var(--text-ui);
    font-weight: var(--font-semibold);
  }

  .context-description {
    margin: var(--space-1) 0 0;
    color: var(--color-text-secondary);
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
  }

  .context-evidence {
    margin: var(--space-2) 0 0;
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
  }

  .link-list {
    display: grid;
    margin: 0;
    padding: var(--space-1) 0;
    list-style: none;
  }

  .nav-link {
    position: relative;
    display: flex;
    align-items: center;
    gap: var(--space-2);
    height: 28px;
    padding: 0 var(--space-3);
    color: var(--color-text-secondary);
    font-size: var(--text-ui);
    text-decoration: none;
    transition: var(--transition-colors);
  }

  .nav-link:hover,
  .related-link:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .nav-link.active {
    background: var(--color-primary-muted);
    color: var(--color-text-primary);
  }

  .nav-link.active::before {
    content: '';
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 2px;
    background: var(--color-primary);
  }

  .nav-link:focus-visible,
  .related-link:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
  }

  .nav-label {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .nav-route {
    margin-left: auto;
    color: var(--color-text-muted);
    font-family: var(--font-mono);
    font-size: var(--text-label);
  }

  .related-link {
    display: grid;
    gap: 2px;
    padding: 6px var(--space-3);
    color: var(--color-text-secondary);
    text-decoration: none;
    transition: var(--transition-colors);
  }

  .related-hint {
    color: var(--color-text-muted);
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
  }
</style>
