/**
 * The Status column's honest states (.loom/38 "Honest states"), derived only
 * from what the catalog and this replica report — never from a name matching
 * a name.
 *
 *   Draft          never compiled, or edited since the latest revision
 *                  (draft.version > latest.compiledFromVersion)
 *   Compiled rN    the latest revision was compiled from the current draft version
 *   Referenced     ≥ 1 lifecycle definition revision names a revision's digest
 *   Deployed       a referencing definition's snapshot is deployed or paused
 *   Mounted here   this replica runs one of the connection's revisions
 *
 * They are additive ("Compiled r3 · Referenced · Mounted here"). A draft
 * edited after the revision this replica runs reads "Draft (r3 mounted)": the
 * mount is a fact about a revision, and the draft is not it. When the replica
 * runs an older revision than the latest, the token names it ("r2 mounted
 * here"). An archived connection says so first; archiving advances the draft
 * version, so by the formula above it reads as a draft.
 */
import type { BadgeTone } from '$lib/ui/primitives';

export interface ConnectionStatusInput {
  version: number;
  archived?: boolean | undefined;
  latestRevision: { revisionId: string; compiledFromVersion: number } | null;
  references: ReadonlyArray<{ definitionId: string; state: string }>;
  /** revisionId is absent or null from servers before it was a field. */
  runtime: { mounted: boolean; detail: string | null; revisionId?: string | null | undefined };
}

export type ConnectionStatusKey = 'archived' | 'draft' | 'compiled' | 'referenced' | 'deployed' | 'mounted';

export interface ConnectionStatusToken {
  key: ConnectionStatusKey;
  label: string;
  tone: BadgeTone;
  /** Why this state holds, for the token's tooltip. */
  title: string;
}

/** Definition snapshot states that count as Deployed. */
const DEPLOYED_STATES = new Set(['deployed', 'paused']);

/**
 * The revision the runtime says is mounted. C-0 writes the detail as
 * "revision <id>: <what runs it>" (connection.runtimeStateFor); anything else
 * yields null and the label does not guess.
 */
export function mountedRevisionId(detail: string | null): string | null {
  const match = /^revision (\S+):/.exec(detail ?? '');
  return match ? (match[1] ?? null) : null;
}

/**
 * The mounted revision: the runtime's structured revisionId when it sends
 * one, else — an older server, or the HTTP ingress, which is bound by
 * definition and carries no revisionId — the id the detail names.
 */
export function runtimeRevisionId(runtime: ConnectionStatusInput['runtime']): string | null {
  return runtime.revisionId ?? mountedRevisionId(runtime.detail);
}

export function connectionStatus(input: ConnectionStatusInput): ConnectionStatusToken[] {
  const tokens: ConnectionStatusToken[] = [];
  const latest = input.latestRevision;
  const edited = latest !== null && input.version > latest.compiledFromVersion;
  const mounted = input.runtime.mounted;
  const mountedRevision = mounted ? runtimeRevisionId(input.runtime) : null;

  if (input.archived) {
    tokens.push({
      key: 'archived',
      label: 'Archived',
      tone: 'neutral',
      title: 'Archived: the draft accepts no change; its revisions stay readable.'
    });
  }

  if (latest === null) {
    tokens.push({ key: 'draft', label: 'Draft', tone: 'neutral', title: 'Never compiled.' });
  } else if (edited) {
    const label = mounted
      ? `Draft (${mountedRevision ? `r${mountedRevision}` : 'older revision'} mounted)`
      : 'Draft';
    tokens.push({
      key: 'draft',
      label,
      tone: 'neutral',
      title: `Draft version ${input.version} changed after r${latest.revisionId} was compiled from version ${latest.compiledFromVersion}.`
    });
  } else {
    tokens.push({
      key: 'compiled',
      label: `Compiled r${latest.revisionId}`,
      tone: 'info',
      title: `r${latest.revisionId} was compiled from the current draft version ${latest.compiledFromVersion}.`
    });
  }

  if (input.references.length > 0) {
    const definitions = [...new Set(input.references.map((reference) => reference.definitionId))];
    tokens.push({
      key: 'referenced',
      label: 'Referenced',
      tone: 'neutral',
      title: `Named by ${input.references.length} definition revision${input.references.length === 1 ? '' : 's'}: ${definitions.join(', ')}.`
    });
  }

  const deployed = input.references.filter((reference) => DEPLOYED_STATES.has(reference.state));
  if (deployed.length > 0) {
    tokens.push({
      key: 'deployed',
      label: 'Deployed',
      tone: 'success',
      title: `A referencing definition is ${[...new Set(deployed.map((reference) => reference.state))].join(' or ')}.`
    });
  }

  if (mounted && !edited) {
    // The runtime reports the newest revision it runs; when that is not the
    // latest one, the token names it rather than implying the latest runs.
    const older = mountedRevision !== null && latest !== null && mountedRevision !== latest.revisionId;
    tokens.push({
      key: 'mounted',
      label: older ? `r${mountedRevision} mounted here` : 'Mounted here',
      tone: 'success',
      title: input.runtime.detail ?? 'This replica runs a revision of this connection.'
    });
  }

  return tokens;
}

/** The tokens as one line, for tooltips and text assertions. */
export function connectionStatusText(tokens: readonly ConnectionStatusToken[]): string {
  return tokens.map((token) => token.label).join(' · ');
}
