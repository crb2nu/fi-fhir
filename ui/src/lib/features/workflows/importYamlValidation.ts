/**
 * Validation for the WorkflowDraftLibrary "Import YAML" editor.
 *
 * Extracted as a pure helper so the result is unit-testable and so the issues
 * surface *inline* (the existing `role="alert"` issues list under the editor)
 * rather than being duplicated in a toast — per the toast-budget policy
 * (.loom/22, B1 persistent validation inline; B4 no double-surface).
 */
import { yamlToDraft } from './workflowYaml';
import { validateWorkflowDraft } from './workflowTypes';

export type ImportYamlEvaluation = {
  /** Inline-ready issue messages; empty means the YAML is import-ready. */
  issues: string[];
  /** Parsed workflow name (or '(unnamed)'), empty when the YAML can't be parsed. */
  parsedName: string;
};

/**
 * Parse and validate the import-YAML string.
 *
 * - empty input → a single "required" issue
 * - parse failure → the parser's message as the sole issue
 * - parsed → structural validation issues from `validateWorkflowDraft`
 */
export function evaluateImportYaml(yaml: string): ImportYamlEvaluation {
  const trimmed = yaml.trim();
  if (!trimmed) {
    return { issues: ['YAML input is required'], parsedName: '' };
  }

  try {
    const draft = yamlToDraft(trimmed);
    return {
      issues: validateWorkflowDraft(draft),
      parsedName: draft.name || '(unnamed)',
    };
  } catch (err) {
    return {
      issues: [err instanceof Error ? err.message : 'Invalid YAML'],
      parsedName: '',
    };
  }
}
