import { graphqlFetch } from '$lib/graphql/client';
import { subscribe } from '$lib/graphql/subscriptions';
import {
  AddStreamingSessionSampleDocument,
  CreateStreamingIntegrationSessionDocument,
  PreviewIntegrationMessageDocument,
  RunStreamingSessionPreviewDocument,
  StreamIntegrationSessionEventsDocument,
  UpdateStreamingSessionProfileDocument,
  type AddStreamingSessionSampleMutation,
  type AddStreamingSessionSampleMutationVariables,
  type CreateStreamingIntegrationSessionMutation,
  type CreateStreamingIntegrationSessionMutationVariables,
  type IntegrationSessionRunFieldsFragment,
  type ParsePreviewQuery,
  type PreviewIntegrationMessageMutation,
  type PreviewIntegrationMessageMutationVariables,
  type RunStreamingSessionPreviewMutation,
  type RunStreamingSessionPreviewMutationVariables,
  type SourceProfile,
  type StreamIntegrationSessionEventsSubscription,
  type StreamIntegrationSessionEventsSubscriptionVariables,
  type UpdateStreamingSessionProfileMutation,
  type UpdateStreamingSessionProfileMutationVariables
} from '$lib/gen/graphql';
import type {
  AuthenticatedIntegrationPreviewResult,
  IntegrationSessionPreviewMeta
} from './types';

const DEFAULT_PREVIEW_REASON = 'interactive IDE preview';

export type AuthenticatedIntegrationPreviewInput = {
  data: string;
  integrationId?: string | null;
  correlationId?: string | null;
  reason?: string | null;

  // Compatibility-only editor context. These values are deliberately not sent
  // because source/profile/session binding is owned by the server registry.
  source?: string | null;
  profileId?: string | null;
  sessionId?: string | null;
  profile?: SourceProfile | null;
  onSessionUpdate?: (session: IntegrationSessionPreviewMeta) => void;
};

export function isIntegrationSessionEngineEnabled(): boolean {
  return import.meta.env.VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED === 'true';
}

export async function runAuthenticatedIntegrationPreview(
  input: AuthenticatedIntegrationPreviewInput
): Promise<AuthenticatedIntegrationPreviewResult> {
  if (isIntegrationSessionEngineEnabled()) {
    return runStreamingSessionPreview(input);
  }
  const integrationId =
    input.integrationId?.trim() || import.meta.env.VITE_FI_FHIR_PREVIEW_INTEGRATION_ID?.trim();
  if (!integrationId) {
    throw new Error('Preview integration is not configured');
  }

  const correlationId = input.correlationId?.trim() || newCorrelationID();
  const reason = input.reason?.trim() || DEFAULT_PREVIEW_REASON;
  const variables: PreviewIntegrationMessageMutationVariables = {
    input: {
      integrationId,
      data: input.data,
      correlationId,
      reason
    }
  };
  const response = await graphqlFetch<
    PreviewIntegrationMessageMutation,
    PreviewIntegrationMessageMutationVariables
  >(PreviewIntegrationMessageDocument, variables);

  validatePreviewResult(response.previewIntegrationMessage, correlationId);

  return {
    parsePreview: projectCurrentInspectorView(response.previewIntegrationMessage),
    preview: response.previewIntegrationMessage
  };
}

async function runStreamingSessionPreview(
  input: AuthenticatedIntegrationPreviewInput
): Promise<AuthenticatedIntegrationPreviewResult> {
  const sessionId = input.sessionId?.trim() || (await createSession());
  const sampleId = await addSessionSample(sessionId, input);
  if (input.profile) {
    await updateSessionProfile(sessionId, input.profile);
  }
  input.onSessionUpdate?.({
    mode: 'session',
    id: sessionId,
    sampleId,
    runId: null,
    state: 'pending',
    diagnostics: [],
    stages: [],
    lineage: [],
    streamState: 'connecting',
    error: null
  });

  let streamError: string | null = null;
  let opened = false;
  let runFinished = false;
  let resolveOpen: () => void = () => {};
  let rejectOpen: (error: Error) => void = () => {};
  const open = new Promise<void>((resolve, reject) => {
    resolveOpen = resolve;
    rejectOpen = reject;
  });
  const seenEvents = new Set<string>();
  const unsubscribe = subscribe<
    StreamIntegrationSessionEventsSubscription,
    StreamIntegrationSessionEventsSubscriptionVariables
  >(
    StreamIntegrationSessionEventsDocument,
    { sessionId },
    {
      onOpen: () => {
        opened = true;
        resolveOpen();
      },
      onData: (data) => {
        const event = data.integrationSessionEvents;
        if (seenEvents.has(event.id)) return;
        seenEvents.add(event.id);
        if (event.run) input.onSessionUpdate?.(projectSessionMeta(sessionId, sampleId, event.run, 'running', null));
      },
      onError: (error) => {
        streamError = error.message;
        if (!opened) rejectOpen(error);
      },
      onComplete: () => {
        if (!runFinished) streamError = 'Integration Session stream ended before the run completed';
      }
    }
  );

  try {
    await waitForStreamOpen(open);
    const response = await graphqlFetch<
      RunStreamingSessionPreviewMutation,
      RunStreamingSessionPreviewMutationVariables
    >(RunStreamingSessionPreviewDocument, {
      input: { sessionId, sampleId, data: null, format: null, source: input.source?.trim() || null }
    });
    runFinished = true;
    const run = response.runSessionPreview;
    const streamState = streamError ? 'error' : run.status === 'completed' ? 'complete' : 'error';
    const session = projectSessionMeta(sessionId, sampleId, run, streamState, streamError);
    input.onSessionUpdate?.(session);
    return {
      preview: null,
      parsePreview: projectSessionInspectorView(run),
      session
    };
  } finally {
    unsubscribe();
  }
}

