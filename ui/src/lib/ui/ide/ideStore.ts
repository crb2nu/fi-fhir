/**
 * IDE state store with localStorage persistence for layout dimensions and full layout state.
 *
 * M2: Introduces WorkspaceDocument model. Route-type documents behave
 * identically to the former EditorTab. Non-route documents (workflow-draft,
 * debug-session, trace, event, profile) open as artifact tabs.
 */
import { writable, derived, get } from 'svelte/store';
import type {
  IDEState,
  IDEView,
  WorkspaceDocument,
  DocumentType,
  PanelTab,
  IDEAppRoute,
} from './types';

const SIDEBAR_WIDTH_KEY = 'fi-fhir-ide-sidebar-width';
const BOTTOM_PANEL_HEIGHT_KEY = 'fi-fhir-ide-bottom-panel-height';
const LAYOUT_KEY = 'fi-fhir-ide-layout';

/**
 * Serializable subset of IDE state for layout persistence.
 */
interface PersistedLayout {
  openTabs: WorkspaceDocument[];
  activeTabId: string | null;
  workspaceSplit: boolean;
  bottomPanelOpen: boolean;
  activePanelTab: PanelTab;
  activeView: IDEView;
}

const VALID_PANEL_TABS = new Set<PanelTab>(['output', 'problems', 'debug', 'trace', 'copilot']);
const VALID_VIEWS = new Set<IDEView>(['hl7', 'workflows', 'events', 'profiles', 'terminology', 'system']);

function loadLayout(): PersistedLayout | null {
  if (typeof window === 'undefined') return null;
  try {
    const raw = localStorage.getItem(LAYOUT_KEY);
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object') return null;
    const obj = parsed as Record<string, unknown>;

    // Validate required fields
    if (!Array.isArray(obj['openTabs'])) return null;
    if (typeof obj['activeTabId'] !== 'string' && obj['activeTabId'] !== null) return null;
    if (typeof obj['workspaceSplit'] !== 'boolean') return null;
    if (typeof obj['bottomPanelOpen'] !== 'boolean') return null;
    if (typeof obj['activePanelTab'] !== 'string' || !VALID_PANEL_TABS.has(obj['activePanelTab'] as PanelTab)) return null;
    if (typeof obj['activeView'] !== 'string' || !VALID_VIEWS.has(obj['activeView'] as IDEView)) return null;

    return {
      openTabs: obj['openTabs'] as WorkspaceDocument[],
      activeTabId: obj['activeTabId'] as string | null,
      workspaceSplit: obj['workspaceSplit'] as boolean,
      bottomPanelOpen: obj['bottomPanelOpen'] as boolean,
      activePanelTab: obj['activePanelTab'] as PanelTab,
      activeView: obj['activeView'] as IDEView,
    };
  } catch {
    return null;
  }
}

function saveLayout(state: IDEState): void {
  if (typeof window === 'undefined') return;
  try {
    const layout: PersistedLayout = {
      openTabs: state.openTabs,
      activeTabId: state.activeTabId,
      workspaceSplit: state.workspaceSplit,
      bottomPanelOpen: state.bottomPanelOpen,
      activePanelTab: state.activePanelTab,
      activeView: state.activeView,
    };
    localStorage.setItem(LAYOUT_KEY, JSON.stringify(layout));
  } catch {
    // Ignore storage errors
  }
}

let _layoutRestored = false;

// Domain-first editor tab titles (match the ActivityBar labels, Slice 3). The
// journey metaphor (Source Intake / Delivery / …) lives in the journey panel and
// the per-stage sidebar headings, not in the nav chrome.
const WORKSPACE_ROUTE_TITLES: Record<IDEView, string> = {
  system: 'Dashboard',
  hl7: 'HL7 / Intake',
  workflows: 'Workflows',
  events: 'Events',
  profiles: 'Profiles',
  terminology: 'Terminology',
};

const WORKSPACE_VIEW_ROUTES: Record<IDEView, IDEAppRoute> = {
  system: '/',
  hl7: '/hl7',
  workflows: '/workflows',
  events: '/events',
  profiles: '/profiles',
  terminology: '/terminology',
};

function normalizeWorkspacePathname(pathname: string): string {
  if (!pathname) return '/';
  if (pathname.length > 1 && pathname.endsWith('/')) {
    return pathname.replace(/\/+$/, '');
  }
  return pathname;
}

function workspaceViewForPath(pathname: string): IDEView {
  const normalized = normalizeWorkspacePathname(pathname);
  if (normalized === '/') return 'system';
  if (normalized.startsWith('/events')) return 'events';
  if (normalized.startsWith('/hl7')) return 'hl7';
  if (normalized.startsWith('/profiles')) return 'profiles';
  if (normalized.startsWith('/terminology')) return 'terminology';
  if (normalized.startsWith('/workflows')) return 'workflows';
  return 'system';
}

