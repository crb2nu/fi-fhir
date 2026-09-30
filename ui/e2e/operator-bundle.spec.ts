/**
 * The repaired deployment (stack `operator-bundle`): trusted network, the
 * full operator bundle, Integration Session streaming on, no LLM, no loom
 * platform endpoint. Checks 1–5 of `.loom/36` R-D (check 3 as corrected
 * 2026-09-26).
 */
import { readFile } from 'node:fs/promises';
import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import {
  OPERATOR_BUNDLE,
  SYNTHETIC_ADT_A01,
  enterHL7Message,
  fetchAuthStatus,
  graphqlData,
  hl7PreviewButton,
  isStreamRequest,
  openHL7Samples,
  openIDE,
  selects,
  watchPage
} from './support';

test('1. auth status: trusted network, operator plane readable, integrationSessionEvents streamable', async ({
  request
}, testInfo) => {
  const status = await fetchAuthStatus(request, testInfo);

  expect(status.authenticated).toBe(true);
  expect(status.authVia).toBe('network');
  expect(status.roles).toEqual(OPERATOR_BUNDLE);
  expect(status.capabilities).toMatchObject({
    operatorRead: true,
    operatorDelivery: true,
    operatorDeployment: true,
    integrationSessions: true,
    streaming: true,
    llm: { configured: false }
  });
  expect(status.capabilities.subscriptions).toContain('integrationSessionEvents');
  expect(status.missingRoles['operatorRead']).toEqual([]);
});

test("2. operator page lists the fixture's Messages, with no pre-flight and no \"forbidden\" anywhere", async ({
  page
}) => {
  const watch = await watchPage(page);
  const receipts = page.waitForResponse((response) => selects(response.request(), 'operatorReceipts'), {
    timeout: 10_000
  });

  await openIDE(page, '/operator');

  // Whichever the page settles on — the list or the pre-flight — then fail
  // fast, naming the pre-flight, rather than waiting on a query it never sends.
  // The E-0 fixture (e2e/fixture.sh) admitted three messages before this ran.
  const list = page.getByRole('table', { name: 'Durable admission receipts', exact: true });
  const preflight = page.getByTestId('operator-preflight');
  await expect(list.or(preflight)).toBeVisible();
  await expect(preflight, 'the operator pre-flight rendered').toHaveCount(0);

  const response = await receipts;
  expect(response.status()).toBe(200);
  const body = (await response.json()) as { errors?: unknown[] };
  expect(body.errors ?? [], 'operatorReceipts answered without GraphQL errors').toEqual([]);

  await expect(page.getByRole('heading', { name: 'Operator', exact: true })).toBeVisible();
  await expect(page.getByRole('tab', { name: 'Messages' })).toBeVisible();
  expect(await page.getByTestId('receipt-row').count()).toBeGreaterThanOrEqual(4);
  await expect(page.locator('body')).not.toContainText(/forbidden/i);
  expect(watch.errorToasts).toEqual([]);
});

test('3. an Integration Session stream opens; Events → Live Stream shows the honest state', async ({
  page
}) => {
  const watch = await watchPage(page);
  const stream = page.waitForResponse(
    (response) =>
      isStreamRequest(response.request()) && selects(response.request(), 'integrationSessionEvents'),
    { timeout: 10_000 }
  );

  // HL7 intake: with the build flag on AND capabilities.integrationSessions,
  // Preview runs on the session engine, which opens the stream before it runs.
  await openIDE(page, '/hl7');
  await enterHL7Message(page, SYNTHETIC_ADT_A01);
  await hl7PreviewButton(page).click();

  const response = await stream;
  expect(response.status(), 'SSE POST /graphql for integrationSessionEvents').toBe(200);
  expect(response.headers()['content-type'] ?? '').toMatch(/^text\/event-stream/);

  // Listening, then done: runStreamingSessionPreview issues the run only after
  // the stream's onOpen, and reports `complete` only when the stream raised no
  // error — so the panel leaving "connecting" for "complete" within 10 s is the
  // session panel having listened on an open stream for the whole run.
  const progress = page.getByRole('region', { name: 'Server preview progression' });
  await expect(progress).toHaveClass(/state-complete/, { timeout: 10_000 });
  await expect(progress).toContainText('Preview complete');
  await expect(progress.locator('.stream-error')).toHaveCount(0);

  // Events → Live Stream subscribes to eventStream, a root the SSE allowlist
  // refuses by design: the tab must say so, and must not try.
  await openIDE(page, '/events');
  await page.getByRole('tab', { name: 'Live Stream' }).click();
  const unavailable = page.getByTestId('streaming-unavailable');
  await expect(unavailable).toBeVisible();
  await expect(unavailable).toHaveAttribute('data-stream', 'eventStream');
  await expect(unavailable).toHaveAttribute('data-reason', 'not-allowlisted');

  expect(watch.graphql.filter((request) => selects(request, 'eventStream'))).toHaveLength(0);
  expect(watch.errorToasts).toEqual([]);
});

