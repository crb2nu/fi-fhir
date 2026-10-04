/** Record links stay aligned with the selected editor without replacing drafts. */
import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import { graphqlData, openIDE, selects, watchPage } from './support';

async function createConnection(request: APIRequestContext, suffix: string, direction: 'SOURCE' | 'DESTINATION') {
  const id = `nav-${suffix}-${Date.now().toString(36)}`;
  const name = `Navigation ${suffix}`;
  const data = await graphqlData<{ createConnection: { id: string; version: number } }>(request,
    'mutation ($input: CreateConnectionInput!) { createConnection(input: $input) { id version } }', {
      input: { id, name, direction, kind: direction === 'SOURCE' ? 'MLLP' : 'FHIR', spec: {}, reason: 'E2E: synthetic record navigation fixture' }
    });
  return { ...data.createConnection, name };
}

function visibleConnectionField(page: Page, path: string) {
  return page.locator(`[data-testid="connection-form"]:visible [data-path="${path}"] input`);
}

function connectionURL(id: string): string {
  return `/connections?${new URLSearchParams({ connection: id })}`;
}

test('R1. Explorer opens real source and destination records and keeps edits through Back/Forward', async ({ page, request }, testInfo) => {
  const source = await createConnection(request, 'source', 'SOURCE');
  const destination = await createConnection(request, 'destination', 'DESTINATION');
  const watch = await watchPage(page);
  await openIDE(page, '/hl7');
  await page.getByRole('button', { name: 'Show explorer' }).click();
  const explorer = page.getByRole('complementary', { name: 'Explorer' });
  const sourceLink = explorer.getByRole('link').filter({ hasText: source.name });
  const destinationLink = explorer.getByRole('link').filter({ hasText: destination.name });
  await expect(sourceLink).toHaveAttribute('href', connectionURL(source.id));
  await sourceLink.click();
  await expect(visibleConnectionField(page, 'id')).toHaveValue(source.id);
  await expect(sourceLink).toHaveAttribute('aria-current', 'page');
  await visibleConnectionField(page, 'name').fill('Unsaved navigation source');

  await destinationLink.click();
  await expect(page).toHaveURL(new RegExp(`connection=${destination.id}$`));
  await expect(visibleConnectionField(page, 'id')).toHaveValue(destination.id);
  await expect(destinationLink).toHaveAttribute('aria-current', 'page');
  await expect(sourceLink).not.toHaveAttribute('aria-current', 'page');
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await page.goBack();
  await expect(visibleConnectionField(page, 'id')).toHaveValue(source.id);
  await expect(visibleConnectionField(page, 'name')).toHaveValue('Unsaved navigation source');
  await expect(sourceLink).toHaveAttribute('aria-current', 'page');
  await page.goForward();
  await expect(visibleConnectionField(page, 'id')).toHaveValue(destination.id);
  await sourceLink.click();
  await expect(visibleConnectionField(page, 'name')).toHaveValue('Unsaved navigation source');
  await page.screenshot({ path: testInfo.outputPath('explorer-connections.png') });

  const writes = watch.graphql.filter((request) =>
    ['createConnection', 'updateConnection', 'archiveConnection', 'compileConnection'].some((field) => selects(request, field)));
  expect(writes, 'record navigation never saves or compiles a connection').toEqual([]);
});

test('R2. copied connection URLs resolve archived records and missing targets honestly', async ({ page, request }) => {
  const record = await createConnection(request, 'archived', 'SOURCE');
  await graphqlData(request, 'mutation ($input: ConnectionCommandInput!) { archiveConnection(input: $input) { id } }', {
    input: { id: record.id, expectedVersion: record.version, reason: 'E2E: archived record link fixture' }
  });
  await openIDE(page, connectionURL(record.id));
  await expect(visibleConnectionField(page, 'id')).toHaveValue(record.id);
  await expect(visibleConnectionField(page, 'name')).toHaveAttribute('readonly');
  await expect(page.locator('[data-testid="connection-form"]:visible')).toBeVisible();
  await page.reload();
  await expect(visibleConnectionField(page, 'id')).toHaveValue(record.id);

  await openIDE(page, connectionURL(`missing-${record.id}`));
  await expect(page.getByTestId('connection-target-state')).toContainText(/unavailable|not available|not found/i);
  await expect(page.locator('[data-testid="connection-form"]:visible')).toHaveCount(0);
});

test('R3. browsing an existing definition retains the unfinished definition form', async ({ page }) => {
  await openIDE(page, '/connections?definition=e2e-batch-adt&revision=v1');
  await expect(page.getByTestId('definition-details')).toBeVisible();
  await page.getByTestId('definitions-new').click();
  const form = page.getByTestId('definition-new');
  await form.getByTestId('definition-new-id').fill('unfinished-navigation-definition');
  await page.getByTestId('definitions-table').locator('[data-row="e2e-batch-adt/v1"]').click();
  await expect(page).toHaveURL(/definition=e2e-batch-adt&revision=v1$/);
  await expect(page.getByTestId('definition-details')).toBeVisible();
  await expect(form).toBeHidden();
  await expect(page.getByRole('dialog')).toHaveCount(0);
  await expect(page.getByTestId('definitions-new')).toHaveText('Continue draft');
  await page.getByTestId('definitions-new').click();
  await expect(form.getByTestId('definition-new-id')).toHaveValue('unfinished-navigation-definition');
  expect(new URL(page.url()).searchParams.has('definition')).toBe(false);
});


