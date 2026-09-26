/**
 * One glyph per view, shared by the activity bar, the editor tabs and the
 * sidebar. Categories get an icon and a label, never a colour.
 */
import FileInput from '@lucide/svelte/icons/file-input';
import Languages from '@lucide/svelte/icons/languages';
import LayoutDashboard from '@lucide/svelte/icons/layout-dashboard';
import ServerCog from '@lucide/svelte/icons/server-cog';
import SlidersHorizontal from '@lucide/svelte/icons/sliders-horizontal';
import Workflow from '@lucide/svelte/icons/workflow';
import Zap from '@lucide/svelte/icons/zap';
import type { IconComponent } from '$lib/ui/primitives';
import type { IDEView } from './types';

export const VIEW_ICONS: Record<IDEView, IconComponent> = {
  system: LayoutDashboard,
  hl7: FileInput,
  profiles: SlidersHorizontal,
  terminology: Languages,
  workflows: Workflow,
  events: Zap,
  operator: ServerCog,
};
