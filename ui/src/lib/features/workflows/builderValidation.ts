/**
 * The workflow builder's validation model (`.loom/42` E-5, `.loom/22` B1/B2).
 *
 * Every precondition the builder used to announce with a toast after a click
 * is computed here instead, from state, so the page can disable the control
 * and say why in a sentence next to it, before anyone clicks. Pure functions
 * of a {@link BuilderState} snapshot: the component builds the snapshot, these
 * decide, and the tests pin each sentence.
 */
import { collectWorkflowDraftIssues, type WorkflowDraft, type WorkflowDraftIssue } from './workflowTypes';
import { isEmptyDefaultDraft } from './workflowStore';

export type BuilderVersion = {
  id: string;
  versionNumber: number;
  validation: { valid: boolean };
};

export type BuilderApproval = { status: string };

export type BuilderState = {
  draft: WorkflowDraft;
  linkedWorkflowId: string;
  linkedWorkflowName: string;
  /** The linked definition's status (`draft`, `archived`), when known. */
  linkedStatus: string | null;
  versions: readonly BuilderVersion[];
  selectedVersionId: string;
  hasUnsavedManagedChanges: boolean;
  publishEnvironment: string;
  /** Approval requests for the selected version in the publish environment. */
  approvals: readonly BuilderApproval[];
  compareFromVersionId: string;
  compareToVersionId: string;
};

const NOT_LINKED = 'Create or open a managed workflow definition first.';
const ARCHIVED = 'This definition is archived. Restore it to draft in Inventory before changing it.';

function isArchived(state: BuilderState): boolean {
  return (state.linkedStatus ?? '').toLowerCase() === 'archived';
}

function selectedVersion(state: BuilderState): BuilderVersion | null {
  return state.versions.find((version) => version.id === state.selectedVersionId) ?? null;
}

function plural(n: number, word: string): string {
  return `${n} ${word}${n === 1 ? '' : 's'}`;
}

/**
 * The draft counts as work (its problems are shown) once it differs from the
 * empty default the builder starts with. The untouched default is nobody's
 * work yet, so it shows no errors and puts nothing in the Problems badge.
 */
export function draftIsLive(draft: WorkflowDraft): boolean {
  return !isEmptyDefaultDraft(draft);
}

/** Structural problems of the draft, empty while the draft is the default. */
export function liveDraftIssues(draft: WorkflowDraft): WorkflowDraftIssue[] {
  return draftIsLive(draft) ? collectWorkflowDraftIssues(draft) : [];
}

/** Issues grouped by the route they belong to (route-level and below). */
export function issuesByRoute(issues: readonly WorkflowDraftIssue[]): Record<string, WorkflowDraftIssue[]> {
  const out: Record<string, WorkflowDraftIssue[]> = {};
  for (const issue of issues) {
    if (!issue.routeKey) continue;
    (out[issue.routeKey] ??= []).push(issue);
  }
  return out;
}

/** The one-line summary above the builder; null while the draft is the default. */
export function draftSummary(draft: WorkflowDraft): string | null {
  if (!draftIsLive(draft)) return null;
  const issues = collectWorkflowDraftIssues(draft);
  if (issues.length === 0) return 'The draft is structurally valid.';
  const first = issues[0]!.text;
  const more = issues.length > 1 ? `, and ${issues.length - 1} more` : '';
  return `${plural(issues.length, 'problem')} in this draft: ${first}${more}.`;
}

/** The message under the Workflow name field. */
export function nameFieldError(state: BuilderState): string | null {
  const name = state.draft.name.trim();
  if (!name) return draftIsLive(state.draft) ? 'Workflow name is required.' : null;
  if (state.linkedWorkflowName && name !== state.linkedWorkflowName) {
    return `Versions of this definition must be named "${state.linkedWorkflowName}".`;
  }
  return null;
}

/** Why "Create definition" is disabled, or null when it is enabled. */
export function createBlocker(state: BuilderState): string | null {
  if (state.linkedWorkflowId) return 'A definition is already linked. Unlink it to create another.';
  if (!state.draft.name.trim()) return 'Enter a workflow name to create a definition with it.';
  return null;
}

