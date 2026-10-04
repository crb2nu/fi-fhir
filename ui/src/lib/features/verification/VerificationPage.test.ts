/**
 * .loom/42 E-2: Verification over durable admissions — the pre-flight, the
 * Admissions browse with its filters and links, Statistics and Retention.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import VerificationPage from './VerificationPage.svelte';
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

const api = vi.hoisted(() => ({
  fetchAdmissions: vi.fn(),
  fetchAdmissionStatistics: vi.fn(),
  fetchRetentionPosture: vi.fn()
}));

vi.mock('./verificationApi', () => api);

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
      clinicalRead: false,
      integrationSessions: false,
      streaming: false,
      controlPlane: true,
      ...capabilities
    },
    missingRoles: {}
  };
}

function admission(receiptId: string, extra: Record<string, unknown> = {}) {
  return {
    eventId: `event-${receiptId}`,
    eventType: 'patient_admit',
    sourceMessageId: `MSH-${receiptId}`,
    correlationId: `corr-${receiptId}`,
    classification: 'phi',
    recordedAt: '2026-09-29T04:11:00Z',
    receiptId,
    receiptStatus: 'accepted',
    definition: { artifactId: 'adt-east', revisionId: '1', digest: 'sha256:d' },
    source: { artifactId: 'source-adt', revisionId: 's1', digest: 'sha256:s' },
    payloadFields: [
      { path: 'patient.mrn', kind: 'string', repeated: false },
      { path: 'encounter.class', kind: 'string', repeated: false }
    ],
    payloadTruncated: false,
    purgeAfter: null,
    purgedAt: null,
    ...extra
  };
}

const page = (nodes: unknown[], hasNextPage = false, endCursor: string | null = null) => ({
  nodes,
  pageInfo: { hasNextPage, endCursor }
});

function statistics(extra: Record<string, unknown> = {}) {
  return {
    from: '2026-09-28T12:00:00Z',
    to: '2026-09-29T12:00:00Z',
    bucket: 'HOUR',
    acceptedReceipts: 3,
    rejectedReceipts: 0,
    canonicalEvents: 3,
    purgedEvents: 1,
    scheduledForPurge: 1,
    queuedAttempts: 2,
    succeededAttempts: 1,
    failedAttempts: 1,
    eventsByType: [{ key: 'patient_admit', count: 3 }],
    receiptsByDefinition: [{ definitionId: 'adt-east', revisionId: '1', accepted: 3, rejected: 0 }],
    attemptsByDestination: [{ destinationArtifactId: 'fhir-primary', queued: 2, succeeded: 1, failed: 1 }],
    groupsTruncated: false,
    series: [
      { start: '2026-09-29T03:00:00Z', accepted: 0, rejected: 0, queued: 0, succeeded: 0, failed: 0 },
      { start: '2026-09-29T04:00:00Z', accepted: 3, rejected: 0, queued: 2, succeeded: 1, failed: 1 }
    ],
    ...extra
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
  api.fetchAdmissions.mockResolvedValue(page([]));
  api.fetchAdmissionStatistics.mockResolvedValue(statistics());
  api.fetchRetentionPosture.mockResolvedValue({ replicaId: 'replica-a', retentionPurge: false });
});

afterEach(() => {
  resetAccessCapabilities();
  window.history.replaceState(null, '', '/');
});

describe('VerificationPage pre-flight', () => {
  it('says the control plane is not configured before a missing role, and queries nothing', async () => {
    setAccessStatus(status({ operatorRead: false, controlPlane: false }, ['integration:preview']));
    render(VerificationPage);
    const preflight = await screen.findByTestId('verification-preflight');
    expect(preflight).toHaveAttribute('data-reason', 'not-configured');
    expect(preflight).toHaveTextContent('FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true');
    expect(preflight).toHaveAttribute('data-missing-roles', 'integration.operator');
    expect(screen.queryByRole('tab', { name: 'Admissions' })).toBeNull();
    expect(api.fetchAdmissions).not.toHaveBeenCalled();
  });

  it('names the missing read role when the control plane is configured', async () => {
    setAccessStatus(status({ operatorRead: false }, ['graphql:operator']));
    render(VerificationPage);
    const preflight = await screen.findByTestId('verification-preflight');
    expect(preflight).toHaveAttribute('data-reason', 'missing-role');
    expect(preflight).toHaveTextContent('holds graphql:operator but not integration.operator');
    expect(api.fetchAdmissions).not.toHaveBeenCalled();
  });
});

describe('VerificationPage admissions', () => {
  it('changes linked receipts in place, retaining applied filters and unfinished inputs but resetting paging and details', async () => {
    setAccessStatus(status({}));
    api.fetchAdmissions.mockImplementation((filter: { receiptId: string }) => Promise.resolve(page([admission(filter.receiptId)], true, 'next-page')));
    window.history.replaceState(null, '', '/events?receipt=receipt-a');
    render(VerificationPage);
    await screen.findByTestId('admission-row');
    const eventFilter = screen.getByRole('textbox', { name: 'Event type' });
    await fireEvent.input(eventFilter, { target: { value: 'lab_result' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
    await waitFor(() => expect(api.fetchAdmissions).toHaveBeenCalledTimes(2));
    await fireEvent.click(screen.getByRole('button', { name: 'Next' }));
    await waitFor(() => expect(api.fetchAdmissions).toHaveBeenCalledTimes(3));
    await fireEvent.click(screen.getByTestId('admission-row'));
    expect(screen.getByTestId('admission-detail')).toBeInTheDocument();
    await fireEvent.input(eventFilter, { target: { value: 'unfinished-type' } });

    const next = deferred<ReturnType<typeof page>>();
    api.fetchAdmissions.mockReturnValueOnce(next.promise);
    await navigate('/events?receipt=receipt-b');
    expect(screen.queryByTestId('admission-row')).toBeNull();
    expect(screen.queryByTestId('admission-detail')).toBeNull();
    expect(screen.getByRole('textbox', { name: 'Event type' })).toBe(eventFilter);
    expect(eventFilter).toHaveValue('unfinished-type');
    expect(screen.getByRole('textbox', { name: 'Receipt' })).toHaveValue('receipt-b');
    expect(api.fetchAdmissions).toHaveBeenLastCalledWith(expect.objectContaining({ eventType: 'lab_result', receiptId: 'receipt-b' }), { first: 25, after: null });
    next.resolve(page([admission('receipt-b')]));
    expect(await screen.findByTestId('admission-row')).toHaveAttribute('data-receipt-id', 'receipt-b');
    expect(screen.getByText('Page 1')).toBeInTheDocument();
    await navigate('/events?receipt=receipt-b&unrelated=value');
    expect(api.fetchAdmissions).toHaveBeenCalledTimes(4);
  });

  it('ignores an older receipt response and clears receipt scope honestly without applying unfinished filters', async () => {
    setAccessStatus(status({}));
    const older = deferred<ReturnType<typeof page>>();
    api.fetchAdmissions.mockReturnValueOnce(older.promise).mockResolvedValue(page([admission('receipt-b')]));
    window.history.replaceState(null, '', '/events?receipt=receipt-a');
    render(VerificationPage);
    await waitFor(() => expect(api.fetchAdmissions).toHaveBeenCalledTimes(1));
    await fireEvent.input(screen.getByRole('textbox', { name: 'Definition' }), { target: { value: 'unfinished-definition' } });
    await navigate('/events?receipt=receipt-b');
    expect(await screen.findByTestId('admission-row')).toHaveAttribute('data-receipt-id', 'receipt-b');
    older.resolve(page([admission('receipt-a')]));
    await tick();
    expect(screen.getByTestId('admission-row')).toHaveAttribute('data-receipt-id', 'receipt-b');

    api.fetchAdmissions.mockResolvedValue(page([]));
    await navigate('/events?receipt=missing');
    expect(await screen.findByTestId('admissions-empty')).toHaveTextContent('No admissions match these filters.');
    expect(screen.queryByTestId('admission-row')).toBeNull();
    await navigate('/events');
    expect(screen.getByRole('textbox', { name: 'Receipt' })).toHaveValue('');
    expect(screen.getByRole('textbox', { name: 'Definition' })).toHaveValue('unfinished-definition');
    expect(api.fetchAdmissions).toHaveBeenLastCalledWith(expect.objectContaining({ receiptId: null, definitionId: null }), { first: 25, after: null });
    expect(await screen.findByTestId('admissions-empty')).toHaveTextContent('No admissions yet.');
  });

  it.each(['receipt-a', 'receipt-b'])('returns to Admissions when %s arrives from another local view', async (receiptId) => {
    setAccessStatus(status({}));
    window.history.replaceState(null, '', '/events?receipt=receipt-a');
    render(VerificationPage);
    await waitFor(() => expect(api.fetchAdmissions).toHaveBeenCalledTimes(1));
    await fireEvent.click(screen.getByRole('tab', { name: 'Statistics' }));
    await navigate(`/events?receipt=${receiptId}`);
    expect(screen.getByRole('tab', { name: 'Admissions' })).toHaveAttribute('aria-selected', 'true');
    expect(screen.getByRole('textbox', { name: 'Receipt' })).toHaveValue(receiptId);
    await waitFor(() => expect(api.fetchAdmissions).toHaveBeenLastCalledWith(expect.objectContaining({ receiptId }), expect.anything()));
  });

  it('updates the receipt URL on Apply and Clear without resetting its own filter inputs', async () => {
    setAccessStatus(status({}));
    window.history.replaceState(null, '', '/events');
    render(VerificationPage);
    await waitFor(() => expect(api.fetchAdmissions).toHaveBeenCalledTimes(1));
    await fireEvent.input(screen.getByRole('textbox', { name: 'Receipt' }), { target: { value: 'receipt-applied' } });
    await fireEvent.input(screen.getByRole('textbox', { name: 'Event type' }), { target: { value: 'lab_result' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
    expect(window.location.search).toBe('?receipt=receipt-applied');
    expect(navigation.goto).toHaveBeenLastCalledWith(
      new URL('/events?receipt=receipt-applied', window.location.origin),
      { replaceState: true, noScroll: true, keepFocus: true }
    );
    await waitFor(() => expect(api.fetchAdmissions).toHaveBeenCalledTimes(2));
    await navigate('/events?receipt=receipt-applied');
    expect(api.fetchAdmissions).toHaveBeenCalledTimes(2);
    expect(screen.getByRole('textbox', { name: 'Event type' })).toHaveValue('lab_result');
    await fireEvent.click(screen.getByRole('button', { name: 'Clear' }));
    expect(window.location.search).toBe('');
    await waitFor(() => expect(api.fetchAdmissions).toHaveBeenCalledTimes(3));
  });

  it('lists admissions with links to the trace, the definition and the source, and says why there is no live tab or timeline', async () => {
    setAccessStatus(status({}));
    api.fetchAdmissions.mockResolvedValueOnce(page([admission('receipt-1'), admission('receipt-2', { source: null })]));
    render(VerificationPage);

    const rows = await screen.findAllByTestId('admission-row');
    expect(rows).toHaveLength(2);
    expect(within(rows[0]!).getByTestId('admission-receipt-link')).toHaveAttribute('href', '/operator?receipt=receipt-1');
    expect(within(rows[0]!).getByTestId('admission-definition-link')).toHaveAttribute(
      'href',
      '/connections?definition=adt-east&revision=1'
    );
    expect(within(rows[0]!).getByTestId('admission-source-link')).toHaveAttribute(
      'href',
      '/connections?connection=source-adt'
    );
    expect(within(rows[1]!).queryByTestId('admission-source-link')).toBeNull();

    expect(screen.getByTestId('verification-not-streamed')).toHaveTextContent('not streamed');
    expect(screen.getByTestId('verification-no-timeline')).toHaveTextContent('payload values are never read back');
    expect(screen.queryByRole('tab', { name: /Live/ })).toBeNull();
    expect(screen.queryByRole('tab', { name: /Timeline/ })).toBeNull();

    // Selecting a row shows the payload's structure, never a value.
    await fireEvent.click(rows[0]!);
    const fields = await screen.findByTestId('admission-fields');
    expect(fields).toHaveTextContent('patient.mrn');
    expect(fields).toHaveTextContent('string');
  });

  it('opens filtered to the receipt a ?receipt= link names', async () => {
    setAccessStatus(status({}));
    window.history.replaceState(null, '', '/events?receipt=receipt-7');
    render(VerificationPage);
    await waitFor(() => expect(api.fetchAdmissions).toHaveBeenCalled());
    expect(api.fetchAdmissions.mock.calls[0]![0]).toMatchObject({ receiptId: 'receipt-7', includePurged: false });
    expect(screen.getByRole('textbox', { name: 'Receipt' })).toHaveValue('receipt-7');
  });

  it('sends the chosen filters and pages by the server cursor', async () => {
    setAccessStatus(status({}));
    api.fetchAdmissions.mockResolvedValue(page([admission('receipt-1')], true, 'cursor-1'));
    render(VerificationPage);
    await screen.findByTestId('admission-row');

    await fireEvent.input(screen.getByRole('textbox', { name: 'Event type' }), { target: { value: 'lab_result' } });
    await fireEvent.click(screen.getByTestId('admissions-include-purged'));
    await fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
    await waitFor(() => expect(api.fetchAdmissions).toHaveBeenCalledTimes(2));
    expect(api.fetchAdmissions.mock.calls[1]![0]).toMatchObject({ eventType: 'lab_result', includePurged: true });
    expect(api.fetchAdmissions.mock.calls[1]![1]).toEqual({ first: 25, after: null });

    await fireEvent.click(await screen.findByRole('button', { name: 'Next' }));
    await waitFor(() => expect(api.fetchAdmissions).toHaveBeenCalledTimes(3));
    expect(api.fetchAdmissions.mock.calls[2]![1]).toEqual({ first: 25, after: 'cursor-1' });
  });

  it('pages and refreshes with the applied filters, never un-applied input', async () => {
    setAccessStatus(status({}));
    api.fetchAdmissions.mockResolvedValue(page([admission('receipt-1')], true, 'cursor-a'));
    render(VerificationPage);
    await screen.findByTestId('admission-row');

    // Apply filter A.
    await fireEvent.input(screen.getByRole('textbox', { name: 'Event type' }), { target: { value: 'patient_admit' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
    await waitFor(() => expect(api.fetchAdmissions).toHaveBeenCalledTimes(2));
    const applied = api.fetchAdmissions.mock.calls[1]![0];
    expect(applied).toMatchObject({ eventType: 'patient_admit', includePurged: false });

    // Edit the inputs without applying, then page and refresh.
    await fireEvent.input(screen.getByRole('textbox', { name: 'Event type' }), { target: { value: 'lab_result' } });
    await fireEvent.click(screen.getByTestId('admissions-include-purged'));
    await fireEvent.click(await screen.findByRole('button', { name: 'Next' }));
    await waitFor(() => expect(api.fetchAdmissions).toHaveBeenCalledTimes(3));
    expect(api.fetchAdmissions.mock.calls[2]![0]).toEqual(applied);
    expect(api.fetchAdmissions.mock.calls[2]![1]).toEqual({ first: 25, after: 'cursor-a' });

    await fireEvent.click(screen.getByRole('button', { name: 'Refresh admissions' }));
    await waitFor(() => expect(api.fetchAdmissions).toHaveBeenCalledTimes(4));
    expect(api.fetchAdmissions.mock.calls[3]![0]).toEqual(applied);
    expect(api.fetchAdmissions.mock.calls[3]![1]).toEqual({ first: 25, after: 'cursor-a' });
  });

  it('Clear resets every filter, Include tombstoned too', async () => {
    setAccessStatus(status({}));
    api.fetchAdmissions.mockResolvedValue(page([admission('receipt-1')]));
    render(VerificationPage);
    await screen.findByTestId('admission-row');
    await fireEvent.input(screen.getByRole('textbox', { name: 'Receipt' }), { target: { value: 'receipt-1' } });
    await fireEvent.click(screen.getByTestId('admissions-include-purged'));
    await fireEvent.click(screen.getByRole('button', { name: 'Apply' }));
    await waitFor(() => expect(api.fetchAdmissions).toHaveBeenCalledTimes(2));

    await fireEvent.click(await screen.findByRole('button', { name: 'Clear' }));
    await waitFor(() => expect(api.fetchAdmissions).toHaveBeenCalledTimes(3));
    expect(api.fetchAdmissions.mock.calls[2]![0]).toMatchObject({ receiptId: null, includePurged: false });
    expect(screen.getByTestId('admissions-include-purged')).not.toBeChecked();
  });

  it('marks a tombstoned admission', async () => {
    setAccessStatus(status({}));
    api.fetchAdmissions.mockResolvedValueOnce(
      page([admission('receipt-9', { purgeAfter: '2026-09-29T00:00:00Z', purgedAt: '2026-09-29T01:00:00Z' })])
    );
    render(VerificationPage);
    expect(await screen.findByTestId('admission-retention')).toHaveTextContent('tombstoned');
  });
});

describe('VerificationPage statistics and retention', () => {
  it('shows the server counts, the delivered ratio and one bar per bucket', async () => {
    setAccessStatus(status({}));
    render(VerificationPage);
    await fireEvent.click(await screen.findByRole('tab', { name: 'Statistics' }));

    expect(await screen.findByTestId('stat-accepted')).toHaveAttribute('data-value', '3');
    expect(api.fetchAdmissionStatistics.mock.calls[0]![1]).toBe('HOUR');
    expect(screen.getByTestId('stat-delivered')).toHaveTextContent(
      '1 of 4 delivery attempts created in this window succeeded (25%).'
    );
    const buckets = screen.getAllByTestId('series-bucket');
    expect(buckets).toHaveLength(2);
    expect(buckets[1]).toHaveAttribute('data-accepted', '3');
    expect(within(screen.getByTestId('stat-destination-row')).getByRole('link', { name: 'fhir-primary' })).toHaveAttribute(
      'href',
      '/connections?connection=fhir-primary'
    );

    await fireEvent.change(screen.getByRole('combobox', { name: 'Window' }), { target: { value: 'week' } });
    await waitFor(() => expect(api.fetchAdmissionStatistics).toHaveBeenCalledTimes(2));
    expect(api.fetchAdmissionStatistics.mock.calls[1]![1]).toBe('DAY');
  });

  it('says when the retention purge is off and counts the window', async () => {
    setAccessStatus(status({}));
    render(VerificationPage);
    await fireEvent.click(await screen.findByRole('tab', { name: 'Retention' }));

    const posture = await screen.findByTestId('retention-posture');
    await waitFor(() => expect(posture).toHaveAttribute('data-purge', 'false'));
    expect(posture).toHaveTextContent('The retention purge is off on replica replica-a');
    expect(posture).toHaveTextContent('FI_FHIR_RETENTION_POLICY_PATH');
    expect(await screen.findByTestId('retention-purged')).toHaveAttribute('data-value', '1');
    expect(screen.getByTestId('retention-scheduled')).toHaveAttribute('data-value', '1');
    expect(screen.getByTestId('retention-unscheduled')).toHaveAttribute('data-value', '1');
  });

  it('keeps the error inline when a read is refused', async () => {
    setAccessStatus(status({}));
    api.fetchAdmissionStatistics.mockRejectedValue(new Error('operator control-plane action forbidden'));
    render(VerificationPage);
    await fireEvent.click(await screen.findByRole('tab', { name: 'Statistics' }));
    expect(await screen.findByRole('alert')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Retry' })).toBeInTheDocument();
  });
});
