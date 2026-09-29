/**
 * Sessions as first-class work (.loom/42 E-3), on the operator-bundle stack:
 * a Preview creates a session, `/hl7?session=<id>` reopens it with its run in
 * the session rail, Home › Recent links to it, Export is audited (the reason
 * lands on the server's append-only export row) and carries no sample text
 * without integration.phi.export, which the operator bundle does not hold,
 * and Archive takes it off Recent.
 *
 * The checks share one session and run in order (one worker, serial).
 */
import { execFileSync } from 'node:child_process';
import { readFile } from 'node:fs/promises';
import { expect, test, type Page } from '@playwright/test';
import {
  SYNTHETIC_ADT_A01,
  enterHL7Message,
  fetchAuthStatus,
  graphqlData,
  hl7PreviewButton,
  openIDE,
  selects,
  watchPage
} from './support';

test.describe.configure({ mode: 'serial' });

let sessionId = '';
let runId = '';

function rail(page: Page) {
  return page.getByTestId('hl7-session-rail');
}

test('E3-1. a Preview creates a session; /hl7?session= and Home › Recent reopen it with its run', async ({
  page
}, testInfo) => {
  const watch = await watchPage(page);
  await openIDE(page, '/hl7');
  await enterHL7Message(page, SYNTHETIC_ADT_A01);
  const ran = page.waitForResponse((response) => selects(response.request(), 'runSessionPreview'), {
    timeout: 15_000
  });
  await hl7PreviewButton(page).click();
  const body = (await (await ran).json()) as { data?: { runSessionPreview: { id: string; sessionId: string } } };
  runId = body.data?.runSessionPreview.id ?? '';
  sessionId = body.data?.runSessionPreview.sessionId ?? '';
  expect(runId, 'runSessionPreview returned a run').not.toBe('');

  // The page writes the deep link as soon as it has a session.
  await expect(page).toHaveURL(new RegExp(`[?&]session=${encodeURIComponent(sessionId)}`));
  await expect(rail(page).getByTestId('hl7-session-sidebar')).toHaveAttribute('data-session-id', sessionId);

  // A fresh load of the link reopens it: the run is in the rail and in the results.
  await openIDE(page, `/hl7?session=${encodeURIComponent(sessionId)}`);
  const row = rail(page).locator(`[data-testid="hl7-session-run"][data-run-id="${runId}"]`);
  await expect(row).toHaveAttribute('data-status', 'completed');
  await expect(rail(page).getByTestId('hl7-session-selected-run')).toHaveAttribute('data-run-id', runId);
  await expect(rail(page).getByText('In results')).toBeVisible();
  await expect(page.getByRole('region', { name: 'Server preview progression' })).toHaveClass(/state-complete/);

  await page.setViewportSize({ width: 1440, height: 900 });
  await testInfo.attach('hl7-session-reopened.png', {
    body: await page.screenshot({ animations: 'disabled', caret: 'hide' }),
    contentType: 'image/png'
  });

  // Home › Recent lists it, and the row opens the same link.
  await openIDE(page, '/');
  const recent = page.getByRole('table', { name: 'Recent documents and sessions' });
  await recent.getByTitle(/^Open /).filter({ hasText: sessionId }).click();
  await expect(page).toHaveURL(new RegExp(`/hl7\\?session=${encodeURIComponent(sessionId)}`));
  await expect(rail(page).locator(`[data-run-id="${runId}"]`).first()).toBeVisible();

  expect(watch.errorToasts).toEqual([]);
});

