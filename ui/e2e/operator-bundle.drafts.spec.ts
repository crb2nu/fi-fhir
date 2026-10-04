/** Draft guards use the real shell/router and operator API; no navigation mocks. */
import { expect, test, type Page } from '@playwright/test';
import { graphqlData, openIDE } from './support';

async function newSourceDraft(page: Page, name: string): Promise<void> {
  const sources = page.getByRole('tabpanel', { name: 'Sources' });
  await sources.getByTestId('connections-new').click();
  await page.getByRole('dialog', { name: 'New source connection' }).getByRole('button', { name: /^MLLP/ }).click();
  await sources.getByTestId('connection-form').locator('[data-path="name"] input').fill(name);
}

function sourceName(page: Page) {
  return page.getByRole('tabpanel', { name: 'Sources' }).getByTestId('connection-form').locator('[data-path="name"] input');
}

test('D1. every shell navigation keeps a connection draft on cancel, and Back/Forward retains history', async ({ page }, testInfo) => {
  await openIDE(page, '/hl7');
  await page.getByRole('button', { name: 'Connections', exact: true }).click();
  await expect(page).toHaveURL(/\/connections$/);
  await newSourceDraft(page, 'Unsaved intake listener');
  const historyLength = await page.evaluate(() => window.history.length);
  const dialog = page.getByRole('dialog', { name: 'Leave Connections?' });

  for (const method of ['activity', 'tab', 'stage', 'brand', 'command', 'explorer', 'back']) {
    if (method === 'activity') await page.getByRole('button', { name: 'HL7 / Intake', exact: true }).click();
    if (method === 'tab') await page.getByRole('tab', { name: 'HL7 / Intake', exact: true }).click();
    if (method === 'stage') await page.getByTestId('stage-control').locator('[data-stage="source-intake"]').click();
    if (method === 'brand') await page.getByRole('link', { name: 'fi-fhir dashboard' }).click();
    if (method === 'command') {
      await page.getByRole('button', { name: 'Open commands' }).click();
      await page.getByRole('option', { name: /Go to HL7/ }).click();
    }
    if (method === 'explorer') {
      await page.getByRole('button', { name: 'Show explorer' }).click();
      await page.getByRole('complementary', { name: 'Explorer' }).getByRole('link', { name: 'HL7 / Intake', exact: true }).click();
    }
    if (method === 'back') await page.evaluate(() => window.history.back());
    await expect(dialog, method).toBeVisible();
    await expect(page.getByRole('dialog')).toHaveCount(1);
    if (method === 'stage') await page.screenshot({ path: testInfo.outputPath('connection-draft-guard.png') });
    await dialog.getByRole('button', { name: 'Stay here' }).click();
    await expect(page).toHaveURL(/\/connections$/);
    await expect(sourceName(page)).toHaveValue('Unsaved intake listener');
    await expect(page.getByRole('tab', { name: /^Connections/ })).toHaveAttribute('aria-selected', 'true');
    await expect(page.getByRole('tab', { name: 'HL7 / Intake', exact: true })).toHaveAttribute('aria-selected', 'false');
    if (method === 'explorer') await page.getByRole('button', { name: 'Collapse explorer' }).click();
  }

  await page.evaluate(() => window.history.back());
  await dialog.getByRole('button', { name: 'Leave view' }).click();
  await expect(page).toHaveURL(/\/hl7$/);
  expect(await page.evaluate(() => window.history.length)).toBe(historyLength);
  await page.goForward();
  await expect(page).toHaveURL(/\/connections$/);
  await expect(page.getByRole('tabpanel', { name: 'Sources' })).toBeVisible();
  await expect(page.getByTestId('connection-form')).toHaveCount(0);
});

