import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import { connectionState, isPolling, start, stop } from './connectionStore';

type Pending = { resolve: (value: Response) => void; signal: AbortSignal | undefined };

let pending: Pending[] = [];

function okResponse(): Response {
  return { ok: true, status: 200 } as Response;
}

beforeEach(() => {
  vi.useFakeTimers();
  pending = [];
  vi.stubGlobal(
    'fetch',
    vi.fn(
      (_url: string, init?: RequestInit) =>
        new Promise<Response>((resolve, reject) => {
          const signal = init?.signal ?? undefined;
          signal?.addEventListener('abort', () =>
            reject(new DOMException('The operation was aborted.', 'AbortError'))
          );
          pending.push({ resolve, signal });
        })
    )
  );
});

afterEach(() => {
  stop();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});

async function settle(): Promise<void> {
  await Promise.resolve();
  await Promise.resolve();
}

describe('connectionStore', () => {
  it('is idempotent: a second start() does not add a second poller', async () => {
    start();
    start();
    expect(fetch).toHaveBeenCalledTimes(1);

    pending[0]?.resolve(okResponse());
    await settle();
    vi.advanceTimersByTime(30_000);
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it('keeps one request in flight: a slow /health is neither overlapped nor aborted by another check', async () => {
    start();
    expect(fetch).toHaveBeenCalledTimes(1);

    // Something asks for a check while the first is still waiting.
    vi.advanceTimersByTime(5_000);
    document.dispatchEvent(new Event('visibilitychange'));
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(pending[0]?.signal?.aborted).toBe(false);

    pending[0]?.resolve(okResponse());
    await settle();
    expect(get(connectionState)).toBe('connected');
  });

  it('reports disconnected when a check times out', async () => {
    start();
    vi.advanceTimersByTime(10_000);
    await settle();

    expect(pending[0]?.signal?.aborted).toBe(true);
    expect(get(connectionState)).toBe('disconnected');
  });

  it('stop() ends polling and releases the in-flight request without calling it a disconnect', async () => {
    start();
    pending[0]?.resolve(okResponse());
    await settle();
    expect(get(connectionState)).toBe('connected');

    vi.advanceTimersByTime(30_000);
    expect(fetch).toHaveBeenCalledTimes(2);

    stop();
    await settle();
    expect(isPolling()).toBe(false);
    expect(pending[1]?.signal?.aborted).toBe(true);
    expect(get(connectionState)).toBe('connected');

    vi.advanceTimersByTime(120_000);
    expect(fetch).toHaveBeenCalledTimes(2);
  });

  it('skips ticks while the tab is hidden and checks once when it returns', async () => {
    start();
    pending[0]?.resolve(okResponse());
    await settle();

    const visibility = vi.spyOn(document, 'visibilityState', 'get').mockReturnValue('hidden');
    vi.advanceTimersByTime(90_000);
    expect(fetch).toHaveBeenCalledTimes(1);

    visibility.mockReturnValue('visible');
    document.dispatchEvent(new Event('visibilitychange'));
    expect(fetch).toHaveBeenCalledTimes(2);
    visibility.mockRestore();
  });
});
