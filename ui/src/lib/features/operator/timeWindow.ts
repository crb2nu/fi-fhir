/**
 * The from/to window the receipt and attempt stores filter on
 * (`recorded_at >= from AND recorded_at <= to`). The inputs are the browser's
 * `datetime-local` controls, read in the operator's local time zone and sent as
 * UTC instants.
 */

export interface TimeWindow {
  from: string | null;
  to: string | null;
}

export type TimeWindowResult = { ok: true; window: TimeWindow } | { ok: false; message: string };

/** A `datetime-local` value as an ISO instant; null when empty or unparsable. */
export function localInputToISO(value: string): string | null {
  const trimmed = value.trim();
  if (!trimmed) return null;
  const parsed = new Date(trimmed);
  return Number.isNaN(parsed.getTime()) ? null : parsed.toISOString();
}

/** Validates the pair and returns the window, or the sentence that says why not. */
export function readTimeWindow(fromInput: string, toInput: string): TimeWindowResult {
  const from = localInputToISO(fromInput);
  const to = localInputToISO(toInput);
  if (fromInput.trim() && !from) return { ok: false, message: 'The From time is not a valid date and time.' };
  if (toInput.trim() && !to) return { ok: false, message: 'The To time is not a valid date and time.' };
  if (from && to && from > to) return { ok: false, message: 'From must be earlier than To.' };
  return { ok: true, window: { from, to } };
}
