/**
 * Copilot store — manages the LLM assistant conversation state.
 *
 * Uses Svelte 4 writable/derived stores for consistency with the codebase.
 * LLM calls are simulated until the real MCP LLM tool is wired up.
 */
import { writable, derived, get } from 'svelte/store';
import { platformState } from '$lib/platform';

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
      content: 'Copilot ready. Select text and choose an action.',
      timestamp: Date.now(),
    },
  ],
  isStreaming: false,
  currentAction: null,
  context: {},
  error: null,
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
// Simulated streaming responses
// ---------------------------------------------------------------------------

const SIMULATED_RESPONSES: Record<CopilotAction, (input: string) => string> = {
  explain: (input: string) => {
    const trimmed = input.slice(0, 60).replace(/\n/g, ' ');
    return (
      `**HL7 Segment Analysis**\n\n` +
      `The input \`${trimmed}...\` represents a standard HL7 v2 message segment commonly used ` +
      `in healthcare integration workflows. This segment carries patient demographic data, ` +
      `clinical identifiers, and routing metadata that downstream FHIR transforms depend on.\n\n` +
      `**Key Fields:**\n` +
      `- **Field 1 (Set ID):** Sequential counter for repeating segments\n` +
      `- **Field 3 (Patient Identifier):** Maps to \`Patient.identifier\` in FHIR R4\n` +
      `- **Field 5 (Patient Name):** Decomposes into \`HumanName.family\`, \`.given\`, and \`.prefix\`\n\n` +
      `When mapping this segment, pay attention to the encoding characters and field separators. ` +
      `Misaligned delimiters are the most common source of parse failures in production HL7 feeds.`
    );
  },

  suggest: (_input: string) => {
    return (
      `**Terminology Mapping Suggestions**\n\n` +
      `| Source Code | Target System | Target Code | Display | Confidence |\n` +
      `|------------|---------------|-------------|---------|------------|\n` +
      `| \`OBX-3\` | LOINC | \`8867-4\` | Heart rate | **94%** |\n` +
      `| \`OBX-3\` | LOINC | \`8310-5\` | Body temperature | **87%** |\n` +
      `| \`OBX-3\` | SNOMED CT | \`364075005\` | Heart rate | **82%** |\n` +
      `| \`OBX-3\` | LOINC | \`9279-1\` | Respiratory rate | **71%** |\n\n` +
      `**Recommendation:** The top match (\`8867-4\` Heart rate) has high confidence and aligns ` +
      `with the US Core Vital Signs profile. Consider adding a \`ConceptMap\` entry for institutional ` +
      `codes that don't have a direct LOINC equivalent.`
    );
  },

  generate: (input: string) => {
    const desc = input.slice(0, 40).replace(/\n/g, ' ');
    return (
      `**Generated CEL Expression**\n\n` +
      `\`\`\`cel\n` +
      `// Filter: ${desc}\n` +
      `message.MSH.sending_facility == "MAIN_LAB"\n` +
      `  && message.PID.patient_class in ["I", "E"]\n` +
      `  && size(message.OBX) > 0\n` +
      `  && message.OBX.exists(o, o.observation_id.matches("^8[0-9]{3}"))\n` +
      `\`\`\`\n\n` +
      `**Explanation:**\n` +
      `- Filters messages from the \`MAIN_LAB\` sending facility\n` +
      `- Accepts only inpatient (\`I\`) and emergency (\`E\`) encounters\n` +
      `- Requires at least one OBX segment with an observation ID starting with \`8\`\n\n` +
      `This expression runs in the CEL evaluator before FHIR transformation. ` +
      `Test with the Dry Run panel to validate against sample messages.`
    );
  },

  review: (_input: string) => {
    return (
      `**Mapping Review**\n\n` +
      `**Overall Assessment:** Acceptable with minor issues\n\n` +
      `**Findings:**\n\n` +
      `1. **Patient Identifier Mapping** - The MRN is mapped to \`Patient.identifier\` ` +
      `with system \`urn:oid:2.16.840.1.113883\`. This is correct, but consider adding ` +
      `an \`assigner\` reference for traceability.\n\n` +
      `2. **Name Handling** - The mapping splits on \`^\` correctly, but does not handle ` +
      `the suffix component (field 5.4). Approximately 3% of production messages include ` +
      `suffixes like "Jr" or "III".\n\n` +
      `3. **Date Formatting** - The DOB mapping assumes \`YYYYMMDD\` format. Add a fallback ` +
      `for \`YYYYMMDD HHmmss\` which appears in ~12% of ADT messages.\n\n` +
      `**Suggested Actions:**\n` +
      `- [ ] Add \`assigner\` to identifier mapping\n` +
      `- [ ] Handle name suffix component\n` +
      `- [ ] Add date format fallback`
    );
  },
};

