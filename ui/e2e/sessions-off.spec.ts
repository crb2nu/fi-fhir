/**
 * Negative control 6b (stack `sessions-off`): the same UI build — session
 * engine flag on — against an API with FI_FHIR_INTEGRATION_SESSION_ENABLED
 * unset. This is production's shape between R-C's Dockerfile flip and the
 * gitops env flip: the session surfaces must be honest, and HL7 intake must
 * keep previewing on the stateless path (`.loom/36` correction 1).
 */
import { expect, test } from '@playwright/test';
import {
  OPERATOR_BUNDLE,
  SYNTHETIC_ADT_A01,
  enterHL7Message,
  fetchAuthStatus,
  hl7PreviewButton,
  isStreamRequest,
  openHL7Samples,
  openIDE,
  selects,
  watchPage
} from './support';

test('6b. with sessions off: no subscriptions, streaming false, the session surface is honest and preview still works', async ({
  page,
  request
}, testInfo) => {
  const status = await fetchAuthStatus(request, testInfo);
  expect(status.authVia).toBe('network');
  expect(status.roles).toEqual(OPERATOR_BUNDLE);
  expect(status.capabilities.streaming).toBe(false);
  expect(status.capabilities.integrationSessions).toBe(false);
  expect(status.capabilities.subscriptions).toEqual([]);

  const watch = await watchPage(page);
  await openIDE(page, '/hl7');

  const notice = page.locator('[data-testid="streaming-unavailable"][data-stream="integrationSessionEvents"]');
  await expect(notice).toBeVisible();
  await expect(notice).toHaveAttribute('data-reason', 'streaming-off');

  await enterHL7Message(page, SYNTHETIC_ADT_A01);
  const preview = page.waitForResponse(
    (response) => selects(response.request(), 'previewIntegrationMessage'),
    { timeout: 10_000 }
  );
  await hl7PreviewButton(page).click();
  const response = await preview;
  expect(response.status()).toBe(200);
  const body = (await response.json()) as { errors?: unknown[] };
  expect(body.errors ?? [], 'stateless preview answered without GraphQL errors').toEqual([]);

  expect(watch.graphql.filter(isStreamRequest), 'no stream was attempted').toHaveLength(0);
  expect(
    watch.graphql.filter((request) => selects(request, 'createIntegrationSession')),
    'no session was created'
  ).toHaveLength(0);
  expect(watch.errorToasts).toEqual([]);
});

test('6d. with sessions off, HL7 Samples has no "From connection…": intake needs a session to capture into', async ({
  page,
  request
}, testInfo) => {
  const status = await fetchAuthStatus(request, testInfo);
  expect(status.capabilities.integrationSessions).toBe(false);
  // The catalog is on here; only the session workspace is off.
  expect(status.capabilities.connectionCatalog).toBe(true);

  const watch = await watchPage(page);
  await openHL7Samples(page);
  await expect(page.getByTestId('sample-from-connection')).toHaveCount(0);
  await expect(page.getByTestId('connection-capture-row')).toHaveCount(0);
  for (const field of ['engineRuntime', 'connections', 'connectionCaptures']) {
    expect(watch.graphql.filter((request) => selects(request, field)), `${field} was not issued`).toHaveLength(0);
  }
  expect(watch.errorToasts).toEqual([]);
});
