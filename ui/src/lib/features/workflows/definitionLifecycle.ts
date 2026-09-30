/**
 * Workflow definition lifecycle in the inventory (`.loom/42` E-5): which
 * definitions the status filter shows, and whether "Save details" (rename /
 * re-describe through `updateWorkflowDefinition`) can run.
 */

export type DefinitionFilter = 'active' | 'archived' | 'all';

export type DefinitionLike = {
  name: string;
  description?: string | null;
  status: string;
};

export function isArchivedDefinition(definition: Pick<DefinitionLike, 'status'>): boolean {
  return definition.status.toLowerCase() === 'archived';
}

export type FilteredDefinitions<T> = {
  visible: T[];
  /** Archived definitions the Active filter hides. */
  hiddenArchived: number;
  /** The sentence to show when `visible` is empty. */
  emptyMessage: string;
};

export function filterDefinitions<T extends DefinitionLike>(
  definitions: readonly T[],
  filter: DefinitionFilter
): FilteredDefinitions<T> {
  const archived = definitions.filter(isArchivedDefinition);
  const active = definitions.filter((definition) => !isArchivedDefinition(definition));
  if (filter === 'archived') {
    return { visible: archived, hiddenArchived: 0, emptyMessage: 'No archived workflow definitions.' };
  }
  if (filter === 'all') {
    return { visible: [...definitions], hiddenArchived: 0, emptyMessage: 'No managed workflows.' };
  }
  const hidden = archived.length;
  return {
    visible: active,
    hiddenArchived: hidden,
    emptyMessage:
      hidden > 0
        ? `No active workflows. ${hidden} archived definition${hidden === 1 ? ' is' : 's are'} hidden; choose Archived to see ${hidden === 1 ? 'it' : 'them'}.`
        : 'No managed workflows. Create a definition in Design.'
  };
}

/** Why "Save details" is disabled for `definition`, or null when it can run. */
export function definitionEditBlocker(
  definition: DefinitionLike,
  edit: { name: string; description: string }
): string | null {
  const name = edit.name.trim();
  if (!name) return 'A definition needs a name.';
  const unchanged = name === definition.name && edit.description.trim() === (definition.description ?? '').trim();
  if (unchanged) return 'Change the name or description first.';
  return null;
}
