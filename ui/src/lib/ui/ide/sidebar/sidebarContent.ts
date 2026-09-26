import type { IDEAppRoute } from '../types';
import { getJourneyState } from '../journey';

export type SidebarView = 'home' | 'events' | 'hl7' | 'profiles' | 'terminology' | 'workflows' | 'operator';

export type SidebarAction = {
  label: string;
  href: IDEAppRoute;
  hint: string;
};

export type SidebarViewLink = {
  view: SidebarView;
  label: string;
  href: IDEAppRoute;
};

export interface SidebarContext {
  view: SidebarView;
  title: string;
  /** One sentence, system voice: what this view is for. */
  description: string;
  /** Related views, each with a one-line reason to go there. */
  actions: SidebarAction[];
}

// Domain-first navigation, same labels and order as the activity bar.
const viewLinks: SidebarViewLink[] = [
  { view: 'home', label: 'Dashboard', href: '/' },
  { view: 'hl7', label: 'HL7 / Intake', href: '/hl7' },
  { view: 'profiles', label: 'Profiles', href: '/profiles' },
  { view: 'terminology', label: 'Terminology', href: '/terminology' },
  { view: 'workflows', label: 'Workflows', href: '/workflows' },
  { view: 'events', label: 'Events', href: '/events' },
  { view: 'operator', label: 'Operations', href: '/operator' },
];

const contexts: Record<SidebarView, SidebarContext> = {
  home: {
    view: 'home',
    title: 'Dashboard',
    description: 'Integration health and recent work across every stage.',
    actions: [
      { label: 'HL7 / Intake', href: '/hl7', hint: 'Load, parse and preview inbound messages.' },
      { label: 'Operations', href: '/operator', hint: 'Trace receipts and recover failed deliveries.' },
      { label: 'Events', href: '/events', hint: 'Browse the semantic events that were delivered.' },
    ],
  },
  hl7: {
    view: 'hl7',
    title: 'Source Intake',
    description: 'Load inbound messages, inspect raw payloads and review recoverable warnings.',
    actions: [
      { label: 'Profiles', href: '/profiles', hint: 'Identifier and tolerance rules for this source.' },
      { label: 'Terminology', href: '/terminology', hint: 'Code mappings applied after parsing.' },
      { label: 'Events', href: '/events', hint: 'The events a processed message produced.' },
    ],
  },
  profiles: {
    view: 'profiles',
    title: 'Normalization',
    description: 'Identifiers, tolerances and recoverable-anomaly rules in the source profile.',
    actions: [
      { label: 'HL7 / Intake', href: '/hl7', hint: 'Recheck raw messages against these rules.' },
      { label: 'Terminology', href: '/terminology', hint: 'Code system mappings.' },
      { label: 'Workflows', href: '/workflows', hint: 'Routes that consume normalized events.' },
    ],
  },
  terminology: {
    view: 'terminology',
    title: 'Translation',
    description: 'Source codes mapped to shared terms, with every decision traceable.',
    actions: [
      { label: 'Profiles', href: '/profiles', hint: 'Profile rules that feed the mappings.' },
      { label: 'Workflows', href: '/workflows', hint: 'Routing logic that uses translated codes.' },
      { label: 'Events', href: '/events', hint: 'The resulting semantic events.' },
    ],
  },
  workflows: {
    view: 'workflows',
    title: 'Delivery',
    description: 'Routes, transforms and actions that deliver normalized events to destinations.',
    actions: [
      { label: 'Terminology', href: '/terminology', hint: 'Terms the routes match on.' },
      { label: 'Events', href: '/events', hint: 'How routed outcomes landed.' },
      { label: 'HL7 / Intake', href: '/hl7', hint: 'Trace a delivery back to raw input.' },
    ],
  },
  events: {
    view: 'events',
    title: 'Verification',
    description: 'Delivered outcomes compared with source intent on the timeline.',
    actions: [
      { label: 'Workflows', href: '/workflows', hint: 'The route that delivered an event.' },
      { label: 'Terminology', href: '/terminology', hint: 'The mapping behind a semantic term.' },
      { label: 'Operations', href: '/operator', hint: 'Receipts, delivery attempts and dead letters.' },
    ],
  },
  operator: {
    view: 'operator',
    title: 'Operations',
    description: 'What production did, and audited recovery with a recorded reason.',
    actions: [
      { label: 'Workflows', href: '/workflows', hint: 'The route and actions behind a delivery.' },
      { label: 'Events', href: '/events', hint: 'Events a receipt produced.' },
      { label: 'Dashboard', href: '/', hint: 'Integration health across stages.' },
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
  if (normalized.startsWith('/operator')) return 'operator';
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
