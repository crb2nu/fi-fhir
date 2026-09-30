/**
 * The Statistics and Retention window: three presets and a custom range, each
 * resolved to the half-open UTC window and bucket width the server counts
 * (`operatorAdmissionStatistics`), plus the few derived numbers the views
 * print. Nothing here invents a count: every figure is the server's.
 */
import type { OperatorStatisticsBucket } from '$lib/gen/graphql';
import { localInputToISO } from '$lib/features/operator/timeWindow';

export type WindowPreset = 'hour' | 'day' | 'week' | 'custom';

export interface WindowChoice {
  preset: WindowPreset;
  /** `datetime-local` values, read only for `custom`. */
  customFrom: string;
  customTo: string;
}

export interface ResolvedWindow {
  from: string;
  to: string;
  bucket: OperatorStatisticsBucket;
}

export type WindowResult = { ok: true; window: ResolvedWindow } | { ok: false; message: string };

export const WINDOW_PRESETS: ReadonlyArray<{ value: WindowPreset; label: string }> = [
  { value: 'hour', label: 'Last hour' },
  { value: 'day', label: 'Last 24 hours' },
  { value: 'week', label: 'Last 7 days' },
  { value: 'custom', label: 'Custom' }
];

const HOUR_MS = 60 * 60 * 1000;
const DAY_MS = 24 * HOUR_MS;
/** The server refuses a window of more buckets than this (operator.MaxStatisticsBuckets). */
export const MAX_BUCKETS = 744;
/** Custom windows up to two days are counted by hour, longer ones by day. */
const HOURLY_UP_TO_MS = 2 * DAY_MS;

export function defaultWindowChoice(): WindowChoice {
  return { preset: 'day', customFrom: '', customTo: '' };
}

/** Resolves a choice against `now`, or says why the custom range cannot be counted. */
export function resolveWindow(choice: WindowChoice, now: Date = new Date()): WindowResult {
  const end = now.getTime();
  switch (choice.preset) {
    case 'hour':
      return windowOf(end - HOUR_MS, end, 'HOUR');
    case 'day':
      return windowOf(end - DAY_MS, end, 'HOUR');
    case 'week':
      return windowOf(end - 7 * DAY_MS, end, 'DAY');
    case 'custom': {
      if (!choice.customFrom.trim() || !choice.customTo.trim()) {
        return { ok: false, message: 'A custom window needs both a From and a To time.' };
      }
      const from = localInputToISO(choice.customFrom);
      const to = localInputToISO(choice.customTo);
      if (!from) return { ok: false, message: 'The From time is not a valid date and time.' };
      if (!to) return { ok: false, message: 'The To time is not a valid date and time.' };
      const start = Date.parse(from);
      const stop = Date.parse(to);
      if (stop <= start) return { ok: false, message: 'From must be earlier than To.' };
      const bucket: OperatorStatisticsBucket = stop - start <= HOURLY_UP_TO_MS ? 'HOUR' : 'DAY';
      if (bucketCount(start, stop, bucket) > MAX_BUCKETS) {
        return {
          ok: false,
          message: `That window spans more than ${MAX_BUCKETS} days; the server counts at most ${MAX_BUCKETS} buckets. Narrow it.`
        };
      }
      return windowOf(start, stop, bucket);
    }
  }
}

function windowOf(start: number, stop: number, bucket: OperatorStatisticsBucket): WindowResult {
  return { ok: true, window: { from: new Date(start).toISOString(), to: new Date(stop).toISOString(), bucket } };
}

/** Buckets the server will return for [start, stop), mirroring its UTC truncation. */
export function bucketCount(start: number, stop: number, bucket: OperatorStatisticsBucket): number {
  const width = bucket === 'HOUR' ? HOUR_MS : DAY_MS;
  const first = Math.floor(start / width) * width;
  return Math.ceil((stop - first) / width);
}

export interface DeliveredRatio {
  /** Succeeded attempts over every attempt created in the window; null with none. */
  ratio: number | null;
  sentence: string;
}

/**
 * "Delivered" as the attempts ledger knows it: attempts created in the window
 * that have succeeded, over all attempts created in it. Queued attempts count
 * as not delivered yet, and the sentence says so.
 */
export function deliveredRatio(stats: {
  queuedAttempts: number;
  succeededAttempts: number;
  failedAttempts: number;
}): DeliveredRatio {
  const total = stats.queuedAttempts + stats.succeededAttempts + stats.failedAttempts;
  if (total === 0) {
    return { ratio: null, sentence: 'No delivery attempts were created in this window.' };
  }
  const ratio = stats.succeededAttempts / total;
  const percent = Math.round(ratio * 100);
  const attempts = total === 1 ? 'delivery attempt' : 'delivery attempts';
  let sentence = `${stats.succeededAttempts} of ${total} ${attempts} created in this window succeeded (${percent}%).`;
  if (stats.queuedAttempts > 0) {
    sentence += ` ${stats.queuedAttempts} still queued ${stats.queuedAttempts === 1 ? 'counts' : 'count'} as not delivered yet.`;
  }
  return { ratio, sentence };
}

/** Formats a bucket start as its UTC label: `09-29 04:00` by hour, `2026-09-29` by day. */
export function bucketLabel(start: string, bucket: OperatorStatisticsBucket): string {
  const iso = new Date(start).toISOString();
  return bucket === 'DAY' ? iso.slice(0, 10) : `${iso.slice(5, 10)} ${iso.slice(11, 16)}`;
}

/** Describes a resolved window in one line, in UTC, as the server counted it. */
export function describeWindow(window: { from: string; to: string; bucket: OperatorStatisticsBucket }): string {
  const format = (value: string) => new Date(value).toISOString().replace('T', ' ').slice(0, 16);
  return `${format(window.from)} to ${format(window.to)} UTC, by ${window.bucket === 'DAY' ? 'day' : 'hour'}`;
}
