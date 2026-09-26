/**
 * Formatting for event tables: one fixed-width local timestamp so a column of
 * times lines up, and the event type enum kept as the identifier it is.
 */
import type { EventType } from '$lib/gen/graphql';

function pad(value: number): string {
  return String(value).padStart(2, '0');
}

/** `YYYY-MM-DD HH:mm:ss` in local time; the input string when unparseable. */
export function formatEventTime(value: string | null | undefined): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return (
    `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ` +
    `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
  );
}

/** `HH:mm:ss` in local time, for live tails where the date is today. */
export function formatEventClock(value: string | null | undefined): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return `${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
}

/** The event types the filters offer (the common clinical and claims set). */
export const EVENT_TYPES = [
  'PATIENT_ADMIT',
  'PATIENT_DISCHARGE',
  'PATIENT_TRANSFER',
  'PATIENT_UPDATE',
  'LAB_RESULT',
  'LAB_ORDERED',
  'APPOINTMENT_SCHEDULED',
  'APPOINTMENT_CANCELLED',
  'APPOINTMENT_NOSHOW',
  'CLAIM_SUBMITTED',
  'CLAIM_ADJUDICATED',
  'VITAL_SIGN',
  'CONDITION',
  'PROCEDURE',
  'IMMUNIZATION',
  'DOCUMENT'
] as const satisfies readonly EventType[];
