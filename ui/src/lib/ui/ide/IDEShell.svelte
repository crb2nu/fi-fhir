<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import ActivityBar from './ActivityBar.svelte';
  import Sidebar from './Sidebar.svelte';
  import EditorTabs from './EditorTabs.svelte';
  import BottomPanel from './BottomPanel.svelte';
  import StatusBar from './StatusBar.svelte';
  import ThemeToggle from '$lib/theme/ThemeToggle.svelte';
  import CommandPalette from '$lib/ui/CommandPalette.svelte';
  import type { PaletteCommand } from '$lib/ui/CommandPalette.svelte';
  import {
    ideState,
    toggleSidebar,
    setActiveView,
    closeTab as closeTabAction,
    setActiveTab,
    toggleBottomPanel,
    setActivePanelTab,
  } from './ideStore';
  import { initKeyboardShortcuts } from './keyboardShortcuts';
  import type { IDEView, PanelTab } from './types';
  import DebugPanel from '$lib/features/debug/DebugPanel.svelte';
  import TraceTimeline from '$lib/features/debug/TraceTimeline.svelte';
  import { traceSpans } from '$lib/features/debug/debugStore';

  /**
   * IDE Shell composition root.
   * Full VS Code-style layout with activity bar, sidebar, editor tabs,
   * bottom panel, and status bar.
   */

  export let connectionState: 'connected' | 'disconnected' | 'connecting' = 'disconnected';
  export let activeProfile: string = '';
  export let parserStatus: string = '';

  let paletteOpen = false;
  let shortcutLabel = 'Ctrl+K';
  let cleanupShortcuts: (() => void) | null = null;
  type AppRoute = '/' | '/events' | '/hl7' | '/profiles' | '/terminology' | '/workflows';

  const viewRoutes: Record<IDEView, AppRoute> = {
    hl7: '/hl7',
    workflows: '/workflows',
    events: '/events',
    profiles: '/profiles',
    terminology: '/terminology',
    system: '/',
  };

  const routeToView: Record<string, IDEView> = {
    '/hl7': 'hl7',
    '/workflows': 'workflows',
    '/events': 'events',
    '/profiles': 'profiles',
    '/terminology': 'terminology',
    '/': 'system',
  };

  const navCommands: PaletteCommand[] = [
    { id: 'nav:hl7', label: 'Go to HL7 Mapping', hint: '/hl7', keywords: ['navigate', 'hl7'], run: () => goto(resolve('/hl7')) },
    { id: 'nav:workflows', label: 'Go to Workflows', hint: '/workflows', keywords: ['navigate', 'workflows'], run: () => goto(resolve('/workflows')) },
    { id: 'nav:events', label: 'Go to Events', hint: '/events', keywords: ['navigate', 'events'], run: () => goto(resolve('/events')) },
    { id: 'nav:profiles', label: 'Go to Profiles', hint: '/profiles', keywords: ['navigate', 'profiles'], run: () => goto(resolve('/profiles')) },
    { id: 'nav:terminology', label: 'Go to Terminology', hint: '/terminology', keywords: ['navigate', 'terminology'], run: () => goto(resolve('/terminology')) },
    { id: 'nav:system', label: 'Go to System', hint: '/', keywords: ['navigate', 'system', 'home'], run: () => goto(resolve('/')) },
    { id: 'cmd:toggle-sidebar', label: 'Toggle Sidebar', keywords: ['sidebar', 'panel'], run: () => toggleSidebar() },
    { id: 'cmd:toggle-panel', label: 'Toggle Bottom Panel', keywords: ['panel', 'output', 'problems'], run: () => toggleBottomPanel() },
    { id: 'cmd:debug-panel', label: 'Open Debug Panel', hint: 'Cmd+Shift+D', keywords: ['debug', 'breakpoint', 'step'], run: () => { setActivePanelTab('debug'); if (!$ideState.bottomPanelOpen) toggleBottomPanel(); } },
    { id: 'cmd:trace-panel', label: 'Open Trace Timeline', keywords: ['trace', 'timeline', 'spans'], run: () => { setActivePanelTab('trace'); if (!$ideState.bottomPanelOpen) toggleBottomPanel(); } },
  ];

  function detectViewFromPath(pathname: string): IDEView {
    for (const [route, view] of Object.entries(routeToView)) {
      if (route === '/') continue;
      if (pathname === route || pathname.startsWith(route + '/')) {
        return view;
      }
    }
    return 'system';
  }

  // Sync route changes to active view
  $: currentView = detectViewFromPath($page.url.pathname);
  $: setActiveView(currentView);

  function onViewChange(e: CustomEvent<IDEView>): void {
    const view = e.detail;
    const route = viewRoutes[view];
    goto(resolve(route));
  }

  function onTabSelect(e: CustomEvent<string>): void {
    setActiveTab(e.detail);
  }

  function onTabClose(e: CustomEvent<string>): void {
    closeTabAction(e.detail);
  }

  function onPanelTabChange(e: CustomEvent<PanelTab>): void {
    setActivePanelTab(e.detail);
  }

  function onPanelToggle(): void {
    toggleBottomPanel();
  }

  function closeActiveTab(): void {
    const state = $ideState;
    if (state.activeTabId) {
      closeTabAction(state.activeTabId);
    }
  }

  function isHL7Route(pathname: string): boolean {
    return pathname.startsWith('/hl7');
  }

  function openPalette(): void {
    if (isHL7Route($page.url.pathname)) return;
    paletteOpen = true;
  }

  onMount(() => {
    shortcutLabel = navigator.platform.toUpperCase().includes('MAC')
      ? 'Cmd+K'
      : 'Ctrl+K';

    cleanupShortcuts = initKeyboardShortcuts({
      toggleSidebar,
      toggleBottomPanel,
      closeTab: closeActiveTab,
      splitEditor: () => {
        // Split editor placeholder - future feature
      },
      openDebugPanel: () => {
        setActivePanelTab('debug');
        if (!$ideState.bottomPanelOpen) toggleBottomPanel();
      },
    });

    // Also register Cmd+K for command palette (outside HL7 pages)
    const onCmdK = (e: KeyboardEvent) => {
      if (e.defaultPrevented) return;
      if (paletteOpen) return;
      if (isHL7Route($page.url.pathname)) return;
      const el = e.target as HTMLElement | null;
      if (el && (el.tagName === 'INPUT' || el.tagName === 'TEXTAREA' || el.tagName === 'SELECT' || el.isContentEditable)) return;
      const mod = e.metaKey || e.ctrlKey;
      if (mod && (e.key === 'k' || e.key === 'K')) {
        e.preventDefault();
        openPalette();
      }
    };

    window.addEventListener('keydown', onCmdK);

    return () => {
      window.removeEventListener('keydown', onCmdK);
    };
  });

  onDestroy(() => {
    if (cleanupShortcuts) cleanupShortcuts();
  });
