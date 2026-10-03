/**
 * Tests for the merged IDE shell workspace behavior.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get, writable } from 'svelte/store';
import { ideState, markDirty, resetIDEState } from './ideStore';
import { registerCommands, resetCommandRegistry } from './commandRegistry';
import { takeConnectionsIntent } from '$lib/features/connections/connectionsIntent';

const pageStore = writable({ url: new URL('http://localhost/hl7') });

const gotoMock = vi.fn(async (href: string) => {
  pageStore.set({ url: new URL(href, 'http://localhost') });
});

vi.mock('$app/stores', () => ({ page: pageStore }));
vi.mock('$app/navigation', () => ({ goto: gotoMock }));
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
    gotoMock.mockClear();
    refreshJourneyEvidence.mockClear();
    localStorage.clear();
    resetIDEState();
    resetCommandRegistry();
    pageStore.set({ url: new URL('http://localhost/hl7') });
  });

  it('offers a visible explorer toggle and restores the compact navigation on collapse', async () => {
    render(IDEShell);
    const toggle = screen.getByRole('button', { name: 'Show explorer' });
    await fireEvent.click(toggle);
    expect(screen.getByRole('complementary', { name: 'Explorer' })).toBeInTheDocument();
    expect(screen.queryByRole('navigation', { name: 'Activity bar' })).not.toBeInTheDocument();
    expect(toggle).toHaveAttribute('aria-expanded', 'true');
    await fireEvent.click(screen.getByRole('button', { name: 'Collapse explorer' }));
    expect(toggle).toHaveFocus();
    expect(screen.getByRole('navigation', { name: 'Activity bar' })).toBeInTheDocument();
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

    expect(gotoMock).toHaveBeenCalledWith('/hl7');
    expect(screen.getByRole('tab', { name: 'HL7 / Intake' })).toHaveAttribute('aria-selected', 'true');
    expect(get(ideState).activeTabId).toBe('/hl7');
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
    expect(get(ideState).documents.map((doc) => doc.id)).not.toContain('/workflows');
    expect(gotoMock).toHaveBeenCalledWith('/hl7');
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
    expect(takeConnectionsIntent()).toEqual({ view: 'engine' });
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
