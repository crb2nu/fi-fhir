import { afterEach, describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import {
  accessCapabilities,
  capabilityOf,
  missingRoles,
  missingRolesFor,
  operatorDeliveryCapability,
  operatorReadCapability,
  parseAuthStatus,
  resetAccessCapabilities,
  setAccessStatus,
  streamingCapability,
  subscriptionRoots
} from './accessCapabilities';

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
      llmConfigured: true
    });
    expect(missingRolesFor(state, 'operatorRead')).toEqual(['integration.operator']);
    expect(missingRolesFor(state, 'clinicalRead')).toEqual([]);
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

  it('goes back to unknown on reset', () => {
    setAccessStatus(contractStatus());
    resetAccessCapabilities();
    expect(get(operatorReadCapability)).toBeNull();
  });
});
