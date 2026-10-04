import { describe, expect, it } from 'vitest';
import { connectionLocation, connectionSelection, definitionLocation } from './connectionLocation';

describe('connection record locations', () => {
  it('round trips encoded IDs without leaking unrelated selectors', () => {
    const connection = connectionLocation('east/a & b');
    expect(connection).toBe('/connections?connection=east%2Fa+%26+b');
    expect(connectionSelection(connection.split('?')[1]!)).toEqual({ kind: 'connection', id: 'east/a & b' });
    const definition = definitionLocation('a/b', 'r&2');
    expect(definition).toBe('/connections?definition=a%2Fb&revision=r%262');
    expect(connectionSelection(definition.split('?')[1]!)).toEqual({ kind: 'definition', definitionId: 'a/b', revisionId: 'r&2' });
  });

  it.each(['?connection=', '?definition=a', '?revision=b', '?connection=a&definition=b&revision=c'])(
    'refuses an incomplete or ambiguous record link: %s', (search) => {
      expect(connectionSelection(search)?.kind).toBe('invalid');
    }
  );

  it('leaves a plain Connections location without a record selector', () => {
    expect(connectionSelection('')).toBeNull();
  });
});
