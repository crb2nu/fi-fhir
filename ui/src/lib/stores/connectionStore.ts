/**
 * Lightweight connection health store.
 *
 * Polls `/health` every 30 s and exposes a reactive `connectionState`
 * that the root layout passes to IDEShell's status bar.
 */
import { writable, derived, type Readable } from 'svelte/store';

export type ConnectionState = 'connected' | 'connecting' | 'disconnected';

const POLL_INTERVAL_MS = 30_000;
const TIMEOUT_MS = 5_000;

const _state = writable<ConnectionState>('connecting');

let timer: ReturnType<typeof setInterval> | null = null;

async function check(): Promise<void> {
  try {
    const res = await fetch('/health', {
      cache: 'no-store',
      signal: AbortSignal.timeout(TIMEOUT_MS)
    });
    _state.set(res.ok ? 'connected' : 'disconnected');
  } catch {
    _state.set('disconnected');
  }
}

export function start(): void {
  if (timer) return;
  check();
  timer = setInterval(check, POLL_INTERVAL_MS);
}

export function stop(): void {
  if (timer) {
    clearInterval(timer);
    timer = null;
  }
}

/** Reactive connection state for use in Svelte components. */
export const connectionState: Readable<ConnectionState> = derived(_state, (s) => s);
