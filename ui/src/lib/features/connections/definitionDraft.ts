/**
 * The New definition form's model (.loom/42 E-1): pure functions from the
 * author's choices to an IntegrationDefinitionDraftInput, the binding rows the
 * chosen revisions require, and what each lifecycle state allows next.
 */
import type {
  ConnectionChoice,
  DefinitionDetail,
  DefinitionRow,
  IntegrationDefinitionDraftInput,
  RegistryArtifact
} from './definitionsApi';

/** One secret binding the chosen revisions name, and where its reference comes from. */
export interface BindingRow {
  name: string;
  provider: string;
  key: string;
  version: string;
  /** Which chosen connection names it: `source adt-mllp`, `destination fhir-primary`. */
  requiredBy: string;
}

export interface PolicyForm {
  validationTimeoutSeconds: number;
  validationMaxAgeSeconds: number;
  maxInFlight: number;
  maxQueued: number;
  maxMessagesPerSecond: number;
}

export interface RetentionForm {
  mode: 'ephemeral' | 'encrypted';
  ttlSeconds: number;
  purpose: string;
  storageArtifactId: string;
  storageRevisionId: string;
  storageDigest: string;
  keyProvider: string;
  keyKey: string;
}

export interface DraftForm {
  definitionId: string;
  revisionId: string;
  parentRevisionId: string;
  /** Source connection id; its latest compiled revision is bound. */
  sourceId: string;
  /** Destination connection ids; each one's latest compiled revision is bound. */
  destinationIds: string[];
  /** Registry integration whose profile and workflow refs are bound. */
  integrationId: string;
  bindings: BindingRow[];
  retention: RetentionForm;
  customPolicy: boolean;
  policy: PolicyForm;
}

/** The seed's defaults (authoring.DefaultDeploymentPolicy), with the deployment's max age. */
export function defaultPolicy(maxAgeSeconds: number | null): PolicyForm {
  return {
    validationTimeoutSeconds: 5,
    validationMaxAgeSeconds: maxAgeSeconds ?? 300,
    maxInFlight: 2,
    maxQueued: 10,
    maxMessagesPerSecond: 100
  };
}

export function newDraftForm(maxAgeSeconds: number | null): DraftForm {
  return {
    definitionId: '',
    revisionId: 'v1',
    parentRevisionId: '',
    sourceId: '',
    destinationIds: [],
    integrationId: '',
    bindings: [],
    retention: {
      mode: 'ephemeral',
      ttlSeconds: 86400,
      purpose: '',
      storageArtifactId: '',
      storageRevisionId: '',
      storageDigest: '',
      keyProvider: 'file',
      keyKey: ''
    },
    customPolicy: false,
    policy: defaultPolicy(maxAgeSeconds)
  };
}

/** Only connections with a compiled revision can be bound. */
export function compiled(choices: readonly ConnectionChoice[]): ConnectionChoice[] {
  return choices.filter((choice) => choice.latestRevision !== null);
}

/**
 * The binding rows the chosen revisions require, each pre-filled with the
 * reference its connection declares under the same name. Rows the author
 * already edited keep their reference. Names only a destination and the
 * source share keep the source's reference; the server reports a conflict.
 */
export function deriveBindings(
  source: ConnectionChoice | null,
  destinations: readonly ConnectionChoice[],
  previous: readonly BindingRow[] = []
): BindingRow[] {
  const rows: BindingRow[] = [];
  const seen = new Set<string>();
  const add = (choice: ConnectionChoice, role: string) => {
    for (const name of choice.latestRevision?.secretBindingNames ?? []) {
      if (seen.has(name)) continue;
      seen.add(name);
      const kept = previous.find((row) => row.name === name);
      const declared = choice.secretBindings.find((binding) => binding.name === name);
      rows.push({
        name,
        provider: kept?.provider ?? declared?.provider ?? '',
        key: kept?.key ?? declared?.key ?? '',
        version: kept?.version ?? declared?.version ?? '',
        requiredBy: `${role} ${choice.id}`
      });
    }
  };
  if (source) add(source, 'source');
  for (const destination of destinations) add(destination, 'destination');
  return rows;
}

/** The draft input the server checks and writes; null parts stay empty for the server to report. */
export function toDraftInput(
  form: DraftForm,
  sources: readonly ConnectionChoice[],
  destinations: readonly ConnectionChoice[],
  artifacts: readonly RegistryArtifact[]
): IntegrationDefinitionDraftInput {
  const source = sources.find((choice) => choice.id === form.sourceId)?.latestRevision ?? null;
  const artifact = artifacts.find((candidate) => candidate.integrationId === form.integrationId) ?? null;
  const empty = { artifactId: '', revisionId: '', digest: '' };
  const ref = (value: { artifactId: string; revisionId: string; digest: string } | null) =>
    value ? { artifactId: value.artifactId, revisionId: value.revisionId, digest: value.digest } : empty;
  const retention = form.retention;
  return {
    definitionId: form.definitionId.trim(),
    revisionId: form.revisionId.trim(),
    parentRevisionId: form.parentRevisionId.trim() || null,
    source: { artifactId: source?.artifactId ?? '', revisionId: source?.revisionId ?? '' },
    destinations: form.destinationIds.map((id) => {
      const revision = destinations.find((choice) => choice.id === id)?.latestRevision ?? null;
      return { artifactId: revision?.artifactId ?? id, revisionId: revision?.revisionId ?? '' };
    }),
    profile: ref(artifact?.profile ?? null),
    workflow: ref(artifact?.workflow ?? null),
    secretBindings: form.bindings.map((binding) => ({
      name: binding.name,
      provider: binding.provider.trim(),
      key: binding.key.trim(),
      version: binding.version.trim() || null
    })),
    rawRetention:
      retention.mode === 'encrypted'
        ? {
            mode: 'encrypted',
            ttlSeconds: retention.ttlSeconds,
            purpose: retention.purpose.trim() || null,
            storageRevision: {
              artifactId: retention.storageArtifactId.trim(),
              revisionId: retention.storageRevisionId.trim(),
              digest: retention.storageDigest.trim()
            },
            encryptionKey: { provider: retention.keyProvider.trim(), key: retention.keyKey.trim(), version: null }
          }
        : null,
    deployment: form.customPolicy
      ? {
          validationTimeoutSeconds: form.policy.validationTimeoutSeconds,
          validationMaxAgeSeconds: form.policy.validationMaxAgeSeconds,
          scheduleMode: 'continuous',
          cronExpression: null,
          timezone: null,
          healthStartupGraceSeconds: 5,
          healthCheckIntervalSeconds: 30,
          healthTimeoutSeconds: 5,
          healthFailureThreshold: 3,
          maxInFlight: form.policy.maxInFlight,
          maxQueued: form.policy.maxQueued,
          maxMessagesPerSecond: form.policy.maxMessagesPerSecond
        }
      : null
  };
}

