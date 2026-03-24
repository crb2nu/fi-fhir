<script lang="ts">
  /**
   * Collapsible sidebar panel.
   * Renders slot content when open; collapses to zero width when closed.
   */

  export let open: boolean = false;
  export let width: number = 280;
</script>

<aside
  class="sidebar"
  class:open
  style="--sidebar-w: {width}px"
  aria-hidden={!open}
>
  {#if open}
    <div class="sidebar-content">
      <slot />
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
    transition: width var(--duration-slow) var(--ease-in-out),
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
    padding: var(--space-3);
  }
</style>
