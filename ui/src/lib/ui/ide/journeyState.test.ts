import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { parseAuthStatus, type AccessCapabilityState } from '$lib/graphql/accessCapabilities';
import { GraphQLResponseError } from '$lib/graphql/client';
import {
  defaultJourneyEvidenceApi,
  journeyEvidence,
  loadJourneyEvidence,
  refreshJourneyEvidence,
  resetJourneyEvidence,
  type JourneyEvidenceApi
} from './journeyState';

const summaries = vi.hoisted(() => vi.fn());
vi.mock('$lib/features/dashboard/dashboardApi', () => ({ fetchSessionSummaries: summaries }));

describe('default intake evidence', () => {
  it('asks only for one session with a run instead of reading the inventory and its run arrays', async () => {
    summaries.mockResolvedValueOnce({ nodes: [{ id: 'session-with-run' }], hasMore: true, nextOffset: 1 });
    expect(await defaultJourneyEvidenceApi.hasSessionWithRun()).toBe(true);
    expect(summaries).toHaveBeenLastCalledWith({ hasRuns: true, limit: 1 });
  });

  it('does not claim evidence when the bounded query returns no matching session', async () => {
    summaries.mockResolvedValueOnce({ nodes: [], hasMore: false, nextOffset: null });
    expect(await defaultJourneyEvidenceApi.hasSessionWithRun()).toBe(false);
  });
});

const OPERATOR_BUNDLE = [
  'integration:preview',
  'graphql:operator',
  'clinical:read',
  'integration.operator',
  'integration.delivery.operator',
  'integration.deployment.operator'
];

function access(roles: string[], capabilities: Record<string, unknown>, missingRoles: Record<string, string[]> = {}): AccessCapabilityState {
  return parseAuthStatus({
    authenticated: true,
    authVia: 'network',
    principal: 'tester',
    roles,
    capabilities: {
      operatorRead: false,
      operatorDelivery: false,
      operatorDeployment: false,
      clinicalRead: false,
      integrationSessions: false,
      streaming: false,
      subscriptions: [],
      llm: { configured: false },
      ...capabilities
    },
    missingRoles
  });
}

const bundle = access(OPERATOR_BUNDLE, {
  operatorRead: true,
  integrationSessions: true,
  controlPlane: true,
  connectionCatalog: true
});

/** The hosted demo's identity (e2e preview-only): integration:preview, nothing configured. */
const previewOnly = access(
  ['integration:preview'],
  { controlPlane: false, connectionCatalog: false, connectionsRead: false, connectionsWrite: false },
  { operatorRead: ['integration.operator'], integrationSessions: ['integration.session'] }
);

function api(overrides: Partial<JourneyEvidenceApi> = {}): JourneyEvidenceApi & Record<string, ReturnType<typeof vi.fn>> {
  return {
    hasSessionWithRun: vi.fn(async () => false),
    publishedProfiles: vi.fn(async () => 0),
    mappings: vi.fn(async () => 0),
    approvedAutoroutes: vi.fn(async () => 0),
    publishedWorkflows: vi.fn(async () => 0),
    hasAcceptedReceipt: vi.fn(async () => false),
    ...overrides
  } as JourneyEvidenceApi & Record<string, ReturnType<typeof vi.fn>>;
}

