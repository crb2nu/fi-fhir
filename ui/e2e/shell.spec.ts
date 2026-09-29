/**
 * Shell honesty (.loom/42 E-4), in the `operator-bundle` project (it runs
 * after operator-bundle.spec.ts): the journey stages come from evidence the
 * engine holds, not from route order, and there is one command palette.
 *
 *   E4-1  on the operator-bundle stack every stage's state equals what the
 *         API says about its evidence, and Source Intake turns complete once
 *         a session has a run
 *   E4-2  on the preview-only stack (:3003) no stage is complete: each is
 *         `unknown`, says why, and nothing but health is queried
 *   E4-3  on /hl7, Cmd/Ctrl+K and the header button open the same single
 *         palette, holding HL7's commands and the shell's
 */
import { expect, test, type APIRequestContext, type Page } from '@playwright/test';
import {
  SYNTHETIC_ADT_A01,
  enterHL7Message,
  hl7PreviewButton,
  openIDE,
  selects,
  watchPage
} from './support';

const PREVIEW_ONLY_URL = process.env.E2E_PREVIEW_ONLY_URL ?? 'http://127.0.0.1:3003';

type StageId = 'source-intake' | 'normalization' | 'translation' | 'delivery' | 'verification';
type Evidence = 'complete' | 'incomplete' | 'unknown';
const STAGES: StageId[] = ['source-intake', 'normalization', 'translation', 'delivery', 'verification'];

/** One GraphQL read through nginx; `null` when it answered with errors. */
async function read<T>(request: APIRequestContext, query: string): Promise<T | null> {
  const response = await request.post('/graphql', {
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    data: { query }
  });
  if (response.status() !== 200) return null;
  const body = (await response.json()) as { data?: T; errors?: unknown[] };
  return body.errors && body.errors.length > 0 ? null : (body.data ?? null);
}

/** The evidence the stack holds, by the same rules as journeyState.ts. */
async function expectedEvidence(request: APIRequestContext): Promise<Record<StageId, Evidence>> {
  const sessions = await read<{ integrationSessions: { runs: unknown[] }[] }>(
    request,
    '{ integrationSessions { runs { id } } }'
  );
  const profiles = await read<{ profiles: unknown[] }>(request, '{ profiles(activeOnly: true) { id } }');
  const mappings = await read<{ listMappings: { totalCount: number } }>(
    request,
    '{ listMappings(input: { first: 1 }) { totalCount } }'
  );
  const autoroutes = await read<{ pendingAutorouteStats: { approvedCount: number } }>(
    request,
    '{ pendingAutorouteStats { approvedCount } }'
  );
  const workflows = await read<{ workflowDefinitions: { publishedVersionsByEnv: unknown }[] }>(
    request,
    '{ workflowDefinitions { publishedVersionsByEnv } }'
  );
  const receipts = await read<{ operatorReceipts: { nodes: unknown[] } }>(
    request,
    '{ operatorReceipts(filter: { status: "accepted" }, page: { first: 1 }) { nodes { receiptId } } }'
  );

  const count = (value: number | null): Evidence => (value === null ? 'unknown' : value > 0 ? 'complete' : 'incomplete');
  const published = (byEnv: unknown) =>
    !!byEnv && typeof byEnv === 'object' && Object.values(byEnv as Record<string, unknown>).some((v) => v !== null && v !== '');

  const mappingCount = mappings?.listMappings.totalCount ?? null;
  const approvedCount = autoroutes?.pendingAutorouteStats.approvedCount ?? null;
  let translation: Evidence;
  if ((mappingCount ?? 0) > 0 || (approvedCount ?? 0) > 0) translation = 'complete';
  else if (mappingCount === null || approvedCount === null) translation = 'unknown';
  else translation = 'incomplete';

  return {
    'source-intake': count(sessions ? sessions.integrationSessions.filter((s) => s.runs.length > 0).length : null),
    normalization: count(profiles ? profiles.profiles.length : null),
    translation,
    delivery: count(workflows ? workflows.workflowDefinitions.filter((w) => published(w.publishedVersionsByEnv)).length : null),
    verification: count(receipts ? receipts.operatorReceipts.nodes.length : null)
  };
}

function stage(page: Page, id: StageId) {
  return page.getByTestId('stage-control').locator(`a[data-stage="${id}"]`);
}

/** Resolves once the shell has read every stage's evidence. */
async function evidenceSettled(page: Page): Promise<void> {
  await expect(page.getByTestId('stage-control').locator('a[data-stage]')).toHaveCount(5);
  await expect(page.getByTestId('stage-control').locator('a[data-state="pending"]')).toHaveCount(0);
}

