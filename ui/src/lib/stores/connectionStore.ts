/**
 * Lightweight connection health store.
 *
 * Polls `/health` every 30 s and exposes a reactive `connectionState` that
 * the root layout passes to IDEShell's status bar.
 *
 * Ownership: the root layout is the only caller of `start()`/`stop()` — it
 * starts polling once the credential gate opens and stops it when access is
 * cleared or the layout is destroyed. `start()` is idempotent.
 *
 * At most one request is in flight: a tick that finds the previous check
 * still running is skipped instead of racing it, and a request is only ever
 * aborted by its own timeout or by `stop()` — never by the next poll. While
 * the tab is hidden, ticks are skipped (a throttled background tab would
 * otherwise time out its own checks) and one check runs when it returns.
 */
import { writable, derived, type Readable } from 'svelte/store';

export type ConnectionState = 'connected' | 'connecting' | 'disconnected';

const POLL_INTERVAL_MS = 30_000;
const TIMEOUT_MS = 10_000;

const _state = writable<ConnectionState>('connecting');

let timer: ReturnType<typeof setInterval> | null = null;
let inFlight: AbortController | null = null;

function pageHidden(): boolean {
  return typeof document !== 'undefined' && document.visibilityState === 'hidden';
}

async function check(): Promise<void> {
  if (inFlight) return;
  const controller = new AbortController();
  inFlight = controller;
  const timeout = setTimeout(() => controller.abort(), TIMEOUT_MS);
  try {
    const res = await fetch('/health', { cache: 'no-store', signal: controller.signal });
    if (inFlight === controller) _state.set(res.ok ? 'connected' : 'disconnected');
  } catch {
    // A request cancelled by stop() is not a disconnect; stop() already
    // released it, so only a still-current request reports the failure.
    if (inFlight === controller) _state.set('disconnected');
  } finally {
    clearTimeout(timeout);
    if (inFlight === controller) inFlight = null;
  }
}

function tick(): void {
  if (pageHidden()) return;
  void check();
}

function onVisibilityChange(): void {
  if (!pageHidden()) void check();
}

export function start(): void {
  if (timer) return;
  void check();
  timer = setInterval(tick, POLL_INTERVAL_MS);
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', onVisibilityChange);
  }
}

export function stop(): void {
  if (timer) {
    clearInterval(timer);
    timer = null;
  }
  if (typeof document !== 'undefined') {
    document.removeEventListener('visibilitychange', onVisibilityChange);
  }
  if (inFlight) {
    const pending = inFlight;
    inFlight = null;
    pending.abort();
  }
}

/** True while polling is running (used by tests and diagnostics). */
export function isPolling(): boolean {
  return timer !== null;
}

/** Reactive connection state for use in Svelte components. */
export const connectionState: Readable<ConnectionState> = derived(_state, (s) => s);
