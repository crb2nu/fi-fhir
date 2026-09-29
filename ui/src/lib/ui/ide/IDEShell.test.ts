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
    resetIDEState();
    resetCommandRegistry();
    pageStore.set({ url: new URL('http://localhost/hl7') });
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

    const dialog = await screen.findByRole('alertdialog', { name: 'Close Workflows?' });
    expect(dialog).toHaveTextContent('Workflows has changes that are not saved on the server.');
    expect(gotoMock).not.toHaveBeenCalled();

    await fireEvent.click(screen.getByRole('button', { name: 'Keep open' }));
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
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
    const sidebar = screen.getByRole('option', { name: /Toggle sidebar/ });
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
