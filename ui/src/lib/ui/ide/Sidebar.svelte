<script lang="ts">
  import { resolve } from '$app/paths';
  import Panel from '$lib/ui/Panel.svelte';
  import JourneyProgress from './JourneyProgress.svelte';
  import { getSidebarContext, getSidebarViewLinks } from './sidebar/sidebarContent';

  /**
   * Contextual right sidebar for the IDE shell.
   * Keeps the route-specific workbench metadata close to the main canvas.
   */

  export let open: boolean = false;
  export let width: number = 280;
  export let pathname: string = '/';

  let context = getSidebarContext(pathname);
  const viewLinks = getSidebarViewLinks();

  $: context = getSidebarContext(pathname);
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
      <JourneyProgress pathname={pathname} variant="compact" showAction={false} />

      <Panel title="Current stage" padding="md">
        <div class="hero">
          <div class="stage-header">
            <div class="stage-copy">
              <div class="eyebrow">{context.eyebrow}</div>
              <h3>{context.title}</h3>
            </div>

            <span class="stage-badge">
              {#if context.journey.stage}
                {context.journey.stage.order}/{context.journey.totalStages}
              {:else}
                Ready
              {/if}
            </span>
          </div>

          <p class="description">{context.description}</p>

          <div class="chips" aria-label="Current focus">
            {#each context.highlights as highlight (highlight)}
              <span class="chip">{highlight}</span>
            {/each}
          </div>

          <a
            class="primary-action"
            href={resolve(context.journey.nextAction.href)}
          >
            <span class="action-label">{context.journey.nextAction.label}</span>
            <span class="action-hint">{context.journey.nextAction.hint}</span>
          </a>
        </div>
      </Panel>

      <Panel title="Navigator" padding="sm">
        <nav class="stack" aria-label="Journey navigation">
          {#each viewLinks as link (link.view)}
            <a
              class="nav-link"
              class:active={link.view === context.view}
              aria-current={link.view === context.view ? 'page' : undefined}
              href={resolve(link.href)}
            >
              <span class="nav-label">{link.label}</span>
              <span class="nav-hint">{link.href}</span>
            </a>
          {/each}
        </nav>
      </Panel>

      <Panel title="Operator surfaces" padding="sm">
        <div class="sections">
          <div class="section">
            <div class="section-label">Supporting moves</div>
            <div class="stack">
              {#each context.actions as action, index (action.label)}
                <a
                  class="action-link"
                  aria-describedby={`action-hint-${context.view}-${index}`}
                  href={resolve(action.href)}
                >
                  <span class="action-label">{action.label}</span>
                  <span
                    id={`action-hint-${context.view}-${index}`}
                    class="action-hint"
                    aria-hidden="true"
                  >
                    {action.hint}
                  </span>
                </a>
              {/each}
            </div>
          </div>

          <div class="section">
            <div class="section-label">Related surfaces</div>
            <div class="stack">
              {#each context.recent as item (item.label)}
                <div class="asset">
                  <div class="asset-label">{item.label}</div>
                  <div class="asset-detail">{item.detail}</div>
                </div>
              {/each}
            </div>
          </div>
        </div>
      </Panel>
    </div>
  {/if}
</aside>

<style>
  .sidebar {
    width: 0;
    min-width: 0;
    overflow: hidden;
    background: var(--ide-sidebar-bg, var(--color-bg-surface));
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
    width: var(--sidebar-w, 280px);
    height: 100%;
    overflow: auto;
    display: grid;
    gap: var(--space-3);
    padding: var(--space-3);
  }

  .hero {
    display: grid;
    gap: var(--space-3);
  }

  .stage-header {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .stage-copy {
    display: grid;
    gap: 4px;
    min-width: 0;
  }

  .eyebrow {
    color: var(--color-text-muted);
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    letter-spacing: var(--tracking-wider);
    text-transform: uppercase;
  }

  h3 {
    margin: 0;
    font-size: var(--text-base);
    line-height: var(--leading-tight);
    color: var(--color-text-primary);
  }

  .stage-badge {
    padding: 4px 9px;
    border-radius: var(--radius-full);
    border: 1px solid var(--color-border-subtle);
    background: rgba(255, 255, 255, 0.04);
    color: var(--color-text-muted);
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    white-space: nowrap;
  }

  .description {
    margin: 0;
    color: var(--color-text-secondary);
    line-height: var(--leading-relaxed);
    font-size: var(--text-sm);
  }

  .chips {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .chip {
    padding: 4px 10px;
    border-radius: var(--radius-full);
    border: 1px solid var(--color-border-subtle);
    background: rgba(255, 255, 255, 0.03);
    color: var(--color-text-secondary);
    font-size: var(--text-2xs);
    font-weight: var(--font-semibold);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wide);
  }

  .stack {
    display: grid;
    gap: var(--space-2);
  }

  .sections {
    display: grid;
    gap: var(--space-3);
  }

  .section {
    display: grid;
    gap: var(--space-2);
  }

  .section-label {
    color: var(--color-text-muted);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }

  .nav-link,
  .primary-action,
  .action-link {
    display: grid;
    gap: 4px;
    padding: var(--space-3);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
    color: inherit;
    text-decoration: none;
    transition: var(--transition-all);
  }

  .primary-action {
    border-color: var(--color-primary-border);
    background: var(--color-primary-muted);
  }

  .primary-action:hover {
    transform: translateY(-1px);
    border-color: var(--color-primary-border);
    background: var(--color-primary);
    color: var(--color-text-inverse);
    box-shadow: var(--shadow-glow-primary);
  }

  .nav-link.active {
    border-color: var(--color-primary-border);
    background: var(--color-primary-muted);
  }

  .primary-action:hover,
  .action-link:hover {
    transform: translateY(-1px);
    border-color: var(--color-border-strong);
    background: var(--color-bg-hover);
    box-shadow: var(--shadow-sm);
  }

  .nav-link:hover {
    transform: translateY(-1px);
    border-color: var(--color-border-strong);
    background: var(--color-bg-hover);
    box-shadow: var(--shadow-sm);
  }

  .nav-label,
  .action-label,
  .asset-label {
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .nav-hint,
  .action-hint,
  .asset-detail {
    font-size: var(--text-xs);
    color: var(--color-text-muted);
    line-height: var(--leading-snug);
  }

  .asset {
    display: grid;
    gap: 4px;
    padding: var(--space-3);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-subtle);
    background: linear-gradient(180deg, rgba(255, 255, 255, 0.02), transparent);
  }

  @media (max-width: 980px) {
    .sidebar {
      position: static;
      top: auto;
    }
  }
</style>
