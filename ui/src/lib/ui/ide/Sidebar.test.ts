import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { writable } from 'svelte/store';
import { createWorkspaceTab, openTab, resetIDEState } from './ideStore';

const capability = vi.hoisted(() => ({ value: true }));
const sessionsCapability = writable<boolean | null>(true);
vi.mock('$lib/graphql/accessCapabilities', () => ({ integrationSessionsCapability: sessionsCapability }));
vi.mock('$lib/features/integration-session/api', () => ({ isIntegrationSessionBuildEnabled: () => capability.value }));
const fetchSessions = vi.fn();
vi.mock('$lib/features/dashboard/dashboardApi', () => ({ fetchRecentSessions: (...args: unknown[]) => fetchSessions(...args) }));

const { default: Sidebar } = await import('./Sidebar.svelte');

const onclose = vi.fn();
const onnavigate = vi.fn();
const props = { open: true, onclose, onnavigate };
const session = { id: 'session/1', name: 'ADT intake', updatedAt: '2026-10-03T12:00:00Z', runs: [{ id: 'run-1', status: 'completed' }] };

beforeEach(() => {
  localStorage.clear();
  resetIDEState();
  vi.clearAllMocks();
  capability.value = true;
  sessionsCapability.set(true);
  fetchSessions.mockResolvedValue([session]);
});

describe('Explorer', () => {
  it('resumes remembered records and shows real recent sessions', async () => {
    openTab(createWorkspaceTab('/operator', 'operator', '?receipt=receipt-1'));
    render(Sidebar, { props });
    const operator = screen.getByRole('link', { name: 'Operator' });
    expect(operator).toHaveAttribute('href', '/operator?receipt=receipt-1');
    await fireEvent.click(operator);
    expect(onnavigate).toHaveBeenCalledWith('/operator?receipt=receipt-1');
    const recent = await screen.findByRole('link', { name: /ADT intake/ });
    expect(recent).toHaveAttribute('href', '/hl7?session=session%2F1');
    await fireEvent.click(recent);
    expect(onnavigate).toHaveBeenLastCalledWith('/hl7?session=session%2F1');
    expect(fetchSessions).toHaveBeenCalledWith(8);
  });

  it('keeps modified links usable in another tab', async () => {
    render(Sidebar, { props });
    await fireEvent.click(screen.getByRole('link', { name: 'Profiles' }), { ctrlKey: true });
    expect(onnavigate).not.toHaveBeenCalled();
  });

  it('persists collapsed sections and temporarily reveals matches when filtering', async () => {
    const view = render(Sidebar, { props });
    await fireEvent.click(screen.getByRole('button', { name: 'Build' }));
    expect(screen.queryByRole('link', { name: 'Profiles' })).not.toBeInTheDocument();
    await fireEvent.input(screen.getByRole('textbox', { name: 'Filter explorer' }), { target: { value: 'profiles' } });
    expect(screen.getByRole('link', { name: 'Profiles' })).toBeInTheDocument();
    expect(screen.queryByRole('link', { name: 'Operator' })).not.toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: 'Clear explorer filter' }));
    expect(screen.getByRole('textbox', { name: 'Filter explorer' })).toHaveFocus();
    expect(screen.queryByRole('link', { name: 'Profiles' })).not.toBeInTheDocument();
    view.unmount();
    render(Sidebar, { props });
    expect(screen.getByRole('button', { name: 'Build' })).toHaveAttribute('aria-expanded', 'false');
  });

  it('filters sessions by name or id and reports an empty search', async () => {
    render(Sidebar, { props });
    await screen.findByRole('link', { name: /ADT intake/ });
    const filter = screen.getByRole('textbox', { name: 'Filter explorer' });
    await fireEvent.input(filter, { target: { value: 'session/1' } });
    expect(screen.getByRole('link', { name: /ADT intake/ })).toBeInTheDocument();
    await fireEvent.input(filter, { target: { value: 'missing' } });
    expect(screen.getByRole('status')).toHaveTextContent('No matching views or recent sessions.');
  });

  it('marks only the current intake session, and clears the mark in other views', async () => {
    fetchSessions.mockResolvedValue([session, { ...session, id: 'session/2', name: 'Second intake' }]);
    openTab(createWorkspaceTab('/hl7', 'hl7', '?session=session%2F1'));
    render(Sidebar, { props });
    const first = await screen.findByRole('link', { name: /ADT intake/ });
    const second = screen.getByRole('link', { name: /Second intake/ });
    expect(first).toHaveAttribute('aria-current', 'page');
    expect(first).toHaveClass('active');
    expect(second).not.toHaveAttribute('aria-current');

    openTab(createWorkspaceTab('/hl7', 'hl7', '?session=session%2F2'));
    await waitFor(() => expect(second).toHaveAttribute('aria-current', 'page'));
    expect(first).not.toHaveAttribute('aria-current');
    openTab(createWorkspaceTab('/operator'));
    await waitFor(() => expect(second).not.toHaveAttribute('aria-current'));
  });

  it('shows loading, a retryable error, and the empty session state', async () => {
    let reject!: (error: Error) => void;
    fetchSessions.mockReturnValueOnce(new Promise((_, no) => { reject = no; }));
    render(Sidebar, { props });
    expect(screen.getByRole('status')).toHaveTextContent('Loading sessions');
    reject(new Error('offline'));
    expect(await screen.findByText(/Could not load sessions/)).toBeInTheDocument();
    fetchSessions.mockResolvedValueOnce([]);
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh recent sessions' }));
    expect(await screen.findByText(/No saved sessions yet/)).toBeInTheDocument();
  });

  it('does not read sessions when the build or identity cannot reopen them', async () => {
    sessionsCapability.set(false);
    const view = render(Sidebar, { props });
    expect(screen.getByText(/Sessions are unavailable/)).toBeInTheDocument();
    expect(fetchSessions).not.toHaveBeenCalled();
    view.unmount();
    capability.value = false;
    sessionsCapability.set(true);
    render(Sidebar, { props });
    expect(fetchSessions).not.toHaveBeenCalled();
  });

  it('ignores stale responses after access is revoked', async () => {
    let finish!: (rows: typeof session[]) => void;
    fetchSessions.mockReturnValueOnce(new Promise((yes) => { finish = yes; }));
    render(Sidebar, { props });
    sessionsCapability.set(false);
    await waitFor(() => expect(screen.getByText(/Sessions are unavailable/)).toBeInTheDocument());
    finish([session]);
    await waitFor(() => expect(screen.queryByRole('link', { name: /ADT intake/ })).not.toBeInTheDocument());
  });

  it('does not render or fetch when closed', () => {
    render(Sidebar, { props: { ...props, open: false } });
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(fetchSessions).not.toHaveBeenCalled();
  });

  it('opens a labelled drawer and returns focus after Escape', async () => {
    const opener = document.createElement('button');
    document.body.appendChild(opener);
    opener.focus();
    const view = render(Sidebar, { props: { ...props, drawer: true } });
    const filter = await screen.findByRole('textbox', { name: 'Filter explorer' });
    await waitFor(() => expect(filter).toHaveFocus());
    expect(screen.getByRole('dialog', { name: 'Explorer' })).toHaveAttribute('aria-modal', 'true');
    await fireEvent.keyDown(filter, { key: 'Escape' });
    expect(onclose).toHaveBeenCalled();
    await view.rerender({ ...props, drawer: true, open: false });
    expect(opener).toHaveFocus();
    opener.remove();
  });
});
