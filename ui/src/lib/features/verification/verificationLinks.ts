/**
 * Deep links into and out of Verification (/events, .loom/42 "Shared
 * contracts").
 *
 * In: `/events?receipt=<id>` opens Admissions filtered to that receipt.
 * Out: each admission links to its receipt trace on Operator, its source to
 * Connections, and its definition to Connections › Definitions. Link targets
 * render their own honest states when absent; nothing here decides that.
 */
import { resolve } from '$app/paths';
import { connectionHref, operatorReceiptHref } from '$lib/features/operator/operatorLinks';

export { connectionHref, operatorReceiptHref };

/** The server's own bound on an identifier (operator/cursor.go validToken). */
const MAX_ID_LENGTH = 256;

function hasControlCharacter(value: string): boolean {
  for (const char of value) {
    const code = char.charCodeAt(0);
    if (code < 0x20 || code === 0x7f) return true;
  }
  return false;
}

/** The receipt a `/events?receipt=` link names, or null when absent or malformed. */
export function parseVerificationDeepLink(search: string): { receiptId: string } | null {
  const value = new URLSearchParams(search).get('receipt')?.trim() ?? '';
  if (value === '' || value.length > MAX_ID_LENGTH || hasControlCharacter(value)) return null;
  return { receiptId: value };
}

/** Connections › Definitions, opened on one definition revision (lane E-1 reads it). */
export function definitionHref(definitionId: string, revisionId: string): string {
  const query = new URLSearchParams({ definition: definitionId, revision: revisionId });
  return `${resolve('/connections')}?${query.toString()}`;
}
