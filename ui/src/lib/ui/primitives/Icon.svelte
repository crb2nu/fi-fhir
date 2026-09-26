<!--
  Icon — renders a Lucide icon at the system defaults (16px, 1.75 stroke).
  Decorative by default (aria-hidden); pass `label` when the icon alone carries
  meaning. No emoji, no sparkle glyphs: every icon in the UI comes through here.
-->
<script lang="ts">
  import type { ClassValue } from 'svelte/elements';
  import type { IconComponent } from './types';

  interface Props {
    icon: IconComponent;
    size?: number;
    strokeWidth?: number;
    /** Accessible name. Omit for decorative icons. */
    label?: string | undefined;
    class?: ClassValue | undefined;
    style?: string | undefined;
    [dataAttribute: `data-${string}`]: unknown;
  }

  let {
    icon: IconGlyph,
    size = 16,
    strokeWidth = 1.75,
    label,
    class: className,
    ...rest
  }: Props = $props();
</script>

<IconGlyph
  {...rest}
  {size}
  {strokeWidth}
  class={['ui-icon', className]}
  role={label ? 'img' : undefined}
  aria-label={label}
  aria-hidden={label ? undefined : 'true'}
/>

<style>
  :global(.ui-icon) {
    flex: 0 0 auto;
    display: inline-block;
    vertical-align: middle;
  }
</style>
