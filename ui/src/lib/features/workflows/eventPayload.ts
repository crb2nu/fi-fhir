/**
 * Validation for the manual "Event JSON" payload used to trigger a workflow run.
 *
 * Extracted as a pure helper so the validation messages are unit-testable and so
 * the failure path can be surfaced *inline* (next to the field) rather than as a
 * transient toast — per the toast-budget policy (.loom/22, B1: persistent
 * validation belongs inline, not in a 4-second toast).
 */

export type EventPayloadResult =
  | { ok: true; value: Record<string, unknown> }
  | { ok: false; message: string };

/**
 * Parse and validate a raw event-JSON string.
 *
 * Returns the parsed object on success, or a human-readable message describing
 * why the payload is unusable. The message is intended for an inline field error.
 */
export function validateEventPayload(raw: string): EventPayloadResult {
  const trimmed = raw.trim();
  if (!trimmed) {
    return { ok: false, message: 'Provide an event JSON payload first' };
  }

  let parsed: unknown;
  try {
    parsed = JSON.parse(trimmed);
  } catch {
    return { ok: false, message: 'Invalid JSON payload' };
  }

  if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
    return { ok: false, message: 'Event payload must be a JSON object' };
  }

  return { ok: true, value: parsed as Record<string, unknown> };
}
