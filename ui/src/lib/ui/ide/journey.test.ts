import { describe, expect, it } from 'vitest';
import { getJourneyStage, getJourneyStages, getJourneyState, stageTitle } from './journey';
import { evidenceOf } from './__fixtures__/journeyEvidence';

describe('journey model', () => {
  it('lists the five stages in order with their routes', () => {
    expect(getJourneyStages().map((stage) => [stage.label, stage.route])).toEqual([
      ['Source Intake', '/hl7'],
      ['Normalization', '/profiles'],
      ['Translation', '/terminology'],
      ['Delivery', '/workflows'],
      ['Verification', '/events'],
    ]);
  });

  it('maps nested and trailing-slash routes to their stage', () => {
    expect(getJourneyStage('/hl7/sample')?.id).toBe('source-intake');
    expect(getJourneyStage('/profiles/')?.id).toBe('normalization');
    expect(getJourneyStage('/events/patient-123')?.id).toBe('verification');
    expect(getJourneyStage('/operator')).toBeNull();
    expect(getJourneyStage('/')).toBeNull();
  });

  it('never marks a stage complete by position: without evidence every stage is pending', () => {
    const state = getJourneyState('/workflows');

    expect(state.stage?.label).toBe('Delivery');
    expect(state.stageIndex).toBe(3);
    expect(state.totalStages).toBe(5);
    expect(state.steps.map((step) => step.state)).toEqual(['pending', 'pending', 'pending', 'pending', 'pending']);
    expect(state.steps.map((step) => step.current)).toEqual([false, false, false, true, false]);
    expect(state.nextStage).toBeNull();
  });

  it('takes each stage state from its evidence, independent of the route', () => {
    const evidence = evidenceOf({ 'source-intake': 'unknown', translation: 'complete', verification: 'complete' });
    const state = getJourneyState('/profiles', evidence);

    expect(state.steps.map((step) => step.state)).toEqual([
      'unknown',
      'incomplete',
      'complete',
      'incomplete',
      'complete',
    ]);
    expect(state.steps[0]?.reason).toBe('source-intake is unknown.');
    expect(state.steps[1]?.current).toBe(true);
  });

  it('offers the earliest incomplete stage other than the current one as next', () => {
    const evidence = evidenceOf({ 'source-intake': 'complete', normalization: 'unknown' });
    expect(getJourneyState('/hl7', evidence).nextStage?.label).toBe('Translation');
    expect(getJourneyState('/terminology', evidence).nextStage?.label).toBe('Delivery');
    expect(getJourneyState('/', evidence).nextStage?.label).toBe('Translation');
  });

  it('offers nothing when no stage is incomplete, and nothing off the stages', () => {
    const done = evidenceOf({
      'source-intake': 'complete',
      normalization: 'complete',
      translation: 'unknown',
      delivery: 'complete',
      verification: 'complete',
    });
    expect(getJourneyState('/hl7', done).nextStage).toBeNull();

    const fresh = evidenceOf();
    expect(getJourneyState('/operator', fresh).nextStage).toBeNull();
    expect(getJourneyState('/connections', fresh).nextStage).toBeNull();
    expect(getJourneyState('/', fresh).nextStage?.label).toBe('Source Intake');
  });

  it('titles a stage with its state and reason', () => {
    const step = getJourneyState('/', evidenceOf({ normalization: 'unknown' })).steps[1]!;
    expect(stageTitle(step, 5)).toBe('Stage 2 of 5: Normalization — unknown. normalization is unknown.');
    const pending = getJourneyState('/').steps[0]!;
    expect(stageTitle(pending, 5)).toBe('Stage 1 of 5: Source Intake — not checked yet.');
  });
});
