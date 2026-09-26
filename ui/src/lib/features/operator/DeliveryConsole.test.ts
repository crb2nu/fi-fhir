import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import DeliveryConsole from './DeliveryConsole.svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';

const { fetchDeadLettersMock, fetchCircuitsMock, fetchAttemptMock, toastsMock } = vi.hoisted(
  () => ({
    fetchDeadLettersMock: vi.fn(),
    fetchCircuitsMock: vi.fn(),
    fetchAttemptMock: vi.fn(),
    toastsMock: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
  })
);

vi.mock('./operatorApi', () => ({
  fetchDeadLetters: (...args: unknown[]) => fetchDeadLettersMock(...args),
  fetchCircuits: (...args: unknown[]) => fetchCircuitsMock(...args),
  fetchAttempt: (...args: unknown[]) => fetchAttemptMock(...args)
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

  it('opens a dead letter’s Delivery block on demand, once per row', async () => {
    fetchDeadLettersMock.mockResolvedValue(page([deadLetter()]));
    fetchAttemptMock.mockResolvedValue({
      attemptId: 'attempt-a',
      deliveries: [
        {
          transport: 'fhir',
          outcome: 'refused',
          endpointAdvisory: 'https://fhir.example.test/r4',
          fhirResourceTypes: ['Patient', 'Encounter'],
          fhirEntryCount: 2,
          fhirOutcomeCodesAdvisory: ['invalid', 'not-found']
        }
      ]
    });
    render(DeliveryConsole);

    const toggle = await screen.findByRole('button', { name: 'Show delivery' });
    expect(toggle).toHaveAttribute('aria-expanded', 'false');
    // Nothing is fetched for a row the operator has not opened.
    expect(fetchAttemptMock).not.toHaveBeenCalled();

    await fireEvent.click(toggle);
    const block = await screen.findByRole('list', {
      name: 'Destination deliveries for attempt-a, newest first'
    });
    expect(fetchAttemptMock).toHaveBeenCalledWith('attempt-a');
    expect(within(block).getByText('FHIR')).toBeInTheDocument();
    expect(within(block).getByText('Refused')).toBeInTheDocument();
    expect(within(block).getByText('not-found')).toBeInTheDocument();
    const hide = screen.getByRole('button', { name: 'Hide delivery' });
    expect(hide).toHaveAttribute('aria-expanded', 'true');
    expect(document.getElementById(hide.getAttribute('aria-controls') ?? '')).not.toBeNull();

    await fireEvent.click(hide);
    expect(screen.queryByRole('list', { name: /Destination deliveries/ })).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'Show delivery' }));
    expect(
      await screen.findByRole('list', { name: 'Destination deliveries for attempt-a, newest first' })
    ).toBeInTheDocument();
    expect(fetchAttemptMock).toHaveBeenCalledTimes(1);
  });

  it('renders a Delivery block failure inline in its row without a second toast', async () => {
    fetchDeadLettersMock.mockResolvedValue(page([deadLetter()]));
    fetchAttemptMock.mockRejectedValue(new Error('operator control plane unavailable'));
    render(DeliveryConsole);

    await fireEvent.click(await screen.findByRole('button', { name: 'Show delivery' }));

    expect(await screen.findByRole('alert')).toHaveTextContent(/not enabled on this deployment/i);
    expect(toastsMock.error).not.toHaveBeenCalled();
  });

  it('disables every recovery action with the missing delivery role when the identity lacks it', async () => {
    setAccessStatus({
      authenticated: true,
      authVia: 'network',
      roles: ['graphql:operator', 'integration.operator'],
      capabilities: {
        operatorRead: true,
        operatorDelivery: false,
        operatorDeployment: false,
        clinicalRead: true,
        integrationSessions: false,
        streaming: false
      },
      missingRoles: { operatorDelivery: ['integration.delivery.operator'] }
    });
    fetchDeadLettersMock.mockResolvedValue(page([deadLetter()]));
    render(DeliveryConsole);

    try {
      for (const name of ['Replay', 'Resubmit', 'Discard']) {
        const button = await screen.findByRole('button', { name });
        expect(button).toBeDisabled();
        expect(button).toHaveAttribute(
          'title',
          expect.stringMatching(/integration\.delivery\.operator.*FI_FHIR_GRAPHQL_ROLES/)
        );
      }
    } finally {
      resetAccessCapabilities();
    }
  });

  it('says so when the attempt behind a dead letter is not in the tenant', async () => {
    fetchDeadLettersMock.mockResolvedValue(page([deadLetter()]));
    fetchAttemptMock.mockResolvedValue(null);
    render(DeliveryConsole);

    await fireEvent.click(await screen.findByRole('button', { name: 'Show delivery' }));

    expect(await screen.findByRole('alert')).toHaveTextContent(/not available in your tenant/i);
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
