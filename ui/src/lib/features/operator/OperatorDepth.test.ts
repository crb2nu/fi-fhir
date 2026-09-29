/**
 * E-0 operator depth: attempt search, the attempt inspector with paged audit,
 * deployment history and validation, the trace's hidden fields, deep links,
 * and the control-plane pre-flight.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import AttemptInspector from './AttemptInspector.svelte';
import AttemptSearch from './AttemptSearch.svelte';
import DeploymentControls from './DeploymentControls.svelte';
import MessageTrace from './MessageTrace.svelte';
import OperatorPage from './OperatorPage.svelte';
import { VALIDATION_REQUIRED_REASON } from './attemptPresentation';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';

const api = vi.hoisted(() => ({
  fetchReceipts: vi.fn(),
  fetchMessageTrace: vi.fn(),
  fetchAttempt: vi.fn(),
  fetchAttempts: vi.fn(),
  fetchDeadLetters: vi.fn(),
  fetchCircuits: vi.fn(),
  fetchAttemptAudit: vi.fn(),
  fetchDeployments: vi.fn(),
  fetchDeploymentEvents: vi.fn()
}));

vi.mock('./operatorApi', () => ({
  ...api,
  replayDelivery: vi.fn(),
  resubmitMessage: vi.fn(),
  discardDeadLetter: vi.fn(),
  pauseDeployment: vi.fn(),
  resumeDeployment: vi.fn(),
  retireDeployment: vi.fn(),
  deployRelease: vi.fn()
}));

const page = (nodes: unknown[], hasNextPage = false, endCursor: string | null = null) => ({
  nodes,
  pageInfo: { hasNextPage, endCursor }
});

function attempt(attemptId: string, extra: Record<string, unknown> = {}) {
  return {
    tenantId: 'tenant-a',
    attemptId,
    parentAttemptId: null,
    receiptId: 'receipt-a',
    eventId: 'event-a',
    traceId: 'trace-a',
    destination: { artifactId: 'fhir-primary', revisionId: 'destination-1', digest: 'sha256:' + 'd'.repeat(64), class: 'production' },
    route: 'admit',
    action: 'send-fhir',
    status: 'queued',
    attemptCount: 0,
    recordedAt: '2026-09-29T04:00:00Z',
    scheduledAt: '2026-09-29T04:00:00Z',
    completedAt: null,
    lastErrorCode: '',
    lastErrorDetail: '',
    outboxStatus: 'pending',
    topic: 'integration.delivery.v1',
    leaseOwner: '',
    leaseExpiresAt: null,
    deadLetter: null,
    deliveries: [],
    ...extra
  };
}

function audit(auditId: string, eventKind: string, detail: Record<string, unknown> = {}) {
  return {
    auditId,
    attemptId: 'attempt-a',
    eventKind,
    attemptCount: 1,
    reason: '',
    recordedAt: '2026-09-29T04:00:01Z',
    detail,
    principal: { id: '', kind: '', authMethod: '', roles: [] }
  };
}

function status(capabilities: Record<string, unknown>, roles: string[] = ['graphql:operator', 'integration.operator']) {
  return {
    authenticated: true,
    authVia: 'network',
    principal: 'e2e',
    roles,
    capabilities: {
      operatorRead: true,
      operatorDelivery: true,
      operatorDeployment: true,
      clinicalRead: true,
      integrationSessions: false,
      streaming: false,
      ...capabilities
    },
    missingRoles: {}
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  resetAccessCapabilities();
  api.fetchReceipts.mockResolvedValue(page([]));
  api.fetchAttempts.mockResolvedValue(page([]));
  api.fetchDeadLetters.mockResolvedValue(page([]));
  api.fetchCircuits.mockResolvedValue([]);
  api.fetchAttemptAudit.mockResolvedValue(page([]));
  api.fetchDeployments.mockResolvedValue([]);
  api.fetchDeploymentEvents.mockResolvedValue([]);
});

afterEach(() => {
  resetAccessCapabilities();
  window.history.replaceState(null, '', '/');
});

describe('AttemptSearch', () => {
  it('sends the chosen filters and pages by the server cursor', async () => {
    api.fetchAttempts.mockResolvedValueOnce(page([attempt('attempt-a')], true, 'cursor-1'));
    const oninspect = vi.fn();
    const ontrace = vi.fn();
    render(AttemptSearch, { oninspect, ontrace });

    const row = await screen.findByTestId('attempt-row');
    expect(row).toHaveAttribute('data-attempt-id', 'attempt-a');
    await fireEvent.click(within(row).getByRole('button', { name: 'receipt-a' }));
    expect(ontrace).toHaveBeenCalledWith('receipt-a');
    await fireEvent.click(within(row).getByRole('button', { name: 'attempt-a' }));
    expect(oninspect).toHaveBeenCalledWith('attempt-a');

    api.fetchAttempts.mockResolvedValueOnce(page([attempt('attempt-b')]));
    await fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    await waitFor(() => expect(api.fetchAttempts).toHaveBeenLastCalledWith(expect.anything(), { first: 25, after: 'cursor-1' }));

    await fireEvent.change(screen.getByRole('combobox', { name: 'Attempt status' }), { target: { value: 'failed' } });
    await fireEvent.input(screen.getByRole('textbox', { name: 'Destination' }), { target: { value: 'fhir-primary' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
    await waitFor(() =>
      expect(api.fetchAttempts).toHaveBeenLastCalledWith(
        expect.objectContaining({ status: 'failed', destinationArtifactId: 'fhir-primary', receiptId: null }),
        { first: 25, after: null }
      )
    );
  });

  it('refuses an inverted time window without querying', async () => {
    render(AttemptSearch, { oninspect: vi.fn(), ontrace: vi.fn() });
    await waitFor(() => expect(api.fetchAttempts).toHaveBeenCalledTimes(1));
    await fireEvent.input(screen.getByLabelText('Recorded from'), { target: { value: '2026-09-29T05:00' } });
    await fireEvent.input(screen.getByLabelText('Recorded to'), { target: { value: '2026-09-29T04:00' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
    expect(await screen.findByText('From must be earlier than To.')).toBeInTheDocument();
    expect(api.fetchAttempts).toHaveBeenCalledTimes(1);
  });
});

describe('AttemptInspector', () => {
  const props = () => ({ ontrace: vi.fn(), oninspect: vi.fn(), oncontrol: vi.fn(), onclose: vi.fn() });

  it('shows the resubmit parent, lease, and pages the audit trail with its detail', async () => {
    api.fetchAttempt.mockResolvedValue(
      attempt('attempt-child', { parentAttemptId: 'attempt-a', leaseOwner: 'worker-1', leaseExpiresAt: '2000-01-01T00:00:00Z' })
    );
    api.fetchAttemptAudit
      .mockResolvedValueOnce(page([audit('1', 'claimed'), audit('2', 'retry_scheduled', { code: 'KAFKA_PUBLISH_FAILED' })], true, 'a-2'))
      .mockResolvedValueOnce(page([audit('3', 'dlq_entered', { code: 'KAFKA_PUBLISH_FAILED' })]));
    const handlers = props();
    render(AttemptInspector, { attemptId: 'attempt-child', ...handlers });

    const parent = await screen.findByTestId('attempt-parent');
    await fireEvent.click(within(parent).getByRole('button', { name: 'attempt-a' }));
    expect(handlers.oninspect).toHaveBeenCalledWith('attempt-a');
    expect(screen.getByText(/lease by worker-1 expired/)).toBeInTheDocument();

    const trail = screen.getByTestId('attempt-audit');
    expect(await within(trail).findAllByTestId('audit-row')).toHaveLength(2);
    expect(within(trail).getByText('KAFKA_PUBLISH_FAILED')).toBeInTheDocument();
    expect(within(trail).getByTestId('audit-page')).toHaveTextContent('Audit page 1');
    await fireEvent.click(within(trail).getByRole('button', { name: 'Next' }));
    await waitFor(() => expect(within(trail).getByTestId('audit-page')).toHaveTextContent('Audit page 2'));
    expect(api.fetchAttemptAudit).toHaveBeenLastCalledWith('attempt-child', { first: 5, after: 'a-2' });
    expect(within(trail).getAllByTestId('audit-row')).toHaveLength(1);
  });

  it('renders the honest absent state for an attempt outside the tenant', async () => {
    api.fetchAttempt.mockResolvedValue(null);
    render(AttemptInspector, { attemptId: 'attempt-elsewhere', ...props() });
    expect(await screen.findByTestId('attempt-inspector-missing')).toHaveTextContent(
      'Delivery attempt attempt-elsewhere is not available in your tenant.'
    );
  });
});

function deployment(extra: Record<string, unknown> = {}) {
  return {
    definitionRevision: { artifactId: 'e2e-batch-adt', revisionId: 'v1', digest: 'sha256:' + 'a'.repeat(64) },
    state: 'paused',
    version: 6,
    releaseId: 'release-1',
    health: 'unknown',
    validationPassed: true,
    validationExpiresAt: '2026-09-29T04:05:00Z',
    validationCurrent: false,
    updatedReason: 'maintenance',
    updatedAt: '2026-09-29T04:00:00Z',
    updatedBy: { id: 'operator', kind: 'human', authMethod: 'oidc', roles: [] },
    ...extra
  };
}

describe('DeploymentControls validation and history', () => {
  it('shows expired evidence, the release, and blocks Resume with the reason', async () => {
    api.fetchDeployments.mockResolvedValue([deployment()]);
    render(DeploymentControls);
    const badge = await screen.findByTestId('validation-badge');
    expect(badge).toHaveAttribute('data-validation', 'expired');
    expect(screen.getByText('release-1')).toBeInTheDocument();
    const resume = screen.getByRole('button', { name: 'Resume' });
    expect(resume).toBeDisabled();
    expect(resume).toHaveAttribute('title', VALIDATION_REQUIRED_REASON);
  });

  it('opens a deep-linked revision history, and says when the revision is absent', async () => {
    api.fetchDeployments.mockResolvedValue([deployment({ validationCurrent: true })]);
    api.fetchDeploymentEvents.mockResolvedValue([
      { eventId: 'e1', version: 1, action: 'create_draft', fromState: '', toState: 'draft', health: 'unknown', releaseId: null, reason: 'seed', occurredAt: '2026-09-29T03:59:00Z', actor: { id: 'seed', kind: 'human', authMethod: 'postgres', roles: [] } },
      { eventId: 'e5', version: 5, action: 'deploy', fromState: 'published', toState: 'deployed', health: 'starting', releaseId: 'release-1', reason: 'seed', occurredAt: '2026-09-29T04:00:00Z', actor: { id: 'seed', kind: 'human', authMethod: 'postgres', roles: [] } }
    ]);
    const { unmount } = render(DeploymentControls, { focus: { definitionId: 'e2e-batch-adt', revisionId: 'v1' } });
    const history = await screen.findByTestId('deployment-history');
    const rows = await within(history).findAllByTestId('history-row');
    expect(rows.map((row) => row.getAttribute('data-action'))).toEqual(['deploy', 'create_draft']);
    expect(api.fetchDeploymentEvents).toHaveBeenCalledWith('e2e-batch-adt', 'v1');
    unmount();

    render(DeploymentControls, { focus: { definitionId: 'gone', revisionId: 'v9' } });
    expect(await screen.findByTestId('deployment-focus-missing')).toHaveTextContent(
      "Revision gone@v9 is not in this tenant's lifecycle catalog"
    );
  });
});

describe('MessageTrace depth', () => {
  it('renders the resubmit chain, tombstones, and links out', () => {
    const traceValue = {
      receipt: {
        tenantId: 'tenant-a', receiptId: 'receipt-a', status: 'accepted', recordedAt: '2026-09-29T04:00:00Z',
        correlationId: 'c', rawRetentionMode: 'ephemeral',
        integrationRevision: { artifactId: 'integration-adt', revisionId: 'r1', digest: 'sha256:0' },
        principal: { id: 'sender', kind: 'service', authMethod: 'bearer', roles: [] },
        reason: '', eventCount: 1, attemptCount: 2, failedAttemptCount: 1, deadLetterCount: 0
      },
      events: [
        { eventId: 'event-a', receiptId: 'receipt-a', eventType: 'patient_admit', sourceMessageId: 'E2E-1', correlationId: 'c', classification: 'phi', recordedAt: '2026-09-29T04:00:00Z', purgeAfter: '2026-10-29T04:00:00Z', purgedAt: '2026-10-29T05:00:00Z', payloadTruncated: false, payloadFields: [] }
      ],
      lineage: [
        { lineageId: 'l', receiptId: 'receipt-a', eventId: 'event-a', traceId: 't', correlationId: 'c', sourceMessageId: 'E2E-1', recordedAt: '2026-09-29T04:00:00Z',
          artifactRevisions: { source: { artifactId: 'source-adt', revisionId: 's1', digest: 'sha256:1' }, profile: { artifactId: 'p', revisionId: '1', digest: 'sha256:2' }, workflow: { artifactId: 'w', revisionId: '1', digest: 'sha256:3' } },
          routes: [], diagnostics: [] }
      ],
      attempts: [attempt('attempt-a', { status: 'failed' }), attempt('attempt-child', { parentAttemptId: 'attempt-a' })],
      audit: [{ ...audit('9', 'resubmitted', { source_attempt_id: 'attempt-a' }), attemptId: 'attempt-child' }]
    };
    render(MessageTrace, { props: { receiptId: 'receipt-a', trace: traceValue as never } });

    const chain = screen.getByTestId('resubmit-chain');
    expect(within(chain).getAllByRole('listitem').map((item) => item.textContent?.replace(/\s+/g, ' ').trim())).toEqual([
      'attempt-a original',
      'attempt-child resubmit 1'
    ]);
    expect(screen.getByTestId('event-tombstone')).toBeInTheDocument();
    expect(screen.getByTestId('event-purged')).toHaveTextContent('2026-10-29 05:00:00Z');
    expect(screen.getByTestId('trace-events-link')).toHaveAttribute('href', '/events?receipt=receipt-a');
    expect(screen.getByTestId('trace-source-link')).toHaveAttribute('href', '/connections?connection=source-adt');
    expect(screen.getAllByTestId('trace-destination-link')[0]).toHaveAttribute('href', '/connections?connection=fhir-primary');
    expect(screen.getByTestId('audit-detail')).toHaveTextContent('source_attempt_id attempt-a');
  });
});

describe('OperatorPage deep links and pre-flight', () => {
  it('opens the attempt inspector on Delivery for ?attempt=', async () => {
    setAccessStatus(status({ controlPlane: true }));
    api.fetchAttempt.mockResolvedValue(attempt('attempt-z'));
    window.history.replaceState(null, '', '/operator?attempt=attempt-z');
    render(OperatorPage);
    expect(await screen.findByTestId('attempt-inspector')).toHaveAttribute('data-attempt-id', 'attempt-z');
    expect(screen.getByRole('tab', { name: 'Delivery' })).toHaveAttribute('aria-selected', 'true');
  });

  it('opens the trace for ?receipt=', async () => {
    setAccessStatus(status({ controlPlane: true }));
    api.fetchMessageTrace.mockResolvedValue(null);
    window.history.replaceState(null, '', '/operator?receipt=receipt-q');
    render(OperatorPage);
    await waitFor(() => expect(api.fetchMessageTrace).toHaveBeenCalledWith('receipt-q'));
    expect(await screen.findByText('This receipt is not available in your tenant.')).toBeInTheDocument();
  });

  it('says "not configured" before the missing role and queries nothing', async () => {
    setAccessStatus(status({ operatorRead: false, controlPlane: false }, ['integration:preview']));
    window.history.replaceState(null, '', '/operator?receipt=receipt-q');
    render(OperatorPage);
    const preflight = await screen.findByTestId('operator-preflight');
    expect(preflight).toHaveAttribute('data-reason', 'not-configured');
    expect(preflight).toHaveTextContent('FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true');
    expect(preflight).toHaveAttribute('data-missing-roles', 'integration.operator');
    expect(api.fetchReceipts).not.toHaveBeenCalled();
    expect(api.fetchMessageTrace).not.toHaveBeenCalled();
  });
});
