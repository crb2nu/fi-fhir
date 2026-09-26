/**
 * Negative control 6a (stack `missing-operator-role`): the operator bundle
 * minus `integration.operator` — production's shape before 2026-09-25, when
 * the transport grant admitted every operator field and the control plane
 * refused them all. The gate must be able to fail for the reason it exists:
 * this stack differs from `operator-bundle` in that one role only.
 */
import { expect, test } from '@playwright/test';
import { fetchAuthStatus, openIDE, selects, watchPage } from './support';

test('6a. without integration.operator the operator page pre-flights, names the role, and queries nothing', async ({
  page,
  request
}, testInfo) => {
  const status = await fetchAuthStatus(request, testInfo);
  expect(status.authVia).toBe('network');
  expect(status.roles).toContain('graphql:operator');
  expect(status.roles).not.toContain('integration.operator');
  expect(status.capabilities.operatorRead).toBe(false);
  expect(status.missingRoles['operatorRead']).toEqual(['integration.operator']);

  const watch = await watchPage(page);
  await openIDE(page, '/operator');

  const preflight = page.getByTestId('operator-preflight');
  await expect(preflight).toBeVisible();
  await expect(preflight).toHaveAttribute('data-missing-roles', 'integration.operator');
  await expect(preflight).toContainText('integration.operator');
  await expect(page.getByText('No messages match these filters')).toHaveCount(0);

  expect(watch.graphql.filter((request) => selects(request, 'operatorReceipts'))).toHaveLength(0);
  expect(watch.errorToasts).toEqual([]);
});
