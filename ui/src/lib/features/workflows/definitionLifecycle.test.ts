import { describe, expect, it } from 'vitest';
import { definitionEditBlocker, filterDefinitions } from './definitionLifecycle';

const DEFS = [
  { name: 'adt-routing', description: 'ADT', status: 'draft' },
  { name: 'old-labs', description: null, status: 'archived' },
  { name: 'claims', description: null, status: 'DRAFT' }
];

describe('filterDefinitions', () => {
  it('hides archived definitions by default and counts them', () => {
    const result = filterDefinitions(DEFS, 'active');
    expect(result.visible.map((d) => d.name)).toEqual(['adt-routing', 'claims']);
    expect(result.hiddenArchived).toBe(1);
  });

  it('shows only archived definitions under Archived, and everything under All', () => {
    expect(filterDefinitions(DEFS, 'archived').visible.map((d) => d.name)).toEqual(['old-labs']);
    expect(filterDefinitions(DEFS, 'all').visible).toHaveLength(3);
  });

  it('says why the active list is empty', () => {
    expect(filterDefinitions([DEFS[1]!], 'active').emptyMessage).toBe(
      'No active workflows. 1 archived definition is hidden; choose Archived to see it.'
    );
    expect(filterDefinitions([], 'active').emptyMessage).toBe('No managed workflows. Create a definition in Design.');
    expect(filterDefinitions([DEFS[0]!], 'archived').emptyMessage).toBe('No archived workflow definitions.');
  });
});

describe('definitionEditBlocker', () => {
  it('needs a name and a change', () => {
    const def = DEFS[0]!;
    expect(definitionEditBlocker(def, { name: ' ', description: 'ADT' })).toBe('A definition needs a name.');
    expect(definitionEditBlocker(def, { name: 'adt-routing', description: 'ADT ' })).toBe(
      'Change the name or description first.'
    );
    expect(definitionEditBlocker(def, { name: 'adt-routing-v2', description: 'ADT' })).toBeNull();
    expect(definitionEditBlocker(DEFS[1]!, { name: 'old-labs', description: 'retired' })).toBeNull();
  });
});
