/**
 * Copilot store — manages the LLM assistant conversation state.
 *
 * Uses Svelte 4 writable/derived stores for consistency with the codebase.
 * Each action dispatches a real, codegen'd GraphQL LLM operation via
 * `copilotDispatch` (Wave 2, `.loom/23` Slice 2a) — there is no simulator.
 * Availability comes from the API's `llmCapability`, never from the optional
 * loom platform connection (`.loom/36` R-B).
 * The "streaming" shell (placeholder message + spinner + cancel) is preserved
 * to signal the in-flight network call; the real formatted response replaces
 * the placeholder when it lands.
 */
import { writable, derived, get } from 'svelte/store';
import { isErrorToasted } from '$lib/graphql/client';
import { dispatchCopilotAction } from './copilotDispatch';
import { llmCapabilityState, actionBlockReason } from './llmCapabilityStore';

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
      content: "Pick an action and describe what you need. The Copilot uses this deployment's LLM.",
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

/**
 * False only when the API has definitively said its LLM cannot serve
 * (disabled or unavailable). The Copilot runs on the backend LLM, so the loom
 * platform connection has no say here; an unprobed state fails open.
 */
export const isAvailable = derived(
  llmCapabilityState,
  ($llm) => $llm.status !== 'disabled' && $llm.status !== 'unavailable'
);

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
  // Honest LLM-state gate: never dispatch an action the backend has said it
  // cannot serve (disabled/unavailable, or this action's feature row off).
  const blockReason = actionBlockReason(get(llmCapabilityState), action);
  if (blockReason) {
    copilotState.update((s) => ({ ...s, error: blockReason }));
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
