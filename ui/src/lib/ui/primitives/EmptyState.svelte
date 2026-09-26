<!--
  EmptyState — one sentence, at most one primary action, a 16px icon. No
  illustration, no headline, no explainer paragraph. Honest states (operator
  preflight, streaming unavailable, no alert source) use this too: say what is
  true and what to do, and keep their data-testid on the root.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';
  import Button from './Button.svelte';
  import Icon from './Icon.svelte';
  import type { IconComponent } from './types';

  interface Props extends HTMLAttributes<HTMLDivElement> {
    icon?: IconComponent | undefined;
    /** The one sentence. Use `children` instead when it needs inline markup. */
    message?: string | undefined;
    actionLabel?: string | undefined;
    onaction?: (() => void) | undefined;
    /** Custom action (e.g. a link styled as a Button). Replaces actionLabel. */
    action?: Snippet | undefined;
    /** Center in the available space (default) or sit at the top-left. */
    align?: 'center' | 'start';
    children?: Snippet;
  }

  let {
    icon,
    message,
    actionLabel,
    onaction,
    action,
    align = 'center',
    class: className,
    children,
    ...rest
  }: Props = $props();
</script>

<div {...rest} class={['ui-empty', `ui-empty--${align}`, className]}>
  <p class="ui-empty-message">
    {#if icon}
      <Icon {icon} class="ui-empty-icon" />
    {/if}
    <span>
      {#if children}{@render children()}{:else}{message}{/if}
    </span>
  </p>
  {#if action}
    {@render action()}
  {:else if actionLabel && onaction}
    <Button variant="primary" onclick={onaction}>{actionLabel}</Button>
  {/if}
</div>

<style>
  .ui-empty {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-6) var(--space-4);
    color: var(--color-text-secondary);
    font-size: var(--text-ui);
  }

  .ui-empty--center {
    align-items: center;
    justify-content: center;
    text-align: center;
    min-height: 120px;
  }

  .ui-empty--start {
    align-items: flex-start;
  }

  .ui-empty-message {
    display: inline-flex;
    align-items: flex-start;
    gap: var(--space-2);
    max-width: 60ch;
    margin: 0;
    line-height: var(--leading-ui);
  }

  .ui-empty-message :global(.ui-empty-icon) {
    margin-top: 2px;
    color: var(--color-text-tertiary);
  }
</style>
