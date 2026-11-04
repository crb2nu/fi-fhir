import { afterEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import {
  accessCapabilities,
  capabilityOf,
  connectionCatalogCapability,
  connectionsReadCapability,
  connectionsWriteCapability,
  controlPlaneCapability,
  missingRoles,
  missingRolesFor,
  operatorDeliveryCapability,
  operatorReadCapability,
  parseAuthStatus,
  refresh,
  refreshOnLegacyRefusal,
  refreshSoon,
  REFRESH_COALESCE_MS,
  resetAccessCapabilities,
  setAccessStatus,
  streamingCapability,
  subscriptionRoots
} from './accessCapabilities';

const credentialMocks = vi.hoisted(() => ({ requireAuthorization: vi.fn() }));
vi.mock('./credentials', () => ({ requireGraphQLAuthorization: credentialMocks.requireAuthorization }));

/** The R-A contract as documented in .loom/36 (transport grant, no service roles). */
function contractStatus(overrides: Record<string, unknown> = {}) {
  return {
    authenticated: true,
    authVia: 'network',
    principal: 'fi-fhir-ide-operator',
    roles: ['integration:preview', 'graphql:operator', 'clinical:read'],
    capabilities: {
      operatorRead: false,
      operatorDelivery: false,
      operatorDeployment: false,
      clinicalRead: true,
      integrationSessions: false,
      streaming: false,
      subscriptions: [],
      llm: { configured: true }
    },
    missingRoles: {
      operatorRead: ['integration.operator'],
      operatorDelivery: ['integration.delivery.operator'],
      operatorDeployment: ['integration.deployment.operator'],
      clinicalRead: []
    },
    ...overrides
  };
}

afterEach(() => {
  resetAccessCapabilities();
  vi.unstubAllGlobals();
  vi.useRealTimers();
  credentialMocks.requireAuthorization.mockReset();
});

describe('parseAuthStatus', () => {
  it('treats the old two-key status as unknown', () => {
    expect(parseAuthStatus({ authenticated: true, authVia: 'network' })).toEqual({ state: 'unknown' });
  });

  it('treats an unauthenticated or malformed body as unknown', () => {
    expect(parseAuthStatus({ authenticated: false })).toEqual({ state: 'unknown' });
    expect(parseAuthStatus(null)).toEqual({ state: 'unknown' });
    expect(parseAuthStatus('nope')).toEqual({ state: 'unknown' });
    expect(
      parseAuthStatus({ authenticated: true, authVia: 'network', capabilities: { streaming: true } })
    ).toEqual({ state: 'unknown' });
  });

  it('reads every capability, the subscription roots, the LLM bit and the missing roles', () => {
    const state = parseAuthStatus(
      contractStatus({
        capabilities: {
          ...contractStatus().capabilities,
          streaming: true,
          integrationSessions: true,
          subscriptions: ['integrationSessionEvents', 'sessionRunEvents', 42]
        }
      })
    );
    expect(state.state).toBe('known');
    if (state.state !== 'known') return;
    expect(state.principal).toBe('fi-fhir-ide-operator');
    expect(state.roles).toContain('graphql:operator');
    expect(state.capabilities).toEqual({
      operatorRead: false,
      operatorDelivery: false,
      operatorDeployment: false,
      clinicalRead: true,
      integrationSessions: true,
      streaming: true,
      subscriptions: ['integrationSessionEvents', 'sessionRunEvents'],
      llmConfigured: true,
      // An R-A body predates the connection catalog: those four are unknown, never false.
      connectionsRead: null,
      connectionsWrite: null,
      controlPlane: null,
      connectionCatalog: null,
      // Nor does it know phiExport (.loom/42 E-3) or definitionAuthoring (E-1).
      phiExport: null,
      definitionAuthoring: null
    });
    expect(missingRolesFor(state, 'operatorRead')).toEqual(['integration.operator']);
    expect(missingRolesFor(state, 'clinicalRead')).toEqual([]);
  });

  it('reads phiExport and its missing roles (.loom/42 E-3)', () => {
    const state = parseAuthStatus(
      contractStatus({
        capabilities: { ...contractStatus().capabilities, phiExport: false },
        missingRoles: { ...contractStatus().missingRoles, phiExport: ['integration.phi.export'] }
      })
    );
    expect(capabilityOf(state, 'phiExport')).toBe(false);
    expect(missingRolesFor(state, 'phiExport')).toEqual(['integration.phi.export']);
    const granted = parseAuthStatus(
      contractStatus({ capabilities: { ...contractStatus().capabilities, phiExport: true } })
    );
    expect(capabilityOf(granted, 'phiExport')).toBe(true);
  });

  it('reads the connection catalog capabilities and their missing roles (.loom/38 C-0)', () => {
    const state = parseAuthStatus(
      contractStatus({
        capabilities: {
          ...contractStatus().capabilities,
          connectionsRead: true,
          connectionsWrite: false,
          controlPlane: true,
          connectionCatalog: true
        },
        missingRoles: {
          ...contractStatus().missingRoles,
          connectionsRead: [],
          connectionsWrite: ['integration.deployment.operator']
        }
      })
    );
    expect(state.state).toBe('known');
    if (state.state !== 'known') return;
    expect(capabilityOf(state, 'connectionsRead')).toBe(true);
    expect(capabilityOf(state, 'connectionsWrite')).toBe(false);
    expect(capabilityOf(state, 'controlPlane')).toBe(true);
    expect(capabilityOf(state, 'connectionCatalog')).toBe(true);
    expect(missingRolesFor(state, 'connectionsWrite')).toEqual(['integration.deployment.operator']);
    expect(missingRolesFor(state, 'connectionsRead')).toEqual([]);
  });

  it('reads definitionAuthoring and its missing roles (.loom/42 E-1)', () => {
    const state = parseAuthStatus(
      contractStatus({
        capabilities: { ...contractStatus().capabilities, definitionAuthoring: false },
        missingRoles: { ...contractStatus().missingRoles, definitionAuthoring: ['integration.deployment.operator'] }
      })
    );
    expect(capabilityOf(state, 'definitionAuthoring')).toBe(false);
    expect(missingRolesFor(state, 'definitionAuthoring')).toEqual(['integration.deployment.operator']);
  });

  it('keeps a connection capability unknown when its value is not a boolean', () => {
    const state = parseAuthStatus(
      contractStatus({
        capabilities: { ...contractStatus().capabilities, connectionsRead: 'yes', controlPlane: 1 }
      })
    );
    expect(state.state === 'known' && state.capabilities.connectionsRead).toBeNull();
    expect(state.state === 'known' && state.capabilities.controlPlane).toBeNull();
  });

  it('reports subscriptions and the LLM bit as unreported when absent', () => {
    const body = contractStatus();
    const caps: Record<string, unknown> = { ...body.capabilities };
    delete caps.subscriptions;
    delete caps.llm;
    const state = parseAuthStatus({ ...body, capabilities: caps });
    expect(state.state === 'known' && state.capabilities.subscriptions).toBeNull();
    expect(state.state === 'known' && state.capabilities.llmConfigured).toBeNull();
  });
});

describe('accessCapabilities store selectors', () => {
  it('starts unknown: every selector says null / empty', () => {
    expect(get(accessCapabilities)).toEqual({ state: 'unknown' });
    expect(get(operatorReadCapability)).toBeNull();
    expect(get(streamingCapability)).toBeNull();
    expect(get(subscriptionRoots)).toBeNull();
    expect(get(missingRoles)).toEqual({});
  });

  it('exposes the reported capability values once the gate records a status', () => {
    setAccessStatus(contractStatus());
    expect(get(operatorReadCapability)).toBe(false);
    expect(get(operatorDeliveryCapability)).toBe(false);
    expect(get(streamingCapability)).toBe(false);
    expect(get(subscriptionRoots)).toEqual([]);
    expect(get(missingRoles).operatorRead).toEqual(['integration.operator']);
    expect(capabilityOf(get(accessCapabilities), 'clinicalRead')).toBe(true);
  });

  it('exposes the connection capability stores: null when unknown or unreported', () => {
    expect(get(connectionsReadCapability)).toBeNull();
    setAccessStatus(contractStatus());
    expect(get(connectionsReadCapability)).toBeNull();
    expect(get(controlPlaneCapability)).toBeNull();
    setAccessStatus(
      contractStatus({
        capabilities: {
          ...contractStatus().capabilities,
          connectionsRead: false,
          connectionsWrite: false,
          controlPlane: false,
          connectionCatalog: false
        }
      })
    );
    expect(get(connectionsReadCapability)).toBe(false);
    expect(get(connectionsWriteCapability)).toBe(false);
    expect(get(controlPlaneCapability)).toBe(false);
    expect(get(connectionCatalogCapability)).toBe(false);
  });

  it('goes back to unknown on reset', () => {
    setAccessStatus(contractStatus());
    resetAccessCapabilities();
    expect(get(operatorReadCapability)).toBeNull();
  });
});

function statusResponse(body: unknown, ok = true): Response {
  return { ok, json: async () => body } as Response;
}

function sessionsStatus(sessionsOn: boolean) {
  return contractStatus({
    capabilities: { ...contractStatus().capabilities, integrationSessions: sessionsOn, streaming: sessionsOn }
  });
}

describe('refresh', () => {
  it('re-reads the status with the current credential and publishes it', async () => {
    credentialMocks.requireAuthorization.mockResolvedValue('Bearer token');
    const fetchMock = vi.fn().mockResolvedValue(statusResponse(sessionsStatus(false)));
    vi.stubGlobal('fetch', fetchMock);
    setAccessStatus(sessionsStatus(true));

    await refresh();

    expect(fetchMock).toHaveBeenCalledWith('/api/auth/status', {
      headers: { Accept: 'application/json', Authorization: 'Bearer token' },
      cache: 'no-store'
    });
    expect(capabilityOf(get(accessCapabilities), 'integrationSessions')).toBe(false);
  });

  it('sends no Authorization header for headerless access', async () => {
    credentialMocks.requireAuthorization.mockResolvedValue(null);
    const fetchMock = vi.fn().mockResolvedValue(statusResponse(sessionsStatus(true)));
    vi.stubGlobal('fetch', fetchMock);

    await refresh();

    expect(fetchMock).toHaveBeenCalledWith('/api/auth/status', {
      headers: { Accept: 'application/json' },
      cache: 'no-store'
    });
    expect(capabilityOf(get(accessCapabilities), 'integrationSessions')).toBe(true);
  });

  it('keeps the current state when the probe fails', async () => {
    credentialMocks.requireAuthorization.mockResolvedValue(null);
    setAccessStatus(sessionsStatus(true));

    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(statusResponse({}, false)));
    await refresh();
    expect(capabilityOf(get(accessCapabilities), 'integrationSessions')).toBe(true);

    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('network down')));
    await refresh();
    expect(capabilityOf(get(accessCapabilities), 'integrationSessions')).toBe(true);

    credentialMocks.requireAuthorization.mockRejectedValue(new Error('no credential'));
    await refresh();
    expect(capabilityOf(get(accessCapabilities), 'integrationSessions')).toBe(true);
  });

  it('does not publish a probe that resolves after the credential was cleared', async () => {
    credentialMocks.requireAuthorization.mockResolvedValue('Bearer token');
    let answer: (response: Response) => void = () => {};
    vi.stubGlobal('fetch', vi.fn().mockReturnValue(new Promise<Response>((resolve) => (answer = resolve))));

    const probe = refresh();
    await vi.waitFor(() => expect(fetch).toHaveBeenCalled());
    resetAccessCapabilities();
    answer(statusResponse(sessionsStatus(true)));
    await probe;

    expect(get(accessCapabilities)).toEqual({ state: 'unknown' });
  });
});

