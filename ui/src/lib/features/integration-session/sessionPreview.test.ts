import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { DefinitionNode, DocumentNode } from 'graphql';
import { graphqlFetch } from '$lib/graphql/client';
import {
  PreviewIntegrationMessageDocument,
  type PreviewIntegrationMessageMutation
} from '$lib/gen/graphql';
import { parseHL7Preview } from '$lib/features/hl7/hl7Preview';
import { runAuthenticatedIntegrationPreview } from './api';

vi.mock('$lib/graphql/client', () => ({
  graphqlFetch: vi.fn()
}));

const mockFetch = graphqlFetch as unknown as ReturnType<typeof vi.fn>;

const rawMessage =
  'MSH|^~\\&|SENDING|FAC|RECEIVING|FAC|20260713120000||ADT^A01|control-123|P|2.5.1';

const enginePreview: PreviewIntegrationMessageMutation['previewIntegrationMessage'] = {
  __typename: 'IntegrationPreviewResult' as const,
  mode: 'preview',
  tenantId: 'tenant-a',
  integrationRevision: {
    __typename: 'IntegrationArtifactRevision' as const,
    artifactId: 'integration-adt',
    revisionId: 'definition-revision-1',
    digest: `sha256:${'a'.repeat(64)}`
  },
  artifactRevisions: {
    __typename: 'IntegrationExecutionArtifactRevisions' as const,
    source: {
      __typename: 'IntegrationArtifactRevision' as const,
      artifactId: 'source-adt',
      revisionId: 'source-1',
      digest: `sha256:${'b'.repeat(64)}`
    },
    profile: {
      __typename: 'IntegrationArtifactRevision' as const,
      artifactId: 'profile-adt',
      revisionId: '1',
      digest: `sha256:${'c'.repeat(64)}`
    },
    workflow: {
      __typename: 'IntegrationArtifactRevision' as const,
      artifactId: 'workflow-adt',
      revisionId: 'workflow-version-1',
      digest: `sha256:${'d'.repeat(64)}`
    }
  },
  events: [
    {
      __typename: 'IntegrationPreviewEvent' as const,
      tenantId: 'tenant-a',
      id: 'event-1',
      type: 'patient_admit',
      sourceMessageId: 'control-123',
      correlationId: 'correlation-123',
      classification: 'phi',
      payload: {
        id: 'event-1',
        type: 'patient_admit',
        timestamp: '2026-07-13T16:00:00Z',
        received_at: '2026-07-13T16:30:00Z',
        source: 'adt-east',
        source_format: 'hl7v2',
        source_profile_id: 'profile-adt',
        source_message_id: 'control-123',
        correlation_id: 'correlation-123',
        patient: {
          mrn: 'MRN-123',
          family_name: 'Patient',
          given_name: 'Test',
          date_of_birth: '1980-01-01T00:00:00Z',
          gender: 'F'
        },
        encounter: {
          id: 'visit-123',
          class: 'I',
          location: { facility: 'FAC', unit: 'UNIT', room: '101', bed: 'A' }
        }
      }
    }
  ],
  diagnostics: [
    {
      __typename: 'IntegrationPreviewDiagnostic' as const,
      tenantId: 'tenant-a',
      severity: 'warning',
      stage: 'semantic',
      code: 'EXAMPLE_WARNING',
      message: 'Example warning',
      path: 'PID-3',
      source: 'adt-east',
      classification: 'phi'
    }
  ],
  routes: [],
  deliveries: [],
  correlations: {
    __typename: 'IntegrationPreviewCorrelations' as const,
    tenantId: 'tenant-a',
    correlationId: 'correlation-123',
    traceId: null,
    sourceMessageId: 'control-123',
    eventIds: ['event-1'],
    workflowRunId: null
  }
};

function operationName(document: unknown): string {
  const doc = document as DocumentNode;
  const operation = doc.definitions.find((definition: DefinitionNode) =>
    definition.kind === 'OperationDefinition' ? definition : undefined
  );
  return operation && 'name' in operation && operation.name ? operation.name.value : '';
}

beforeEach(() => {
  mockFetch.mockReset();
});