async function waitForStreamOpen(open: Promise<void>): Promise<void> {
  let timeout: ReturnType<typeof setTimeout> | undefined;
  try {
    await Promise.race([
      open,
      new Promise<never>((_, reject) => {
        timeout = setTimeout(() => reject(new Error('Integration Session stream timed out')), 5000);
      })
    ]);
  } finally {
    if (timeout) clearTimeout(timeout);
  }
}

async function createSession(): Promise<string> {
  const response = await graphqlFetch<
    CreateStreamingIntegrationSessionMutation,
    CreateStreamingIntegrationSessionMutationVariables
  >(CreateStreamingIntegrationSessionDocument, {
    input: { name: 'HL7 source profile workspace', description: 'Mapping Studio live preview session' }
  });
  return response.createIntegrationSession.id;
}

async function addSessionSample(
  sessionId: string,
  input: AuthenticatedIntegrationPreviewInput
): Promise<string> {
  const response = await graphqlFetch<
    AddStreamingSessionSampleMutation,
    AddStreamingSessionSampleMutationVariables
  >(AddStreamingSessionSampleDocument, {
    input: {
      sessionId,
      name: 'Mapping Studio preview',
      format: 'HL7V2',
      data: input.data,
      source: input.source?.trim() || null,
      retainRawPayload: false,
      payloadRef: null
    }
  });
  return response.addSessionSample.id;
}

async function updateSessionProfile(sessionId: string, profile: SourceProfile): Promise<void> {
  await graphqlFetch<
    UpdateStreamingSessionProfileMutation,
    UpdateStreamingSessionProfileMutationVariables
  >(UpdateStreamingSessionProfileDocument, {
    input: {
      sessionId,
      name: profile.name,
      content: JSON.stringify(projectExecutableProfile(profile))
    }
  });
}

function projectExecutableProfile(profile: SourceProfile): Record<string, unknown> {
  const hl7v2 = profile.hl7v2;
  return {
    hl7v2: {
      default_version: hl7v2?.defaultVersion || '2.5.1',
      timezone: hl7v2?.timezone || 'UTC',
      tolerance: {
        missing_segments: hl7v2?.tolerance?.missingSegments || [],
        nte_anywhere: hl7v2?.tolerance?.nteAnywhere ?? false,
        extra_components: hl7v2?.tolerance?.extraComponents ?? false,
        unknown_segments: hl7v2?.tolerance?.unknownSegments ?? false,
        non_standard_delimiters: hl7v2?.tolerance?.nonStandardDelimiters ?? false
      },
      event_classifications: (hl7v2?.eventClassifications || []).map((classification) => ({
        message_type: classification.messageType,
        event_type: classification.eventType.toLowerCase(),
        priority: classification.priority
      }))
    }
  };
}

function projectSessionInspectorView(
  run: IntegrationSessionRunFieldsFragment
): ParsePreviewQuery['parsePreview'] {
  return {
    __typename: 'ParseResult',
    success: run.status === 'completed',
    errors: run.diagnostics.filter((entry) => entry.severity === 'error').map((entry) => entry.message),
    events: run.events,
    warnings: run.warnings
  };
}

