import { describe, expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/svelte';
import MessageTrace from './MessageTrace.svelte';

function attempt(attemptId: string, deliveries: unknown[]) {
  return {
    tenantId: 'tenant-a',
    attemptId,
    parentAttemptId: null,
    receiptId: 'receipt-a',
    eventId: 'event-a',
    traceId: 'trace-a',
    destination: {
      artifactId: 'destination-fhir',
      revisionId: 'destination-1',
      digest: 'sha256:' + '4'.repeat(64),
      class: 'production'
    },
    route: 'admit',
    action: 'send-fhir',
    status: 'succeeded',
    attemptCount: 1,
    recordedAt: '2026-09-24T09:00:00Z',
    scheduledAt: '2026-09-24T09:00:00Z',
    completedAt: '2026-09-24T09:06:00Z',
    lastErrorCode: '',
    lastErrorDetail: '',
    outboxStatus: 'published',
    topic: 'integration.delivery.v1',
    leaseOwner: '',
    leaseExpiresAt: null,
    deadLetter: null,
    deliveries
  };
}

function trace(attempts: unknown[]) {
  return {
    receipt: {
      tenantId: 'tenant-a',
      receiptId: 'receipt-a',
      status: 'accepted',
      recordedAt: '2026-09-24T09:00:00Z',
      correlationId: 'correlation-a',
      rawRetentionMode: 'ephemeral',
      integrationRevision: { artifactId: 'adt-http', revisionId: 'rev-1', digest: 'sha256:0' },
      principal: { id: 'adt-gateway', kind: 'service', authMethod: 'oauth2', roles: [] },
      reason: 'production ingress',
      eventCount: 0,
      attemptCount: attempts.length,
      failedAttemptCount: 0,
      deadLetterCount: 0
    },
    events: [],
    lineage: [],
    attempts,
    audit: []
  };
}

describe('MessageTrace delivery block', () => {
  it('shows the FHIR delivery recorded for an attempt', () => {
    render(MessageTrace, {
      props: {
        receiptId: 'receipt-a',
        // The generated query type is structurally what this fixture builds.
        trace: trace([
          attempt('attempt-a', [
            {
              transport: 'fhir',
              outcome: 'delivered',
              endpointAdvisory: 'https://fhir.example.test/r4',
              fhirResourceTypes: ['Patient', 'Encounter'],
              fhirEntryCount: 2,
              fhirOutcomeCodesAdvisory: []
            }
          ])
        ]) as never
      }
    });

    expect(screen.getByRole('heading', { name: 'Delivery' })).toBeInTheDocument();
    const block = screen.getByRole('list', {
      name: 'Destination deliveries for attempt-a, newest first'
    });
    expect(within(block).getByText('FHIR')).toBeInTheDocument();
    expect(within(block).getByText('Delivered')).toBeInTheDocument();
    expect(within(block).getByText('Patient')).toBeInTheDocument();
    expect(within(block).getByText('Encounter')).toBeInTheDocument();
    expect(within(block).getByText('2 bundle entries')).toBeInTheDocument();
    expect(within(block).getByText('https://fhir.example.test/r4')).toBeInTheDocument();
  });

  it('says so when this process contacted no destination for an attempt', () => {
    render(MessageTrace, {
      props: { receiptId: 'receipt-a', trace: trace([attempt('attempt-kafka', [])]) as never }
    });
    expect(screen.getByRole('heading', { name: 'Delivery' })).toBeInTheDocument();
    expect(screen.getByText(/contacted no destination for this attempt/i)).toBeInTheDocument();
  });
});
