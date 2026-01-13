import type { HL7ProfileDraft } from './types';

function quoteIfNeeded(v: string): string {
  if (v === '') return '""';
  if (/^[a-zA-Z0-9._-]+$/.test(v)) return v;
  return JSON.stringify(v);
}

export function toSourceProfileYAML(draft: HL7ProfileDraft): string {
  const missing = draft.tolerate.missingSegments;
  const missingInline =
    missing.length === 0 ? '[]' : `[${missing.map((s) => quoteIfNeeded(s)).join(', ')}]`;

  return [
    'source_profile:',
    `  id: ${quoteIfNeeded(draft.id)}`,
    `  name: ${quoteIfNeeded(draft.name)}`,
    `  version: ${quoteIfNeeded(draft.version)}`,
    '',
    '  hl7v2:',
    `    default_version: ${quoteIfNeeded(draft.defaultVersion)}`,
    `    timezone: ${quoteIfNeeded(draft.timezone)}`,
    '',
    '    tolerate:',
    `      missing_segments: ${missingInline}`,
    `      nte_anywhere: ${draft.tolerate.nteAnywhere}`,
    `      extra_components: ${draft.tolerate.extraComponents}`,
    `      unknown_segments: ${draft.tolerate.unknownSegments}`,
    `      non_standard_delimiters: ${draft.tolerate.nonStandardDelimiters}`,
    '',
    '  identifiers:',
    '    validation:',
    `      npi: { enabled: ${draft.identifiers.validation.npi.enabled}, on_invalid: ${quoteIfNeeded(draft.identifiers.validation.npi.onInvalid)} }`,
    `      mbi: { enabled: ${draft.identifiers.validation.mbi.enabled}, on_invalid: ${quoteIfNeeded(draft.identifiers.validation.mbi.onInvalid)} }`,
    `      ssn: { enabled: ${draft.identifiers.validation.ssn.enabled}, on_invalid: ${quoteIfNeeded(draft.identifiers.validation.ssn.onInvalid)} }`,
    '',
    '    normalization:',
    '      ssn:',
    `        strip_dashes: ${draft.identifiers.normalization.ssn.stripDashes}`,
    `        reject_patterns: [${draft.identifiers.normalization.ssn.rejectPatterns.map((p) => quoteIfNeeded(p)).join(', ')}]`,
    '      phone:',
    `        strip_country_code: ${draft.identifiers.normalization.phone.stripCountryCode}`,
    `        normalize_to_digits: ${draft.identifiers.normalization.phone.normalizeToDigits}`,
    ''
  ].join('\n');
}
