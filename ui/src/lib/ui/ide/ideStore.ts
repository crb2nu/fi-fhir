/**
 * IDE state store with localStorage persistence for layout dimensions.
 */
import { writable, get } from 'svelte/store';
import type { IDEState, IDEView, EditorTab, PanelTab } from './types';

const SIDEBAR_WIDTH_KEY = 'fi-fhir-ide-sidebar-width';
const BOTTOM_PANEL_HEIGHT_KEY = 'fi-fhir-ide-bottom-panel-height';

const WORKSPACE_ROUTE_TITLES: Record<IDEView, string> = {
  system: 'Workbench',
  hl7: 'HL7 Mapping',
  workflows: 'Workflows',
  events: 'Events',
  profiles: 'Profiles',
  terminology: 'Terminology',
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

export function createWorkspaceTab(pathname: string, view?: IDEView): EditorTab {
  const normalized = normalizeWorkspacePathname(pathname);
  const workspaceView = view ?? workspaceViewForPath(normalized);
  return {
    id: normalized,
    title: getWorkspaceTabTitle(normalized, workspaceView),
    dirty: false,
    view: workspaceView,
    path: normalized,
  };
}

export function resolveNextWorkspaceTabId(
  tabs: EditorTab[],
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

function createInitialState(): IDEState {
  return {
    sidebarOpen: false,
    sidebarWidth: loadNumber(SIDEBAR_WIDTH_KEY, 280),
    activeView: 'hl7',
    openTabs: [],
    activeTabId: null,
    workspaceSplit: false,
    bottomPanelOpen: false,
    bottomPanelHeight: loadNumber(BOTTOM_PANEL_HEIGHT_KEY, 200),
    activePanelTab: 'output',
  };
}

export const ideState = writable<IDEState>(createInitialState());

export function toggleSidebar(): void {
  ideState.update((s) => ({ ...s, sidebarOpen: !s.sidebarOpen }));
}

export function setSidebarWidth(width: number): void {
  saveNumber(SIDEBAR_WIDTH_KEY, width);
  ideState.update((s) => ({ ...s, sidebarWidth: width }));
}

export function setActiveView(view: IDEView): void {
  ideState.update((s) => ({ ...s, activeView: view }));
}

export function toggleWorkspaceSplit(): void {
  ideState.update((s) => ({ ...s, workspaceSplit: !s.workspaceSplit }));
}

export function setWorkspaceSplit(enabled: boolean): void {
  ideState.update((s) => ({ ...s, workspaceSplit: enabled }));
}

export function openTab(tab: EditorTab): void {
  ideState.update((s) => {
    const exists = s.openTabs.some((t) => t.id === tab.id);
    if (exists) {
      return { ...s, activeTabId: tab.id };
    }
    return {
      ...s,
      openTabs: [...s.openTabs, tab],
      activeTabId: tab.id,
    };
  });
}

export function closeTab(tabId: string): void {
  ideState.update((s) => {
    const idx = s.openTabs.findIndex((t) => t.id === tabId);
    if (idx < 0) return s;

    const next = s.openTabs.filter((t) => t.id !== tabId);
    let nextActive = s.activeTabId;

    if (s.activeTabId === tabId) {
      if (next.length === 0) {
        nextActive = null;
      } else if (idx >= next.length) {
        nextActive = next[next.length - 1]?.id ?? null;
      } else {
        nextActive = next[idx]?.id ?? null;
      }
    }

    return { ...s, openTabs: next, activeTabId: nextActive };
  });
}

export function setActiveTab(tabId: string): void {
  ideState.update((s) => ({ ...s, activeTabId: tabId }));
}

export function toggleBottomPanel(): void {
  ideState.update((s) => ({ ...s, bottomPanelOpen: !s.bottomPanelOpen }));
}

export function setBottomPanelHeight(height: number): void {
  saveNumber(BOTTOM_PANEL_HEIGHT_KEY, height);
  ideState.update((s) => ({ ...s, bottomPanelHeight: height }));
}

export function setActivePanelTab(tab: PanelTab): void {
  ideState.update((s) => ({ ...s, activePanelTab: tab }));
}

/** Reset state (useful for tests). */
export function resetIDEState(): void {
  ideState.set(createInitialState());
}

/** Read current state snapshot (non-reactive). */
export function getIDEState(): IDEState {
  return get(ideState);
}