function projectSessionMeta(
  sessionId: string,
  sampleId: string,
  run: IntegrationSessionRunFieldsFragment,
  streamState: IntegrationSessionPreviewMeta['streamState'],
  error: string | null
): IntegrationSessionPreviewMeta {
  return {
    mode: 'session',
    id: sessionId,
    sampleId,
    runId: run.id,
    state: run.status,
    diagnostics: run.diagnostics.map((diagnostic) => ({
      id: diagnostic.id,
      runId: diagnostic.runId,
      code: diagnostic.code,
      message: diagnostic.message,
      path: diagnostic.path,
      severity: diagnostic.severity,
      fixSuggestion: diagnostic.fixSuggestion,
      accepted: diagnostic.accepted,
      acceptedAt: diagnostic.acceptedAt,
      lineage: diagnostic.lineage
    })),
    stages: run.stages.map((stage) => ({
      id: stage.id,
      name: stage.name,
      status: stage.status,
      startedAt: stage.startedAt,
      completedAt: stage.completedAt,
      durationMs: stage.durationMs
    })),
    lineage: run.lineage,
    streamState,
    error
  };
}

function newCorrelationID(): string {
  if (globalThis.crypto && typeof globalThis.crypto.randomUUID === 'function') {
    return globalThis.crypto.randomUUID();
  }
  throw new Error('Secure correlation ID generation is unavailable');
}

type PreviewResult = PreviewIntegrationMessageMutation['previewIntegrationMessage'];
type PreviewEvent = PreviewResult['events'][number];
type InspectorEvent = ParsePreviewQuery['parsePreview']['events'][number];
type InspectorPatientAdmitEvent = Extract<InspectorEvent, { __typename: 'PatientAdmitEvent' }>;

function projectCurrentInspectorView(result: PreviewResult): ParsePreviewQuery['parsePreview'] {
  if (result.mode !== 'preview') {
    throw invalidPreview('mode');
  }

  return {
    __typename: 'ParseResult',
    success: true,
    errors: [],
    events: result.events.map(projectPatientAdmitEvent),
    warnings: result.diagnostics.map((diagnostic) => ({
      __typename: 'ParseWarning',
      phase: diagnostic.stage,
      code: diagnostic.code,
      message: diagnostic.message,
      path: diagnostic.path,
      explanation: null,
      fixSuggestion: null,
      impact: null,
      severity: diagnostic.severity || null,
      fromCache: null
    }))
  };
}

function projectPatientAdmitEvent(event: PreviewEvent): InspectorPatientAdmitEvent {
  if (event.type !== 'patient_admit') {
    throw invalidPreview('events.type');
  }

  const payload = objectValue(event.payload, 'events.payload');
  if (
    stringValue(payload, 'id', 'events.payload.id') !== event.id ||
    stringValue(payload, 'type', 'events.payload.type') !== event.type ||
    stringValue(payload, 'correlation_id', 'events.payload.correlation_id') !==
      event.correlationId
  ) {
    throw invalidPreview('events.payload provenance');
  }
  if (stringValue(payload, 'source_format', 'events.payload.source_format') !== 'hl7v2') {
    throw invalidPreview('events.payload.source_format');
  }

  const patient = objectField(payload, 'patient', 'events.payload.patient');
  const encounter = objectField(payload, 'encounter', 'events.payload.encounter');
  const location = optionalObjectField(encounter, 'location', 'events.payload.encounter.location');

  return {
    __typename: 'PatientAdmitEvent',
    id: event.id,
    type: 'PATIENT_ADMIT',
    timestamp: stringValue(payload, 'timestamp', 'events.payload.timestamp'),
    source: stringValue(payload, 'source', 'events.payload.source'),
    sourceFormat: 'HL7V2',
    correlationId: event.correlationId,
    patient: {
      __typename: 'Patient',
      mrn: stringValue(patient, 'mrn', 'events.payload.patient.mrn'),
      familyName: stringValue(patient, 'family_name', 'events.payload.patient.family_name'),
      givenName: stringValue(patient, 'given_name', 'events.payload.patient.given_name'),
      dateOfBirth: nullableStringValue(
        patient,
        'date_of_birth',
        'events.payload.patient.date_of_birth'
      ),
      gender: nullableStringValue(patient, 'gender', 'events.payload.patient.gender')
    },
    encounter: {
      __typename: 'Encounter',
      class: stringValue(encounter, 'class', 'events.payload.encounter.class'),
      location: projectLocation(location)
    }
  };
}

