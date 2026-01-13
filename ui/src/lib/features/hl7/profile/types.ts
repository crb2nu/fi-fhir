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

export type ProfileFix = {
  id: string;
  title: string;
  description: string;
  apply: (draft: HL7ProfileDraft) => HL7ProfileDraft;
};
