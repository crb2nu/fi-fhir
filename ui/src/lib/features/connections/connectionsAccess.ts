/**
 * What the Connections page may do, from the identity's reported capabilities
 * (`/api/auth/status`, .loom/38 Decision 4).
 *
 * The honest states, in the spec's precedence:
 *   1. the control plane (and so the catalog) is not configured on this
 *      deployment → the catalog tabs say so and name the variable;
 *   2. the identity cannot read connections → the page names the missing role
 *      and issues no query;
 *   3. the identity cannot change connections → every form is read-only, with
 *      one status line naming the missing role.
 *
 * A capability the server did not report (an older API, a bearer session) is
 * unknown, and unknown never blocks: the page tries and renders the failure
 * inline.
 */
import {
  missingRolesFor,
  type AccessCapabilityState
} from '$lib/graphql/accessCapabilities';
import {
  CATALOG_PREREQUISITE_KEYS,
  CONNECTION_READ_ROLE,
  CONNECTION_WRITE_ROLE,
  CONTROL_PLANE_ENV_KEY
} from './connectionsErrors';

export { CATALOG_PREREQUISITE_KEYS, CONNECTION_READ_ROLE, CONNECTION_WRITE_ROLE, CONTROL_PLANE_ENV_KEY };

/**
 * The catalog is not configured on this deployment. It is on whenever the
 * operator control plane is: with the PostgreSQL submission store
 * (`FI_FHIR_DATABASE_*`) or `FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true`.
 */
export interface CatalogNotConfigured {
  reason: 'not-configured';
  /** What turns it on: the database settings, or the control-plane flag. */
  keys: readonly string[];
}

/** The identity lacks the role that reads connections and the engine runtime. */
export interface ConnectionsMissingRole {
  reason: 'missing-role';
  principal: string;
  missingRoles: string[];
}

export type ConnectionsPreflight = CatalogNotConfigured | ConnectionsMissingRole;

/**
 * The state of the catalog tabs (Sources, Destinations), or null when they
 * may query. Not configured takes precedence over a missing role.
 */
export function catalogPreflight(state: AccessCapabilityState): ConnectionsPreflight | null {
  if (state.state !== 'known') return null;
  const { controlPlane, connectionCatalog } = state.capabilities;
  if (controlPlane === false || connectionCatalog === false) {
    return { reason: 'not-configured', keys: CATALOG_PREREQUISITE_KEYS };
  }
  return readPreflight(state);
}

/**
 * The missing-role state that blocks every read on the page — the catalog and
 * `engineRuntime` share `integration.operator` — or null.
 */
export function readPreflight(state: AccessCapabilityState): ConnectionsMissingRole | null {
  if (state.state !== 'known' || state.capabilities.connectionsRead !== false) return null;
  const reported = missingRolesFor(state, 'connectionsRead');
  return {
    reason: 'missing-role',
    principal: state.principal,
    missingRoles: reported.length > 0 ? reported : [CONNECTION_READ_ROLE]
  };
}

/** The roles this identity lacks to change connections, or null when it may (or it is unknown). */
export function writeBlockedRoles(state: AccessCapabilityState): string[] | null {
  if (state.state !== 'known' || state.capabilities.connectionsWrite !== false) return null;
  const reported = missingRolesFor(state, 'connectionsWrite');
  return reported.length > 0 ? reported : [CONNECTION_WRITE_ROLE];
}

/** One sentence for a disabled control's `title` when writes are blocked. */
export function writeBlockedReason(roles: readonly string[] | null): string | undefined {
  if (!roles) return undefined;
  return `Read only: this identity does not hold ${roles.join(', ')}.`;
}
