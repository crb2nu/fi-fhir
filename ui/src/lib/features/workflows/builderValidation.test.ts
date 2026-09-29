import { describe, expect, it } from 'vitest';
import {
  approvalBlockers,
  blockerSentence,
  compareBlocker,
  createBlocker,
  draftIsLive,
  draftSummary,
  issuesByRoute,
  liveDraftIssues,
  nameFieldError,
  previewBlocker,
  promoteBlocker,
  publishBlockers,
  readinessItems,
  saveBlocker,
  type BuilderState
} from './builderValidation';
import { createEmptyWorkflow, type WorkflowDraft } from './workflowTypes';

function validDraft(name = 'adt-routing'): WorkflowDraft {
  return {
    name,
    version: '1.0',
    routes: [
      {
        _key: 'r1',
        name: 'admits',
        filter: { eventTypes: ['PATIENT_ADMIT'], sources: [], condition: '' },
        transforms: [],
        actions: [{ _key: 'a1', type: 'log', config: { message: 'admitted' } }],
        expanded: true
      }
    ]
  };
}

function state(overrides: Partial<BuilderState> = {}): BuilderState {
  return {
    draft: validDraft(),
    linkedWorkflowId: 'wf-1',
    linkedWorkflowName: 'adt-routing',
    linkedStatus: 'draft',
    versions: [{ id: 'v1', versionNumber: 1, validation: { valid: true } }],
    selectedVersionId: 'v1',
    hasUnsavedManagedChanges: false,
    publishEnvironment: 'staging',
    approvals: [],
    compareFromVersionId: '',
    compareToVersionId: '',
    ...overrides
  };
}

describe('draft liveness', () => {
  it('treats the untouched default draft as nobody’s work', () => {
    const draft = createEmptyWorkflow();
    expect(draftIsLive(draft)).toBe(false);
    expect(liveDraftIssues(draft)).toEqual([]);
    expect(draftSummary(draft)).toBeNull();
    expect(nameFieldError(state({ draft, linkedWorkflowId: '', linkedWorkflowName: '' }))).toBeNull();
  });

  it('shows the draft’s problems once it differs from the default', () => {
    const draft = { ...createEmptyWorkflow(), name: 'adt' };
    expect(draftIsLive(draft)).toBe(true);
    const issues = liveDraftIssues(draft);
    expect(issues.map((issue) => issue.text)).toEqual([
      'Route 1: route name is required',
      'Route 1: at least one action is required'
    ]);
    expect(draftSummary(draft)).toBe('2 problems in this draft: Route 1: route name is required, and 1 more.');
  });

  it('says a valid draft is valid', () => {
    expect(draftSummary(validDraft())).toBe('The draft is structurally valid.');
  });

  it('groups issues by route and points each at its field', () => {
    const draft = validDraft();
    draft.routes[0]!.actions.push({ _key: 'a2', type: 'webhook', config: {} });
    const byRoute = issuesByRoute(liveDraftIssues(draft));
    expect(byRoute['r1']).toEqual([
      expect.objectContaining({ field: 'action.field', actionKey: 'a2', configKey: 'url', message: 'URL is required' })
    ]);
  });
});

describe('name field', () => {
  it('requires a name on a live draft', () => {
    const draft = validDraft('');
    expect(nameFieldError(state({ draft, linkedWorkflowId: '', linkedWorkflowName: '' }))).toBe(
      'Workflow name is required.'
    );
  });

  it('requires the linked definition’s name', () => {
    expect(nameFieldError(state({ draft: validDraft('other') }))).toBe(
      'Versions of this definition must be named "adt-routing".'
    );
    expect(nameFieldError(state())).toBeNull();
  });
});

describe('create definition', () => {
  it('is disabled while a definition is linked or the name is empty', () => {
    expect(createBlocker(state())).toBe('A definition is already linked. Unlink it to create another.');
    expect(createBlocker(state({ linkedWorkflowId: '', draft: validDraft('') }))).toBe(
      'Enter a workflow name to create a definition with it.'
    );
    expect(createBlocker(state({ linkedWorkflowId: '' }))).toBeNull();
  });
});