describe('authenticated integration preview routing', () => {
  it('uses one stateless mutation and excludes all browser-owned binding fields', async () => {
    mockFetch.mockResolvedValue({ previewIntegrationMessage: enginePreview });

    const result = await parseHL7Preview({
      integrationId: 'adt-east',
      data: rawMessage,
      correlationId: 'correlation-123',
      reason: 'verify ADT mapping',
      source: 'browser-controlled-source',
      profileId: 'browser-controlled-profile',
      sessionId: 'legacy-session-id'
    });

    expect(mockFetch).toHaveBeenCalledTimes(1);
    expect(mockFetch).toHaveBeenCalledWith(PreviewIntegrationMessageDocument, {
      input: {
        integrationId: 'adt-east',
        data: rawMessage,
        correlationId: 'correlation-123',
        reason: 'verify ADT mapping'
      }
    });
    expect(operationName(mockFetch.mock.calls[0]![0])).toBe('PreviewIntegrationMessage');
    expect(result.preview).toBe(enginePreview);
    expect(result.session).toBeUndefined();
    expect(result.parsePreview).toMatchObject({
      success: true,
      errors: [],
      events: [
        {
          __typename: 'PatientAdmitEvent',
          id: 'event-1',
          type: 'PATIENT_ADMIT',
          source: 'adt-east',
          sourceFormat: 'HL7V2',
          correlationId: 'correlation-123',
          patient: { mrn: 'MRN-123', familyName: 'Patient', givenName: 'Test' },
          encounter: {
            class: 'I',
            location: { facility: 'FAC', unit: 'UNIT', room: '101', bed: 'A' }
          }
        }
      ],
      warnings: [
        {
          phase: 'semantic',
          code: 'EXAMPLE_WARNING',
          message: 'Example warning',
          path: 'PID-3',
          severity: 'warning'
        }
      ]
    });
  });

  it('exposes the same sole mutation through the integration-session API', async () => {
    mockFetch.mockResolvedValue({ previewIntegrationMessage: enginePreview });

    await runAuthenticatedIntegrationPreview({
      integrationId: 'adt-east',
      data: rawMessage,
      correlationId: 'correlation-123',
      reason: 'integration-session compatibility path'
    });

    expect(mockFetch).toHaveBeenCalledTimes(1);
    expect(operationName(mockFetch.mock.calls[0]![0])).toBe('PreviewIntegrationMessage');
  });

  it('uses the Vite-built public integration alias when the caller omits one', async () => {
    const env = import.meta.env as Record<string, string | undefined>;
    const previous = env.VITE_FI_FHIR_PREVIEW_INTEGRATION_ID;
    env.VITE_FI_FHIR_PREVIEW_INTEGRATION_ID = 'adt-east';
    mockFetch.mockResolvedValue({ previewIntegrationMessage: enginePreview });
    try {
      await runAuthenticatedIntegrationPreview({
        data: rawMessage,
        correlationId: 'correlation-123',
        reason: 'configured alias'
      });
      expect(mockFetch).toHaveBeenCalledWith(PreviewIntegrationMessageDocument, {
        input: {
          integrationId: 'adt-east',
          data: rawMessage,
          correlationId: 'correlation-123',
          reason: 'configured alias'
        }
      });
    } finally {
      env.VITE_FI_FHIR_PREVIEW_INTEGRATION_ID = previous;
    }
  });

  it('propagates failure without invoking a legacy fallback', async () => {
    mockFetch.mockRejectedValue(new Error('preview unavailable'));

    await expect(
      parseHL7Preview({
        integrationId: 'adt-east',
        data: rawMessage,
        correlationId: 'correlation-123',
        reason: 'verify ADT mapping'
      })
    ).rejects.toThrow('preview unavailable');

    expect(mockFetch).toHaveBeenCalledTimes(1);
    expect(operationName(mockFetch.mock.calls[0]![0])).toBe('PreviewIntegrationMessage');
  });

  it('rejects a response that contains a raw payload field', async () => {
    const unsafe = structuredClone(enginePreview);
    const payload = unsafe.events[0]!.payload as Record<string, unknown>;
    Object.assign(payload, { raw_payload: rawMessage });
    mockFetch.mockResolvedValue({ previewIntegrationMessage: unsafe });

    await expect(
      runAuthenticatedIntegrationPreview({
        integrationId: 'adt-east',
        data: rawMessage,
        correlationId: 'correlation-123',
        reason: 'verify ADT mapping'
      })
    ).rejects.toThrow('Invalid integration preview response at events.0.payload.raw_payload');
  });

  it('rejects nested raw payload fields before returning preview provenance', async () => {
    const unsafe = structuredClone(enginePreview);
    const payload = unsafe.events[0]!.payload as Record<string, unknown>;
    Object.assign(payload.patient as Record<string, unknown>, { rawMessage });
    mockFetch.mockResolvedValue({ previewIntegrationMessage: unsafe });

    await expect(
      runAuthenticatedIntegrationPreview({
        integrationId: 'adt-east',
        data: rawMessage,
        correlationId: 'correlation-123',
        reason: 'verify ADT mapping'
      })
    ).rejects.toThrow('Invalid integration preview response at events.0.payload.patient.rawMessage');
  });

  it.each([
    ['event tenant', (preview: typeof enginePreview) => (preview.events[0]!.tenantId = 'tenant-b'), 'events.0.tenantId'],
    [
      'diagnostic tenant',
      (preview: typeof enginePreview) => (preview.diagnostics[0]!.tenantId = 'tenant-b'),
      'diagnostics.0.tenantId'
    ],
    [
      'source message',
      (preview: typeof enginePreview) => (preview.correlations.sourceMessageId = 'control-other'),
      'correlations.sourceMessageId'
    ],
    [
      'event references',
      (preview: typeof enginePreview) => (preview.correlations.eventIds = ['event-other']),
      'correlations.eventIds'
    ]
  ])('rejects %s provenance drift', async (_name, mutate, path) => {
    const unsafe = structuredClone(enginePreview);
    mutate(unsafe);
    mockFetch.mockResolvedValue({ previewIntegrationMessage: unsafe });

    await expect(
      runAuthenticatedIntegrationPreview({
        integrationId: 'adt-east',
        data: rawMessage,
        correlationId: 'correlation-123',
        reason: 'verify ADT mapping'
      })
    ).rejects.toThrow(`Invalid integration preview response at ${path}`);
  });

  it('rejects any delivery that is not a linked suppressed preview plan', async () => {
    const unsafe = structuredClone(enginePreview);
    unsafe.routes.push({
      __typename: 'IntegrationPreviewRoute',
      tenantId: 'tenant-a',
      eventId: 'event-1',
      route: 'fhir-primary',
      matched: true,
      skipped: false,
      skipReason: null,
      transformCount: 0,
      plannedActions: ['send-fhir'],
      diagnosticCodes: []
    });
    unsafe.deliveries.push({
      __typename: 'IntegrationPreviewDelivery',
      tenantId: 'tenant-a',
      eventId: 'event-1',
      destination: {
        __typename: 'IntegrationPreviewDestination',
        artifactId: 'fhir-primary',
        revisionId: 'destination-1',
        digest: `sha256:${'e'.repeat(64)}`,
        class: 'production'
      },
      route: 'fhir-primary',
      action: 'send-fhir',
      status: 'succeeded',
      diagnosticCodes: []
    });
    mockFetch.mockResolvedValue({ previewIntegrationMessage: unsafe });

    await expect(
      runAuthenticatedIntegrationPreview({
        integrationId: 'adt-east',
        data: rawMessage,
        correlationId: 'correlation-123',
        reason: 'verify ADT mapping'
      })
    ).rejects.toThrow('Invalid integration preview response at deliveries.0.status');
  });

  it('fails before transport when no integration alias is configured', async () => {
    const env = import.meta.env as Record<string, string | undefined>;
    const previous = env.VITE_FI_FHIR_PREVIEW_INTEGRATION_ID;
    delete env.VITE_FI_FHIR_PREVIEW_INTEGRATION_ID;
    try {
      await expect(
        runAuthenticatedIntegrationPreview({
          data: rawMessage,
          correlationId: 'correlation-123',
          reason: 'verify ADT mapping'
        })
      ).rejects.toThrow('Preview integration is not configured');
      expect(mockFetch).not.toHaveBeenCalled();
    } finally {
      env.VITE_FI_FHIR_PREVIEW_INTEGRATION_ID = previous;
    }
  });
});
