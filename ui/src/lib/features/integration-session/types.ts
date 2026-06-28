import type { ParsePreviewQuery } from '$lib/gen/graphql';

export type IntegrationSessionDiagnostic = {
  id: string;
  phase: string;
  code: string;
  message: string;
  path: string | null;
  severity: string | null;
  status: string | null;
  fixSuggestion: string | null;
};

export type IntegrationSessionStage = {
  id: string;
  name: string;
  state: string;
  startedAt: string | null;
  completedAt: string | null;
  durationMs: number | null;
};

export type IntegrationSessionRun = {
  id: string;
  state: string;
  preview: ParsePreviewQuery['parsePreview'] | null;
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
