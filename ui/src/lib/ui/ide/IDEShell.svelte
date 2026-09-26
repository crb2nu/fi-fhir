<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { resolve } from '$app/paths';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import Search from '@lucide/svelte/icons/search';
  import ActivityBar from './ActivityBar.svelte';
  import Sidebar from './Sidebar.svelte';
  import EditorTabs from './EditorTabs.svelte';
  import BottomPanel from './BottomPanel.svelte';
  import StatusBar from './StatusBar.svelte';
  import StageControl from './StageControl.svelte';
  import ThemeToggle from '$lib/theme/ThemeToggle.svelte';
  import CommandPalette from '$lib/ui/CommandPalette.svelte';
  import type { PaletteCommand } from '$lib/ui/CommandPalette.svelte';
  import type { AccessSession } from '$lib/graphql/GraphQLCredentialGate.svelte';
  import { Button, Icon, Panel } from '$lib/ui/primitives';
  import {
    ideState,
    toggleSidebar,
    setActiveView,
    openTab as openTabAction,
    closeTab as closeTabAction,
    setActiveTab,
    toggleBottomPanel,
    toggleWorkspaceSplit,
    openPanelTab,
    createWorkspaceTab,
    resolveNextWorkspaceTabId,
  } from './ideStore';
  import { initKeyboardShortcuts } from './keyboardShortcuts';
  import { getJourneyStage } from './journey';
  import type { ConnectionState } from './connection';
  import type { IDEView, PanelTab, IDEAppRoute } from './types';
  import DebugPanel from '$lib/features/debug/DebugPanel.svelte';
  import TraceTimeline from '$lib/features/debug/TraceTimeline.svelte';
  import { traceSpans } from '$lib/features/debug/debugStore';
  import SplitPane from './SplitPane.svelte';
  import RuntimeOutputPanel from './panels/RuntimeOutputPanel.svelte';
  import ProblemsPanel from './panels/ProblemsPanel.svelte';
  import {
    PLATFORM_CONFIG,
    platformState,
    initializePlatform,
    teardownPlatform
  } from '$lib/platform';

  /**
   * IDE shell composition root: a 40 px header (wordmark, stage control,
   * breadcrumb, command palette, theme), the activity bar, editor tabs, the
   * document region, the bottom panel, the contextual sidebar and a 24 px
   * status bar — the one place for connection and access state (plus
   * Next: stage and the build).
   */

  export let connectionState: ConnectionState = 'disconnected';
  export let activeProfile: string = '';
  export let parserStatus: string = '';
  /** Credential state from GraphQLCredentialGate; rendered as the status-bar chip. */
  export let access: AccessSession | null = null;
  /** Ends a bearer session (GraphQLCredentialGate.clearCredential). */
  export let onClearAccess: (() => void) | undefined = undefined;

  let paletteOpen = false;
  let cleanupShortcuts: (() => void) | null = null;
  type WorkspaceTab = ReturnType<typeof createWorkspaceTab>;
  let currentPath = '/';
  let currentView: IDEView = 'hl7';
  let currentWorkspaceTab: WorkspaceTab = createWorkspaceTab('/', 'system');

  const isMac = typeof navigator !== 'undefined' && /mac/i.test(navigator.platform);

  /** A shortcut in the platform's notation: ⌘B / ⇧⌘D on macOS, Ctrl+B elsewhere. */
  function shortcut(key: string, shift = false): string {
    if (isMac) return `${shift ? '⇧' : ''}⌘${key}`;
    return `Ctrl+${shift ? 'Shift+' : ''}${key}`;
  }

  const viewRoutes: Record<IDEView, IDEAppRoute> = {
    hl7: '/hl7',
    workflows: '/workflows',
    events: '/events',
    profiles: '/profiles',
    terminology: '/terminology',
    operator: '/operator',
    system: '/',
  };

  const routeToView: Record<string, IDEView> = {
    '/hl7': 'hl7',
    '/workflows': 'workflows',
    '/events': 'events',
    '/profiles': 'profiles',
    '/terminology': 'terminology',
    '/operator': 'operator',
    '/': 'system',
  };

  /** Navigate to a resolved path, bypassing SvelteKit typed route constraints. */
  function navigateTo(path: string): void {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any, svelte/no-navigation-without-resolve -- resolve() is called inside
    void (goto as any)((resolve as any)(path));
  }

  // ── Command palette commands ──

  const navCommands: PaletteCommand[] = [
    { id: 'nav:system', label: 'Go to Dashboard', hint: '/', category: 'Navigation', keywords: ['navigate', 'home', 'dashboard', 'health'], run: () => goto(resolve('/')) },
    { id: 'nav:hl7', label: 'Go to HL7 / Intake', hint: '/hl7', category: 'Navigation', keywords: ['navigate', 'hl7', 'source intake'], run: () => goto(resolve('/hl7')) },
    { id: 'nav:profiles', label: 'Go to Profiles', hint: '/profiles', category: 'Navigation', keywords: ['navigate', 'profiles', 'normalization'], run: () => goto(resolve('/profiles')) },
    { id: 'nav:terminology', label: 'Go to Terminology', hint: '/terminology', category: 'Navigation', keywords: ['navigate', 'terminology', 'translation'], run: () => goto(resolve('/terminology')) },
    { id: 'nav:workflows', label: 'Go to Workflows', hint: '/workflows', category: 'Navigation', keywords: ['navigate', 'workflows', 'delivery'], run: () => goto(resolve('/workflows')) },
    { id: 'nav:events', label: 'Go to Events', hint: '/events', category: 'Navigation', keywords: ['navigate', 'events', 'verification'], run: () => goto(resolve('/events')) },
    { id: 'nav:operator', label: 'Go to Operations', hint: '/operator', category: 'Navigation', keywords: ['navigate', 'operator', 'operations', 'replay', 'dead letter', 'deployments'], run: () => goto(resolve('/operator')) },
    { id: 'cmd:toggle-sidebar', label: 'Toggle sidebar', shortcut: shortcut('B'), category: 'Workspace', keywords: ['sidebar', 'context'], run: () => toggleSidebar() },
    { id: 'cmd:toggle-panel', label: 'Toggle bottom panel', shortcut: shortcut('J'), category: 'Workspace', keywords: ['panel', 'output', 'problems', 'copilot'], run: () => toggleBottomPanel() },
    { id: 'cmd:close-tab', label: 'Close editor tab', shortcut: shortcut('W'), category: 'Workspace', keywords: ['close', 'tab'], run: () => closeActiveTab() },
    { id: 'cmd:debug-panel', label: 'Open debug panel', shortcut: shortcut('D', true), category: 'Workspace', keywords: ['debug', 'breakpoint', 'step'], run: () => openPanelTab('debug') },
    { id: 'cmd:trace-panel', label: 'Open trace timeline', category: 'Workspace', keywords: ['trace', 'timeline', 'spans'], run: () => openPanelTab('trace') },
    { id: 'cmd:copilot', label: 'Open Copilot', category: 'Workspace', keywords: ['copilot', 'llm', 'assistant'], run: () => openPanelTab('copilot') },
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
    navigateTo(doc.path ?? doc.route ?? getWorkspaceTabRoute(doc.view ?? 'system'));
  }

  function closeTabById(closingTabId: string): void {
    const nextTabId = resolveNextWorkspaceTabId($ideState.documents, $ideState.activeDocumentId, closingTabId);
    const nextDoc = nextTabId ? $ideState.documents.find((d) => d.id === nextTabId) ?? null : null;
    const closingWasActive = closingTabId === $ideState.activeDocumentId;

    closeTabAction(closingTabId);

    if (!closingWasActive) return;

    if (nextDoc) {
      navigateTo(nextDoc.path ?? nextDoc.route ?? getWorkspaceTabRoute(nextDoc.view ?? 'system'));
      return;
    }

    navigateTo('/');
  }

  function onTabClose(e: CustomEvent<string>): void {
    closeTabById(e.detail);
  }

  function onPanelTabChange(e: CustomEvent<PanelTab>): void {
    // Clicking a tab reveals the panel (open-on-select), not just re-labels it.
    openPanelTab(e.detail);
  }

  function onPanelToggle(): void {
    toggleBottomPanel();
  }

  function onPanelNavigate(e: CustomEvent<{ panel: string }>): void {
    openPanelTab(e.detail.panel as import('./types').PanelTab);
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
    if (isHL7Route($page.url.pathname)) {
      // HL7 intake binds Cmd/Ctrl+K to its own, richer palette (its editor
      // commands). The header trigger hands the gesture to it instead of
      // opening a second palette.
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', metaKey: isMac, ctrlKey: !isMac }));
      return;
    }
    paletteOpen = true;
  }

  /** The active editor tab (a route document). */
  $: activeDocument = $ideState.documents.find((d) => d.id === $ideState.activeDocumentId) ?? null;

  // Breadcrumb: Stage ▸ Document (stage omitted off the stage routes).
  $: breadcrumbStage = getJourneyStage(currentPath);
  $: breadcrumbDocument = activeDocument?.title ?? currentWorkspaceTab.title;

  onMount(() => {
    cleanupShortcuts = initKeyboardShortcuts({
      toggleSidebar,
      toggleBottomPanel,
      closeTab: closeActiveTab,
      splitEditor: () => {
        toggleWorkspaceSplit();
      },
      openDebugPanel: () => {
        openPanelTab('debug');
      },
    });

    // Cmd/Ctrl+K opens the shell palette (HL7 intake opens its own).
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

    // The loom platform is an optional HUD integration (PUBLIC_LOOM_ENDPOINT).
    // Without it there is nothing to connect to, so nothing is started.
    if (PLATFORM_CONFIG.enabled) {
      void initializePlatform();
    }

    return () => {
      window.removeEventListener('keydown', onCmdK);
    };
  });

  onDestroy(() => {
    if (cleanupShortcuts) cleanupShortcuts();
    if (PLATFORM_CONFIG.enabled) {
      void teardownPlatform();
    }
  });
