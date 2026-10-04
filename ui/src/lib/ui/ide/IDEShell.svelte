<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte';
  import { resolve } from '$app/paths';
  import { afterNavigate, beforeNavigate, goto } from '$app/navigation';
  import type { AfterNavigate, BeforeNavigate } from '@sveltejs/kit';
  import { toasts } from '$lib/ui/toastStore';
  import { page } from '$app/stores';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import Search from '@lucide/svelte/icons/search';
  import PanelLeft from '@lucide/svelte/icons/panel-left';
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
    setSidebarOpen,
    setActiveView,
    openTab as openTabAction,
    closeTab as closeTabAction,
    toggleBottomPanel,
    openPanelTab,
    setDraftState,
    clearDraftState,
    hasDraftsToLose,
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
   * document region, the bottom panel, the workspace explorer and a 24 px
   * status bar — the one place for connection and access state (plus
   * Next: stage and the build).
   *
   * It owns the one command palette (commandRegistry.ts: the shell's commands
   * plus whatever the current route registers), reads the journey evidence on
   * mount and on every route change (journeyState.ts), and asks before closing
   * leaving a view that would lose edits, or closing a dirty tab.
   */

  export let connectionState: ConnectionState = 'disconnected';
  /** Credential state from GraphQLCredentialGate; rendered as the status-bar chip. */
  export let access: AccessSession | null = null;
  /** Ends a bearer session (GraphQLCredentialGate.clearCredential). */
  export let onClearAccess: (() => void) | undefined = undefined;

  let cleanupShortcuts: (() => void) | null = null;
  let cleanupCommands: (() => void) | null = null;
  type PendingNavigation = {
    kind: 'navigate';
    title: string;
    from: string;
    to: string;
    type: BeforeNavigate['type'];
    delta?: number | undefined;
    willUnload: boolean;
    connectionsIntent?: ConnectionsIntent | undefined;
  };
  type PendingAction = PendingNavigation | { kind: 'close'; id: string; title: string; restoreTabFocus: boolean };
  let pendingAction: PendingAction | null = null;
  let navigationPermit: { from: string; to: string; type: BeforeNavigate['type']; delta?: number | undefined } | null = null;
  let permitTimer: ReturnType<typeof setTimeout> | undefined;
  let cancelHistoryWait: (() => void) | undefined;
  let closingTab: string | null = null;
  let editorTabs: EditorTabs | undefined;
  let navigationArrival = 0;
  let navigationSequence = 0;
  let lastNavigation: AfterNavigate | undefined;
  let navigationRequest: { from: string; to: string; intent: ConnectionsIntent } | null = null;

  afterNavigate((navigation) => {
    navigationArrival += 1;
    lastNavigation = navigation;
  });

  function clearNavigationPermit(): void {
    navigationPermit = null;
    clearTimeout(permitTimer);
  }

  function permitNavigation(to: string, type: BeforeNavigate['type'], delta?: number): void {
    clearNavigationPermit();
    navigationPermit = { from: $page.url.href, to, type, delta };
    // A history traversal can be ignored by the browser. Never leave approval
    // armed for an unrelated future attempt, even when no hook fires.
    permitTimer = setTimeout(clearNavigationPermit, 3000);
  }

  beforeNavigate((navigation) => {
    if (navigation.type === 'leave') {
      if (hasDraftsToLose(currentWorkspaceTab.id, true)) navigation.cancel();
      return;
    }
    if (!navigation.to) return;
    const approved = navigationPermit
      && navigationPermit.from === navigation.from?.url.href
      && navigationPermit.to === navigation.to.url.href
      && navigationPermit.type === navigation.type
      && navigationPermit.delta === navigation.delta;
    clearNavigationPermit();
    if (approved) return;
    if (pendingAction) {
      navigation.cancel();
      return;
    }
    if (!navigation.willUnload && normalizeRoute(navigation.to.url.pathname) === currentPath) return;
    if (!hasDraftsToLose(currentWorkspaceTab.id, navigation.willUnload)) return;

    navigation.cancel();
    pendingAction = {
      kind: 'navigate', title: currentWorkspaceTab.title,
      from: navigation.from?.url.href ?? $page.url.href,
      to: navigation.to.url.href, type: navigation.type,
      delta: navigation.delta, willUnload: navigation.willUnload,
      connectionsIntent: navigationRequest?.from === navigation.from?.url.href && navigationRequest?.to === navigation.to.url.href
        ? navigationRequest.intent : undefined,
    };
  });
  type WorkspaceTab = ReturnType<typeof createWorkspaceTab>;
  let narrowScreen = false;
  let drawerOpen = false;
  $: explorerOpen = narrowScreen ? drawerOpen : $ideState.sidebarOpen;

  async function toggleExplorer(): Promise<void> {
    if (explorerOpen) closeExplorer();
    else if (narrowScreen) drawerOpen = true;
    else {
      toggleSidebar();
      await tick();
      if (!narrowScreen && $ideState.sidebarOpen) {
        document.querySelector<HTMLInputElement>('#workspace-explorer input')?.focus();
      }
    }
  }

  function closeExplorer(): void {
    if (narrowScreen) drawerOpen = false;
    else {
      setSidebarOpen(false);
      document.getElementById('explorer-toggle')?.focus();
    }
  }

  function explore(path: string): void {
    navigateTo(path);
    if (narrowScreen) drawerOpen = false;
  }

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

  /** Publish commands only after arrival; cancelled navigation has no side effects. */
  function openConnections(intent: ConnectionsIntent): void {
    void navigateTo('/connections', intent);
  }

  /** Navigate to a resolved path, bypassing SvelteKit typed route constraints. */
  async function navigateTo(path: string, intent?: ConnectionsIntent): Promise<boolean> {
    const sequence = ++navigationSequence;
    let request: typeof navigationRequest = null;
    try {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any -- paths are validated workspace routes with optional selectors
      const destination = (resolve as any)(path) as string;
      const target = new URL(destination, $page.url);
      request = intent ? { from: $page.url.href, to: target.href, intent } : null;
      navigationRequest = request;
      // eslint-disable-next-line svelte/no-navigation-without-resolve -- resolved above
      await goto(destination);
      await tick();
      // SvelteKit resolves goto after a cancelled transition too.
      const arrived = sequence === navigationSequence && $page.url.href === target.href;
      if (arrived && intent) requestConnectionsView(intent);
      return arrived;
    } catch {
      toasts.error('Could not open that view. Your current work is still open.');
      return false;
    } finally {
      if (navigationRequest === request) navigationRequest = null;
      clearNavigationPermit();
    }
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
    { id: 'cmd:toggle-sidebar', label: 'Toggle explorer', shortcut: shortcut('B'), group: 'Workspace', keywords: ['sidebar', 'drawer', 'navigation'], run: () => toggleExplorer() },
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
  $: setDraftState('/profiles', 'profile-builder', $profileDraftDirty, false);

  function onViewChange(e: CustomEvent<IDEView>): void {
    openView(e.detail);
  }

  function onTabSelect(e: CustomEvent<string>): void {
    const doc = $ideState.documents.find((entry) => entry.id === e.detail);
    if (!doc) return;
    navigateTo(doc.path ?? doc.route ?? getWorkspaceTabRoute(doc.view ?? 'system'));
  }

  /** Closing and route departure share one confirmation, before tab mutation. */
  function requestCloseTab(closingTabId: string): void {
    if (pendingAction || closingTab) return;
    const doc = $ideState.documents.find((entry) => entry.id === closingTabId);
    if (!doc) return;
    const restoreTabFocus = editorTabs?.focusedTabId() === closingTabId;
    if (doc.dirty) {
      pendingAction = { kind: 'close', id: doc.id, title: doc.title, restoreTabFocus };
      return;
    }
    void closeTabById(closingTabId, false, restoreTabFocus);
  }

  function waitForHistoryRestore(from: string): Promise<boolean> {
    if (window.location.href === from) return Promise.resolve(true);
    return new Promise((resolveWait) => {
      const finish = (restored: boolean) => {
        clearTimeout(timeout);
        window.removeEventListener('popstate', onPop);
        cancelHistoryWait = undefined;
        resolveWait(restored);
      };
      const onPop = () => {
        if (window.location.href === from) finish(true);
      };
      const timeout = setTimeout(() => finish(false), 1000);
      cancelHistoryWait = () => finish(false);
      window.addEventListener('popstate', onPop);
    });
  }

  async function confirmPendingAction(): Promise<void> {
    const target = pendingAction;
    pendingAction = null;
    await tick();
    if (!target) return;
    if (target.kind === 'close') {
      await closeTabById(target.id, true, target.restoreTabFocus);
      return;
    }
    if ($page.url.href !== target.from) return;
    if (target.type === 'popstate' && target.delta !== undefined) {
      // SvelteKit first rolls the cancelled traversal back. Resume the same
      // history delta, preserving Forward and existing entry state.
      if (!await waitForHistoryRestore(target.from)) {
        toasts.error('Navigation did not complete. Your edits are still open.');
        return;
      }
      permitNavigation(target.to, 'popstate', target.delta);
      window.history.go(target.delta);
    } else if (target.willUnload) {
      // Let SvelteKit handle the confirmed link so its native-unload path
      // does not ask a second time.
      permitNavigation(target.to, 'link');
      const link = document.createElement('a');
      link.href = target.to;
      document.body.append(link);
      link.click();
      link.remove();
    } else {
      permitNavigation(target.to, 'goto');
      const url = new URL(target.to);
      await navigateTo(url.pathname + url.search + url.hash, target.connectionsIntent);
    }
  }

  async function closeTabById(closingTabId: string, approved = false, restoreTabFocus = false): Promise<void> {
    const documents = $ideState.documents;
    const index = documents.findIndex((doc) => doc.id === closingTabId);
    if (index < 0) return;
    const sourceLocation = $page.url.href;
    const sourceArrival = navigationArrival;
    // Focus follows the removed tab's neighbor; closing an inactive tab must
    // leave the active route alone, even when that is a different survivor.
    const neighbors = [documents[index + 1]?.id, documents[index - 1]?.id];
    closingTab = closingTabId;
    try {
      if (closingTabId === $ideState.activeDocumentId) {
        const nextTabId = resolveNextWorkspaceTabId(documents, $ideState.activeDocumentId, closingTabId);
        const nextDoc = documents.find((doc) => doc.id === nextTabId);
        const destination = nextDoc?.path ?? nextDoc?.route ?? '/';
        if (approved) permitNavigation(new URL(destination, $page.url).href, 'goto');
        const navigation = navigateTo(destination);
        const closeNavigation = navigationSequence;
        const opened = await navigation;
        // URL equality alone cannot identify the close's arrival: a newer link,
        // history entry, or shell request may have reached the same destination.
        const ownsArrival = navigationSequence === closeNavigation
          && navigationArrival === sourceArrival + 1
          && lastNavigation?.type === 'goto'
          && lastNavigation.from?.url.href === sourceLocation
          && lastNavigation.to?.url.href === $page.url.href;
        if (!opened || !ownsArrival) {
          if (restoreTabFocus) {
            await tick();
            // A newer navigation can supersede this close while goto waits.
            // Its page and focus now belong to that later user action.
            if (navigationSequence === closeNavigation && navigationArrival === sourceArrival && $ideState.activeDocumentId === closingTabId && $page.url.href === sourceLocation) {
              editorTabs?.focusTab(closingTabId);
            }
          }
          return;
        }
      }
      if (!$ideState.documents.some((doc) => doc.id === closingTabId)) return;
      closeTabAction(closingTabId);
      // Home remains the fallback when the final editor is closed.
      if (!$ideState.documents.length) openTabAction(createWorkspaceTab('/', 'system'));
      await tick();
      if (restoreTabFocus) {
        const target = neighbors.find((id) => $ideState.documents.some((doc) => doc.id === id))
          ?? $ideState.activeDocumentId;
        if (target) editorTabs?.focusTab(target);
      }
    } finally {
      closingTab = null;
    }
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
    const media = window.matchMedia('(max-width: 960px)');
    const updateViewport = () => {
      narrowScreen = media.matches;
      drawerOpen = false;
    };
    updateViewport();
    media.addEventListener('change', updateViewport);
    cleanupShortcuts = initKeyboardShortcuts({
      toggleSidebar: () => {
        if (!document.querySelector('[aria-modal="true"]') || drawerOpen) toggleExplorer();
      },
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
      media.removeEventListener('change', updateViewport);
    };
  });

  onDestroy(() => {
    clearNavigationPermit();
    cancelHistoryWait?.();
    clearDraftState('/profiles', 'profile-builder');
    if (cleanupShortcuts) cleanupShortcuts();
    if (cleanupCommands) cleanupCommands();
    if (PLATFORM_CONFIG.enabled) {
      void teardownPlatform();
    }
  });
</script>

<CommandPalette />

<Dialog
  open={pendingAction !== null}
  title={pendingAction ? `${pendingAction.kind === 'close' ? 'Close' : 'Leave'} ${pendingAction.title}?` : 'Unsaved changes'}
  description={pendingAction
    ? pendingAction.kind === 'close'
      ? `${pendingAction.title} has changes that are not published yet.`
      : 'Leaving this view will discard its unsaved edits.'
    : undefined}
  size="sm"
  onclose={() => (pendingAction = null)}
  data-testid="draft-navigation-dialog"
>
  {#snippet footer()}
    <Button variant="ghost" size="md" onclick={() => (pendingAction = null)}>{pendingAction?.kind === 'close' ? 'Keep open' : 'Stay here'}</Button>
    <Button size="md" onclick={confirmPendingAction}>{pendingAction?.kind === 'close' ? 'Close tab' : 'Leave view'}</Button>
  {/snippet}
</Dialog>

<div class="ide-shell">
  <header class="ide-header">
    <a class="ide-brand" href={resolve('/')} aria-label="fi-fhir dashboard">fi-fhir</a>

    <Button
      id="explorer-toggle"
      variant="ghost"
      icon={PanelLeft}
      aria-label={explorerOpen ? 'Hide explorer' : 'Show explorer'}
      aria-expanded={explorerOpen}
      aria-controls="workspace-explorer"
      title="Explorer ({shortcut('B')})"
      onclick={toggleExplorer}
    >
      <span class="explorer-label">Explorer</span>
    </Button>

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

  <!-- Main body: explorer (or compact activity bar) + content -->
  <div class="ide-body">
    {#if !explorerOpen && !narrowScreen}
      <ActivityBar activeView={currentView} on:change={onViewChange} />
    {/if}
    <Sidebar
      open={explorerOpen}
      drawer={narrowScreen}
      width={$ideState.sidebarWidth}
      onclose={closeExplorer}
      onnavigate={explore}
    />

    <div class="ide-main">
      {#if $ideState.documents.length > 0}
        <EditorTabs
          bind:this={editorTabs}
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

    .explorer-label,
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
