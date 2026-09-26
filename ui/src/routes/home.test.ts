import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';

vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));
vi.mock('$app/navigation', () => ({ goto: vi.fn() }));

// Every panel's data boundary is mocked so the page renders deterministically.
const { fetchDeploymentsMock, fetchRecentSessionsMock, graphqlFetchMock } = vi.hoisted(() => ({
  fetchDeploymentsMock: vi.fn(),
  fetchRecentSessionsMock: vi.fn(),
  graphqlFetchMock: vi.fn()
}));

vi.mock('$lib/features/operator/operatorApi', () => ({
  fetchDeployments: (...args: unknown[]) => fetchDeploymentsMock(...args)
}));
vi.mock('$lib/features/dashboard/dashboardApi', () => ({
  fetchRecentSessions: (...args: unknown[]) => fetchRecentSessionsMock(...args)
}));
vi.mock('$lib/graphql/client', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/graphql/client')>();
  return { ...actual, graphqlFetch: (...args: unknown[]) => graphqlFetchMock(...args) };
});
vi.mock('$lib/features/observability/observabilityStore', async (importOriginal) => {
  const actual =
    await importOriginal<typeof import('$lib/features/observability/observabilityStore')>();
  return { ...actual, fetchAlerts: vi.fn().mockResolvedValue(undefined) };
});

const { default: HomePage } = await import('./+page.svelte');

function status(overrides: Record<string, unknown> = {}) {
  return {
    authenticated: true,
    authVia: 'network',
    principal: 'ide',
    roles: ['graphql:operator', 'integration.operator'],
    capabilities: {
      operatorRead: true,
      operatorDelivery: true,
      operatorDeployment: true,
      clinicalRead: true,
      integrationSessions: true,
      streaming: true
    },
    missingRoles: {},
    ...overrides
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  fetchDeploymentsMock.mockResolvedValue([]);
  fetchRecentSessionsMock.mockResolvedValue([]);
  graphqlFetchMock.mockResolvedValue({ health: { status: 'healthy', version: '0.0.0-test' } });
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ status: 'healthy', version: '0.0.0-test' })
    })
  );
});

afterEach(() => {
  resetAccessCapabilities();
  vi.unstubAllGlobals();
});

describe('home (operational overview)', () => {
  it('is a toolbar titled Home with no hero, recommended move, or journey copy', () => {
    render(HomePage);

    expect(screen.getByRole('heading', { level: 1, name: 'Home' })).toBeInTheDocument();
    for (const copy of [
      'Build the interface from source to destination',
      'Recommended move',
      'Mission control',
      'Start Source Intake',
      'Demo data'
    ]) {
      expect(screen.queryByText(copy)).toBeNull();
    }
  });

  it('shows Recent, Integrations, Health and Alerts from their real sources', async () => {
    setAccessStatus(status());
    render(HomePage);

    for (const name of ['Recent', 'Integrations', 'Health', 'Alerts']) {
      expect(screen.getByRole('region', { name })).toBeInTheDocument();
    }
    await waitFor(() => expect(fetchDeploymentsMock).toHaveBeenCalledTimes(1));
    expect(fetchRecentSessionsMock).toHaveBeenCalledTimes(1);
    expect(await screen.findByText('No integration is deployed.')).toBeInTheDocument();
    expect(screen.getByText('No alert source configured.')).toBeInTheDocument();
    expect(await screen.findByTestId('health-summary')).toHaveTextContent('Healthy');
  });

  it('says nothing is open and offers HL7 intake when there is no recent work', async () => {
    setAccessStatus(status());
    render(HomePage);

    expect(await screen.findByText('Nothing opened yet.')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Open HL7 intake' })).toBeInTheDocument();
  });

  it('lists recent integration sessions with their last run status', async () => {
    setAccessStatus(status());
    fetchRecentSessionsMock.mockResolvedValue([
      {
        id: 'sess_synthetic_0001',
        name: 'HL7 source profile workspace',
        updatedAt: new Date().toISOString(),
        runs: [{ id: 'run_1', status: 'completed' }]
      }
    ]);
    render(HomePage);

    const table = await screen.findByRole('table', { name: 'Recent documents and sessions' });
    expect(table).toHaveTextContent('HL7 source profile workspace');
    expect(table).toHaveTextContent('sess_synthetic_0001');
    expect(table).toHaveTextContent('completed');
  });

  it('lists integration deployments with revision, state and health', async () => {
    setAccessStatus(status());
    fetchDeploymentsMock.mockResolvedValue([
      {
        definitionRevision: {
          artifactId: 'adt-to-fhir',
          revisionId: 'rev-1',
          digest: 'sha256:' + 'a'.repeat(64)
        },
        state: 'deployed',
        health: 'healthy',
        version: 3,
        validationPassed: true,
        updatedBy: { id: 'operator@example.test' },
        updatedAt: '2026-09-25T10:00:00Z',
        updatedReason: 'initial rollout'
      }
    ]);
    render(HomePage);

    const table = await screen.findByRole('table', { name: 'Integration deployments' });
    expect(table).toHaveTextContent('adt-to-fhir');
    expect(table).toHaveTextContent('rev-1');
    expect(table).toHaveTextContent('deployed');
    expect(table).toHaveTextContent('healthy');
    // Home is read-only: lifecycle commands live on /operator.
    expect(screen.queryByRole('button', { name: 'Pause' })).toBeNull();
  });

  it('names the missing role and queries no deployments without operator read', async () => {
    setAccessStatus(
      status({
        roles: ['graphql:operator'],
        capabilities: {
          operatorRead: false,
          operatorDelivery: false,
          operatorDeployment: false,
          clinicalRead: true,
          integrationSessions: false,
          streaming: false
        },
        missingRoles: { operatorRead: ['integration.operator'] }
      })
    );
    render(HomePage);

    const panel = screen.getByTestId('integrations-panel');
    expect(panel).toHaveTextContent('integration.operator');
    expect(fetchDeploymentsMock).not.toHaveBeenCalled();
    expect(fetchRecentSessionsMock).not.toHaveBeenCalled();
    expect(panel).not.toHaveTextContent(/forbidden/i);
  });
});
