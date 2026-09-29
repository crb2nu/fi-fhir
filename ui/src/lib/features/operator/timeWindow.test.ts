import { describe, expect, it } from 'vitest';
import { localInputToEndISO, localInputToISO, readTimeWindow } from './timeWindow';

describe('timeWindow', () => {
  it('turns datetime-local values into ISO instants and ignores empty ones', () => {
    expect(localInputToISO('')).toBeNull();
    expect(localInputToISO('   ')).toBeNull();
    expect(localInputToISO('garbage')).toBeNull();
    const iso = localInputToISO('2026-09-29T04:11');
    expect(iso).toBe(new Date('2026-09-29T04:11').toISOString());
  });

  it('makes a minute-precision To inclusive of that whole minute', () => {
    expect(localInputToEndISO('2026-09-29T04:11')).toBe(
      new Date(new Date('2026-09-29T04:11').getTime() + 59_999).toISOString()
    );
    expect(localInputToEndISO('2026-09-29T04:11:30')).toBe(new Date('2026-09-29T04:11:30').toISOString());
    expect(localInputToEndISO('')).toBeNull();
    // The same minute on both ends is a valid one-minute window.
    expect(readTimeWindow('2026-09-29T04:11', '2026-09-29T04:11')).toMatchObject({ ok: true });
  });

  it('accepts an open or closed window and refuses an inverted or invalid one', () => {
    expect(readTimeWindow('', '')).toEqual({ ok: true, window: { from: null, to: null } });
    expect(readTimeWindow('2026-09-29T04:00', '')).toMatchObject({ ok: true });
    expect(readTimeWindow('2026-09-29T05:00', '2026-09-29T04:00')).toEqual({
      ok: false,
      message: 'From must be earlier than To.'
    });
    expect(readTimeWindow('nope', '')).toMatchObject({ ok: false });
  });
});
