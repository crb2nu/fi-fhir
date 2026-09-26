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
      principal: 'lan',
      holdsTransportGrant: true,
      missingRoles: ['integration.operator']
    });
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
