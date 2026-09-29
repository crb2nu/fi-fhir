import { describe, expect, it } from 'vitest';
import { parseAuthStatus } from '$lib/graphql/accessCapabilities';
import {
  deliveryControlBlockedReason,
  deploymentControlBlockedReason,
  operatorPreflight
} from './operatorAccess';

function known(capabilities: Record<string, boolean>, roles: string[] = ['graphql:operator']) {
  return parseAuthStatus({
    authenticated: true,
    authVia: 'network',
    principal: 'lan',
    roles,
    capabilities: {
      operatorRead: false,
      operatorDelivery: false,
      operatorDeployment: false,
      clinicalRead: false,
      integrationSessions: false,
      streaming: false,
      ...capabilities
    },
    missingRoles: {}
  });
}

describe('operatorAccess', () => {
  it('never blocks while capabilities are unknown', () => {
    const unknown = parseAuthStatus({ authenticated: true, authVia: 'network' });
    expect(operatorPreflight(unknown)).toBeNull();
    expect(deliveryControlBlockedReason(unknown)).toBeNull();
    expect(deploymentControlBlockedReason(unknown)).toBeNull();
  });

  it('pre-flights a transport-only identity and says it holds graphql:operator', () => {
    expect(operatorPreflight(known({}))).toEqual({
      reason: 'missing-role',
      principal: 'lan',
      holdsTransportGrant: true,
      missingRoles: ['integration.operator'],
      keys: []
    });
  });

  it('says "not configured" before a missing role when the control plane is off', () => {
    const demo = known({ controlPlane: false }, ['integration:preview']);
    expect(operatorPreflight(demo)).toEqual({
      reason: 'not-configured',
      principal: 'lan',
      holdsTransportGrant: false,
      missingRoles: ['integration.operator'],
      keys: ['FI_FHIR_DATABASE_*', 'FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true']
    });
    // Even a fully granted identity cannot use a control plane that is not there.
    const granted = known({ operatorRead: true, controlPlane: false });
    expect(operatorPreflight(granted)).toMatchObject({ reason: 'not-configured', missingRoles: [] });
    // Configured and readable: nothing to pre-flight.
    expect(operatorPreflight(known({ operatorRead: true, controlPlane: true }))).toBeNull();
  });

  it('does not pre-flight once operatorRead is granted, but still gates each control plane', () => {
    const readOnly = known({ operatorRead: true });
    expect(operatorPreflight(readOnly)).toBeNull();
    expect(deliveryControlBlockedReason(readOnly)).toMatch(/integration\.delivery\.operator/);
    expect(deploymentControlBlockedReason(readOnly)).toMatch(/integration\.deployment\.operator/);

    const full = known({ operatorRead: true, operatorDelivery: true, operatorDeployment: true });
    expect(deliveryControlBlockedReason(full)).toBeNull();
    expect(deploymentControlBlockedReason(full)).toBeNull();
  });
});
