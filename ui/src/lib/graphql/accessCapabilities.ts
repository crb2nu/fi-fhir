/**
 * What this deployment lets the signed-in identity do.
 *
 * Populated by the credential gate from `GET /api/auth/status` (the same fetch
 * that decides whether the gate steps aside), and re-probed with `refresh()`
 * after a bearer is installed or `refreshSoon()` when the server refuses work
 * the tab believed it could do — so a rollback or a new credential is picked up
 * without reloading the tab.
 * The server derives every flag from the identity's roles and its own runtime
 * configuration, so a surface can explain what is missing instead of
 * discovering it by failing.
 *
 * An older server answers with two keys (`authenticated`, `authVia`) and no
 * `capabilities`; that is the **unknown** state, and every surface keeps its
 * pre-capability behaviour (try, then render the failure inline).
 */
import { derived, get, writable, type Readable } from 'svelte/store';
import { requireGraphQLAuthorization } from './credentials';

/** Capability flags the API reports (`capabilities.*`). */
export interface AccessCapabilities {
  operatorRead: boolean;
  operatorDelivery: boolean;
  operatorDeployment: boolean;
  clinicalRead: boolean;
  integrationSessions: boolean;
  streaming: boolean;
  /**
   * Subscription root fields this caller may open over SSE. `null` when the
   * server did not report the list (then only `streaming` is known).
   */
  subscriptions: string[] | null;
  /** Whether the API has an LLM provider configured; `null` when not reported. */
  llmConfigured: boolean | null;
  /**
   * The connection catalog (.loom/38). Role capabilities: reading connections
   * and the engine runtime, and changing connections. Deployment capabilities:
   * the operator control plane and the catalog it migrates are configured.
   * Each is `null` when the server predates it — unknown, which never blocks.
   */
  connectionsRead: boolean | null;
  connectionsWrite: boolean | null;
  controlPlane: boolean | null;
  connectionCatalog: boolean | null;
  /**
   * The identity may export a session bundle with raw sample payloads (it
   * holds `integration.phi.export`, .loom/42 E-3); `null` when not reported.
   */
  phiExport: boolean | null;
  /**
   * Definition authoring (.loom/42 E-1): a deployment fact, like
   * `controlPlane`. The roles its writes need are in
   * `missingRoles.definitionAuthoring`.
   */
  definitionAuthoring: boolean | null;
  /**
   * The terminology mapping store is composed on this deployment: a deployment
   * fact, like `connectionCatalog`. `false` means the Terminology page has no
   * backend, so it says so instead of querying; `null` when not reported.
   */
  terminologyStore: boolean | null;
}

export type CapabilityKey = keyof Omit<AccessCapabilities, 'subscriptions' | 'llmConfigured'>;

export type AccessCapabilityState =
  | { state: 'unknown' }
  | {
      state: 'known';
      authVia: string;
      principal: string;
      roles: string[];
      capabilities: AccessCapabilities;
      /** Roles the identity would need, per capability (empty or absent when held). */
      missingRoles: Record<string, string[]>;
    };

const UNKNOWN: AccessCapabilityState = { state: 'unknown' };

const store = writable<AccessCapabilityState>(UNKNOWN);

/** The current capability state (read-only; the credential gate owns writes). */
export const accessCapabilities: Readable<AccessCapabilityState> = { subscribe: store.subscribe };

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function stringList(value: unknown): string[] {
  return Array.isArray(value) ? value.filter((entry): entry is string => typeof entry === 'string') : [];
}

/** A reported boolean, or `null` when the server did not report the key. */
function reportedBoolean(value: unknown): boolean | null {
  return typeof value === 'boolean' ? value : null;
}

/**
 * Parses an `/api/auth/status` body. Anything without a `capabilities` object
 * carrying a boolean `operatorRead` is the old shape and stays unknown.
 */
export function parseAuthStatus(body: unknown): AccessCapabilityState {
  if (!isRecord(body) || body.authenticated !== true) return UNKNOWN;
  const caps = body.capabilities;
  if (!isRecord(caps) || typeof caps.operatorRead !== 'boolean') return UNKNOWN;

  const llm = isRecord(caps.llm) ? caps.llm : null;
  const missing: Record<string, string[]> = {};
  if (isRecord(body.missingRoles)) {
    for (const [key, roles] of Object.entries(body.missingRoles)) {
      missing[key] = stringList(roles);
    }
  }

  return {
    state: 'known',
    authVia: typeof body.authVia === 'string' ? body.authVia : '',
    principal: typeof body.principal === 'string' ? body.principal : '',
    roles: stringList(body.roles),
    capabilities: {
      operatorRead: caps.operatorRead === true,
      operatorDelivery: caps.operatorDelivery === true,
      operatorDeployment: caps.operatorDeployment === true,
      clinicalRead: caps.clinicalRead === true,
      integrationSessions: caps.integrationSessions === true,
      streaming: caps.streaming === true,
      subscriptions: Array.isArray(caps.subscriptions) ? stringList(caps.subscriptions) : null,
      llmConfigured: llm && typeof llm.configured === 'boolean' ? llm.configured : null,
      connectionsRead: reportedBoolean(caps.connectionsRead),
      connectionsWrite: reportedBoolean(caps.connectionsWrite),
      controlPlane: reportedBoolean(caps.controlPlane),
      connectionCatalog: reportedBoolean(caps.connectionCatalog),
      phiExport: reportedBoolean(caps.phiExport),
      definitionAuthoring: reportedBoolean(caps.definitionAuthoring),
      terminologyStore: reportedBoolean(caps.terminologyStore)
    },
    missingRoles: missing
  };
}