/** Why "Save version" is disabled, or null when it is enabled. */
export function saveBlocker(state: BuilderState): string | null {
  if (!state.linkedWorkflowId) return NOT_LINKED;
  if (isArchived(state)) return ARCHIVED;
  const name = state.draft.name.trim();
  if (!name) return 'Give the draft a workflow name.';
  if (state.linkedWorkflowName && name !== state.linkedWorkflowName) {
    return `The draft is named "${name}" but the definition is "${state.linkedWorkflowName}". Use the definition's name.`;
  }
  const issues = collectWorkflowDraftIssues(state.draft);
  if (issues.length > 0) {
    return `Resolve the ${plural(issues.length, 'problem')} in this draft first.`;
  }
  return null;
}

/** Why promoting a local snapshot or imported YAML named `draftName` is refused. */
export function promoteBlocker(state: BuilderState, draftName: string): string | null {
  if (!state.linkedWorkflowId) return NOT_LINKED;
  if (isArchived(state)) return ARCHIVED;
  const name = draftName.trim();
  if (state.linkedWorkflowName && name && name !== state.linkedWorkflowName) {
    return `It is named "${name}" but the definition is "${state.linkedWorkflowName}".`;
  }
  return null;
}

/** Why "Compare" is disabled, or null when it is enabled. */
export function compareBlocker(state: BuilderState): string | null {
  if (!state.linkedWorkflowId) return NOT_LINKED;
  if (!state.compareFromVersionId || !state.compareToVersionId) return 'Select two versions to compare.';
  if (state.compareFromVersionId === state.compareToVersionId) return 'Choose two different versions to compare.';
  return null;
}

function hasApproval(state: BuilderState, status: string): boolean {
  return state.approvals.some((approval) => approval.status === status);
}

/** Everything that stops "Publish", in the order an operator fixes it. */
export function publishBlockers(state: BuilderState): string[] {
  const blockers: string[] = [];
  const version = selectedVersion(state);
  if (!state.linkedWorkflowId) blockers.push('Managed definition is not linked');
  if (isArchived(state)) blockers.push('Definition is archived');
  if (!state.selectedVersionId) blockers.push('No version is selected');
  if (version && !version.validation.valid) blockers.push('Selected version has validation errors');
  if (state.hasUnsavedManagedChanges) blockers.push('Builder has unsaved managed changes');
  if (state.publishEnvironment === 'production' && !hasApproval(state, 'approved')) {
    blockers.push('Production approval is not approved for selected version');
  }
  return blockers;
}

/** Everything that stops "Request approval". */
export function approvalBlockers(state: BuilderState): string[] {
  const blockers: string[] = [];
  const version = selectedVersion(state);
  if (!state.linkedWorkflowId) blockers.push('Managed definition is not linked');
  if (isArchived(state)) blockers.push('Definition is archived');
  if (!state.selectedVersionId) blockers.push('No version is selected');
  if (version && !version.validation.valid) blockers.push('Selected version has validation errors');
  if (state.hasUnsavedManagedChanges) blockers.push('Builder has unsaved managed changes');
  if (state.publishEnvironment !== 'production') {
    blockers.push('Set publish environment to production to request approval');
  }
  if (hasApproval(state, 'approved')) {
    blockers.push('Selected version is already approved for production');
  } else if (hasApproval(state, 'pending')) {
    blockers.push('Approval request is already pending for selected version');
  }
  return blockers;
}

export type ReadinessItem = { key: string; label: string; ready: boolean };

/** The pre-publish checklist. */
export function readinessItems(state: BuilderState): ReadinessItem[] {
  const version = selectedVersion(state);
  const production = state.publishEnvironment === 'production';
  return [
    { key: 'definition', label: 'Managed definition linked', ready: !!state.linkedWorkflowId },
    { key: 'version-selected', label: 'Version selected', ready: !!state.selectedVersionId },
    {
      key: 'draft-valid',
      label: 'Current draft is structurally valid',
      ready: collectWorkflowDraftIssues(state.draft).length === 0
    },
    { key: 'version-valid', label: 'Selected server version passed validation', ready: !!version?.validation.valid },
    {
      key: 'production-approval',
      label: production
        ? 'Production approval is approved'
        : 'Production approval not required for non-production publish',
      ready: production ? hasApproval(state, 'approved') : true
    }
  ];
}

/** A blocker list as one sentence for the line under a disabled button. */
export function blockerSentence(action: string, blockers: readonly string[]): string | null {
  if (blockers.length === 0) return null;
  const first = blockers[0]!;
  const more = blockers.length > 1 ? ` (and ${blockers.length - 1} more below)` : '';
  return `${action} is unavailable: ${first.charAt(0).toLowerCase()}${first.slice(1)}${more}.`;
}