async function simulateStream(
  action: CopilotAction,
  input: string,
  _context: CopilotContext,
  signal: AbortSignal
): Promise<void> {
  const responseText = SIMULATED_RESPONSES[action](input);
  const assistantId = nextId();

  // Add empty assistant message placeholder
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
        streaming: true,
      },
    ],
  }));

  // Simulate initial thinking delay (500-1500ms)
  const thinkDelay = 500 + Math.random() * 1000;
  await delay(thinkDelay, signal);

  // Stream character by character in small chunks
  const chunkSize = 3 + Math.floor(Math.random() * 5); // 3-7 chars per tick
  let pos = 0;

  while (pos < responseText.length) {
    if (signal.aborted) {
      // Mark message as done (partial)
      copilotState.update((s) => ({
        ...s,
        messages: s.messages.map((m) =>
          m.id === assistantId ? { ...m, streaming: false } : m
        ),
        isStreaming: false,
        currentAction: null,
      }));
      return;
    }

    const end = Math.min(pos + chunkSize, responseText.length);
    const chunk = responseText.slice(0, end);
    pos = end;

    copilotState.update((s) => ({
      ...s,
      messages: s.messages.map((m) =>
        m.id === assistantId ? { ...m, content: chunk } : m
      ),
    }));

    const tickDelay = 20 + Math.random() * 20; // 20-40ms
    await delay(tickDelay, signal);
  }

  // Mark streaming complete
  copilotState.update((s) => ({
    ...s,
    messages: s.messages.map((m) =>
      m.id === assistantId ? { ...m, streaming: false } : m
    ),
    isStreaming: false,
    currentAction: null,
  }));
}

function delay(ms: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) {
      reject(new DOMException('Aborted', 'AbortError'));
      return;
    }
    const timer = setTimeout(resolve, ms);
    signal.addEventListener(
      'abort',
      () => {
        clearTimeout(timer);
        reject(new DOMException('Aborted', 'AbortError'));
      },
      { once: true }
    );
  });
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Send an action request to the copilot.
 * Creates a user message and streams back a simulated assistant response.
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
      error: 'Connect to the platform to use the Copilot',
    }));
    return;
  }

  // Cancel any existing stream
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
    timestamp: Date.now(),
  };

  copilotState.update((s) => ({
    ...s,
    messages: [...s.messages, userMessage],
    isStreaming: true,
    currentAction: action,
    error: null,
  }));

  activeAbort = new AbortController();

  try {
    await simulateStream(action, input, mergedContext, activeAbort.signal);
  } catch (err) {
    if (err instanceof DOMException && err.name === 'AbortError') {
      // Cancelled — already handled inside simulateStream
      return;
    }
    const message = err instanceof Error ? err.message : 'Unknown error';
    copilotState.update((s) => ({
      ...s,
      isStreaming: false,
      currentAction: null,
      error: message,
    }));
  } finally {
    activeAbort = null;
  }
}

/** Cancel any in-progress streaming response. */
export function cancelStream(): void {
  if (activeAbort) {
    activeAbort.abort();
    activeAbort = null;
  }
  copilotState.update((s) => ({
    ...s,
    isStreaming: false,
    currentAction: null,
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
    context: { ...s.context, ...ctx },
  }));
}