function validatePreviewResult(result: PreviewResult, expectedCorrelationId: string): void {
  rejectRawFieldsDeep(result);
  if (result.mode !== 'preview') {
    throw invalidPreview('mode');
  }

  const tenantId = canonicalString(result.tenantId, 'tenantId');
  validateArtifactRevision(result.integrationRevision, 'integrationRevision');
  validateArtifactRevision(result.artifactRevisions.source, 'artifactRevisions.source');
  validateArtifactRevision(result.artifactRevisions.profile, 'artifactRevisions.profile');
  validateArtifactRevision(result.artifactRevisions.workflow, 'artifactRevisions.workflow');

  const correlations = result.correlations;
  assertEqual(correlations.tenantId, tenantId, 'correlations.tenantId');
  assertEqual(correlations.correlationId, expectedCorrelationId, 'correlations.correlationId');
  if (correlations.workflowRunId != null) {
    throw invalidPreview('correlations.workflowRunId');
  }

  const eventIDs = new Set<string>();
  const eventSources = new Set<string>();
  const eventClassifications = new Set<string>();
  const sourceMessageIDs = new Set<string>();
  for (const [index, event] of result.events.entries()) {
    const path = `events.${index}`;
    assertEqual(event.tenantId, tenantId, `${path}.tenantId`);
    assertEqual(event.correlationId, expectedCorrelationId, `${path}.correlationId`);
    canonicalString(event.classification, `${path}.classification`);
    if (eventIDs.has(event.id)) {
      throw invalidPreview(`${path}.id`);
    }
    eventIDs.add(canonicalString(event.id, `${path}.id`));
    eventClassifications.add(event.classification);

    const payload = objectValue(event.payload, `${path}.payload`);
    assertEqual(stringValue(payload, 'id', `${path}.payload.id`), event.id, `${path}.payload.id`);
    assertEqual(
      stringValue(payload, 'type', `${path}.payload.type`),
      event.type,
      `${path}.payload.type`
    );
    assertEqual(
      stringValue(payload, 'correlation_id', `${path}.payload.correlation_id`),
      expectedCorrelationId,
      `${path}.payload.correlation_id`
    );
    assertEqual(
      stringValue(payload, 'source_format', `${path}.payload.source_format`),
      'hl7v2',
      `${path}.payload.source_format`
    );
    assertEqual(
      stringValue(payload, 'source_profile_id', `${path}.payload.source_profile_id`),
      result.artifactRevisions.profile.artifactId,
      `${path}.payload.source_profile_id`
    );
    eventSources.add(canonicalString(stringValue(payload, 'source', `${path}.payload.source`), `${path}.payload.source`));

    const payloadSourceMessageID = optionalStringValue(
      payload,
      'source_message_id',
      `${path}.payload.source_message_id`
    );
    if (payloadSourceMessageID !== event.sourceMessageId) {
      throw invalidPreview(`${path}.sourceMessageId`);
    }
    if (event.sourceMessageId != null) {
      sourceMessageIDs.add(canonicalString(event.sourceMessageId, `${path}.sourceMessageId`));
    }
  }

  if (sourceMessageIDs.size > 1) {
    throw invalidPreview('events.sourceMessageId');
  }
  if (sourceMessageIDs.size === 1 && !sourceMessageIDs.has(correlations.sourceMessageId ?? '')) {
    throw invalidPreview('correlations.sourceMessageId');
  }
  assertExactIDSet(correlations.eventIds, eventIDs, 'correlations.eventIds');

  for (const [index, diagnostic] of result.diagnostics.entries()) {
    const path = `diagnostics.${index}`;
    assertEqual(diagnostic.tenantId, tenantId, `${path}.tenantId`);
    canonicalString(diagnostic.code, `${path}.code`);
    canonicalString(diagnostic.classification, `${path}.classification`);
    if (eventClassifications.size > 0 && !eventClassifications.has(diagnostic.classification)) {
      throw invalidPreview(`${path}.classification`);
    }
    if (diagnostic.source != null && eventSources.size > 0 && !eventSources.has(diagnostic.source)) {
      throw invalidPreview(`${path}.source`);
    }
  }

  for (const [index, route] of result.routes.entries()) {
    const path = `routes.${index}`;
    assertEqual(route.tenantId, tenantId, `${path}.tenantId`);
    if (!eventIDs.has(route.eventId)) {
      throw invalidPreview(`${path}.eventId`);
    }
    canonicalString(route.route, `${path}.route`);
  }

  for (const [index, delivery] of result.deliveries.entries()) {
    const path = `deliveries.${index}`;
    assertEqual(delivery.tenantId, tenantId, `${path}.tenantId`);
    if (!eventIDs.has(delivery.eventId)) {
      throw invalidPreview(`${path}.eventId`);
    }
    if (delivery.status !== 'suppressed') {
      throw invalidPreview(`${path}.status`);
    }
    validateArtifactRevision(delivery.destination, `${path}.destination`);
    const route = result.routes.find(
      (candidate) => candidate.eventId === delivery.eventId && candidate.route === delivery.route
    );
    if (!route || !route.plannedActions.includes(delivery.action)) {
      throw invalidPreview(`${path}.route`);
    }
  }
}

