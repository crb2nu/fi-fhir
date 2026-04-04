/**
 * IDE shell components and state management.
 *
 * Re-exports all IDE layout components for convenient imports.
 */

// Components
export { default as IDEShell } from './IDEShell.svelte';
export { default as ActivityBar } from './ActivityBar.svelte';
export { default as Sidebar } from './Sidebar.svelte';
export { default as EditorTabs } from './EditorTabs.svelte';
export { default as BottomPanel } from './BottomPanel.svelte';
export { default as StatusBar } from './StatusBar.svelte';
export { default as SplitPane } from './SplitPane.svelte';
export { default as DocumentHost } from './DocumentHost.svelte';

// Store
export {
  ideState,
  toggleSidebar,
  setSidebarWidth,
  setActiveView,
  openTab,
  openDocument,
  closeTab,
  closeDocument,
  splitDocument,
  markDirty,
  setActiveTab,
  setSecondaryDocument,
  toggleWorkspaceSplit,
  setWorkspaceSplit,
  toggleBottomPanel,
  setBottomPanelHeight,
  setActivePanelTab,
  createWorkspaceTab,
  createDocument,
  getWorkspaceTabTitle,
  resolveNextWorkspaceTabId,
  resetIDEState,
  getIDEState,
} from './ideStore';

// Keyboard shortcuts
export { initKeyboardShortcuts } from './keyboardShortcuts';

// Types
export type {
  IDEView,
  EditorTab,
  WorkspaceDocument,
  DocumentType,
  PanelTab,
  SplitOrientation,
  IDEState,
} from './types';
