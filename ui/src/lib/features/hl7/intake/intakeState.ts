/**
 * The honest states of sample intake on /hl7 (.loom/38 C-3), derived only
 * from what the API reports: `/api/auth/status`, `engineRuntime`, the catalog
 * (`connections(direction: SOURCE)`) and this session's `connectionCaptures`.
 *
 * Precedence, as the C-2 handoff gates the surface:
 *   1. no session engine on this page          → the entry is absent
 *   2. the catalog is not configured           → the entry is disabled, the reason in its title
 *   3. the identity lacks integration.operator → the dialog names the role and queries nothing
 *   4. no enabled source and no catalog source → "No source connection is mounted on this deployment"
 *   5. otherwise one row per capture target, each with its own state
 *
 * A capability the server did not report is unknown, and unknown never blocks.
 * Nothing here computes an expiry: a capture is armed until the server says
 * otherwise, and the countdown is only a countdown.
 */
import {
  missingRolesFor,
  type AccessCapabilityState
} from '$lib/graphql/accessCapabilities';
import { connectionStatus } from '$lib/features/connections/connectionStatus';
import type { ConnectionRow, EngineAdapterRow, EngineRuntimeView } from '$lib/features/connections/connectionsApi';
import { CONNECTION_READ_ROLE, CONTROL_PLANE_ENV_KEY } from '$lib/features/connections/connectionsErrors';
import { shortHash } from '$lib/features/connections/presentation';
import type { ConnectionCaptureRow, IntakeSessionSample } from './intakeApi';

// ── Entry ────────────────────────────────────────────────────────────────────

export type IntakeEntry = { visible: false } | { visible: true; disabledReason: string | null };

export const CATALOG_NOT_CONFIGURED_REASON = `The connection catalog is not configured on this deployment (${CONTROL_PLANE_ENV_KEY}), so there is no source to sample from.`;

/** Whether "From connection…" is shown, and whether it is usable. */
export function intakeEntry(access: AccessCapabilityState, sessionEngine: boolean): IntakeEntry {
  if (!sessionEngine) return { visible: false };
  if (access.state === 'known') {
    const { integrationSessions, connectionCatalog, controlPlane } = access.capabilities;
    if (!integrationSessions) return { visible: false };
    if (connectionCatalog === false || controlPlane === false) {
      return { visible: true, disabledReason: CATALOG_NOT_CONFIGURED_REASON };
    }
  }
  return { visible: true, disabledReason: null };
}

/** The roles the identity lacks to use intake, or null when it may (or it is unknown). */
export function intakeMissingRoles(access: AccessCapabilityState): string[] | null {
  if (access.state !== 'known' || access.capabilities.connectionsRead !== false) return null;
  const reported = missingRolesFor(access, 'connectionsRead');
  return reported.length > 0 ? reported : [CONNECTION_READ_ROLE];
}

// ── Sources ──────────────────────────────────────────────────────────────────

export type IntakeMode = 'stream' | 'peek';

export interface IntakeSource {
  /** `stream:<sourceId>`, `peek:<connectionId>`, or `runtime-batch:<sourceId>`. */
  key: string;
  mode: IntakeMode;
  /** mllp | http | batch_s3 | batch_sftp | batch (a mounted runner with no catalog connection). */
  kind: string;
  /** The catalog connection's name, or the adapter's role when only the runtime knows it. */
  label: string;
  /** The runtime source ID: what a stream capture taps, what a batch revision names. */
  sourceId: string | null;
  /** The catalog connection a peek reads, or the one that names this stream source. */
  connectionId: string | null;
  /** The mounted digest when this replica runs it, else the latest compiled revision's. */
  digest: string | null;
  /** This replica mounts it (an enabled adapter, or the catalog's runtime state). */
  mounted: boolean;
  /** The honest state, as text. */
  state: string;
  /** Why the state holds, for the state cell's title. */
  stateTitle: string;
  /** False when the action would be refused; `unavailableReason` says why. */
  available: boolean;
  unavailableReason: string | null;
  /** The capture armed on this source by this page's session, if any. */
  armed: ConnectionCaptureRow | null;
}

