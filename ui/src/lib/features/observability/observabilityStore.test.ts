/**
 * Tests for observabilityStore presentation helpers.
 */
import { describe, it, expect } from 'vitest';
import { severityLabel, type Alert } from './observabilityStore';

describe('severityLabel', () => {
  it('returns a human-readable label for each severity', () => {
    expect(severityLabel('critical')).toBe('Critical');
    expect(severityLabel('warning')).toBe('Warning');
    expect(severityLabel('info')).toBe('Info');
  });

  it('provides a non-color text cue for every Alert severity value (WCAG 1.4.1)', () => {
    const severities: Array<Alert['severity']> = ['critical', 'warning', 'info'];
    for (const severity of severities) {
      const label = severityLabel(severity);
      expect(label.length).toBeGreaterThan(0);
      // Label must be distinct, non-empty text — not a color or class token.
      expect(label).not.toMatch(/^#|rgb|var\(/);
    }
  });

  it('falls back to "Info" for an unexpected value', () => {
    expect(severityLabel('unknown' as Alert['severity'])).toBe('Info');
  });
});
