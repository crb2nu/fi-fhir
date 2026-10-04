/**
 * E-0 operator depth: attempt search, the attempt inspector with paged audit,
 * deployment history and validation, the trace's hidden fields, deep links,
 * and the control-plane pre-flight.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import AttemptInspector from './AttemptInspector.svelte';
import AttemptSearch from './AttemptSearch.svelte';
import DeploymentControls from './DeploymentControls.svelte';
import MessageBrowser from './MessageBrowser.svelte';
import MessageTrace from './MessageTrace.svelte';
import OperatorPage from './OperatorPage.svelte';
import { VALIDATION_REQUIRED_REASON } from './attemptPresentation';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';

const navigation = vi.hoisted(() => ({
  callback: null as ((navigation: { to: { url: URL } }) => void) | null,
  goto: vi.fn()
}));
vi.mock('$app/navigation', () => ({
  afterNavigate: (callback: typeof navigation.callback) => { navigation.callback = callback; },
  goto: navigation.goto
}));

async function navigate(path: string): Promise<void> {
  history.pushState(null, '', path);
  navigation.callback?.({ to: { url: new URL(window.location.href) } });
  await tick();
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => { resolve = done; });
  return { promise, resolve };
}

function messageTrace(receiptId: string) {
  return {
    receipt: {
      tenantId: 'tenant-a', receiptId, status: 'accepted', recordedAt: '2026-09-29T04:00:00Z',
      correlationId: `correlation-${receiptId}`, rawRetentionMode: 'ephemeral',
      integrationRevision: { artifactId: 'integration-adt', revisionId: 'r1', digest: 'sha256:0' },
      principal: { id: 'sender', kind: 'service', authMethod: 'bearer', roles: [] },
      reason: '', eventCount: 0, attemptCount: 0, failedAttemptCount: 0, deadLetterCount: 0
    },
    events: [], lineage: [], attempts: [], audit: []
  };
}

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
  navigation.callback = null;
  navigation.goto.mockImplementation(async (url: URL) => {
    await Promise.resolve();
    history.replaceState(null, '', url);
    navigation.callback?.({ to: { url: new URL(window.location.href) } });
  });
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

  it('reads with the applied filters only: a half-typed filter never reaches refresh or paging', async () => {
    api.fetchAttempts.mockResolvedValue(page([attempt('attempt-a')], true, 'cursor-1'));
    render(AttemptSearch, { oninspect: vi.fn(), ontrace: vi.fn() });
    await screen.findByTestId('attempt-row');

    await fireEvent.change(screen.getByRole('combobox', { name: 'Attempt status' }), { target: { value: 'failed' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
    await waitFor(() => expect(api.fetchAttempts).toHaveBeenLastCalledWith(expect.objectContaining({ status: 'failed' }), expect.anything()));

    // Typed, never applied.
    await fireEvent.input(screen.getByRole('textbox', { name: 'Destination' }), { target: { value: 'half-typ' } });
    await fireEvent.change(screen.getByRole('combobox', { name: 'Attempt status' }), { target: { value: 'queued' } });

    const calls = api.fetchAttempts.mock.calls.length;
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh delivery attempts' }));
    await waitFor(() => expect(api.fetchAttempts.mock.calls.length).toBe(calls + 1));
    expect(api.fetchAttempts).toHaveBeenLastCalledWith(
      expect.objectContaining({ status: 'failed', destinationArtifactId: null }),
      { first: 25, after: null }
    );

    await fireEvent.click(await screen.findByRole('button', { name: 'Next' }));
    await waitFor(() =>
      expect(api.fetchAttempts).toHaveBeenLastCalledWith(
        expect.objectContaining({ status: 'failed', destinationArtifactId: null }),
        { first: 25, after: 'cursor-1' }
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

describe('MessageBrowser', () => {
  it('auto-refresh and refresh re-read the applied filters, not the live inputs', async () => {
    api.fetchReceipts.mockResolvedValue(page([]));
    render(MessageBrowser);
    await waitFor(() => expect(api.fetchReceipts).toHaveBeenCalledTimes(1));

    await fireEvent.input(screen.getByRole('textbox', { name: 'Correlation ID' }), { target: { value: 'corr-applied' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
    await waitFor(() => expect(api.fetchReceipts).toHaveBeenCalledTimes(2));

    await fireEvent.input(screen.getByRole('textbox', { name: 'Correlation ID' }), { target: { value: 'corr-half' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh messages' }));
    await waitFor(() => expect(api.fetchReceipts).toHaveBeenCalledTimes(3));
    expect(api.fetchReceipts).toHaveBeenLastCalledWith(
      expect.objectContaining({ correlationId: 'corr-applied' }),
      { first: 25, after: null }
    );
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
  it('replaces a same-route receipt without remounting filters, and ignores an older completion', async () => {
    setAccessStatus(status({ controlPlane: true }));
    const older = deferred<ReturnType<typeof messageTrace>>();
    api.fetchMessageTrace.mockImplementation((id: string) => id === 'receipt-b' ? older.promise : Promise.resolve(messageTrace(id)));
    window.history.replaceState(null, '', '/operator?receipt=receipt-a');
    render(OperatorPage);
    expect(await screen.findByTestId('trace-events-link')).toHaveAttribute('href', '/events?receipt=receipt-a');
    const filter = screen.getByRole('textbox', { name: 'Correlation ID' });
    await fireEvent.input(filter, { target: { value: 'unfinished-filter' } });

    await navigate('/operator?receipt=receipt-b');
    expect(screen.queryByTestId('trace-events-link')).toBeNull();
    expect(screen.getByRole('textbox', { name: 'Correlation ID' })).toBe(filter);
    expect(filter).toHaveValue('unfinished-filter');
    await navigate('/operator?receipt=receipt-c');
    expect(await screen.findByTestId('trace-events-link')).toHaveAttribute('href', '/events?receipt=receipt-c');
    older.resolve(messageTrace('receipt-b'));
    await tick();
    expect(screen.getByTestId('trace-events-link')).toHaveAttribute('href', '/events?receipt=receipt-c');
    await navigate('/operator?receipt=receipt-c&unrelated=value');
    expect(api.fetchMessageTrace).toHaveBeenCalledTimes(3);
  });

  it('clears a pending receipt on base navigation and does not restore its late result', async () => {
    setAccessStatus(status({ controlPlane: true }));
    const pending = deferred<ReturnType<typeof messageTrace>>();
    api.fetchMessageTrace.mockReturnValue(pending.promise);
    window.history.replaceState(null, '', '/operator?receipt=receipt-a');
    render(OperatorPage);
    await waitFor(() => expect(api.fetchMessageTrace).toHaveBeenCalledWith('receipt-a'));
    await navigate('/operator');
    expect(screen.getByText('No message selected.')).toBeInTheDocument();
    pending.resolve(messageTrace('receipt-a'));
    await tick();
    expect(screen.queryByTestId('trace-events-link')).toBeNull();
    expect(screen.getByText('No message selected.')).toBeInTheDocument();
  });

  it('shows an unavailable receipt honestly after another receipt was open', async () => {
    setAccessStatus(status({ controlPlane: true }));
    api.fetchMessageTrace.mockResolvedValueOnce(messageTrace('receipt-a')).mockResolvedValueOnce(null);
    window.history.replaceState(null, '', '/operator?receipt=receipt-a');
    render(OperatorPage);
    await screen.findByTestId('trace-events-link');
    await navigate('/operator?receipt=missing');
    expect(await screen.findByText('This receipt is not available in your tenant.')).toBeInTheDocument();
    expect(screen.queryByTestId('trace-events-link')).toBeNull();
  });

  it('switches attempt and audit targets in place, clears old details, and ignores stale reads', async () => {
    setAccessStatus(status({ controlPlane: true }));
    const oldAttempt = deferred<ReturnType<typeof attempt>>();
    const oldAudit = deferred<ReturnType<typeof page>>();
    api.fetchAttempt.mockImplementation((id: string) => id === 'attempt-b' ? oldAttempt.promise : Promise.resolve(attempt(id, { receiptId: `receipt-${id}` })));
    api.fetchAttemptAudit.mockImplementation((id: string) => id === 'attempt-b' ? oldAudit.promise : Promise.resolve(page([audit(id, `audit-${id}`)])));
    window.history.replaceState(null, '', '/operator?attempt=attempt-a');
    render(OperatorPage);
    await screen.findByRole('button', { name: 'receipt-attempt-a' });
    const filter = screen.getByRole('textbox', { name: 'Destination' });
    await fireEvent.input(filter, { target: { value: 'unfinished-destination' } });
    await navigate('/operator?attempt=attempt-b');
    expect(screen.queryByRole('button', { name: 'receipt-attempt-a' })).toBeNull();
    expect(screen.getByRole('textbox', { name: 'Destination' })).toBe(filter);
    expect(filter).toHaveValue('unfinished-destination');
    await navigate('/operator?attempt=attempt-c');
    await screen.findByRole('button', { name: 'receipt-attempt-c' });
    oldAttempt.resolve(attempt('attempt-b', { receiptId: 'stale-receipt' }));
    oldAudit.resolve(page([audit('stale-audit', 'stale-audit')]));
    await tick();
    expect(screen.queryByRole('button', { name: 'stale-receipt' })).toBeNull();
    expect(screen.queryByText('stale-audit')).toBeNull();
    await fireEvent.click(screen.getByRole('button', { name: 'Close attempt' }));
    expect(window.location.search).toBe('');
    expect(screen.getByRole('tab', { name: 'Delivery' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.queryByTestId('attempt-inspector')).toBeNull();
  });

  it('switches deployment history for same-route links and removes old history for missing or cleared selectors', async () => {
    setAccessStatus(status({ controlPlane: true }));
    api.fetchDeployments.mockResolvedValue([
      deployment(),
      deployment({ definitionRevision: { artifactId: 'second', revisionId: 'v2', digest: 'sha256:2' } })
    ]);
    window.history.replaceState(null, '', '/operator?definition=e2e-batch-adt&revision=v1');
    render(OperatorPage);
    expect(await screen.findByTestId('deployment-history')).toHaveAttribute('data-definition-id', 'e2e-batch-adt');
    await navigate('/operator?definition=second&revision=v2');
    await waitFor(() => expect(screen.getAllByTestId('deployment-history')).toHaveLength(1));
    expect(screen.getByTestId('deployment-history')).toHaveAttribute('data-definition-id', 'second');
    expect(api.fetchDeployments).toHaveBeenCalledTimes(1);
    await navigate('/operator?definition=missing&revision=v9');
    expect(await screen.findByTestId('deployment-focus-missing')).toHaveTextContent('missing@v9');
    expect(screen.queryByTestId('deployment-history')).toBeNull();
    await navigate('/operator');
    expect(screen.getByRole('tab', { name: 'Messages' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByText('No message selected.')).toBeInTheDocument();
  });

  it('keeps the URL aligned with internal receipt and deployment selections', async () => {
    setAccessStatus(status({ controlPlane: true }));
    api.fetchAttempt.mockResolvedValue(attempt('attempt-a'));
    api.fetchMessageTrace.mockResolvedValue(messageTrace('receipt-a'));
    api.fetchDeployments.mockResolvedValue([deployment()]);
    window.history.replaceState(null, '', '/operator?attempt=attempt-a');
    render(OperatorPage);
    await fireEvent.click(await screen.findByRole('button', { name: 'receipt-a' }));
    expect(window.location.search).toBe('?receipt=receipt-a');
    expect(navigation.goto).toHaveBeenLastCalledWith(
      new URL('/operator?receipt=receipt-a', window.location.origin),
      { replaceState: true, noScroll: true, keepFocus: true }
    );
    await screen.findByTestId('trace-events-link');
    await navigate('/operator?receipt=receipt-a');
    expect(api.fetchMessageTrace).toHaveBeenCalledTimes(1);
    await fireEvent.click(screen.getByRole('tab', { name: 'Deployments' }));
    expect(window.location.search).toBe('');
    await fireEvent.click(await screen.findByRole('button', { name: 'Show lifecycle history' }));
    expect(window.location.search).toBe('?definition=e2e-batch-adt&revision=v1');
    await fireEvent.click(screen.getByRole('button', { name: 'Hide lifecycle history' }));
    expect(window.location.search).toBe('');
    expect(screen.queryByTestId('deployment-history')).toBeNull();
  });

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
