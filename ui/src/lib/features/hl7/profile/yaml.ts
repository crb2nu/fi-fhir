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
    ''
  ].join('\n');
}
