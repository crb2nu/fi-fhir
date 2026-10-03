import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

const key = 'fi-fhir-ide-layout';
const layout = (openTabs: unknown[]) => ({
  openTabs,
  activeTabId: '/hl7',
  activeView: 'hl7',
  bottomPanelOpen: true,
  activePanelTab: 'problems',
});

beforeEach(() => {
  localStorage.clear();
  vi.resetModules();
});

afterEach(() => vi.restoreAllMocks());

describe('workspace startup', () => {
  it('restores saved tabs before the first persistence write on a fresh import', async () => {
    localStorage.setItem(key, JSON.stringify(layout([
      { id: '/hl7', path: '/hl7?session=saved-session', dirty: true },
      { id: '/operator', path: '/operator?receipt=saved-receipt' },
    ])));

    const { getIDEState, openTab, createWorkspaceTab } = await import('./ideStore');
    expect(getIDEState().documents.map((doc) => doc.path)).toEqual([
      '/hl7?session=saved-session', '/operator?receipt=saved-receipt',
    ]);
    expect(getIDEState()).toMatchObject({ activeTabId: '/hl7', bottomPanelOpen: true, activePanelTab: 'problems' });
    expect(getIDEState().documents.every((doc) => !doc.dirty)).toBe(true);
    expect(JSON.parse(localStorage.getItem(key)!).openTabs).toHaveLength(2);

    // The incoming URL replaces its own selection without losing another tab.
    openTab(createWorkspaceTab('/operator', 'operator', '?receipt=incoming-receipt'));
    expect(getIDEState().activeTabId).toBe('/operator');
    expect(getIDEState().documents.map((doc) => doc.path)).toEqual([
      '/hl7?session=saved-session', '/operator?receipt=incoming-receipt',
    ]);
  });

  it('restores only canonical routes and supported record selectors', async () => {
    localStorage.setItem(key, JSON.stringify(layout([
      null, { id: 4 }, { id: '/missing' },
      { id: 'trace:1', type: 'trace', path: '/operator' },
      { path: 'https://elsewhere.example/hl7' },
      { path: '//elsewhere.example/hl7' },
      { path: '/\\elsewhere.example/hl7' },
      { path: '/\\[' },
      { path: '/hl7?session=older' },
      { path: '/hl7/?session=current&token=discard-me#fragment', view: 'operator', title: 'Old title', dirty: true },
      { route: '/profiles', title: 'Old profiles' },
    ])));
    const { getIDEState } = await import('./ideStore');
    expect(getIDEState().documents).toMatchObject([
      { id: '/hl7', path: '/hl7?session=current', view: 'hl7', title: 'HL7 / Intake', dirty: false },
      { id: '/profiles', path: '/profiles', title: 'Profiles', dirty: false },
    ]);
    expect(localStorage.getItem(key)).not.toContain('discard-me');
  });

  it.each(['{broken', 'null', '{}'])('starts with a usable empty workspace for invalid storage: %s', async (stored) => {
    localStorage.setItem(key, stored);
    const { getIDEState, openTab, createWorkspaceTab } = await import('./ideStore');
    expect(getIDEState().documents).toEqual([]);
    openTab(createWorkspaceTab('/profiles'));
    expect(getIDEState().activeTabId).toBe('/profiles');
  });

  it('works when browser storage is unavailable', async () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => { throw new Error('disabled'); });
    vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => { throw new Error('disabled'); });
    const { getIDEState, openTab, createWorkspaceTab } = await import('./ideStore');
    openTab(createWorkspaceTab('/operator'));
    expect(getIDEState().activeTabId).toBe('/operator');
  });
});
