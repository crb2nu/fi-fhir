import { describe, it, expect, vi, beforeEach } from 'vitest';
import { createMcpClient } from './mcpClient';
import type { PlatformConfig } from './config';

// Mock $app/environment
vi.mock('$app/environment', () => ({ browser: true }));

const mockConfig: PlatformConfig = {
  endpoint: 'http://localhost:8080/mcp',
  token: 'test-token',
  agentId: 'test-agent',
  enabled: true,
};

const disabledConfig: PlatformConfig = {
  ...mockConfig,
  enabled: false,
};

describe('mcpClient', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });

  it('creates a no-op client when disabled', () => {
    const client = createMcpClient(disabledConfig);
    expect(client.isConnected()).toBe(false);
  });

  it('creates a functional client when enabled', () => {
    const client = createMcpClient(mockConfig);
    expect(client.isConnected()).toBe(false);
    expect(typeof client.connect).toBe('function');
    expect(typeof client.callTool).toBe('function');
  });

  it('sends JSON-RPC request with correct format on callTool', async () => {
    const mockResponse = {
      jsonrpc: '2.0',
      id: 1,
      result: { session_id: 'test-123' },
    };

    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve(mockResponse),
    }));

    const client = createMcpClient(mockConfig);
    const result = await client.callTool('agent_context', 'agent_session_start', {
      namespace: 'test',
    });

    expect(result).toEqual({ session_id: 'test-123' });
    expect(fetch).toHaveBeenCalledWith(
      'http://localhost:8080/mcp',
      expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({
          'Content-Type': 'application/json',
          'Authorization': 'Bearer test-token',
        }),
      })
    );

    const calls = (fetch as ReturnType<typeof vi.fn>).mock.calls;
    const callArgs = calls[0]?.[1] as { body: string } | undefined;
    expect(callArgs).toBeDefined();
    const body = JSON.parse(callArgs!.body);
    expect(body.method).toBe('tools/call');
    expect(body.params.name).toBe('agent_context__agent_session_start');
    expect(body.params.arguments).toEqual({ namespace: 'test' });
  });

  it('marks as connected after successful call', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({ jsonrpc: '2.0', id: 1, result: {} }),
    }));

    const client = createMcpClient(mockConfig);
    expect(client.isConnected()).toBe(false);

    await client.callTool('test', 'tool', {});
    expect(client.isConnected()).toBe(true);
  });

  it('throws on RPC error response', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: () => Promise.resolve({
        jsonrpc: '2.0',
        id: 1,
        error: { code: -32600, message: 'Invalid Request' },
      }),
    }));

    const client = createMcpClient(mockConfig);
    await expect(client.callTool('test', 'tool', {})).rejects.toThrow('MCP RPC error');
  });

  it('disconnects cleanly', () => {
    const client = createMcpClient(mockConfig);
    client.disconnect();
    expect(client.isConnected()).toBe(false);
  });
});
