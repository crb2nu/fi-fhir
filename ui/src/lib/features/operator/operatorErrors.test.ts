import { describe, expect, it } from 'vitest';
import { describeOperatorFailure } from './operatorErrors';

describe('describeOperatorFailure', () => {
  it('turns an optimistic-concurrency conflict into a reload-and-retry instruction', () => {
    const failure = describeOperatorFailure(
      new Error('integration deployment version conflict')
    );
    expect(failure.staleView).toBe(true);
    expect(failure.message).toMatch(/another operator changed this deployment/i);
    // The guidance must never suggest an automatic retry: the whole point of an
    // expected version is that the operator re-reads before re-deciding.
    expect(failure.message).not.toMatch(/automatically|retrying for you/i);
  });

  it('explains a spent idempotency key without marking the view stale', () => {
    const failure = describeOperatorFailure(
      new Error('operator operation idempotency conflict')
    );
    expect(failure.staleView).toBe(false);
    expect(failure.message).toMatch(/already used for a different operation/i);
  });

  it('flags a resolved dead letter as a stale view', () => {
    const failure = describeOperatorFailure(new Error('delivery attempt is not dead-lettered'));
    expect(failure.staleView).toBe(true);
    expect(failure.message).toMatch(/no longer an open dead letter/i);
  });

  it('explains a missing operator role as a permissions problem', () => {
    const failure = describeOperatorFailure(
      new Error('operator control-plane action forbidden')
    );
    expect(failure.message).toMatch(/operator role/i);
    expect(failure.staleView).toBe(false);
  });

  it('explains an unconfigured control plane', () => {
    expect(describeOperatorFailure(new Error('operator control plane unavailable')).message).toMatch(
      /not enabled on this deployment/i
    );
  });

  it('passes through an unrecognized message rather than inventing one', () => {
    const failure = describeOperatorFailure(new Error('Network error: fetch failed'));
    expect(failure.message).toBe('Network error: fetch failed');
    expect(failure.staleView).toBe(false);
  });

  it('falls back for non-Error and empty values', () => {
    expect(describeOperatorFailure(undefined).message).toMatch(/could not complete/i);
    expect(describeOperatorFailure(new Error('')).message).toMatch(/could not complete/i);
    expect(describeOperatorFailure('operator control-plane record not found').message).toMatch(
      /not available in this tenant/i
    );
  });
});
