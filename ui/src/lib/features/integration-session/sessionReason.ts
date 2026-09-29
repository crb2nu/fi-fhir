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

/**
 * The sentence the export dialog shows in place of the raw-payload option
 * when the identity cannot use it: which role is missing, from the server's
 * `missingRoles.phiExport` when it reported one.
 */
export function rawPayloadBlockedSentence(missing: readonly string[]): string {
  const roles = missing.length > 0 ? missing : [PHI_EXPORT_ROLE];
  return `Raw sample payloads are not offered: exporting them needs ${roles.join(', ')}, which this identity does not hold. The export carries runs, diagnostics, drafts and publications without sample text.`;
}
