import type { Component } from 'svelte';
import type { LucideProps } from '@lucide/svelte';

/**
 * A Lucide icon component. Import icons by deep path so dev and tests load one
 * module, not the whole set:
 *
 *   import Play from '@lucide/svelte/icons/play';
 *   <Icon icon={Play} />
 */
export type IconComponent = Component<LucideProps>;

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger';
export type ControlSize = 'sm' | 'md';

export type BadgeTone = 'neutral' | 'accent' | 'success' | 'warning' | 'danger' | 'info';

export interface TabItem {
  /** Stable key; `value` of the Tabs is one of these. */
  id: string;
  label: string;
  /** Optional count rendered as a mono badge after the label. */
  count?: number | undefined;
  disabled?: boolean | undefined;
  icon?: IconComponent | undefined;
  /** id of the tabpanel this tab controls (sets aria-controls). */
  controls?: string | undefined;
  /** data-testid for this tab button. */
  testid?: string | undefined;
}

export interface SelectOption {
  value: string;
  label: string;
  disabled?: boolean | undefined;
}

export interface KeyValueItem {
  key: string;
  value: string | number | null | undefined;
  /** Render the value in the monospace register (ids, hashes, numbers). */
  mono?: boolean | undefined;
  /** Single line with ellipsis; the full value goes in `title`. */
  truncate?: boolean | undefined;
}

export type SortDirection = 'ascending' | 'descending' | 'none';

export type PopoverPlacement = 'bottom-start' | 'bottom-end' | 'top-start' | 'top-end';

/** Props the Popover hands its trigger snippet; spread them on the button. */
export interface PopoverTriggerProps {
  'aria-expanded': boolean;
  'aria-controls': string;
  'aria-haspopup': 'dialog';
  onclick: () => void;
}
