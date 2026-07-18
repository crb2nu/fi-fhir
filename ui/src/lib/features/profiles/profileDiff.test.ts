import { describe, expect, it } from 'vitest';
import { lineDiff } from './profileDiff';

describe('lineDiff', () => {
  it('aligns unchanged lines after an insertion', () => {
    expect(lineDiff('a\nb\nc', 'a\ninserted\nb\nc')).toEqual([
      { type: 'same', text: 'a' },
      { type: 'added', text: 'inserted' },
      { type: 'same', text: 'b' },
      { type: 'same', text: 'c' }
    ]);
  });

  it('represents a replacement as removal and addition', () => {
    expect(lineDiff('a\nold\nc', 'a\nnew\nc')).toEqual([
      { type: 'same', text: 'a' },
      { type: 'removed', text: 'old' },
      { type: 'added', text: 'new' },
      { type: 'same', text: 'c' }
    ]);
  });
});
