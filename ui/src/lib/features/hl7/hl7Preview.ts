import type { ParsePreviewQuery } from '$lib/gen/graphql';
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
 * integration preview mutation backed by the deterministic processor kernel.
 */
export async function parseHL7Preview(input: HL7PreviewInput): Promise<HL7PreviewResult> {
  return runAuthenticatedIntegrationPreview(input);
}
