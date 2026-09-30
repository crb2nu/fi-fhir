/**
 * Deep links into and out of the Operator page (.loom/42 "Shared contracts").
 *
 * In: `/operator?receipt=<id>` opens that message's trace on Messages,
 * `/operator?attempt=<id>` opens that attempt's inspector on Delivery, and
 * `/operator?definition=<id>&revision=<id>` opens that deployment's history on
 * Deployments. A target that is absent or forbidden renders the view's existing
 * honest state; nothing here decides that.
 *
 * Out: a trace's source and destination refs link to their connection, and a
 * receipt links to its admissions on Verification (/events).
 */
import { resolve } from '$app/paths';

export type OperatorTab = 'messages' | 'delivery' | 'deployments';

export type OperatorDeepLink =
  | { tab: 'messages'; receiptId: string }
  | { tab: 'delivery'; attemptId: string }
  | { tab: 'deployments'; definitionId: string; revisionId: string };

/** The server's own bound on an identifier (operator/types.go validToken). */
const MAX_ID_LENGTH = 256;

function hasControlCharacter(value: string): boolean {
  for (const char of value) {
    const code = char.charCodeAt(0);
    if (code < 0x20 || code === 0x7f) return true;
  }
  return false;
}

function identifier(params: URLSearchParams, key: string): string | null {
  const value = params.get(key)?.trim() ?? '';
  if (value === '' || value.length > MAX_ID_LENGTH || hasControlCharacter(value)) return null;
  return value;
}

/**
 * Reads the first deep link a query string names, in the order receipt,
 * attempt, definition+revision. A definition without a revision (or the
 * reverse) is not a link: the lifecycle addresses a revision, never a bare
 * definition.
 */
export function parseOperatorDeepLink(search: string): OperatorDeepLink | null {
  const params = new URLSearchParams(search);
  const receiptId = identifier(params, 'receipt');
  if (receiptId) return { tab: 'messages', receiptId };
  const attemptId = identifier(params, 'attempt');
  if (attemptId) return { tab: 'delivery', attemptId };
  const definitionId = identifier(params, 'definition');
  const revisionId = identifier(params, 'revision');
  if (definitionId && revisionId) return { tab: 'deployments', definitionId, revisionId };
  return null;
}

function withQuery(path: string, query: Record<string, string>): string {
  return `${path}?${new URLSearchParams(query).toString()}`;
}

/** The Connections page, opened on one connection (its id is the revision's artifact id). */
export function connectionHref(connectionId: string): string {
  return withQuery(resolve('/connections'), { connection: connectionId });
}

/** Verification (/events), filtered to one receipt's admissions. */
export function eventsReceiptHref(receiptId: string): string {
  return withQuery(resolve('/events'), { receipt: receiptId });
}

export function operatorReceiptHref(receiptId: string): string {
  return withQuery(resolve('/operator'), { receipt: receiptId });
}

export function operatorAttemptHref(attemptId: string): string {
  return withQuery(resolve('/operator'), { attempt: attemptId });
}

export function operatorDeploymentHref(definitionId: string, revisionId: string): string {
  return withQuery(resolve('/operator'), { definition: definitionId, revision: revisionId });
}
