/**
 * Home › Recent: an integration session row reopens that session in HL7
 * intake by deep link (.loom/42 E-3).
 */
import { afterEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';

const { gotoMock, fetchRecentSessionsMock } = vi.hoisted(() => ({
  gotoMock: vi.fn(),
  fetchRecentSessionsMock: vi.fn()
}));

vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$app/navigation', () => ({ goto: gotoMock }));
vi.mock('./dashboardApi', () => ({
  fetchRecentSessions: (...args: unknown[]) => fetchRecentSessionsMock(...args)
}));

const { default: RecentWork } = await import('./RecentWork.svelte');

afterEach(() => {
  resetAccessCapabilities();
  vi.clearAllMocks();
});

describe('RecentWork session rows', () => {
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
        runs: [{ id: 'run_1', status: 'completed' }]
      }
    ]);
    render(RecentWork);

    const table = await screen.findByRole('table', { name: 'Recent documents and sessions' });
    const row = within(table).getByTitle('Open HL7 source profile workspace');
    await fireEvent.click(row);
    expect(gotoMock).toHaveBeenCalledWith('/hl7?session=sess%20synthetic%2F0001');
  });
});
