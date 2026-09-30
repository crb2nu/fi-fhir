import { describe, expect, it } from 'vitest';
import {
  MAX_BUCKETS,
  bucketCount,
  bucketLabel,
  deliveredRatio,
  describeWindow,
  resolveWindow
} from './statisticsWindow';

const NOW = new Date('2026-09-29T12:34:00.000Z');

describe('resolveWindow', () => {
  it('resolves the presets against now, hourly for an hour and a day, daily for a week', () => {
    expect(resolveWindow({ preset: 'hour', customFrom: '', customTo: '' }, NOW)).toEqual({
      ok: true,
      window: { from: '2026-09-29T11:34:00.000Z', to: '2026-09-29T12:34:00.000Z', bucket: 'HOUR' }
    });
    expect(resolveWindow({ preset: 'day', customFrom: '', customTo: '' }, NOW)).toEqual({
      ok: true,
      window: { from: '2026-09-28T12:34:00.000Z', to: '2026-09-29T12:34:00.000Z', bucket: 'HOUR' }
    });
    expect(resolveWindow({ preset: 'week', customFrom: '', customTo: '' }, NOW)).toEqual({
      ok: true,
      window: { from: '2026-09-22T12:34:00.000Z', to: '2026-09-29T12:34:00.000Z', bucket: 'DAY' }
    });
  });

  it('counts a custom range by hour up to two days and by day beyond', () => {
    const short = resolveWindow({ preset: 'custom', customFrom: '2026-09-28T00:00', customTo: '2026-09-29T12:00' }, NOW);
    expect(short.ok && short.window.bucket).toBe('HOUR');
    const long = resolveWindow({ preset: 'custom', customFrom: '2026-09-01T00:00', customTo: '2026-09-29T00:00' }, NOW);
    expect(long.ok && long.window.bucket).toBe('DAY');
  });

  it('says why a custom range cannot be counted', () => {
    expect(resolveWindow({ preset: 'custom', customFrom: '', customTo: '' }, NOW)).toEqual({
      ok: false,
      message: 'A custom window needs both a From and a To time.'
    });
    expect(resolveWindow({ preset: 'custom', customFrom: '2026-09-29T10:00', customTo: '2026-09-29T09:00' }, NOW)).toEqual({
      ok: false,
      message: 'From must be earlier than To.'
    });
    const huge = resolveWindow({ preset: 'custom', customFrom: '2020-01-01T00:00', customTo: '2026-09-29T00:00' }, NOW);
    expect(huge.ok).toBe(false);
    expect(!huge.ok && huge.message).toContain(`${MAX_BUCKETS} buckets`);
  });
});

describe('bucketCount', () => {
  it('mirrors the server: the first bucket starts at the truncated From', () => {
    const start = Date.parse('2026-09-29T10:30:00Z');
    expect(bucketCount(start, Date.parse('2026-09-29T12:00:00Z'), 'HOUR')).toBe(2);
    expect(bucketCount(start, Date.parse('2026-09-29T12:00:01Z'), 'HOUR')).toBe(3);
    expect(bucketCount(start, Date.parse('2026-10-01T00:00:00Z'), 'DAY')).toBe(2);
  });
});

describe('deliveredRatio', () => {
  it('is succeeded over every attempt created, and names queued attempts as not delivered yet', () => {
    expect(deliveredRatio({ queuedAttempts: 2, succeededAttempts: 1, failedAttempts: 1 })).toEqual({
      ratio: 0.25,
      sentence:
        '1 of 4 delivery attempts created in this window succeeded (25%). 2 still queued count as not delivered yet.'
    });
    expect(deliveredRatio({ queuedAttempts: 0, succeededAttempts: 1, failedAttempts: 0 }).sentence).toBe(
      '1 of 1 delivery attempt created in this window succeeded (100%).'
    );
  });

  it('claims no ratio when nothing was attempted', () => {
    expect(deliveredRatio({ queuedAttempts: 0, succeededAttempts: 0, failedAttempts: 0 })).toEqual({
      ratio: null,
      sentence: 'No delivery attempts were created in this window.'
    });
  });
});

describe('labels', () => {
  it('prints bucket starts and the window in UTC', () => {
    expect(bucketLabel('2026-09-29T04:00:00Z', 'HOUR')).toBe('09-29 04:00');
    expect(bucketLabel('2026-09-29T00:00:00Z', 'DAY')).toBe('2026-09-29');
    expect(describeWindow({ from: '2026-09-28T12:34:00Z', to: '2026-09-29T12:34:00Z', bucket: 'HOUR' })).toBe(
      '2026-09-28 12:34 to 2026-09-29 12:34 UTC, by hour'
    );
  });
});
