import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import DeliveryConsole from './DeliveryConsole.svelte';

const { fetchDeadLettersMock, fetchCircuitsMock, toastsMock } = vi.hoisted(() => ({
  fetchDeadLettersMock: vi.fn(),
  fetchCircuitsMock: vi.fn(),
  toastsMock: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
}));

vi.mock('./operatorApi', () => ({
  fetchDeadLetters: (...args: unknown[]) => fetchDeadLettersMock(...args),
  fetchCircuits: (...args: unknown[]) => fetchCircuitsMock(...args)
}));

vi.mock('$lib/ui/toastStore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/ui/toastStore')>();
  return { ...actual, toasts: toastsMock };
});

function deadLetter(overrides: Record<string, unknown> = {}) {
  return {
    attemptId: 'attempt-a',
    active: true,
    failureCode: 'DESTINATION_UNAVAILABLE',
    failureDetail: 'FHIR destination refused the connection',
    failedAt: '2026-08-08T09:00:00.000Z',
    replayCount: 0,
    lastReplayedAt: null,
    resolution: '',
    resolvedAt: null,
    ...overrides
  };
}

function page(nodes: unknown[]) {
  return { nodes, pageInfo: { hasNextPage: false, endCursor: null } };
}

describe('DeliveryConsole', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    fetchCircuitsMock.mockResolvedValue([]);
  });

  it('renders an honest empty state when nothing is dead-lettered', async () => {
    fetchDeadLettersMock.mockResolvedValue(page([]));
    render(DeliveryConsole);

    expect(await screen.findByText(/no open dead letters/i)).toBeInTheDocument();
    // Nothing is fabricated: the empty state explains the condition rather than
    // showing placeholder rows.
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });

  it('enables every recovery action on an open dead letter', async () => {
    fetchDeadLettersMock.mockResolvedValue(page([deadLetter()]));
    render(DeliveryConsole);

    for (const name of ['Replay', 'Resubmit', 'Discard']) {
      const button = await screen.findByRole('button', { name });
      expect(button).toBeEnabled();
      expect(button).not.toHaveAttribute('title');
    }
  });

  it('disables recovery on a resolved dead letter and explains why', async () => {
    fetchDeadLettersMock.mockResolvedValue(
      page([deadLetter({ active: false, resolution: 'replayed', replayCount: 1 })])
    );
    render(DeliveryConsole, { props: { activeOnly: false } });

    const replay = await screen.findByRole('button', { name: 'Replay' });
    expect(replay).toBeDisabled();
    expect(replay).toHaveAttribute('title', expect.stringMatching(/already resolved/i));
    expect(screen.getByText(/closed by replay/i)).toBeInTheDocument();
  });

  it('dispatches the requested control action for the selected attempt', async () => {
    fetchDeadLettersMock.mockResolvedValue(page([deadLetter()]));
    const onControl = vi.fn();
    render(DeliveryConsole, { events: { control: onControl } });

    await fireEvent.click(await screen.findByRole('button', { name: 'Discard' }));

    expect(onControl).toHaveBeenCalledTimes(1);
    expect(onControl.mock.calls[0]?.[0]?.detail).toEqual({
      action: 'discard',
      attemptId: 'attempt-a'
    });
  });

  it('renders a load failure inline without adding a second toast', async () => {
    fetchDeadLettersMock.mockRejectedValue(new Error('operator control plane unavailable'));
    render(DeliveryConsole);

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent(/not enabled on this deployment/i);
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument();
    // B4: the global graphqlFetch net already toasted this failure; the panel
    // must not double-surface it.
    expect(toastsMock.error).not.toHaveBeenCalled();
  });
});
