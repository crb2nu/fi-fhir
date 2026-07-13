import { beforeEach, describe, expect, it } from 'vitest';
import { purgeLegacyHL7BrowserStorage } from './legacyStorage';

describe('legacy HL7 browser-storage purge', () => {
  beforeEach(() => localStorage.clear());

  it('removes known raw and filename keys while preserving unrelated preferences', () => {
    localStorage.setItem('fi-fhir:hl7:samples:v1', 'MSH|PHI-SENTINEL');
    localStorage.setItem('fi-fhir:hl7:recent-sources:v1', '["Jane_Doe_MRN123"]');
    localStorage.setItem('fi-fhir:theme', 'dark');

    purgeLegacyHL7BrowserStorage(localStorage);

    expect(localStorage.getItem('fi-fhir:hl7:samples:v1')).toBeNull();
    expect(localStorage.getItem('fi-fhir:hl7:recent-sources:v1')).toBeNull();
    expect(localStorage.getItem('fi-fhir:theme')).toBe('dark');
    expect(localStorage).toHaveLength(1);
  });

  it('does not block startup when the localStorage getter is unavailable', () => {
    const descriptor = Object.getOwnPropertyDescriptor(globalThis, 'localStorage');
    Object.defineProperty(globalThis, 'localStorage', {
      configurable: true,
      get: () => {
        throw new DOMException('storage disabled', 'SecurityError');
      }
    });

    try {
      expect(() => purgeLegacyHL7BrowserStorage()).not.toThrow();
    } finally {
      if (descriptor) Object.defineProperty(globalThis, 'localStorage', descriptor);
    }
  });
});
