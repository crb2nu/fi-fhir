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
    expect(screen.getByRole('heading', { name: 'Source Intake' })).toBeInTheDocument();
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

  it('toggles split workspace with Cmd+\\', async () => {
    render(IDEShell);
    await tick();

    expect(screen.queryByText('Split workspace')).not.toBeInTheDocument();

    await fireEvent.keyDown(window, { key: '\\', metaKey: true });

    expect(screen.getByText('Split workspace')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Close split workspace' })).toBeInTheDocument();
  });
});
