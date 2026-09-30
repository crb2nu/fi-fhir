/**
 * The hosted demo's identity (stack `preview-only`, `.loom/40` D-3): the
 * trusted network hands every caller `integration:preview` as
 * `fi-fhir-demo-visitor`; the API has no database, no control plane, no
 * sessions and no LLM. The transport gate admits only `health` and
 * `previewIntegrationMessage` for that role.
 *
 * What must hold for the demo to be honest: HL7 Preview of the built-in sample
 * works, and every other route shows its "not available" state — a pre-flight
 * naming the missing role, or the "not configured" state naming the env key —
 * without issuing the query it cannot run, without an error toast, without a
 * console error, and without a spinner left behind.
 */
import { expect, test, type ConsoleMessage, type Page, type Request } from '@playwright/test';
import {
  FORBIDDEN_COPY,
  fetchAuthStatus,
  hl7PreviewButton,
  isStreamRequest,
  openIDE,
  selects,
  watchPage,
  type PageWatch
} from './support';

const DEMO_PRINCIPAL = 'fi-fhir-demo-visitor';

/** Every console error and uncaught page error, for the page's whole life. */
function watchConsole(page: Page): string[] {
  const errors: string[] = [];
  page.on('console', (message: ConsoleMessage) => {
    if (message.type() === 'error') errors.push(message.text());
  });
  page.on('pageerror', (error) => errors.push(`pageerror: ${error.message}`));
  return errors;
}

/** GraphQL requests other than the shell's `health` poll. */
function nonHealthRequests(watch: PageWatch): Request[] {
  return watch.graphql.filter((request) => !selects(request, 'health'));
}

/** The copy register, "forbidden" anywhere, and nothing left busy. */
async function expectHonestSettledPage(page: Page): Promise<void> {
  await expect(page.locator('[aria-busy="true"]'), 'nothing is busy').toHaveCount(0);
  await expect(page.getByText(/^Loading\b/).filter({ visible: true }), 'nothing is loading').toHaveCount(0);
  await expect(page.locator('body')).not.toContainText(/forbidden/i);
  const text = await page.evaluate(() => document.body.innerText);
  expect(FORBIDDEN_COPY.filter((phrase) => text.includes(phrase)), 'copy register').toEqual([]);
}

test('P1. auth status: the demo visitor holds integration:preview only, on a deployment with nothing configured', async ({
  request
}, testInfo) => {
  const status = await fetchAuthStatus(request, testInfo);
  expect(status.authenticated).toBe(true);
  expect(status.authVia).toBe('network');
  expect(status.principal).toBe(DEMO_PRINCIPAL);
  expect(status.roles).toEqual(['integration:preview']);
  expect(status.capabilities).toMatchObject({
    operatorRead: false,
    operatorDelivery: false,
    operatorDeployment: false,
    clinicalRead: false,
    connectionsRead: false,
    connectionsWrite: false,
    integrationSessions: false,
    streaming: false,
    subscriptions: [],
    llm: { configured: false },
    controlPlane: false,
    connectionCatalog: false
  });
  expect(status.missingRoles['clinicalRead']).toEqual(['clinical:read']);
  expect(status.missingRoles['operatorRead']).toEqual(['integration.operator']);

  // The transport gate is the boundary: health answers, a catalog read is refused.
  const health = await request.post('/graphql', {
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    data: { query: 'query Health { health { status } }' }
  });
  expect(((await health.json()) as { data?: { health?: { status?: string } } }).data?.health?.status).toBe(
    'healthy'
  );
  const refused = await request.post('/graphql', {
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    data: { query: 'query Profiles { profiles { id } }' }
  });
  const refusedBody = (await refused.json()) as { data?: unknown; errors?: { extensions?: { code?: string } }[] };
  expect(refusedBody.data ?? null).toBeNull();
  expect(refusedBody.errors?.[0]?.extensions?.code).toBe('FORBIDDEN');
});

