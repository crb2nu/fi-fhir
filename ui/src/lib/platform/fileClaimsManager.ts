/**
 * File Claims Manager
 *
 * Advisory file locking via MCP. When an agent claims a file, other agents
 * see a warning when they open it. Claims are non-blocking — operators can
 * override them.
 *
 * Delegates to the collaboration store for claim state; this module provides
 * convenience helpers for checking individual file claims.
 */
import { derived } from 'svelte/store';
import {
  collaborationState,
  claimFile as storeClaimFile,
  releaseFile as storeReleaseFile,
  CURRENT_AGENT_ID
} from '$lib/features/collaboration/collaborationStore';
import type { FileClaim } from '$lib/features/collaboration/collaborationStore';

/** All active file claims. */
export const fileClaims = derived(
  collaborationState,
  ($s) => $s.fileClaims
);

/** Claim a file for the current operator. */
export async function claimFile(path: string): Promise<void> {
  await storeClaimFile(path);
}

/** Release a previously claimed file. */
export async function releaseFile(path: string): Promise<void> {
  await storeReleaseFile(path);
}

/** Check if a file is claimed by another agent. Returns the claim or null. */
export function checkClaim(
  claims: FileClaim[],
  path: string
): FileClaim | null {
  const claim = claims.find((c) => c.filePath === path);
  if (!claim) return null;
  if (claim.claimedBy === CURRENT_AGENT_ID) return null;
  return claim;
}

/**
 * Derived store: map of filePath -> claiming agent display name
 * (only claims by *other* agents).
 */
export const otherAgentClaims = derived(
  collaborationState,
  ($s) => {
    const map = new Map<string, string>();
    for (const claim of $s.fileClaims) {
      if (claim.claimedBy !== CURRENT_AGENT_ID) {
        const agent = $s.presence.find((a) => a.agentId === claim.claimedBy);
        map.set(claim.filePath, agent?.displayName ?? claim.claimedBy);
      }
    }
    return map;
  }
);
