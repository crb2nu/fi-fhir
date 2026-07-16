import type { IDEAppRoute } from '../types';
import { getJourneyState } from '../journey';

export type SidebarView = 'home' | 'events' | 'hl7' | 'profiles' | 'terminology' | 'workflows';

export type SidebarAction = {
  label: string;
  href: IDEAppRoute;
  hint: string;
};

export type SidebarAsset = {
  label: string;
  detail: string;
};

export type SidebarViewLink = {
  view: SidebarView;
  label: string;
  href: IDEAppRoute;
};

export interface SidebarContext {
  view: SidebarView;
  eyebrow: string;
  title: string;
  description: string;
  highlights: string[];
  actions: SidebarAction[];
  recent: SidebarAsset[];
}

// Domain-first sidebar navigation (matches the ActivityBar labels, Slice 3).
// The journey metaphor is preserved in the per-stage headings + journey panel.
const viewLinks: SidebarViewLink[] = [
  { view: 'home', label: 'Dashboard', href: '/' },
  { view: 'hl7', label: 'HL7 / Intake', href: '/hl7' },
  { view: 'profiles', label: 'Profiles', href: '/profiles' },
  { view: 'terminology', label: 'Terminology', href: '/terminology' },
  { view: 'workflows', label: 'Workflows', href: '/workflows' },
  { view: 'events', label: 'Events', href: '/events' },
];

