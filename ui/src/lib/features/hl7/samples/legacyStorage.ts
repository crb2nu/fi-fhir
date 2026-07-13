const LEGACY_HL7_STORAGE_KEYS = [
  'fi-fhir:hl7:samples:v1',
  'fi-fhir:hl7:recent-sources:v1'
] as const;

/**
 * Removes browser-persisted raw samples and PHI-bearing source labels written
 * by releases that predate the tab-memory boundary. Unrelated preferences are
 * deliberately preserved.
 */
export function purgeLegacyHL7BrowserStorage(
  storage?: Storage
): void {
  if (!storage) {
    try {
      storage = globalThis.localStorage;
    } catch {
      // Some privacy modes expose localStorage through a throwing getter.
      return;
    }
  }
  if (!storage) return;
  for (const key of LEGACY_HL7_STORAGE_KEYS) {
    try {
      storage.removeItem(key);
    } catch {
      // A blocked storage API must not prevent the credential gate rendering.
    }
  }
}
