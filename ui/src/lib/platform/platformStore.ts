/**
 * Reactive platform connection state.
 * Manages the MCP client lifecycle and exposes connection status to the UI.
 */
import { writable } from 'svelte/store';
import { PLATFORM_CONFIG } from './config';
import { createMcpClient, type McpClient } from './mcpClient';
import { startOperatorSession, endOperatorSession } from './sessionManager';

export interface PlatformState {
  connected: boolean;
  connecting: boolean;
  endpoint: string | null;
  agentId: string;
  sessionId: string | null;
  error: string | null;
}

const initialState: PlatformState = {
  connected: false,
  connecting: false,
  endpoint: PLATFORM_CONFIG.enabled ? PLATFORM_CONFIG.endpoint : null,
  agentId: PLATFORM_CONFIG.agentId,
  sessionId: null,
  error: null,
};

export const platformState = writable<PlatformState>(initialState);

let client: McpClient | null = null;
let activeSessionId: string | null = null;

export function getPlatformClient(): McpClient | null {
  return client;
}

export async function initializePlatform(): Promise<void> {
  if (!PLATFORM_CONFIG.enabled) return;

  platformState.update((s) => ({ ...s, connecting: true, error: null }));

  client = createMcpClient(PLATFORM_CONFIG);

  try {
    await client.connect();

    if (client.isConnected()) {
      const sessionId = await startOperatorSession(
        client,
        `mapping-studio/${PLATFORM_CONFIG.agentId}`
      );
      activeSessionId = sessionId;

      platformState.update((s) => ({
        ...s,
        connected: true,
        connecting: false,
        sessionId,
        error: null,
      }));
    } else {
      platformState.update((s) => ({
        ...s,
        connected: false,
        connecting: false,
        error: 'Failed to connect to platform',
      }));
    }
  } catch (err) {
    const message = err instanceof Error ? err.message : 'Unknown error';
    platformState.update((s) => ({
      ...s,
      connected: false,
      connecting: false,
      error: message,
    }));
  }
}

export async function teardownPlatform(): Promise<void> {
  if (client && activeSessionId) {
    try {
      await endOperatorSession(client, activeSessionId);
    } catch {
      // Best-effort cleanup
    }
  }

  activeSessionId = null;
  client?.disconnect();
  client = null;

  platformState.set(initialState);
}
