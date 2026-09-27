import { describe, expect, it } from 'vitest';
import { parseAuthStatus } from '$lib/graphql/accessCapabilities';
import { clinicalReadPreflight, compatibilityGrantPreflight, roleBlockedReason } from './rolePreflight';

const OPERATOR_BUNDLE = [
  'integration:preview',
  'graphql:operator',
  'clinical:read',
  'integration.operator',
  'integration.delivery.operator',
  'integration.deployment.operator'
];

function known(roles: string[], clinicalRead: boolean, missingRoles: Record<string, string[]> = {}) {
  return parseAuthStatus({
    authenticated: true,
    authVia: 'network',
    principal: 'fi-fhir-demo-visitor',
    roles,
    capabilities: {
      operatorRead: false,
      operatorDelivery: false,
      operatorDeployment: false,
      clinicalRead,
      integrationSessions: false,
      streaming: false
    },
    missingRoles
  });
}

describe('rolePreflight', () => {
  it('never blocks while capabilities are unknown', () => {
    const unknown = parseAuthStatus({ authenticated: true, authVia: 'network' });
    expect(compatibilityGrantPreflight(unknown)).toBeNull();
    expect(clinicalReadPreflight(unknown)).toBeNull();
  });

  it('pre-flights the preview-only demo identity on both gates, naming the roles', () => {
    const visitor = known(['integration:preview'], false, { clinicalRead: ['clinical:read'] });
    expect(compatibilityGrantPreflight(visitor)).toEqual({
      principal: 'fi-fhir-demo-visitor',
      missingRoles: ['graphql:operator']
    });
    expect(clinicalReadPreflight(visitor)).toEqual({
      principal: 'fi-fhir-demo-visitor',
      missingRoles: ['clinical:read']
    });
  });

  it('does not pre-flight the operator bundle', () => {
    const bundle = known(OPERATOR_BUNDLE, true);
    expect(compatibilityGrantPreflight(bundle)).toBeNull();
    expect(clinicalReadPreflight(bundle)).toBeNull();
  });

  it('gates each surface on its own role', () => {
    // clinical:read without the grant: events readable, profiles not.
    const analyst = known(['integration:preview', 'clinical:read'], true);
    expect(clinicalReadPreflight(analyst)).toBeNull();
    expect(compatibilityGrantPreflight(analyst)?.missingRoles).toEqual(['graphql:operator']);
  });

  it('falls back to clinical:read when the server listed no missing role', () => {
    expect(clinicalReadPreflight(known(['integration:preview'], false))?.missingRoles).toEqual(['clinical:read']);
  });

  it('turns a pre-flight into a control title', () => {
    expect(roleBlockedReason(null, 'Process')).toBeNull();
    expect(
      roleBlockedReason({ principal: 'x', missingRoles: ['graphql:operator'] }, 'Process')
    ).toBe('Process needs graphql:operator, which this identity does not hold.');
  });
});