const STREAM_KINDS = new Set(['mllp', 'http']);
const BATCH_KINDS = new Set(['batch_s3', 'batch_sftp']);
const ADAPTER_ROLE: Record<string, string> = {
  mllp: 'MLLP listener',
  http: 'HTTP ingress',
  batch: 'Batch runner'
};

function specSourceId(spec: unknown): string | null {
  if (typeof spec !== 'object' || spec === null) return null;
  const value = (spec as Record<string, unknown>)['source_id'];
  return typeof value === 'string' && value.trim() !== '' ? value : null;
}

function catalogState(connection: ConnectionRow): string {
  return connectionStatus(connection)
    .map((token) => token.label)
    .join(' · ');
}

function armedFor(sourceId: string | null, captures: readonly ConnectionCaptureRow[]): ConnectionCaptureRow | null {
  if (!sourceId) return null;
  return (
    captures.find((capture) => capture.mode === 'STREAM' && capture.status === 'ARMED' && capture.sourceId === sourceId) ??
    null
  );
}

function runtimeStreamSource(adapter: EngineAdapterRow, sourceId: string): IntakeSource {
  const revision = adapter.sourceRevisionId ? ` r${adapter.sourceRevisionId}` : '';
  return {
    key: `stream:${sourceId}`,
    mode: 'stream',
    kind: adapter.kind,
    label: ADAPTER_ROLE[adapter.kind] ?? adapter.kind,
    sourceId,
    connectionId: null,
    digest: adapter.sourceDigest ?? null,
    mounted: true,
    state: `Mounted here${revision}`,
    stateTitle: `This replica's ${ADAPTER_ROLE[adapter.kind] ?? adapter.kind} admits frames under source ID ${sourceId}${
      adapter.sourceDigest ? ` (${adapter.sourceDigest})` : ''
    }.`,
    available: true,
    unavailableReason: null,
    armed: null
  };
}

/**
 * One row per capture target: stream sources keyed by the runtime source ID
 * (the server allows one armed capture per source), batch sources keyed by
 * the catalog connection a peek reads. A catalog stream connection whose
 * `source_id` is the one an enabled adapter admits joins that adapter's row —
 * they are the same capture target — and never borrows its state.
 */
