import type { ParsePreviewQuery } from '$lib/gen/graphql';

export type IntegrationSessionDiagnostic = {
  id: string;
  code: string;
  message: string;
  path: string | null;
  severity: string | null;
  fixSuggestion: string | null;
  accepted: boolean;
  acceptedAt: string | null;
};

export type IntegrationSessionStage = {
  id: string;
  name: string;
  status: string;
  startedAt: string | null;
  completedAt: string | null;
  durationMs: number | null;
};

export type IntegrationSessionRun = {
  id: string;
  status: string;
  events: ParsePreviewQuery['parsePreview']['events'];
  warnings: ParsePreviewQuery['parsePreview']['warnings'];
  diagnostics: IntegrationSessionDiagnostic[];
  stages: IntegrationSessionStage[];
};

export type IntegrationSessionPreviewMeta = {
  mode: 'session' | 'fallback';
  id: string | null;
  sampleId: string | null;
  runId: string | null;
  state: string | null;
  diagnostics: IntegrationSessionDiagnostic[];
  stages: IntegrationSessionStage[];
  error: string | null;
};

export type SessionBackedPreviewResult = {
  parsePreview: ParsePreviewQuery['parsePreview'];
  session: IntegrationSessionPreviewMeta;
};
