import { describe, expect, it } from 'vitest';
import {
  attemptStatusVariant,
  circuitStateVariant,
  deadLetterStateLabel,
  deliveryActionBlockedReason,
  deploymentActionBlockedReason,
  deploymentHealthVariant,
  deploymentStateVariant,
  deliveryOutcomeVariant,
  describeDestinationDelivery,
  formatTimestamp,
  outboxStatusVariant,
  shortDigest
} from './attemptPresentation';

describe('status variants', () => {
  it('maps durable attempt statuses', () => {
    expect(attemptStatusVariant('succeeded')).toBe('success');
    expect(attemptStatusVariant('failed')).toBe('danger');
    expect(attemptStatusVariant('queued')).toBe('info');
    expect(attemptStatusVariant('unknown-future-status')).toBe('default');
  });

  it('maps durable outbox statuses including the lease state', () => {
    expect(outboxStatusVariant('published')).toBe('success');
    expect(outboxStatusVariant('failed')).toBe('danger');
    expect(outboxStatusVariant('leased')).toBe('warning');
    expect(outboxStatusVariant('pending')).toBe('info');
  });

  it('maps circuit, deployment state, and health', () => {
    expect(circuitStateVariant('open')).toBe('danger');
    expect(circuitStateVariant('closed')).toBe('success');
    expect(deploymentStateVariant('deployed')).toBe('success');
    expect(deploymentStateVariant('paused')).toBe('warning');
    expect(deploymentStateVariant('retired')).toBe('danger');
    expect(deploymentHealthVariant('degraded')).toBe('warning');
    expect(deploymentHealthVariant('unknown')).toBe('default');
  });
});

describe('deadLetterStateLabel', () => {
  it('distinguishes never-dead-lettered from open and from each resolution', () => {
    expect(deadLetterStateLabel(null)).toBe('Never dead-lettered');
    expect(deadLetterStateLabel({ active: true, resolution: '' })).toMatch(/awaiting operator/i);
    expect(deadLetterStateLabel({ active: false, resolution: 'replayed' })).toMatch(/replay/i);
    expect(deadLetterStateLabel({ active: false, resolution: 'resubmitted' })).toMatch(/resubmit/i);
    expect(deadLetterStateLabel({ active: false, resolution: 'discarded' })).toMatch(/discard/i);
  });
});

describe('deliveryActionBlockedReason', () => {
  const active = { status: 'failed', outboxStatus: 'failed', deadLetter: { active: true, resolution: '' } };

  it('allows every recovery action on an open dead letter', () => {
    expect(deliveryActionBlockedReason(active, 'replay')).toBeNull();
    expect(deliveryActionBlockedReason(active, 'resubmit')).toBeNull();
    expect(deliveryActionBlockedReason(active, 'discard')).toBeNull();
  });

  it('explains why a healthy attempt cannot be recovered', () => {
    const succeeded = { status: 'succeeded', outboxStatus: 'published', deadLetter: null };
    expect(deliveryActionBlockedReason(succeeded, 'replay')).toMatch(/never entered the dead-letter/i);
  });

  it('explains why an already-resolved dead letter cannot be recovered again', () => {
    const resolved = {
      status: 'queued',
      outboxStatus: 'pending',
      deadLetter: { active: false, resolution: 'replayed' }
    };
    expect(deliveryActionBlockedReason(resolved, 'discard')).toMatch(/already resolved/i);
  });

  it('explains the empty selection instead of allowing a dead click', () => {
    expect(deliveryActionBlockedReason(null, 'replay')).toMatch(/select a delivery attempt/i);
  });
});

describe('deploymentActionBlockedReason', () => {
  it('encodes the closed lifecycle state machine', () => {
    expect(deploymentActionBlockedReason('deployed', 'pause')).toBeNull();
    expect(deploymentActionBlockedReason('paused', 'resume')).toBeNull();
    expect(deploymentActionBlockedReason('published', 'deploy')).toBeNull();
    for (const state of ['published', 'deployed', 'paused']) {
      expect(deploymentActionBlockedReason(state, 'retire')).toBeNull();
    }
  });

  it('blocks transitions the server would reject, and says which states allow them', () => {
    expect(deploymentActionBlockedReason('paused', 'pause')).toMatch(/Allowed from: deployed/);
    expect(deploymentActionBlockedReason('deployed', 'resume')).toMatch(/Allowed from: paused/);
    expect(deploymentActionBlockedReason('draft', 'deploy')).toMatch(/Allowed from: published/);
    expect(deploymentActionBlockedReason('retired', 'retire')).toMatch(/cannot retire/i);
  });

  it('explains the empty selection', () => {
    expect(deploymentActionBlockedReason(null, 'pause')).toMatch(/select a deployment/i);
  });
});