test('P2. HL7 intake previews the built-in sample on the stateless path; Profile draft and Process are honest', async ({
  page
}) => {
  const watch = await watchPage(page);
  const consoleErrors = watchConsole(page);
  await openIDE(page, '/hl7');

  // The built-in sample is in the editor; streaming is honestly unavailable.
  await expect(page.getByTestId('code-editor').locator('.cm-content')).toContainText('MSH|');
  const notice = page.locator('[data-testid="streaming-unavailable"][data-stream="integrationSessionEvents"]');
  await expect(notice).toBeVisible();
  await expect(notice).toHaveAttribute('data-reason', 'streaming-off');

  const preview = page.waitForResponse((response) => selects(response.request(), 'previewIntegrationMessage'), {
    timeout: 10_000
  });
  await hl7PreviewButton(page).click();
  const response = await preview;
  expect(response.status()).toBe(200);
  const body = (await response.json()) as {
    errors?: unknown[];
    data?: { previewIntegrationMessage?: { events?: { type?: string }[] } };
  };
  expect(body.errors ?? [], 'preview answered without GraphQL errors').toEqual([]);
  // The real engine turned the built-in ADT^A01 into an event.
  expect(body.data?.previewIntegrationMessage?.events?.length ?? 0, 'the engine produced events').toBeGreaterThan(0);
  await expect(page.getByTestId('hl7-run-status')).toHaveText('Parsed');

  // Process needs the compatibility grant: disabled, the reason in its title.
  const process = page.getByRole('button', { name: 'Process', exact: true });
  await expect(process).toBeDisabled();
  await expect(process).toHaveAttribute('title', /graphql:operator/);
  await page.getByRole('tab', { name: 'Process', exact: true }).click();
  await expect(page.getByTestId('hl7-process-preflight')).toHaveAttribute('data-missing-roles', 'graphql:operator');

  // Profile draft names the role instead of listing profiles it cannot read.
  await page.getByRole('tab', { name: 'Profile draft', exact: true }).click();
  const profile = page.getByTestId('hl7-profile-preflight');
  await expect(profile).toBeVisible();
  await expect(profile).toHaveAttribute('data-reason', 'missing-role');
  await expect(profile).toHaveAttribute('data-missing-roles', 'graphql:operator');
  await expect(profile).toContainText(DEMO_PRINCIPAL);

  // Copilot: the status endpoint said no LLM; the panel says so and does not probe.
  await page.getByRole('tab', { name: /Copilot/ }).click();
  const llm = page.getByTestId('copilot-llm-state');
  await expect(llm).toHaveAttribute('data-state', 'not-configured');

  await expectHonestSettledPage(page);
  const issued = nonHealthRequests(watch).filter((request) => !selects(request, 'previewIntegrationMessage'));
  expect(issued.map((request) => request.postData()?.slice(0, 80)), 'only health and preview were sent').toEqual([]);
  expect(watch.graphql.filter(isStreamRequest), 'no stream was attempted').toHaveLength(0);
  expect(watch.errorToasts).toEqual([]);
  expect(consoleErrors).toEqual([]);
});

interface HonestRoute {
  id: string;
  path: string;
  /** The honest state's data-testid. */
  testid: string;
  reason?: 'missing-role' | 'not-configured';
  missingRoles?: string;
}

const ROUTES: HonestRoute[] = [
  // No database, so no control plane: "not configured" outranks the missing
  // role (.loom/42 E-0, the Connections precedence); the role is still named.
  { id: 'P3', path: '/', testid: 'integrations-preflight', reason: 'not-configured' },
  { id: 'P4', path: '/workflows', testid: 'workflows-preflight', reason: 'missing-role', missingRoles: 'graphql:operator' },
  // Verification reads the operator control plane since .loom/42 E-2: no
  // database here, so "not configured" (the Connections precedence).
  { id: 'P5', path: '/events', testid: 'verification-preflight', reason: 'not-configured' },
  { id: 'P6', path: '/profiles', testid: 'profiles-preflight', reason: 'missing-role', missingRoles: 'graphql:operator' },
  {
    id: 'P7',
    path: '/terminology',
    testid: 'terminology-preflight',
    reason: 'missing-role',
    missingRoles: 'graphql:operator'
  },
  // The catalog needs a database this deployment does not have: "not configured"
  // outranks the missing role (.loom/38 C-1 precedence).
  { id: 'P8', path: '/connections', testid: 'connections-preflight', reason: 'not-configured' },
  { id: 'P9', path: '/operator', testid: 'operator-preflight', reason: 'not-configured' }
];

