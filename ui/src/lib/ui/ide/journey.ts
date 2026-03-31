import type { IDEAppRoute } from './types';

export type JourneyStageId =
  | 'source-intake'
  | 'normalization'
  | 'translation'
  | 'delivery'
  | 'verification';

export type JourneyStageState = 'complete' | 'current' | 'upcoming';

export interface JourneyStageAction {
  label: string;
  href: IDEAppRoute;
  hint: string;
}

export interface JourneyStage {
  id: JourneyStageId;
  order: number;
  label: string;
  route: IDEAppRoute;
  summary: string;
  focus: string[];
  nextAction: JourneyStageAction;
}

export interface JourneyStepState {
  id: JourneyStageId;
  order: number;
  label: string;
  route: IDEAppRoute;
  summary: string;
  state: JourneyStageState;
  focus: string[];
}

export interface JourneyState {
  currentRoute: string;
  isMissionControl: boolean;
  progressLabel: string;
  title: string;
  description: string;
  focus: string[];
  stage: JourneyStage | null;
  nextStage: JourneyStage | null;
  nextAction: JourneyStageAction;
  stageIndex: number;
  totalStages: number;
  steps: JourneyStepState[];
}

const journeyStages: JourneyStage[] = [
  {
    id: 'source-intake',
    order: 1,
    label: 'Source Intake',
    route: '/hl7',
    summary:
      'Load inbound interfaces, keep the raw payload visible, and surface recoverable warnings before they spread downstream.',
    focus: ['Raw payloads', 'Warnings', 'Source profile'],
    nextAction: {
      label: 'Continue to Normalization',
      href: '/profiles',
      hint: 'Open source profiles and tighten identifier rules.',
    },
  },
  {
    id: 'normalization',
    order: 2,
    label: 'Normalization',
    route: '/profiles',
    summary:
      'Shape identifiers, tolerances, and profile rules so the feed reads like the domain instead of the wire format.',
    focus: ['Identifiers', 'Tolerances', 'Profile rules'],
    nextAction: {
      label: 'Continue to Translation',
      href: '/terminology',
      hint: 'Check code system mappings and canonical terms.',
    },
  },
  {
    id: 'translation',
    order: 3,
    label: 'Translation',
    route: '/terminology',
    summary:
      'Map source codes into shared semantic terms and keep the trace explainable for every decision.',
    focus: ['Mappings', 'Candidates', 'Traceability'],
    nextAction: {
      label: 'Continue to Delivery',
      href: '/workflows',
      hint: 'Push normalized data into routes and action chains.',
    },
  },
  {
    id: 'delivery',
    order: 4,
    label: 'Delivery',
    route: '/workflows',
    summary:
      'Package the normalized event into workflows, destinations, and downstream actions without losing the thread.',
    focus: ['Routes', 'Actions', 'Destinations'],
    nextAction: {
      label: 'Continue to Verification',
      href: '/events',
      hint: 'Watch outcomes land in the event stream and timeline.',
    },
  },
  {
    id: 'verification',
    order: 5,
    label: 'Verification',
    route: '/events',
    summary:
      'Compare routed outcomes against source intent, watch for gaps, and decide what to tune next.',
    focus: ['Timeline', 'Outcomes', 'Feedback'],
    nextAction: {
      label: 'Return to Mission Control',
      href: '/',
      hint: 'Review the full source-to-destination path.',
    },
  },
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
  return journeyStages.slice();
}

export function getJourneyStage(pathname: string): JourneyStage | null {
  const normalized = normalizePathname(pathname);
  return journeyStages.find((stage) => matchesStageRoute(normalized, stage.route)) ?? null;
}

export function getJourneyState(pathname: string): JourneyState {
  const normalized = normalizePathname(pathname);
  const stage = getJourneyStage(normalized);
  const stageIndex = stage ? stage.order - 1 : -1;
  const totalStages = journeyStages.length;
  const nextStage = stage ? journeyStages[stage.order] ?? null : journeyStages[0] ?? null;
  const progressLabel = stage ? `Stage ${stage.order} of ${totalStages}` : 'Mission control';
  const title = stage ? stage.label : 'Mission control';
  const description = stage
    ? stage.summary
    : 'Coordinate source intake, normalization, translation, delivery, and verification from one place.';
  const focus = stage ? stage.focus : ['Source intake', 'Normalization', 'Translation', 'Delivery', 'Verification'];
  const nextAction = stage ? stage.nextAction : journeyStages[0]?.nextAction ?? {
    label: 'Start Source Intake',
    href: '/hl7',
    hint: 'Open inbound interfaces and inspect the raw feed.',
  };

  const steps: JourneyStepState[] = journeyStages.map((step) => {
    let state: JourneyStageState = 'upcoming';
    if (stage) {
      if (step.order < stage.order) {
        state = 'complete';
      } else if (step.order === stage.order) {
        state = 'current';
      }
    }

    return {
      id: step.id,
      order: step.order,
      label: step.label,
      route: step.route,
      summary: step.summary,
      focus: step.focus,
      state,
    };
  });

  return {
    currentRoute: normalized,
    isMissionControl: !stage,
    progressLabel,
    title,
    description,
    focus,
    stage,
    nextStage,
    nextAction,
    stageIndex,
    totalStages,
    steps,
  };
}
