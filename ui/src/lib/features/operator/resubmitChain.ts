/**
 * Resubmit lineage inside one message trace.
 *
 * `resubmitMessage` forks a child attempt whose `parentAttemptId` names the
 * dead-lettered source; a child can itself be resubmitted. Every attempt of a
 * receipt is in its trace, so the chains are recoverable from the attempts
 * alone. A parent outside the trace (another receipt, never expected) starts
 * its own chain rather than being invented.
 */

export interface ChainAttempt {
  attemptId: string;
  parentAttemptId?: string | null | undefined;
}

/**
 * Chains of two or more attempts, each ordered root first, following the
 * first-recorded child at every fork (a fork — two resubmits of one parent —
 * adds a chain per extra branch). Attempts that were never resubmitted and
 * have no parent are not a chain.
 */
export function resubmitChains(attempts: readonly ChainAttempt[]): string[][] {
  const ids = new Set(attempts.map((attempt) => attempt.attemptId));
  const children = new Map<string, string[]>();
  for (const attempt of attempts) {
    const parent = attempt.parentAttemptId;
    if (!parent || !ids.has(parent)) continue;
    children.set(parent, [...(children.get(parent) ?? []), attempt.attemptId]);
  }
  const roots = attempts
    .filter((attempt) => !attempt.parentAttemptId || !ids.has(attempt.parentAttemptId))
    .map((attempt) => attempt.attemptId);

  const chains: string[][] = [];
  const walk = (path: string[], seen: Set<string>) => {
    const last = path[path.length - 1] ?? "";
    const next = (children.get(last) ?? []).filter((id) => !seen.has(id));
    if (next.length === 0) {
      if (path.length > 1) chains.push(path);
      return;
    }
    for (const child of next) {
      walk([...path, child], new Set([...seen, child]));
    }
  };
  for (const root of roots) walk([root], new Set([root]));
  return chains;
}

/** The attempts resubmitted from `attemptId`, in trace order. */
export function childAttempts(attempts: readonly ChainAttempt[], attemptId: string): string[] {
  return attempts.filter((attempt) => attempt.parentAttemptId === attemptId).map((attempt) => attempt.attemptId);
}