// Bumped by every write so a probe that resolves after the credential changed
// (cleared, replaced) cannot publish the previous identity's capabilities.
let generation = 0;

/** Records the status body the credential gate fetched. */
export function setAccessStatus(body: unknown): void {
  generation += 1;
  store.set(parseAuthStatus(body));
}

/** Back to unknown (credential cleared, gate destroyed, tests). */
export function resetAccessCapabilities(): void {
  generation += 1;
  lastProbeAt = Number.NEGATIVE_INFINITY;
  pendingProbe = null;
  store.set(UNKNOWN);
}

/**
 * The API's refusal while its Integration Session workspace is off
 * (`ErrLegacyExecutionUnavailable`, internal/api/graphql/resolvers/resolver.go).
 * A tab that still believes sessions are on hears it after a rollback.
 */
export const LEGACY_EXECUTION_UNAVAILABLE = 'legacy integration execution is unavailable';

/** Calls to `refreshSoon` within this window share one probe. */
export const REFRESH_COALESCE_MS = 2000;

let lastProbeAt = Number.NEGATIVE_INFINITY;
let pendingProbe: Promise<void> | null = null;

const probesInFlight = writable(0);

/**
 * True while a `refresh()` status probe has not answered. A bearer session
 * mounts the IDE before its first probe returns, so `unknown` alone does not
 * say whether capabilities are still coming (wait) or never will be (an older
 * server: proceed). `unknown` with no probe in flight is settled.
 */
export const accessProbePending: Readable<boolean> = derived(probesInFlight, (count) => count > 0);

/**
 * Re-reads `/api/auth/status` with the current credential and publishes it.
 * A failed probe (no credential, network error, non-2xx) keeps the current
 * state: an outage is not evidence that a capability went away.
 */
export async function refresh(): Promise<void> {
  const started = generation;
  probesInFlight.update((count) => count + 1);
  try {
    const authorization = await requireGraphQLAuthorization();
    const headers: Record<string, string> = { Accept: 'application/json' };
    if (authorization) headers.Authorization = authorization;
    const response = await fetch('/api/auth/status', { headers, cache: 'no-store' });
    if (!response.ok) return;
    const body: unknown = await response.json();
    if (generation !== started) return;
    setAccessStatus(body);
  } catch {
    // Keep what the tab already knows.
  } finally {
    probesInFlight.update((count) => count - 1);
  }
}

/**
 * `refresh()`, at most once per `REFRESH_COALESCE_MS`: a burst of refused
 * mutations costs one probe. Calls inside the window share the probe already
 * started.
 */
export function refreshSoon(): Promise<void> {
  const now = Date.now();
  if (pendingProbe && now - lastProbeAt < REFRESH_COALESCE_MS) return pendingProbe;
  lastProbeAt = now;
  pendingProbe = refresh();
  return pendingProbe;
}

/** Re-probes (coalesced) when an error is the legacy-execution refusal. */
export function refreshOnLegacyRefusal(message: string): void {
  if (message.toLowerCase().includes(LEGACY_EXECUTION_UNAVAILABLE)) {
    void refreshSoon();
  }
}

/** Snapshot read for non-reactive callers (API helpers). */
export function currentAccessCapabilities(): AccessCapabilityState {
  return get(store);
}

/**
 * `true`/`false` when the server reported the capability, `null` when unknown
 * (no capabilities at all, or a server that predates this key).
 */
export function capabilityOf(state: AccessCapabilityState, key: CapabilityKey): boolean | null {
  return state.state === 'known' ? state.capabilities[key] : null;
}

/**
 * Whether the deployment has an LLM configured, as the status endpoint
 * reported it; `null` when unknown.
 */
export function llmConfiguredReported(state: AccessCapabilityState): boolean | null {
  return state.state === 'known' ? state.capabilities.llmConfigured : null;
}

function capabilityStore(key: CapabilityKey): Readable<boolean | null> {
  return derived(store, ($state) => capabilityOf($state, key));
}

export const operatorReadCapability = capabilityStore('operatorRead');
export const operatorDeliveryCapability = capabilityStore('operatorDelivery');
export const operatorDeploymentCapability = capabilityStore('operatorDeployment');
export const clinicalReadCapability = capabilityStore('clinicalRead');
export const integrationSessionsCapability = capabilityStore('integrationSessions');
export const streamingCapability = capabilityStore('streaming');
export const connectionsReadCapability = capabilityStore('connectionsRead');
export const connectionsWriteCapability = capabilityStore('connectionsWrite');
export const controlPlaneCapability = capabilityStore('controlPlane');
export const connectionCatalogCapability = capabilityStore('connectionCatalog');
export const phiExportCapability = capabilityStore('phiExport');
export const terminologyStoreCapability = capabilityStore('terminologyStore');

/** Allowlisted subscription roots, or `null` when unknown / not reported. */
export const subscriptionRoots: Readable<string[] | null> = derived(store, ($state) =>
  $state.state === 'known' ? $state.capabilities.subscriptions : null
);

/** Missing roles per capability; empty when unknown or nothing is missing. */
export const missingRoles: Readable<Record<string, string[]>> = derived(store, ($state) =>
  $state.state === 'known' ? $state.missingRoles : {}
);

/** Missing roles for one capability, from a state snapshot. */
export function missingRolesFor(state: AccessCapabilityState, key: CapabilityKey): string[] {
  return state.state === 'known' ? (state.missingRoles[key] ?? []) : [];
}