describe('loadJourneyEvidence', () => {
  it('is incomplete everywhere on an empty, fully permitted deployment', async () => {
    const evidence = await loadJourneyEvidence(bundle, api());
    expect(Object.values(evidence).map((stage) => stage.state)).toEqual([
      'incomplete',
      'incomplete',
      'incomplete',
      'incomplete',
      'incomplete'
    ]);
    expect(evidence['source-intake'].reason).toBe('No integration session has a run yet.');
  });

  it('completes each stage from its own evidence', async () => {
    const evidence = await loadJourneyEvidence(
      bundle,
      api({
        hasSessionWithRun: vi.fn(async () => true),
        publishedProfiles: vi.fn(async () => 1),
        approvedAutoroutes: vi.fn(async () => 3),
        publishedWorkflows: vi.fn(async () => 1),
        hasAcceptedReceipt: vi.fn(async () => true)
      })
    );
    expect(evidence['source-intake']).toEqual({ state: 'complete', reason: 'An integration session has a run.' });
    expect(evidence.normalization).toEqual({ state: 'complete', reason: '1 published profile.' });
    expect(evidence.translation).toEqual({ state: 'complete', reason: '3 approved autoroutes.' });
    expect(evidence.delivery).toEqual({ state: 'complete', reason: '1 workflow with a published version.' });
    expect(evidence.verification).toEqual({ state: 'complete', reason: 'At least one message was accepted.' });
  });

  it('counts translation complete from mappings alone, even when the autoroute stats fail', async () => {
    const evidence = await loadJourneyEvidence(
      bundle,
      api({ mappings: vi.fn(async () => 4), approvedAutoroutes: vi.fn(async () => Promise.reject(new Error('boom'))) })
    );
    expect(evidence.translation).toEqual({ state: 'complete', reason: '4 mappings.' });
  });

  it('is unknown, never incomplete, when a read fails and nothing proves the stage', async () => {
    const evidence = await loadJourneyEvidence(
      bundle,
      api({ approvedAutoroutes: vi.fn(async () => Promise.reject(new Error('boom'))) })
    );
    expect(evidence.translation).toEqual({ state: 'unknown', reason: 'Autoroute statistics could not be read.' });
  });

  it('does not read a 503 or a proxy error page as a missing capability', async () => {
    const serviceUnavailable = new Error('HTTP 503: Service Unavailable');
    const disabledText = new GraphQLResponseError('feature disabled: upstream unavailable', [
      { message: 'feature disabled: upstream unavailable' }
    ]);
    const evidence = await loadJourneyEvidence(
      bundle,
      api({
        hasAcceptedReceipt: vi.fn(async () => Promise.reject(serviceUnavailable)),
        publishedWorkflows: vi.fn(async () => Promise.reject(disabledText))
      })
    );
    expect(evidence.verification).toEqual({ state: 'unknown', reason: 'Receipts could not be read.' });
    expect(evidence.delivery).toEqual({ state: 'unknown', reason: 'The workflow catalog could not be read.' });
  });

  it('names a refused read without echoing the server message', async () => {
    const refused = new GraphQLResponseError('GraphQL operation forbidden', [
      { message: 'GraphQL operation forbidden', extensions: { code: 'FORBIDDEN' } }
    ]);
    const evidence = await loadJourneyEvidence(bundle, api({ publishedProfiles: vi.fn(async () => Promise.reject(refused)) }));
    expect(evidence.normalization).toEqual({ state: 'unknown', reason: 'This identity may not read source profiles.' });
  });

  it('pre-flights every stage on the preview-only identity: all unknown, no query issued', async () => {
    const reads = api();
    const evidence = await loadJourneyEvidence(previewOnly, reads);

    expect(Object.values(evidence).every((stage) => stage.state === 'unknown')).toBe(true);
    for (const read of Object.values(reads)) expect(read).not.toHaveBeenCalled();
    expect(evidence['source-intake'].reason).toBe(
      'Reading integration sessions needs integration.session, which this identity does not hold.'
    );
    expect(evidence.normalization.reason).toBe('Reading source profiles needs graphql:operator, which this identity does not hold.');
    // Not configured outranks the missing role (Connections precedence).
    expect(evidence.verification.reason).toBe('The operator control plane is not configured on this deployment.');
  });

  it('names the missing operator role when the control plane is configured', async () => {
    const noOperator = access(
      OPERATOR_BUNDLE.filter((role) => role !== 'integration.operator'),
      { integrationSessions: true, controlPlane: true },
      { operatorRead: ['integration.operator'] }
    );
    const reads = api();
    const evidence = await loadJourneyEvidence(noOperator, reads);
    expect(evidence.verification).toEqual({
      state: 'unknown',
      reason: 'Reading receipts needs integration.operator, which this identity does not hold.'
    });
    expect(reads.hasAcceptedReceipt).not.toHaveBeenCalled();
    expect(reads.hasSessionWithRun).toHaveBeenCalled();
  });

  it('says sessions are off when the capability is false with no missing role', async () => {
    const sessionsOff = access(OPERATOR_BUNDLE, { operatorRead: true, controlPlane: true, integrationSessions: false });
    const evidence = await loadJourneyEvidence(sessionsOff, api());
    expect(evidence['source-intake']).toEqual({
      state: 'unknown',
      reason: 'Integration sessions are not enabled on this deployment.'
    });
  });

  it('tries every read when the capabilities are unknown (an older API, a bearer session)', async () => {
    const reads = api();
    await loadJourneyEvidence({ state: 'unknown' }, reads);
    for (const read of Object.values(reads)) expect(read).toHaveBeenCalledTimes(1);
  });
});

describe('refreshJourneyEvidence', () => {
  beforeEach(() => resetJourneyEvidence());

  it('goes idle → loading → ready', async () => {
    expect(get(journeyEvidence).status).toBe('idle');
    const done = refreshJourneyEvidence({ api: api(), access: bundle });
    expect(get(journeyEvidence).status).toBe('loading');
    await done;
    expect(get(journeyEvidence).status).toBe('ready');
    expect(get(journeyEvidence).evidence?.delivery.state).toBe('incomplete');
  });

  it('coalesces overlapping refreshes into one follow-up read', async () => {
    const reads = api();
    const first = refreshJourneyEvidence({ api: reads, access: bundle });
    const second = refreshJourneyEvidence({ api: reads, access: bundle });
    const third = refreshJourneyEvidence({ api: reads, access: bundle });
    expect(second).toBe(first);
    expect(third).toBe(first);
    await first;
    expect(reads.publishedProfiles).toHaveBeenCalledTimes(2);
    expect(get(journeyEvidence).status).toBe('ready');
  });

  it('keeps the last answer visible while a refresh runs', async () => {
    await refreshJourneyEvidence({ api: api({ hasAcceptedReceipt: vi.fn(async () => true) }), access: bundle });
    const pending = refreshJourneyEvidence({ api: api(), access: bundle });
    expect(get(journeyEvidence).status).toBe('loading');
    expect(get(journeyEvidence).evidence?.verification.state).toBe('complete');
    await pending;
    expect(get(journeyEvidence).evidence?.verification.state).toBe('incomplete');
  });
});
