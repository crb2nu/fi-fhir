import type { ConnectionState } from '$lib/stores/connectionStore';

export type { ConnectionState };

/**
 * Words for the API health poll (`connectionStore`). The store reports
 * `connecting` only until its first answer, so there is no "Reconnecting":
 * after a failed check it stays `disconnected` (Offline) until one succeeds.
 */
export function connectionLabel(state: ConnectionState): string {
  if (state === 'connected') return 'Connected';
  if (state === 'connecting') return 'Connecting';
  return 'Offline';
}
