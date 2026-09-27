import { describe, expect, it } from 'vitest';
import { parseAuthStatus } from '$lib/graphql/accessCapabilities';
import { catalogPreflight, readPreflight, writeBlockedReason, writeBlockedRoles } from './connectionsAccess';

function status(capabilities: Record<string, unknown>, missingRoles: Record<string, string[]> = {}) {
  return parseAuthStatus({
    authenticated: true,
    authVia: 'network',
    principal: 'e2e-ide-operator',
    roles: ['graphql:operator'],
    capabilities: {
      operatorRead: true,
      operatorDelivery: true,
      operatorDeployment: true,
      clinicalRead: true,
      integrationSessions: true,
      streaming: true,
      ...capabilities
    },
    missingRoles
  });
}

describe('connectionsAccess — the honest states in the spec precedence', () => {
  it('never blocks while capabilities are unknown', () => {
    const unknown = parseAuthStatus({ authenticated: true, authVia: 'network' });
    expect(catalogPreflight(unknown)).toBeNull();
    expect(readPreflight(unknown)).toBeNull();
    expect(writeBlockedRoles(unknown)).toBeNull();
  });

  it('never blocks on an API that predates the connection capabilities', () => {
    const older = status({});
    expect(catalogPreflight(older)).toBeNull();
    expect(readPreflight(older)).toBeNull();
    expect(writeBlockedRoles(older)).toBeNull();
  });

  it('says not configured before it says a role is missing', () => {
    const state = status(
      { controlPlane: false, connectionCatalog: false, connectionsRead: false },
      { connectionsRead: ['integration.operator'] }
    );
    expect(catalogPreflight(state)).toEqual({
      reason: 'not-configured',
      keys: ['FI_FHIR_DATABASE_*', 'FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true']
    });
    // The engine runtime does not need the catalog, but it does need the role.
    expect(readPreflight(state)?.missingRoles).toEqual(['integration.operator']);
  });

  it('names the reported missing role, or the read role when none is reported', () => {
    const state = status(
      { controlPlane: true, connectionCatalog: true, connectionsRead: false },
      { connectionsRead: ['integration.operator'] }
    );
    expect(catalogPreflight(state)).toEqual({
      reason: 'missing-role',
      principal: 'e2e-ide-operator',
      missingRoles: ['integration.operator']
    });
    expect(readPreflight(status({ connectionsRead: false }))?.missingRoles).toEqual(['integration.operator']);
  });

  it('makes the forms read only without connectionsWrite, naming the role', () => {
    const state = status(
      { controlPlane: true, connectionCatalog: true, connectionsRead: true, connectionsWrite: false },
      { connectionsWrite: ['integration.deployment.operator'] }
    );
    expect(catalogPreflight(state)).toBeNull();
    expect(writeBlockedRoles(state)).toEqual(['integration.deployment.operator']);
    expect(writeBlockedReason(writeBlockedRoles(state))).toBe(
      'Read only: this identity does not hold integration.deployment.operator.'
    );
    expect(writeBlockedReason(null)).toBeUndefined();
  });
});