function validateArtifactRevision(
  revision: { artifactId: string; revisionId: string; digest: string },
  path: string
): void {
  canonicalString(revision.artifactId, `${path}.artifactId`);
  canonicalString(revision.revisionId, `${path}.revisionId`);
  if (!/^sha256:[0-9a-f]{64}$/.test(revision.digest)) {
    throw invalidPreview(`${path}.digest`);
  }
}

function canonicalString(value: string, path: string): string {
  if (!value || value.trim() !== value) {
    throw invalidPreview(path);
  }
  for (const character of value) {
    const codePoint = character.codePointAt(0) ?? 0;
    if (/\s/u.test(character) || codePoint < 0x20 || codePoint === 0x7f) {
      throw invalidPreview(path);
    }
  }
  return value;
}

function assertEqual(actual: string, expected: string, path: string): void {
  if (actual !== expected) {
    throw invalidPreview(path);
  }
}

function assertExactIDSet(values: string[], expected: Set<string>, path: string): void {
  const actual = new Set(values);
  if (actual.size !== values.length || actual.size !== expected.size) {
    throw invalidPreview(path);
  }
  for (const value of actual) {
    if (!expected.has(value)) {
      throw invalidPreview(path);
    }
  }
}

function optionalStringValue(
  value: Record<string, unknown>,
  key: string,
  path: string
): string | null {
  const field = value[key];
  if (field == null || field === '') {
    return null;
  }
  if (typeof field !== 'string') {
    throw invalidPreview(path);
  }
  return field;
}

function rejectRawFieldsDeep(value: unknown, path = '', seen = new WeakSet<object>()): void {
  if (!value || typeof value !== 'object') {
    return;
  }
  if (seen.has(value)) {
    throw invalidPreview(path || 'response');
  }
  seen.add(value);
  if (Array.isArray(value)) {
    value.forEach((item, index) => rejectRawFieldsDeep(item, joinResponsePath(path, String(index)), seen));
    return;
  }
  for (const [key, field] of Object.entries(value as Record<string, unknown>)) {
    const fieldPath = joinResponsePath(path, key);
    const normalized = key.replaceAll('_', '').toLowerCase();
    if (normalized === 'rawpayload' || normalized === 'rawmessage') {
      throw invalidPreview(fieldPath);
    }
    rejectRawFieldsDeep(field, fieldPath, seen);
  }
}

function joinResponsePath(parent: string, child: string): string {
  return parent ? `${parent}.${child}` : child;
}

function projectLocation(
  location: Record<string, unknown> | null
): InspectorPatientAdmitEvent['encounter']['location'] {
  if (!location) {
    return null;
  }
  const projected = {
    __typename: 'Location' as const,
    facility: nullableStringValue(location, 'facility', 'events.payload.encounter.location.facility'),
    unit: nullableStringValue(location, 'unit', 'events.payload.encounter.location.unit'),
    room: nullableStringValue(location, 'room', 'events.payload.encounter.location.room'),
    bed: nullableStringValue(location, 'bed', 'events.payload.encounter.location.bed')
  };
  if (!projected.facility && !projected.unit && !projected.room && !projected.bed) {
    return null;
  }
  return projected;
}

function objectValue(value: unknown, path: string): Record<string, unknown> {
  if (!value || typeof value !== 'object' || Array.isArray(value)) {
    throw invalidPreview(path);
  }
  return value as Record<string, unknown>;
}

function objectField(
  value: Record<string, unknown>,
  key: string,
  path: string
): Record<string, unknown> {
  return objectValue(value[key], path);
}

function optionalObjectField(
  value: Record<string, unknown>,
  key: string,
  path: string
): Record<string, unknown> | null {
  if (value[key] == null) {
    return null;
  }
  return objectValue(value[key], path);
}

function stringValue(value: Record<string, unknown>, key: string, path: string): string {
  if (typeof value[key] !== 'string') {
    throw invalidPreview(path);
  }
  return value[key];
}

function nullableStringValue(
  value: Record<string, unknown>,
  key: string,
  path: string
): string | null {
  const field = value[key];
  if (field == null || field === '') {
    return null;
  }
  if (typeof field !== 'string') {
    throw invalidPreview(path);
  }
  return field;
}

function invalidPreview(path: string): Error {
  return new Error(`Invalid integration preview response at ${path}`);
}
