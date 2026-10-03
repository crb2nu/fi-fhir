/**
 * Tests for the IDE state store.
 */
import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import {
  ideState,
  toggleSidebar,
  setSidebarWidth,
  setActiveView,
  openTab,
  closeTab,
  setActiveTab,
  toggleBottomPanel,
  setBottomPanelHeight,
  setActivePanelTab,
  openPanelTab,
  markDirty,
  clearDirty,
  isDirty,
  createWorkspaceTab,
  resolveNextWorkspaceTabId,
  resetIDEState,
  getIDEState,
  restoreLayout,
} from './ideStore';
import type { EditorTab } from './types';

describe('ideStore', () => {
  beforeEach(() => {
    localStorage.clear();
    resetIDEState();
  });

  describe('initial state', () => {
    it('should have sidebar closed by default', () => {
      const state = get(ideState);
      expect(state.sidebarOpen).toBe(false);
    });

    it('should have default sidebar width', () => {
      const state = get(ideState);
      expect(state.sidebarWidth).toBe(280);
    });

    it('should have hl7 as default active view', () => {
      const state = get(ideState);
      expect(state.activeView).toBe('hl7');
    });

    it('should have empty open tabs', () => {
      const state = get(ideState);
      expect(state.openTabs).toHaveLength(0);
      expect(state.activeTabId).toBeNull();
    });

    it('has no split workspace (removed with its placeholder pane)', () => {
      const state = get(ideState) as unknown as Record<string, unknown>;
      expect('workspaceSplit' in state).toBe(false);
      expect('secondaryDocumentId' in state).toBe(false);
    });

    it('should have bottom panel closed by default', () => {
      const state = get(ideState);
      expect(state.bottomPanelOpen).toBe(false);
    });

    it('should have default bottom panel height', () => {
      const state = get(ideState);
      expect(state.bottomPanelHeight).toBe(200);
    });

    it('should have output as default panel tab', () => {
      const state = get(ideState);
      expect(state.activePanelTab).toBe('output');
    });
  });

  describe('toggleSidebar', () => {
    it('should toggle sidebar open state', () => {
      expect(get(ideState).sidebarOpen).toBe(false);
      toggleSidebar();
      expect(get(ideState).sidebarOpen).toBe(true);
      toggleSidebar();
      expect(get(ideState).sidebarOpen).toBe(false);
    });
  });

  it('remembers whether the desktop explorer is open across reloads', () => {
    toggleSidebar();
    resetIDEState();
    expect(get(ideState).sidebarOpen).toBe(true);
    toggleSidebar();
    resetIDEState();
    expect(get(ideState).sidebarOpen).toBe(false);
  });

  describe('setSidebarWidth', () => {
    it('should update sidebar width', () => {
      setSidebarWidth(350);
      expect(get(ideState).sidebarWidth).toBe(350);
    });

    it('should persist width to localStorage', () => {
      setSidebarWidth(400);
      expect(localStorage.getItem('fi-fhir-ide-sidebar-width')).toBe('400');
    });
  });

  describe('setActiveView', () => {
    it('should update active view', () => {
      setActiveView('workflows');
      expect(get(ideState).activeView).toBe('workflows');
    });
  });

  describe('workspace tab helpers', () => {
    it('keeps record selectors in the location without changing the tab identity', () => {
      const tab = createWorkspaceTab('/operator', 'operator', '?definition=lab%2Fadt&revision=r2&token=private');
      expect(tab.id).toBe('/operator');
      expect(tab.route).toBe('/operator');
      expect(tab.path).toBe('/operator?definition=lab%2Fadt&revision=r2');
      expect(createWorkspaceTab('/hl7', 'hl7', '?session=sess-2&receipt=wrong-view').path)
        .toBe('/hl7?session=sess-2');
    });

    it('updates the remembered record and persists it without duplicating or clearing a dirty tab', () => {
      openTab(createWorkspaceTab('/hl7', 'hl7', '?session=first'));
      markDirty('/hl7');
      openTab(createWorkspaceTab('/hl7', 'hl7', '?session=second'));
      expect(get(ideState).documents).toHaveLength(1);
      expect(get(ideState).documents[0]).toMatchObject({ path: '/hl7?session=second', dirty: true });
      const layout = JSON.parse(localStorage.getItem('fi-fhir-ide-layout')!);
      expect(layout.openTabs[0]).toMatchObject({ path: '/hl7?session=second', dirty: false });
      openTab(createWorkspaceTab('/hl7'));
      expect(get(ideState).documents[0]?.path).toBe('/hl7');
    });

    it('should build route-aware tabs from a pathname', () => {
      const tab = createWorkspaceTab('/workflows/');
      expect(tab.id).toBe('/workflows');
      expect(tab.title).toBe('Workflows');
      expect(tab.view).toBe('workflows');
      expect(tab.path).toBe('/workflows');
      expect(tab.dirty).toBe(false);
    });

    it('should resolve the next active tab when closing the current tab', () => {
      const tabs: EditorTab[] = [
        { id: '/hl7', title: 'HL7 Mapping', dirty: false, view: 'hl7' },
        { id: '/workflows', title: 'Workflows', dirty: false, view: 'workflows' },
        { id: '/events', title: 'Verification', dirty: false, view: 'events' },
      ];

      expect(resolveNextWorkspaceTabId(tabs, '/workflows', '/workflows')).toBe('/events');
      expect(resolveNextWorkspaceTabId(tabs, '/events', '/events')).toBe('/workflows');
      expect(resolveNextWorkspaceTabId(tabs, '/events', '/missing')).toBe('/events');
    });
  });

  describe('openTab', () => {
    const tab1: EditorTab = {
      id: 'tab-1',
      title: 'Test Tab',
      dirty: false,
      view: 'hl7',
    };

    const tab2: EditorTab = {
      id: 'tab-2',
      title: 'Another Tab',
      dirty: true,
      view: 'workflows',
    };

    it('should add a new tab and make it active', () => {
      openTab(tab1);
      const state = get(ideState);
      expect(state.openTabs).toHaveLength(1);
      expect(state.openTabs[0]!.id).toBe('tab-1');
      expect(state.activeTabId).toBe('tab-1');
    });

    it('should not duplicate an existing tab', () => {
      openTab(tab1);
      openTab(tab1);
      const state = get(ideState);
      expect(state.openTabs).toHaveLength(1);
    });

    it('should activate existing tab when reopened', () => {
      openTab(tab1);
      openTab(tab2);
      expect(get(ideState).activeTabId).toBe('tab-2');
      openTab(tab1);
      expect(get(ideState).activeTabId).toBe('tab-1');
    });

    it('should support multiple tabs', () => {
      openTab(tab1);
      openTab(tab2);
      expect(get(ideState).openTabs).toHaveLength(2);
    });
  });

  describe('closeTab', () => {
    const tab1: EditorTab = { id: 'tab-1', title: 'Tab 1', dirty: false, view: 'hl7' };
    const tab2: EditorTab = { id: 'tab-2', title: 'Tab 2', dirty: false, view: 'workflows' };
    const tab3: EditorTab = { id: 'tab-3', title: 'Tab 3', dirty: false, view: 'events' };

    it('should remove a tab', () => {
      openTab(tab1);
      openTab(tab2);
      closeTab('tab-1');
      expect(get(ideState).openTabs).toHaveLength(1);
      expect(get(ideState).openTabs[0]!.id).toBe('tab-2');
    });

    it('should select next tab when active tab is closed', () => {
      openTab(tab1);
      openTab(tab2);
      openTab(tab3);
      setActiveTab('tab-2');
      closeTab('tab-2');
      expect(get(ideState).activeTabId).toBe('tab-3');
    });

    it('should select previous tab when last tab is closed', () => {
      openTab(tab1);
      openTab(tab2);
      setActiveTab('tab-2');
      closeTab('tab-2');
      expect(get(ideState).activeTabId).toBe('tab-1');
    });

    it('should set activeTabId to null when all tabs are closed', () => {
      openTab(tab1);
      closeTab('tab-1');
      expect(get(ideState).activeTabId).toBeNull();
      expect(get(ideState).openTabs).toHaveLength(0);
    });

    it('should not change active tab when closing an inactive tab', () => {
      openTab(tab1);
      openTab(tab2);
      closeTab('tab-1');
      expect(get(ideState).activeTabId).toBe('tab-2');
    });

    it('should handle closing non-existent tab gracefully', () => {
      openTab(tab1);
      closeTab('non-existent');
      expect(get(ideState).openTabs).toHaveLength(1);
    });
  });

  describe('setActiveTab', () => {
    it('should update active tab id', () => {
      const tab: EditorTab = { id: 'tab-1', title: 'Tab', dirty: false, view: 'hl7' };
      openTab(tab);
      setActiveTab('tab-1');
      expect(get(ideState).activeTabId).toBe('tab-1');
    });
  });

  describe('toggleBottomPanel', () => {
    it('should toggle bottom panel open state', () => {
      expect(get(ideState).bottomPanelOpen).toBe(false);
      toggleBottomPanel();
      expect(get(ideState).bottomPanelOpen).toBe(true);
      toggleBottomPanel();
      expect(get(ideState).bottomPanelOpen).toBe(false);
    });
  });

  describe('setBottomPanelHeight', () => {
    it('should update bottom panel height', () => {
      setBottomPanelHeight(300);
      expect(get(ideState).bottomPanelHeight).toBe(300);
    });

    it('should persist height to localStorage', () => {
      setBottomPanelHeight(250);
      expect(localStorage.getItem('fi-fhir-ide-bottom-panel-height')).toBe('250');
    });
  });

  describe('setActivePanelTab', () => {
    it('should update active panel tab', () => {
      setActivePanelTab('problems');
      expect(get(ideState).activePanelTab).toBe('problems');
    });

    it('should accept all valid panel tabs', () => {
      setActivePanelTab('trace');
      expect(get(ideState).activePanelTab).toBe('trace');

      setActivePanelTab('output');
      expect(get(ideState).activePanelTab).toBe('output');
    });
  });

  describe('openPanelTab', () => {
    it('opens a collapsed panel and selects the tab in one action', () => {
      expect(get(ideState).bottomPanelOpen).toBe(false);
      openPanelTab('problems');
      expect(get(ideState).bottomPanelOpen).toBe(true);
      expect(get(ideState).activePanelTab).toBe('problems');
    });

    it('switches tabs without closing an already-open panel', () => {
      toggleBottomPanel(); // open
      openPanelTab('output');
      expect(get(ideState).bottomPanelOpen).toBe(true);
      openPanelTab('trace');
      expect(get(ideState).bottomPanelOpen).toBe(true);
      expect(get(ideState).activePanelTab).toBe('trace');
    });
  });

  describe('unsaved changes', () => {
    it('marks and clears a tab, and shows the mark on its document', () => {
      openTab(createWorkspaceTab('/profiles'));
      expect(get(ideState).documents[0]?.dirty).toBe(false);

      markDirty('/profiles');
      expect(isDirty('/profiles')).toBe(true);
      expect(get(ideState).documents[0]?.dirty).toBe(true);

      clearDirty('/profiles');
      expect(isDirty('/profiles')).toBe(false);
      expect(get(ideState).documents[0]?.dirty).toBe(false);
    });

    it('treats markDirty(id, false) as clearDirty', () => {
      markDirty('/workflows');
      markDirty('/workflows', false);
      expect(isDirty('/workflows')).toBe(false);
    });

    it('keeps a mark made before the tab opens and applies it on open', () => {
      markDirty('/connections');
      openTab(createWorkspaceTab('/connections'));
      expect(get(ideState).documents.find((doc) => doc.id === '/connections')?.dirty).toBe(true);
    });

    it('never persists a dirty mark: a reload starts clean', () => {
      openTab(createWorkspaceTab('/profiles'));
      markDirty('/profiles');
      const stored = JSON.parse(localStorage.getItem('fi-fhir-ide-layout') ?? '{}') as {
        openTabs: Array<{ dirty: boolean }>;
      };
      expect(stored.openTabs.map((tab) => tab.dirty)).toEqual([false]);

      resetIDEState();
      localStorage.setItem(
        'fi-fhir-ide-layout',
        JSON.stringify({ ...stored, openTabs: [{ ...createWorkspaceTab('/profiles'), dirty: true }] })
      );
      expect(restoreLayout()).toBe(true);
      expect(get(ideState).documents[0]?.dirty).toBe(false);
    });
  });

  describe('localStorage persistence', () => {
    it('should load sidebar width from localStorage', () => {
      localStorage.setItem('fi-fhir-ide-sidebar-width', '320');
      resetIDEState();
      expect(get(ideState).sidebarWidth).toBe(320);
    });

    it('should load bottom panel height from localStorage', () => {
      localStorage.setItem('fi-fhir-ide-bottom-panel-height', '350');
      resetIDEState();
      expect(get(ideState).bottomPanelHeight).toBe(350);
    });

    it('should use defaults for invalid stored values', () => {
      localStorage.setItem('fi-fhir-ide-sidebar-width', 'not-a-number');
      resetIDEState();
      expect(get(ideState).sidebarWidth).toBe(280);
    });

    it('restores a layout saved on the Operator view', () => {
      localStorage.setItem(
        'fi-fhir-ide-layout',
        JSON.stringify({
          openTabs: [createWorkspaceTab('/operator')],
          activeTabId: '/operator',
          bottomPanelOpen: false,
          activePanelTab: 'output',
          activeView: 'operator',
        })
      );

      expect(restoreLayout()).toBe(true);
      expect(get(ideState).activeView).toBe('operator');
      expect(get(ideState).activeTabId).toBe('/operator');
    });

    it('restores a layout saved on the Connections view', () => {
      localStorage.setItem(
        'fi-fhir-ide-layout',
        JSON.stringify({
          openTabs: [createWorkspaceTab('/connections')],
          activeTabId: '/connections',
          bottomPanelOpen: false,
          activePanelTab: 'output',
          activeView: 'connections',
        })
      );

      expect(restoreLayout()).toBe(true);
      expect(get(ideState).activeView).toBe('connections');
      expect(get(ideState).activeTabId).toBe('/connections');
      expect(get(ideState).openTabs.map((tab) => tab.title)).toEqual(['Connections']);
    });

    it('restores tab titles from the route, so renamed views come back renamed', () => {
      localStorage.setItem(
        'fi-fhir-ide-layout',
        JSON.stringify({
          openTabs: [
            { ...createWorkspaceTab('/'), title: 'Dashboard' },
            { ...createWorkspaceTab('/operator'), title: 'Operations' },
          ],
          activeTabId: '/operator',
          bottomPanelOpen: false,
          activePanelTab: 'output',
          activeView: 'operator',
        })
      );

      expect(restoreLayout()).toBe(true);
      expect(get(ideState).openTabs.map((tab) => tab.title)).toEqual(['Home', 'Operator']);
    });

    it('drops editor-less artifact tabs from an older stored layout', () => {
      localStorage.setItem(
        'fi-fhir-ide-layout',
        JSON.stringify({
          openTabs: [
            createWorkspaceTab('/hl7'),
            { id: 'trace:1a2b3c4d', type: 'trace', title: 'Active Trace', dirty: false },
          ],
          activeTabId: 'trace:1a2b3c4d',
          bottomPanelOpen: false,
          activePanelTab: 'output',
          activeView: 'hl7',
        })
      );

      expect(restoreLayout()).toBe(true);
      expect(get(ideState).openTabs.map((tab) => tab.id)).toEqual(['/hl7']);
      expect(get(ideState).activeTabId).toBe('/hl7');
    });
  });

  describe('getIDEState', () => {
    it('should return current state snapshot', () => {
      toggleSidebar();
      const state = getIDEState();
      expect(state.sidebarOpen).toBe(true);
    });
  });
});
