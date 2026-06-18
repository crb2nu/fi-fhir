/**
 * Copilot store — manages the LLM assistant conversation state.
 *
 * Uses Svelte 4 writable/derived stores for consistency with the codebase.
 * Each action dispatches a real, codegen'd GraphQL LLM operation via
 * `copilotDispatch` (Wave 2, `.loom/23` Slice 2a) — there is no simulator.
 * The "streaming" shell (placeholder message + spinner + cancel) is preserved
 * to signal the in-flight network call; the real formatted response replaces
 * the placeholder when it lands.
 */
import { writable, derived, get } from 'svelte/store';
import { platformState } from '$lib/platform';
import { isErrorToasted } from '$lib/graphql/client';
import { dispatchCopilotAction } from './copilotDispatch';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type CopilotAction = 'explain' | 'suggest' | 'generate' | 'review';

export interface CopilotContext {
  stage?: string;
  selection?: string;
  documentType?: string;
  artifactId?: string;
  metadata?: Record<string, unknown>;
}

export interface CopilotMessage {
  id: string;
  role: 'user' | 'assistant' | 'system';
  action?: CopilotAction;
  content: string;
  context?: CopilotContext;
  timestamp: number;
  streaming?: boolean;
  error?: string;
  /** Real model name when the backend op reported one (e.g. review/analyzeQuality). */
  model?: string;
}

export interface CopilotState {
  messages: CopilotMessage[];
  isStreaming: boolean;
  currentAction: CopilotAction | null;
  context: CopilotContext;
  error: string | null;
}

// ---------------------------------------------------------------------------
// Initial state
// ---------------------------------------------------------------------------

const initialState: CopilotState = {
  messages: [
    {
      id: 'system-welcome',
      role: 'system',
      content: 'Copilot ready. Pick an action and describe what you need.',
      timestamp: Date.now()
    }
  ],
  isStreaming: false,
  currentAction: null,
  context: {},
  error: null
};

// ---------------------------------------------------------------------------
// Store
// ---------------------------------------------------------------------------

export const copilotState = writable<CopilotState>(initialState);

/** True when the platform connection is active and copilot can be used. */
export const isAvailable = derived(platformState, ($ps) => $ps.connected);

// ---------------------------------------------------------------------------
// Abort controller for cancellation
// ---------------------------------------------------------------------------

let activeAbort: AbortController | null = null;

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

let idCounter = 0;
function nextId(): string {
  idCounter += 1;
  return `msg-${Date.now()}-${idCounter}`;
}

// ---------------------------------------------------------------------------
// Action runner — dispatches the real GraphQL op behind the streaming shell
// ---------------------------------------------------------------------------

async function runAction(
  action: CopilotAction,
  input: string,
  context: CopilotContext,
  signal: AbortSignal
): Promise<void> {
  const assistantId = nextId();

  // Add empty assistant placeholder (shows the streaming spinner while in flight).
  copilotState.update((s) => ({
    ...s,
    messages: [
      ...s.messages,
      {
        id: assistantId,
        role: 'assistant' as const,
        action,
        content: '',
        timestamp: Date.now(),
        streaming: true
      }
    ]
  }));

  let result;
  try {
    result = await dispatchCopilotAction(action, input, context);
  } catch (err) {
    // Cancelled mid-flight: drop the placeholder, leave no error.
    if (signal.aborted) {
      copilotState.update((s) => ({
        ...s,
        messages: s.messages.filter((m) => m.id !== assistantId),
        isStreaming: false,
        currentAction: null
      }));
      return;
    }
    // Real failure. The global net (`graphqlFetch`) already toasted network/
    // GraphQL errors and tagged them; only toast here if it did not (B4 dedup,
    // `.loom/22 §5i`). The inline message is surfaced regardless.
    const message = err instanceof Error ? err.message : 'Copilot request failed';
    copilotState.update((s) => ({
      ...s,
      messages: s.messages.map((m) =>
        m.id === assistantId
          ? { ...m, content: '', streaming: false, error: message }
          : m
      ),
      isStreaming: false,
      currentAction: null,
      error: message
    }));
    if (err instanceof Error && !isErrorToasted(err)) {
      // Pre-fetch/local throw the net never saw — re-throw so the caller toasts.
      throw err;
    }
    return;
  }

  // Cancelled after the response arrived but before we rendered it: discard.
  if (signal.aborted) {
    copilotState.update((s) => ({
      ...s,
      messages: s.messages.filter((m) => m.id !== assistantId),
      isStreaming: false,
      currentAction: null
    }));
    return;
  }

  copilotState.update((s) => ({
    ...s,
    messages: s.messages.map((m) =>
      m.id === assistantId
        ? {
            ...m,
            content: result.content,
            streaming: false,
            ...(result.model ? { model: result.model } : {})
          }
        : m
    ),
    isStreaming: false,
    currentAction: null
  }));
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Send an action request to the copilot.
 * Creates a user message and dispatches the real GraphQL op for the action,
 * streaming a placeholder while the request is in flight.
 */
export async function sendAction(
  action: CopilotAction,
  input: string,
  context?: CopilotContext
): Promise<void> {
  const ps = get(platformState);
  if (!ps.connected) {
    copilotState.update((s) => ({
      ...s,
      error: 'Connect to the platform to use the Copilot'
    }));
    return;
  }

  // Cancel any existing request.
  if (activeAbort) {
    activeAbort.abort();
    activeAbort = null;
  }

  const mergedContext = { ...get(copilotState).context, ...context };
  const userMessage: CopilotMessage = {
    id: nextId(),
    role: 'user',
    action,
    content: input,
    context: mergedContext,
    timestamp: Date.now()
  };

  copilotState.update((s) => ({
    ...s,
    messages: [...s.messages, userMessage],
    isStreaming: true,
    currentAction: action,
    error: null
  }));

  activeAbort = new AbortController();

  try {
    await runAction(action, input, mergedContext, activeAbort.signal);
  } catch (err) {
    const message = err instanceof Error ? err.message : 'Unknown error';
    copilotState.update((s) => ({
      ...s,
      isStreaming: false,
      currentAction: null,
      error: message
    }));
  } finally {
    activeAbort = null;
  }
}

/** Cancel any in-progress request. */
export function cancelStream(): void {
  if (activeAbort) {
    activeAbort.abort();
    activeAbort = null;
  }
  copilotState.update((s) => ({
    ...s,
    isStreaming: false,
    currentAction: null
  }));
}

/** Clear conversation history and reset to welcome message. */
export function clearMessages(): void {
  cancelStream();
  copilotState.set(initialState);
}

/** Update the ambient context that the copilot tracks. */
export function setContext(ctx: Partial<CopilotContext>): void {
  copilotState.update((s) => ({
    ...s,
    context: { ...s.context, ...ctx }
  }));
}
