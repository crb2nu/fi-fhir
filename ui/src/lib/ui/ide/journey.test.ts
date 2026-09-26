import { describe, expect, it } from 'vitest';
import { getJourneyStage, getJourneyStages, getJourneyState } from './journey';

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

  it('marks earlier stages complete, the route stage current, later ones upcoming', () => {
    const state = getJourneyState('/profiles');

    expect(state.stage?.label).toBe('Normalization');
    expect(state.stageIndex).toBe(1);
    expect(state.totalStages).toBe(5);
    expect(state.steps.map((step) => step.state)).toEqual([
      'complete',
      'current',
      'upcoming',
      'upcoming',
      'upcoming',
    ]);
  });

  it('offers the following stage as next, and none after the last', () => {
    expect(getJourneyState('/profiles').nextStage?.label).toBe('Translation');
    expect(getJourneyState('/workflows').nextStage?.route).toBe('/events');
    expect(getJourneyState('/events').nextStage).toBeNull();
  });

  it('starts at Source Intake from the dashboard and offers nothing off the stages', () => {
    const home = getJourneyState('/');
    expect(home.stage).toBeNull();
    expect(home.stageIndex).toBe(-1);
    expect(home.nextStage?.label).toBe('Source Intake');
    expect(home.steps.every((step) => step.state === 'upcoming')).toBe(true);

    expect(getJourneyState('/operator').nextStage).toBeNull();
  });
});
