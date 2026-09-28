/**
 * Role pre-flights for the surfaces outside the operator plane (.loom/40 D-3).
 *
 * The operator plane and the connection catalog have their own pre-flights
 * (`operatorAccess.ts`, `ConnectionsPage.svelte`). The rest of the IDE's server
 * surfaces sit behind one of two gates at the GraphQL transport:
 *
 * - `clinical:read` — the event store (Events). The status endpoint reports it
 *   as the `clinicalRead` capability with its `missingRoles` entry.
 * - `graphql:operator` — the compatibility grant: profiles, the workflow
 *   catalog, terminology, legacy submit ("Process"). No fine-grained role
 *   describes these yet (`operation_authorization_roles.go`, the
 *   "compatibility bucket"), so the status endpoint has no capability for
 *   them; the grant itself is the requirement, read from the reported roles
 *   exactly as `operatorPreflight` reads it for `holdsTransportGrant`.
 *
 * An identity that holds neither — the hosted demo's preview-only visitor —
 * would otherwise discover each refusal by failing ("GraphQL operation
 * forbidden" as a toast and an inline error). With a pre-flight the surface
 * names the role and issues nothing.
 *
 * Unknown capabilities (an older API, or a bearer session) never block.
 */
import {
  missingRolesFor,
  type AccessCapabilityState
} from '$lib/graphql/accessCapabilities';
import { TRANSPORT_OPERATOR_ROLE } from '$lib/features/operator/operatorAccess';

export const CLINICAL_READ_ROLE = 'clinical:read';

export interface RolePreflight {
  /** The identity the status endpoint reported ('' when it named none). */
  principal: string;
  /** Roles the identity would need; never empty. */
  missingRoles: string[];
}

/** A pre-flight decision: null when the surface may query. */
export type RolePreflightCheck = (state: AccessCapabilityState) => RolePreflight | null;

/**
 * The compatibility grant (`graphql:operator`): null when the identity holds
 * it or the capabilities are unknown.
 */
export function compatibilityGrantPreflight(state: AccessCapabilityState): RolePreflight | null {
  if (state.state !== 'known' || state.roles.includes(TRANSPORT_OPERATOR_ROLE)) return null;
  return { principal: state.principal, missingRoles: [TRANSPORT_OPERATOR_ROLE] };
}

/**
 * The event store (`clinicalRead`): null when the capability is held or the
 * capabilities are unknown. Names the server's `missingRoles.clinicalRead`,
 * falling back to `clinical:read` when the server listed none.
 */
export function clinicalReadPreflight(state: AccessCapabilityState): RolePreflight | null {
  if (state.state !== 'known' || state.capabilities.clinicalRead) return null;
  const reported = missingRolesFor(state, 'clinicalRead');
  return {
    principal: state.principal,
    missingRoles: reported.length > 0 ? reported : [CLINICAL_READ_ROLE]
  };
}

/** A control's disabled `title` for a missing role, or null when it may run. */
export function roleBlockedReason(preflight: RolePreflight | null, what: string): string | null {
  if (!preflight) return null;
  return `${what} needs ${preflight.missingRoles.join(', ')}, which this identity does not hold.`;
}
