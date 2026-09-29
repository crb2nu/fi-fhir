/**
 * What the Definitions tab may do, from `/api/auth/status` (.loom/42 E-1), in
 * the Connections precedence:
 *   1. not configured — the control plane, the catalog, or the authoring
 *      service (the deployment fact `definitionAuthoring`) is absent;
 *   2. missing role — the identity cannot read definitions; nothing is queried;
 *   3. read only — the identity reads but cannot author; every write is
 *      disabled with one status line naming the missing roles.
 * Unknown (an older API) never blocks.
 */
import { missingRolesFor, type AccessCapabilityState } from '$lib/graphql/accessCapabilities';
import { CATALOG_PREREQUISITE_KEYS, CONNECTION_READ_ROLE, CONNECTION_WRITE_ROLE } from './connectionsErrors';

export type DefinitionsPreflight =
  | { reason: 'not-configured'; keys: readonly string[] }
  | { reason: 'missing-role'; principal: string; missingRoles: string[] }
  | { reason: 'read-only'; missingRoles: string[] };

export function definitionsPreflight(state: AccessCapabilityState): DefinitionsPreflight | null {
  if (state.state !== 'known') return null;
  const { controlPlane, connectionCatalog, connectionsRead, connectionsWrite, definitionAuthoring } =
    state.capabilities;
  if (controlPlane === false || connectionCatalog === false || definitionAuthoring === false) {
    return { reason: 'not-configured', keys: CATALOG_PREREQUISITE_KEYS };
  }
  if (connectionsRead === false) {
    const reported = missingRolesFor(state, 'connectionsRead');
    return {
      reason: 'missing-role',
      principal: state.principal,
      missingRoles: reported.length > 0 ? reported : [CONNECTION_READ_ROLE]
    };
  }
  const authoringMissing = missingRolesFor(state, 'definitionAuthoring');
  if (authoringMissing.length > 0 || connectionsWrite === false) {
    const reported = authoringMissing.length > 0 ? authoringMissing : missingRolesFor(state, 'connectionsWrite');
    return { reason: 'read-only', missingRoles: reported.length > 0 ? reported : [CONNECTION_WRITE_ROLE] };
  }
  return null;
}
