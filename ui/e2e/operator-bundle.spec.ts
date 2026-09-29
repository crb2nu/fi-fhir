/**
 * The repaired deployment (stack `operator-bundle`): trusted network, the
 * full operator bundle, Integration Session streaming on, no LLM, no loom
 * platform endpoint. Checks 1–5 of `.loom/36` R-D (check 3 as corrected
 * 2026-09-26).
 */
import { readFile } from 'node:fs/promises';
import { expect, test, type Page } from '@playwright/test';
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

test('2. operator page lists the (empty) Messages, with no pre-flight and no "forbidden" anywhere', async ({
  page
}) => {
  const watch = await watchPage(page);
  const receipts = page.waitForResponse((response) => selects(response.request(), 'operatorReceipts'), {
    timeout: 10_000
  });

  await openIDE(page, '/operator');

  // Whichever the page settles on — the list or the pre-flight — then fail
  // fast, naming the pre-flight, rather than waiting on a query it never sends.
  const emptyList = page.getByText('No messages match these filters');
  const preflight = page.getByTestId('operator-preflight');
  await expect(emptyList.or(preflight)).toBeVisible();
  await expect(preflight, 'the operator pre-flight rendered').toHaveCount(0);

  const response = await receipts;
  expect(response.status()).toBe(200);
  const body = (await response.json()) as { errors?: unknown[] };
  expect(body.errors ?? [], 'operatorReceipts answered without GraphQL errors').toEqual([]);

  await expect(page.getByRole('heading', { name: 'Operator', exact: true })).toBeVisible();
  await expect(page.getByRole('tab', { name: 'Messages' })).toBeVisible();
  await expect(emptyList).toBeVisible();
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

// --- .loom/42 E-1: definition authoring ---------------------------------------

/** The destination the golden registry's adt-east workflow delivers to. */
const E1_DESTINATION = {
  id: 'fhir-primary',
  spec: {
    destination_id: 'fhir-primary',
    class: 'sandbox',
    fhir: { base_url: 'https://fhir.example.test/r4', token_binding: 'fhir-token', interaction: 'transaction' }
  },
  // A reference, never a value: the key names where the token would live.
  secretBindings: [{ name: 'fhir-token', provider: 'env', key: 'FI_FHIR_CONNECTION_SECRET_E2E_FHIR_TOKEN' }]
} as const;

type E1Connection = { id: string; kind: string; version: number; latestRevision: { revisionId: string } | null };

/**
 * Setup through the API, not the surface under test: a compiled fhir-primary
 * destination and a compiled MLLP source (check 7's, or one of its own when
 * run alone). Both are reused on a kept stack.
 */
async function e1CompiledConnections(request: import('@playwright/test').APIRequestContext): Promise<string> {
  const fields = '{ id kind version latestRevision { revisionId } }';
  const compile = async (connection: E1Connection) => {
    if (connection.latestRevision) return;
    await graphqlData(request, `mutation Op($input: ConnectionCommandInput!) { compileConnection(input: $input) { revision { revisionId } } }`, {
      input: { id: connection.id, expectedVersion: connection.version, reason: 'E1 setup: compile' }
    });
  };
  const { connection: existing } = await graphqlData<{ connection: E1Connection | null }>(
    request,
    `query Op { connection(id: "${E1_DESTINATION.id}") ${fields} }`
  );
  const destination =
    existing ??
    (
      await graphqlData<{ createConnection: E1Connection }>(
        request,
        `mutation Op($input: CreateConnectionInput!) { createConnection(input: $input) ${fields} }`,
        {
          input: {
            id: E1_DESTINATION.id, direction: 'DESTINATION', kind: 'FHIR', name: 'E1 hospital FHIR (sandbox)',
            spec: E1_DESTINATION.spec, secretBindings: E1_DESTINATION.secretBindings, reason: 'E1 setup: declare the destination'
          }
        }
      )
    ).createConnection;
  await compile(destination);

  const { connections } = await graphqlData<{ connections: E1Connection[] }>(
    request,
    `query Op { connections(direction: SOURCE) ${fields} }`
  );
  let source = connections.find((candidate) => candidate.kind === 'MLLP' && candidate.latestRevision) ?? null;
  if (!source) {
    const spec = {
      source_id: 'e1-adt-east', listen_address: '0.0.0.0:22576', encoding: 'utf-8',
      framing: { start_byte: 11, end_byte: 28, trailer_byte: 13 },
      timeouts: { read_seconds: 5, write_seconds: 5, idle_seconds: 60, process_seconds: 30 },
      tls: { mode: 'disabled' }, clients: { allowed_cidrs: ['192.0.2.0/24'] },
      acknowledgements: { mode: 'application', include_error_segment: false },
      max_message_bytes: 1048576, max_connections: 16
    };
    source = (
      await graphqlData<{ createConnection: E1Connection }>(
        request,
        `mutation Op($input: CreateConnectionInput!) { createConnection(input: $input) ${fields} }`,
        {
          input: {
            id: `e1-mllp-${Date.now().toString(36)}`, direction: 'SOURCE', kind: 'MLLP', name: 'E1 MLLP east',
            spec, reason: 'E1 setup: declare the source'
          }
        }
      )
    ).createConnection;
    await compile(source);
  }
  return source.id;
}

/** Confirms the reason dialog the Definitions tab opened. */
async function e1Confirm(page: Page, confirm: string, field: string, reason: string) {
  const answered = page.waitForResponse((response) => selects(response.request(), field), { timeout: 15_000 });
  const dialog = page.getByTestId('connection-reason-dialog');
  await dialog.getByRole('textbox', { name: /Reason/ }).fill(reason);
  await dialog.getByRole('button', { name: confirm, exact: true }).click();
  const response = await answered;
  const body = (await response.json()) as { errors?: unknown[] };
  expect(body.errors ?? [], `${field} answered without GraphQL errors`).toEqual([]);
  await expect(dialog).toHaveCount(0);
}

test('E1-1. definitions: an MLLP source becomes a published definition through the Definitions tab, and Operator offers Deploy', async ({
  page,
  request
}, testInfo) => {
  const status = await fetchAuthStatus(request, testInfo);
  expect(status.capabilities).toMatchObject({ definitionAuthoring: true });
  const sourceId = await e1CompiledConnections(request);
  const definitionId = `e1-adt-east-${Date.now().toString(36)}`;
  const watch = await watchPage(page);

  await openIDE(page, '/connections');
  await page.getByTestId('connections-tab-definitions').click();
  await expect(page.getByTestId('definitions-preflight')).toHaveCount(0);
  await page.getByTestId('definitions-new').click();
  const form = page.getByTestId('definition-new');
  await form.getByTestId('definition-new-id').fill(definitionId);
  await form.getByTestId('definition-new-source').selectOption(sourceId);
  await form.getByTestId('definition-new-artifacts').selectOption('adt-east');
  await form.locator(`[data-destination="${E1_DESTINATION.id}"]`).check();
  // The destination's binding is derived as its own reference.
  await expect(form.getByLabel('fhir-token key')).toHaveValue(E1_DESTINATION.secretBindings[0].key);
  await expect(form.getByTestId('definition-create')).toBeDisabled();

  // Check: every pre-flight the seed runs, and no write.
  await form.getByTestId('definition-check').click();
  await expect(form.getByTestId('definition-problems')).toHaveAttribute('data-blocking', 'false');
  await form.getByTestId('definition-create').click();
  await e1Confirm(page, 'Create draft', 'createIntegrationDefinitionDraft', 'E1: author the east MLLP definition');

  const details = page.getByTestId('definition-details');
  await expect(details).toHaveAttribute('data-state', 'draft');

  // STATIC contacts nothing and says so: this stack mounts no MLLP listener
  // (check 9), so the evidence is recorded as failed, SOURCE_NOT_MOUNTED.
  await details.getByTestId('definition-mode-static').check();
  await details.getByTestId('definition-validate').click();
  await e1Confirm(page, 'Record validation', 'validateIntegrationDefinition', 'E1: static check of the east source');
  await expect(details.getByTestId('definition-validation')).toContainText('VALIDATION_STATIC');
  await expect(details.getByTestId('definition-validation')).toContainText('SOURCE_NOT_MOUNTED');
  await expect(details).toHaveAttribute('data-state', 'draft');
  // REAL is not offered: this replica mounts no batch source.
  await expect(details.getByTestId('definition-mode-real')).toBeDisabled();

  await details.getByTestId('definition-mode-skip').check();
  await details.getByTestId('definition-validate').click();
  await e1Confirm(page, 'Record validation', 'validateIntegrationDefinition', 'E1: smoke stack has no listener, skipping the check');
  await expect(details).toHaveAttribute('data-state', 'validated');
  await expect(details.getByTestId('definition-validation')).toContainText('VALIDATION_SKIPPED');

  await details.getByTestId('definition-approve').click();
  await e1Confirm(page, 'Approve', 'approveIntegrationDefinition', 'E1: approve the east definition');
  await expect(details).toHaveAttribute('data-state', 'approved');
  await details.getByTestId('definition-publish').click();
  await e1Confirm(page, 'Publish release', 'publishIntegrationDefinition', 'E1: publish the east definition');
  await expect(details).toHaveAttribute('data-state', 'published');
  await details.getByTestId('definition-tab-release').click();
  await expect(details.getByTestId('definition-release')).toContainText('sha256:');
  await details.getByTestId('definition-tab-history').click();
  await expect(details.getByTestId('definition-history')).toContainText('publish');

  const deployLink = details.getByTestId('definition-deploy-link');
  await expect(deployLink).toHaveAttribute('href', `/operator?definition=${definitionId}&revision=v1`);
  await deployLink.click();
  await expect(page).toHaveURL(/\/operator\?definition=/);
  await page.getByRole('tab', { name: 'Deployments' }).click();
  const row = page.getByRole('row').filter({ hasText: definitionId });
  await expect(row).toHaveCount(1);
  await expect(row.getByRole('button', { name: 'Deploy', exact: true })).toBeEnabled();
  expect(watch.errorToasts).toEqual([]);

  // The deep link opens the same definition on a fresh load.
  await openIDE(page, `/connections?definition=${definitionId}&revision=v1`);
  await expect(page.getByTestId('definition-details')).toHaveAttribute('data-state', 'published');
});
