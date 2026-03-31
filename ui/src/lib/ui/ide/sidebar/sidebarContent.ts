import type { IDEAppRoute } from '../types';

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

const viewLinks: SidebarViewLink[] = [
  { view: 'home', label: 'Home', href: '/' },
  { view: 'events', label: 'Events', href: '/events' },
  { view: 'hl7', label: 'HL7 Mapping', href: '/hl7' },
  { view: 'profiles', label: 'Profiles', href: '/profiles' },
  { view: 'terminology', label: 'Terminology', href: '/terminology' },
  { view: 'workflows', label: 'Workflows', href: '/workflows' },
];

const contexts: Record<SidebarView, SidebarContext> = {
  home: {
    view: 'home',
    eyebrow: 'Workbench overview',
    title: 'Mapping studio',
    description:
      'Keep the core surfaces close at hand: event review, HL7 inspection, profile tuning, terminology mapping, and workflow design.',
    highlights: ['Events', 'HL7', 'Profiles', 'Terminology', 'Workflows'],
    actions: [
      { label: 'Open event dashboard', href: '/events', hint: 'Review the latest processed events' },
      { label: 'Preview HL7 messages', href: '/hl7', hint: 'Inspect warnings and sample inboxes' },
      { label: 'Edit source profiles', href: '/profiles', hint: 'Tune builder, YAML, and revisions' },
    ],
    recent: [
      { label: 'Recent events feed', detail: 'Jump back into the newest message stream and patient timeline.' },
      { label: 'HL7 preview workspace', detail: 'Resume parsing, warnings, and sample inbox triage.' },
      { label: 'Workflow builder', detail: 'Continue authoring routing logic and dry-run edits.' },
    ],
  },
  events: {
    view: 'events',
    eyebrow: 'Event operations',
    title: 'Events workbench',
    description:
      'Use the sidebar to pivot between the event browser, timeline, and routing surfaces without losing your place.',
    highlights: ['Browser', 'Stream', 'Timeline'],
    actions: [
      { label: 'Open patient timeline', href: '/events', hint: 'Focus on a single patient journey' },
      { label: 'Jump to HL7 preview', href: '/hl7', hint: 'Trace an event back to raw input' },
      { label: 'Review workflows', href: '/workflows', hint: 'See where events are routed next' },
    ],
    recent: [
      { label: 'Event browser', detail: 'Browse and filter the current event feed.' },
      { label: 'Live event stream', detail: 'Watch newly processed events land in real time.' },
      { label: 'Patient timeline', detail: 'Follow a correlated patient narrative across feeds.' },
    ],
  },
  hl7: {
    view: 'hl7',
    eyebrow: 'Parser triage',
    title: 'HL7 preview',
    description:
      'Keep parsing, warnings, sample inboxes, and profile tuning close together while you work through messy interface traffic.',
    highlights: ['Samples', 'Warnings', 'Inspector'],
    actions: [
      { label: 'Open profile builder', href: '/profiles', hint: 'Refine profile rules and identifiers' },
      { label: 'Inspect terminology', href: '/terminology', hint: 'Check mappings and autoroute decisions' },
      { label: 'Review event output', href: '/events', hint: 'See what landed downstream' },
    ],
    recent: [
      { label: 'Sample inbox', detail: 'Reload sample messages and source overrides.' },
      { label: 'HL7 inspector', detail: 'Drill into segments, paths, and selected values.' },
      { label: 'Profile draft panel', detail: 'Tune tolerances, identifiers, and event rules.' },
    ],
  },
  profiles: {
    view: 'profiles',
    eyebrow: 'Source profile studio',
    title: 'Profiles workspace',
    description:
      'Pair the builder with YAML and revision history so edits stay auditable and easy to recover.',
    highlights: ['Builder', 'YAML', 'Revisions'],
    actions: [
      { label: 'Open HL7 preview', href: '/hl7', hint: 'Validate profile changes against sample traffic' },
      { label: 'Check terminology mapping', href: '/terminology', hint: 'Verify code system routes' },
      { label: 'Review workflows', href: '/workflows', hint: 'See how profile output is consumed' },
    ],
    recent: [
      { label: 'Identifier editor', detail: 'Normalize PID-3 repetitions and assigning authorities.' },
      { label: 'Tolerance editor', detail: 'Adjust recoverable parse and validation behavior.' },
      { label: 'YAML revisions', detail: 'Track the latest saved profile snapshot and history.' },
    ],
  },
  terminology: {
    view: 'terminology',
    eyebrow: 'Mapping review',
    title: 'Terminology map',
    description:
      'Move between the browser, autoroute resolver, pending review queue, and workflow traces without hunting for the next step.',
    highlights: ['Browser', 'Review', 'Autoroute', 'Trace'],
    actions: [
      { label: 'Open profiles', href: '/profiles', hint: 'Check source profile identifiers and rules' },
      { label: 'Review workflows', href: '/workflows', hint: 'See downstream routing and retry surfaces' },
      { label: 'Check events', href: '/events', hint: 'Confirm mapped codes in the event stream' },
    ],
    recent: [
      { label: 'Mapping browser', detail: 'Browse code system mappings and equivalence values.' },
      { label: 'Pending review', detail: 'Inspect high-confidence suggestions before approval.' },
      { label: 'Autoroute resolver', detail: 'Study mapping traces, candidates, and decisions.' },
    ],
  },
  workflows: {
    view: 'workflows',
    eyebrow: 'Flow orchestration',
    title: 'Workflow builder',
    description:
      'Keep the authoring, dry-run, and monitoring surfaces nearby so routing changes stay easy to reason about.',
    highlights: ['Builder', 'Dry run', 'Monitor'],
    actions: [
      { label: 'Open HL7 preview', href: '/hl7', hint: 'Test workflow inputs against parsed messages' },
      { label: 'Check event history', href: '/events', hint: 'See how routed events landed downstream' },
      { label: 'Review terminology', href: '/terminology', hint: 'Inspect code mapping prerequisites' },
    ],
    recent: [
      { label: 'Workflow builder', detail: 'Shape routes, transforms, and action chains.' },
      { label: 'Dry run panel', detail: 'Preview a change before it hits production traffic.' },
      { label: 'Workflow monitor', detail: 'Inspect execution status, retries, and health.' },
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
  if (normalized.startsWith('/events')) return 'events';
  if (normalized.startsWith('/hl7')) return 'hl7';
  if (normalized.startsWith('/profiles')) return 'profiles';
  if (normalized.startsWith('/terminology')) return 'terminology';
  if (normalized.startsWith('/workflows')) return 'workflows';
  return 'home';
}

export function getSidebarContext(pathname: string): SidebarContext {
  return contexts[getSidebarView(pathname)];
}

export function getSidebarViewLinks(): SidebarViewLink[] {
  return viewLinks.slice();
}
