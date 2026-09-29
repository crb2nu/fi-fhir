import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import WorkflowBuilder from './WorkflowBuilder.svelte';
import { resetWorkflowBuilderOpened, workflowDraft } from '../workflowStore';
import { workflowProblemCounts } from '$lib/ui/ide/panels/workflowProblemsStore';
import { clearDirty, isDirty } from '$lib/ui/ide/ideStore';

const mocks = vi.hoisted(() => ({
  toastError: vi.fn(),
  toastSuccess: vi.fn(),
  fetchWorkflowVersions: vi.fn(),
  fetchWorkflowVersionById: vi.fn(),
  fetchWorkflowApprovalRequests: vi.fn(),
  saveWorkflowVersion: vi.fn(),
  publishWorkflowVersion: vi.fn(),
  createWorkflowDefinition: vi.fn()
}));

vi.mock('$lib/ui/toastStore', () => ({
  toasts: { error: mocks.toastError, success: mocks.toastSuccess, info: vi.fn(), warning: vi.fn() }
}));

vi.mock('../workflowApi', () => ({
  fetchWorkflowVersions: (...args: unknown[]) => mocks.fetchWorkflowVersions(...args),
  fetchWorkflowVersionById: (...args: unknown[]) => mocks.fetchWorkflowVersionById(...args),
  fetchWorkflowApprovalRequests: (...args: unknown[]) => mocks.fetchWorkflowApprovalRequests(...args),
  saveWorkflowVersion: (...args: unknown[]) => mocks.saveWorkflowVersion(...args),
  publishWorkflowVersion: (...args: unknown[]) => mocks.publishWorkflowVersion(...args),
  createWorkflowDefinition: (...args: unknown[]) => mocks.createWorkflowDefinition(...args),
  requestWorkflowApproval: vi.fn(),
  dryRunWorkflow: vi.fn(),
  generateWorkflow: vi.fn(),
  explainWorkflow: vi.fn()
}));

// A saved version whose log action carries a nested map: the value the old
// parser turned into "[object Object]".
const NESTED_YAML = `name: e5-nested
version: "1.0"
routes:
  - name: admits
    filter:
      event_type: PATIENT_ADMIT
    actions:
      - type: log
        message: admitted
        labels:
          unit: 4B
`;

const VERSION = {
  id: 'v1',
  workflowId: 'wf-1',
  versionNumber: 1,
  yaml: NESTED_YAML,
  createdBy: 'e2e',
  createdAt: '2026-09-29T10:00:00Z',
  notes: null,
  validation: { valid: true, errors: [], warnings: [], info: [] }
};

const SELECTION = {
  workflowId: 'wf-1',
  name: 'e5-nested',
  description: null,
  status: 'draft',
  versionId: 'v1',
  versionNumber: 1
};

