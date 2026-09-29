/**
 * Workflow authoring honesty (`.loom/42` E-5) on the operator-bundle stack:
 * a version with nested action config survives the round trip through the
 * API, the untouched draft puts nothing in the Problems badge, and archiving
 * a definition hides it from the active inventory. Runs in the
 * `operator-bundle` Playwright project (testMatch `operator-bundle*.spec.ts`).
 */
import { expect, test, type Page } from '@playwright/test';
import { graphqlData, openIDE, selects, watchPage } from './support';

const DRAFT_STORAGE_KEY = 'fi-fhir:workflow:draft:v1';

// Synthetic workflow: the log action carries a nested map, which the old
// yamlToDraft turned into "[object Object]" and the builder's baseline hid.
function nestedWorkflowYaml(name: string): string {
  return [
    `name: ${name}`,
    'version: "1.0"',
    'routes:',
    '  - name: admits',
    '    filter:',
    '      event_type: PATIENT_ADMIT',
    '    actions:',
    '      - type: log',
    '        message: admitted',
    '        labels:',
    '          unit: 4B',
    '          shift: night',
    ''
  ].join('\n');
}

async function openDesign(page: Page) {
  await openIDE(page, '/workflows');
  await page.getByRole('tab', { name: 'Design', exact: true }).click();
  const design = page.getByRole('tabpanel', { name: 'Design' });
  await expect(design.getByRole('complementary', { name: 'Managed version' })).toBeVisible();
  return design;
}

type DefinitionsData = {
  workflowDefinitions: { id: string; name: string; status: string; latestVersion: { id: string } | null }[];
};

const DEFINITIONS = `query E5Definitions {
  workflowDefinitions(paging: { limit: 200, offset: 0 }) { id name status latestVersion { id } }
}`;

test('E5-1. a version with nested action config is saved and reloaded intact', async ({ page, request }) => {
  const watch = await watchPage(page);
  const name = `e5-nested-${Date.now()}`;
  const design = await openDesign(page);

  // Create the definition from the draft's name.
  await design.getByLabel('Workflow name', { exact: true }).fill(name);
  await page.getByTestId('workflow-create-definition').click();
  await expect(design.getByRole('complementary', { name: 'Managed version' })).toContainText(name);

  // Import the YAML into the builder: the nested key is listed, not mangled.
  await page.getByTestId('code-editor').locator('.cm-content').click();
  await page.keyboard.insertText(nestedWorkflowYaml(name));
  await page.getByRole('button', { name: 'Load into builder', exact: true }).click();
  const yamlOnly = page.getByTestId('workflow-yaml-only');
  await expect(yamlOnly).toContainText('labels');
  await expect(yamlOnly).toContainText('Route "admits", action 1 (log)');
  await expect(page.getByTestId('action-yaml-only').first()).toContainText('labels');
  await expect(page.locator('body')).not.toContainText('[object Object]');

  // Save: enabled, with no reason line, and the API answers.
  const save = page.getByTestId('workflow-save-version');
  await expect(save).toBeEnabled();
  await expect(page.getByTestId('workflow-save-blocked')).toHaveCount(0);
  const saved = page.waitForResponse((response) => selects(response.request(), 'saveWorkflowVersion'));
  await save.click();
  const body = (await (await saved).json()) as { errors?: unknown[] };
  expect(body.errors ?? [], 'saveWorkflowVersion answered without GraphQL errors').toEqual([]);

  // The stored YAML carries the nested map.
  const definitions = await graphqlData<DefinitionsData>(request, DEFINITIONS);
  const definition = definitions.workflowDefinitions.find((item) => item.name === name);
  expect(definition?.latestVersion?.id, 'the definition has a saved version').toBeTruthy();
  const version = await graphqlData<{ workflowVersion: { yaml: string } }>(
    request,
    'query E5Version($id: ID!) { workflowVersion(id: $id) { yaml } }',
    { id: definition!.latestVersion!.id }
  );
  expect(version.workflowVersion.yaml).toContain('labels:');
  expect(version.workflowVersion.yaml).toContain('unit: 4B');
  expect(version.workflowVersion.yaml).not.toContain('[object Object]');

  // Reload with no local draft, then open the version from the inventory:
  // what the builder shows now came from the server.
  await page.evaluate((key) => localStorage.removeItem(key), DRAFT_STORAGE_KEY);
  await page.reload();
  await expect(page.getByTestId('access-chip')).toContainText('Trusted network');
  await page.getByRole('tab', { name: 'Inventory', exact: true }).click();
  const inventory = page.getByRole('tabpanel', { name: 'Inventory' });
  await inventory.getByRole('table', { name: 'Managed workflows', exact: true }).getByText(name, { exact: true }).click();
  await inventory.getByRole('button', { name: 'Open in Design', exact: true }).click();

  await expect(page.getByTestId('workflow-yaml-only')).toContainText('labels');
  await expect(page.getByTestId('workflow-yaml-only')).toContainText('Route "admits", action 1 (log)');
  // Loaded and untouched: the baseline sees no change, so Save is ready.
  await expect(page.getByText(/^Unsaved changes/)).toHaveCount(0);
  await expect(page.getByTestId('workflow-save-version')).toBeEnabled();
  expect(watch.errorToasts).toEqual([]);
});

