import type { IDEAppRoute } from './types';

/**
 * The five integration stages as navigation: which route each stage lives on,
 * which stage a pathname belongs to, and which stage comes next. The header's
 * stage control and the status bar's `Next:` item read this model; it holds
 * labels and routes only — no copy.
 */

export type JourneyStageId =
  | 'source-intake'
  | 'normalization'
  | 'translation'
  | 'delivery'
  | 'verification';

export type JourneyStageState = 'complete' | 'current' | 'upcoming';

export interface JourneyStage {
  id: JourneyStageId;
  order: number;
  label: string;
  route: IDEAppRoute;
}

export interface JourneyStepState extends JourneyStage {
  state: JourneyStageState;
}

export interface JourneyState {
  currentRoute: string;
  /** The stage the pathname belongs to; null on routes outside the stages. */
  stage: JourneyStage | null;
  /**
   * The stage to offer as `Next:`. The following stage on a stage route (none
   * after the last), Source Intake from the dashboard, nothing elsewhere.
   */
  nextStage: JourneyStage | null;
  stageIndex: number;
  totalStages: number;
  steps: JourneyStepState[];
}

const journeyStages: readonly JourneyStage[] = [
  { id: 'source-intake', order: 1, label: 'Source Intake', route: '/hl7' },
  { id: 'normalization', order: 2, label: 'Normalization', route: '/profiles' },
  { id: 'translation', order: 3, label: 'Translation', route: '/terminology' },
  { id: 'delivery', order: 4, label: 'Delivery', route: '/workflows' },
  { id: 'verification', order: 5, label: 'Verification', route: '/events' },
];

function normalizePathname(pathname: string): string {
  if (!pathname) return '/';
  if (pathname.length > 1 && pathname.endsWith('/')) {
    return pathname.replace(/\/+$/, '');
  }
  return pathname;
}

function matchesStageRoute(pathname: string, stageRoute: IDEAppRoute): boolean {
  return pathname === stageRoute || pathname.startsWith(`${stageRoute}/`);
}

export function getJourneyStages(): JourneyStage[] {
  return journeyStages.map((stage) => ({ ...stage }));
}

export function getJourneyStage(pathname: string): JourneyStage | null {
  const normalized = normalizePathname(pathname);
  return journeyStages.find((stage) => matchesStageRoute(normalized, stage.route)) ?? null;
}

export function getJourneyState(pathname: string): JourneyState {
  const normalized = normalizePathname(pathname);
  const stage = getJourneyStage(normalized);
  const stageIndex = stage ? stage.order - 1 : -1;

  let nextStage: JourneyStage | null = null;
  if (stage) {
    nextStage = journeyStages[stage.order] ?? null;
  } else if (normalized === '/') {
    nextStage = journeyStages[0] ?? null;
  }

  const steps: JourneyStepState[] = journeyStages.map((step) => {
    let state: JourneyStageState = 'upcoming';
    if (stage) {
      if (step.order < stage.order) state = 'complete';
      else if (step.order === stage.order) state = 'current';
    }
    return { ...step, state };
  });

  return {
    currentRoute: normalized,
    stage,
    nextStage,
    stageIndex,
    totalStages: journeyStages.length,
    steps,
  };
}
