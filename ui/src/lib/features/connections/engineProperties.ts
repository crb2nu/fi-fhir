/**
 * How the Engine tab presents `engineRuntime` (.loom/38 "Engine properties").
 *
 * Process properties are immutable per process: the way to change one is the
 * GitOps environment, so each shown property carries the variable that sets
 * it — but only where the runtime itself lists that variable in its property
 * allowlist (`engineRuntime.properties`). The UI never names a key the server
 * did not.
 */
import type { EngineAdapterRow, EngineRuntimeView } from './connectionsApi';

type AdapterField = Exclude<keyof EngineAdapterRow, '__typename' | 'kind' | 'enabled'>;

export interface AdapterFieldSpec {
  field: AdapterField;
  label: string;
  mono?: boolean | undefined;
  /** The variable that sets this property, when one does. */
  envKey?: string | undefined;
}

export interface AdapterPresentation {
  title: string;
  /** Variables that mount the adapter at startup. */
  enableKeys: readonly string[];
  fields: readonly AdapterFieldSpec[];
  /** One sentence the panel always shows; `{ code }` segments render as code. */
  note?: ReadonlyArray<string | { code: string }> | undefined;
}

export const ADAPTER_PRESENTATION: Record<string, AdapterPresentation> = {
  http: {
    title: 'HTTP ingress',
    enableKeys: ['FI_FHIR_HTTP_INGRESS_INTEGRATION_ID'],
    note: [
      'Configured from ',
      { code: 'FI_FHIR_HTTP_INGRESS_*' },
      ' at startup; an HTTP source connection is a declaration the runtime does not load.'
    ],
    fields: [
      { field: 'integrationId', label: 'Integration', mono: true, envKey: 'FI_FHIR_HTTP_INGRESS_INTEGRATION_ID' },
      { field: 'definitionId', label: 'Definition', mono: true },
      { field: 'sourceId', label: 'Source ID', mono: true },
      { field: 'sourceRevisionId', label: 'Source revision', mono: true },
      { field: 'sourceDigest', label: 'Source digest', mono: true },
      { field: 'listenAddress', label: 'Listen address', mono: true },
      { field: 'path', label: 'Path', mono: true },
      { field: 'authMode', label: 'Auth mode', mono: true, envKey: 'FI_FHIR_HTTP_INGRESS_AUTH_MODE' },
      { field: 'maxBodyBytes', label: 'Max body bytes', mono: true, envKey: 'FI_FHIR_HTTP_INGRESS_MAX_BODY_BYTES' }
    ]
  },
  mllp: {
    title: 'MLLP listener',
    enableKeys: ['FI_FHIR_MLLP_SOURCE_CONFIG_PATH', 'FI_FHIR_MLLP_DEFINITION_ID'],
    fields: [
      { field: 'definitionId', label: 'Definition', mono: true, envKey: 'FI_FHIR_MLLP_DEFINITION_ID' },
      { field: 'integrationId', label: 'Integration', mono: true },
      { field: 'sourceId', label: 'Source ID', mono: true },
      { field: 'sourceRevisionId', label: 'Source revision', mono: true, envKey: 'FI_FHIR_MLLP_SOURCE_CONFIG_PATH' },
      { field: 'sourceDigest', label: 'Source digest', mono: true },
      { field: 'listenAddress', label: 'Listen address', mono: true },
      { field: 'tlsMode', label: 'TLS mode', mono: true },
      { field: 'maxConnections', label: 'Max connections', mono: true },
      { field: 'maxMessageBytes', label: 'Max message bytes', mono: true },
      {
        field: 'requireClientIdentity',
        label: 'Require client identity',
        envKey: 'FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY'
      }
    ]
  },
  batch: {
    title: 'Batch runner',
    enableKeys: ['FI_FHIR_BATCH_SOURCE_CONFIG_PATH', 'FI_FHIR_BATCH_DEFINITION_ID'],
    fields: [
      { field: 'definitionId', label: 'Definition', mono: true, envKey: 'FI_FHIR_BATCH_DEFINITION_ID' },
      { field: 'integrationId', label: 'Integration', mono: true },
      { field: 'sourceId', label: 'Source ID', mono: true },
      { field: 'sourceRevisionId', label: 'Source revision', mono: true, envKey: 'FI_FHIR_BATCH_SOURCE_CONFIG_PATH' },
      { field: 'sourceDigest', label: 'Source digest', mono: true },
      { field: 'provider', label: 'Provider', mono: true },
      { field: 'pollSeconds', label: 'Poll seconds', mono: true },
      { field: 'maxMessageBytes', label: 'Max message bytes', mono: true },
      {
        field: 'requireWorkloadIdentity',
        label: 'Require workload identity',
        envKey: 'FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY'
      },
      { field: 'workerId', label: 'Worker ID', mono: true, envKey: 'FI_FHIR_BATCH_WORKER_ID' }
    ]
  },
  delivery: {
    title: 'Delivery worker',
    enableKeys: ['FI_FHIR_DELIVERY_WORKER_ENABLED'],
    fields: [
      { field: 'queueDriver', label: 'Queue driver', mono: true, envKey: 'FI_FHIR_QUEUE_DRIVER' },
      { field: 'maxAttempts', label: 'Max attempts', mono: true, envKey: 'FI_FHIR_DELIVERY_MAX_ATTEMPTS' },
      { field: 'workerId', label: 'Worker ID', mono: true, envKey: 'FI_FHIR_DELIVERY_WORKER_ID' }
    ]
  }
};

export const DESTINATION_IDENTITY_KEYS = ['FI_FHIR_DELIVERY_IDENTITY_REGISTRY_PATH', 'FI_FHIR_DELIVERY_IDENTITY_MODE'];

/** The presentation of one adapter row; an unexpected kind gets a plain one. */
export function adapterPresentation(kind: string): AdapterPresentation {
  return ADAPTER_PRESENTATION[kind] ?? { title: kind, enableKeys: [], fields: [] };
}

/** The variables the runtime lists in its property allowlist. */
export function listedKeys(runtime: Pick<EngineRuntimeView, 'properties'>): Set<string> {
  return new Set(runtime.properties.map((property) => property.key));
}

/** `envKey` when the runtime lists it, else undefined. */
export function listedKey(listed: ReadonlySet<string>, envKey: string | undefined): string | undefined {
  return envKey && listed.has(envKey) ? envKey : undefined;
}

/**
 * A secret property as the page shows it: exactly `unset` when the runtime
 * says so and `set` otherwise — never the string the server sent, so a
 * server that broke its own contract still cannot put a value on screen.
 */
export function secretDisplay(value: string): 'set' | 'unset' {
  return value === 'unset' ? 'unset' : 'set';
}

export function yesNo(value: boolean | null | undefined): string | null {
  if (value === null || value === undefined) return null;
  return value ? 'Yes' : 'No';
}

export function onOff(value: boolean): string {
  return value ? 'On' : 'Off';
}

/** An adapter field as display text, or null when the runtime reports none. */
export function adapterValue(adapter: EngineAdapterRow, field: AdapterField): string | null {
  const value = adapter[field];
  if (value === null || value === undefined || value === '') return null;
  if (typeof value === 'boolean') return yesNo(value);
  return String(value);
}
