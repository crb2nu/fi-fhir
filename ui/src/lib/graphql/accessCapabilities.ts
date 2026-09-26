/**
 * What this deployment lets the signed-in identity do.
 *
 * Populated once by the credential gate from `GET /api/auth/status` (the same
 * fetch that decides whether the gate steps aside — there is no second fetch).
 * The server derives every flag from the identity's roles and its own runtime
 * configuration, so a surface can explain what is missing instead of
 * discovering it by failing.
 *
 * An older server answers with two keys (`authenticated`, `authVia`) and no
 * `capabilities`; that is the **unknown** state, and every surface keeps its
 * pre-capability behaviour (try, then render the failure inline).
 */
import { derived, get, writable, type Readable } from 'svelte/store';

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
      llmConfigured: llm && typeof llm.configured === 'boolean' ? llm.configured : null
    },
    missingRoles: missing
  };
}

/** Records the status body the credential gate fetched. */
export function setAccessStatus(body: unknown): void {
  store.set(parseAuthStatus(body));
}

/** Back to unknown (credential cleared, gate destroyed, tests). */
export function resetAccessCapabilities(): void {
  store.set(UNKNOWN);
}

/** Snapshot read for non-reactive callers (API helpers). */
export function currentAccessCapabilities(): AccessCapabilityState {
  return get(store);
}

/** `true`/`false` when the server reported the capability, `null` when unknown. */
export function capabilityOf(state: AccessCapabilityState, key: CapabilityKey): boolean | null {
  return state.state === 'known' ? state.capabilities[key] : null;
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
