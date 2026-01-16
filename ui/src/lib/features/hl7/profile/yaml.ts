import type { SourceProfile } from '$lib/gen/graphql';

function quoteIfNeeded(v: string): string {
  if (v === '') return '""';
  if (/^[a-zA-Z0-9._-]+$/.test(v)) return v;
  return JSON.stringify(v);
}

export function toSourceProfileYAML(profile: SourceProfile): string {
  const hl7v2 = profile.hl7v2;
  const tolerance = hl7v2?.tolerance;
  const identifiers = profile.identifiers;
  const validation = identifiers?.validation;
  const normalization = identifiers?.normalization;

  const missing = tolerance?.missingSegments || [];
  const missingInline =
    missing.length === 0 ? '[]' : `[${missing.map((s) => quoteIfNeeded(s)).join(', ')}]`;

  const npi = validation?.npi || { enabled: false, onInvalid: 'pass' };
  const mbi = validation?.mbi || { enabled: false, onInvalid: 'pass' };
  const ssn = validation?.ssn || { enabled: false, onInvalid: 'pass' };

  return [
    'source_profile:',
    `  id: ${quoteIfNeeded(profile.id)}`,
    `  name: ${quoteIfNeeded(profile.name)}`,
    `  version: ${quoteIfNeeded(profile.version)}`,
    '',
    '  hl7v2:',
    `    default_version: ${quoteIfNeeded(hl7v2?.defaultVersion || '2.5.1')}`,
    `    timezone: ${quoteIfNeeded(hl7v2?.timezone || 'UTC')}`,
    '',
    '    tolerate:',
    `      missing_segments: ${missingInline}`,
    `      nte_anywhere: ${tolerance?.nteAnywhere ?? false}`,
    `      extra_components: ${tolerance?.extraComponents ?? false}`,
    `      unknown_segments: ${tolerance?.unknownSegments ?? false}`,
    `      non_standard_delimiters: ${tolerance?.nonStandardDelimiters ?? false}`,
    '',
    '  identifiers:',
    '    validation:',
    `      npi: { enabled: ${npi.enabled}, on_invalid: ${quoteIfNeeded(npi.onInvalid)} }`,
    `      mbi: { enabled: ${mbi.enabled}, on_invalid: ${quoteIfNeeded(mbi.onInvalid)} }`,
    `      ssn: { enabled: ${ssn.enabled}, on_invalid: ${quoteIfNeeded(ssn.onInvalid)} }`,
    '',
    '    normalization:',
    '      ssn:',
    `        strip_dashes: ${normalization?.ssnStripDashes ?? false}`,
    `        reject_patterns: [${(normalization?.ssnRejectPatterns || []).map((p) => quoteIfNeeded(p)).join(', ')}]`,
    '      phone:',
    `        normalize: ${normalization?.phoneNormalize ?? false}`,
    `        format: ${normalization?.phoneFormat ? quoteIfNeeded(normalization.phoneFormat) : 'null'}`,
    ''
  ].join('\n');
}
