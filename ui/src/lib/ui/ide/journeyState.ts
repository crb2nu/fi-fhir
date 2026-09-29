/**
 * Journey stage evidence, read from the engine (.loom/42 E-4, decision 6):
 *
 * | stage          | complete when                                        | read through            |
 * |----------------|------------------------------------------------------|-------------------------|
 * | Source Intake  | an integration session has at least one run          | fetchRecentSessions     |
 * | Normalization  | at least one published (active) source profile       | ListProfiles            |
 * | Translation    | a terminology mapping, or an approved autoroute      | ListMappings, PendingAutorouteStats |
 * | Delivery       | a workflow definition has a published version        | ListWorkflowDefinitions |
 * | Verification   | at least one accepted receipt                        | fetchReceipts           |
 *
 * Each stage pre-flights first, in the Connections precedence (not
 * configured, then missing role), from the capabilities the credential gate
 * already fetched: a stage its identity cannot read is `unknown` and issues
 * no query. Unknown capabilities (an older API, a bearer session) never
 * block; a failed query makes its stage `unknown`, never complete.
 *
 * Every read is inline-only: a background shell check never raises a toast
 * (.loom/22 B4). The session and receipt reads are the features' own
 * wrappers, which already opt out; the profile, terminology and workflow
 * wrappers always toast, so those three stages send the same generated
 * documents through `graphqlFetch` with the toast off instead.
 *
 * The shell calls `refreshJourneyEvidence()` on mount and on every route
 * change; overlapping calls coalesce into one follow-up read. The last
 * answer stays visible while a refresh runs.
 */
import { derived, get, writable, type Readable } from 'svelte/store';
import {
  ListMappingsDocument,
  ListProfilesDocument,
  ListWorkflowDefinitionsDocument,
  PendingAutorouteStatsDocument
} from '$lib/gen/graphql';
import { graphqlFetch, graphQLErrorEntries } from '$lib/graphql/client';
import {
  currentAccessCapabilities,
  missingRolesFor,
  type AccessCapabilityState
} from '$lib/graphql/accessCapabilities';
import { compatibilityGrantPreflight } from '$lib/features/access/rolePreflight';
import { fetchRecentSessions } from '$lib/features/dashboard/dashboardApi';
import { fetchReceipts } from '$lib/features/operator/operatorApi';
import type { JourneyEvidence, JourneyStageId, StageEvidence } from './journey';

/** The reads each stage needs; swapped in tests. */
export interface JourneyEvidenceApi {
  /** Number of integration sessions that have at least one run. */
  sessionsWithRuns(): Promise<number>;
  /** Number of active (published) source profiles. */
  publishedProfiles(): Promise<number>;
  /** Total terminology mappings. */
  mappings(): Promise<number>;
  /** Pending autoroutes that were approved into a mapping. */
  approvedAutoroutes(): Promise<number>;
  /** Number of workflow definitions with a published version in any environment. */
  publishedWorkflows(): Promise<number>;
  /** Whether at least one receipt was accepted. */
  hasAcceptedReceipt(): Promise<boolean>;
}

const INLINE = { showErrorToast: false } as const;

function hasPublishedVersion(byEnv: unknown): boolean {
  if (!byEnv || typeof byEnv !== 'object' || Array.isArray(byEnv)) return false;
  return Object.values(byEnv as Record<string, unknown>).some(
    (version) => version !== null && version !== undefined && version !== ''
  );
}

export const defaultJourneyEvidenceApi: JourneyEvidenceApi = {
  async sessionsWithRuns() {
    const sessions = await fetchRecentSessions(Number.MAX_SAFE_INTEGER);
    return sessions.filter((session) => session.runs.length > 0).length;
  },
  async publishedProfiles() {
    const result = await graphqlFetch(ListProfilesDocument, { activeOnly: true }, INLINE);
    return result.profiles.length;
  },
  async mappings() {
    const input = {
      sourceSystem: null,
      targetSystem: null,
      profileId: null,
      origin: null,
      uploadBatchId: null,
      equivalence: null,
      createdAfter: null,
      createdBefore: null,
      first: 1,
      offset: 0
    };
    const result = await graphqlFetch(ListMappingsDocument, { input }, INLINE);
    return result.listMappings.totalCount;
  },
  async approvedAutoroutes() {
    const result = await graphqlFetch(PendingAutorouteStatsDocument, {}, INLINE);
    return result.pendingAutorouteStats.approvedCount;
  },
  async publishedWorkflows() {
    const result = await graphqlFetch(
      ListWorkflowDefinitionsDocument,
      { filter: null, paging: null },
      INLINE
    );
    return result.workflowDefinitions.filter((definition) =>
      hasPublishedVersion(definition.publishedVersionsByEnv)
    ).length;
  },
  async hasAcceptedReceipt() {
    const page = await fetchReceipts(
      {
        status: 'accepted',
        integrationArtifactId: null,
        correlationId: null,
        sourceMessageId: null,
        from: null,
        to: null
      },
      { first: 1, after: null }
    );
    return page.nodes.length > 0;
  }
};