test('E4-1. journey stages equal the evidence the operator-bundle stack holds; a session run completes Source Intake', async ({
  page,
  request
}) => {
  // Evidence for Source Intake: a session with a run. Operator-bundle check 3
  // leaves one; make one here if this file ran alone.
  let expected = await expectedEvidence(request);
  if (expected['source-intake'] !== 'complete') {
    await openIDE(page, '/hl7');
    await enterHL7Message(page, SYNTHETIC_ADT_A01);
    await hl7PreviewButton(page).click();
    await expect(page.getByRole('region', { name: 'Server preview progression' })).toHaveClass(/state-complete/);
    expected = await expectedEvidence(request);
  }
  expect(expected['source-intake'], 'the stack holds a session with a run').toBe('complete');

  await openIDE(page, '/operator');
  await evidenceSettled(page);
  for (const id of STAGES) {
    await expect(stage(page, id), `${id} matches the API`).toHaveAttribute('data-state', expected[id]);
  }
  // Complete by evidence while not behind the current route: /operator is
  // outside the stages, so nothing is "earlier" here.
  await expect(stage(page, 'source-intake').locator('.stage-check')).toHaveCount(1);
  await expect(stage(page, 'source-intake')).toHaveAttribute('title', /integration sessions? with a run\./);
  // The bundle holds every role: nothing is unknown for want of one.
  for (const id of STAGES) {
    await expect(stage(page, id)).not.toHaveAttribute('title', /which this identity does not hold/);
  }

  // Next: the earliest stage whose evidence is missing (Home offers it).
  await openIDE(page, '/');
  await evidenceSettled(page);
  const firstIncomplete = STAGES.find((id) => expected[id] === 'incomplete');
  if (firstIncomplete) {
    await expect(page.getByTestId('status-next')).toHaveAttribute('data-stage', firstIncomplete);
  } else {
    await expect(page.getByTestId('status-next')).toHaveCount(0);
  }

  // The sidebar's stage badge reads the same evidence.
  await openIDE(page, '/hl7');
  await evidenceSettled(page);
  await page.keyboard.press('ControlOrMeta+b');
  const badge = page.getByTestId('sidebar-stage-badge');
  await expect(badge).toHaveText('1/5');
  await expect(badge).toHaveAttribute('data-state', 'complete');
});

test('E4-2. on the preview-only stack no stage is complete: each is unknown, says why, and nothing is queried', async ({
  page
}) => {
  const watch = await watchPage(page);
  await openIDE(page, `${PREVIEW_ONLY_URL}/`);
  await evidenceSettled(page);

  for (const id of STAGES) {
    await expect(stage(page, id), `${id} is unknown here`).toHaveAttribute('data-state', 'unknown');
  }
  await expect(page.getByTestId('stage-control').locator('.stage-check')).toHaveCount(0);
  await expect(stage(page, 'normalization')).toHaveAttribute('title', /needs graphql:operator, which this identity does not hold\./);
  await expect(stage(page, 'verification')).toHaveAttribute('title', /not configured on this deployment\./);
  await expect(page.getByTestId('status-next')).toHaveCount(0);

  const issued = watch.graphql.filter((request) => !selects(request, 'health'));
  expect(issued.map((request) => request.postData()?.slice(0, 80)), 'no evidence query was sent').toEqual([]);
  expect(watch.errorToasts).toEqual([]);
});

test('E4-3. on /hl7 Cmd/Ctrl+K and the header button open one palette with HL7 and shell commands', async ({ page }) => {
  await openIDE(page, '/hl7');
  await expect(page.getByTestId('code-editor').locator('.cm-content')).toContainText('MSH|');
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());

  await page.keyboard.press('ControlOrMeta+k');
  const palette = page.getByRole('dialog', { name: 'Commands', exact: true });
  await expect(palette).toBeVisible();
  await expect(page.getByRole('dialog')).toHaveCount(1);
  await expect(palette.getByRole('textbox', { name: 'Search commands' })).toBeFocused();
  await expect(palette.getByRole('option', { name: /Preview \(parse\)/ })).toBeVisible();
  await expect(palette.getByRole('option', { name: /Go to Operator/ })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).toHaveCount(0);

  await page.getByRole('button', { name: 'Open commands' }).click();
  await expect(palette).toBeVisible();
  await expect(page.getByRole('dialog')).toHaveCount(1);
  await expect(palette.getByRole('option', { name: /Preview \(parse\)/ })).toBeVisible();
  await page.keyboard.press('Escape');
  await expect(page.getByRole('dialog')).toHaveCount(0);
});
