/** Older work is searchable without loading every session and run. */
import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { SYNTHETIC_ADT_A01, enterHL7Message, graphqlData, hl7PreviewButton, openIDE, selects, watchPage } from './support';

test.describe.configure({ mode: 'serial' });
const prefix = `Session browser ${Date.now().toString(36)}`;
let olderSessionId = '';
let olderRunId = '';

async function createSession(request: APIRequestContext, name: string): Promise<string> {
  const { createIntegrationSession } = await graphqlData<{ createIntegrationSession: { id: string } }>(request,
    'mutation ($input: CreateIntegrationSessionInput!) { createIntegrationSession(input: $input) { id } }', { input: { name } });
  return createIntegrationSession.id;
}

function browser(page: Page) {
  return page.getByRole('dialog', { name: 'Browse sessions', exact: true });
}

async function searchSessions(page: Page, text: string): Promise<void> {
  await browser(page).getByRole('textbox', { name: 'Search sessions' }).fill(text);
  await browser(page).getByRole('textbox', { name: 'Search sessions' }).press('Enter');
}

test('S1. Home pages beyond recent work and reopens the exact older session and run', async ({ page, request }, testInfo) => {
  olderSessionId = await createSession(request, `${prefix} oldest`);
  await openIDE(page, `/hl7?session=${encodeURIComponent(olderSessionId)}`);
  await expect(page.getByTestId('hl7-session-sidebar')).toHaveAttribute('data-session-id', olderSessionId);
  await enterHL7Message(page, SYNTHETIC_ADT_A01);
  const ran = page.waitForResponse((response) => selects(response.request(), 'runSessionPreview'));
  await hl7PreviewButton(page).click();
  const answer = await (await ran).json() as { data: { runSessionPreview: { id: string; sessionId: string } } };
  expect(answer.data.runSessionPreview.sessionId).toBe(olderSessionId);
  olderRunId = answer.data.runSessionPreview.id;
  for (let index = 1; index < 30; index++) await createSession(request, `${prefix} recent ${String(index).padStart(2, '0')}`);

  const watch = await watchPage(page);
  await openIDE(page, '/');
  const recent = page.getByRole('table', { name: 'Recent documents and sessions' });
  const recentSessions = recent.getByRole('row').filter({ has: page.getByRole('cell', { name: 'Session', exact: true }) });
  await expect(recentSessions).toHaveCount(8);
  await expect(recentSessions.filter({ hasText: olderSessionId })).toHaveCount(0);
  await page.getByRole('button', { name: 'Browse sessions', exact: true }).click();
  await searchSessions(page, prefix);
  const rows = browser(page).getByTestId('session-browser-row');
  await expect(rows).toHaveCount(25);
  await expect(browser(page).locator(`[data-session-id="${olderSessionId}"]`)).toHaveCount(0);
  await browser(page).getByRole('button', { name: 'Load more' }).click();
  await expect(rows).toHaveCount(30);
  const older = browser(page).locator(`[data-session-id="${olderSessionId}"]`);
  await expect(older).toContainText('completed');
  await page.screenshot({ path: testInfo.outputPath('session-browser-history.png') });
  await older.getByRole('link').click();
  await expect(page).toHaveURL((url) => url.pathname === '/hl7' && url.searchParams.get('session') === olderSessionId);
  await expect(page.getByTestId('hl7-session-selected-run')).toHaveAttribute('data-run-id', olderRunId);
  expect(watch.graphql.filter((request) => selects(request, 'integrationSessions'))).toHaveLength(0);
  const summaryReads = watch.graphql.filter((request) => selects(request, 'integrationSessionSummaries'));
  expect(summaryReads.length).toBeGreaterThan(0);
  for (const request of summaryReads) {
    const body = request.postDataJSON() as { query: string; variables: { input: { limit: number } } };
    expect(body.variables.input.limit).toBeLessThanOrEqual(25);
    expect(body.query).not.toMatch(/\bruns\s*\{/);
  }
});

test('S2. Explorer finds archived work by ID and narrow-drawer navigation respects draft protection', async ({ page, request }) => {
  expect(olderSessionId).not.toBe('');
  const archivedId = await createSession(request, `${prefix} archived`);
  await graphqlData(request, 'mutation ($id: ID!) { archiveIntegrationSession(id: $id) { id } }', { id: archivedId });
  await page.setViewportSize({ width: 900, height: 900 });
  await openIDE(page, '/connections');
  await page.getByRole('tabpanel', { name: 'Sources', exact: true }).getByRole('button', { name: 'New', exact: true }).click();
  await page.getByRole('dialog', { name: 'New source connection' }).getByRole('button', { name: /^MLLP/ }).click();
  const draftName = page.getByTestId('connection-form').locator('[data-path="name"] input');
  await draftName.fill('Keep this draft while finding older work');
  await page.getByRole('button', { name: 'Show explorer' }).click();
  await page.getByRole('dialog', { name: 'Explorer', exact: true }).getByRole('button', { name: 'Browse sessions', exact: true }).click();
  await searchSessions(page, archivedId);
  await expect(browser(page).getByTestId('session-browser-row')).toHaveCount(0);
  await browser(page).getByRole('checkbox', { name: 'Include archived' }).check();
  await expect(browser(page).locator(`[data-session-id="${archivedId}"]`)).toContainText(/archived/i);
  await searchSessions(page, olderSessionId);
  await browser(page).locator(`[data-session-id="${olderSessionId}"]`).getByRole('link').click();
  const guard = page.getByRole('dialog', { name: 'Leave Connections?' });
  await expect(guard).toBeVisible();
  await expect(page.getByRole('dialog', { name: 'Explorer', exact: true })).toHaveCount(0);
  await guard.getByRole('button', { name: 'Stay here' }).click();
  await expect(draftName).toHaveValue('Keep this draft while finding older work');
  await expect(page).toHaveURL(/\/connections$/);
});