export function getWorkspaceTabTitle(pathname: string, view?: IDEView): string {
  const workspaceView = view ?? workspaceViewForPath(pathname);
  return WORKSPACE_ROUTE_TITLES[workspaceView];
}

/** Create a route-type workspace document (backward compat with createWorkspaceTab). */
export function createWorkspaceTab(pathname: string, view?: IDEView): WorkspaceDocument {
  const normalized = normalizeWorkspacePathname(pathname);
  const workspaceView = view ?? workspaceViewForPath(normalized);
  const workspaceRoute = WORKSPACE_VIEW_ROUTES[workspaceView];
  return {
    id: workspaceRoute,
    type: 'route',
    title: getWorkspaceTabTitle(normalized, workspaceView),
    dirty: false,
    view: workspaceView,
    path: workspaceRoute,
    route: workspaceRoute,
  };
}

/** Create an artifact-backed workspace document. */
export function createDocument(
  type: DocumentType,
  title: string,
  opts?: { subtitle?: string; artifactId?: string; id?: string }
): WorkspaceDocument {
  const id = opts?.id ?? `${type}:${opts?.artifactId ?? crypto.randomUUID().slice(0, 8)}`;
  return {
    id,
    type,
    title,
    subtitle: opts?.subtitle,
    artifactId: opts?.artifactId,
    dirty: false,
  };
}

export function resolveNextWorkspaceTabId(
  tabs: WorkspaceDocument[],
  activeTabId: string | null,
  closingTabId: string
): string | null {
  const idx = tabs.findIndex((tab) => tab.id === closingTabId);
  if (idx < 0) return activeTabId;

  const next = tabs.filter((tab) => tab.id !== closingTabId);
  if (tabs.length === 0) return null;

  if (activeTabId !== closingTabId) {
    return activeTabId;
  }

  if (next.length === 0) {
    return null;
  }

  if (idx >= next.length) {
    return next[next.length - 1]?.id ?? null;
  }

  return next[idx]?.id ?? null;
}

function loadNumber(key: string, fallback: number): number {
  if (typeof window === 'undefined') return fallback;
  try {
    const raw = localStorage.getItem(key);
    if (raw === null) return fallback;
    const val = Number(raw);
    return Number.isFinite(val) ? val : fallback;
  } catch {
    return fallback;
  }
}

function saveNumber(key: string, value: number): void {
  if (typeof window === 'undefined') return;
  try {
    localStorage.setItem(key, String(value));
  } catch {
    // Ignore storage errors
  }
}

/** Internal state shape uses documents/activeDocumentId. */
interface InternalIDEState {
  sidebarOpen: boolean;
  sidebarWidth: number;
  activeView: IDEView;
  documents: WorkspaceDocument[];
  activeDocumentId: string | null;
  secondaryDocumentId: string | null;
  workspaceSplit: boolean;
  bottomPanelOpen: boolean;
  bottomPanelHeight: number;
  activePanelTab: PanelTab;
}

function createInitialState(): InternalIDEState {
  return {
    sidebarOpen: false,
    sidebarWidth: loadNumber(SIDEBAR_WIDTH_KEY, 280),
    activeView: 'hl7',
    documents: [],
    activeDocumentId: null,
    secondaryDocumentId: null,
    workspaceSplit: false,
    bottomPanelOpen: false,
    bottomPanelHeight: loadNumber(BOTTOM_PANEL_HEIGHT_KEY, 200),
    activePanelTab: 'output',
  };
}

const _store = writable<InternalIDEState>(createInitialState());

/**
 * Public ideState derived store that exposes the full IDEState interface
 * including backward-compat aliases (openTabs, activeTabId).
 */
export const ideState = derived(_store, ($s): IDEState => ({
  ...$s,
  openTabs: $s.documents,
  activeTabId: $s.activeDocumentId,
}));

// ---------------------------------------------------------------------------
// Sidebar / layout actions
// ---------------------------------------------------------------------------

// Persist layout on every state change
ideState.subscribe((state) => {
  saveLayout(state);
});

export function toggleSidebar(): void {
  _store.update((s) => ({ ...s, sidebarOpen: !s.sidebarOpen }));
}

export function setSidebarWidth(width: number): void {
  saveNumber(SIDEBAR_WIDTH_KEY, width);
  _store.update((s) => ({ ...s, sidebarWidth: width }));
}

export function setActiveView(view: IDEView): void {
  _store.update((s) => ({ ...s, activeView: view }));
}

export function toggleWorkspaceSplit(): void {
  _store.update((s) => {
    const next = !s.workspaceSplit;
    return {
      ...s,
      workspaceSplit: next,
      secondaryDocumentId: next ? s.activeDocumentId : null,
    };
  });
}

