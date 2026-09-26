/**
 * One glyph per view and per document type, shared by the activity bar and
 * the editor tabs. Categories get an icon and a label, never a colour.
 */
import Activity from '@lucide/svelte/icons/activity';
import Bug from '@lucide/svelte/icons/bug';
import FileInput from '@lucide/svelte/icons/file-input';
import Languages from '@lucide/svelte/icons/languages';
import LayoutDashboard from '@lucide/svelte/icons/layout-dashboard';
import ServerCog from '@lucide/svelte/icons/server-cog';
import SlidersHorizontal from '@lucide/svelte/icons/sliders-horizontal';
import Workflow from '@lucide/svelte/icons/workflow';
import Zap from '@lucide/svelte/icons/zap';
import type { IconComponent } from '$lib/ui/primitives';
import type { DocumentType, IDEView } from './types';

export const VIEW_ICONS: Record<IDEView, IconComponent> = {
  system: LayoutDashboard,
  hl7: FileInput,
  profiles: SlidersHorizontal,
  terminology: Languages,
  workflows: Workflow,
  events: Zap,
  operator: ServerCog,
};

export const DOCUMENT_ICONS: Record<Exclude<DocumentType, 'route'>, IconComponent> = {
  'workflow-draft': Workflow,
  'debug-session': Bug,
  trace: Activity,
  event: Zap,
  profile: SlidersHorizontal,
};

export const DOCUMENT_TYPE_LABELS: Record<Exclude<DocumentType, 'route'>, string> = {
  'workflow-draft': 'Workflow draft',
  'debug-session': 'Debug session',
  trace: 'Trace',
  event: 'Event',
  profile: 'Source profile',
};
