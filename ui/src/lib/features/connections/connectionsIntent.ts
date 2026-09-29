/**
 * Command-palette requests for the Connections page ("New source connection",
 * "New destination connection", "Engine properties"). The shell's palette
 * records one and navigates; the page takes it once mounted. A shared
 * `svelte/store` singleton, as the IDE keeps no URL state.
 */
import { get, writable } from 'svelte/store';

export type ConnectionsView = 'sources' | 'destinations' | 'definitions' | 'engine';

export interface ConnectionsIntent {
  view: ConnectionsView;
  /** Open the New menu of that view's direction. */
  openNew?: boolean | undefined;
}

export const connectionsIntent = writable<ConnectionsIntent | null>(null);

export function requestConnectionsView(intent: ConnectionsIntent): void {
  connectionsIntent.set(intent);
}

/** Returns the pending request, if any, and clears it. */
export function takeConnectionsIntent(): ConnectionsIntent | null {
  const intent = get(connectionsIntent);
  if (intent) connectionsIntent.set(null);
  return intent;
}
