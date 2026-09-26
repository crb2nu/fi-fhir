import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import OperatorPage from './OperatorPage.svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';

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

/** A status body in R-A's contract shape. */
function status(operatorRead: boolean, extra: Record<string, unknown> = {}) {
  return {
    authenticated: true,
    authVia: 'network',
    principal: 'fi-fhir-ide-operator',
    roles: operatorRead
      ? ['graphql:operator', 'integration.operator']
      : ['integration:preview', 'graphql:operator', 'clinical:read'],
    capabilities: {
      operatorRead,
      operatorDelivery: operatorRead,
      operatorDeployment: operatorRead,
      clinicalRead: true,
      integrationSessions: false,
      streaming: false,
      subscriptions: [],
      llm: { configured: false }
    },
    missingRoles: operatorRead ? {} : { operatorRead: ['integration.operator'] },
    ...extra
  };
}

describe('OperatorPage — capability pre-flight', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    resetAccessCapabilities();
    fetchReceiptsMock.mockResolvedValue({
      nodes: [],
      pageInfo: { hasNextPage: false, endCursor: null }
    });
  });

  afterEach(() => {
    resetAccessCapabilities();
  });

  it('day-1 gate, inverted: without operatorRead it renders the pre-flight and issues no query', async () => {
    setAccessStatus(status(false));
    render(OperatorPage);

    const preflight = await screen.findByTestId('operator-preflight');
    expect(preflight).toHaveAttribute('data-missing-roles', 'integration.operator');
    expect(preflight).toHaveTextContent('graphql:operator');
    expect(preflight).toHaveTextContent('integration.operator');
    expect(preflight).toHaveTextContent('FI_FHIR_GRAPHQL_ROLES');
    expect(preflight).toHaveTextContent('FI_FHIR_GRAPHQL_ACCESS_PRINCIPALS');
    expect(preflight).toHaveTextContent(/trusted network inherits/i);
    // No tab mounted, so nothing was sent to a control plane that would refuse it.
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument();
    await Promise.resolve();
    expect(fetchReceiptsMock).not.toHaveBeenCalled();
  });

  it('keeps the old behaviour when capabilities are unknown (two-key status): queries on mount', async () => {
    setAccessStatus({ authenticated: true, authVia: 'network' });
    render(OperatorPage);

    await waitFor(() => expect(fetchReceiptsMock).toHaveBeenCalledTimes(1));
    expect(screen.queryByTestId('operator-preflight')).not.toBeInTheDocument();
    expect(await screen.findByText(/no messages match these filters/i)).toBeInTheDocument();
  });

  it('queries on mount when the identity can read the operator plane', async () => {
    setAccessStatus(status(true));
    render(OperatorPage);

    await waitFor(() => expect(fetchReceiptsMock).toHaveBeenCalledTimes(1));
    expect(screen.queryByTestId('operator-preflight')).not.toBeInTheDocument();
  });

  it('names the missing role even when the server omits missingRoles', async () => {
    setAccessStatus(status(false, { missingRoles: {}, roles: [] }));
    render(OperatorPage);

    const preflight = await screen.findByTestId('operator-preflight');
    expect(preflight).toHaveAttribute('data-missing-roles', 'integration.operator');
    expect(preflight).toHaveTextContent(/does not hold/);
  });
});