test('4. Copilot shows the backend LLM state (not configured) and never the platform gate', async ({
  page
}) => {
  await openIDE(page, '/');
  await page.getByRole('tab', { name: /Copilot/ }).click();

  const state = page.getByTestId('copilot-llm-state');
  await expect(state).toHaveAttribute('data-state', 'not-configured');
  await expect(state).toContainText('No LLM is configured for this deployment');
  await expect(page.locator('body')).not.toContainText('Platform connection required');
});

test('5. a fresh load shows no phantom Problems badge and no Platform indicator', async ({ page }) => {
  await openIDE(page, '/');

  // Positive anchors first, so the absences below are not read off a page
  // that has not rendered yet.
  const statusBar = page.getByRole('status').filter({ hasText: 'fi-fhir' });
  await expect(statusBar).toContainText('Connected');
  await expect(page.getByRole('tab', { name: /^Problems/ })).toBeVisible();

  await expect(page.getByTestId('problems-badge')).toHaveCount(0);
  await expect(page.getByTestId('platform-indicator')).toHaveCount(0);
});

// Check 9 sits here, before check 7, on purpose: with one worker the file runs
// in order, and check 7 is what first puts a source in this stack's catalog.
// Here the stack mounts no MLLP listener, HTTP ingress or batch runner and the
// catalog is still empty, which is the honest empty state (.loom/38 C-3).
test('9. intake: From connection… on /hl7 reaches the honest empty state, creating no session and querying no capture', async ({
  page,
  request
}, testInfo) => {
  const status = await fetchAuthStatus(request, testInfo);
  expect(status.capabilities).toMatchObject({ integrationSessions: true, connectionCatalog: true, connectionsRead: true });
  const { connections, engineRuntime } = await graphqlData<{
    connections: unknown[];
    engineRuntime: { adapters: Array<{ kind: string; enabled: boolean }> };
  }>(request, '{ connections(direction: SOURCE) { id } engineRuntime { adapters { kind enabled } } }');
  expect(connections, 'check 9 runs before check 7 creates a source connection').toEqual([]);
  expect(engineRuntime.adapters.filter((adapter) => adapter.enabled)).toEqual([]);

  const watch = await watchPage(page);
  await openHL7Samples(page);
  await page.getByTestId('sample-from-connection').click();
  const dialog = page.getByTestId('connection-intake-dialog');
  await expect(dialog).toBeVisible();
  const empty = dialog.getByTestId('connection-intake-empty');
  await expect(empty).toContainText('No source connection is mounted on this deployment.');
  await expect(dialog.getByTestId('connection-intake-sources')).toHaveCount(0);
  await expect(dialog.getByTestId('connection-intake-preflight')).toHaveCount(0);

  // Listing sources is a read: no session, no audit row, no capture poll.
  for (const field of ['createIntegrationSession', 'connectionCaptures', 'startConnectionCapture', 'peekBatchConnection']) {
    expect(watch.graphql.filter((request) => selects(request, field)), `${field} was not issued`).toHaveLength(0);
  }
  // What it did read: this replica's adapters and the catalog's sources.
  expect(watch.graphql.filter((request) => selects(request, 'engineRuntime')).length).toBeGreaterThan(0);
  expect(watch.graphql.filter((request) => selects(request, 'connections')).length).toBeGreaterThan(0);

  await dialog.getByRole('button', { name: 'Close', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(watch.errorToasts).toEqual([]);
});

/**
 * A synthetic MLLP source (.loom/38 C-1): a listen address, timeouts and
 * client CIDRs no real system uses, and no secret binding at all.
 */
const E2E_MLLP = {
  // Unique per run, so a kept or reused stack (UI_E2E_KEEP=1) does not refuse it as a duplicate.
  id: `e2e-mllp-${Date.now().toString(36)}`,
  name: 'E2E MLLP east',
  fields: {
    source_id: 'e2e-adt-east',
    listen_address: '0.0.0.0:22575',
    'timeouts.read_seconds': '5',
    'timeouts.write_seconds': '5',
    'timeouts.idle_seconds': '60',
    'timeouts.process_seconds': '30',
    'clients.allowed_cidrs': '10.20.0.0/16\n192.0.2.0/24',
    max_message_bytes: '1048576',
    max_connections: '16'
  },
  selects: { 'tls.mode': 'disabled', 'acknowledgements.mode': 'application' }
} as const;

/** The control inside the Settings field for a spec path. */
function specField(page: Page, path: string) {
  return page
    .getByTestId('connection-form')
    .locator(`[data-path="${path}"]`)
    .locator('input, select, textarea')
    .first();
}

/** Opens the reason dialog with `action`, records `reason`, confirms, and returns the answer. */
async function confirmWithReason(page: Page, action: string, field: string, reason: string) {
  const answered = page.waitForResponse((response) => selects(response.request(), field), { timeout: 10_000 });
  await page.getByTestId('connection-details').getByRole('button', { name: action, exact: true }).click();
  const dialog = page.getByTestId('connection-reason-dialog');
  await dialog.getByRole('textbox', { name: /Reason/ }).fill(reason);
  await dialog.getByRole('button', { name: action, exact: true }).click();
  const response = await answered;
  expect(response.status()).toBe(200);
  const body = (await response.json()) as { data?: Record<string, unknown>; errors?: unknown[] };
  expect(body.errors ?? [], `${field} answered without GraphQL errors`).toEqual([]);
  await expect(dialog).toHaveCount(0);
  return body.data ?? {};
}

test('7. connections: an MLLP source is created, saved and compiled through the UI; Download is revisionJson byte for byte', async ({
  page,
  request
}, testInfo) => {
  const status = await fetchAuthStatus(request, testInfo);
  expect(status.capabilities).toMatchObject({
    connectionsRead: true,
    connectionsWrite: true,
    controlPlane: true,
    connectionCatalog: true
  });

  const watch = await watchPage(page);
  await openIDE(page, '/connections');
  const sources = page.getByRole('tabpanel', { name: 'Sources' });
  // A fresh stack starts empty; a reused one lists what earlier runs made.
  await expect(
    sources.getByText('No source connections are defined.').or(sources.getByTestId('connections-table'))
  ).toBeVisible();
  await expect(page.getByTestId('connections-preflight')).toHaveCount(0);

  // New ▾ → MLLP, then the kind's generated form.
  await sources.getByTestId('connections-new').click();
  await page.getByRole('dialog', { name: 'New source connection' }).getByRole('button', { name: /^MLLP/ }).click();
  await specField(page, 'id').fill(E2E_MLLP.id);
  await specField(page, 'name').fill(E2E_MLLP.name);
  for (const [path, value] of Object.entries(E2E_MLLP.fields)) await specField(page, path).fill(value);
  for (const [path, value] of Object.entries(E2E_MLLP.selects)) await specField(page, path).selectOption(value);
  // validateConnectionSpec answered for exactly this spec, with nothing to fix.
  await expect(page.getByTestId('connection-form').getByRole('status')).toHaveText('No problems: this spec compiles.');

  await confirmWithReason(page, 'Create', 'createConnection', 'e2e: synthetic MLLP source');
  const row = page.getByTestId('connections-table').locator(`[data-row="${E2E_MLLP.id}"]`);
  await expect(row).toHaveAttribute('data-status', 'Draft');

  // Save an edit: the dialog carries the draft version it started from.
  await specField(page, 'max_connections').fill('32');
  const saved = await confirmWithReason(page, 'Save', 'updateConnection', 'e2e: raise max connections');
  expect(saved['updateConnection']).toMatchObject({ id: E2E_MLLP.id, version: 2 });

  // Compile the saved draft with the MLLP constructor.
  await expect(page.getByTestId('connection-form').getByRole('status')).toHaveText('No problems: this spec compiles.');
  const compiled = await confirmWithReason(page, 'Compile', 'compileConnection', 'e2e: first revision');
  const result = compiled['compileConnection'] as { revision: { digest: string } | null; problems: unknown[] };
  expect(result.problems).toEqual([]);
  expect(result.revision?.digest).toMatch(/^sha256:[0-9a-f]{64}$/);
  await expect(row).toHaveAttribute('data-status', 'Compiled r1');

  // Revisions shows the digest the catalog stored for r1 ...
  const stored = (
    await graphqlData<{ connectionRevision: { digest: string; revisionJson: string } | null }>(
      request,
      'query ($id: ID!) { connectionRevision(artifactId: $id, revisionId: "1") { digest revisionJson } }',
      { id: E2E_MLLP.id }
    )
  ).connectionRevision;
  expect(stored?.digest).toBe(result.revision?.digest);
  const detail = page.getByTestId('connection-revisions').getByTestId('revision-detail');
  await expect(detail).toHaveAttribute('data-digest', stored?.digest ?? 'missing');
  await expect(detail).toContainText(stored?.digest ?? 'missing');

  // ... and Download hands over exactly those bytes, as <id>-r<N>.json.
  const downloading = page.waitForEvent('download');
  await page.getByTestId('connection-revisions').getByRole('button', { name: 'Download' }).click();
  const download = await downloading;
  expect(download.suggestedFilename()).toBe(`${E2E_MLLP.id}-r1.json`);
  const bytes = await readFile(await download.path());
  expect(bytes.equals(Buffer.from(stored?.revisionJson ?? '', 'utf8')), 'download equals revisionJson').toBe(true);
  await testInfo.attach(`${E2E_MLLP.id}-r1.json`, { body: bytes, contentType: 'application/json' });

  await expect(page.locator('body')).not.toContainText(/forbidden/i);
  expect(watch.errorToasts).toEqual([]);
});

test('8. connections: the Engine tab shows the four adapters exactly as this replica reports them', async ({
  page,
  request
}) => {
  const { engineRuntime } = await graphqlData<{
    engineRuntime: { adapters: Array<{ kind: string; enabled: boolean }>; registry: { integrationCount: number } };
  }>(request, '{ engineRuntime { adapters { kind enabled } registry { integrationCount } } }');
  expect(engineRuntime.adapters.map((adapter) => adapter.kind)).toEqual(['http', 'mllp', 'batch', 'delivery']);
  // The stack mounts no HTTP ingress, MLLP listener, batch runner or delivery worker.
  expect(engineRuntime.adapters.filter((adapter) => adapter.enabled)).toEqual([]);

  const watch = await watchPage(page);
  await openIDE(page, '/connections');
  await page.getByRole('tab', { name: 'Engine' }).click();
  const engine = page.getByTestId('engine-runtime');
  const adapters = engine.getByTestId('engine-adapter');
  await expect(adapters).toHaveCount(4);
  for (const [index, adapter] of engineRuntime.adapters.entries()) {
    const panel = adapters.nth(index);
    await expect(panel).toHaveAttribute('data-kind', adapter.kind);
    await expect(panel).toHaveAttribute('data-enabled', String(adapter.enabled));
    await expect(panel).toContainText(adapter.enabled ? 'Enabled' : 'Disabled');
  }
  await expect(engine.getByTestId('engine-registry')).toContainText(String(engineRuntime.registry.integrationCount));
  await expect(engine.getByTestId('engine-properties')).toContainText('FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED');
  expect(watch.errorToasts).toEqual([]);
});

test('10. intake: a capture armed on the compiled MLLP source of check 7 shows its row, polls, and stops polling once cancelled', async ({
  page
}, testInfo) => {
  const watch = await watchPage(page);
  await openHL7Samples(page);
  await page.getByTestId('sample-from-connection').click();
  const dialog = page.getByTestId('connection-intake-dialog');
  const row = dialog
    .getByTestId('connection-intake-sources')
    .locator(`[data-row="stream:${E2E_MLLP.fields.source_id}"]`);
  // Compiled in the catalog, mounted by no replica: capturable, and it says so.
  await expect(row).toContainText('Compiled r1 · not mounted here');
  await row.getByRole('button', { name: 'Capture…' }).click();
  await expect(dialog).toHaveAttribute('data-step', 'capture');
  await expect(dialog).toContainText('messages with segments beyond MSH/EVN/PID/PV1 are currently rejected at admission');

  await dialog.getByRole('spinbutton', { name: /^Messages/ }).fill('2');
  await dialog.getByRole('spinbutton', { name: /^Expires after/ }).fill('120');
  await dialog.getByRole('textbox', { name: /^Reason/ }).fill('e2e: capture the synthetic east feed');
  const started = page.waitForResponse((response) => selects(response.request(), 'startConnectionCapture'), {
    timeout: 10_000
  });
  await dialog.getByRole('button', { name: 'Arm capture' }).click();
  const response = await started;
  const body = (await response.json()) as {
    data?: { startConnectionCapture: { sessionId: string; status: string; sourceId: string } };
    errors?: unknown[];
  };
  expect(body.errors ?? [], 'startConnectionCapture answered without GraphQL errors').toEqual([]);
  expect(body.data?.startConnectionCapture).toMatchObject({ status: 'ARMED', sourceId: E2E_MLLP.fields.source_id });
  await expect(dialog).toHaveCount(0);

  // No Preview had run: intake created the page's session, once.
  expect(watch.graphql.filter((request) => selects(request, 'createIntegrationSession'))).toHaveLength(1);

  const capture = page.getByTestId('connection-capture-row');
  await expect(capture).toHaveAttribute('data-status', 'ARMED');
  await expect(capture).toContainText(/0 \/ 2 captured · expires in [12]:\d\d/);
  // Polling: connectionCaptures of the page's session, every 2–3 s while armed.
  await expect
    .poll(() => watch.graphql.filter((request) => selects(request, 'connectionCaptures')).length, { timeout: 8_000 })
    .toBeGreaterThanOrEqual(2);

  await page.setViewportSize({ width: 1440, height: 900 });
  await testInfo.attach('hl7-capture-armed.png', {
    body: await page.screenshot({ animations: 'disabled', caret: 'hide' }),
    contentType: 'image/png'
  });

  await capture.getByRole('button', { name: 'Cancel', exact: true }).click();
  await expect(dialog).toHaveAttribute('data-step', 'cancel');
  await dialog.getByRole('textbox', { name: /^Reason/ }).fill('e2e: done');
  await dialog.getByRole('button', { name: 'Cancel capture' }).click();
  await expect(dialog).toHaveCount(0);
  await expect(capture).toHaveAttribute('data-status', 'CANCELLED');
  await expect(capture).toContainText('0 / 2 captured · cancelled');

  // Nothing is armed: the page stops asking.
  const polls = watch.graphql.filter((request) => selects(request, 'connectionCaptures')).length;
  await page.waitForTimeout(6_000);
  expect(watch.graphql.filter((request) => selects(request, 'connectionCaptures'))).toHaveLength(polls);
  expect(watch.errorToasts).toEqual([]);
});

// ---------------------------------------------------------------------------
// E-0 operator depth (.loom/42), over the fixture e2e/fixture.sh wrote: the
// deployed definition e2e-batch-adt/v1, E2E-FIXTURE-001 and -004 dead-lettered
// by the worker (six audit rows each), E2E-FIXTURE-002/003 queued. Only E0-4
// writes, and only to -004's dead letter.
// ---------------------------------------------------------------------------

interface FixtureAttempt {
  attemptId: string;
  receiptId: string;
  status: string;
  parentAttemptId: string | null;
}

/** The fixture's dead-lettered attempt for one correlation id (see e2e/fixture.sh). */
async function deadLetterFor(request: APIRequestContext, correlationId: string): Promise<FixtureAttempt> {
  const { operatorReceipts } = await graphqlData<{ operatorReceipts: { nodes: Array<{ receiptId: string }> } }>(
    request,
    `query ($correlationId: String) {
      operatorReceipts(filter: { correlationId: $correlationId }, page: { first: 5 }) { nodes { receiptId } }
    }`,
    { correlationId }
  );
  expect(operatorReceipts.nodes, `one fixture receipt for ${correlationId}`).toHaveLength(1);
  const receiptId = operatorReceipts.nodes[0]!.receiptId;
  const { operatorDeliveryAttempts } = await graphqlData<{ operatorDeliveryAttempts: { nodes: FixtureAttempt[] } }>(
    request,
    `query ($receiptId: ID) {
      operatorDeliveryAttempts(filter: { receiptId: $receiptId, status: "failed" }, page: { first: 5 }) {
        nodes { attemptId receiptId status parentAttemptId }
      }
    }`,
    { receiptId }
  );
  expect(operatorDeliveryAttempts.nodes, `one dead-lettered attempt for ${correlationId}`).toHaveLength(1);
  return operatorDeliveryAttempts.nodes[0]!;
}

async function fixtureAttempts(request: APIRequestContext, status: string): Promise<FixtureAttempt[]> {
  const { operatorDeliveryAttempts } = await graphqlData<{ operatorDeliveryAttempts: { nodes: FixtureAttempt[] } }>(
    request,
    `query ($status: String) {
      operatorDeliveryAttempts(filter: { status: $status }, page: { first: 50 }) {
        nodes { attemptId receiptId status parentAttemptId }
      }
    }`,
    { status }
  );
  return operatorDeliveryAttempts.nodes;
}

test("E0-1. deployments: a deep link opens the seeded revision's lifecycle history, validation and release", async ({
  page
}) => {
  const watch = await watchPage(page);
  await openIDE(page, '/operator?definition=e2e-batch-adt&revision=v1');
  await expect(page.getByRole('tab', { name: 'Deployments' })).toHaveAttribute('aria-selected', 'true');
  const history = page.getByTestId('deployment-history');
  await expect(history).toHaveAttribute('data-definition-id', 'e2e-batch-adt');
  const rows = history.getByTestId('history-row');
  await expect(rows.first()).toBeVisible();
  const actions = await rows.evaluateAll((elements) => elements.map((element) => element.getAttribute('data-action')));
  for (const action of ['create_draft', 'validate_connection', 'approve', 'publish', 'deploy']) {
    expect(actions, `history records ${action}`).toContain(action);
  }
  expect(actions[0], 'newest first').toBe('deploy');
  const row = page.getByTestId('deployment-row').filter({ has: page.getByText('e2e-batch-adt', { exact: true }) });
  await expect(row.getByTestId('validation-badge')).toHaveAttribute('data-validation', 'current');
  await expect(row).toContainText(/release-[0-9a-f]+/);
  expect(watch.graphql.filter((request) => selects(request, 'operatorDeploymentEvents'))).toHaveLength(1);

  // An absent revision is said, not rendered as a blank.
  await openIDE(page, '/operator?definition=e2e-no-such-definition&revision=v1');
  await expect(page.getByTestId('deployment-focus-missing')).toContainText(
    "e2e-no-such-definition@v1 is not in this tenant's lifecycle catalog"
  );
  expect(watch.errorToasts).toEqual([]);
});

test('E0-2. delivery: the attempts list filters by status across receipts and opens a row in the inspector', async ({
  page,
  request
}) => {
  const queued = await fixtureAttempts(request, 'queued');
  expect(queued.length, 'the fixture left at least two queued attempts').toBeGreaterThanOrEqual(2);

  const watch = await watchPage(page);
  await openIDE(page, '/operator');
  await page.getByRole('tab', { name: 'Delivery' }).click();
  const search = page.getByTestId('attempt-search');
  await expect(search.getByTestId('attempt-row').first()).toBeVisible();

  await search.getByRole('combobox', { name: 'Attempt status' }).selectOption('queued');
  const filtered = page.waitForResponse((response) => selects(response.request(), 'operatorDeliveryAttempts'));
  await search.getByRole('button', { name: 'Apply' }).click();
  await filtered;
  const rows = search.getByTestId('attempt-row');
  await expect(rows).toHaveCount(queued.length);
  expect(await rows.evaluateAll((elements) => elements.map((element) => element.getAttribute('data-status')))).toEqual(
    queued.map(() => 'queued')
  );

  const first = queued[0]!;
  await search.locator(`[data-attempt-id="${first.attemptId}"]`).getByRole('button', { name: first.attemptId }).click();
  const inspector = page.getByTestId('attempt-inspector');
  await expect(inspector).toHaveAttribute('data-attempt-id', first.attemptId);
  await expect(inspector).toContainText('outbox pending');
  await expect(inspector).toContainText('No destination delivery recorded');

  // The receipt link leaves for the trace on Messages.
  await inspector.getByRole('button', { name: first.receiptId }).click();
  await expect(page.getByRole('tab', { name: 'Messages' })).toHaveAttribute('aria-selected', 'true');
  await expect(page.locator('.receipt-id')).toHaveText(first.receiptId);
  expect(watch.errorToasts).toEqual([]);
});

test("E0-3. audit: the dead-lettered attempt's audit trail pages five at a time with its detail", async ({
  page,
  request
}) => {
  // Read-only for every check: its six audit rows never change.
  const dead = await deadLetterFor(request, 'e2e-fixture-dead-letter');

  const watch = await watchPage(page);
  await openIDE(page, `/operator?attempt=${encodeURIComponent(dead.attemptId)}`);
  const inspector = page.getByTestId('attempt-inspector');
  await expect(inspector).toHaveAttribute('data-attempt-id', dead.attemptId);
  const trail = inspector.getByTestId('attempt-audit');
  const rows = trail.getByTestId('audit-row');
  await expect(rows).toHaveCount(5);
  await expect(rows.first()).toHaveAttribute('data-event-kind', 'dlq_entered');
  await expect(rows.first().getByTestId('audit-detail')).toContainText('KAFKA_PUBLISH_FAILED');
  await expect(trail.getByTestId('audit-page')).toHaveText('Audit page 1');

  await trail.getByRole('button', { name: 'Next' }).click();
  await expect(trail.getByTestId('audit-page')).toHaveText('Audit page 2');
  await expect(rows).toHaveCount(1);
  await expect(rows.first()).toHaveAttribute('data-event-kind', 'claimed');
  await expect(trail.getByRole('button', { name: 'Next' })).toBeDisabled();
  expect(watch.graphql.filter((request) => selects(request, 'operatorAttemptAudit')).length).toBeGreaterThanOrEqual(2);
  expect(watch.errorToasts).toEqual([]);
});

test('E0-4. resubmit: resubmitting the dead letter from the inspector renders the resubmit chain on the trace', async ({
  page,
  request
}) => {
  // The fixture's second dead letter exists for this check alone.
  const dead = await deadLetterFor(request, 'e2e-fixture-resubmit');

  const watch = await watchPage(page);
  await openIDE(page, `/operator?attempt=${encodeURIComponent(dead.attemptId)}`);
  const inspector = page.getByTestId('attempt-inspector');
  await inspector.getByRole('button', { name: 'Resubmit', exact: true }).click();
  const dialog = page.getByRole('dialog');
  await dialog.getByRole('textbox', { name: /Reason/ }).fill('e2e: resubmit the dead-lettered fixture message');
  const resubmitted = page.waitForResponse((response) => selects(response.request(), 'resubmitMessage'));
  await dialog.getByRole('button', { name: 'Resubmit', exact: true }).click();
  const body = (await (await resubmitted).json()) as {
    data?: { resubmitMessage: { resultAttemptId: string; attempt: { parentAttemptId: string } } };
    errors?: unknown[];
  };
  expect(body.errors ?? []).toEqual([]);
  const child = body.data!.resubmitMessage.resultAttemptId;
  expect(body.data!.resubmitMessage.attempt.parentAttemptId).toBe(dead.attemptId);
  await expect(dialog).toHaveCount(0);

  await openIDE(page, `/operator?receipt=${encodeURIComponent(dead.receiptId)}`);
  const chain = page.getByTestId('resubmit-chain');
  await expect(chain).toBeVisible();
  const items = chain.getByRole('listitem');
  await expect(items).toHaveCount(2);
  await expect(items.nth(0)).toContainText(dead.attemptId);
  await expect(items.nth(0)).toContainText('original');
  await expect(items.nth(1)).toContainText(child);
  await expect(items.nth(1)).toContainText('resubmit 1');
  expect(watch.errorToasts).toEqual([]);
});

test('E0-5. deep links: ?receipt= opens the trace and links out; absent targets render the honest states', async ({
  page,
  request
}) => {
  const [queued] = await fixtureAttempts(request, 'queued');
  const watch = await watchPage(page);

  await openIDE(page, `/operator?receipt=${encodeURIComponent(queued!.receiptId)}`);
  await expect(page.locator('.receipt-id')).toHaveText(queued!.receiptId);
  await expect(page.getByTestId('trace-events-link')).toHaveAttribute(
    'href',
    `/events?receipt=${encodeURIComponent(queued!.receiptId)}`
  );
  await expect(page.getByTestId('trace-source-link')).toHaveAttribute('href', '/connections?connection=source-adt');
  await expect(page.getByTestId('trace-destination-link').first()).toHaveAttribute(
    'href',
    '/connections?connection=fhir-primary'
  );

  await openIDE(page, '/operator?receipt=e2e-no-such-receipt');
  await expect(page.getByText('This receipt is not available in your tenant.')).toBeVisible();

  await openIDE(page, '/operator?attempt=e2e-no-such-attempt');
  await expect(page.getByTestId('attempt-inspector-missing')).toContainText(
    'Delivery attempt e2e-no-such-attempt is not available in your tenant.'
  );

  // Home links each deployment to its history.
  await openIDE(page, '/');
  const link = page.getByTestId('integrations-panel').getByRole('link', { name: 'e2e-batch-adt' });
  await expect(link).toHaveAttribute('href', '/operator?definition=e2e-batch-adt&revision=v1');
  await expect(page.locator('body')).not.toContainText(/forbidden/i);
  expect(watch.errorToasts).toEqual([]);
});

test("E0-6. fleet: the Engine tab and Home report every replica's heartbeat, this one fresh", async ({ page }) => {
  const watch = await watchPage(page);
  await openIDE(page, '/connections');
  await page.getByRole('tab', { name: 'Engine' }).click();
  const fleet = page.getByTestId('engine-fleet');
  const replicas = fleet.getByTestId('fleet-replica');
  await expect(replicas.first()).toBeVisible();
  // The bundle API plus the fixture's two admission processes, which have stopped.
  expect(await replicas.count()).toBeGreaterThanOrEqual(3);
  await expect(replicas.first()).toContainText('(this replica)');
  await expect(replicas.first()).toHaveAttribute('data-stale', 'false');

  await openIDE(page, '/');
  const row = page.getByTestId('health-fleet');
  // Counted as the server counts totalReplicas: fresh replicas only.
  await expect(row).toHaveAttribute('data-fresh', /^[1-9]\d*$/);
  await expect(row).toContainText(/\d+ replicas? with a fresh heartbeat/);
  expect(watch.errorToasts).toEqual([]);
});
