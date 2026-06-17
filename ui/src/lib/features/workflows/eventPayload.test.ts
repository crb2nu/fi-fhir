import { describe, expect, it } from 'vitest';
import { validateEventPayload } from './eventPayload';

describe('validateEventPayload', () => {
  it('rejects an empty payload with an inline-ready message', () => {
    const result = validateEventPayload('   ');
    expect(result).toEqual({
      ok: false,
      message: 'Provide an event JSON payload first',
    });
  });

  it('rejects unparseable JSON', () => {
    const result = validateEventPayload('{not json');
    expect(result).toEqual({ ok: false, message: 'Invalid JSON payload' });
  });

  it('rejects JSON that is not an object (array)', () => {
    const result = validateEventPayload('[1, 2, 3]');
    expect(result).toEqual({
      ok: false,
      message: 'Event payload must be a JSON object',
    });
  });

  it('rejects JSON that is not an object (primitive)', () => {
    expect(validateEventPayload('42')).toEqual({
      ok: false,
      message: 'Event payload must be a JSON object',
    });
    expect(validateEventPayload('null')).toEqual({
      ok: false,
      message: 'Event payload must be a JSON object',
    });
  });

  it('accepts a valid JSON object and returns the parsed value', () => {
    const result = validateEventPayload('{"type":"PATIENT_ADMIT","source":"ui"}');
    expect(result).toEqual({
      ok: true,
      value: { type: 'PATIENT_ADMIT', source: 'ui' },
    });
  });

  it('trims surrounding whitespace before parsing', () => {
    const result = validateEventPayload('\n  {"a":1}  \n');
    expect(result).toEqual({ ok: true, value: { a: 1 } });
  });
});