export function intakeSources(
  runtime: EngineRuntimeView | null,
  catalog: readonly ConnectionRow[] | null,
  captures: readonly ConnectionCaptureRow[] = []
): IntakeSource[] {
  const rows: IntakeSource[] = [];
  const streams = new Map<string, IntakeSource>();
  const runtimeBatch: Array<{ adapter: EngineAdapterRow; sourceId: string | null }> = [];

  for (const adapter of runtime?.adapters ?? []) {
    if (!adapter.enabled) continue;
    if (STREAM_KINDS.has(adapter.kind) && adapter.sourceId) {
      const row = runtimeStreamSource(adapter, adapter.sourceId);
      streams.set(adapter.sourceId, row);
      rows.push(row);
    } else if (adapter.kind === 'batch') {
      runtimeBatch.push({ adapter, sourceId: adapter.sourceId ?? null });
    }
  }

  const catalogRows: IntakeSource[] = [];
  const batchSourceIds = new Set<string>();
  const sorted = [...(catalog ?? [])]
    .filter((connection) => connection.direction === 'SOURCE' && !connection.archived)
    .sort((a, b) => a.name.localeCompare(b.name) || a.id.localeCompare(b.id));

  for (const connection of sorted) {
    const kind = connection.kind.toLowerCase();
    const sourceId = specSourceId(connection.spec);
    const compiled = connection.latestRevision !== null;
    const state = catalogState(connection);

    if (STREAM_KINDS.has(kind)) {
      const existing = sourceId ? streams.get(sourceId) : undefined;
      if (existing) {
        // The same capture target: an adapter admits it, or another catalog
        // connection names the same source_id.
        if (existing.connectionId === null) {
          existing.connectionId = connection.id;
          existing.label = connection.name;
        }
        existing.stateTitle = `${existing.stateTitle} Catalog connection ${connection.id}: ${state}.`;
        if (compiled && !existing.available) {
          existing.available = true;
          existing.unavailableReason = null;
        }
        continue;
      }
      if (!sourceId) {
        catalogRows.push({
          key: `stream-draft:${connection.id}`,
          mode: 'stream',
          kind,
          label: connection.name,
          sourceId: null,
          connectionId: connection.id,
          digest: null,
          mounted: false,
          state,
          stateTitle: 'The spec names no source_id yet.',
          available: false,
          unavailableReason: 'The spec names no source_id: complete it in Connections before capturing.',
          armed: null
        });
        continue;
      }
      const mounted = connection.runtime.mounted;
      const row: IntakeSource = {
        key: `stream:${sourceId}`,
        mode: 'stream',
        kind,
        label: connection.name,
        sourceId,
        connectionId: connection.id,
        digest: connection.latestRevision?.digest ?? null,
        mounted,
        state: mounted ? state : `${state} · not mounted here`,
        stateTitle: mounted
          ? `This replica runs a revision of ${connection.id}.`
          : `No listener on this replica admits ${sourceId}. A capture records only frames some replica admits under this source ID, so it waits until one does or it expires.`,
        available: compiled,
        unavailableReason: compiled
          ? null
          : 'Never compiled: the capture tap sees only mounted or compiled sources. Compile it in Connections first.',
        armed: null
      };
      catalogRows.push(row);
      streams.set(sourceId, row);
      continue;
    }

    if (BATCH_KINDS.has(kind)) {
      if (sourceId) batchSourceIds.add(sourceId);
      catalogRows.push({
        key: `peek:${connection.id}`,
        mode: 'peek',
        kind,
        label: connection.name,
        sourceId,
        connectionId: connection.id,
        digest: connection.latestRevision?.digest ?? null,
        mounted: connection.runtime.mounted,
        state,
        stateTitle: 'A peek lists the objects under the input prefix and reads one; it takes no lease, writes no checkpoint, and moves nothing.',
        available: compiled,
        unavailableReason: compiled ? null : 'Never compiled: only a compiled batch connection can be browsed. Compile it in Connections first.',
        armed: null
      });
    }
  }

  for (const { adapter, sourceId } of runtimeBatch) {
    if (sourceId && batchSourceIds.has(sourceId)) continue;
    rows.push({
      key: `runtime-batch:${sourceId ?? adapter.definitionId ?? 'batch'}`,
      mode: 'peek',
      kind: 'batch',
      label: ADAPTER_ROLE['batch']!,
      sourceId,
      connectionId: null,
      digest: adapter.sourceDigest ?? null,
      mounted: true,
      state: 'Mounted here · no catalog connection',
      stateTitle: `This replica runs a batch runner${sourceId ? ` for ${sourceId}` : ''}${
        adapter.sourceDigest ? ` (${shortHash(adapter.sourceDigest)})` : ''
      }.`,
      available: false,
      unavailableReason:
        'A peek reads a compiled catalog connection. Define this batch source in Connections and compile it to browse its objects.',
      armed: null
    });
  }

  const all = [...rows, ...catalogRows];
  for (const row of all) row.armed = row.mode === 'stream' ? armedFor(row.sourceId, captures) : null;
  return all;
}

// ── Dialog view ──────────────────────────────────────────────────────────────

export type Loadable<T> =
  | { status: 'idle' }
  | { status: 'loading' }
  | { status: 'ok'; value: T }
  | { status: 'error'; message: string };

export type IntakeDialogView =
  | { kind: 'hidden' }
  | { kind: 'not-configured'; reason: string }
  | { kind: 'missing-role'; roles: string[]; principal: string }
  | { kind: 'loading' }
  | { kind: 'error'; messages: string[] }
  | { kind: 'empty' }
  | { kind: 'sources'; sources: IntakeSource[]; errors: string[] };

export interface IntakeDialogInput {
  access: AccessCapabilityState;
  sessionEngine: boolean;
  runtime: Loadable<EngineRuntimeView>;
  catalog: Loadable<ConnectionRow[]>;
  captures?: readonly ConnectionCaptureRow[] | undefined;
}