describe('refreshSoon', () => {
  it('collapses a burst into one probe and probes again after the window', async () => {
    vi.useFakeTimers();
    credentialMocks.requireAuthorization.mockResolvedValue(null);
    const fetchMock = vi.fn().mockResolvedValue(statusResponse(sessionsStatus(false)));
    vi.stubGlobal('fetch', fetchMock);

    await Promise.all([refreshSoon(), refreshSoon(), refreshSoon()]);
    vi.advanceTimersByTime(REFRESH_COALESCE_MS - 1);
    await refreshSoon();
    expect(fetchMock).toHaveBeenCalledTimes(1);

    vi.advanceTimersByTime(1);
    await refreshSoon();
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('re-probes only for the legacy-execution refusal', async () => {
    credentialMocks.requireAuthorization.mockResolvedValue(null);
    const fetchMock = vi.fn().mockResolvedValue(statusResponse(sessionsStatus(false)));
    vi.stubGlobal('fetch', fetchMock);

    refreshOnLegacyRefusal('connection not found');
    expect(fetchMock).not.toHaveBeenCalled();

    setAccessStatus(sessionsStatus(true));
    refreshOnLegacyRefusal('Legacy integration execution is unavailable');
    await vi.waitFor(() =>
      expect(capabilityOf(get(accessCapabilities), 'integrationSessions')).toBe(false)
    );
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
