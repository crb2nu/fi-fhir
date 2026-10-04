/** Closing editors preserves keyboard position through the real router and guard. */
import { expect, test } from '@playwright/test';
import { openIDE } from './support';

test('K1. Delete focuses the adjacent editor without activating it, and the final editor falls back to Home', async ({ page }) => {
  await openIDE(page, '/hl7');
  const activity = page.getByRole('navigation', { name: 'Activity bar' });
  const tabs = page.getByRole('tablist', { name: 'Open editors' });
  await activity.getByRole('button', { name: 'Connections', exact: true }).click();
  await expect(page).toHaveURL(/\/connections$/);
  await activity.getByRole('button', { name: 'Home', exact: true }).click();
  await expect(page).toHaveURL(/\/$/);

  const intake = tabs.getByRole('tab', { name: 'HL7 / Intake', exact: true });
  const connections = tabs.getByRole('tab', { name: 'Connections', exact: true });
  const home = tabs.getByRole('tab', { name: 'Home', exact: true });
  await intake.click();
  await expect(page).toHaveURL(/\/hl7$/);
  // The router resets focus on arrival; subsequent movement is keyboard-only.
  await intake.focus();
  await page.keyboard.press('ArrowRight');
  await expect(connections).toBeFocused();
  await page.keyboard.press('Delete');
  await expect(connections).toHaveCount(0);
  await expect(home).toBeFocused();
  await expect(home).toHaveAttribute('aria-selected', 'false');
  await expect(intake).toHaveAttribute('aria-selected', 'true');
  await expect(page).toHaveURL(/\/hl7$/);

  await page.keyboard.press('Delete');
  await expect(home).toHaveCount(0);
  await expect(intake).toBeFocused();
  await expect(page).toHaveURL(/\/hl7$/);
  await page.keyboard.press('Delete');
  await expect(page).toHaveURL(/\/$/);
  await expect(intake).toHaveCount(0);
  await expect(home).toBeFocused();
  await expect(tabs.getByRole('tab')).toHaveCount(1);
  // Closing the sole Home tab recreates the same ID and must still restore focus.
  await page.keyboard.press('Delete');
  await expect(home).toBeFocused();
  await expect(tabs.getByRole('tab')).toHaveCount(1);
});

test('K2. cancelling a keyboard close returns to the dirty tab; confirmation focuses the surviving editor', async ({ page }, testInfo) => {
  await openIDE(page, '/hl7');
  await page.getByRole('navigation', { name: 'Activity bar' }).getByRole('button', { name: 'Connections', exact: true }).click();
  const sources = page.getByRole('tabpanel', { name: 'Sources' });
  await sources.getByTestId('connections-new').click();
  await page.getByRole('dialog', { name: 'New source connection' }).getByRole('button', { name: /^MLLP/ }).click();
  const name = sources.getByTestId('connection-form').locator('[data-path="name"] input');
  await name.fill('Keyboard close draft');

  const tabs = page.getByRole('tablist', { name: 'Open editors' });
  const connections = tabs.getByRole('tab', { name: /^Connections/ });
  const intake = tabs.getByRole('tab', { name: 'HL7 / Intake', exact: true });
  const dialog = page.getByRole('dialog', { name: 'Close Connections?' });
  await connections.focus();
  await page.keyboard.press('Delete');
  await expect(dialog).toBeVisible();
  await dialog.getByRole('button', { name: 'Keep open' }).click();
  await expect(connections).toBeFocused();
  await expect(name).toHaveValue('Keyboard close draft');

  await page.keyboard.press('Delete');
  await expect(dialog).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(dialog).toHaveCount(0);
  await expect(connections).toBeFocused();
  await expect(name).toHaveValue('Keyboard close draft');
  await expect(page).toHaveURL(/\/connections$/);

  await page.keyboard.press('Delete');
  await dialog.getByRole('button', { name: 'Close tab' }).focus();
  await page.keyboard.press('Enter');
  await expect(page).toHaveURL(/\/hl7$/);
  await expect(connections).toHaveCount(0);
  await expect(intake).toBeFocused();
  await expect(dialog).toHaveCount(0);
  await page.screenshot({ path: testInfo.outputPath('editor-tab-keyboard-focus.png') });
});
