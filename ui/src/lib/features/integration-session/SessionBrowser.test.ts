import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import type { RecentSession, SessionSummaryPage } from '$lib/features/dashboard/dashboardApi';
import SessionBrowser from './SessionBrowser.svelte';

const api = vi.hoisted(() => ({ fetch: vi.fn(), buildEnabled: true }));
vi.mock('$lib/features/dashboard/dashboardApi', () => ({ fetchSessionSummaries: api.fetch }));
vi.mock('./api', () => ({ isIntegrationSessionBuildEnabled: () => api.buildEnabled }));

function status(allowed = true, principal = 'operator') {
  setAccessStatus({ authenticated: true, authVia: 'network', principal, capabilities: { operatorRead: true, integrationSessions: allowed } });
}
function session(id: string, extra: Partial<RecentSession> = {}): RecentSession {
  return {
    id, name: `Intake ${id}`, archived: false, createdAt: '2026-10-01T10:00:00Z', updatedAt: '2026-10-02T10:00:00Z',
    latestRun: { id: `run-${id}`, status: 'completed', createdAt: '2026-10-01T10:01:00Z' }, ...extra
  };
}
function page(nodes: RecentSession[], nextOffset: number | null = null, hasMore = nextOffset !== null): SessionSummaryPage {
  return { nodes, nextOffset, hasMore };
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}
const onclose = vi.fn();
const onnavigate = vi.fn();
const props = { open: true, onclose, onnavigate };

beforeEach(() => {
  vi.clearAllMocks();
  api.buildEnabled = true;
  api.fetch.mockReset().mockResolvedValue(page([]));
  status();
});
afterEach(() => { cleanup(); resetAccessCapabilities(); });