test('E3-2. export without raw payloads: the phiExport sentence, a JSON download, and the reason on the export row', async ({
  page,
  request
}, testInfo) => {
  expect(sessionId, 'E3-1 created the session').not.toBe('');
  const status = await fetchAuthStatus(request, testInfo);
  // The operator bundle does not carry integration.phi.export (ui/e2e/run.sh BUNDLE_ROLES).
  expect((status.capabilities as Record<string, unknown>)['phiExport']).toBe(false);
  expect(status.missingRoles['phiExport']).toEqual(['integration.phi.export']);

  const watch = await watchPage(page);
  await openIDE(page, `/hl7?session=${encodeURIComponent(sessionId)}`);
  await rail(page).getByTestId('hl7-session-export').click();
  const dialog = page.getByTestId('hl7-session-export-dialog');
  await expect(dialog.getByTestId('session-export-phi-missing')).toContainText(
    'needs integration.phi.export, which this identity does not hold'
  );
  await expect(dialog.getByRole('checkbox')).toHaveCount(0);

  const reason = `e2e: synthetic export ${Date.now().toString(36)}`;
  await dialog.getByRole('textbox', { name: /Reason/ }).fill(reason);
  await testInfo.attach('hl7-session-export-dialog.png', {
    body: await page.screenshot({ animations: 'disabled', caret: 'hide' }),
    contentType: 'image/png'
  });
  const exported = page.waitForResponse((response) => selects(response.request(), 'exportIntegrationBundle'), {
    timeout: 10_000
  });
  const downloading = page.waitForEvent('download');
  await dialog.getByRole('button', { name: 'Export', exact: true }).click();
  const answer = (await (await exported).json()) as { errors?: unknown[] };
  expect(answer.errors ?? []).toEqual([]);
  const download = await downloading;
  expect(download.suggestedFilename()).toMatch(/^fi-fhir-session-[A-Za-z0-9._-]+-\d{8}T\d{6}Z\.json$/);

  const file = JSON.parse(await readFile(await download.path(), 'utf8')) as {
    export: { sessionId: string; reason: string; includeRawPayload: boolean };
    bundle: { sessionId: string; runs: Array<{ id: string }>; samples: Array<{ rawPayload: string | null }> };
  };
  expect(file.export).toMatchObject({ sessionId, reason, includeRawPayload: false });
  expect(file.bundle.sessionId).toBe(sessionId);
  expect(file.bundle.runs.map((run) => run.id)).toContain(runId);
  expect(file.bundle.samples.length).toBeGreaterThan(0);
  for (const sample of file.bundle.samples) expect(sample.rawPayload).toBeNull();
  await expect(rail(page).getByTestId('hl7-session-export-done')).toContainText(download.suggestedFilename());

  // The server's own record: the append-only export row with the verified
  // caller and the reason exactly as typed.
  const databaseUrl = process.env['E2E_BUNDLE_DATABASE_URL'];
  expect(databaseUrl, 'run.sh passes the bundle stack database').toBeTruthy();
  const row = execFileSync(
    'psql',
    [databaseUrl ?? '', '-X', '-q', '-t', '-A', '-F', '|', '-v', 'ON_ERROR_STOP=1', '-v', `sid=${sessionId}`],
    {
      input:
        "SELECT reason, include_raw_payload, principal_json->>'id' FROM integration_session_exports WHERE session_id = :'sid' ORDER BY exported_at DESC LIMIT 1;\n",
      encoding: 'utf8'
    }
  ).trim();
  expect(row).toBe(`${reason}|f|e2e-ide-operator`);
  expect(watch.errorToasts).toEqual([]);
});

test('E3-3. archive takes the session off Home › Recent; its link still opens it, marked archived', async ({
  page,
  request
}) => {
  expect(sessionId, 'E3-1 created the session').not.toBe('');
  const watch = await watchPage(page);
  await openIDE(page, `/hl7?session=${encodeURIComponent(sessionId)}`);
  await rail(page).getByTestId('hl7-session-archive').click();
  const dialog = page.getByTestId('hl7-session-archive-dialog');
  await expect(dialog).toContainText('The API records no reason for an archive.');
  await expect(dialog.getByRole('textbox')).toHaveCount(0);
  const archived = page.waitForResponse((response) => selects(response.request(), 'archiveIntegrationSession'), {
    timeout: 10_000
  });
  await dialog.getByRole('button', { name: 'Archive', exact: true }).click();
  expect(((await (await archived).json()) as { errors?: unknown[] }).errors ?? []).toEqual([]);
  await expect(rail(page).getByTestId('hl7-session-archived')).toBeVisible();
  await expect(rail(page).getByTestId('hl7-session-archive')).toBeDisabled();

  const { integrationSessions } = await graphqlData<{ integrationSessions: Array<{ id: string }> }>(
    request,
    '{ integrationSessions { id } }'
  );
  expect(integrationSessions.map((session) => session.id)).not.toContain(sessionId);

  await openIDE(page, `/hl7?session=${encodeURIComponent(sessionId)}`);
  await expect(rail(page).getByTestId('hl7-session-archived')).toBeVisible();
  await expect(rail(page).locator(`[data-run-id="${runId}"]`).first()).toBeVisible();
  expect(watch.errorToasts).toEqual([]);
});

test('E3-4. a deep link to an id the store does not hold says so and creates nothing', async ({ page }) => {
  const watch = await watchPage(page);
  await openIDE(page, '/hl7?session=e2e-no-such-session');
  await expect(page.getByTestId('hl7-session-absent')).toContainText(
    "Session e2e-no-such-session is not in this deployment's session store"
  );
  expect(watch.graphql.filter((request) => selects(request, 'createIntegrationSession'))).toHaveLength(0);
  expect(watch.errorToasts).toEqual([]);
});
