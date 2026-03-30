/**
 * Type definitions for the IDE shell layout.
 */

export type IDEView = 'hl7' | 'workflows' | 'events' | 'profiles' | 'terminology' | 'system';

export interface EditorTab {
  id: string;
  title: string;
  icon?: string;
  dirty: boolean;
  view: IDEView;
  path?: string;
}

export type PanelTab = 'output' | 'problems' | 'debug' | 'trace';

export type SplitOrientation = 'horizontal' | 'vertical';

export interface IDEState {
  sidebarOpen: boolean;
  sidebarWidth: number;
  activeView: IDEView;
  openTabs: EditorTab[];
  activeTabId: string | null;
  bottomPanelOpen: boolean;
  bottomPanelHeight: number;
  activePanelTab: PanelTab;
}
