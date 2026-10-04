export type ConnectionSelection =
  | { kind: 'connection'; id: string }
  | { kind: 'definition'; definitionId: string; revisionId: string }
  | { kind: 'invalid'; message: string };

export function connectionLocation(id: string): string {
  return `/connections?${new URLSearchParams({ connection: id })}`;
}

export function definitionLocation(definitionId: string, revisionId: string): string {
  return `/connections?${new URLSearchParams({ definition: definitionId, revision: revisionId })}`;
}

/** Ambiguous selectors must not silently open a different record than the link names. */
export function connectionSelection(search: string): ConnectionSelection | null {
  const params = new URLSearchParams(search);
  if (params.has('connection') && (params.has('definition') || params.has('revision'))) {
    return { kind: 'invalid', message: 'This link names both a connection and a definition. Open one record at a time.' };
  }
  if (params.has('connection')) {
    const id = params.get('connection')?.trim() ?? '';
    return id ? { kind: 'connection', id } : { kind: 'invalid', message: 'This link is missing its connection ID.' };
  }
  if (params.has('definition') || params.has('revision')) {
    const definitionId = params.get('definition')?.trim() ?? '';
    const revisionId = params.get('revision')?.trim() ?? '';
    return definitionId && revisionId
      ? { kind: 'definition', definitionId, revisionId }
      : { kind: 'invalid', message: 'This link needs both a definition ID and a revision ID.' };
  }
  return null;
}
