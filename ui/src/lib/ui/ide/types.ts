/**
 * Type definitions for the IDE shell layout.
 */

export type IDEView = 'hl7' | 'workflows' | 'events' | 'profiles' | 'terminology' | 'operator' | 'system';
export type IDEAppRoute = '/' | '/hl7' | '/workflows' | '/events' | '/profiles' | '/terminology' | '/operator';

/**
 * What a workspace tab holds. Only routes: the editor-less artifact types
 * (trace, workflow draft, debug session, event, profile) were removed with
 * their placeholder surface; a layout persisted before that drops them on
 * restore.
 */
export type DocumentType = 'route';

/** A workspace document: one open route tab. */
export interface WorkspaceDocument {
  id: string;
  /** Document type. Defaults to 'route' when omitted (backward compat). */
  type?: DocumentType;
  title: string;
  subtitle?: string | undefined;
  route?: string | undefined;
  artifactId?: string | undefined;
  dirty: boolean;
  restorableState?: unknown;
  /** @deprecated Kept for backward compat with route-type documents. */
  view?: IDEView | undefined;
  /** @deprecated Kept for backward compat with route-type documents. */
  path?: IDEAppRoute | undefined;
}

/**
 * Legacy alias — route-type documents satisfy this shape.
 * @deprecated Use WorkspaceDocument instead.
 */
export type EditorTab = WorkspaceDocument;

export type PanelTab = 'output' | 'problems' | 'debug' | 'trace' | 'copilot';

export type SplitOrientation = 'horizontal' | 'vertical';

export interface IDEState {
  sidebarOpen: boolean;
  sidebarWidth: number;
  activeView: IDEView;
  /** All open documents (tabs). */
  documents: WorkspaceDocument[];
  /** Active document in the primary pane. */
  activeDocumentId: string | null;
  /** Document shown in the secondary (split) pane, if any. */
  secondaryDocumentId: string | null;
  workspaceSplit: boolean;
  bottomPanelOpen: boolean;
  bottomPanelHeight: number;
  activePanelTab: PanelTab;
  /**
   * @deprecated Alias for documents — reads/writes are forwarded.
   * Kept so existing code referencing openTabs still compiles.
   */
  openTabs: WorkspaceDocument[];
  /**
   * @deprecated Alias for activeDocumentId.
   */
  activeTabId: string | null;
}
