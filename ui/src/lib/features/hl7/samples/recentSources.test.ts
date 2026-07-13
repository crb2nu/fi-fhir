import { beforeEach, describe, expect, it } from 'vitest';
import { rememberRecentSource } from './recentSources';

describe('recent HL7 source privacy', () => {
  beforeEach(() => localStorage.clear());

  it('keeps filename-derived source labels in tab memory', () => {
    localStorage.setItem('fi-fhir:theme', 'dark');

    const sources = rememberRecentSource([], 'Jane_Doe_MRN123', 8);

    expect(sources).toEqual(['Jane_Doe_MRN123']);
    expect(localStorage.getItem('fi-fhir:theme')).toBe('dark');
    expect(localStorage).toHaveLength(1);
  });
});
