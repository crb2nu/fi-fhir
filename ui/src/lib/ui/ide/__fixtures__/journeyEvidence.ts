import type { JourneyEvidence, JourneyStageId, StageEvidence } from '../journey';

/** Test evidence: every stage `incomplete` unless overridden by state. */
export function evidenceOf(
  states: Partial<Record<JourneyStageId, StageEvidence['state']>> = {}
): JourneyEvidence {
  const ids: JourneyStageId[] = ['source-intake', 'normalization', 'translation', 'delivery', 'verification'];
  return Object.fromEntries(
    ids.map((id) => {
      const state = states[id] ?? 'incomplete';
      return [id, { state, reason: `${id} is ${state}.` }];
    })
  ) as JourneyEvidence;
}
