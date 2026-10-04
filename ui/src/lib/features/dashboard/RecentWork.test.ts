/**
 * Home › Recent: an integration session row reopens that session in HL7
 * intake by deep link (.loom/42 E-3).
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import { createWorkspaceTab, markDirty, openDocument, resetIDEState } from '$lib/ui/ide/ideStore';

const { gotoMock, fetchRecentSessionsMock, fetchSessionSummariesMock } = vi.hoisted(() => ({
  gotoMock: vi.fn(),
  fetchRecentSessionsMock: vi.fn(),
  fetchSessionSummariesMock: vi.fn()
}));

vi.mock('$lib/features/integration-session/api', () => ({ isIntegrationSessionBuildEnabled: () => true }));
vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$app/navigation', () => ({ goto: gotoMock }));
vi.mock('./dashboardApi', () => ({
  fetchRecentSessions: (...args: unknown[]) => fetchRecentSessionsMock(...args),
  fetchSessionSummaries: (...args: unknown[]) => fetchSessionSummariesMock(...args)
}));

const { default: RecentWork } = await import('./RecentWork.svelte');

beforeEach(() => {
  localStorage.clear();
  resetIDEState();
  fetchRecentSessionsMock.mockResolvedValue([]);
  fetchSessionSummariesMock.mockResolvedValue({ nodes: [], hasMore: false, nextOffset: null });
  setAccessStatus({
    authenticated: true,
    authVia: 'network',
    capabilities: { operatorRead: true, integrationSessions: true }
  });
});

afterEach(() => {
  resetAccessCapabilities();
  vi.clearAllMocks();
});

describe('RecentWork session rows', () => {
  it('opens the shared browser beyond Recent and preserves it when dismissed', async () => {
    fetchSessionSummariesMock.mockResolvedValue({
      nodes: [{ id: 'older-session', name: 'Older intake', archived: false, createdAt: '2026-10-01T10:00:00Z', updatedAt: '2026-10-01T10:00:00Z', latestRun: null }],
      hasMore: false, nextOffset: null
    });
    render(RecentWork);
    const browse = screen.getByRole('button', { name: 'Browse sessions' });
    browse.focus();
    await fireEvent.click(browse);
    await fireEvent.input(screen.getByRole('textbox', { name: 'Search sessions' }), { target: { value: 'older' } });
    await screen.findByRole('link', { name: /Older intake/ });
    await fireEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(browse).toHaveFocus();
    await fireEvent.click(browse);
    expect(screen.getByRole('textbox', { name: 'Search sessions' })).toHaveValue('older');
    await fireEvent.click(screen.getByRole('link', { name: /Older intake/ }));
    expect(screen.queryByRole('dialog')).toBeNull();
    expect(gotoMock).toHaveBeenLastCalledWith('/hl7?session=older-session');
    expect(fetchRecentSessionsMock).toHaveBeenCalledWith(8);
    expect(fetchSessionSummariesMock).toHaveBeenCalledTimes(1);
  });

  it('open /hl7?session=<id>, with the id encoded', async () => {
    setAccessStatus({
      authenticated: true,
      authVia: 'network',
      capabilities: { operatorRead: true, integrationSessions: true }
    });
    fetchRecentSessionsMock.mockResolvedValue([
      {
        id: 'sess synthetic/0001',
        name: 'HL7 source profile workspace',
        updatedAt: new Date().toISOString(),
        archived: false, createdAt: '2026-10-03T12:00:00Z', latestRun: { id: 'run_1', status: 'completed', createdAt: '2026-10-03T12:00:00Z' }
      }
    ]);
    render(RecentWork);

    const table = await screen.findByRole('table', { name: 'Recent documents and sessions' });
    const row = within(table).getByTitle('Open HL7 source profile workspace');
    await fireEvent.click(row);
    expect(gotoMock).toHaveBeenCalledWith('/hl7?session=sess%20synthetic%2F0001');
  });

  it.each([false, true])('shows an open session once and preserves its dirty state (%s)', async (dirty) => {
    const doc = createWorkspaceTab('/hl7', 'hl7', '?session=sess%2F1');
    openDocument(doc);
    if (dirty) markDirty(doc.id);
    fetchRecentSessionsMock.mockResolvedValue([
      {
        id: 'sess/1',
        name: 'Saved intake session',
        updatedAt: new Date().toISOString(),
        archived: false, createdAt: '2026-10-03T12:00:00Z', latestRun: { id: 'run_1', status: 'completed', createdAt: '2026-10-03T12:00:00Z' }
      }
    ]);
    render(RecentWork);

    const row = await screen.findByTitle('Open Saved intake session');
    expect(screen.queryByTitle('Open HL7 / Intake')).not.toBeInTheDocument();
    expect(within(row).getByText(dirty ? 'unsaved' : 'completed')).toBeInTheDocument();
    await fireEvent.click(row);
    expect(gotoMock).toHaveBeenCalledWith('/hl7?session=sess%2F1');
  });

  it('keeps the open document available when recent sessions cannot load', async () => {
    openDocument(createWorkspaceTab('/hl7', 'hl7', '?session=sess%2F1'));
    fetchRecentSessionsMock.mockRejectedValue(new Error('Unavailable'));
    render(RecentWork);

    expect(await screen.findByRole('status')).toHaveTextContent('Unavailable');
    const row = screen.getByTitle('Open HL7 / Intake');
    await fireEvent.click(row);
    expect(gotoMock).toHaveBeenCalledWith('/hl7?session=sess%2F1');
  });
});