test('D2. cancelling close retains the active draft; accepting closes it after one confirmation', async ({ page }) => {
  await openIDE(page, '/hl7');
  await page.getByRole('button', { name: 'Connections', exact: true }).click();
  await newSourceDraft(page, 'Draft before closing');
  await page.getByRole('button', { name: 'Close Connections' }).click();
  const dialog = page.getByRole('dialog', { name: 'Close Connections?' });
  await dialog.getByRole('button', { name: 'Keep open' }).click();
  await expect(sourceName(page)).toHaveValue('Draft before closing');
  await page.getByRole('button', { name: 'Close Connections' }).click();
  await dialog.getByRole('button', { name: 'Close tab' }).click();
  await expect(page).toHaveURL(/\/hl7$/);
  await expect(page.getByRole('tab', { name: /^Connections/ })).toHaveCount(0);
  await expect(page.getByRole('dialog')).toHaveCount(0);
});

test('D3. YAML-only edits survive cancelled departure and native reload with one warning', async ({ page, request }) => {
  const profile = { id: `draft-guard-${Date.now().toString(36)}`, name: 'Draft guard profile' };
  await graphqlData(request, 'mutation ($input: CreateProfileInput!) { createProfile(input: $input) { id } }', { input: profile });
  await openIDE(page, '/profiles');
  await page.getByRole('row', { name: new RegExp(profile.name) }).click();
  await page.getByRole('tab', { name: 'YAML', exact: true }).click();
  // YAML transport is deliberately unavailable on this build; its editable
  // local buffer still needs protection, including after a failed save.
  const editor = page.getByTestId('code-editor').locator('.cm-content');
  await expect(editor).toBeVisible();
  const draft = `id: ${profile.id}\nname: Local unsaved YAML`;
  await editor.fill(draft);
  await expect(page.getByRole('tab', { name: /^Profiles/ }).getByRole('img', { name: 'Unsaved changes' })).toBeVisible();
  await page.getByRole('button', { name: 'Save YAML', exact: true }).click();
  await expect(page.getByRole('alert')).toContainText('Profile YAML transport is unavailable');
  await page.getByTestId('stage-control').locator('[data-stage="source-intake"]').click();
  await page.getByRole('dialog', { name: 'Leave Profiles?' }).getByRole('button', { name: 'Stay here' }).click();
  await expect.poll(() => editor.innerText()).toBe(draft);
  await expect(page).toHaveURL(/\/profiles$/);

  let nativeWarnings = 0;
  page.on('dialog', () => { nativeWarnings += 1; });
  const reloadWarning = page.waitForEvent('dialog');
  const reload = page.evaluate(() => {
    window.setTimeout(() => window.location.reload(), 0);
  });
  const warning = await reloadWarning;
  expect(warning.type()).toBe('beforeunload');
  await warning.dismiss();
  await reload;
  expect(nativeWarnings).toBe(1);
  await expect.poll(() => editor.innerText()).toBe(draft);

  await page.getByTestId('stage-control').locator('[data-stage="source-intake"]').click();
  await page.getByRole('dialog', { name: 'Leave Profiles?' }).getByRole('button', { name: 'Leave view' }).click();
  await expect(page).toHaveURL(/\/hl7$/);
  await expect(page.getByRole('tab', { name: /^Profiles/ }).getByRole('img', { name: 'Unsaved changes' })).toHaveCount(0);
});

test('D4. confirmed full-page navigation does not ask a second time at native unload', async ({ page }) => {
  await openIDE(page, '/connections');
  await newSourceDraft(page, 'Draft leaving this deployment');
  const destination = `${process.env.E2E_PREVIEW_ONLY_URL ?? 'http://127.0.0.1:3003'}/hl7`;
  // A controlled link to the second real test stack exercises SvelteKit's
  // willUnload path, including its beforeunload suppression after approval.
  await page.evaluate((href) => {
    const link = document.createElement('a');
    link.href = href;
    link.textContent = 'Open preview deployment';
    document.body.appendChild(link);
  }, destination);
  let nativeWarnings = 0;
  page.on('dialog', async (dialog) => { nativeWarnings += 1; await dialog.dismiss(); });
  await page.getByRole('link', { name: 'Open preview deployment' }).click();
  const dialog = page.getByRole('dialog', { name: 'Leave Connections?' });
  await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: 'Leave view' }).click();
  await expect(page).toHaveURL(destination);
  expect(nativeWarnings).toBe(0);
  await expect(page.getByRole('dialog')).toHaveCount(0);
});
