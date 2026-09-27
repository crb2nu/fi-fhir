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

/**
 * A synthetic MLLP source (.loom/38 C-1): a listen address, timeouts and
 * client CIDRs no real system uses, and no secret binding at all.
 */
const E2E_MLLP = {
  id: 'e2e-mllp-east',
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
  await expect(sources.getByText('No source connections are defined.')).toBeVisible();
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
