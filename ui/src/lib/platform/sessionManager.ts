/**
 * Agent-context session lifecycle management via MCP tool calls.
 * Wraps the loom-core agent_context MCP server tools.
 */
import type { McpClient } from './mcpClient';

export interface ContextEntry {
  entry_type: 'decision' | 'finding' | 'task' | 'note';
  title: string;
  content: string;
}

export async function startOperatorSession(
  client: McpClient,
  namespace: string
): Promise<string> {
  const result = await client.callTool('agent_context', 'agent_session_start', {
    namespace,
    description: 'Mapping Studio operator session',
    auto_recall: true,
  });

  const data = result as { session_id?: string } | null;
  if (!data?.session_id) {
    throw new Error('No session_id returned from agent_session_start');
  }

  return data.session_id;
}

export async function endOperatorSession(
  client: McpClient,
  sessionId: string
): Promise<void> {
  await client.callTool('agent_context', 'agent_session_end', {
    session_id: sessionId,
    summarize: true,
  });
}

export async function addContextEntry(
  client: McpClient,
  sessionId: string,
  entry: ContextEntry
): Promise<void> {
  await client.callTool('agent_context', 'agent_context_add', {
    session_id: sessionId,
    entries: [entry],
  });
}