export function setWorkspaceSplit(enabled: boolean): void {
  _store.update((s) => ({
    ...s,
    workspaceSplit: enabled,
    secondaryDocumentId: enabled ? s.activeDocumentId : null,
  }));
}

// ---------------------------------------------------------------------------
// Document lifecycle
// ---------------------------------------------------------------------------

/** Open (or focus) a document tab. Backward compat: also works with EditorTab. */
export function openTab(doc: WorkspaceDocument): void {
  openDocument(doc);
}

/** Open (or focus) a workspace document. */
export function openDocument(doc: WorkspaceDocument): void {
  _store.update((s) => {
    const exists = s.documents.some((d) => d.id === doc.id);
    if (exists) {
      return { ...s, activeDocumentId: doc.id };
    }
    return {
      ...s,
      documents: [...s.documents, doc],
      activeDocumentId: doc.id,
    };
  });
}

/** Close a document by id. */
export function closeDocument(id: string): void {
  _store.update((s) => {
    const idx = s.documents.findIndex((d) => d.id === id);
    if (idx < 0) return s;

    const next = s.documents.filter((d) => d.id !== id);
    let nextActive = s.activeDocumentId;

    if (s.activeDocumentId === id) {
      if (next.length === 0) {
        nextActive = null;
      } else if (idx >= next.length) {
        nextActive = next[next.length - 1]?.id ?? null;
      } else {
        nextActive = next[idx]?.id ?? null;
      }
    }

    let nextSecondary = s.secondaryDocumentId;
    if (nextSecondary === id) {
      nextSecondary = nextActive;
    }

    return {
      ...s,
      documents: next,
      activeDocumentId: nextActive,
      secondaryDocumentId: nextSecondary,
    };
  });
}

/** Close tab by id (backward compat alias). */
export function closeTab(tabId: string): void {
  closeDocument(tabId);
}

/** Move a document into the secondary split pane. */
export function splitDocument(id: string): void {
  _store.update((s) => ({
    ...s,
    workspaceSplit: true,
    secondaryDocumentId: id,
  }));
}

/** Toggle dirty flag on a document. */
export function markDirty(id: string, dirty: boolean): void {
  _store.update((s) => ({
    ...s,
    documents: s.documents.map((d) =>
      d.id === id ? { ...d, dirty } : d
    ),
  }));
}

export function setActiveTab(tabId: string): void {
  _store.update((s) => ({ ...s, activeDocumentId: tabId }));
}

/** Set the document shown in the secondary pane. */
export function setSecondaryDocument(id: string | null): void {
  _store.update((s) => ({ ...s, secondaryDocumentId: id }));
}

// ---------------------------------------------------------------------------
// Bottom panel
// ---------------------------------------------------------------------------

export function toggleBottomPanel(): void {
  _store.update((s) => ({ ...s, bottomPanelOpen: !s.bottomPanelOpen }));
}

export function setBottomPanelHeight(height: number): void {
  saveNumber(BOTTOM_PANEL_HEIGHT_KEY, height);
  _store.update((s) => ({ ...s, bottomPanelHeight: height }));
}

export function setActivePanelTab(tab: PanelTab): void {
  _store.update((s) => ({ ...s, activePanelTab: tab }));
}

/**
 * Select a panel tab and ensure the bottom panel is open.
 *
 * Tabs are "open-on-select": clicking Output/Problems/etc. while the panel is
 * collapsed selects that tab and reveals it in one action. Already-open panels
 * just switch tabs. Use this for any user gesture that should surface a tab
 * (tab clicks, deep links, command-palette panel commands).
 */
export function openPanelTab(tab: PanelTab): void {
  _store.update((s) => ({ ...s, activePanelTab: tab, bottomPanelOpen: true }));
}

// ---------------------------------------------------------------------------
// Utility
// ---------------------------------------------------------------------------

/** Reset state (useful for tests). */
export function resetIDEState(): void {
  _store.set(createInitialState());
}

/** Read current state snapshot (non-reactive). */
export function getIDEState(): IDEState {
  return get(ideState);
}

/**
 * Restore layout from localStorage.
 * Returns true if a saved layout was found and applied.
 */
export function restoreLayout(): boolean {
  const saved = loadLayout();
  if (!saved) return false;
  _layoutRestored = true;
  _store.update((s) => ({
    ...s,
    documents: saved.openTabs,
    activeDocumentId: saved.activeTabId,
    workspaceSplit: saved.workspaceSplit,
    bottomPanelOpen: saved.bottomPanelOpen,
    activePanelTab: saved.activePanelTab,
    activeView: saved.activeView,
  }));
  return true;
}

/** Derived boolean indicating whether a saved layout exists in localStorage. */
export const hasRestoredLayout = derived(ideState, () => {
  if (_layoutRestored) return true;
  // Check if there's a saved layout available
  return loadLayout() !== null;
});
