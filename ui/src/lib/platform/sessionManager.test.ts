import { describe, it, expect, vi } from 'vitest';
import { startOperatorSession, endOperatorSession, addContextEntry } from './sessionManager';
import type { McpClient } from './mcpClient';

function createMockClient(): McpClient {
  return {
    connect: vi.fn(),
    disconnect: vi.fn(),
    callTool: vi.fn(),
    isConnected: vi.fn().mockReturnValue(true),
  };
}

describe('sessionManager', () => {
  it('startOperatorSession calls correct tool with namespace', async () => {
    const client = createMockClient();
    (client.callTool as ReturnType<typeof vi.fn>).mockResolvedValue({
      session_id: 'session-123',
    });

    const sessionId = await startOperatorSession(client, 'mapping-studio/test');

    expect(sessionId).toBe('session-123');
    expect(client.callTool).toHaveBeenCalledWith(
      'agent_context',
      'agent_session_start',
      {
        namespace: 'mapping-studio/test',
        description: 'Mapping Studio operator session',
        auto_recall: true,
      }
    );
  });

  it('startOperatorSession throws when no session_id returned', async () => {
    const client = createMockClient();
    (client.callTool as ReturnType<typeof vi.fn>).mockResolvedValue({});

    await expect(startOperatorSession(client, 'test')).rejects.toThrow(
      'No session_id returned'
    );
  });

  it('endOperatorSession calls correct tool with session id', async () => {
    const client = createMockClient();
    (client.callTool as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);

    await endOperatorSession(client, 'session-123');

    expect(client.callTool).toHaveBeenCalledWith(
      'agent_context',
      'agent_session_end',
      {
        session_id: 'session-123',
        summarize: true,
      }
    );
  });

  it('addContextEntry sends entries array', async () => {
    const client = createMockClient();
    (client.callTool as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);

    await addContextEntry(client, 'session-123', {
      entry_type: 'decision',
      title: 'Chose approach A',
      content: 'Because it is simpler',
    });

    expect(client.callTool).toHaveBeenCalledWith(
      'agent_context',
      'agent_context_add',
      {
        session_id: 'session-123',
        entries: [
          {
            entry_type: 'decision',
            title: 'Chose approach A',
            content: 'Because it is simpler',
          },
        ],
      }
    );
  });
});