const contexts: Record<SidebarView, Omit<SidebarContext, 'journey'>> = {
  home: {
    view: 'home',
    eyebrow: 'Overview',
    title: 'Mission control',
    description: 'The full source-to-destination journey at a glance.',
    highlights: ['Source intake', 'Normalization', 'Translation', 'Delivery', 'Verification'],
    actions: [
      { label: 'Start source intake', href: '/hl7', hint: 'Open inbound messages and review recoverable warnings.' },
      { label: 'Shape normalization', href: '/profiles', hint: 'Tighten identifiers, tolerances, and source profile rules.' },
      { label: 'Inspect translation', href: '/terminology', hint: 'Verify code mappings before delivery.' },
    ],
    recent: [
      { label: 'Latest events', detail: 'Review the most recent downstream outcomes and patient timeline.' },
      { label: 'Active workflow queue', detail: 'Pick up the next route, transform, or action chain.' },
      { label: 'System health', detail: 'Check parser, routing, and runtime status before you continue.' },
    ],
  },
  hl7: {
    view: 'hl7',
    eyebrow: 'Stage 1',
    title: 'Source intake',
    description: 'Load inbound messages, inspect raw payloads, and review recoverable warnings.',
    highlights: ['Raw payloads', 'Warnings', 'Source profile'],
    actions: [
      { label: 'Open normalization', href: '/profiles', hint: 'Tune identifiers and tolerance rules.' },
      { label: 'Check translation', href: '/terminology', hint: 'Validate mapping candidates before delivery.' },
      { label: 'Review verification', href: '/events', hint: 'Confirm the downstream event trail.' },
    ],
    recent: [
      { label: 'Inbound queue', detail: 'Keep the current interface and sample inbox in view.' },
      { label: 'Parser warnings', detail: 'Review recoverable anomalies before they become downstream noise.' },
      { label: 'Raw message inspector', detail: 'Inspect segments, paths, and source payload values.' },
    ],
  },
  profiles: {
    view: 'profiles',
    eyebrow: 'Stage 2',
    title: 'Normalization',
    description: 'Tune identifiers, tolerances, and recoverable-anomaly rules in the source profile.',
    highlights: ['Identifiers', 'Tolerances', 'Profile rules'],
    actions: [
      { label: 'Return to source intake', href: '/hl7', hint: 'Recheck raw messages against your profile rules.' },
      { label: 'Open translation', href: '/terminology', hint: 'Verify code system decisions and semantic mapping.' },
      { label: 'Advance to delivery', href: '/workflows', hint: 'Route normalized data into downstream actions.' },
    ],
    recent: [
      { label: 'Identifier editor', detail: 'Normalize PID-3 repetitions and assigning authorities.' },
      { label: 'Tolerance editor', detail: 'Adjust how the parser treats recoverable anomalies.' },
      { label: 'Profile revisions', detail: 'Track the latest YAML snapshot and review history.' },
    ],
  },
  terminology: {
    view: 'terminology',
    eyebrow: 'Stage 3',
    title: 'Translation',
    description: 'Map source codes to shared semantic terms, with every decision traceable.',
    highlights: ['Mappings', 'Candidates', 'Traceability'],
    actions: [
      { label: 'Return to normalization', href: '/profiles', hint: 'Revisit profile-driven identifier and tolerance rules.' },
      { label: 'Advance to delivery', href: '/workflows', hint: 'Turn translated codes into routing logic.' },
      { label: 'Review verification', href: '/events', hint: 'Confirm the resulting semantic event trail.' },
    ],
    recent: [
      { label: 'Mapping browser', detail: 'Browse code systems, equivalence values, and source candidates.' },
      { label: 'Pending review', detail: 'Inspect high-confidence suggestions before approval.' },
      { label: 'Autoroute trace', detail: 'Study candidate scoring and the path each decision took.' },
    ],
  },
  workflows: {
    view: 'workflows',
    eyebrow: 'Stage 4',
    title: 'Delivery',
    description: 'Route normalized events into workflows, destinations, and action chains.',
    highlights: ['Routes', 'Actions', 'Destinations'],
    actions: [
      { label: 'Return to translation', href: '/terminology', hint: 'Check the semantic terms before you ship them.' },
      { label: 'Open verification', href: '/events', hint: 'Watch how routed outcomes land in the event stream.' },
      { label: 'Revisit source intake', href: '/hl7', hint: 'Trace the delivery path back to raw input.' },
    ],
    recent: [
      { label: 'Workflow builder', detail: 'Shape routes, transforms, and action chains.' },
      { label: 'Dry run panel', detail: 'Preview a change before it reaches live traffic.' },
      { label: 'Workflow monitor', detail: 'Inspect retries, status, and delivery health.' },
    ],
  },
  events: {
    view: 'events',
    eyebrow: 'Stage 5',
    title: 'Verification',
    description: 'Compare routed outcomes with source intent on the timeline.',
    highlights: ['Timeline', 'Outcomes', 'Feedback'],
    actions: [
      { label: 'Return to delivery', href: '/workflows', hint: 'Trace the route that delivered the event.' },
      { label: 'Check translation', href: '/terminology', hint: 'Validate the semantic mapping that fed verification.' },
      { label: 'Go back to mission control', href: '/', hint: 'Review the full journey and choose the next interface.' },
    ],
    recent: [
      { label: 'Event browser', detail: 'Review the latest semantic events and filters.' },
      { label: 'Patient timeline', detail: 'Follow a correlated journey across downstream systems.' },
      { label: 'Verification log', detail: 'Inspect what landed, what failed, and what needs tuning.' },
    ],
  },
};

function normalizePathname(pathname: string): string {
  if (!pathname) return '/';
  if (pathname.length > 1 && pathname.endsWith('/')) return pathname.replace(/\/+$/, '');
  return pathname;
}

export function getSidebarView(pathname: string): SidebarView {
  const normalized = normalizePathname(pathname);

  if (normalized === '/') return 'home';
  if (normalized.startsWith('/hl7')) return 'hl7';
  if (normalized.startsWith('/profiles')) return 'profiles';
  if (normalized.startsWith('/terminology')) return 'terminology';
  if (normalized.startsWith('/workflows')) return 'workflows';
  if (normalized.startsWith('/events')) return 'events';
  return 'home';
}

export function getSidebarContext(pathname: string): SidebarContext & { journey: ReturnType<typeof getJourneyState> } {
  const view = getSidebarView(pathname);
  return {
    ...contexts[view],
    journey: getJourneyState(pathname),
  };
}

export function getSidebarViewLinks(): SidebarViewLink[] {
  return viewLinks.slice();
}
