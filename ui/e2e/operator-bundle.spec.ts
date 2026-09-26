/**
 * The repaired deployment (stack `operator-bundle`): trusted network, the
 * full operator bundle, Integration Session streaming on, no LLM, no loom
 * platform endpoint. Checks 1–5 of `.loom/36` R-D (check 3 as corrected
 * 2026-09-26).
 */
import { expect, test } from '@playwright/test';
import {
  OPERATOR_BUNDLE,
  SYNTHETIC_ADT_A01,
  enterHL7Message,
  fetchAuthStatus,
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
