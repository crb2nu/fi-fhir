import type { IDEAppRoute } from './types';

/**
 * The five integration stages as navigation, and their state from evidence:
 * which route each stage lives on, which stage a pathname belongs to, whether
 * each stage's evidence exists (journeyState.ts reads it), and which stage to
 * offer as `Next:`. The header's stage control, the status bar's `Next:` item
 * and the sidebar's stage badge read this model.
 *
 * A stage is complete only when its evidence exists — never because its route
 * comes before the current one (.loom/42 decision 6).
 */

export type JourneyStageId =
  | 'source-intake'
  | 'normalization'
  | 'translation'
  | 'delivery'
  | 'verification';

/**
 * - `complete`: the stage's evidence exists.
 * - `incomplete`: the evidence query answered, and there is none yet.
 * - `unknown`: the query is not allowed for this identity, the capability is
 *   not configured, or the query failed; rendered as not complete.
 * - `pending`: not checked yet.
 */
export type JourneyStageState = 'complete' | 'incomplete' | 'unknown' | 'pending';

/** One stage's evidence: its state and one sentence saying why. */
export interface StageEvidence {
  state: Exclude<JourneyStageState, 'pending'>;
  reason: string;
}

export type JourneyEvidence = Record<JourneyStageId, StageEvidence>;

export interface JourneyStage {
  id: JourneyStageId;
  order: number;
  label: string;
  route: IDEAppRoute;
}

export interface JourneyStepState extends JourneyStage {
  state: JourneyStageState;
  /** The route belongs to this stage. */
  current: boolean;
  /** Why the stage is in its state; "" while pending. */
  reason: string;
}

export interface JourneyState {
  currentRoute: string;
  /** The stage the pathname belongs to; null on routes outside the stages. */
  stage: JourneyStage | null;
  /**
   * The stage to offer as `Next:` — the earliest stage, other than the
   * current one, whose evidence query answered with nothing yet. Null until
   * the evidence is known, when no stage is incomplete, and off the stage
   * routes and Home (Connections, Operator).
   */
  nextStage: JourneyStepState | null;
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

export function getJourneyState(pathname: string, evidence: JourneyEvidence | null = null): JourneyState {
  const normalized = normalizePathname(pathname);
  const stage = getJourneyStage(normalized);
  const stageIndex = stage ? stage.order - 1 : -1;

  const steps: JourneyStepState[] = journeyStages.map((step) => {
    const known = evidence?.[step.id];
    return {
      ...step,
      current: stage?.id === step.id,
      state: known?.state ?? 'pending',
      reason: known?.reason ?? '',
    };
  });

  let nextStage: JourneyStepState | null = null;
  if (evidence && (stage || normalized === '/')) {
    nextStage = steps.find((step) => !step.current && step.state === 'incomplete') ?? null;
  }

  return {
    currentRoute: normalized,
    stage,
    nextStage,
    stageIndex,
    totalStages: journeyStages.length,
    steps,
  };
}

/** "complete" / "not complete" / "unknown" / "not checked yet", for labels. */
export function stageStateWord(state: JourneyStageState): string {
  switch (state) {
    case 'complete':
      return 'complete';
    case 'incomplete':
      return 'not complete';
    case 'unknown':
      return 'unknown';
    default:
      return 'not checked yet';
  }
}

/** A stage's tooltip: "Stage 2 of 5: Normalization — complete. 1 published profile." */
export function stageTitle(step: JourneyStepState, total: number): string {
  const head = `Stage ${step.order} of ${total}: ${step.label} — ${stageStateWord(step.state)}.`;
  return step.reason ? `${head} ${step.reason}` : head;
}
