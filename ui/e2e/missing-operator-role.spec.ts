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

test('6c. without integration.operator the connections page pre-flights, names the role, and queries nothing', async ({
  page,
  request
}, testInfo) => {
  const status = await fetchAuthStatus(request, testInfo);
  // The catalog is configured here; only the role is missing.
  expect(status.capabilities.controlPlane).toBe(true);
  expect(status.capabilities.connectionsRead).toBe(false);
  expect(status.missingRoles['connectionsRead']).toEqual(['integration.operator']);

  const watch = await watchPage(page);
  await openIDE(page, '/connections');

  const preflight = page.getByTestId('connections-preflight');
  await expect(preflight).toBeVisible();
  await expect(preflight).toHaveAttribute('data-reason', 'missing-role');
  await expect(preflight).toHaveAttribute('data-missing-roles', 'integration.operator');
  await expect(preflight).toContainText('integration.operator');
  await expect(page.getByRole('tablist', { name: 'Connection views' })).toHaveCount(0);

  const connectionReads = watch.graphql.filter(
    (request) => selects(request, 'connections') || selects(request, 'engineRuntime')
  );
  expect(connectionReads, 'no connections or engineRuntime query was sent').toHaveLength(0);
  await expect(page.locator('body')).not.toContainText(/forbidden/i);
  expect(watch.errorToasts).toEqual([]);
});

test('E0-7. controlPlane pre-flight: configured here, so /operator and Home name the missing role, never "not configured"', async ({
  page,
  request
}, testInfo) => {
  const status = await fetchAuthStatus(request, testInfo);
  expect(status.capabilities.controlPlane).toBe(true);
  expect(status.capabilities.operatorRead).toBe(false);

  const watch = await watchPage(page);
  await openIDE(page, '/operator');
  const preflight = page.getByTestId('operator-preflight');
  await expect(preflight).toHaveAttribute('data-reason', 'missing-role');
  await expect(preflight).toHaveAttribute('data-missing-roles', 'integration.operator');
  await expect(preflight).not.toContainText('not configured');

  await openIDE(page, '/');
  const integrations = page.getByTestId('integrations-preflight');
  await expect(integrations).toHaveAttribute('data-reason', 'missing-role');
  await expect(integrations).toHaveAttribute('data-missing-roles', 'integration.operator');
  // Home's fleet row is gated on the same role: it says so and reads nothing.
  await expect(page.getByTestId('health-fleet')).toContainText('integration.operator');

  for (const field of ['operatorReceipts', 'operatorDeployments', 'engineRuntime']) {
    expect(watch.graphql.filter((request) => selects(request, field)), `${field} was not issued`).toHaveLength(0);
  }
  expect(watch.errorToasts).toEqual([]);
});