describe('formatting helpers', () => {
  it('shortens digests without losing the algorithm-stripped prefix', () => {
    expect(shortDigest('sha256:' + 'a'.repeat(64))).toBe('aaaaaaaaaaaa…');
    expect(shortDigest('short')).toBe('short');
  });

  it('renders timestamps and tolerates absent or unparsable values', () => {
    expect(formatTimestamp('2026-08-08T09:00:00.000Z')).toBe('2026-08-08 09:00:00Z');
    expect(formatTimestamp(null)).toBe('—');
    expect(formatTimestamp(undefined)).toBe('—');
    expect(formatTimestamp('not-a-date')).toBe('not-a-date');
  });
});

describe('describeDestinationDelivery', () => {
  const fhirDelivered = {
    transport: 'fhir',
    outcome: 'delivered',
    endpointAdvisory: 'https://fhir.example.test/r4',
    fhirResourceTypes: ['Patient', 'Encounter'],
    fhirEntryCount: 2,
    fhirOutcomeCodesAdvisory: []
  };

  it('renders a delivered FHIR transaction with its resource types and entry count', () => {
    expect(describeDestinationDelivery(fhirDelivered)).toEqual({
      transportLabel: 'FHIR',
      outcomeLabel: 'Delivered',
      outcomeVariant: 'success',
      resourceTypes: ['Patient', 'Encounter'],
      entryCountText: '2 bundle entries',
      outcomeCodes: [],
      endpointText: 'https://fhir.example.test/r4'
    });
  });

  it('carries OperationOutcome issue codes for a refused transaction', () => {
    const display = describeDestinationDelivery({
      ...fhirDelivered,
      outcome: 'refused',
      fhirEntryCount: 1,
      fhirOutcomeCodesAdvisory: ['invalid', 'not-found']
    });
    expect(display.outcomeVariant).toBe('danger');
    expect(display.outcomeLabel).toBe('Refused');
    expect(display.outcomeCodes).toEqual(['invalid', 'not-found']);
    expect(display.entryCountText).toBe('1 bundle entry');
  });

  it('shows no Bundle facts for an https delivery', () => {
    const display = describeDestinationDelivery({
      transport: 'https',
      outcome: 'retryable',
      endpointAdvisory: 'https://destination.example.test/ingest',
      fhirResourceTypes: [],
      fhirEntryCount: 0,
      fhirOutcomeCodesAdvisory: []
    });
    expect(display.transportLabel).toBe('HTTPS');
    expect(display.outcomeVariant).toBe('warning');
    expect(display.resourceTypes).toEqual([]);
    expect(display.entryCountText).toBeNull();
    expect(display.outcomeCodes).toEqual([]);
  });

  it('never invents an endpoint and never mutates the ledger row', () => {
    const row = { ...fhirDelivered, endpointAdvisory: '' };
    const display = describeDestinationDelivery(row);
    expect(display.endpointText).toBe('No endpoint declared');
    display.resourceTypes.push('Observation');
    expect(row.fhirResourceTypes).toEqual(['Patient', 'Encounter']);
  });

  it('maps every ledger outcome, and tolerates a future one', () => {
    expect(deliveryOutcomeVariant('delivered')).toBe('success');
    expect(deliveryOutcomeVariant('retryable')).toBe('warning');
    expect(deliveryOutcomeVariant('refused')).toBe('danger');
    expect(deliveryOutcomeVariant('unknown-future-outcome')).toBe('default');
    expect(describeDestinationDelivery({ ...fhirDelivered, transport: 'mllp' }).transportLabel).toBe(
      'MLLP'
    );
  });
});
