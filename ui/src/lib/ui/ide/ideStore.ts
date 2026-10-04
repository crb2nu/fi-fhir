/**
 * IDE state store with localStorage persistence for layout dimensions and full layout state.
 *
 * Tabs are route documents (WorkspaceDocument). The editor-less artifact
 * document types were removed; `loadLayout` drops any a stored layout holds.
 *
 * Draft owners report independent dirty state. Badges aggregate by route, while
 * navigation guards distinguish component-local drafts from shared state that
 * survives route changes. Draft flags are never persisted.
 */
import { writable, derived, get } from 'svelte/store';
import type {
  IDEState,
  IDEView,
  WorkspaceDocument,
  PanelTab,
  IDEAppRoute,
} from './types';

const SIDEBAR_OPEN_KEY = 'fi-fhir-ide-sidebar-open';
const SIDEBAR_WIDTH_KEY = 'fi-fhir-ide-sidebar-width';
const BOTTOM_PANEL_HEIGHT_KEY = 'fi-fhir-ide-bottom-panel-height';
const LAYOUT_KEY = 'fi-fhir-ide-layout';

/**
 * Serializable subset of IDE state for layout persistence.
 */
interface PersistedLayout {
  openTabs: WorkspaceDocument[];
  activeTabId: string | null;
  bottomPanelOpen: boolean;
  activePanelTab: PanelTab;
  activeView: IDEView;
}

const VALID_PANEL_TABS = new Set<PanelTab>(['output', 'problems', 'debug', 'trace', 'copilot']);
const VALID_VIEWS = new Set<IDEView>([
  'hl7',
  'workflows',
  'events',
  'profiles',
  'terminology',
  'connections',
  'operator',
  'system',
]);

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
    if (typeof obj['bottomPanelOpen'] !== 'boolean') return null;
    if (typeof obj['activePanelTab'] !== 'string' || !VALID_PANEL_TABS.has(obj['activePanelTab'] as PanelTab)) return null;
    if (typeof obj['activeView'] !== 'string' || !VALID_VIEWS.has(obj['activeView'] as IDEView)) return null;

    // Rebuild saved tabs through the same route/query allowlist as live
    // navigation. Older layouts can contain obsolete documents or extra data.
    const documents = new Map<string, WorkspaceDocument>();
    for (const entry of obj['openTabs']) {
      if (!entry || typeof entry !== 'object') continue;
      const doc = entry as Record<string, unknown>;
      if (doc.type !== undefined && doc.type !== 'route') continue;
      const path = doc.path ?? doc.route ?? doc.id;
      if (typeof path !== 'string' || !path.startsWith('/') || path.startsWith('//')) continue;
      try {
        const url = new URL(path, 'https://workspace.invalid');
        const route = normalizeWorkspacePathname(url.pathname);
        if (url.origin !== 'https://workspace.invalid' || !Object.values(WORKSPACE_VIEW_ROUTES).includes(route as IDEAppRoute)) continue;
        const tab = createWorkspaceTab(route, undefined, url.search);
        documents.set(tab.id, tab);
      } catch { /* One malformed saved path must not discard the other tabs. */ }
    }
    const openTabs = [...documents.values()];
    const storedActive = obj['activeTabId'] as string | null;
    const activeTabId = openTabs.some((doc) => doc.id === storedActive)
      ? storedActive
      : (openTabs[0]?.id ?? null);

    return {
      openTabs,
      activeTabId,
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
      openTabs: state.openTabs.map((doc) => ({ ...doc, dirty: false })),
      activeTabId: state.activeTabId,
      bottomPanelOpen: state.bottomPanelOpen,
      activePanelTab: state.activePanelTab,
      activeView: state.activeView,
    };
    localStorage.setItem(LAYOUT_KEY, JSON.stringify(layout));
  } catch {
    // Ignore storage errors
  }
}

// Editor tab titles: the same words as the ActivityBar labels and the route
// toolbars ("Home", "Operator"). Stage names (Source Intake, Delivery, …) live
// in the header stage control, not in the tabs.
const WORKSPACE_ROUTE_TITLES: Record<IDEView, string> = {
  system: 'Home',
  hl7: 'HL7 / Intake',
  workflows: 'Workflows',
  events: 'Verification',
  profiles: 'Profiles',
  terminology: 'Terminology',
  connections: 'Connections',
  operator: 'Operator',
};

const WORKSPACE_VIEW_ROUTES: Record<IDEView, IDEAppRoute> = {
  system: '/',
  hl7: '/hl7',
  workflows: '/workflows',
  events: '/events',
  profiles: '/profiles',
  terminology: '/terminology',
  connections: '/connections',
  operator: '/operator',
};

// Keep only record selectors understood by the routes. Arbitrary URL params
// (including credentials) must not become part of the persisted layout.
const WORKSPACE_QUERY_KEYS: Partial<Record<IDEView, string[]>> = {
  hl7: ['session'],
  operator: ['receipt', 'attempt', 'definition', 'revision'],
  connections: ['connection', 'definition', 'revision'],
  events: ['receipt'],
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
  if (normalized.startsWith('/operator')) return 'operator';
  if (normalized.startsWith('/connections')) return 'connections';
  if (normalized.startsWith('/terminology')) return 'terminology';
  if (normalized.startsWith('/workflows')) return 'workflows';
  return 'system';
}

export function getWorkspaceTabTitle(pathname: string, view?: IDEView): string {
  const workspaceView = view ?? workspaceViewForPath(pathname);
  return WORKSPACE_ROUTE_TITLES[workspaceView];
}

