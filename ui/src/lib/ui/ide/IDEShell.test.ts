/**
 * Tests for the merged IDE shell workspace behavior.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get, writable } from 'svelte/store';
import { ideState, resetIDEState } from './ideStore';

const pageStore = writable({ url: new URL('http://localhost/hl7') });

const gotoMock = vi.fn(async (href: string) => {
  pageStore.set({ url: new URL(href, 'http://localhost') });
});

vi.mock('$app/stores', () => ({ page: pageStore }));
vi.mock('$app/navigation', () => ({ goto: gotoMock }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));

const { default: IDEShell } = await import('./IDEShell.svelte');

// jsdom has no layout; the palette scrolls its active option into view.
Element.prototype.scrollIntoView ??= function scrollIntoView() {};

describe('IDEShell workspace', () => {
  beforeEach(() => {
    gotoMock.mockClear();
    resetIDEState();
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

  it('toggles split workspace with Cmd+\\', async () => {
    render(IDEShell);
    await tick();

    expect(screen.queryByText('Split workspace')).not.toBeInTheDocument();

    await fireEvent.keyDown(window, { key: '\\', metaKey: true });

    expect(screen.getByText('Split workspace')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Close split workspace' })).toBeInTheDocument();
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
