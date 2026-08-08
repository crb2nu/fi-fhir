/**
 * Validation and idempotency-key derivation for operator control actions.
 *
 * Every control action the backend exposes (replay, resubmit, discard, and the
 * lifecycle commands) requires a nonempty actor reason and — for delivery
 * recovery — a caller-owned idempotency key. Those are persistent
 * preconditions, not transient results, so a failure belongs inline next to the
 * field per the toast-budget policy (.loom/22, B1). This module is pure so the
 * dialog can render the message and disable its confirm control without a
 * round trip.
 */

/** Server-side bound on a recorded reason (`octet_length(reason) <= 1024`). */
export const MAX_REASON_LENGTH = 1024;

/** Server-side bound on an idempotency key (`octet_length(key) <= 512`). */
export const MAX_IDEMPOTENCY_KEY_LENGTH = 512;

/**
 * Returns an inline-ready message when the reason cannot be recorded, else
 * null. The backend trims before persisting, so whitespace-only input is
 * rejected here rather than becoming a confusing server error.
 */
export function validateReason(reason: string): string | null {
  const trimmed = reason.trim();
  if (trimmed.length === 0) {
    return 'A reason is required. It is recorded in the append-only audit trail.';
  }
  if (byteLength(trimmed) > MAX_REASON_LENGTH) {
    return `A reason must be ${MAX_REASON_LENGTH} bytes or fewer.`;
  }
  if (hasControlCharacter(trimmed)) {
    return 'A reason cannot contain control characters.';
  }
  return null;
}

/**
 * Returns an inline-ready message when the idempotency key is unusable, else
 * null. An empty key is valid at this layer because the dialog derives one; a
 * key the operator typed themselves still has to satisfy the server's bounds.
 */
export function validateIdempotencyKey(key: string): string | null {
  if (key.length === 0) {
    return null;
  }
  if (key.trim() !== key) {
    return 'An idempotency key cannot start or end with whitespace.';
  }
  if (byteLength(key) > MAX_IDEMPOTENCY_KEY_LENGTH) {
    return `An idempotency key must be ${MAX_IDEMPOTENCY_KEY_LENGTH} bytes or fewer.`;
  }
  if (hasControlCharacter(key)) {
    return 'An idempotency key cannot contain control characters.';
  }
  return null;
}

export interface ControlDraft {
  reason: string;
  idempotencyKey: string;
}

export interface ControlIssues {
  reason: string | null;
  idempotencyKey: string | null;
}

/** Collects every inline issue for one control dialog submission. */
export function validateControlDraft(draft: ControlDraft): ControlIssues {
  return {
    reason: validateReason(draft.reason),
    idempotencyKey: validateIdempotencyKey(draft.idempotencyKey)
  };
}

/** True when the draft can be submitted. Drives the confirm control's state. */
export function controlDraftReady(draft: ControlDraft): boolean {
  const issues = validateControlDraft(draft);
  return issues.reason === null && issues.idempotencyKey === null;
}

/**
 * Derives a stable, caller-owned idempotency key for one intent.
 *
 * The key deliberately covers the action, the target attempt, and the reason:
 * repeating the identical intent is a safe no-op on the server, while a
 * genuinely different reason produces a different key rather than silently
 * reusing a spent one (which the backend refuses with a conflict).
 */
export function deriveIdempotencyKey(
  action: string,
  attemptId: string,
  reason: string,
  nonce: string
): string {
  const parts = [action, attemptId, reason.trim(), nonce].map(sanitizeKeyPart);
  return truncateBytes(`op-${parts.join('-')}`, MAX_IDEMPOTENCY_KEY_LENGTH);
}

function sanitizeKeyPart(value: string): string {
  const cleaned = value.replace(/[^A-Za-z0-9._-]+/g, '_').replace(/^_+|_+$/g, '');
  return cleaned.length > 0 ? cleaned : 'x';
}

function truncateBytes(value: string, maxBytes: number): string {
  if (byteLength(value) <= maxBytes) {
    return value;
  }
  let candidate = value;
  while (candidate.length > 0 && byteLength(candidate) > maxBytes) {
    candidate = candidate.slice(0, -1);
  }
  return candidate;
}

function byteLength(value: string): number {
  // TextEncoder is available in every supported browser and in jsdom.
  return new TextEncoder().encode(value).length;
}

function hasControlCharacter(value: string): boolean {
  for (const character of value) {
    const code = character.codePointAt(0) ?? 0;
    if (code < 0x20 || code === 0x7f) {
      return true;
    }
  }
  return false;
}
