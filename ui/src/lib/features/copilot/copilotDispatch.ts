/**
 * Copilot dispatch — maps the four free-text Copilot actions onto the real,
 * codegen'd GraphQL LLM operations (Wave 2, `.loom/23` Slice 2a).
 *
 * The Copilot UI is a free-text chat (one textarea, four action buttons), but
 * the backend LLM ops have structured, task-specific inputs. Each action here
 * coerces the free-text input (best-effort) into the input its op requires,
 * runs the op via `graphqlFetch`, and formats the typed result into the
 * markdown-ish string `CopilotPanel`'s `formatContent` renders.
 *
 * Mapping (operator-selected 2026-06-18, "wire all 4 best-effort"):
 *   - generate → GenerateWorkflow{description}        (clean free-text fit)
 *   - explain  → ExplainWorkflow{workflowYaml}        (input treated as workflow YAML)
 *   - suggest  → SuggestMappings{sourceCode,...}      (best-effort coercion)
 *   - review   → AnalyzeQuality{event, eventType}     (best-effort coercion; returns model)
 *
 * Only `AnalyzeQuality`/`ExtractEntities` expose a `model` field in the schema,
 * so `review` is the only action that surfaces a real model name; the others
 * return `model: null` (honest — we don't fabricate one).
 */
import { graphqlFetch } from '$lib/graphql/client';
import {
  GenerateWorkflowDocument,
  ExplainWorkflowDocument,
  SuggestMappingsDocument,
  AnalyzeQualityDocument,
  type GenerateWorkflowMutation,
  type ExplainWorkflowQuery,
  type SuggestMappingsQuery,
  type AnalyzeQualityQuery,
  type SuggestMappingsInput,
  type AnalyzeQualityInput,
  type EventType
} from '$lib/gen/graphql';
import type { CopilotAction, CopilotContext } from './copilotStore';

/** Normalized result handed back to the store: rendered markdown + optional model. */
export interface CopilotResult {
  content: string;
  /** Real model name when the op returns one (AnalyzeQuality); null otherwise. */
  model: string | null;
}

// ---------------------------------------------------------------------------
// Best-effort free-text → structured-input coercion (pure, unit-tested)
// ---------------------------------------------------------------------------

/** LOINC is the most common terminology target; used when none can be parsed. */
const DEFAULT_TARGET_SYSTEM = 'http://loinc.org';
/** Fallback source system when context/input carry none. */
const DEFAULT_SOURCE_SYSTEM = 'unknown';
/** Generic fallback when no event type can be derived from context. */
const DEFAULT_EVENT_TYPE: EventType = 'DOCUMENT';

/** Valid `EventType` enum values, for runtime validation of context hints. */
const EVENT_TYPES: ReadonlySet<string> = new Set<EventType>([
  'ALLERGY_INTOLERANCE',
  'APPOINTMENT_CANCELLED',
  'APPOINTMENT_CHECKED_IN',
  'APPOINTMENT_MODIFIED',
  'APPOINTMENT_NOSHOW',
  'APPOINTMENT_RESCHEDULED',
  'APPOINTMENT_SCHEDULED',
  'CLAIM_ADJUDICATED',
  'CLAIM_STATUS_REQUEST',
  'CLAIM_STATUS_RESPONSE',
  'CLAIM_SUBMITTED',
  'CONDITION',
  'DOCUMENT',
  'DOCUMENT_ADDENDUM',
  'DOCUMENT_EDIT',
  'DOCUMENT_ORIGINAL',
  'DOCUMENT_REPLACEMENT',
  'DOCUMENT_STATUS_CHANGE',
  'ELIGIBILITY_INQUIRY',
  'ELIGIBILITY_RESPONSE',
  'FINANCIAL_TRANSACTION',
  'IMMUNIZATION',
  'LAB_CANCELLED',
  'LAB_ORDERED',
  'LAB_RESULT',
  'MEDICATION_REQUEST',
  'PATIENT_ADMIT',
  'PATIENT_DISCHARGE',
  'PATIENT_MERGE',
  'PATIENT_TRANSFER',
  'PATIENT_UPDATE',
  'PRIOR_AUTH_REQUEST',
  'PRIOR_AUTH_RESPONSE',
  'PROCEDURE',
  'SOCIAL_HISTORY',
  'VITAL_SIGN'
]);