</script>

<CommandPalette
  bind:open={paletteOpen}
  title="Commands"
  commands={navCommands}
/>

<div class="ide-shell">
  <header class="ide-header">
    <a class="ide-brand" href={resolve('/')} aria-label="fi-fhir dashboard">fi-fhir</a>

    <StageControl pathname={currentPath} />

    <nav class="breadcrumb" aria-label="Breadcrumb">
      <ol>
        {#if breadcrumbStage}
          <li class="crumb crumb-stage">{breadcrumbStage.label}</li>
          <li class="crumb-sep" aria-hidden="true"><Icon icon={ChevronRight} size={12} /></li>
        {/if}
        <li class="crumb crumb-document" aria-current="page" title={breadcrumbDocument}>
          {breadcrumbDocument}
        </li>
      </ol>
    </nav>

    <div class="ide-header-right">
      <Button
        variant="ghost"
        icon={Search}
        class="command-trigger"
        aria-label="Open commands"
        title="Open commands ({shortcut('K')})"
        onclick={openPalette}
      >
        <span class="command-label">Commands</span>
        <kbd class="command-kbd">{shortcut('K')}</kbd>
      </Button>

      <ThemeToggle />
    </div>
  </header>

  <!-- Main body: activity bar + content + sidebar -->
  <div class="ide-body">
    <ActivityBar activeView={currentView} on:change={onViewChange} />

    <div class="ide-main">
      {#if $ideState.documents.length > 0}
        <EditorTabs
          tabs={$ideState.documents}
          activeTabId={$ideState.activeDocumentId}
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
          <!-- Primary pane -->
          <div class="workspace-pane ide-document">
            <slot />
          </div>

          <!-- Secondary pane -->
          <div slot="secondary" class="workspace-secondary">
            <Panel title="Split workspace" titleTag="h2">
              {#snippet actions()}
                <Button variant="ghost" onclick={toggleWorkspaceSplit}>Close split workspace</Button>
              {/snippet}
              <p class="split-copy">The second pane cannot show another route yet.</p>
            </Panel>
          </div>
        </SplitPane>
      {:else}
        <div class="ide-content ide-document">
          <slot />
        </div>
      {/if}

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

  <StatusBar
    {connectionState}
    {activeProfile}
    {parserStatus}
    {access}
    {onClearAccess}
    pathname={currentPath}
    platformEnabled={PLATFORM_CONFIG.enabled}
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
    font-family: var(--font-ui);
    font-size: var(--text-ui);
    color: var(--color-text-primary);
    background: var(--color-bg-base);
  }

  /* ── Header (40 px) ── */
  .ide-header {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    flex: 0 0 auto;
    height: var(--header-height, 40px);
    padding: 0 var(--space-2) 0 var(--space-3);
    background: var(--color-bg-elevated);
    border-bottom: 1px solid var(--color-border-subtle);
    z-index: var(--z-sticky);
  }

  .ide-brand {
    flex: 0 0 auto;
    color: var(--color-text-primary);
    font-family: var(--font-heading);
    font-size: var(--text-lg);
    font-weight: var(--font-bold);
    letter-spacing: var(--tracking-tight);
    text-decoration: none;
  }

  .ide-brand:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 2px;
    border-radius: var(--radius-sm);
  }

  .breadcrumb {
    flex: 1 1 auto;
    min-width: 0;
  }

  .breadcrumb ol {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
    min-width: 0;
  }

  .crumb {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .crumb-stage {
    flex: 0 0 auto;
    color: var(--color-text-tertiary);
  }

  .crumb-sep {
    display: inline-flex;
    color: var(--color-text-muted);
  }

  .crumb-document {
    color: var(--color-text-primary);
    font-weight: var(--font-medium);
  }

  .ide-header-right {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 0 0 auto;
    margin-left: auto;
  }

  .ide-header-right :global(.command-trigger) {
    gap: var(--space-2);
    padding: 0 6px 0 8px;
  }

  .command-label {
    color: var(--color-text-secondary);
    font-weight: var(--font-normal);
  }

  .command-kbd {
    padding: 2px 5px;
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    color: var(--color-text-tertiary);
    font-family: var(--font-mono);
    font-size: var(--text-label);
    line-height: 1;
  }

  /* ── Body (activity bar + main + sidebar) ── */
  .ide-body {
    display: flex;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }

  .ide-main {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
  }

  /*
   * Document region. Routes that follow the page pattern open with a
   * `Toolbar` and own their edges, so the region drops its padding for them;
   * routes not yet on the pattern keep a 12 px inset.
   */
  .ide-content {
    flex: 1;
    min-height: 0;
    overflow: auto;
  }

  .ide-document {
    padding: var(--space-3);
  }

  .ide-document:has(:global(.ui-toolbar)) {
    padding: 0;
  }

  .workspace-pane {
    height: 100%;
    min-width: 0;
    min-height: 0;
    overflow: auto;
  }

  .workspace-secondary {
    display: grid;
    align-content: start;
    gap: var(--space-3);
    height: 100%;
    min-width: 0;
    min-height: 0;
    padding: var(--space-3);
    overflow: auto;
    background: var(--color-bg-base);
    border-left: 1px solid var(--color-border-subtle);
  }

  .split-copy {
    margin: 0;
    color: var(--color-text-secondary);
    font-size: var(--text-xs);
  }

  /* ── Narrow windows: collapse the header's secondary text ── */
  @media (max-width: 960px) {
    .breadcrumb {
      display: none;
    }

    .command-label,
    .command-kbd {
      display: none;
    }
  }

  @media (max-width: 768px) {
    .ide-document {
      padding: var(--space-2);
    }
  }

  /* Phones: tighter header gaps keep the stage control on one line. */
  @media (max-width: 640px) {
    .ide-header {
      gap: var(--space-2);
      padding: 0 var(--space-1) 0 var(--space-2);
    }

    .ide-header-right {
      gap: var(--space-1);
    }
  }
</style>
