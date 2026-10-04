import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import WorkflowsPage from './WorkflowsPage.svelte';
import { workflowDraft } from './workflowStore';
import { clearDraftState, hasDraftsToLose } from '$lib/ui/ide/ideStore';

const mocks = vi.hoisted(() => ({ versions: vi.fn(), version: vi.fn() }));
vi.mock('./components/WorkflowList.svelte', async () => ({
  default: (await import('./__fixtures__/ManagedInventoryHarness.svelte')).default
}));
vi.mock('./components/WorkflowMonitor.svelte', async () => ({
  default: (await import('./__fixtures__/VerificationHarness.svelte')).default
}));
vi.mock('./workflowApi', () => ({
  fetchWorkflowVersions: mocks.versions,
  fetchWorkflowVersionById: mocks.version,
  fetchWorkflowApprovalRequests: vi.fn(),
  createWorkflowDefinition: vi.fn(), saveWorkflowVersion: vi.fn(), publishWorkflowVersion: vi.fn(),
  requestWorkflowApproval: vi.fn(), dryRunWorkflow: vi.fn(), generateWorkflow: vi.fn(), explainWorkflow: vi.fn()
}));

function version(workflowId: string) {
  return {
    id: `${workflowId}-v1`, workflowId, versionNumber: 1,
    yaml: `name: ${workflowId}\nversion: "1.0"\nroutes: []\n`,
    createdBy: 'test', createdAt: '2026-10-03T10:00:00Z', notes: null,
    validation: { valid: true, errors: [], warnings: [], info: [] }
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  workflowDraft.reset();
  mocks.versions.mockImplementation(async (id: string) => ({ workflowVersions: [version(id)] }));
  mocks.version.mockImplementation(async (id: string) => ({ workflowVersion: version(id.replace(/-v1$/, '')) }));
});

afterEach(() => {
  cleanup();
  workflowDraft.reset();
  clearDraftState('/workflows', 'managed-workflow');
});

async function openFromInventory(name: string) {
  await fireEvent.click(screen.getByRole('tab', { name: 'Inventory' }));
  await fireEvent.click(screen.getByRole('button', { name }));
}

describe('WorkflowsPage managed draft retention', { timeout: 20_000 }, () => {
  it('keeps the builder, unsaved changes and local notes through Inventory and Verification', async () => {
    render(WorkflowsPage);
    await openFromInventory('Design workflow A');
    const nameInput = await screen.findByDisplayValue('workflow-a');
    workflowDraft.update((draft) => ({ ...draft, version: '1.1' }));
    await fireEvent.input(screen.getByPlaceholderText('Version or approval note'), { target: { value: 'Keep this note' } });
    await waitFor(() => expect(hasDraftsToLose('/workflows')).toBe(true));
    await fireEvent.click(screen.getByRole('tab', { name: 'Inventory' }));
    expect(nameInput).not.toBeVisible();
    expect(hasDraftsToLose('/workflows')).toBe(true);
    await fireEvent.click(screen.getByRole('tab', { name: 'Verification' }));
    expect(screen.getByText('Workflow verification')).toBeVisible();
    await fireEvent.click(screen.getByRole('tab', { name: 'Design' }));

    expect(screen.getByDisplayValue('workflow-a')).toBe(nameInput);
    expect(screen.getByDisplayValue('1.1')).toBeInTheDocument();
    expect(screen.getByPlaceholderText('Version or approval note')).toHaveValue('Keep this note');
    expect(mocks.version).toHaveBeenCalledTimes(1);
    expect(hasDraftsToLose('/workflows')).toBe(true);
  });

  it('restores the actual selection after cancel, allowing the same replacement to be chosen and confirmed later', async () => {
    render(WorkflowsPage);
    await openFromInventory('Design workflow A');
    await screen.findByDisplayValue('workflow-a');
    workflowDraft.update((draft) => ({ ...draft, version: '1.1' }));
    await waitFor(() => expect(hasDraftsToLose('/workflows')).toBe(true));
    await openFromInventory('Design workflow B');
    let dialog = await screen.findByRole('alertdialog', { name: 'Discard unsaved changes?' });
    await fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
    expect(get(workflowDraft).name).toBe('workflow-a');
    expect(get(workflowDraft).version).toBe('1.1');
    expect(mocks.version).toHaveBeenCalledTimes(1);

    await openFromInventory('Design workflow B');
    dialog = await screen.findByRole('alertdialog', { name: 'Discard unsaved changes?' });
    await fireEvent.click(within(dialog).getByRole('button', { name: 'Discard changes' }));
    await screen.findByDisplayValue('workflow-b');
    expect(get(workflowDraft).version).toBe('1.0');
    expect(hasDraftsToLose('/workflows')).toBe(false);
  });
});