/** A problem blocks unless it is an unused binding (the catalog's rule). */
export function blocking(problems: ReadonlyArray<{ code: string }>): boolean {
  return problems.some((problem) => problem.code !== 'UNUSED_BINDING');
}

/** The catalog's own freshness rule: passed, recorded, and not yet expired. */
export function validationCurrent(definition: Pick<DefinitionRow, 'validationPassed' | 'validationExpiresAt'>, now: Date): boolean {
  if (!definition.validationPassed || !definition.validationExpiresAt) return false;
  return new Date(definition.validationExpiresAt).getTime() > now.getTime();
}

export type ValidationLabel = 'none' | 'current' | 'expired' | 'failed';

export function validationLabel(definition: DefinitionRow, now: Date): ValidationLabel {
  if (!definition.validationCheckedAt) return 'none';
  if (!definition.validationPassed) return 'failed';
  return validationCurrent(definition, now) ? 'current' : 'expired';
}

export type LifecycleStep = 'validate' | 'approve' | 'publish';

/** Validation may be recorded in these states (lifecycle validationAllowed). */
export function canValidate(state: string): boolean {
  return ['draft', 'validated', 'approved', 'published', 'paused'].includes(state);
}

/**
 * The next authoring step a state offers, and why a gated one is disabled:
 * approve and publish need current validation evidence; deploy is on Operator.
 */
export function nextStep(
  definition: DefinitionRow,
  now: Date
): { step: LifecycleStep | 'deploy' | null; blockedReason: string | null } {
  const current = validationCurrent(definition, now);
  switch (definition.state) {
    case 'draft':
      return { step: 'validate', blockedReason: null };
    case 'validated':
      return current
        ? { step: 'approve', blockedReason: null }
        : { step: 'approve', blockedReason: 'Validation evidence has expired; validate again before approving.' };
    case 'approved':
      return current
        ? { step: 'publish', blockedReason: null }
        : { step: 'publish', blockedReason: 'Validation evidence has expired; validate again before publishing.' };
    case 'published':
      return { step: 'deploy', blockedReason: null };
    default:
      return { step: null, blockedReason: null };
  }
}

/** `?definition=<id>&revision=<id>`: the query both pages' deep links carry. */
export function definitionQuery(definition: Pick<DefinitionRow, 'definitionId' | 'revisionId'>): string {
  return `?${new URLSearchParams({ definition: definition.definitionId, revision: definition.revisionId }).toString()}`;
}

/** The Operator page's deep link to deploy this revision. */
export function operatorDeployLink(definition: Pick<DefinitionRow, 'definitionId' | 'revisionId'>): string {
  return `/operator${definitionQuery(definition)}`;
}

/** `/connections?definition=&revision=`: this tab's own deep link. */
export function definitionLink(definition: Pick<DefinitionRow, 'definitionId' | 'revisionId'>): string {
  const params = new URLSearchParams({ definition: definition.definitionId, revision: definition.revisionId });
  return `/connections?${params.toString()}`;
}

/** Reads `?definition=&revision=` from a query string; null unless both are present. */
export function definitionFromQuery(search: string): { definitionId: string; revisionId: string } | null {
  const params = new URLSearchParams(search);
  const definitionId = params.get('definition')?.trim() ?? '';
  const revisionId = params.get('revision')?.trim() ?? '';
  return definitionId && revisionId ? { definitionId, revisionId } : null;
}

/** The validation mode sentences the picker shows: what each mode checks, and what it does not. */
export const MODE_TEXT = {
  REAL: 'Contacts the source: the batch provider this replica runs lists one object of the input location with the credentials the batch runner uses. Offered only for the batch source this replica mounts.',
  STATIC:
    'Contacts nothing. Records whether a replica reported this exact source revision mounted in its recent heartbeats and whether the source’s secret bindings are bound. It does not prove the endpoint is reachable; for a batch source, REAL is the check that contacts it.',
  SKIP: 'Checks nothing. Records VALIDATION_SKIPPED with your reason, which must be at least 16 characters.'
} as const;

export const MIN_SKIP_REASON = 16;

export function detailKey(detail: Pick<DefinitionDetail, 'definition'>): string {
  return `${detail.definition.definitionId}/${detail.definition.revisionId}`;
}