/** Create a route-type workspace document (backward compat with createWorkspaceTab). */
export function createWorkspaceTab(pathname: string, view?: IDEView, search = ''): WorkspaceDocument {
  const normalized = normalizeWorkspacePathname(pathname);
  const workspaceView = view ?? workspaceViewForPath(normalized);
  const workspaceRoute = WORKSPACE_VIEW_ROUTES[workspaceView];
  const params = new URLSearchParams(search);
  const selection = new URLSearchParams();
  for (const key of WORKSPACE_QUERY_KEYS[workspaceView] ?? []) {
    const value = params.get(key);
    if (value) selection.set(key, value);
  }
  const query = selection.toString();
  return {
    id: workspaceRoute,
    type: 'route',
    title: getWorkspaceTabTitle(normalized, workspaceView),
    dirty: false,
    view: workspaceView,
    path: query ? `${workspaceRoute}?${query}` : workspaceRoute,
    route: workspaceRoute,
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
  drafts: DraftState[];
  bottomPanelOpen: boolean;
  bottomPanelHeight: number;
  activePanelTab: PanelTab;
}

function createInitialState(saved: PersistedLayout | null = null): InternalIDEState {
  return {
    sidebarOpen: loadNumber(SIDEBAR_OPEN_KEY, 0) === 1,
    sidebarWidth: loadNumber(SIDEBAR_WIDTH_KEY, 280),
    activeView: saved?.activeView ?? 'hl7',
    documents: saved?.openTabs ?? [],
    activeDocumentId: saved?.activeTabId ?? null,
    drafts: [],
    bottomPanelOpen: saved?.bottomPanelOpen ?? false,
    bottomPanelHeight: loadNumber(BOTTOM_PANEL_HEIGHT_KEY, 200),
    activePanelTab: saved?.activePanelTab ?? 'output',
  };
}

// Restore before the persistence subscription's first synchronous emission.
// The shell then applies the actual URL, preserving the other saved tabs.
const savedLayout = loadLayout();
let _layoutRestored = savedLayout !== null;
const _store = writable<InternalIDEState>(createInitialState(savedLayout));

/**
 * Public ideState derived store that exposes the full IDEState interface
 * including backward-compat aliases (openTabs, activeTabId).
 */
export const ideState = derived(_store, ($s): IDEState => {
  const { drafts, ...rest } = $s;
  const documents = $s.documents.map((doc) => {
    const dirty = drafts.some((draft) => draft.route === doc.id);
    return doc.dirty === dirty ? doc : { ...doc, dirty };
  });
  return {
    ...rest,
    documents,
    openTabs: documents,
    activeTabId: $s.activeDocumentId,
  };
});

// ---------------------------------------------------------------------------
// Sidebar / layout actions
// ---------------------------------------------------------------------------

// Persist layout on every state change
ideState.subscribe((state) => {
  saveLayout(state);
});

export function toggleSidebar(): void {
  setSidebarOpen(!get(_store).sidebarOpen);
}

export function setSidebarOpen(open: boolean): void {
  saveNumber(SIDEBAR_OPEN_KEY, open ? 1 : 0);
  _store.update((s) => ({ ...s, sidebarOpen: open }));
}

export function setSidebarWidth(width: number): void {
  saveNumber(SIDEBAR_WIDTH_KEY, width);
  _store.update((s) => ({ ...s, sidebarWidth: width }));
}

export function setActiveView(view: IDEView): void {
  _store.update((s) => ({ ...s, activeView: view }));
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
      return {
        ...s,
        documents: s.documents.map((existing) => existing.id === doc.id ? { ...existing, ...doc } : existing),
        activeDocumentId: doc.id,
      };
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

    return {
      ...s,
      documents: next,
      activeDocumentId: nextActive,
    };
  });
}

/** Close tab by id (backward compat alias). */
export function closeTab(tabId: string): void {
  closeDocument(tabId);
}

interface DraftState {
  route: string;
  owner: string;
  lostOnLeave: boolean;
}

/** Report one editor's state without clearing other editors on the same route. */
export function setDraftState(route: string, owner: string, dirty: boolean, lostOnLeave = true): void {
  _store.update((s) => {
    const current = s.drafts.find((draft) => draft.route === route && draft.owner === owner);
    if ((!dirty && !current) || (dirty && current?.lostOnLeave === lostOnLeave)) return s;
    const drafts = s.drafts.filter((draft) => draft.route !== route || draft.owner !== owner);
    if (dirty) drafts.push({ route, owner, lostOnLeave });
    return { ...s, drafts };
  });
}

/** Saving, discarding or unmounting an editor releases only its own mark. */
export function clearDraftState(route: string, owner: string): void {
  setDraftState(route, owner, false);
}

/** Legacy single-owner API. New editors should use an explicit owner. */
export function markDirty(id: string, dirty = true): void {
  setDraftState(id, 'legacy', dirty);
}

export function clearDirty(id: string): void {
  clearDraftState(id, 'legacy');
}

export function isDirty(id: string): boolean {
  return get(_store).drafts.some((draft) => draft.route === id);
}

/** Reload/unload loses shared drafts too, even if their editor isn't open. */
export function hasDraftsToLose(route: string, leavingApp = false): boolean {
  return get(_store).drafts.some((draft) => leavingApp || (draft.route === route && draft.lostOnLeave));
}

export function setActiveTab(tabId: string): void {
  _store.update((s) => ({ ...s, activeDocumentId: tabId }));
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
