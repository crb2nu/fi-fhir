import { derived, writable } from 'svelte/store';
import type { ExplainedWarning } from '$lib/gen/graphql';
import { groupWarningsByPhase } from '$lib/domain/warnings';
import { parseHL7Message } from '$lib/domain/hl7v2';
import type { IntegrationSessionPreviewMeta } from '$lib/features/integration-session';
import type { HL7PreviewResult } from './hl7Preview';

export type HL7PreviewState = {
  source: string;
  data: string;
  loading: boolean;
  error: string | null;
  result: HL7PreviewResult | null;
  session: IntegrationSessionPreviewMeta | null;
};

const defaultSample =
  'MSH|^~\\\\&|EPIC|HOSPITAL|FI-FHIR|DEST|20240115103000||ADT^A01|MSG001|P|2.5\\r' +
  'PID|1||MRN123^^^HOSP^MR||DOE^JOHN||19800101|M\\r' +
  'PV1|1|I|ICU^101^A^HOSPITAL';

export function createHL7PreviewStore() {
  const state = writable<HL7PreviewState>({
    source: 'ui_preview',
    data: defaultSample,
    loading: false,
    error: null,
    result: null,
    session: null
  });

  const warningsByPhase = derived(state, ($s) => {
    const warnings = $s.result?.parsePreview.warnings ?? [];
    return groupWarningsByPhase(warnings);
  });

  const events = derived(state, ($s) => $s.result?.parsePreview.events ?? []);

  const sessionDiagnostics = derived(state, ($s) => $s.session?.diagnostics ?? []);

  const hl7 = derived(state, ($s) => parseHL7Message($s.data));

  /**
   * Updates a warning in the store with explanation data from the LLM.
   */
  function updateWarningExplanation(code: string, explanation: ExplainedWarning) {
    state.update((s) => {
      if (!s.result?.parsePreview.warnings) return s;

      const warnings = s.result.parsePreview.warnings.map((w) =>
        w.code === code
          ? {
              ...w,
              explanation: explanation.explanation,
              fixSuggestion: explanation.fixSuggestion,
              impact: explanation.impact,
              fromCache: explanation.fromCache
            }
          : w
      );

      return {
        ...s,
        result: {
          ...s.result,
          parsePreview: {
            ...s.result.parsePreview,
            warnings
          }
        }
      };
    });
  }

  return { state, warningsByPhase, events, sessionDiagnostics, hl7, updateWarningExplanation };
}
