/**
 * Browser-native MCP client using Streamable HTTP (JSON-RPC 2.0).
 *
 * Communicates with loom-core's MCP proxy endpoint using fetch().
 * Includes automatic reconnection with exponential backoff.
 */
import { browser } from '$app/environment';
import type { PlatformConfig } from './config';

export interface McpClient {
  connect(): Promise<void>;
  disconnect(): void;
  callTool(server: string, tool: string, args: Record<string, unknown>): Promise<unknown>;
  isConnected(): boolean;
}

interface JsonRpcRequest {
  jsonrpc: '2.0';
  id: number;
  method: string;
  params: Record<string, unknown>;
}

interface JsonRpcResponse {
  jsonrpc: '2.0';
  id: number;
  result?: unknown;
  error?: { code: number; message: string; data?: unknown };
}

const MAX_RETRIES = 3;
const BASE_DELAY_MS = 1000;

function noopClient(): McpClient {
  return {
    connect: async () => {},
    disconnect: () => {},
    callTool: async () => null,
    isConnected: () => false,
  };
}

export function createMcpClient(config: PlatformConfig): McpClient {
  if (!browser) return noopClient();
  if (!config.enabled) return noopClient();

  let connected = false;
  let requestId = 0;

  function nextId(): number {
    requestId += 1;
    return requestId;
  }

  function buildHeaders(): Record<string, string> {
    const headers: Record<string, string> = {
      'Content-Type': 'application/json',
    };
    if (config.token) {
      headers['Authorization'] = `Bearer ${config.token}`;
    }
    return headers;
  }

  async function rpcCall(method: string, params: Record<string, unknown>): Promise<unknown> {
    const request: JsonRpcRequest = {
      jsonrpc: '2.0',
      id: nextId(),
      method,
      params,
    };

    let lastError: Error | null = null;

    for (let attempt = 0; attempt < MAX_RETRIES; attempt++) {
      try {
        const res = await fetch(config.endpoint, {
          method: 'POST',
          headers: buildHeaders(),
          body: JSON.stringify(request),
        });

        if (!res.ok) {
          throw new Error(`MCP HTTP ${res.status}: ${res.statusText}`);
        }

        const json = (await res.json()) as JsonRpcResponse;

        if (json.error) {
          throw new Error(`MCP RPC error ${json.error.code}: ${json.error.message}`);
        }

        connected = true;
        return json.result;
      } catch (err) {
        lastError = err instanceof Error ? err : new Error(String(err));
        if (attempt < MAX_RETRIES - 1) {
          const delay = BASE_DELAY_MS * Math.pow(2, attempt);
          await new Promise((resolve) => setTimeout(resolve, delay));
        }
      }
    }

    connected = false;
    throw lastError ?? new Error('MCP call failed');
  }

  return {
    async connect(): Promise<void> {
      try {
        await rpcCall('initialize', {
          protocolVersion: '2025-03-26',
          capabilities: {},
          clientInfo: { name: config.agentId, version: '1.0.0' },
        });
        connected = true;
      } catch {
        connected = false;
      }
    },

    disconnect(): void {
      connected = false;
    },

    async callTool(server: string, tool: string, args: Record<string, unknown>): Promise<unknown> {
      return rpcCall('tools/call', {
        name: `${server}__${tool}`,
        arguments: args,
      });
    },

    isConnected(): boolean {
      return connected;
    },
  };
}