test('E5-2. the untouched draft puts nothing in the Problems badge; an edited one does', async ({ page }) => {
  const watch = await watchPage(page);
  const design = await openDesign(page);

  // Positive anchors first, then the absences.
  await expect(page.getByRole('tab', { name: /^Problems/ })).toBeVisible();
  await expect(page.getByTestId('workflow-save-blocked')).toContainText(
    'Create or open a managed workflow definition first.'
  );
  await expect(page.getByTestId('problems-badge')).toHaveCount(0);
  await expect(page.getByTestId('workflow-draft-summary')).toHaveCount(0);

  // Control: once the draft differs from the default, its problems count.
  await design.getByLabel('Workflow name', { exact: true }).fill('e5-edited');
  await expect(page.getByTestId('workflow-draft-summary')).toContainText('problems in this draft');
  await expect(page.getByTestId('problems-badge')).toHaveText(/^\s*[1-9]\d*\s*$/);
  expect(watch.errorToasts).toEqual([]);
});

test('E5-3. archiving a definition hides it from the active inventory', async ({ page, request }) => {
  const watch = await watchPage(page);
  const name = `e5-archive-${Date.now()}`;
  await graphqlData(
    request,
    'mutation E5Create($input: CreateWorkflowDefinitionInput!) { createWorkflowDefinition(input: $input) { id } }',
    { input: { name, description: 'E5 archive check' } }
  );

  await openIDE(page, '/workflows');
  await page.getByRole('tab', { name: 'Inventory', exact: true }).click();
  const inventory = page.getByRole('tabpanel', { name: 'Inventory' });
  const table = inventory.getByRole('table', { name: 'Managed workflows', exact: true });
  await table.getByText(name, { exact: true }).click();

  await page.getByTestId('workflow-definition-archive').click();
  const dialog = page.getByTestId('workflow-archive-dialog');
  await expect(dialog).toBeVisible();
  await expect(dialog).toHaveAccessibleName(`Archive ${name}?`);
  const archived = page.waitForResponse((response) => selects(response.request(), 'archiveWorkflowDefinition'));
  await dialog.getByRole('button', { name: 'Archive', exact: true }).click();
  await archived;
  await expect(dialog).toHaveCount(0);

  // Gone from Active (the default filter), counted as hidden, shown under Archived.
  await expect(table.getByText(name, { exact: true })).toHaveCount(0);
  await expect(page.getByTestId('workflow-count')).toContainText('archived hidden');
  await page.getByTestId('workflow-status-filter').selectOption('archived');
  await expect(table.getByText(name, { exact: true })).toBeVisible();
  await expect(page.getByTestId('workflow-definition-restore')).toBeVisible();

  const after = await graphqlData<DefinitionsData>(request, DEFINITIONS);
  expect(after.workflowDefinitions.find((item) => item.name === name)?.status).toBe('archived');
  expect(watch.errorToasts).toEqual([]);
});