describe('save version', () => {
  it('names the first precondition that fails, in order', () => {
    expect(saveBlocker(state({ linkedWorkflowId: '' }))).toBe(
      'Create or open a managed workflow definition first.'
    );
    expect(saveBlocker(state({ linkedStatus: 'archived' }))).toBe(
      'This definition is archived. Restore it to draft in Inventory before changing it.'
    );
    expect(saveBlocker(state({ draft: validDraft('') }))).toBe('Give the draft a workflow name.');
    expect(saveBlocker(state({ draft: validDraft('other') }))).toBe(
      'The draft is named "other" but the definition is "adt-routing". Use the definition\'s name.'
    );
    const invalid = validDraft();
    invalid.routes[0]!.name = '';
    expect(saveBlocker(state({ draft: invalid }))).toBe('Resolve the 1 problem in this draft first.');
    expect(saveBlocker(state())).toBeNull();
  });
});

describe('promote a snapshot or imported YAML', () => {
  it('refuses a name that does not match and an archived definition', () => {
    expect(promoteBlocker(state(), 'other')).toBe('It is named "other" but the definition is "adt-routing".');
    expect(promoteBlocker(state({ linkedStatus: 'archived' }), 'adt-routing')).toMatch(/archived/);
    expect(promoteBlocker(state({ linkedWorkflowId: '' }), 'adt-routing')).toMatch(/Create or open/);
    expect(promoteBlocker(state(), 'adt-routing')).toBeNull();
  });
});

describe('compare', () => {
  it('needs two different versions', () => {
    expect(compareBlocker(state())).toBe('Select two versions to compare.');
    expect(compareBlocker(state({ compareFromVersionId: 'v1', compareToVersionId: 'v1' }))).toBe(
      'Choose two different versions to compare.'
    );
    expect(compareBlocker(state({ compareFromVersionId: 'v1', compareToVersionId: 'v2' }))).toBeNull();
  });
});

describe('publish and approval', () => {
  it('publishes to staging with a valid, saved, selected version', () => {
    expect(publishBlockers(state())).toEqual([]);
    expect(blockerSentence('Publish', publishBlockers(state()))).toBeNull();
  });

  it('blocks production until an approval is granted and says so in one sentence', () => {
    const blockers = publishBlockers(state({ publishEnvironment: 'production', hasUnsavedManagedChanges: true }));
    expect(blockers).toEqual([
      'Builder has unsaved managed changes',
      'Production approval is not approved for selected version'
    ]);
    expect(blockerSentence('Publish', blockers)).toBe(
      'Publish is unavailable: builder has unsaved managed changes (and 1 more below).'
    );
    expect(publishBlockers(state({ publishEnvironment: 'production', approvals: [{ status: 'approved' }] }))).toEqual(
      []
    );
  });

  it('blocks an archived definition', () => {
    expect(publishBlockers(state({ linkedStatus: 'archived' }))).toContain('Definition is archived');
    expect(approvalBlockers(state({ linkedStatus: 'archived', publishEnvironment: 'production' }))).toContain(
      'Definition is archived'
    );
  });

  it('allows one approval request per version', () => {
    expect(approvalBlockers(state({ publishEnvironment: 'production' }))).toEqual([]);
    expect(
      approvalBlockers(state({ publishEnvironment: 'production', approvals: [{ status: 'pending' }] }))
    ).toEqual(['Approval request is already pending for selected version']);
  });

  it('lists the checklist with the draft’s structural validity', () => {
    const invalid = validDraft();
    invalid.routes[0]!.actions = [];
    const items = readinessItems(state({ draft: invalid }));
    expect(items.find((item) => item.key === 'draft-valid')?.ready).toBe(false);
    expect(readinessItems(state()).every((item) => item.ready)).toBe(true);
  });
});

describe('preview and dry run', () => {
  it('say why they are unavailable', () => {
    expect(previewBlocker(createEmptyWorkflow())).toBe('give the draft a workflow name.');
    expect(previewBlocker({ ...validDraft(), routes: [] })).toBe('add at least one route.');
    const noAction = validDraft();
    noAction.routes[0]!.actions = [];
    expect(previewBlocker(noAction)).toBe('route 1 needs a name and at least one action.');
    expect(previewBlocker(validDraft())).toBeNull();
  });
});

describe('a default-shaped draft carrying YAML-only keys', () => {
  it('is live: the kept keys are someone’s work', () => {
    const draft = createEmptyWorkflow();
    expect(draftIsLive({ ...draft, yamlOnly: { description: 'kept' } })).toBe(true);
    const routeBag = createEmptyWorkflow();
    routeBag.routes[0]!.yamlOnly = { priority: 5 };
    expect(draftIsLive(routeBag)).toBe(true);
    const filterBag = createEmptyWorkflow();
    filterBag.routes[0]!.filter.yamlOnly = { tenant: 'east' };
    expect(draftIsLive(filterBag)).toBe(true);
  });
});

