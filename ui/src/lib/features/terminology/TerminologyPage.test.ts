import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, render, screen, waitFor } from '@testing-library/svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import { toasts } from '$lib/ui/toastStore';

// The mapping-store boundary is mocked. A refusal behaves as the global
// graphqlFetch net does without a store: it toasts, then rejects.
const STORE_REFUSAL = 'terminology mapping store not configured';
const api = vi.hoisted(() => ({
  listMappings: vi.fn(),
  deleteMapping: vi.fn(),
  deleteMappingBatch: vi.fn(),
  exportMappingsCSV: vi.fn(),
  getPendingAutorouteStats: vi.fn()
}));
vi.mock('./terminologyApi', () => api);

const { default: TerminologyPage } = await import('./TerminologyPage.svelte');

function status(capabilities: Record<string, unknown> = {}) {
  return {
    authenticated: true,
    authVia: 'network',
    principal: 'e2e-ide-operator',
    roles: ['integration:preview', 'graphql:operator', 'clinical:read'],
    capabilities: {
      operatorRead: false,
      operatorDelivery: false,
      operatorDeployment: false,
      clinicalRead: true,
      integrationSessions: false,
      streaming: false,
      subscriptions: [],
      llm: { configured: false },
      ...capabilities
    },
    missingRoles: {}
  };
}

function refuseLikeTheGlobalNet() {
  toasts.error(STORE_REFUSAL);
  return Promise.reject(new Error(STORE_REFUSAL));
}

let toastError: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  toastError = vi.spyOn(toasts, 'error');
  api.listMappings.mockResolvedValue({ nodes: [], totalCount: 0 });
  api.getPendingAutorouteStats.mockResolvedValue({ pendingCount: 3 });
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
  toasts.dismissAll();
  resetAccessCapabilities();
});

describe('TerminologyPage terminologyStore capability', () => {
  it('says the store is not configured, issues no query, and fires no toast', async () => {
    api.listMappings.mockImplementation(refuseLikeTheGlobalNet);
    api.getPendingAutorouteStats.mockImplementation(refuseLikeTheGlobalNet);
    setAccessStatus(status({ terminologyStore: false }));

    render(TerminologyPage);

    const preflight = await screen.findByTestId('terminology-preflight');
    expect(preflight.dataset.reason).toBe('not-configured');
    expect(preflight.textContent).toContain('not configured on this deployment');
    expect(preflight.textContent).toContain('FI_FHIR_TERMINOLOGY_DB_URL');
    expect(screen.queryByRole('tablist')).toBeNull();
    await Promise.resolve();
    expect(api.listMappings).not.toHaveBeenCalled();
    expect(api.getPendingAutorouteStats).not.toHaveBeenCalled();
    expect(toastError).not.toHaveBeenCalled();
  });

  it('browses mappings when the store is configured', async () => {
    setAccessStatus(status({ terminologyStore: true }));

    render(TerminologyPage);

    await waitFor(() => expect(api.listMappings).toHaveBeenCalledTimes(1));
    expect(api.getPendingAutorouteStats).toHaveBeenCalledTimes(1);
    expect(screen.queryByTestId('terminology-preflight')).toBeNull();
    expect(screen.getByRole('tablist')).toBeTruthy();
    expect(toastError).not.toHaveBeenCalled();
  });

  it('keeps querying when the server does not report the flag (unknown never blocks)', async () => {
    api.listMappings.mockImplementation(refuseLikeTheGlobalNet);
    setAccessStatus(status());

    render(TerminologyPage);

    await waitFor(() => expect(api.listMappings).toHaveBeenCalledTimes(1));
    expect(screen.queryByTestId('terminology-preflight')).toBeNull();
    expect(toastError).toHaveBeenCalledWith(STORE_REFUSAL);
  });
});