describe('SessionBrowser', () => {
  it('queries only when opened and shows bounded metadata, identifiers and the current session', async () => {
    api.fetch.mockResolvedValue(page([session('session/a'), session('other', { name: 'Intake session/a', latestRun: null, archived: true })], 25));
    const view = render(SessionBrowser, { ...props, open: false, currentSessionId: 'session/a' });
    expect(api.fetch).not.toHaveBeenCalled();
    await view.rerender({ ...props, currentSessionId: 'session/a' });
    const dialog = screen.getByRole('dialog', { name: 'Browse sessions' });
    const rows = await within(dialog).findAllByTestId('session-browser-row');
    expect(api.fetch).toHaveBeenCalledWith({ search: '', includeArchived: false, limit: 25, offset: 0 });
    expect(rows).toHaveLength(2);
    expect(within(rows[0]!).getByRole('link')).toHaveAttribute('href', '/hl7?session=session%2Fa');
    expect(within(rows[0]!).getByRole('link')).toHaveAttribute('aria-current', 'page');
    expect(rows[0]).toHaveTextContent('Current session');
    expect(rows[0]).toHaveTextContent('completed');
    expect(rows[0]!.querySelector('time')).toHaveAttribute('datetime', '2026-10-02T10:00:00Z');
    expect(rows[1]).toHaveTextContent('other');
    expect(rows[1]).toHaveTextContent('No runs');
    expect(rows[1]).toHaveTextContent('Archived');
    await waitFor(() => expect(screen.getByRole('textbox', { name: 'Search sessions' })).toHaveFocus());
  });

  it.each(['off', 'denied', 'unknown'])('does not query when sessions are %s', async (kind) => {
    if (kind === 'off') api.buildEnabled = false;
    if (kind === 'denied') status(false);
    if (kind === 'unknown') resetAccessCapabilities();
    render(SessionBrowser, props);
    expect(screen.getByText('Sessions are unavailable for this connection.')).toBeInTheDocument();
    expect(api.fetch).not.toHaveBeenCalled();
  });

  it('searches the full tenant by name or ID and ignores results from an older search', async () => {
    api.fetch.mockResolvedValueOnce(page([session('recent')], 25));
    render(SessionBrowser, props);
    await screen.findByTestId('session-browser-row');
    const old = deferred<SessionSummaryPage>();
    api.fetch.mockReturnValueOnce(old.promise).mockResolvedValueOnce(page([session('older')]))
      .mockResolvedValueOnce(page([session('archived', { archived: true })]));
    const search = screen.getByRole('textbox', { name: 'Search sessions' });
    await fireEvent.input(search, { target: { value: 'earlier name' } });
    await fireEvent.submit(screen.getByRole('form', { name: 'Find sessions' }));
    expect(screen.queryByTestId('session-browser-row')).toBeNull();
    await fireEvent.input(search, { target: { value: 'older' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Search' }));
    expect(await screen.findByTestId('session-browser-row')).toHaveAttribute('data-session-id', 'older');
    expect(api.fetch).toHaveBeenLastCalledWith({ search: 'older', includeArchived: false, limit: 25, offset: 0 });
    old.resolve(page([session('stale')]));
    await tick();
    expect(screen.getByTestId('session-browser-row')).toHaveAttribute('data-session-id', 'older');
    await fireEvent.click(screen.getByRole('checkbox', { name: 'Include archived' }));
    expect(api.fetch).toHaveBeenLastCalledWith({ search: 'older', includeArchived: true, limit: 25, offset: 0 });
    expect(await screen.findByTestId('session-browser-row')).toHaveAttribute('data-session-id', 'archived');
  });

  it('retains rows after a load-more failure, retries that page and deduplicates moving rows', async () => {
    api.fetch.mockResolvedValueOnce(page([session('a'), session('b')], 25))
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(page([session('b', { name: 'Updated B' }), session('c')]));
    render(SessionBrowser, props);
    await screen.findAllByTestId('session-browser-row');
    await fireEvent.input(screen.getByRole('textbox', { name: 'Search sessions' }), { target: { value: 'unfinished' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Load more' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Could not load more sessions. offline');
    expect(screen.getAllByTestId('session-browser-row')).toHaveLength(2);
    await fireEvent.click(screen.getByRole('button', { name: 'Retry sessions' }));
    await waitFor(() => expect(screen.getAllByTestId('session-browser-row')).toHaveLength(3));
    expect(api.fetch).toHaveBeenLastCalledWith({ search: '', includeArchived: false, limit: 25, offset: 25 });
    expect(screen.getByText('Updated B')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Load more' })).toBeNull();
  });

  it('explains the browse cap and exposes an honest empty search', async () => {
    api.fetch.mockResolvedValueOnce(page([session('a')], null, true)).mockResolvedValueOnce(page([]));
    render(SessionBrowser, props);
    expect(await screen.findByText('More sessions are available. Refine your search to continue.')).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Load more' })).toBeNull();
    await fireEvent.input(screen.getByRole('textbox', { name: 'Search sessions' }), { target: { value: 'missing' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Search' }));
    expect(await screen.findByText('No sessions match this search.')).toBeInTheDocument();
  });

  it('retries the first page after a read failure', async () => {
    api.fetch.mockRejectedValueOnce(new Error('offline')).mockResolvedValueOnce(page([session('recovered')]));
    render(SessionBrowser, props);
    await fireEvent.click(await screen.findByRole('button', { name: 'Retry sessions' }));
    expect(await screen.findByTestId('session-browser-row')).toHaveAttribute('data-session-id', 'recovered');
    expect(api.fetch).toHaveBeenCalledTimes(2);
  });

  it('retains search and results while closed, returning focus to the opener', async () => {
    const opener = document.createElement('button');
    document.body.appendChild(opener);
    opener.focus();
    api.fetch.mockResolvedValue(page([session('saved')]));
    const view = render(SessionBrowser, props);
    await screen.findByTestId('session-browser-row');
    await fireEvent.input(screen.getByRole('textbox', { name: 'Search sessions' }), { target: { value: 'saved' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Search' }));
    await waitFor(() => expect(api.fetch).toHaveBeenCalledTimes(2));
    await fireEvent.keyDown(screen.getByRole('textbox', { name: 'Search sessions' }), { key: 'Escape' });
    expect(onclose).toHaveBeenCalledTimes(1);
    await view.rerender({ ...props, open: false });
    expect(opener).toHaveFocus();
    await view.rerender(props);
    expect(screen.getByRole('textbox', { name: 'Search sessions' })).toHaveValue('saved');
    expect(screen.getByTestId('session-browser-row')).toHaveAttribute('data-session-id', 'saved');
    expect(api.fetch).toHaveBeenCalledTimes(2);
    opener.remove();
  });

  it('invalidates rows for revoked access and an allowed identity change', async () => {
    const old = deferred<SessionSummaryPage>();
    api.fetch.mockReturnValueOnce(old.promise).mockResolvedValueOnce(page([session('new-identity')]));
    render(SessionBrowser, props);
    await waitFor(() => expect(api.fetch).toHaveBeenCalledTimes(1));
    status(false);
    await screen.findByText('Sessions are unavailable for this connection.');
    old.resolve(page([session('previous-identity')]));
    await tick();
    expect(screen.queryByTestId('session-browser-row')).toBeNull();
    status(true, 'second-operator');
    expect(await screen.findByTestId('session-browser-row')).toHaveAttribute('data-session-id', 'new-identity');
    api.fetch.mockResolvedValueOnce(page([session('third-identity')]));
    status(true, 'third-operator');
    await waitFor(() => expect(screen.getByTestId('session-browser-row')).toHaveAttribute('data-session-id', 'third-identity'));
  });

  it('closes before requesting one guarded navigation and leaves modified links native', async () => {
    api.fetch.mockResolvedValue(page([session('saved/a')]));
    const order: string[] = [];
    onclose.mockImplementation(() => { order.push('close'); });
    onnavigate.mockImplementation(() => { order.push('navigate'); });
    render(SessionBrowser, props);
    const link = await screen.findByRole('link', { name: /Intake saved\/a/ });
    await fireEvent.click(link, { ctrlKey: true });
    expect(onclose).not.toHaveBeenCalled();
    expect(onnavigate).not.toHaveBeenCalled();
    await fireEvent.click(link);
    expect(order).toEqual(['close', 'navigate']);
    expect(onnavigate).toHaveBeenCalledExactlyOnceWith('/hl7?session=saved%2Fa');
  });
});
