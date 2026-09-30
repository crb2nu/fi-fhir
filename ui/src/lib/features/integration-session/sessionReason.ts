/** The export reason's server limit (`maxExportReasonBytes`, internal/integration/session/types.go). */
export const MAX_EXPORT_REASON_BYTES = 1024;

/** The role the session store requires for raw sample payloads in an export. */
export const PHI_EXPORT_ROLE = 'integration.phi.export';

function byteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

/** Why an export reason would be refused, or null when the server accepts it. */
export function exportReasonProblem(reason: string): string | null {
  const trimmed = reason.trim();
  if (trimmed === '') return 'A reason is required. It is recorded with your identity on the export.';
  if (byteLength(trimmed) > MAX_EXPORT_REASON_BYTES) {
    return `A reason must be ${MAX_EXPORT_REASON_BYTES} bytes or fewer.`;
  }
  for (const character of trimmed) {
    if (character === '\n' || character === '\t') continue;
    const code = character.codePointAt(0) ?? 0;
    if (code < 0x20 || (code >= 0x7f && code <= 0x9f)) return 'A reason cannot contain control characters.';
  }
  return null;
}

/** What the export file holds without raw payloads, said the same way everywhere. */
export const EXPORT_CONTENTS_SENTENCE =
  'The file holds runs (status, stages, diagnostics, lineage, event ids and types), drafts, simulations and publications; it holds no raw sample text and no parsed patient fields. Diagnostic messages are included as the parser wrote them.';

/**
 * The sentence the export dialog shows in place of the raw-payload option.
 * `reported` false: the status endpoint said nothing about phiExport (an
 * older API, or a bearer session), so the option is not offered on a guess.
 * Otherwise it names the missing role, from `missingRoles.phiExport`.
 */
export function rawPayloadBlockedSentence(missing: readonly string[], reported = true): string {
  if (!reported) {
    return `Raw sample payloads are not offered: this deployment did not report whether this identity holds ${PHI_EXPORT_ROLE}.`;
  }
  const roles = missing.length > 0 ? missing : [PHI_EXPORT_ROLE];
  return `Raw sample payloads are not offered: exporting them needs ${roles.join(', ')}, which this identity does not hold.`;
}
