import { describe, expect, it } from 'vitest';
import { childAttempts, resubmitChains } from './resubmitChain';

describe('resubmitChains', () => {
  it('returns nothing for attempts that were never resubmitted', () => {
    expect(resubmitChains([{ attemptId: 'a' }, { attemptId: 'b', parentAttemptId: null }])).toEqual([]);
  });

  it('orders a resubmit chain root first, across generations', () => {
    const attempts = [
      { attemptId: 'grandchild', parentAttemptId: 'child' },
      { attemptId: 'root' },
      { attemptId: 'child', parentAttemptId: 'root' },
      { attemptId: 'other' }
    ];
    expect(resubmitChains(attempts)).toEqual([['root', 'child', 'grandchild']]);
    expect(childAttempts(attempts, 'root')).toEqual(['child']);
  });

  it('keeps a fork as one chain per branch and never invents a missing parent', () => {
    const attempts = [
      { attemptId: 'root' },
      { attemptId: 'left', parentAttemptId: 'root' },
      { attemptId: 'right', parentAttemptId: 'root' },
      { attemptId: 'orphan', parentAttemptId: 'not-in-this-trace' }
    ];
    expect(resubmitChains(attempts)).toEqual([
      ['root', 'left'],
      ['root', 'right']
    ]);
  });
});
