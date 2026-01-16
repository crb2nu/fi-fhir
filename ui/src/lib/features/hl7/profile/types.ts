import type { UpdateProfileInput, SourceProfile } from '$lib/gen/graphql';

// Legacy type for compatibility
export type HL7ProfileDraft = {
  id: string;
  name: string;
  version: string;
  defaultVersion: string;
  timezone: string;
  tolerate: {
    missingSegments: string[];
    nteAnywhere: boolean;
    extraComponents: boolean;
    unknownSegments: boolean;
    nonStandardDelimiters: boolean;
  };
  identifiers: {
    validation: {
      npi: { enabled: boolean; onInvalid: 'error' | 'warn' | 'pass' };
      mbi: { enabled: boolean; onInvalid: 'error' | 'warn' | 'pass' };
      ssn: { enabled: boolean; onInvalid: 'error' | 'warn' | 'pass' };
    };
    normalization: {
      ssn: { stripDashes: boolean; rejectPatterns: string[] };
      phone: { stripCountryCode: boolean; normalizeToDigits: boolean };
    };
  };
};

// New fix type that works with the backend-connected store
export type ProfileFix = {
  id: string;
  title: string;
  description: string;
  /** The changes to apply to the profile */
  changes: UpdateProfileInput;
  /** Optional: legacy apply function for backward compatibility */
  apply?: (draft: HL7ProfileDraft) => HL7ProfileDraft;
};

/** Helper to create a profile fix from a SourceProfile and changes */
export function createFix(
  id: string,
  title: string,
  description: string,
  changes: UpdateProfileInput
): ProfileFix {
  return { id, title, description, changes };
}
