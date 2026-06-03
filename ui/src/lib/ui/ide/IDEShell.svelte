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
  import DocumentHost from './DocumentHost.svelte';
  import ThemeToggle from '$lib/theme/ThemeToggle.svelte';
  import CommandPalette from '$lib/ui/CommandPalette.svelte';
  import type { PaletteCommand } from '$lib/ui/CommandPalette.svelte';
  import {
    ideState,
    toggleSidebar,
    setActiveView,
    openTab as openTabAction,
    openDocument,
    closeTab as closeTabAction,
    setActiveTab,
    toggleBottomPanel,
    toggleWorkspaceSplit,
    setActivePanelTab,
    setSecondaryDocument,
    createWorkspaceTab,
    createDocument,
    resolveNextWorkspaceTabId,
  } from './ideStore';
  import { initKeyboardShortcuts } from './keyboardShortcuts';
  import type { IDEView, PanelTab, IDEAppRoute, DocumentType } from './types';
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

  /** Navigate to a resolved path, bypassing SvelteKit typed route constraints. */
  function navigateTo(path: string): void {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any, svelte/no-navigation-without-resolve -- resolve() is called inside
    void (goto as any)((resolve as any)(path));
  }

  // ── Command palette commands ──

  const navCommands: PaletteCommand[] = [
    { id: 'nav:hl7', label: 'Go to HL7 / Intake', hint: '/hl7', category: 'Navigation', keywords: ['navigate', 'hl7', 'source intake'], run: () => goto(resolve('/hl7')) },
    { id: 'nav:workflows', label: 'Go to Workflows', hint: '/workflows', category: 'Navigation', keywords: ['navigate', 'workflows', 'delivery'], run: () => goto(resolve('/workflows')) },
    { id: 'nav:events', label: 'Go to Events', hint: '/events', category: 'Navigation', keywords: ['navigate', 'events', 'verification'], run: () => goto(resolve('/events')) },
    { id: 'nav:profiles', label: 'Go to Profiles', hint: '/profiles', category: 'Navigation', keywords: ['navigate', 'profiles', 'normalization'], run: () => goto(resolve('/profiles')) },
    { id: 'nav:terminology', label: 'Go to Terminology', hint: '/terminology', category: 'Navigation', keywords: ['navigate', 'terminology', 'translation'], run: () => goto(resolve('/terminology')) },
    { id: 'nav:system', label: 'Go to Dashboard', hint: '/', category: 'Navigation', keywords: ['navigate', 'mission control', 'home', 'dashboard'], run: () => goto(resolve('/')) },
    { id: 'cmd:toggle-sidebar', label: 'Toggle Sidebar', category: 'Workspace', keywords: ['sidebar', 'panel'], run: () => toggleSidebar() },
    { id: 'cmd:toggle-panel', label: 'Toggle Bottom Panel', category: 'Workspace', keywords: ['panel', 'output', 'problems'], run: () => toggleBottomPanel() },
    { id: 'cmd:debug-panel', label: 'Open Debug Panel', hint: 'Cmd+Shift+D', category: 'Workspace', keywords: ['debug', 'breakpoint', 'step'], run: () => { setActivePanelTab('debug'); if (!$ideState.bottomPanelOpen) toggleBottomPanel(); } },
    { id: 'cmd:trace-panel', label: 'Open Trace Timeline', category: 'Workspace', keywords: ['trace', 'timeline', 'spans'], run: () => { setActivePanelTab('trace'); if (!$ideState.bottomPanelOpen) toggleBottomPanel(); } },
    // Document artifact commands
    {
      id: 'doc:open-trace',
      label: 'Open Active Trace',
      hint: 'View trace timeline',
      category: 'Documents',
      keywords: ['trace', 'timeline', 'debug'],
      run: () => {
        const doc = createDocument('trace', 'Active Trace', { subtitle: 'Current run' });
        openDocument(doc);
      },
    },
    {
      id: 'doc:open-recent-workflow',
      label: 'Reopen Recent Workflow',
      hint: 'Resume workflow editing',
      category: 'Documents',
      keywords: ['workflow', 'draft', 'recent'],
      run: () => {
        const doc = createDocument('workflow-draft', 'Recent Workflow', { subtitle: 'Draft' });
        openDocument(doc);
      },
    },
    {
      id: 'doc:compare-events',
      label: 'Compare Events',
      hint: 'Side-by-side event diff',
      category: 'Documents',
      keywords: ['compare', 'diff', 'events'],
      run: () => {
        const doc = createDocument('event', 'Event Comparison', { subtitle: 'Side-by-side diff' });
        openDocument(doc);
      },
    },
    {
      id: 'doc:open-profile',
      label: 'Open Profile Revision',
      hint: 'View source profile',
      category: 'Documents',
      keywords: ['profile', 'revision', 'source'],
      run: () => {
        const doc = createDocument('profile', 'Profile Revision', { subtitle: 'Source' });
        openDocument(doc);
      },
    },
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
    navigateTo(route);
  }

  function onTabSelect(e: CustomEvent<string>): void {
    const doc = $ideState.documents.find((entry) => entry.id === e.detail);
    if (!doc) return;
    setActiveTab(doc.id);
    // Only navigate for route-type documents
    if (doc.type === 'route' || !doc.type) {
      navigateTo(doc.path ?? doc.route ?? getWorkspaceTabRoute(doc.view ?? 'system'));
    }
  }

  function closeTabById(closingTabId: string): void {
    const nextTabId = resolveNextWorkspaceTabId($ideState.documents, $ideState.activeDocumentId, closingTabId);
    const nextDoc = nextTabId ? $ideState.documents.find((d) => d.id === nextTabId) ?? null : null;
    const closingWasActive = closingTabId === $ideState.activeDocumentId;

    closeTabAction(closingTabId);

    if (!closingWasActive) return;

    if (nextDoc) {
      if (nextDoc.type === 'route' || !nextDoc.type) {
        navigateTo(nextDoc.path ?? nextDoc.route ?? getWorkspaceTabRoute(nextDoc.view ?? 'system'));
      }
      return;
    }

    navigateTo('/');
  }

  function onTabClose(e: CustomEvent<string>): void {
    closeTabById(e.detail);
  }

  function onTabAdd(e: CustomEvent<DocumentType>): void {
    const type = e.detail;
    const titles: Record<Exclude<DocumentType, 'route'>, string> = {
      'workflow-draft': 'New Workflow',
      'debug-session': 'Debug Session',
      trace: 'Trace View',
      event: 'Event Payload',
      profile: 'Source Profile',
    };
    if (type !== 'route') {
      const doc = createDocument(type, titles[type]);
      openDocument(doc);
    }
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
    if (state.activeDocumentId) {
      closeTabById(state.activeDocumentId);
    }
  }

  function isHL7Route(pathname: string): boolean {
    return pathname.startsWith('/hl7');
  }

  function openPalette(): void {
    if (isHL7Route($page.url.pathname)) return;
    paletteOpen = true;
  }

  /** Determine which document to show in the active (primary) pane. */
  $: activeDocument = $ideState.documents.find((d) => d.id === $ideState.activeDocumentId) ?? null;

  /** Determine the secondary pane document. */
  $: secondaryDocument = $ideState.secondaryDocumentId
    ? $ideState.documents.find((d) => d.id === $ideState.secondaryDocumentId) ?? null
    : null;

  /** Whether the active document is a non-route artifact. */
  $: isArtifactActive = activeDocument != null && activeDocument.type !== 'route' && !!activeDocument.type;

  /** Whether the secondary document is a non-route artifact. */
  $: isArtifactSecondary = secondaryDocument != null && secondaryDocument.type !== 'route' && !!secondaryDocument.type;

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
      {#if $ideState.documents.length > 0}
        <EditorTabs
          tabs={$ideState.documents}
          activeTabId={$ideState.activeDocumentId}
          on:select={onTabSelect}
          on:close={onTabClose}
          on:add={onTabAdd}
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
          <!-- Primary pane -->
          <div class="workspace-pane">
            {#if isArtifactActive && activeDocument}
              <DocumentHost document={activeDocument} />
            {:else}
              <slot />
            {/if}
          </div>

          <!-- Secondary pane -->
          <div slot="secondary" class="workspace-secondary">
            {#if isArtifactSecondary && secondaryDocument}
              <DocumentHost document={secondaryDocument} />
            {:else}
              <section class="workspace-card workspace-summary">
                <div class="workspace-eyebrow">Split workspace</div>
                <h2>{secondaryDocument?.title ?? currentWorkspaceTab.title}</h2>
                <p>Keep another workspace surface open while you move between routes.</p>
                <div class="workspace-path">{secondaryDocument?.route ?? currentWorkspaceTab.path}</div>
                <button type="button" class="workspace-toggle" on:click={toggleWorkspaceSplit}>
                  Close split workspace
                </button>
              </section>

              <section class="workspace-card">
                <div class="workspace-eyebrow">Open documents</div>
                <div class="workspace-tabs">
                  {#each $ideState.documents as doc (doc.id)}
                    <button
                      type="button"
                      class="workspace-tab"
                      class:active={doc.id === $ideState.activeDocumentId}
                      on:click={() => {
                        setSecondaryDocument(doc.id);
                      }}
                    >
                      <span>{doc.title}</span>
                      <small>{doc.type === 'route' ? (doc.path ?? doc.route ?? '/') : doc.type}</small>
                    </button>
                  {/each}
                </div>
              </section>
            {/if}
          </div>
        </SplitPane>
      {:else}
        <!-- Single pane content -->
        <div class="ide-content">
          {#if isArtifactActive && activeDocument}
            <DocumentHost document={activeDocument} />
          {:else}
            <slot />
          {/if}
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