/** Follow a copied same-route link through SvelteKit without reloading the page. */
async function followCopiedLink(page: Page, href: string): Promise<void> {
  await page.evaluate((target) => {
    document.getElementById('e2e-copied-resource-link')?.remove();
    const link = document.createElement('a');
    link.id = 'e2e-copied-resource-link';
    link.href = target;
    link.textContent = 'Open copied resource link';
    document.body.append(link);
    link.click();
    link.remove();
  }, href);
  await expect(page).toHaveURL((url) => url.pathname + url.search === href);
}

test('R4. receipt links update Operator and Verification in place and follow browser history', async ({ page, request }) => {
  const { operatorReceipts } = await graphqlData<{ operatorReceipts: { nodes: Array<{ receiptId: string }> } }>(request,
    'query { operatorReceipts(filter: { status: "accepted" }, page: { first: 5 }) { nodes { receiptId } } }');
  expect(operatorReceipts.nodes.length).toBeGreaterThanOrEqual(2);
  const [first, second] = operatorReceipts.nodes;
  const firstId = first!.receiptId;
  const secondId = second!.receiptId;
  const operatorLink = (id: string) => `/operator?receipt=${encodeURIComponent(id)}`;
  const eventsLink = (id: string) => `/events?receipt=${encodeURIComponent(id)}`;
  await openIDE(page, operatorLink(firstId));
  await expect(page.getByTestId('trace-events-link')).toHaveAttribute('href', eventsLink(firstId));
  await followCopiedLink(page, operatorLink(secondId));
  await expect(page.locator('.receipt-id')).toHaveText(secondId);
  await expect(page.getByTestId('trace-events-link')).toHaveAttribute('href', eventsLink(secondId));
  await page.goBack();
  await expect(page.locator('.receipt-id')).toHaveText(firstId);
  await expect(page.getByTestId('trace-events-link')).toHaveAttribute('href', eventsLink(firstId));
  await page.goForward();
  await expect(page.getByTestId('trace-events-link')).toHaveAttribute('href', eventsLink(secondId));
  await page.getByTestId('trace-events-link').click();
  await expect(page.getByRole('textbox', { name: 'Receipt', exact: true })).toHaveValue(secondId);
  await expect(page.getByTestId('admission-row').first()).toHaveAttribute('data-receipt-id', secondId);
  await page.getByRole('textbox', { name: 'Event type' }).fill('unfinished-filter');
  await followCopiedLink(page, eventsLink(firstId));
  await expect(page.getByRole('textbox', { name: 'Receipt', exact: true })).toHaveValue(firstId);
  await expect(page.getByRole('textbox', { name: 'Event type' })).toHaveValue('unfinished-filter');
  await expect(page.getByTestId('admission-row').first()).toHaveAttribute('data-receipt-id', firstId);
  await page.goBack();
  await expect(page.getByTestId('admission-row').first()).toHaveAttribute('data-receipt-id', secondId);
  await expect(page.getByRole('textbox', { name: 'Event type' })).toHaveValue('unfinished-filter');
  await followCopiedLink(page, '/events?receipt=missing-navigation-receipt');
  await expect(page.getByTestId('admissions-empty')).toContainText('No admissions match');
  await expect(page.getByTestId('admission-row')).toHaveCount(0);
});

test('R5. internal receipt selections are remembered by workspace tabs and survive reload', async ({ page, request }) => {
  const { operatorReceipts } = await graphqlData<{ operatorReceipts: { nodes: Array<{ receiptId: string }> } }>(request,
    'query { operatorReceipts(filter: { status: "accepted" }, page: { first: 5 }) { nodes { receiptId } } }');
  const [first, second] = operatorReceipts.nodes;
  expect(second).toBeTruthy();
  const firstId = first!.receiptId;
  const secondId = second!.receiptId;
  const activity = page.getByRole('navigation', { name: 'Activity bar' });
  const tabs = page.getByRole('tablist', { name: 'Open editors' });
  await openIDE(page, `/operator?receipt=${encodeURIComponent(firstId)}`);
  await page.locator(`[data-testid="receipt-row"][data-receipt-id="${secondId}"]`).click();
  await expect(page).toHaveURL((url) => url.searchParams.get('receipt') === secondId);
  await expect(page.getByTestId('trace-events-link')).toHaveAttribute('href', `/events?receipt=${encodeURIComponent(secondId)}`);
  await activity.getByRole('button', { name: 'Home', exact: true }).click();
  await tabs.getByRole('tab', { name: /Operator/ }).click();
  await expect(page).toHaveURL((url) => url.pathname === '/operator' && url.searchParams.get('receipt') === secondId);
  await expect(page.locator('.receipt-id')).toHaveText(secondId);
  await page.reload();
  await expect(page.locator('.receipt-id')).toHaveText(secondId);

  await page.getByTestId('trace-events-link').click();
  await page.getByRole('textbox', { name: 'Receipt', exact: true }).fill(firstId);
  await page.getByRole('button', { name: 'Apply', exact: true }).click();
  await expect(page).toHaveURL((url) => url.pathname === '/events' && url.searchParams.get('receipt') === firstId);
  await expect(page.getByTestId('admission-row').first()).toHaveAttribute('data-receipt-id', firstId);
  await activity.getByRole('button', { name: 'Home', exact: true }).click();
  await tabs.getByRole('tab', { name: /Verification/ }).click();
  await expect(page).toHaveURL((url) => url.pathname === '/events' && url.searchParams.get('receipt') === firstId);
  await expect(page.getByRole('textbox', { name: 'Receipt', exact: true })).toHaveValue(firstId);
});
