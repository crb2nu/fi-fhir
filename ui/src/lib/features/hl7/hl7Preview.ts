import type { ParsePreviewQuery } from '$lib/gen/graphql';
import { normalizeHL7Newlines } from '$lib/domain/hl7Access';
import {
  runAuthenticatedIntegrationPreview,
  type AuthenticatedIntegrationPreviewInput,
  type AuthenticatedIntegrationPreviewResult,
  type IntegrationSessionPreviewMeta
} from '$lib/features/integration-session';

export type HL7PreviewInput = AuthenticatedIntegrationPreviewInput;

export type HL7PreviewResult = AuthenticatedIntegrationPreviewResult & {
  // Compatibility-only until the active HL7 layout branch removes its legacy
  // session display. Stateless preview never populates this field.
  session?: IntegrationSessionPreviewMeta | null;
  parsePreview: ParsePreviewQuery['parsePreview'];
};

/**
 * Runs the sole supported IDE preview path: the authenticated, stateless
 * integration preview mutation backed by the deterministic processor kernel,
 * or the Integration Session run when that engine is on.
 *
 * Segments are sent CR-terminated on either path. The editor joins lines with
 * LF and pasted files carry LF or CRLF, while the preview kernel's strict
 * validation accepts only CR; the editor text itself is left as typed.
 */
export async function parseHL7Preview(input: HL7PreviewInput): Promise<HL7PreviewResult> {
  return runAuthenticatedIntegrationPreview({ ...input, data: normalizeHL7Newlines(input.data) });
}
