import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fetchReceipts, replayDelivery } from './operatorApi';
import { describeOperatorFailure } from './operatorErrors';

const { toastsMock } = vi.hoisted(() => ({
  toastsMock: { error: vi.fn(), success: vi.fn(), warning: vi.fn(), info: vi.fn() }
}));

vi.mock('$lib/ui/toastStore', async (importOriginal) => {
  const actual = await importOriginal<typeof import('$lib/ui/toastStore')>();
  return { ...actual, toasts: toastsMock };
});

vi.mock('$lib/graphql/credentials', () => ({
  requireGraphQLAuthorization: vi.fn().mockResolvedValue(null)
}));

function jsonResponse(body: unknown): Response {
  return { ok: true, status: 200, json: async () => body } as Response;
}

describe('operatorApi toast budget', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('keeps a forbidden read inline only — no global toast doubles the inline guidance (B4)', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue(
        jsonResponse({ errors: [{ message: 'operator control-plane action forbidden' }] })
      )
    );

    const failure = await fetchReceipts(null, { first: 25, after: null }).catch((err: unknown) => err);

    expect(failure).toBeInstanceOf(Error);
    expect(describeOperatorFailure(failure).message).toMatch(/does not hold the operator role/);
    expect(toastsMock.error).not.toHaveBeenCalled();
  });

  it('keeps a refused control action inline only, and still toasts its success (R1)', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        jsonResponse({ errors: [{ message: 'operator control-plane action forbidden' }] })
      )
      .mockResolvedValueOnce(jsonResponse({ data: { replayDelivery: { attemptId: 'a' } } }));
    vi.stubGlobal('fetch', fetchMock);
    const input = { attemptId: 'a', reason: 'retry after outage', idempotencyKey: 'k' };

    await expect(replayDelivery(input)).rejects.toThrow(/forbidden/);
    expect(toastsMock.error).not.toHaveBeenCalled();

    await replayDelivery(input);
    expect(toastsMock.success).toHaveBeenCalledWith('Replayed the delivery attempt');
  });
});
