import { describe, expect, it } from 'vitest';
import { formatProfileTimestamp } from './profileFormat';

describe('formatProfileTimestamp', () => {
  it('renders a local YYYY-MM-DD HH:mm stamp', () => {
    const local = new Date(2026, 8, 26, 9, 5, 42);
    expect(formatProfileTimestamp(local.toISOString())).toBe('2026-09-26 09:05');
    expect(formatProfileTimestamp(local.toISOString(), { seconds: true })).toBe(
      '2026-09-26 09:05:42'
    );
  });

  it('shows an em dash for missing values', () => {
    expect(formatProfileTimestamp(null)).toBe('—');
    expect(formatProfileTimestamp(undefined)).toBe('—');
    expect(formatProfileTimestamp('')).toBe('—');
  });

  it('passes unparseable values through', () => {
    expect(formatProfileTimestamp('not a date')).toBe('not a date');
  });
});
