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
  import JourneyProgress from './JourneyProgress.svelte';
  import ThemeToggle from '$lib/theme/ThemeToggle.svelte';
  import CommandPalette from '$lib/ui/CommandPalette.svelte';
  import type { PaletteCommand } from '$lib/ui/CommandPalette.svelte';
  import {
    ideState,
    toggleSidebar,
    setActiveView,
    openTab as openTabAction,
    closeTab as closeTabAction,
    setActiveTab,
    toggleBottomPanel,
    toggleWorkspaceSplit,
    setActivePanelTab,
    createWorkspaceTab,
    resolveNextWorkspaceTabId,
  } from './ideStore';
  import { initKeyboardShortcuts } from './keyboardShortcuts';
  import type { IDEView, PanelTab, IDEAppRoute } from './types';
  import DebugPanel from '$lib/features/debug/DebugPanel.svelte';
  import TraceTimeline from '$lib/features/debug/TraceTimeline.svelte';
  import { traceSpans } from '$lib/features/debug/debugStore';
  import SplitPane from './SplitPane.svelte';
  import RuntimeOutputPanel from './panels/RuntimeOutputPanel.svelte';
  import ProblemsPanel from './panels/ProblemsPanel.svelte';
  import { platformState, initializePlatform, teardownPlatform } from '$lib/platform';

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
  type WorkspaceTab = ReturnType<typeof createWorkspaceTab>;
  let currentPath = '/';
  let currentView: IDEView = 'hl7';
  let currentWorkspaceTab: WorkspaceTab = createWorkspaceTab('/', 'system');

  const viewRoutes: Record<IDEView, IDEAppRoute> = {
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
    { id: 'nav:hl7', label: 'Go to Source Intake', hint: '/hl7', keywords: ['navigate', 'hl7', 'source intake'], run: () => goto(resolve('/hl7')) },
    { id: 'nav:workflows', label: 'Go to Delivery', hint: '/workflows', keywords: ['navigate', 'workflows', 'delivery'], run: () => goto(resolve('/workflows')) },
    { id: 'nav:events', label: 'Go to Verification', hint: '/events', keywords: ['navigate', 'events', 'verification'], run: () => goto(resolve('/events')) },
    { id: 'nav:profiles', label: 'Go to Normalization', hint: '/profiles', keywords: ['navigate', 'profiles', 'normalization'], run: () => goto(resolve('/profiles')) },
    { id: 'nav:terminology', label: 'Go to Translation', hint: '/terminology', keywords: ['navigate', 'terminology', 'translation'], run: () => goto(resolve('/terminology')) },
    { id: 'nav:system', label: 'Go to Mission Control', hint: '/', keywords: ['navigate', 'mission control', 'home'], run: () => goto(resolve('/')) },
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

  function normalizeRoute(pathname: string): string {
    if (!pathname) return '/';
    if (pathname.length > 1 && pathname.endsWith('/')) {
      return pathname.replace(/\/+$/, '');
    }
    return pathname;
  }

  function getWorkspaceTabRoute(view: IDEView): IDEAppRoute {
    return viewRoutes[view];
  }

  $: currentPath = normalizeRoute($page.url.pathname);
  $: currentView = detectViewFromPath(currentPath);
  $: currentWorkspaceTab = createWorkspaceTab(currentPath, currentView);
  $: setActiveView(currentView);
  $: openTabAction(currentWorkspaceTab);

  function onViewChange(e: CustomEvent<IDEView>): void {
    const view = e.detail;
    const route = getWorkspaceTabRoute(view);
    goto(resolve(route));
  }

  function onTabSelect(e: CustomEvent<string>): void {
    const tab = $ideState.openTabs.find((entry) => entry.id === e.detail);
    if (!tab) return;
    setActiveTab(tab.id);
    goto(resolve(tab.path ?? getWorkspaceTabRoute(tab.view)));
  }

  function closeTabById(closingTabId: string): void {
    const nextTabId = resolveNextWorkspaceTabId($ideState.openTabs, $ideState.activeTabId, closingTabId);
    const nextTab = nextTabId ? $ideState.openTabs.find((tab) => tab.id === nextTabId) ?? null : null;
    const closingWasActive = closingTabId === $ideState.activeTabId;

    closeTabAction(closingTabId);

    if (!closingWasActive) return;

    if (nextTab) {
      goto(resolve(nextTab.path ?? getWorkspaceTabRoute(nextTab.view)));
      return;
    }

    goto(resolve('/'));
  }

  function onTabClose(e: CustomEvent<string>): void {
    closeTabById(e.detail);
  }

  function onPanelTabChange(e: CustomEvent<PanelTab>): void {
    setActivePanelTab(e.detail);
  }

  function onPanelToggle(): void {
    toggleBottomPanel();
  }

  function onPanelNavigate(e: CustomEvent<{ panel: string }>): void {
    const tab = e.detail.panel as import('./types').PanelTab;
    setActivePanelTab(tab);
    if (!$ideState.bottomPanelOpen) toggleBottomPanel();
  }

  function closeActiveTab(): void {
    const state = $ideState;
    if (state.activeTabId) {
      closeTabById(state.activeTabId);
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
        toggleWorkspaceSplit();
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

    initializePlatform();

    return () => {
      window.removeEventListener('keydown', onCmdK);
    };
  });

  onDestroy(() => {
    if (cleanupShortcuts) cleanupShortcuts();
    teardownPlatform();
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

  <JourneyProgress pathname={$page.url.pathname} variant="compact" />

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

      {#if $ideState.workspaceSplit}
        <SplitPane
          orientation="horizontal"
          initialSize={780}
          minSize={520}
          maxSize={1080}
          storageKey="fi-fhir-ide-workspace-split-width"
        >
          <div class="workspace-pane">
            <slot />
          </div>

          <div slot="secondary" class="workspace-secondary">
            <section class="workspace-card workspace-summary">
              <div class="workspace-eyebrow">Split workspace</div>
              <h2>{currentWorkspaceTab.title}</h2>
              <p>Keep another workspace surface open while you move between routes.</p>
              <div class="workspace-path">{currentWorkspaceTab.path}</div>
              <button type="button" class="workspace-toggle" on:click={toggleWorkspaceSplit}>
                Close split workspace
              </button>
            </section>

            <section class="workspace-card">
              <div class="workspace-eyebrow">Open tabs</div>
              <div class="workspace-tabs">
                {#each $ideState.openTabs as tab (tab.id)}
                  <button
                    type="button"
                    class="workspace-tab"
                    class:active={tab.id === $ideState.activeTabId}
                    on:click={() => {
                      setActiveTab(tab.id);
                      goto(resolve(tab.path ?? getWorkspaceTabRoute(tab.view)));
                    }}
                  >
                    <span>{tab.title}</span>
                    <small>{tab.path}</small>
                  </button>
                {/each}
              </div>
            </section>
          </div>
        </SplitPane>
      {:else}
        <!-- Page content -->
        <div class="ide-content">
          <slot />
        </div>
      {/if}

      <!-- Bottom panel -->
      <BottomPanel
        open={$ideState.bottomPanelOpen}
        height={$ideState.bottomPanelHeight}
        activeTab={$ideState.activePanelTab}
        on:tabchange={onPanelTabChange}
        on:toggle={onPanelToggle}
        on:navigate={onPanelNavigate}
      >
        {#if $ideState.activePanelTab === 'debug'}
          <DebugPanel />
        {:else if $ideState.activePanelTab === 'trace'}
          <TraceTimeline spans={$traceSpans} />
        {:else if $ideState.activePanelTab === 'output'}
          <RuntimeOutputPanel on:navigate />
        {:else if $ideState.activePanelTab === 'problems'}
          <ProblemsPanel />
        {/if}
      </BottomPanel>
    </div>

    <Sidebar
      open={$ideState.sidebarOpen}
      width={$ideState.sidebarWidth}
      pathname={$page.url.pathname}
    />
  </div>

  <!-- Status bar -->
  <StatusBar
    {connectionState}
    {activeProfile}
    {parserStatus}
    platformConnected={$platformState.connected}
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

  .workspace-pane {
    height: 100%;
    overflow: auto;
    min-width: 0;
    min-height: 0;
  }

  .workspace-secondary {
    display: grid;
    gap: var(--space-4);
    padding: var(--space-4);
    height: 100%;
    overflow: auto;
    min-width: 0;
    min-height: 0;
    background:
      radial-gradient(circle at top, rgba(255, 255, 255, 0.04), transparent 42%),
      var(--color-bg-surface);
    border-left: 1px solid var(--color-border-subtle);
  }

  .workspace-card {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-4);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-xl);
    background: var(--color-bg-base);
    box-shadow: var(--shadow-sm);
  }

  .workspace-summary h2 {
    margin: 0;
    font-size: var(--text-xl);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-tight);
  }

  .workspace-eyebrow {
    color: var(--color-text-muted);
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    letter-spacing: var(--tracking-wider);
    text-transform: uppercase;
  }

  .workspace-summary p {
    margin: 0;
    color: var(--color-text-secondary);
    line-height: var(--leading-relaxed);
  }

  .workspace-path {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-md);
    background: var(--color-bg-surface);
    color: var(--color-text-muted);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    overflow-x: auto;
  }

  .workspace-toggle {
    justify-self: start;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-lg);
    background: var(--color-bg-surface);
    color: var(--color-text-primary);
    font-size: var(--text-sm);
    cursor: pointer;
    transition: var(--transition-all);
  }

  .workspace-toggle:hover {
    background: var(--color-bg-hover);
    border-color: var(--color-border-strong);
  }

  .workspace-tabs {
    display: grid;
    gap: var(--space-2);
  }

  .workspace-tab {
    display: grid;
    gap: 2px;
    padding: var(--space-3);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-lg);
    background: var(--color-bg-surface);
    color: inherit;
    text-align: left;
    cursor: pointer;
    transition: var(--transition-all);
  }

  .workspace-tab:hover {
    border-color: var(--color-border-strong);
    background: var(--color-bg-hover);
  }

  .workspace-tab.active {
    border-color: var(--color-primary-border);
    background: var(--color-primary-muted);
    color: var(--color-primary);
  }

  .workspace-tab span {
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
  }

  .workspace-tab small {
    color: var(--color-text-muted);
    font-size: var(--text-xs);
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