/** Reads a string-valued context-metadata key, or undefined when absent/blank. */
function metaString(context: CopilotContext, key: string): string | undefined {
  const v = context.metadata?.[key];
  if (typeof v === 'string' && v.trim()) return v.trim();
  return undefined;
}

/** Parses input as JSON, returning the value or undefined when it isn't JSON. */
function tryParseJson(input: string): unknown {
  try {
    return JSON.parse(input);
  } catch {
    return undefined;
  }
}

/**
 * Coerces free text into `SuggestMappingsInput`. All three required fields are
 * always populated so the op never fails validation:
 *   - sourceSystem: context hint → DEFAULT_SOURCE_SYSTEM
 *   - targetSystem: a trailing `… to <system>` phrase → context hint → LOINC
 *   - sourceCode:   the code/term before any `to <system>` clause → whole input
 */
export function buildSuggestInput(input: string, context: CopilotContext): SuggestMappingsInput {
  const text = input.trim();
  const toMatch = text.match(/\bto\s+(\S+)\s*$/i);
  const parsedTarget = toMatch ? toMatch[1] : undefined;
  const head = (toMatch ? text.slice(0, toMatch.index).trim() : text).replace(/^map\s+/i, '').trim();
  const sourceCode = head || text || '';

  return {
    sourceCode,
    sourceDisplay: text || null,
    sourceSystem: metaString(context, 'sourceSystem') ?? DEFAULT_SOURCE_SYSTEM,
    targetSystem: metaString(context, 'targetSystem') ?? parsedTarget ?? DEFAULT_TARGET_SYSTEM,
    maxCandidates: 5
  };
}

/**
 * Coerces free text into `AnalyzeQualityInput`. JSON input is passed through as
 * the event payload; anything else is wrapped as `{ raw: <text> }` so the op
 * still receives a structured object. The event type is taken from a context
 * hint when it names a valid `EventType`, else DEFAULT_EVENT_TYPE.
 */
export function buildReviewInput(input: string, context: CopilotContext): AnalyzeQualityInput {
  const parsed = tryParseJson(input.trim());
  const event = parsed && typeof parsed === 'object' ? parsed : { raw: input };
  const hint = (metaString(context, 'eventType') ?? '').toUpperCase();
  const eventType: EventType = EVENT_TYPES.has(hint) ? (hint as EventType) : DEFAULT_EVENT_TYPE;
  return { event, eventType };
}

// ---------------------------------------------------------------------------
// Result formatters (pure, unit-tested) — typed result → markdown string
// ---------------------------------------------------------------------------

function warningsBlock(warnings: readonly string[]): string {
  if (!warnings.length) return '';
  return `\n\n**Warnings:**\n${warnings.map((w) => `- ${w}`).join('\n')}`;
}

export function formatGenerate(gen: GenerateWorkflowMutation['generateWorkflow']): CopilotResult {
  const explanation = gen.explanation?.trim() ? `${gen.explanation.trim()}\n\n` : '';
  const yaml = gen.yaml?.trim() ? `\`\`\`yaml\n${gen.yaml.trim()}\n\`\`\`` : '_No workflow returned._';
  return { content: `${explanation}${yaml}${warningsBlock(gen.warnings ?? [])}`, model: null };
}

export function formatExplain(exp: ExplainWorkflowQuery['explainWorkflow']): CopilotResult {
  const parts: string[] = [];
  if (exp.summary?.trim()) parts.push(`**${exp.summary.trim()}**`);
  if (exp.description?.trim()) parts.push(exp.description.trim());
  if (exp.routeExplanations?.length) {
    const routes = exp.routeExplanations
      .map((r) => {
        const actions = r.actions?.length ? ` _(actions: ${r.actions.join(', ')})_` : '';
        return `- **${r.name}** — ${r.trigger}: ${r.description}${actions}`;
      })
      .join('\n');
    parts.push(`**Routes:**\n${routes}`);
  }
  if (exp.diagram?.trim()) parts.push(`\`\`\`mermaid\n${exp.diagram.trim()}\n\`\`\``);
  const body = parts.join('\n\n') || '_No explanation returned._';
  return { content: `${body}${warningsBlock(exp.warnings ?? [])}`, model: null };
}

