import { describe, expect, it } from 'vitest';
import { evaluateImportYaml } from './importYamlValidation';
import { draftToYaml } from './workflowYaml';
import type { WorkflowDraft } from './workflowTypes';

const validDraft: WorkflowDraft = {
  name: 'adt-routing',
  version: '1.0',
  routes: [
    {
      _key: 'r1',
      name: 'patient_admits',
      filter: { eventTypes: ['PATIENT_ADMIT'], sources: ['epic'], condition: '' },
      transforms: [],
      actions: [{ _key: 'a1', type: 'log', config: { message: 'ok' } }],
      expanded: false,
    },
  ],
};

describe('evaluateImportYaml', () => {
  it('flags empty input with a required issue and no parsed name', () => {
    expect(evaluateImportYaml('   \n ')).toEqual({
      issues: ['YAML input is required'],
      parsedName: '',
    });
  });

  it('returns no issues and the parsed name for a valid workflow YAML', () => {
    const result = evaluateImportYaml(draftToYaml(validDraft));
    expect(result.issues).toEqual([]);
    expect(result.parsedName).toBe('adt-routing');
  });

  it('surfaces structural validation issues for a parseable but invalid draft', () => {
    // A routeless workflow is structurally invalid; the route issue survives the
    // YAML round-trip (yamlToDraft backfills a default name, so the name issue
    // does not — hence we assert on the route issue specifically).
    const invalidYaml = draftToYaml({ name: 'x', version: '1.0', routes: [] });
    const result = evaluateImportYaml(invalidYaml);
    expect(result.issues).toContain('At least one route is required');
    expect(result.parsedName).not.toBe('');
  });

  it('returns a single parse-error issue for unparseable YAML', () => {
    const result = evaluateImportYaml('name: [unterminated');
    expect(result.issues).toHaveLength(1);
    expect(result.parsedName).toBe('');
  });
});