/** The dialog's source step, in the precedence the module comment lists. */
export function intakeDialogView(input: IntakeDialogInput): IntakeDialogView {
  const entry = intakeEntry(input.access, input.sessionEngine);
  if (!entry.visible) return { kind: 'hidden' };
  if (entry.disabledReason) return { kind: 'not-configured', reason: entry.disabledReason };
  const roles = intakeMissingRoles(input.access);
  if (roles) {
    return { kind: 'missing-role', roles, principal: input.access.state === 'known' ? input.access.principal : '' };
  }
  const { runtime, catalog } = input;
  if (runtime.status === 'idle' || runtime.status === 'loading' || catalog.status === 'idle' || catalog.status === 'loading') {
    return { kind: 'loading' };
  }
  const errors = [runtime, catalog].flatMap((part) => (part.status === 'error' ? [part.message] : []));
  const sources = intakeSources(
    runtime.status === 'ok' ? runtime.value : null,
    catalog.status === 'ok' ? catalog.value : null,
    input.captures ?? []
  );
  if (sources.length === 0) {
    // "Nothing is mounted" is a claim only both reads can make.
    return errors.length > 0 ? { kind: 'error', messages: errors } : { kind: 'empty' };
  }
  return { kind: 'sources', sources, errors };
}

// ── Captures ─────────────────────────────────────────────────────────────────

/** Whether any capture of the session is armed: the polling condition. */
export function anyArmed(captures: readonly ConnectionCaptureRow[]): boolean {
  return captures.some((capture) => capture.status === 'ARMED');
}

/**
 * What changes when a sample reached the session: a capture's count, or a
 * capture finishing (a failed slot may still have written its sample).
 */
export function captureSampleSignature(captures: readonly ConnectionCaptureRow[]): string {
  return captures
    .filter((capture) => capture.mode === 'STREAM')
    .map((capture) => `${capture.id}:${capture.captured}:${capture.status === 'ARMED' ? 'a' : 'f'}`)
    .sort()
    .join('|');
}

/** `m:ss` for a countdown; never negative. */
export function formatRemaining(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000));
  const minutes = Math.floor(total / 60);
  const seconds = total % 60;
  return `${minutes}:${String(seconds).padStart(2, '0')}`;
}

/** After this long armed with nothing captured, the row says why that can be. */
export const STALL_NOTE_AFTER_MS = 30_000;

export const KERNEL_LIMITATION_NOTE =
  'Only messages the engine accepted are captured; messages with segments beyond MSH/EVN/PID/PV1 are currently rejected at admission.';

export type CaptureTone = 'info' | 'success' | 'neutral' | 'warning' | 'danger';

export interface CaptureRowView {
  id: string;
  sourceId: string;
  status: ConnectionCaptureRow['status'];
  armed: boolean;
  /** "2 / 5 captured". */
  progress: string;
  /** "expires in 4:12", "expiring", "complete", "expired", "cancelled", "failed". */
  detail: string;
  tone: CaptureTone;
  /** Armed with nothing captured for a while: show the kernel limitation. */
  stalled: boolean;
  problems: ConnectionCaptureRow['problems'];
}

const TERMINAL: Record<Exclude<ConnectionCaptureRow['status'], 'ARMED'>, { detail: string; tone: CaptureTone }> = {
  COMPLETE: { detail: 'complete', tone: 'success' },
  EXPIRED: { detail: 'expired', tone: 'neutral' },
  CANCELLED: { detail: 'cancelled', tone: 'neutral' },
  FAILED: { detail: 'failed', tone: 'danger' }
};

export function captureRowView(capture: ConnectionCaptureRow, nowMs: number): CaptureRowView {
  const armed = capture.status === 'ARMED';
  const progress = `${capture.captured} / ${capture.maxMessages} captured`;
  let detail: string;
  let tone: CaptureTone;
  if (capture.status === 'ARMED') {
    const remaining = Date.parse(capture.expiresAt) - nowMs;
    // Past expiresAt the tap is already inert; the server moves the row to
    // EXPIRED at its next refresh (≤ 2 s). The UI does not decide that.
    detail = remaining > 0 ? `expires in ${formatRemaining(remaining)}` : 'expiring';
    tone = 'info';
  } else {
    ({ detail, tone } = TERMINAL[capture.status]);
  }
  const armedFor = nowMs - Date.parse(capture.requestedAt);
  return {
    id: capture.id,
    sourceId: capture.sourceId,
    status: capture.status,
    armed,
    progress,
    detail,
    tone,
    stalled: armed && capture.captured === 0 && armedFor >= STALL_NOTE_AFTER_MS,
    problems: capture.problems
  };
}

