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
};

export type ProfileFix = {
  id: string;
  title: string;
  description: string;
  apply: (draft: HL7ProfileDraft) => HL7ProfileDraft;
};

