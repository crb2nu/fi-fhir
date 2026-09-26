/**
 * UI primitives — the building blocks every route is assembled from.
 * Rules and usage: ui/docs/DESIGN.md. Gallery (dev only): /design.
 *
 * These are Svelte 5 runes components: pass event handlers as props
 * (`onclick={…}`), not `on:click`, and content as snippets. They work from
 * legacy-syntax parents too (see __fixtures__/LegacyConsumer.svelte).
 *
 * Icons: import each glyph by deep path and render it through `Icon`:
 *   import Play from '@lucide/svelte/icons/play';
 *   <Icon icon={Play} />            // 16px, stroke 1.75
 */
export { default as Badge } from './Badge.svelte';
export { default as Button } from './Button.svelte';
export { default as EmptyState } from './EmptyState.svelte';
export { default as Field } from './Field.svelte';
export { default as Icon } from './Icon.svelte';
export { default as IconButton } from './IconButton.svelte';
export { default as Input } from './Input.svelte';
export { default as KeyValue } from './KeyValue.svelte';
export { default as Panel } from './Panel.svelte';
export { default as Popover } from './Popover.svelte';
export { default as Select } from './Select.svelte';
export { default as Table } from './Table.svelte';
export { default as Tabs } from './Tabs.svelte';
export { default as Td } from './Td.svelte';
export { default as Textarea } from './Textarea.svelte';
export { default as Th } from './Th.svelte';
export { default as Toolbar } from './Toolbar.svelte';
export { default as Tr } from './Tr.svelte';

export type {
  BadgeTone,
  ButtonVariant,
  ControlSize,
  IconComponent,
  KeyValueItem,
  PopoverPlacement,
  PopoverTriggerProps,
  SelectOption,
  SortDirection,
  TabItem
} from './types';
