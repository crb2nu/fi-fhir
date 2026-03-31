<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import { getSidebarContext, getSidebarViewLinks } from './sidebar/sidebarContent';

  /**
   * Contextual right sidebar for the IDE shell.
   * Keeps the route-specific workbench metadata close to the main canvas.
   */

  export let open: boolean = false;
  export let width: number = 280;
  export let pathname: string = '/';

  $: context = getSidebarContext(pathname);
  $: viewLinks = getSidebarViewLinks();
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
      <Panel title={context.title} padding="md">
        <div class="hero">
          <div class="eyebrow">{context.eyebrow}</div>
          <p class="description">{context.description}</p>

          <div class="chips" aria-label="Current focus">
            {#each context.highlights as highlight (highlight)}
              <span class="chip">{highlight}</span>
            {/each}
          </div>
        </div>
      </Panel>

      <Panel title="View navigation" padding="sm">
        <nav class="stack" aria-label="Workbench views">
          {#each viewLinks as link (link.view)}
            <a
              class="nav-link"
              class:active={link.view === context.view}
              aria-current={link.view === context.view ? 'page' : undefined}
              aria-describedby={`nav-hint-${link.view}`}
              href={link.href}
            >
              <span class="nav-label">{link.label}</span>
              <span id={`nav-hint-${link.view}`} class="nav-hint" aria-hidden="true">{link.href}</span>
            </a>
          {/each}
        </nav>
      </Panel>

      <Panel title="Quick actions" padding="sm">
        <div class="stack">
          {#each context.actions as action, index (action.label)}
            <a
              class="action-link"
              aria-describedby={`action-hint-${context.view}-${index}`}
              href={action.href}
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
      </Panel>

      <Panel title="Recent surfaces" padding="sm">
        <div class="stack">
          {#each context.recent as item (item.label)}
            <div class="asset">
              <div class="asset-label">{item.label}</div>
              <div class="asset-detail">{item.detail}</div>
            </div>
          {/each}
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
    gap: var(--space-4);
    padding: var(--space-3);
  }

  .hero {
    display: grid;
    gap: var(--space-3);
  }

  .eyebrow {
    color: var(--color-text-muted);
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    letter-spacing: var(--tracking-wider);
    text-transform: uppercase;
  }

  .description {
    margin: 0;
    color: var(--color-text-secondary);
    line-height: var(--leading-relaxed);
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
    background: var(--color-bg-surface);
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

  .nav-link,
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

  .nav-link:hover,
  .action-link:hover {
    transform: translateY(-1px);
    border-color: var(--color-border-strong);
    background: var(--color-bg-hover);
    box-shadow: var(--shadow-sm);
  }

  .nav-link.active {
    border-color: var(--color-primary-border);
    background: var(--color-primary-muted);
    color: var(--color-primary);
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