for (const route of ROUTES) {
  test(`${route.id}. ${route.path} shows its honest state (${route.testid}) and queries nothing it cannot run`, async ({
    page
  }) => {
    const watch = await watchPage(page);
    const consoleErrors = watchConsole(page);
    await openIDE(page, route.path);

    const state = page.getByTestId(route.testid);
    await expect(state).toBeVisible();
    if (route.reason) await expect(state).toHaveAttribute('data-reason', route.reason);
    if (route.missingRoles) {
      await expect(state).toHaveAttribute('data-missing-roles', route.missingRoles);
      await expect(state).toContainText(route.missingRoles);
    }
    if (route.path === '/') {
      await expect(page.getByTestId('health-summary')).toHaveText(/Healthy|Degraded/);
    }

    await expectHonestSettledPage(page);
    expect(
      nonHealthRequests(watch).map((request) => request.postData()?.slice(0, 80)),
      'no query beyond health was issued'
    ).toEqual([]);
    expect(watch.errorToasts).toEqual([]);
    expect(consoleErrors).toEqual([]);
  });
}

test('E1-2. /connections › Definitions says definition authoring is not configured, and queries no definition', async ({
  page
}) => {
  const watch = await watchPage(page);
  await openIDE(page, '/connections');
  await page.getByTestId('connections-tab-definitions').click();
  const preflight = page.getByTestId('definitions-preflight');
  await expect(preflight).toBeVisible();
  await expect(preflight).toHaveAttribute('data-reason', 'not-configured');
  await expect(preflight).toContainText('Definition authoring is not configured on this deployment');
  for (const field of ['integrationDefinitions', 'integrationDefinition', 'integrationRegistryArtifacts']) {
    expect(watch.graphql.filter((request) => selects(request, field)), `${field} was not issued`).toHaveLength(0);
  }
});

test('E0-8. controlPlane pre-flight: /operator and Home say the control plane is not configured, and query nothing', async ({
  page,
  request
}, testInfo) => {
  const status = await fetchAuthStatus(request, testInfo);
  expect(status.capabilities.controlPlane).toBe(false);

  const watch = await watchPage(page);
  await openIDE(page, '/operator?receipt=e2e-any-receipt');
  const preflight = page.getByTestId('operator-preflight');
  await expect(preflight).toHaveAttribute('data-reason', 'not-configured');
  await expect(preflight).toContainText('The operator control plane is not configured on this deployment');
  await expect(preflight).toContainText('FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true');
  await expect(preflight).toHaveAttribute('data-missing-roles', 'integration.operator');

  await openIDE(page, '/');
  await expect(page.getByTestId('integrations-preflight')).toHaveAttribute('data-reason', 'not-configured');
  await expect(page.getByTestId('health-fleet')).toContainText('not configured');

  expect(nonHealthRequests(watch).map((request) => request.postData()?.slice(0, 80))).toEqual([]);
  expect(watch.errorToasts).toEqual([]);
});

test('E2-5. Verification pre-flight: /events says the control plane is not configured, names the role, and queries nothing', async ({
  page,
  request
}, testInfo) => {
  const status = await fetchAuthStatus(request, testInfo);
  expect(status.capabilities.controlPlane).toBe(false);

  const watch = await watchPage(page);
  await openIDE(page, '/events?receipt=e2e-any-receipt');
  const preflight = page.getByTestId('verification-preflight');
  await expect(preflight).toHaveAttribute('data-reason', 'not-configured');
  await expect(preflight).toContainText('the operator control plane, which is not configured on this deployment');
  await expect(preflight).toContainText('FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true');
  await expect(preflight).toHaveAttribute('data-missing-roles', 'integration.operator');
  await expect(page.getByRole('tab', { name: 'Admissions' })).toHaveCount(0);

  expect(nonHealthRequests(watch).map((request) => request.postData()?.slice(0, 80))).toEqual([]);
  expect(watch.errorToasts).toEqual([]);
});
