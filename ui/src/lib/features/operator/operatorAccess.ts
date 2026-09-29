/**
 * Operator-plane pre-flight, from the identity's reported capabilities.
 *
 * The GraphQL transport admits `graphql:operator`, but the operator control
 * plane checks its own service roles (defence in depth, Slice 4.2a):
 * `integration.operator` to read, `integration.delivery.operator` for
 * replay/resubmit/discard, `integration.deployment.operator` for
 * deploy/pause/resume/retire. When the status endpoint says a role is
 * missing, the page explains where roles are granted instead of issuing
 * queries that the control plane will refuse.
 *
 * Unknown capabilities (an older API, or a bearer session) never block: the
 * page keeps its try-then-explain-inline behaviour.
 */
import { derived, type Readable } from 'svelte/store';
import {
  accessCapabilities,
  missingRolesFor,
  type AccessCapabilityState,
  type CapabilityKey
} from '$lib/graphql/accessCapabilities';

export const OPERATOR_ROLE_BUNDLE = [
  'integration.operator',
  'integration.delivery.operator',
  'integration.deployment.operator'
] as const;

/** The transport grant the UI already relies on to reach the operator API. */
export const TRANSPORT_OPERATOR_ROLE = 'graphql:operator';

/** Where an operator role is granted, in the API's environment. */
export const ROLE_GRANT_LOCATIONS: ReadonlyArray<{ variable: string; scope: string }> = [
  {
    variable: 'FI_FHIR_GRAPHQL_ROLES',
    scope: 'the static bearer credential — the trusted network inherits the same list'
  },
  {
    variable: 'FI_FHIR_GRAPHQL_ACCESS_PRINCIPALS',
    scope: 'each Cloudflare Access email'
  }
];

const DEFAULT_MISSING: Record<'operatorRead' | 'operatorDelivery' | 'operatorDeployment', string> = {
  operatorRead: 'integration.operator',
  operatorDelivery: 'integration.delivery.operator',
  operatorDeployment: 'integration.deployment.operator'
};

type OperatorCapability = keyof typeof DEFAULT_MISSING;

/**
 * What turns the operator control plane on: the PostgreSQL submission store,
 * or the explicit flag (with the database it needs).
 */
export const CONTROL_PLANE_PREREQUISITE_KEYS = [
  'FI_FHIR_DATABASE_*',
  'FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true'
] as const;

export interface OperatorPreflight {
  /**
   * Why nothing is queried, in the Connections precedence (.loom/42 "Shared
   * contracts"): the control plane is not configured on this deployment, or
   * this identity cannot read it.
   */
  reason: 'not-configured' | 'missing-role';
  principal: string;
  /** True when the identity holds the transport grant but not the service role. */
  holdsTransportGrant: boolean;
  /**
   * Roles the identity lacks to read the operator plane. Always non-empty for
   * `missing-role`; for `not-configured`, the roles it would also need (often
   * none).
   */
  missingRoles: string[];
  /** For `not-configured`: the settings that turn the control plane on. */
  keys: readonly string[];
}

function missingFor(state: AccessCapabilityState, key: OperatorCapability): string[] {
  const reported = missingRolesFor(state, key as CapabilityKey);
  return reported.length > 0 ? reported : [DEFAULT_MISSING[key]];
}

/**
 * Returns the pre-flight to render instead of the operator surfaces, or null
 * when the page may query. "Not configured" (the server reported
 * `controlPlane: false`) outranks a missing role; an unreported capability is
 * unknown and never blocks.
 */
export function operatorPreflight(state: AccessCapabilityState): OperatorPreflight | null {
  if (state.state !== 'known') return null;
  const readable = state.capabilities.operatorRead;
  const base = {
    principal: state.principal,
    holdsTransportGrant: state.roles.includes(TRANSPORT_OPERATOR_ROLE)
  };
  if (state.capabilities.controlPlane === false) {
    return {
      ...base,
      reason: 'not-configured',
      missingRoles: readable ? [] : missingFor(state, 'operatorRead'),
      keys: CONTROL_PLANE_PREREQUISITE_KEYS
    };
  }
  if (readable) return null;
  return { ...base, reason: 'missing-role', missingRoles: missingFor(state, 'operatorRead'), keys: [] };
}

function controlBlockedReason(
  state: AccessCapabilityState,
  key: 'operatorDelivery' | 'operatorDeployment',
  what: string
): string | null {
  if (state.state !== 'known' || state.capabilities[key]) return null;
  const roles = missingFor(state, key).join(', ');
  return `Your identity does not hold ${roles}, so ${what} are disabled. Grant it in FI_FHIR_GRAPHQL_ROLES or FI_FHIR_GRAPHQL_ACCESS_PRINCIPALS.`;
}

/** Why replay/resubmit/discard are disabled for this identity, or null. */
export function deliveryControlBlockedReason(state: AccessCapabilityState): string | null {
  return controlBlockedReason(state, 'operatorDelivery', 'delivery recovery actions');
}

/** Why deploy/pause/resume/retire are disabled for this identity, or null. */
export function deploymentControlBlockedReason(state: AccessCapabilityState): string | null {
  return controlBlockedReason(state, 'operatorDeployment', 'deployment controls');
}

export const deliveryControlBlock: Readable<string | null> = derived(
  accessCapabilities,
  deliveryControlBlockedReason
);

export const deploymentControlBlock: Readable<string | null> = derived(
  accessCapabilities,
  deploymentControlBlockedReason
);