// The first render compiles the builder and CodeMirror; under a loaded full
// suite that alone can pass vitest's 5 s default.
describe('WorkflowBuilder', { timeout: 20_000 }, () => {
  let confirmSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    vi.clearAllMocks();
    workflowDraft.reset();
    resetWorkflowBuilderOpened();
    mocks.fetchWorkflowVersions.mockResolvedValue({ workflowVersions: [VERSION] });
    mocks.fetchWorkflowVersionById.mockResolvedValue({ workflowVersion: VERSION });
    mocks.fetchWorkflowApprovalRequests.mockResolvedValue({ workflowApprovalRequests: [] });
    confirmSpy = vi.spyOn(window, 'confirm');
  });

  afterEach(() => {
    confirmSpy.mockRestore();
    workflowDraft.reset();
    clearDirty('/workflows');
  });

  it('opens on the untouched default draft without problems, errors or toasts', () => {
    render(WorkflowBuilder);

    expect(get(workflowProblemCounts).total).toBe(0);
    expect(screen.queryByTestId('workflow-draft-summary')).not.toBeInTheDocument();
    expect(screen.queryByText('Workflow name is required.')).not.toBeInTheDocument();
    // Save is disabled and says why, before any click.
    expect(screen.getByTestId('workflow-save-version')).toBeDisabled();
    expect(screen.getByTestId('workflow-save-blocked')).toHaveTextContent(
      'Save version is unavailable: Create or open a managed workflow definition first.'
    );
    expect(screen.getByTestId('workflow-create-definition')).toBeDisabled();
    expect(screen.getByTestId('workflow-create-blocked')).toHaveTextContent(
      'Enter a workflow name to create a definition with it.'
    );
    // Every other disabled control says why in a sentence too.
    expect(screen.getByTestId('workflow-preview-blocked')).toHaveTextContent(
      'Preview YAML and Dry run are unavailable: give the draft a workflow name.'
    );
    expect(screen.getByTestId('workflow-approval-blocked')).toHaveTextContent(
      'Request approval is unavailable: managed definition is not linked'
    );
    expect(screen.getByTestId('workflow-unlinked-blocked')).toHaveTextContent(
      'Load version, Refresh versions and Unlink are unavailable: no managed definition is linked.'
    );
    expect(mocks.toastError).not.toHaveBeenCalled();
  });

  it('shows field-level messages and one summary line once the draft is edited', async () => {
    render(WorkflowBuilder);
    workflowDraft.update((draft) => ({ ...draft, name: 'adt-routing' }));

    const summary = await screen.findByTestId('workflow-draft-summary');
    expect(summary).toHaveTextContent('2 problems in this draft: Route 1: route name is required, and 1 more.');
    expect(screen.getByText('Route name is required')).toBeInTheDocument();
    expect(screen.getByText('Add at least one action; a route without one is rejected.')).toBeInTheDocument();
    expect(get(workflowProblemCounts).total).toBe(2);
    expect(screen.getByTestId('workflow-create-definition')).toBeEnabled();
    expect(mocks.toastError).not.toHaveBeenCalled();
  });

  it('loads a version with nested action config intact and lists the YAML-only key', async () => {
    render(WorkflowBuilder, { props: { managedSelection: SELECTION } });

    const notice = await screen.findByTestId('workflow-yaml-only');
    expect(within(notice).getByText('labels')).toBeInTheDocument();
    expect(within(notice).getByText('Route "admits", action 1 (log)')).toBeInTheDocument();
    const action = get(workflowDraft).routes[0]!.actions[0]!;
    expect(action.yamlOnly).toEqual({ labels: { unit: '4B' } });
    expect(JSON.stringify(get(workflowDraft))).not.toContain('[object Object]');
    // Loaded and untouched: nothing unsaved, so Save is ready.
    expect(screen.queryByText(/Unsaved changes/)).not.toBeInTheDocument();
    expect(screen.getByTestId('workflow-save-version')).toBeEnabled();
  });

  it('marks the Workflows tab dirty while managed changes are unsaved', async () => {
    render(WorkflowBuilder, { props: { managedSelection: SELECTION } });
    await screen.findByTestId('workflow-yaml-only');
    expect(isDirty('/workflows')).toBe(false);

    workflowDraft.update((draft) => ({ ...draft, version: '1.1' }));
    await waitFor(() => expect(isDirty('/workflows')).toBe(true));
  });

  it('sees dropping the nested value as an unsaved change (the baseline no longer hides it)', async () => {
    render(WorkflowBuilder, { props: { managedSelection: SELECTION } });
    await screen.findByTestId('workflow-yaml-only');

    workflowDraft.update((draft) => {
      const next = structuredClone(draft);
      delete next.routes[0]!.actions[0]!.yamlOnly;
      return next;
    });

    expect(await screen.findByText(/Unsaved changes/)).toBeInTheDocument();
    expect(screen.getByTestId('workflow-publish-blocked')).toHaveTextContent(
      'Publish is unavailable: builder has unsaved managed changes.'
    );
  });

  it('saves the nested config back to the server', async () => {
    mocks.saveWorkflowVersion.mockResolvedValue({ saveWorkflowVersion: { ...VERSION, id: 'v2', versionNumber: 2 } });
    render(WorkflowBuilder, { props: { managedSelection: SELECTION } });
    await screen.findByTestId('workflow-yaml-only');

    await fireEvent.click(screen.getByTestId('workflow-save-version'));

    await waitFor(() => expect(mocks.saveWorkflowVersion).toHaveBeenCalled());
    const yaml = (mocks.saveWorkflowVersion.mock.calls[0]![0] as { yaml: string }).yaml;
    expect(yaml).toContain('labels:');
    expect(yaml).toContain('unit: 4B');
  });

  it('asks in a dialog, not window.confirm, before publishing', async () => {
    mocks.publishWorkflowVersion.mockResolvedValue({ publishWorkflowVersion: { id: 'rel-1' } });
    render(WorkflowBuilder, { props: { managedSelection: SELECTION } });
    await screen.findByTestId('workflow-yaml-only');

    await fireEvent.click(screen.getByTestId('workflow-publish-version'));

    const dialog = await screen.findByRole('alertdialog', { name: 'Publish v1 to staging?' });
    expect(confirmSpy).not.toHaveBeenCalled();
    expect(mocks.publishWorkflowVersion).not.toHaveBeenCalled();

    await fireEvent.click(within(dialog).getByRole('button', { name: 'Publish to staging' }));
    await waitFor(() =>
      expect(mocks.publishWorkflowVersion).toHaveBeenCalledWith({
        workflowId: 'wf-1',
        versionId: 'v1',
        environment: 'staging'
      })
    );
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  });

  it('cancels a publish with Escape', async () => {
    render(WorkflowBuilder, { props: { managedSelection: SELECTION } });
    await screen.findByTestId('workflow-yaml-only');

    await fireEvent.click(screen.getByTestId('workflow-publish-version'));
    const dialog = await screen.findByRole('alertdialog');
    await fireEvent.keyDown(dialog, { key: 'Escape' });

    await waitFor(() => expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument());
    expect(mocks.publishWorkflowVersion).not.toHaveBeenCalled();
  });

  it('refuses to save into an archived definition and says why', async () => {
    render(WorkflowBuilder, { props: { managedSelection: { ...SELECTION, status: 'archived' } } });
    await screen.findByTestId('workflow-yaml-only');

    expect(screen.getByTestId('workflow-save-version')).toBeDisabled();
    expect(screen.getByTestId('workflow-save-blocked')).toHaveTextContent(
      'This definition is archived. Restore it to draft in Inventory before changing it.'
    );
  });
});
