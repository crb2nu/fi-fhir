import { describe, expect, it } from 'vitest';
import { customEventsJsonError } from './dryRunValidation';

describe('customEventsJsonError', () => {
  it('returns null when the source is not custom (even if the JSON is invalid)', () => {
    expect(customEventsJsonError('presets', '{bad')).toBeNull();
    expect(customEventsJsonError('debug', '')).toBeNull();
    expect(customEventsJsonError('recent', 'not json')).toBeNull();
  });

  it('returns null for an empty / whitespace custom payload (nothing to validate yet)', () => {
    expect(customEventsJsonError('custom', '')).toBeNull();
    expect(customEventsJsonError('custom', '   \n ')).toBeNull();
  });

  it('flags unparseable custom JSON with an inline-ready message', () => {
    expect(customEventsJsonError('custom', '{bad')).toBe(
      'Invalid JSON for custom events',
    );
    expect(customEventsJsonError('custom', '[{"type":"X"}')).toBe(
      'Invalid JSON for custom events',
    );
  });

  it('accepts a valid custom JSON array', () => {
    expect(
      customEventsJsonError('custom', '[{"type":"PATIENT_ADMIT","source":"epic"}]'),
    ).toBeNull();
  });

  it('accepts a valid bare JSON object (the panel wraps it into an array)', () => {
    expect(customEventsJsonError('custom', '{"type":"PATIENT_ADMIT"}')).toBeNull();
  });
});
