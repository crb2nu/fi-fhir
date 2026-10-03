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
  import CommandPalette from './CommandPalette.svelte';
  import {
    openPalette,
    paletteOpen,
    registerCommands,
    type Command
  } from './commandRegistry';
  import { refreshJourneyEvidence } from './journeyState';
  import type { AccessSession } from '$lib/graphql/GraphQLCredentialGate.svelte';
  import { isDirty as profileDraftDirty } from '$lib/features/hl7/profile/profileStore';
  import {
    requestConnectionsView,
    type ConnectionsIntent
  } from '$lib/features/connections/connectionsIntent';
  import { Button, Dialog, Icon } from '$lib/ui/primitives';
  import {
    ideState,
    toggleSidebar,
    setActiveView,
    openTab as openTabAction,
    closeTab as closeTabAction,
    setActiveTab,
    toggleBottomPanel,
    openPanelTab,
    markDirty,
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
   *
   * It owns the one command palette (commandRegistry.ts: the shell's commands
   * plus whatever the current route registers), reads the journey evidence on
   * mount and on every route change (journeyState.ts), and asks before closing
   * a tab that holds unsaved changes (ideStore.markDirty).
   */

  export let connectionState: ConnectionState = 'disconnected';
  /** Credential state from GraphQLCredentialGate; rendered as the status-bar chip. */
  export let access: AccessSession | null = null;
  /** Ends a bearer session (GraphQLCredentialGate.clearCredential). */
  export let onClearAccess: (() => void) | undefined = undefined;

  let cleanupShortcuts: (() => void) | null = null;
  let cleanupCommands: (() => void) | null = null;
  /** The dirty tab a close is waiting on (the confirmation dialog's subject). */
  let pendingClose: { id: string; title: string } | null = null;
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
    connections: '/connections',
    operator: '/operator',
    system: '/',
  };

  const routeToView: Record<string, IDEView> = {
    '/hl7': 'hl7',
    '/workflows': 'workflows',
    '/events': 'events',
    '/profiles': 'profiles',
    '/terminology': 'terminology',
    '/connections': 'connections',
    '/operator': 'operator',
    '/': 'system',
  };

  /** Records what the Connections page should open, then goes there. */
  function openConnections(intent: ConnectionsIntent): void {
    requestConnectionsView(intent);
    void goto(resolve('/connections'));
  }

  /** Navigate to a resolved path, bypassing SvelteKit typed route constraints. */
  function navigateTo(path: string): void {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any, svelte/no-navigation-without-resolve -- resolve() is called inside
    void (goto as any)((resolve as any)(path));
  }

  // ── Shell commands (the palette also lists the current route's) ──

  const shellCommands: Command[] = [
    { id: 'nav:system', label: 'Go to Home', hint: '/', group: 'Navigation', keywords: ['navigate', 'home', 'dashboard', 'health'], run: () => openView('system') },
    { id: 'nav:hl7', label: 'Go to HL7 / Intake', hint: '/hl7', group: 'Navigation', keywords: ['navigate', 'hl7', 'source intake'], run: () => openView('hl7') },
    { id: 'nav:profiles', label: 'Go to Profiles', hint: '/profiles', group: 'Navigation', keywords: ['navigate', 'profiles', 'normalization'], run: () => openView('profiles') },
    { id: 'nav:terminology', label: 'Go to Terminology', hint: '/terminology', group: 'Navigation', keywords: ['navigate', 'terminology', 'translation'], run: () => openView('terminology') },
    { id: 'nav:workflows', label: 'Go to Workflows', hint: '/workflows', group: 'Navigation', keywords: ['navigate', 'workflows', 'delivery'], run: () => openView('workflows') },
    { id: 'nav:events', label: 'Go to Verification', hint: '/events', group: 'Navigation', keywords: ['navigate', 'events', 'verification'], run: () => openView('events') },
    { id: 'nav:connections', label: 'Go to Connections', hint: '/connections', group: 'Navigation', keywords: ['navigate', 'connections', 'sources', 'destinations', 'engine'], run: () => openView('connections') },
    { id: 'nav:operator', label: 'Go to Operator', hint: '/operator', group: 'Navigation', keywords: ['navigate', 'operator', 'operations', 'replay', 'dead letter', 'deployments'], run: () => openView('operator') },
    { id: 'cmd:new-source-connection', label: 'New source connection', hint: '/connections', group: 'Connections', keywords: ['connection', 'source', 'mllp', 'http', 'batch', 's3', 'sftp', 'create'], run: () => openConnections({ view: 'sources', openNew: true }) },
    { id: 'cmd:new-destination-connection', label: 'New destination connection', hint: '/connections', group: 'Connections', keywords: ['connection', 'destination', 'https', 'fhir', 'kafka', 'create'], run: () => openConnections({ view: 'destinations', openNew: true }) },
    { id: 'cmd:engine-properties', label: 'Engine properties', hint: '/connections', group: 'Connections', keywords: ['engine', 'runtime', 'adapters', 'properties', 'environment', 'ledgers'], run: () => openConnections({ view: 'engine' }) },
    { id: 'cmd:toggle-sidebar', label: 'Toggle sidebar', shortcut: shortcut('B'), group: 'Workspace', keywords: ['sidebar', 'context'], run: () => toggleSidebar() },
    { id: 'cmd:toggle-panel', label: 'Toggle bottom panel', shortcut: shortcut('J'), group: 'Workspace', keywords: ['panel', 'output', 'problems', 'copilot'], run: () => toggleBottomPanel() },
    { id: 'cmd:close-tab', label: 'Close editor tab', shortcut: shortcut('W'), group: 'Workspace', keywords: ['close', 'tab'], when: () => $ideState.activeDocumentId !== null, run: () => closeActiveTab() },
    { id: 'cmd:debug-panel', label: 'Open debug panel', shortcut: shortcut('D', true), group: 'Workspace', keywords: ['debug', 'breakpoint', 'step'], run: () => openPanelTab('debug') },
    { id: 'cmd:trace-panel', label: 'Open trace timeline', group: 'Workspace', keywords: ['trace', 'timeline', 'spans'], run: () => openPanelTab('trace') },
    { id: 'cmd:copilot', label: 'Open Copilot', group: 'Workspace', keywords: ['copilot', 'llm', 'assistant'], run: () => openPanelTab('copilot') },
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

  function openView(view: IDEView): void {
    const doc = $ideState.documents.find((entry) => entry.view === view);
    navigateTo(doc?.path ?? getWorkspaceTabRoute(view));
  }

  $: currentPath = normalizeRoute($page.url.pathname);
  $: currentView = detectViewFromPath(currentPath);
  $: currentWorkspaceTab = createWorkspaceTab(currentPath, currentView, $page.url.search);
  $: setActiveView(currentView);
  $: openTabAction(currentWorkspaceTab);

  // Journey evidence: read once per mount and again on every route change
  // that enters or leaves Home or a stage route. Moving between routes
  // outside the journey (Operator ↔ Connections) keeps the last answer.
  let evidencePath: string | null = null;
  $: if (currentPath !== evidencePath) {
    const previous = evidencePath;
    evidencePath = currentPath;
    if (previous === null || onJourney(previous) || onJourney(currentPath)) void refreshJourneyEvidence();
  }

  function onJourney(pathname: string): boolean {
    return pathname === '/' || getJourneyStage(pathname) !== null;
  }

  // Unsaved drafts the shell can see without the feature's help: the source
  // profile draft (shared by Profiles and HL7 intake's Profile draft tab) is
  // published from Profiles, so that tab carries the mark.
  $: markDirty('/profiles', $profileDraftDirty);

  function onViewChange(e: CustomEvent<IDEView>): void {
    openView(e.detail);
  }

  function onTabSelect(e: CustomEvent<string>): void {
    const doc = $ideState.documents.find((entry) => entry.id === e.detail);
    if (!doc) return;
    setActiveTab(doc.id);
    navigateTo(doc.path ?? doc.route ?? getWorkspaceTabRoute(doc.view ?? 'system'));
  }

  /** Closes a tab, asking first when it holds unsaved changes. */
  function requestCloseTab(closingTabId: string): void {
    const doc = $ideState.documents.find((entry) => entry.id === closingTabId);
    if (doc?.dirty) {
      pendingClose = { id: doc.id, title: doc.title };
      return;
    }
    closeTabById(closingTabId);
  }

  function confirmPendingClose(): void {
    const target = pendingClose;
    pendingClose = null;
    if (target) closeTabById(target.id);
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
    requestCloseTab(e.detail);
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
      requestCloseTab(state.activeDocumentId);
    }
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
      openDebugPanel: () => {
        openPanelTab('debug');
      },
    });
    cleanupCommands = registerCommands('shell', shellCommands);

    // Capture before CodeMirror handles Ctrl+K, so commands remain available
    // while editing. Other modal dialogs keep their own keyboard context.
    const onCmdK = (e: KeyboardEvent) => {
      if (e.defaultPrevented || e.isComposing || e.altKey || e.shiftKey) return;
      const mod = e.metaKey || e.ctrlKey;
      if (mod && (e.key === 'k' || e.key === 'K')) {
        if (!$paletteOpen && document.querySelector('[aria-modal="true"]')) return;
        e.preventDefault();
        if (!$paletteOpen) openPalette();
      }
    };

    window.addEventListener('keydown', onCmdK, true);

    // The loom platform is an optional HUD integration (PUBLIC_LOOM_ENDPOINT).
    // Without it there is nothing to connect to, so nothing is started.
    if (PLATFORM_CONFIG.enabled) {
      void initializePlatform();
    }

    return () => {
      window.removeEventListener('keydown', onCmdK, true);
    };
  });

  onDestroy(() => {
    if (cleanupShortcuts) cleanupShortcuts();
    if (cleanupCommands) cleanupCommands();
    if (PLATFORM_CONFIG.enabled) {
      void teardownPlatform();
    }
  });
</script>

<CommandPalette />

<Dialog
  open={pendingClose !== null}
  title={pendingClose ? `Close ${pendingClose.title}?` : 'Close tab?'}
  description={pendingClose
    ? `${pendingClose.title} has changes that are not published yet.`
    : undefined}
  size="sm"
  onclose={() => (pendingClose = null)}
  data-testid="close-dirty-tab-dialog"
>
  {#snippet footer()}
    <Button variant="ghost" size="md" onclick={() => (pendingClose = null)}>Keep open</Button>
    <Button size="md" onclick={confirmPendingClose}>Close tab</Button>
  {/snippet}
</Dialog>

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

      <div class="ide-content ide-document">
        <slot />
      </div>

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
