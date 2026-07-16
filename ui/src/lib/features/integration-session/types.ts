import type { ParsePreviewQuery, PreviewIntegrationMessageMutation } from '$lib/gen/graphql';

export type IntegrationSessionLineage = {
  sourcePath: string;
  targetPath: string | null;
  description: string | null;
};

export type IntegrationSessionDiagnostic = {
  id: string;
  code: string;
  message: string;
  path: string | null;
  severity: string | null;
  fixSuggestion: string | null;
  accepted: boolean;
  acceptedAt: string | null;
  runId: string | null;
  lineage: IntegrationSessionLineage[];
};

export type IntegrationSessionStage = {
  id: string;
  name: string;
  status: string;
  startedAt: string | null;
  completedAt: string | null;
  durationMs: number | null;
};

export type IntegrationSessionPreviewMeta = {
  mode: 'session' | 'fallback';
  id: string | null;
  sampleId: string | null;
  runId: string | null;
  state: string | null;
  diagnostics: IntegrationSessionDiagnostic[];
  stages: IntegrationSessionStage[];
  lineage: IntegrationSessionLineage[];
  streamState: 'connecting' | 'running' | 'complete' | 'error';
  error: string | null;
};

/**
 * Safe compatibility view for the current HL7 inspector plus the complete,
 * immutable preview provenance returned by the integration engine.
 */
export type AuthenticatedIntegrationPreviewResult = {
  parsePreview: ParsePreviewQuery['parsePreview'];
  preview: PreviewIntegrationMessageMutation['previewIntegrationMessage'] | null;
  session?: IntegrationSessionPreviewMeta | null;
};
