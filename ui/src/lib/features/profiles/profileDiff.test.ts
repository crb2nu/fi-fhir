import { describe, expect, it } from 'vitest';
import { collapseUnchanged, lineDiff } from './profileDiff';

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

describe('collapseUnchanged', () => {
  const same = (text: string) => ({ type: 'same' as const, text });

  it('folds unchanged runs outside the context window', () => {
    const lines = [
      same('1'),
      same('2'),
      same('3'),
      same('4'),
      { type: 'removed' as const, text: 'old' },
      { type: 'added' as const, text: 'new' },
      same('5'),
      same('6'),
      same('7'),
      same('8')
    ];
    expect(collapseUnchanged(lines, 1)).toEqual([
      { type: 'skip', count: 3 },
      same('4'),
      { type: 'removed', text: 'old' },
      { type: 'added', text: 'new' },
      same('5'),
      { type: 'skip', count: 3 }
    ]);
  });

  it('keeps a single unchanged line instead of folding it', () => {
    const lines = [same('a'), same('b'), { type: 'added' as const, text: 'x' }];
    expect(collapseUnchanged(lines, 1)).toEqual([
      same('a'),
      same('b'),
      { type: 'added', text: 'x' }
    ]);
  });

  it('folds everything when nothing changed', () => {
    expect(collapseUnchanged([same('a'), same('b'), same('c')])).toEqual([
      { type: 'skip', count: 3 }
    ]);
  });
});
