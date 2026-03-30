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
  resetIDEState,
  getIDEState,
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
  });

  describe('getIDEState', () => {
    it('should return current state snapshot', () => {
      toggleSidebar();
      const state = getIDEState();
      expect(state.sidebarOpen).toBe(true);
    });
  });
});
