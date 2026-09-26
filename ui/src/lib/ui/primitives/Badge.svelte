<!--
  Badge — subtle 18px label. Tone describes *state* (success/warning/danger/
  info) or nothing (neutral); `accent` is for selection counts only. `mono`
  for counts and ids. `dot` prefixes a status dot. Not a pill: radius 4px.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';
  import type { BadgeTone } from './types';

  interface Props extends HTMLAttributes<HTMLSpanElement> {
    tone?: BadgeTone;
    mono?: boolean;
    dot?: boolean;
    children?: Snippet;
  }

  let {
    tone = 'neutral',
    mono = false,
    dot = false,
    class: className,
    children,
    ...rest
  }: Props = $props();
</script>

<span
  {...rest}
  class={['ui-badge', `ui-badge--${tone}`, { 'ui-badge--mono': mono }, className]}
  data-tone={tone}
>
  {#if dot}
    <span class="ui-badge-dot" aria-hidden="true"></span>
  {/if}
  {@render children?.()}
</span>

<style>
  .ui-badge {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    height: var(--badge-height);
    padding: 0 var(--badge-padding-x);
    border-radius: var(--badge-radius);
    font-size: var(--text-label);
    font-weight: var(--font-medium);
    line-height: 1;
    white-space: nowrap;
    vertical-align: middle;
  }

  .ui-badge--mono {
    font-family: var(--font-mono);
    font-variant-numeric: tabular-nums;
    font-weight: var(--font-normal);
  }

  .ui-badge-dot {
    width: 6px;
    height: 6px;
    border-radius: var(--radius-full);
    background: currentColor;
  }

  .ui-badge--neutral {
    background: var(--color-bg-active);
    color: var(--color-text-secondary);
  }

  .ui-badge--accent {
    background: var(--color-primary-muted);
    color: var(--color-accent-text);
  }

  .ui-badge--success {
    background: var(--color-success-bg);
    color: var(--color-success-text);
  }

  .ui-badge--warning {
    background: var(--color-warning-bg);
    color: var(--color-warning-text);
  }

  .ui-badge--danger {
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
  }

  .ui-badge--info {
    background: var(--color-info-bg);
    color: var(--color-info-text);
  }
</style>