function plural(count: number, one: string, many = `${one}s`): string {
  return `${count} ${count === 1 ? one : many}`;
}

function unknown(reason: string): StageEvidence {
  return { state: 'unknown', reason };
}

/** "Reading source profiles needs graphql:operator, which this identity does not hold." */
function missingRoleSentence(what: string, roles: readonly string[]): string {
  return `Reading ${what} needs ${roles.join(', ')}, which this identity does not hold.`;
}

/**
 * One sentence for a failed read. Never echoes a raw server message, and
 * trusts only the error-extension code the API emits: `FORBIDDEN` (the
 * transport gate's refusal) names the identity; anything else — a 503, a
 * proxy error page, a timeout — only says the read failed. "Not configured"
 * is never inferred from error text; it comes from the capability pre-flight.
 */
function failureSentence(what: string, err: unknown): string {
  const codes = graphQLErrorEntries(err).map((entry) => entry.extensions?.['code']);
  if (codes.includes('FORBIDDEN')) return `This identity may not read ${what}.`;
  return `${capitalize(what)} could not be read.`;
}

function capitalize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

async function settle<T>(read: () => Promise<T>): Promise<{ ok: true; value: T } | { ok: false; error: unknown }> {
  try {
    return { ok: true, value: await read() };
  } catch (error) {
    return { ok: false, error };
  }
}

async function intake(access: AccessCapabilityState, api: JourneyEvidenceApi): Promise<StageEvidence> {
  if (access.state === 'known' && !access.capabilities.integrationSessions) {
    const roles = missingRolesFor(access, 'integrationSessions');
    return unknown(
      roles.length > 0
        ? missingRoleSentence('integration sessions', roles)
        : 'Integration sessions are not enabled on this deployment.'
    );
  }
  const read = await settle(() => api.sessionsWithRuns());
  if (!read.ok) return unknown(failureSentence('integration sessions', read.error));
  return read.value > 0
    ? { state: 'complete', reason: `${plural(read.value, 'integration session')} with a run.` }
    : { state: 'incomplete', reason: 'No integration session has a run yet.' };
}

async function normalization(access: AccessCapabilityState, api: JourneyEvidenceApi): Promise<StageEvidence> {
  const blocked = compatibilityGrantPreflight(access);
  if (blocked) return unknown(missingRoleSentence('source profiles', blocked.missingRoles));
  const read = await settle(() => api.publishedProfiles());
  if (!read.ok) return unknown(failureSentence('source profiles', read.error));
  return read.value > 0
    ? { state: 'complete', reason: `${plural(read.value, 'published profile')}.` }
    : { state: 'incomplete', reason: 'No source profile is published yet.' };
}

async function translation(access: AccessCapabilityState, api: JourneyEvidenceApi): Promise<StageEvidence> {
  const blocked = compatibilityGrantPreflight(access);
  if (blocked) return unknown(missingRoleSentence('terminology', blocked.missingRoles));
  const [mappings, approved] = await Promise.all([
    settle(() => api.mappings()),
    settle(() => api.approvedAutoroutes())
  ]);
  const parts: string[] = [];
  if (mappings.ok && mappings.value > 0) parts.push(plural(mappings.value, 'mapping'));
  if (approved.ok && approved.value > 0) parts.push(plural(approved.value, 'approved autoroute'));
  if (parts.length > 0) return { state: 'complete', reason: `${parts.join(', ')}.` };
  if (!mappings.ok) return unknown(failureSentence('terminology mappings', mappings.error));
  if (!approved.ok) return unknown(failureSentence('autoroute statistics', approved.error));
  return { state: 'incomplete', reason: 'No terminology mapping or approved autoroute yet.' };
}