</script>

<CommandPalette
  bind:open={paletteOpen}
  title="Commands"
  commands={navCommands}
/>

<div class="ide-shell">
  <!-- Compact header -->
  <header class="ide-header">
    <a class="ide-brand text-gradient" href={resolve('/')}>fi-fhir</a>

    <div class="ide-header-center">
      {#if !isHL7Route($page.url.pathname)}
        <button
          type="button"
          class="command-trigger"
          aria-label="Open commands"
          title="Open commands ({shortcutLabel})"
          on:click={openPalette}
        >
          <svg
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            aria-hidden="true"
          >
            <circle cx="11" cy="11" r="7" />
            <path d="m20 20-3.4-3.4" />
          </svg>
          <span class="command-label">Commands</span>
          <span class="command-shortcut">{shortcutLabel}</span>
        </button>
      {/if}
    </div>

    <div class="ide-header-right">
      <ThemeToggle />
    </div>
  </header>

  <!-- Main body: activity bar + content + sidebar -->
  <div class="ide-body">
    <ActivityBar activeView={currentView} on:change={onViewChange} />

    <div class="ide-main">
      <!-- Editor tabs (only visible when tabs exist) -->
      {#if $ideState.openTabs.length > 0}
        <EditorTabs
          tabs={$ideState.openTabs}
          activeTabId={$ideState.activeTabId}
          on:select={onTabSelect}
          on:close={onTabClose}
        />
      {/if}

      <!-- Page content -->
      <div class="ide-content">
        <slot />
      </div>

      <!-- Bottom panel -->
      <BottomPanel
        open={$ideState.bottomPanelOpen}
        height={$ideState.bottomPanelHeight}
        activeTab={$ideState.activePanelTab}
        on:tabchange={onPanelTabChange}
        on:toggle={onPanelToggle}
      >
        {#if $ideState.activePanelTab === 'debug'}
          <DebugPanel />
        {:else if $ideState.activePanelTab === 'trace'}
          <TraceTimeline spans={$traceSpans} />
        {:else if $ideState.activePanelTab === 'output'}
          <div class="panel-placeholder">Output will appear here during workflow execution.</div>
        {:else if $ideState.activePanelTab === 'problems'}
          <div class="panel-placeholder">No problems detected.</div>
        {/if}
      </BottomPanel>
    </div>

    <Sidebar
      open={$ideState.sidebarOpen}
      width={$ideState.sidebarWidth}
    />
  </div>

  <!-- Status bar -->
  <StatusBar
    {connectionState}
    {activeProfile}
    {parserStatus}
  />
</div>

<style>
  .ide-shell {
    display: flex;
    flex-direction: column;
    height: 100vh;
    width: 100vw;
    overflow: hidden;
    font-family: var(--font-sans);
    color: var(--color-text-primary);
    background: var(--color-bg-base);
  }

  /* ── Header ── */
  .ide-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: var(--ide-header-height, 40px);
    min-height: var(--ide-header-height, 40px);
    padding: 0 var(--space-4);
    background: var(--color-bg-elevated);
    border-bottom: 1px solid var(--color-border-subtle);
    z-index: var(--z-sticky);
  }

  .ide-brand {
    font-family: var(--font-heading);
    font-weight: 800;
    font-size: var(--text-lg);
    letter-spacing: var(--tracking-tight);
    text-decoration: none;
    flex: 0 0 auto;
  }

  .ide-header-center {
    flex: 1;
    display: flex;
    justify-content: center;
  }

  .ide-header-right {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 0 0 auto;
  }

  .command-trigger {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-3);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-lg);
    background: var(--color-bg-surface);
    color: var(--color-text-secondary);
    font-size: var(--text-xs);
    font-weight: var(--font-medium);
    cursor: pointer;
    transition: var(--transition-all);
    min-width: 200px;
    max-width: 400px;
  }

  .command-trigger:hover {
    background: var(--color-bg-hover);
    border-color: var(--color-border-strong);
  }

  .command-trigger:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
    border-color: var(--color-border-focus);
  }

  .command-trigger svg {
    width: 14px;
    height: 14px;
    flex: 0 0 auto;
  }

  .command-label {
    flex: 1;
    text-align: left;
  }

  .command-shortcut {
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    color: var(--color-text-tertiary);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    padding: 1px 5px;
    line-height: 1.2;
  }

  /* ── Body (activity bar + main + sidebar) ── */
  .ide-body {
    display: flex;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }

  /* ── Main content area ── */
  .ide-main {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
  }

  .ide-content {
    flex: 1;
    overflow: auto;
    min-height: 0;
    padding: var(--space-4);
  }

  .panel-placeholder {
    color: var(--color-text-muted);
    font-size: var(--text-sm);
    padding: var(--space-4);
    text-align: center;
  }

  /* ── Mobile responsive: collapse to single pane ── */
  @media (max-width: 768px) {
    .ide-header {
      padding: 0 var(--space-3);
    }

    .command-trigger {
      min-width: auto;
    }

    .command-label,
    .command-shortcut {
      display: none;
    }

    .ide-content {
      padding: var(--space-3);
    }
  }
</style>