export function formatSuggest(candidates: SuggestMappingsQuery['suggestMappings']): CopilotResult {
  if (!candidates.length) {
    return { content: '_No mapping candidates found for that input._', model: null };
  }
  const header = '| Code | Display | System | Confidence |\n|------|---------|--------|------------|';
  const rows = candidates
    .map((c) => `| \`${c.code}\` | ${c.display} | ${c.system} | ${(c.confidence * 100).toFixed(0)}% |`)
    .join('\n');
  const reasoning = candidates
    .filter((c) => c.reasoning?.trim())
    .map((c) => `- \`${c.code}\`: ${c.reasoning}`)
    .join('\n');
  const reasoningBlock = reasoning ? `\n\n**Reasoning:**\n${reasoning}` : '';
  return { content: `**Mapping suggestions**\n\n${header}\n${rows}${reasoningBlock}`, model: null };
}

export function formatReview(score: AnalyzeQualityQuery['analyzeQuality']): CopilotResult {
  const d = score.dimensions;
  const dims =
    '| Dimension | Score |\n|-----------|-------|\n' +
    `| Completeness | ${pct(d.completeness)} |\n` +
    `| Accuracy | ${pct(d.accuracy)} |\n` +
    `| Consistency | ${pct(d.consistency)} |\n` +
    `| Conformance | ${pct(d.conformance)} |\n` +
    `| Timeliness | ${pct(d.timeliness)} |`;
  const issues = score.issues?.length
    ? `\n\n**Issues:**\n${score.issues
        .map((i) => `- **${i.severity}** (${i.dimension})${i.field ? ` \`${i.field}\`` : ''}: ${i.description}`)
        .join('\n')}`
    : '';
  const recs = score.recommendations?.length
    ? `\n\n**Recommendations:**\n${score.recommendations
        .map((r) => `- **${r.title}**: ${r.description}`)
        .join('\n')}`
    : '';
  const content = `**Data Quality — overall ${pct(score.overallScore)}**\n\n${dims}${issues}${recs}`;
  return { content, model: score.model ?? null };
}

function pct(v: number): string {
  return `${Math.round(v * 100)}%`;
}

// ---------------------------------------------------------------------------
// Dispatch — runs the real op for an action and returns formatted markdown
// ---------------------------------------------------------------------------

/**
 * Runs the real GraphQL LLM op for `action` and returns the formatted result.
 * Errors propagate to the caller (the store), which defers to the global toast
 * net via `isErrorToasted` (toast-budget policy, `.loom/22 §5i`).
 */
export async function dispatchCopilotAction(
  action: CopilotAction,
  input: string,
  context: CopilotContext
): Promise<CopilotResult> {
  switch (action) {
    case 'generate': {
      const eventTypes = context.documentType ? [context.documentType] : null;
      const res = await graphqlFetch(GenerateWorkflowDocument, {
        input: { description: input, eventTypes, actionTypes: null }
      });
      return formatGenerate(res.generateWorkflow);
    }
    case 'explain': {
      const res = await graphqlFetch(ExplainWorkflowDocument, {
        input: { workflowYaml: input, audience: metaString(context, 'audience') ?? null }
      });
      return formatExplain(res.explainWorkflow);
    }
    case 'suggest': {
      const res = await graphqlFetch(SuggestMappingsDocument, {
        input: buildSuggestInput(input, context)
      });
      return formatSuggest(res.suggestMappings);
    }
    case 'review': {
      const res = await graphqlFetch(AnalyzeQualityDocument, {
        input: buildReviewInput(input, context)
      });
      return formatReview(res.analyzeQuality);
    }
  }
}
