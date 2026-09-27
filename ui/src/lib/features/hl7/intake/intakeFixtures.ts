/**
 * Synthetic fixtures for the intake tests: placeholder ids, digests and
 * messages only — no patient data.
 */
import type { AccessCapabilityState } from '$lib/graphql/accessCapabilities';
import { parseAuthStatus } from '$lib/graphql/accessCapabilities';
import type { ConnectionRow, EngineAdapterRow, EngineRuntimeView } from '$lib/features/connections/connectionsApi';
import type { ConnectionCaptureRow, IntakeSessionSample } from './intakeApi';

export const DIGEST = 'sha256:' + 'ab'.repeat(32);
export const OTHER_DIGEST = 'sha256:' + 'cd'.repeat(32);

export const BUNDLE = [
  'integration:preview',
  'graphql:operator',
  'clinical:read',
  'integration.operator',
  'integration.delivery.operator',
  'integration.deployment.operator'
];

export function authStatus(
  capabilities: Record<string, unknown> = {},
  missingRoles: Record<string, string[]> = {}
): Record<string, unknown> {
  return {
    authenticated: true,
    authVia: 'network',
    principal: 'e2e-ide-operator',
    roles: BUNDLE,
    capabilities: {
      operatorRead: true,
      operatorDelivery: true,
      operatorDeployment: true,
      clinicalRead: true,
      integrationSessions: true,
      streaming: true,
      subscriptions: ['integrationSessionEvents'],
      llm: { configured: false },
      connectionsRead: true,
      connectionsWrite: true,
      controlPlane: true,
      connectionCatalog: true,
      ...capabilities
    },
    missingRoles
  };
}

export function access(
  capabilities: Record<string, unknown> = {},
  missingRoles: Record<string, string[]> = {}
): AccessCapabilityState {
  return parseAuthStatus(authStatus(capabilities, missingRoles));
}

export function adapter(kind: string, enabled: boolean, extra: Partial<EngineAdapterRow> = {}): EngineAdapterRow {
  return {
    kind,
    enabled,
    definitionId: null,
    integrationId: null,
    sourceId: null,
    sourceRevisionId: null,
    sourceDigest: null,
    listenAddress: null,
    path: null,
    authMode: null,
    tlsMode: null,
    provider: null,
    pollSeconds: null,
    maxConnections: null,
    maxMessageBytes: null,
    maxBodyBytes: null,
    requireClientIdentity: null,
    requireWorkloadIdentity: null,
    queueDriver: null,
    maxAttempts: null,
    workerId: null,
    ...extra
  };
}

/** The e2e stack's runtime: four adapters, none enabled. */
export function runtime(adapters: EngineAdapterRow[] = []): EngineRuntimeView {
  const kinds = ['http', 'mllp', 'batch', 'delivery'];
  const rows = kinds.map((kind) => adapters.find((row) => row.kind === kind) ?? adapter(kind, false));
  return {
    version: 'test',
    tenantId: 'tenant-a',
    replicaId: 'replica-1',
    authMode: 'static',
    trustedNetwork: true,
    accessIdentity: false,
    controlPlane: true,
    integrationSessions: true,
    streaming: true,
    retentionPurge: false,
    llmConfigured: false,
    registry: { integrationCount: 1, integrations: [] },
    adapters: rows,
    destinationIdentity: null,
    ledgers: [],
    properties: []
  };
}

export function connection(overrides: Partial<ConnectionRow> = {}): ConnectionRow {
  return {
    id: 'adt-east-mllp',
    direction: 'SOURCE',
    kind: 'MLLP',
    name: 'ADT east',
    description: '',
    spec: { source_id: 'adt-east', listen_address: '0.0.0.0:22575' },
    secretBindings: [],
    version: 1,
    archived: false,
    latestRevision: {
      artifactId: 'adt-east-mllp',
      revisionId: '1',
      digest: DIGEST,
      compiledFromVersion: 1,
      createdAt: '2026-09-26T10:06:00Z'
    },
    references: [],
    runtime: { mounted: false, role: null, detail: null },
    createdBy: { id: 'e2e-ide-operator', kind: 'service' },
    createdAt: '2026-09-26T10:00:00Z',
    updatedBy: { id: 'e2e-ide-operator', kind: 'service' },
    updatedReason: 'synthetic source',
    updatedAt: '2026-09-26T10:05:00Z',
    ...overrides
  } as ConnectionRow;
}

export function batchConnection(overrides: Partial<ConnectionRow> = {}): ConnectionRow {
  return connection({
    id: 'adt-batch',
    kind: 'BATCH_S3',
    name: 'ADT batch',
    spec: { source_id: 'adt-batch-src', s3: { bucket: 'inbound', input_prefix: 'adt/' } },
    latestRevision: {
      artifactId: 'adt-batch',
      revisionId: '2',
      digest: OTHER_DIGEST,
      compiledFromVersion: 1,
      createdAt: '2026-09-26T10:06:00Z'
    } as ConnectionRow['latestRevision'],
    ...overrides
  });
}

export function capture(overrides: Partial<ConnectionCaptureRow> = {}): ConnectionCaptureRow {
  return {
    id: 'cap-1',
    sessionId: 'session-1',
    sourceId: 'adt-east',
    connectionId: null,
    mode: 'STREAM',
    status: 'ARMED',
    captured: 0,
    maxMessages: 5,
    expiresAt: '2026-09-27T12:05:00Z',
    requestedBy: { id: 'e2e-ide-operator', kind: 'service' },
    reason: 'synthetic capture',
    requestedAt: '2026-09-27T12:00:00Z',
    completedAt: null,
    problems: [],
    ...overrides
  };
}

export const REDACTED_TEXT = [
  'MSH|^~\\&|SYN|SYN|FI_FHIR|SYN|20260101090000||ADT^A01|SYN-0001|T|2.5.1',
  'EVN|A01|20260101090000',
  'PID|1||REDACTED||REDACTED||REDACTED|U',
  'PV1|1|I|WARD^ROOM^BED'
].join('\r');

export function sessionSample(overrides: Partial<IntakeSessionSample> = {}): IntakeSessionSample {
  return {
    id: 'sample_capture_cap-1_1',
    sessionId: 'session-1',
    name: 'capture cap-1 #1',
    format: 'HL7V2',
    source: 'capture:cap-1',
    redactedPayload: REDACTED_TEXT,
    createdAt: '2026-09-27T12:00:10Z',
    ...overrides
  };
}
