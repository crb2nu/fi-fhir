import { derived, writable } from 'svelte/store';
import type { ParsePreviewQuery } from '$lib/gen/graphql';
import { groupWarningsByPhase } from '$lib/domain/warnings';

export type HL7PreviewState = {
  source: string;
  data: string;
  loading: boolean;
  error: string | null;
  result: ParsePreviewQuery | null;
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
    result: null
  });

  const warningsByPhase = derived(state, ($s) => {
    const warnings = $s.result?.parsePreview.warnings ?? [];
    return groupWarningsByPhase(warnings);
  });

  const events = derived(state, ($s) => $s.result?.parsePreview.events ?? []);

  return { state, warningsByPhase, events };
}

