import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import OperatorPage from './OperatorPage.svelte';

const { fetchReceiptsMock } = vi.hoisted(() => ({ fetchReceiptsMock: vi.fn() }));

vi.mock('./operatorApi', () => ({
  fetchReceipts: (...args: unknown[]) => fetchReceiptsMock(...args),
  fetchMessageTrace: vi.fn(),
  fetchAttempt: vi.fn(),
  fetchAttempts: vi.fn(),
  fetchDeadLetters: vi.fn(),
  fetchCircuits: vi.fn(),
  fetchAttemptAudit: vi.fn(),
  fetchDeployments: vi.fn(),
  fetchDeploymentEvents: vi.fn(),
  replayDelivery: vi.fn(),
  resubmitMessage: vi.fn(),
  discardDeadLetter: vi.fn(),
  pauseDeployment: vi.fn(),
  resumeDeployment: vi.fn(),
  retireDeployment: vi.fn(),
  deployRelease: vi.fn()
}));

describe('OperatorPage — day-1 gate', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    fetchReceiptsMock.mockResolvedValue({
      nodes: [],
      pageInfo: { hasNextPage: false, endCursor: null }
    });
  });

  it('issues the Messages list query immediately on mount (today: no capability pre-flight)', async () => {
    render(OperatorPage);

    await waitFor(() => expect(fetchReceiptsMock).toHaveBeenCalledTimes(1));
    expect(await screen.findByText(/no messages match these filters/i)).toBeInTheDocument();
  });
});