async function delivery(access: AccessCapabilityState, api: JourneyEvidenceApi): Promise<StageEvidence> {
  const blocked = compatibilityGrantPreflight(access);
  if (blocked) return unknown(missingRoleSentence('the workflow catalog', blocked.missingRoles));
  const read = await settle(() => api.publishedWorkflows());
  if (!read.ok) return unknown(failureSentence('the workflow catalog', read.error));
  return read.value > 0
    ? { state: 'complete', reason: `${plural(read.value, 'workflow')} with a published version.` }
    : { state: 'incomplete', reason: 'No workflow version is published yet.' };
}

async function verification(access: AccessCapabilityState, api: JourneyEvidenceApi): Promise<StageEvidence> {
  if (access.state === 'known') {
    if (access.capabilities.controlPlane === false) {
      return unknown('The operator control plane is not configured on this deployment.');
    }
    if (!access.capabilities.operatorRead) {
      const roles = missingRolesFor(access, 'operatorRead');
      return unknown(missingRoleSentence('receipts', roles.length > 0 ? roles : ['integration.operator']));
    }
  }
  const read = await settle(() => api.hasAcceptedReceipt());
  if (!read.ok) return unknown(failureSentence('receipts', read.error));
  return read.value
    ? { state: 'complete', reason: 'At least one message was accepted.' }
    : { state: 'incomplete', reason: 'No message has been accepted yet.' };
}

const CHECKS: Record<JourneyStageId, (access: AccessCapabilityState, api: JourneyEvidenceApi) => Promise<StageEvidence>> = {
  'source-intake': intake,
  normalization,
  translation,
  delivery,
  verification
};

/** Reads every stage's evidence once. Never rejects. */
export async function loadJourneyEvidence(
  access: AccessCapabilityState,
  api: JourneyEvidenceApi = defaultJourneyEvidenceApi
): Promise<JourneyEvidence> {
  const ids = Object.keys(CHECKS) as JourneyStageId[];
  const results = await Promise.all(
    ids.map(async (id) => {
      try {
        return await CHECKS[id](access, api);
      } catch {
        return unknown('The check could not run.');
      }
    })
  );
  return Object.fromEntries(ids.map((id, index) => [id, results[index]!])) as JourneyEvidence;
}

export interface JourneyEvidenceState {
  /** `idle` before the first read; `loading` while a read runs (the last answer stays). */
  status: 'idle' | 'loading' | 'ready';
  evidence: JourneyEvidence | null;
}

const IDLE: JourneyEvidenceState = { status: 'idle', evidence: null };
const store = writable<JourneyEvidenceState>(IDLE);

/** The shell's journey evidence (read-only; `refreshJourneyEvidence` writes it). */
export const journeyEvidence: Readable<JourneyEvidenceState> = { subscribe: store.subscribe };

/** Just the evidence, or null before the first answer. */
export const journeyEvidenceValue: Readable<JourneyEvidence | null> = derived(store, ($s) => $s.evidence);

let inflight: Promise<void> | null = null;
let again = false;
let generation = 0;

export interface RefreshOptions {
  api?: JourneyEvidenceApi;
  access?: AccessCapabilityState;
}

/**
 * Re-reads every stage. A call while a read runs schedules exactly one more
 * read after it (with the capabilities current then) and resolves with it.
 */
export function refreshJourneyEvidence(options: RefreshOptions = {}): Promise<void> {
  if (inflight) {
    again = true;
    return inflight;
  }
  const mine = ++generation;
  store.update((state) => ({ ...state, status: 'loading' }));
  inflight = (async () => {
    try {
      do {
        again = false;
        const evidence = await loadJourneyEvidence(
          options.access ?? currentAccessCapabilities(),
          options.api ?? defaultJourneyEvidenceApi
        );
        if (mine !== generation) return;
        if (!again) store.set({ status: 'ready', evidence });
        else store.set({ status: 'loading', evidence });
      } while (again);
    } finally {
      if (mine === generation) inflight = null;
    }
  })();
  return inflight;
}

/** Forget the evidence (tests, identity change). */
export function resetJourneyEvidence(): void {
  generation++;
  inflight = null;
  again = false;
  store.set(IDLE);
}

/** Snapshot for non-reactive callers. */
export function currentJourneyEvidence(): JourneyEvidence | null {
  return get(store).evidence;
}
