import { describe, expect, it } from 'vitest';
import { parseAuthStatus } from '$lib/graphql/accessCapabilities';
import { definitionsPreflight } from './definitionsAccess';

function state(capabilities: Record<string, unknown>, missingRoles: Record<string, string[]> = {}) {
  return parseAuthStatus({
    authenticated: true,
    authVia: 'network',
    principal: 'e2e',
    roles: [],
    capabilities: {
      operatorRead: true,
      connectionsRead: true,
      connectionsWrite: true,
      controlPlane: true,
      connectionCatalog: true,
      definitionAuthoring: true,
      ...capabilities
    },
    missingRoles
  });
}

describe('definitionsPreflight', () => {
  it('is null when authoring is configured and held, and when the server predates it', () => {
    expect(definitionsPreflight(state({}))).toBeNull();
    expect(definitionsPreflight(state({ definitionAuthoring: undefined }))).toBeNull();
    expect(definitionsPreflight({ state: 'unknown' })).toBeNull();
  });

  it('says not configured first: no control plane, no catalog, or no authoring service', () => {
    expect(definitionsPreflight(state({ controlPlane: false, connectionsRead: false }))?.reason).toBe('not-configured');
    expect(definitionsPreflight(state({ definitionAuthoring: false }))?.reason).toBe('not-configured');
  });

  it('names the missing read role, then the missing write roles', () => {
    expect(
      definitionsPreflight(
        state({ connectionsRead: false }, {
          connectionsRead: ['integration.operator'],
          definitionAuthoring: ['integration.operator', 'integration.deployment.operator']
        })
      )
    ).toEqual({ reason: 'missing-role', principal: 'e2e', missingRoles: ['integration.operator'] });
    expect(
      definitionsPreflight(
        state({ connectionsWrite: false }, {
          definitionAuthoring: ['integration.deployment.operator']
        })
      )
    ).toEqual({ reason: 'read-only', missingRoles: ['integration.deployment.operator'] });
  });
});
