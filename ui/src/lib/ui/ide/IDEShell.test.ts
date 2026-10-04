/**
 * Tests for the merged IDE shell workspace behavior.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import type { AfterNavigate, BeforeNavigate } from '@sveltejs/kit';
import { get, writable } from 'svelte/store';
import { ideState, markDirty, setDraftState, resetIDEState, setSidebarOpen, closeTab } from './ideStore';
import { registerCommands, resetCommandRegistry } from './commandRegistry';
import { takeConnectionsIntent } from '$lib/features/connections/connectionsIntent';

const pageStore = writable({ url: new URL('http://localhost/hl7') });

let navigationHook: ((navigation: BeforeNavigate) => void) | undefined;
let arrivalHook: ((navigation: AfterNavigate) => void) | undefined;

function attemptNavigation(href: string, type: BeforeNavigate['type'] = 'goto', delta?: number): boolean {
  const url = new URL(href, 'http://localhost');
  const from = get(pageStore).url;
  let cancelled = false;
  navigationHook?.({
    type, delta, from: { url: from }, to: { url },
    willUnload: type === 'leave', complete: Promise.resolve(),
    cancel: () => { cancelled = true; },
  } as BeforeNavigate);
  if (!cancelled && type !== 'leave') {
    pageStore.set({ url });
    arrivalHook?.({ from: { url: from }, to: { url }, type, delta } as AfterNavigate);
  }
  return !cancelled;
}

const gotoMock = vi.fn(async (href: string) => { attemptNavigation(href); });

vi.mock('$app/stores', () => ({ page: pageStore }));
vi.mock('$app/navigation', () => ({
  goto: gotoMock,
  beforeNavigate: (callback: (navigation: BeforeNavigate) => void) => { navigationHook = callback; },
  afterNavigate: (callback: (navigation: AfterNavigate) => void) => { arrivalHook = callback; },
}));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));

const refreshJourneyEvidence = vi.fn(async () => {});
vi.mock('./journeyState', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./journeyState')>()),
  refreshJourneyEvidence,
}));

const { default: IDEShell } = await import('./IDEShell.svelte');

// jsdom has no layout; the palette scrolls its active option into view.
Element.prototype.scrollIntoView ??= function scrollIntoView() {};

describe('IDEShell workspace', () => {
  beforeEach(() => {
    gotoMock.mockReset().mockImplementation(async (href: string) => { attemptNavigation(href); });
    navigationHook = undefined;
    arrivalHook = undefined;
    refreshJourneyEvidence.mockClear();
    localStorage.clear();
    resetIDEState();
    resetCommandRegistry();
    takeConnectionsIntent();
    pageStore.set({ url: new URL('http://localhost/hl7') });
  });

  it('offers a visible explorer toggle and restores the compact navigation on collapse', async () => {
    render(IDEShell);
    const toggle = screen.getByRole('button', { name: 'Show explorer' });
    await fireEvent.click(toggle);
    expect(screen.getByRole('complementary', { name: 'Explorer' })).toBeInTheDocument();
    expect(screen.queryByRole('navigation', { name: 'Activity bar' })).not.toBeInTheDocument();
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    expect(screen.getByRole('textbox', { name: 'Filter explorer' })).toHaveFocus();
    await fireEvent.click(screen.getByRole('button', { name: 'Collapse explorer' }));
    expect(toggle).toHaveFocus();
    expect(screen.getByRole('navigation', { name: 'Activity bar' })).toBeInTheDocument();
  });

  it('focuses the explorer when opened by shortcut but not when restoring its open state', async () => {
    const editor = document.createElement('textarea');
    document.body.appendChild(editor);
    editor.focus();
    setSidebarOpen(true);
    render(IDEShell);
    await tick();
    expect(editor).toHaveFocus();

    await fireEvent.keyDown(editor, { key: 'b', ctrlKey: true });
    expect(screen.queryByRole('complementary', { name: 'Explorer' })).not.toBeInTheDocument();
    await fireEvent.keyDown(editor, { key: 'b', ctrlKey: true });
    expect(screen.getByRole('textbox', { name: 'Filter explorer' })).toHaveFocus();
    editor.remove();
  });

  it('opens route-aware tabs as navigation changes', async () => {
    render(IDEShell);
    await tick();

    expect(screen.getByRole('tab', { name: 'HL7 / Intake' })).toBeInTheDocument();
    expect(screen.getByRole('link', { current: 'step' })).toHaveTextContent('Source Intake');
    expect(screen.getByRole('tab', { name: 'HL7 / Intake' })).toHaveAttribute('aria-selected', 'true');

    pageStore.set({ url: new URL('http://localhost/workflows') });
    await tick();

    expect(screen.getByRole('tab', { name: 'HL7 / Intake' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Workflows' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: 'Workflows' })).toHaveAttribute('aria-selected', 'true');

    await fireEvent.click(screen.getByRole('tab', { name: 'HL7 / Intake' }));
    expect(gotoMock).toHaveBeenCalledWith('/hl7');
  });

  it('closes the active tab and returns to the remaining route', async () => {
    render(IDEShell);
    await tick();

    pageStore.set({ url: new URL('http://localhost/workflows') });
    await tick();

    await fireEvent.click(screen.getByLabelText('Close Workflows'));
    await tick();

    expect(gotoMock).toHaveBeenCalledWith('/hl7');
    expect(screen.getByRole('tab', { name: 'HL7 / Intake' })).toHaveAttribute('aria-selected', 'true');
    expect(get(ideState).activeTabId).toBe('/hl7');
  });

  it.each([false, true])('focuses the adjacent editor after active Delete navigation completes (dirty=%s)', async (dirty) => {
    render(IDEShell);
    for (const path of ['/operator', '/events', '/operator']) {
      pageStore.set({ url: new URL(`http://localhost${path}`) });
      await tick();
    }
    markDirty('/operator', dirty);
    await tick();
    const original = screen.getByRole('tab', { name: /^Operator/ });
    original.focus();
    let finish!: () => Promise<void>;
    gotoMock.mockImplementationOnce((href: string) => new Promise<void>((resolveGoto) => {
      finish = async () => {
        attemptNavigation(href);
        await tick();
        original.blur(); // The router finishes resetting focus before goto resolves.
        resolveGoto();
      };
    }));
    await fireEvent.keyDown(original, { key: 'Delete' });
    if (dirty) await fireEvent.click(await screen.findByRole('button', { name: 'Close tab' }));
    await waitFor(() => expect(gotoMock).toHaveBeenCalledWith('/events'));
    expect(original).toHaveFocus();
    expect(original).toBeInTheDocument();
    expect(get(ideState).activeDocumentId).toBe('/operator');
    await finish();
    await waitFor(() => expect(screen.getByRole('tab', { name: 'Verification' })).toHaveFocus());
    expect(screen.queryByRole('tab', { name: /^Operator/ })).not.toBeInTheDocument();
    expect(get(ideState).activeDocumentId).toBe('/events');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it.each([
    ['Operator', 'Verification', 'ArrowRight'],
    ['Verification', 'Operator', 'End'],
  ])('moves focus from inactive %s to %s without activating it', async (closing, neighbor, key) => {
    render(IDEShell);
    for (const path of ['/operator', '/events', '/hl7']) {
      pageStore.set({ url: new URL(`http://localhost${path}`) });
      await tick();
    }
    const active = screen.getByRole('tab', { name: 'HL7 / Intake' });
    active.focus();
    await fireEvent.keyDown(active, { key });
    const original = screen.getByRole('tab', { name: closing });
    expect(original).toHaveFocus();
    await fireEvent.keyDown(original, { key: 'Delete' });
    await waitFor(() => expect(screen.getByRole('tab', { name: neighbor })).toHaveFocus());
    expect(original).not.toBeInTheDocument();
    expect(active).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('tab', { name: neighbor })).toHaveAttribute('aria-selected', 'false');
    expect(get(pageStore).url.pathname).toBe('/hl7');
    expect(gotoMock).not.toHaveBeenCalled();
  });

  it.each(['/hl7', '/'])('focuses Home when deleting the sole editor at %s', async (path) => {
    pageStore.set({ url: new URL(`http://localhost${path}`) });
    render(IDEShell);
    await tick();
    const original = within(screen.getByRole('tablist', { name: 'Open editors' })).getByRole('tab');
    original.focus();
    await fireEvent.keyDown(original, { key: 'Delete' });
    await waitFor(() => expect(screen.getByRole('tab', { name: 'Home' })).toHaveFocus());
    expect(get(pageStore).url.pathname).toBe('/');
    expect(get(ideState).documents.map((doc) => doc.id)).toEqual(['/']);
  });

  it.each(['Keep open', 'Escape'])('returns focus after dirty-close %s and permits a fresh keyboard retry', async (cancel) => {
    render(IDEShell);
    pageStore.set({ url: new URL('http://localhost/operator') });
    markDirty('/operator');
    await tick();
    const original = screen.getByRole('tab', { name: /^Operator/ });
    original.focus();
    await fireEvent.keyDown(original, { key: 'Delete' });
    const dialog = await screen.findByRole('dialog', { name: 'Close Operator?' });
    if (cancel === 'Escape') await fireEvent.keyDown(dialog, { key: 'Escape' });
    else await fireEvent.click(within(dialog).getByRole('button', { name: 'Keep open' }));
    await waitFor(() => expect(original).toHaveFocus());
    expect(get(pageStore).url.pathname).toBe('/operator');
    expect(gotoMock).not.toHaveBeenCalled();
    await fireEvent.keyDown(original, { key: 'Delete' });
    await fireEvent.click(await screen.findByRole('button', { name: 'Close tab' }));
    await waitFor(() => expect(screen.getByRole('tab', { name: 'HL7 / Intake' })).toHaveFocus());
    expect(original).not.toBeInTheDocument();
  });

  it.each(['reject', 'cancel'])('restores the original focused tab when close navigation ends with %s', async (outcome) => {
    render(IDEShell);
    pageStore.set({ url: new URL('http://localhost/operator') });
    markDirty('/operator');
    await tick();
    const original = screen.getByRole('tab', { name: /^Operator/ });
    original.focus();
    gotoMock.mockImplementationOnce(async () => {
      original.blur();
      if (outcome === 'reject') throw new Error('load failed');
    });
    await fireEvent.keyDown(original, { key: 'Delete' });
    await fireEvent.click(await screen.findByRole('button', { name: 'Close tab' }));
    await waitFor(() => expect(original).toHaveFocus());
    expect(original).toHaveAttribute('aria-selected', 'true');
    expect(get(pageStore).url.pathname).toBe('/operator');
    await fireEvent.keyDown(original, { key: 'Delete' });
    expect(await screen.findByRole('dialog', { name: 'Close Operator?' })).toBeInTheDocument();
  });

  it('does not reuse canceled keyboard focus for a later close-button request', async () => {
    render(IDEShell);
    pageStore.set({ url: new URL('http://localhost/operator') });
    markDirty('/operator');
    await tick();
    const original = screen.getByRole('tab', { name: /^Operator/ });
    original.focus();
    await fireEvent.keyDown(original, { key: 'Delete' });
    await fireEvent.click(await screen.findByRole('button', { name: 'Keep open' }));
    await waitFor(() => expect(original).toHaveFocus());
    const closeButton = screen.getByRole('button', { name: 'Close Operator' });
    closeButton.focus();
    await fireEvent.click(closeButton);
    await fireEvent.click(await screen.findByRole('button', { name: 'Close tab' }));
    await waitFor(() => expect(original).not.toBeInTheDocument());
    expect(screen.getByRole('tab', { name: 'HL7 / Intake' })).not.toHaveFocus();
  });

  it.each([
    ['/hl7', 'goto', false],
    ['/operator', 'goto', true],
    ['/events', 'goto', true],
    ['/events', 'goto', false],
    ['/operator', 'link', true],
    ['/events', 'link', true],
    ['/events', 'link', false],
    ['/operator', 'popstate', true],
    ['/events', 'popstate', true],
    ['/events', 'popstate', false],
  ] as const)('leaves newer page focus alone after a superseded close ends at %s by %s (via HL7=%s)', async (finalPath, navigationType, viaHL7) => {
    render(IDEShell);
    for (const path of ['/operator', '/events', '/operator']) {
      pageStore.set({ url: new URL(`http://localhost${path}`) });
      await tick();
    }
    const original = screen.getByRole('tab', { name: 'Operator' });
    original.focus();
    let finish!: () => void;
    gotoMock.mockImplementationOnce(() => new Promise<void>((resolveGoto) => { finish = resolveGoto; }));
    await fireEvent.keyDown(original, { key: 'Delete' });
    expect(gotoMock).toHaveBeenCalledWith('/events');
    async function navigate(path: string): Promise<void> {
      if (navigationType === 'goto') {
        const title = path === '/hl7' ? 'HL7 / Intake' : path === '/operator' ? 'Operator' : 'Verification';
        await fireEvent.click(screen.getByRole('tab', { name: title }));
      } else {
        attemptNavigation(path, navigationType, navigationType === 'popstate' ? -1 : undefined);
        await tick();
      }
      expect(get(pageStore).url.pathname).toBe(path);
    }
    if (viaHL7) await navigate('/hl7');
    await navigate(finalPath);

    const editor = document.createElement('textarea');
    document.body.append(editor);
    editor.focus();
    try {
      finish();
      // Let both goto completion and the close handler's render tick finish.
      await new Promise<void>((resolve) => setTimeout(resolve, 0));
      expect(editor).toHaveFocus();
      expect(original).toBeInTheDocument();
      expect(original).toHaveAttribute('aria-selected', String(finalPath === '/operator'));
      expect(get(ideState).activeDocumentId).toBe(finalPath);
    } finally {
      editor.remove();
    }
  });

  it('does not navigate or hand off focus when a confirmed tab has already disappeared', async () => {
    render(IDEShell);
    pageStore.set({ url: new URL('http://localhost/operator') });
    markDirty('/operator');
    await tick();
    const original = screen.getByRole('tab', { name: /^Operator/ });
    original.focus();
    await fireEvent.keyDown(original, { key: 'Delete' });
    const confirm = await screen.findByRole('button', { name: 'Close tab' });
    closeTab('/operator');
    await tick();
    await fireEvent.click(confirm);
    expect(gotoMock).not.toHaveBeenCalled();
    expect(screen.getByRole('tab', { name: 'HL7 / Intake' })).not.toHaveFocus();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('returns to the latest session from editor tabs, the activity bar and commands', async () => {
    pageStore.set({ url: new URL('http://localhost/hl7?session=first') });
    render(IDEShell);
    await tick();
    pageStore.set({ url: new URL('http://localhost/hl7?session=second') });
    await tick();

    for (const method of ['tab', 'activity', 'command']) {
      pageStore.set({ url: new URL('http://localhost/operator?receipt=receipt-1') });
      await tick();
      gotoMock.mockClear();
      if (method === 'tab') {
        await fireEvent.click(screen.getByRole('tab', { name: 'HL7 / Intake' }));
      } else if (method === 'activity') {
        await fireEvent.click(screen.getByRole('button', { name: 'HL7 / Intake' }));
      } else {
        await fireEvent.click(screen.getByRole('button', { name: 'Open commands' }));
        await fireEvent.click(await screen.findByRole('option', { name: /Go to HL7/ }));
      }
      expect(gotoMock).toHaveBeenCalledWith('/hl7?session=second');
      await tick();
      expect(get(ideState).documents.filter((doc) => doc.id === '/hl7')).toHaveLength(1);
    }

    await fireEvent.click(screen.getByLabelText('Close HL7 / Intake'));
    expect(gotoMock).toHaveBeenLastCalledWith('/operator?receipt=receipt-1');
  });

  it('clears the remembered selection on an explicit navigation to the base route', async () => {
    pageStore.set({ url: new URL('http://localhost/hl7?session=first') });
    render(IDEShell);
    await tick();
    pageStore.set({ url: new URL('http://localhost/hl7') });
    await tick();
    pageStore.set({ url: new URL('http://localhost/operator') });
    await tick();
    await fireEvent.click(screen.getByRole('tab', { name: 'HL7 / Intake' }));
    expect(gotoMock).toHaveBeenCalledWith('/hl7');
  });

  it('opens the bottom panel when a panel tab is clicked', async () => {
    render(IDEShell);
    await tick();

    expect(get(ideState).bottomPanelOpen).toBe(false);

    // Name carries a diagnostics badge count (e.g. "Problems 3"), so match by prefix.
    await fireEvent.click(screen.getByRole('tab', { name: /^Problems/ }));

    expect(get(ideState).bottomPanelOpen).toBe(true);
    expect(get(ideState).activePanelTab).toBe('problems');
  });

  it('has no split workspace: Cmd+\\ is left alone', async () => {
    render(IDEShell);
    await tick();

    await fireEvent.keyDown(window, { key: '\\', metaKey: true });

    expect(screen.queryByText('Split workspace')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Close split workspace' })).not.toBeInTheDocument();
  });

  it('reads the journey evidence on mount and again on each route change', async () => {
    render(IDEShell);
    await tick();
    expect(refreshJourneyEvidence).toHaveBeenCalledTimes(1);

    pageStore.set({ url: new URL('http://localhost/profiles') });
    await tick();
    expect(refreshJourneyEvidence).toHaveBeenCalledTimes(2);

    // Same route, new search params: no extra read.
    pageStore.set({ url: new URL('http://localhost/profiles?tab=yaml') });
    await tick();
    expect(refreshJourneyEvidence).toHaveBeenCalledTimes(2);

    // Leaving the journey reads again; moving between routes outside it does not.
    pageStore.set({ url: new URL('http://localhost/operator') });
    await tick();
    expect(refreshJourneyEvidence).toHaveBeenCalledTimes(3);
    pageStore.set({ url: new URL('http://localhost/connections') });
    await tick();
    expect(refreshJourneyEvidence).toHaveBeenCalledTimes(3);
  });

  it('asks before closing a tab with unsaved changes', async () => {
    render(IDEShell);
    await tick();
    pageStore.set({ url: new URL('http://localhost/workflows') });
    await tick();
    markDirty('/workflows');
    await tick();

    expect(screen.getByRole('img', { name: 'Unsaved changes' })).toBeInTheDocument();
    await fireEvent.click(screen.getByLabelText('Close Workflows'));

    const dialog = await screen.findByRole('dialog', { name: 'Close Workflows?' });
    expect(dialog).toHaveTextContent('Workflows has changes that are not published yet.');
    expect(screen.getByRole('button', { name: 'Close tab' })).toHaveAttribute('data-variant', 'secondary');
    expect(gotoMock).not.toHaveBeenCalled();

    await fireEvent.click(screen.getByRole('button', { name: 'Keep open' }));
    expect(screen.queryByRole('dialog', { name: 'Close Workflows?' })).not.toBeInTheDocument();
    expect(get(ideState).documents.map((doc) => doc.id)).toContain('/workflows');

    await fireEvent.click(screen.getByLabelText('Close Workflows'));
    await fireEvent.click(await screen.findByRole('button', { name: 'Close tab' }));
    await tick();
    await tick();
    await waitFor(() => expect(get(ideState).documents.map((doc) => doc.id)).not.toContain('/workflows'));
    expect(gotoMock).toHaveBeenCalledWith('/hl7');
  });

  it.each(['tab', 'activity', 'explorer', 'command', 'link'])(
    'keeps the current URL, tab and inventory when %s navigation is cancelled', async (method) => {
      render(IDEShell);
      pageStore.set({ url: new URL('http://localhost/connections') });
      await tick();
      setDraftState('/connections', 'catalog', true);
      const before = get(ideState).documents.map((doc) => doc.id);
      const connectionsTab = screen.getByRole('tab', { name: 'Connections' });
      const intakeTab = screen.getByRole('tab', { name: 'HL7 / Intake' });
      if (method === 'tab') {
        await fireEvent.click(screen.getByRole('tab', { name: 'HL7 / Intake' }));
      } else if (method === 'activity') {
        await fireEvent.click(screen.getByRole('button', { name: 'HL7 / Intake' }));
      } else if (method === 'explorer') {
        await fireEvent.click(screen.getByRole('button', { name: 'Show explorer' }));
        await fireEvent.click(within(screen.getByRole('complementary', { name: 'Explorer' })).getByRole('link', { name: 'HL7 / Intake' }));
      } else if (method === 'command') {
        await fireEvent.click(screen.getByRole('button', { name: 'Open commands' }));
        await fireEvent.click(await screen.findByRole('option', { name: /Go to HL7/ }));
      } else {
        expect(attemptNavigation('/hl7', 'link')).toBe(false);
      }
      const dialog = await screen.findByRole('dialog', { name: 'Leave Connections?' });
      expect(get(pageStore).url.pathname).toBe('/connections');
      expect(connectionsTab).toHaveAttribute('aria-selected', 'true');
      expect(intakeTab).toHaveAttribute('aria-selected', 'false');
      await fireEvent.click(within(dialog).getByRole('button', { name: 'Stay here' }));
      expect(get(ideState).documents.map((doc) => doc.id)).toEqual(before);
      expect(get(ideState).activeTabId).toBe('/connections');
    }
  );

  it('permits only the confirmed transition and asks again after a failed navigation', async () => {
    pageStore.set({ url: new URL('http://localhost/connections') });
    render(IDEShell);
    setDraftState('/connections', 'catalog', true);
    expect(attemptNavigation('/hl7', 'link')).toBe(false);
    await fireEvent.click(await screen.findByRole('button', { name: 'Leave view' }));
    await tick();
    expect(get(pageStore).url.pathname).toBe('/hl7');
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();

    pageStore.set({ url: new URL('http://localhost/connections') });
    await tick();
    expect(attemptNavigation('/hl7')).toBe(false);
    gotoMock.mockRejectedValueOnce(new Error('network unavailable'));
    await fireEvent.click(await screen.findByRole('button', { name: 'Leave view' }));
    await tick();
    expect(get(pageStore).url.pathname).toBe('/connections');
    expect(attemptNavigation('/hl7')).toBe(false);
    expect(await screen.findByRole('dialog', { name: 'Leave Connections?' })).toBeInTheDocument();
  });

  it('keeps an active dirty tab until confirmed navigation completes, without a second prompt', async () => {
    render(IDEShell);
    pageStore.set({ url: new URL('http://localhost/connections') });
    await tick();
    setDraftState('/connections', 'catalog', true);
    let finish: (() => void) | undefined;
    gotoMock.mockImplementationOnce((href: string) => new Promise<void>((resolveGoto) => {
      finish = () => { attemptNavigation(href); resolveGoto(); };
    }));
    await fireEvent.click(screen.getByLabelText('Close Connections'));
    await fireEvent.click(await screen.findByRole('button', { name: 'Close tab' }));
    expect(get(ideState).documents.some((doc) => doc.id === '/connections')).toBe(true);
    expect(get(ideState).activeTabId).toBe('/connections');
    finish?.();
    await tick();
    await tick();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    await waitFor(() => expect(get(ideState).documents.some((doc) => doc.id === '/connections')).toBe(false));
    expect(get(ideState).activeTabId).toBe('/hl7');
  });

  it('keeps a tab after failed close navigation and asks before the next attempt', async () => {
    render(IDEShell);
    pageStore.set({ url: new URL('http://localhost/connections') });
    await tick();
    setDraftState('/connections', 'catalog', true);
    await fireEvent.click(screen.getByLabelText('Close Connections'));
    gotoMock.mockRejectedValueOnce(new Error('load failed'));
    await fireEvent.click(await screen.findByRole('button', { name: 'Close tab' }));
    await tick();
    expect(get(ideState).documents.some((doc) => doc.id === '/connections')).toBe(true);
    expect(get(ideState).activeTabId).toBe('/connections');
    expect(attemptNavigation('/hl7')).toBe(false);
    expect(await screen.findByRole('dialog', { name: 'Leave Connections?' })).toBeInTheDocument();
  });

  it('allows retained same-route buffers and shared drafts, but protects native unload', async () => {
    pageStore.set({ url: new URL('http://localhost/connections') });
    render(IDEShell);
    setDraftState('/connections', 'catalog', true);
    expect(attemptNavigation('/connections?connection=another')).toBe(true);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    setDraftState('/connections', 'catalog', false);
    setDraftState('/profiles', 'in-memory-builder', true, false);
    expect(attemptNavigation('/profiles')).toBe(true);
    await tick();
    expect(attemptNavigation('/hl7')).toBe(true);
    await tick();
    expect(attemptNavigation('/hl7', 'leave')).toBe(false);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    setDraftState('/profiles', 'in-memory-builder', false);
    expect(attemptNavigation('/hl7', 'leave')).toBe(true);
  });

  it('resumes Back with its original history delta rather than pushing a new entry', async () => {
    window.history.replaceState({}, '', '/connections');
    pageStore.set({ url: new URL(window.location.href) });
    render(IDEShell);
    await tick();
    setDraftState('/connections', 'catalog', true);
    const historyGo = vi.spyOn(window.history, 'go').mockImplementation(() => {});
    try {
      expect(attemptNavigation('/hl7', 'popstate', -2)).toBe(false);
      await fireEvent.click(await screen.findByRole('button', { name: 'Leave view' }));
      await tick();
      expect(historyGo).toHaveBeenCalledWith(-2);
      expect(gotoMock).not.toHaveBeenCalled();
      expect(attemptNavigation('/hl7', 'popstate', -2)).toBe(true);
      await tick();
      expect(get(ideState).activeTabId).toBe('/hl7');
    } finally {
      historyGo.mockRestore();
      window.history.replaceState({}, '', '/');
    }
  });

  it('opens one palette on /hl7 with Cmd/Ctrl+K and the header button, listing route commands first', async () => {
    const preview = vi.fn();
    registerCommands('hl7', [{ id: 'preview', label: 'Preview (parse)', group: 'HL7', run: preview }], { priority: 10 });
    render(IDEShell);
    await tick();

    await fireEvent.keyDown(window, { key: 'k', ctrlKey: true });
    const palette = await screen.findByRole('dialog', { name: 'Commands' });
    expect(screen.getAllByRole('dialog')).toHaveLength(1);
    const options = within(palette).getAllByRole('option');
    expect(options[0]).toHaveTextContent('Preview (parse)');
    expect(within(palette).getByRole('option', { name: /Go to Operator/ })).toBeInTheDocument();

    await fireEvent.keyDown(within(palette).getByRole('textbox', { name: 'Search commands' }), { key: 'Enter' });
    expect(preview).toHaveBeenCalledTimes(1);
    expect(screen.queryByRole('dialog', { name: 'Commands' })).not.toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: 'Open commands' }));
    expect(await screen.findByRole('dialog', { name: 'Commands' })).toBeInTheDocument();
    expect(screen.getAllByRole('dialog')).toHaveLength(1);
  });

  it.each(['input', 'textarea', 'editor'])('opens commands from a focused %s and returns focus on Escape', async (kind) => {
    render(IDEShell);
    await tick();
    const field = document.createElement(kind === 'editor' ? 'div' : kind);
    if (kind === 'editor') {
      field.contentEditable = 'true';
      field.tabIndex = 0;
    }
    // An editor must see the shortcut as handled before its own key binding.
    const editorBinding = vi.fn((event: KeyboardEvent) => {
      expect(event.defaultPrevented).toBe(true);
    });
    field.addEventListener('keydown', editorBinding);
    document.body.append(field);
    try {
      field.focus();
      await fireEvent.keyDown(field, { key: 'k', ctrlKey: true });
      const palette = await screen.findByRole('dialog', { name: 'Commands' });
      const query = within(palette).getByRole('textbox', { name: 'Search commands' });
      expect(query).toHaveFocus();
      expect(editorBinding).toHaveBeenCalledTimes(1);
      await fireEvent.keyDown(query, { key: 'Escape' });
      expect(screen.queryByRole('dialog', { name: 'Commands' })).not.toBeInTheDocument();
      expect(field).toHaveFocus();
    } finally {
      field.remove();
    }
  });

  it('keeps a repeated palette shortcut in the existing palette without resetting its query', async () => {
    render(IDEShell);
    await fireEvent.keyDown(window, { key: 'k', metaKey: true });
    const query = await screen.findByRole('textbox', { name: 'Search commands' });
    await fireEvent.input(query, { target: { value: 'operator' } });
    const event = new KeyboardEvent('keydown', { key: 'k', metaKey: true, bubbles: true, cancelable: true });
    query.dispatchEvent(event);
    await tick();
    expect(event.defaultPrevented).toBe(true);
    expect(screen.getAllByRole('dialog')).toHaveLength(1);
    expect(query).toHaveValue('operator');
  });

  it('leaves another dialog and modified or composing shortcuts alone', async () => {
    render(IDEShell);
    await tick();
    for (const modifier of [{ altKey: true }, { shiftKey: true }, { isComposing: true }]) {
      await fireEvent.keyDown(window, { key: 'k', ctrlKey: true, ...modifier });
      expect(screen.queryByRole('dialog', { name: 'Commands' })).not.toBeInTheDocument();
    }
    markDirty('/hl7');
    await tick();
    await fireEvent.click(screen.getByLabelText('Close HL7 / Intake'));
    const dialog = await screen.findByRole('dialog', { name: 'Close HL7 / Intake?' });
    await fireEvent.keyDown(within(dialog).getByRole('button', { name: 'Keep open' }), { key: 'k', ctrlKey: true });
    expect(screen.queryByRole('dialog', { name: 'Commands' })).not.toBeInTheDocument();
    expect(dialog).toBeInTheDocument();
  });

  it('shows the stage breadcrumb for the current document', async () => {
    pageStore.set({ url: new URL('http://localhost/profiles') });
    render(IDEShell);
    await tick();

    const crumbs = screen.getByRole('navigation', { name: 'Breadcrumb' });
    expect(crumbs).toHaveTextContent('Normalization');
    expect(crumbs.querySelector('[aria-current="page"]')).toHaveTextContent('Profiles');
  });

  it('lists the layout shortcuts in the palette Workspace category', async () => {
    pageStore.set({ url: new URL('http://localhost/operator') });
    render(IDEShell);
    await tick();

    await fireEvent.keyDown(window, { key: 'k', ctrlKey: true });
    const palette = await screen.findByRole('dialog', { name: 'Commands' });
    expect(palette).toHaveTextContent('Workspace');
    const sidebar = screen.getByRole('option', { name: /Toggle explorer/ });
    const panel = screen.getByRole('option', { name: /Toggle bottom panel/ });
    expect(sidebar.querySelector('kbd')?.textContent).toMatch(/B$/);
    expect(panel.querySelector('kbd')?.textContent).toMatch(/J$/);
    // No document types without an editor behind them.
    expect(palette).not.toHaveTextContent('Documents');
    expect(screen.queryByRole('option', { name: /Open active trace|Compare events/ })).not.toBeInTheDocument();
  });

  it('offers the Connections commands and hands the page what to open', async () => {
    pageStore.set({ url: new URL('http://localhost/operator') });
    render(IDEShell);
    await tick();

    await fireEvent.keyDown(window, { key: 'k', ctrlKey: true });
    await screen.findByRole('dialog', { name: 'Commands' });
    for (const name of ['Go to Connections', 'New source connection', 'New destination connection', 'Engine properties']) {
      expect(screen.getByRole('option', { name: new RegExp(name) })).toBeInTheDocument();
    }

    await fireEvent.click(screen.getByRole('option', { name: /Engine properties/ }));
    expect(gotoMock).toHaveBeenCalledWith('/connections');
    await tick();
    expect(takeConnectionsIntent()).toEqual({ view: 'engine' });
  });

  it.each(['cancel', 'failure', 'accept'])('publishes a Connections command only after accepted arrival: %s', async (outcome) => {
    pageStore.set({ url: new URL('http://localhost/profiles') });
    render(IDEShell);
    setDraftState('/profiles', 'yaml-test', true);
    await fireEvent.click(screen.getByRole('button', { name: 'Open commands' }));
    await fireEvent.click(await screen.findByRole('option', { name: /Engine properties/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Leave Profiles?' });
    expect(takeConnectionsIntent()).toBeNull();
    if (outcome === 'cancel') {
      await fireEvent.click(within(dialog).getByRole('button', { name: 'Stay here' }));
    } else {
      if (outcome === 'failure') gotoMock.mockRejectedValueOnce(new Error('load failed'));
      await fireEvent.click(within(dialog).getByRole('button', { name: 'Leave view' }));
      await tick();
      await tick();
    }
    if (outcome === 'accept') {
      expect(get(pageStore).url.pathname).toBe('/connections');
      expect(takeConnectionsIntent()).toEqual({ view: 'engine' });
    } else {
      setDraftState('/profiles', 'yaml-test', false);
      expect(attemptNavigation('/connections')).toBe(true);
      expect(takeConnectionsIntent()).toBeNull();
    }
  });

  it('opens a Connections tab titled Connections, outside the five stages', async () => {
    pageStore.set({ url: new URL('http://localhost/connections') });
    render(IDEShell);
    await tick();

    expect(screen.getByRole('tab', { name: 'Connections' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('button', { name: 'Connections' })).toHaveAttribute('aria-current', 'true');
    const crumbs = screen.getByRole('navigation', { name: 'Breadcrumb' });
    expect(crumbs.querySelector('.crumb-stage')).toBeNull();
    expect(crumbs.querySelector('[aria-current="page"]')).toHaveTextContent('Connections');
  });

  it('keeps connection state in the status bar only and offers no editor-less documents', async () => {
    render(IDEShell, { props: { connectionState: 'connected' } });
    await tick();

    const header = screen.getByRole('banner');
    expect(header).not.toHaveTextContent('Connected');
    expect(screen.getByRole('status')).toHaveTextContent('Connected');
    expect(screen.queryByRole('button', { name: 'Open new document' })).not.toBeInTheDocument();
  });

  it('toggles the sidebar with Cmd/Ctrl+B and the bottom panel with Cmd/Ctrl+J', async () => {
    render(IDEShell);
    await tick();

    expect(get(ideState).sidebarOpen).toBe(false);
    await fireEvent.keyDown(window, { key: 'b', ctrlKey: true });
    expect(get(ideState).sidebarOpen).toBe(true);

    expect(get(ideState).bottomPanelOpen).toBe(false);
    await fireEvent.keyDown(window, { key: 'j', metaKey: true });
    expect(get(ideState).bottomPanelOpen).toBe(true);
  });
});