/** Stream captures for the panel, newest first; peeks are audit rows, not capture rows. */
export function streamCaptures(captures: readonly ConnectionCaptureRow[], limit = 5): ConnectionCaptureRow[] {
  return [...captures]
    .filter((capture) => capture.mode === 'STREAM')
    .sort((a, b) => Date.parse(b.requestedAt) - Date.parse(a.requestedAt))
    .slice(0, limit);
}

// ── Samples ──────────────────────────────────────────────────────────────────

/** A captured or peeked session sample, ready for the tab-memory inbox. */
export interface SessionIntakeSample {
  sessionId: string;
  sampleId: string;
  name: string;
  /** The sample's `source`: `capture:<captureId>` or `peek:<captureId>`. */
  provenance: string;
  /** The runtime source ID the capture or peek read, when the session's audit rows name it. */
  sourceId: string | null;
  /** The capture-redacted text, or '' when the server withheld it. */
  raw: string;
  /** redactedPayload was null: the caller lacks integration.operator. */
  payloadWithheld: boolean;
}

const PROVENANCE = /^(capture|peek):(.+)$/;

/**
 * Keeps the samples that came from a connection (`capture:` / `peek:`
 * provenance) — the ones Preview added are already the editor's text — and
 * names each by the source its audit row read.
 */
export function intakeSamplesFrom(
  samples: readonly IntakeSessionSample[],
  captures: readonly ConnectionCaptureRow[]
): SessionIntakeSample[] {
  const byId = new Map(captures.map((capture) => [capture.id, capture]));
  return samples.flatMap((sample) => {
    const match = PROVENANCE.exec(sample.source ?? '');
    if (!match) return [];
    const capture = byId.get(match[2]!);
    return [
      {
        sessionId: sample.sessionId,
        sampleId: sample.id,
        name: sample.name,
        provenance: sample.source ?? '',
        sourceId: capture?.sourceId ?? null,
        raw: sample.redactedPayload ?? '',
        payloadWithheld: sample.redactedPayload === null
      }
    ];
  });
}

// ── Reasons and bounds ───────────────────────────────────────────────────────

export const MAX_INTAKE_REASON_BYTES = 1024;

/** The server refuses an empty or oversized reason; say so before it does. */
export function intakeReasonProblem(reason: string): string | null {
  const trimmed = reason.trim();
  if (trimmed === '') return 'A reason is required. It is recorded with your identity on the audit row.';
  if (new TextEncoder().encode(trimmed).length > MAX_INTAKE_REASON_BYTES) {
    return `A reason must be ${MAX_INTAKE_REASON_BYTES} bytes or fewer.`;
  }
  return null;
}

/**
 * An integer in [min, max], or the problem with it. `value` is what a number
 * input binds: a number, an empty value (null), or text.
 */
export function boundProblem(value: string | number | null | undefined, min: number, max: number): string | null {
  const trimmed = value === null || value === undefined ? '' : String(value).trim();
  if (trimmed === '') return `Enter a whole number from ${min} to ${max}.`;
  const parsed = Number(trimmed);
  if (!Number.isInteger(parsed) || parsed < min || parsed > max) {
    return `Enter a whole number from ${min} to ${max}.`;
  }
  return null;
}

export const CAPTURE_BOUNDS = { maxMessages: { min: 1, max: 100, fallback: 5 }, ttlSeconds: { min: 1, max: 900, fallback: 300 } } as const;
export const PEEK_BOUNDS = { maxObjects: { min: 1, max: 50, fallback: 10 }, maxMessages: { min: 1, max: 50, fallback: 5 } } as const;
