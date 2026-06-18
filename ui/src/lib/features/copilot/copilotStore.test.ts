import { describe, it, expect, vi, beforeEach } from 'vitest';
import { get } from 'svelte/store';

vi.mock('./copilotDispatch', () => ({ dispatchCopilotAction: vi.fn() }));
vi.mock('$lib/graphql/client', () => ({ isErrorToasted: vi.fn(() => true) }));

import { dispatchCopilotAction } from './copilotDispatch';
import { isErrorToasted } from '$lib/graphql/client';
import { platformState } from '$lib/platform';
import { copilotState, sendAction, cancelStream, clearMessages } from './copilotStore';

const mockDispatch = dispatchCopilotAction as unknown as ReturnType<typeof vi.fn>;
const mockIsToasted = isErrorToasted as unknown as ReturnType<typeof vi.fn>;

function lastAssistant() {
  return get(copilotState).messages.filter((m) => m.role === 'assistant').at(-1);
}

beforeEach(() => {
  mockDispatch.mockReset();
  mockIsToasted.mockReset();
  mockIsToasted.mockReturnValue(true);
  clearMessages();
  platformState.update((s) => ({ ...s, connected: true }));
});

describe('sendAction', () => {
  it('dispatches the real op and renders content + model', async () => {
    mockDispatch.mockResolvedValue({ content: '**Result**', model: 'gemma4-e4b-radeonvii' });

    await sendAction('generate', 'make a workflow');

    expect(mockDispatch).toHaveBeenCalledWith('generate', 'make a workflow', expect.any(Object));
    const msg = lastAssistant();
    expect(msg?.content).toBe('**Result**');
    expect(msg?.model).toBe('gemma4-e4b-radeonvii');
    expect(msg?.streaming).toBe(false);
    expect(get(copilotState).isStreaming).toBe(false);
    // user message recorded
    expect(get(copilotState).messages.some((m) => m.role === 'user' && m.content === 'make a workflow')).toBe(true);
  });

  it('omits the model field when the op returns none', async () => {
    mockDispatch.mockResolvedValue({ content: 'no-model', model: null });
    await sendAction('generate', 'x');
    expect(lastAssistant()?.model).toBeUndefined();
  });

  it('does not dispatch when the platform is disconnected', async () => {
    platformState.update((s) => ({ ...s, connected: false }));
    await sendAction('generate', 'x');
    expect(mockDispatch).not.toHaveBeenCalled();
    expect(get(copilotState).error).toMatch(/Connect to the platform/);
  });

  it('surfaces an inline error without re-toasting when the net already toasted', async () => {
    const err = new Error('GraphQL HTTP 500');
    mockDispatch.mockRejectedValue(err);
    mockIsToasted.mockReturnValue(true); // global net already toasted

    await sendAction('review', '{"x":1}');

    expect(get(copilotState).error).toBe('GraphQL HTTP 500');
    expect(lastAssistant()?.error).toBe('GraphQL HTTP 500');
    expect(get(copilotState).isStreaming).toBe(false);
  });

  it('discards an in-flight result when cancelled', async () => {
    let resolveFn: (v: { content: string; model: string | null }) => void = () => {};
    mockDispatch.mockReturnValue(new Promise((r) => { resolveFn = r; }));

    const p = sendAction('generate', 'slow');
    // placeholder is in flight
    expect(get(copilotState).isStreaming).toBe(true);

    cancelStream();
    resolveFn({ content: 'late response', model: null });
    await p;

    expect(get(copilotState).isStreaming).toBe(false);
    // the late content must not be rendered
    expect(get(copilotState).messages.some((m) => m.content === 'late response')).toBe(false);
  });
});
