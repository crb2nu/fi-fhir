<!--
  Panel — a flat bordered region. No shadow, radius 4px. Optional 32px header
  with a label-register title and right-aligned actions. `flush` drops the body
  padding (for a Table or an editor that owns its own edges).
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';

  interface Props extends Omit<HTMLAttributes<HTMLElement>, 'title'> {
    title?: string | undefined;
    /** Heading element for the title; pick the level that fits the page outline. */
    titleTag?: 'h2' | 'h3' | 'h4';
    /** Custom header content (replaces `title`). */
    header?: Snippet | undefined;
    /** Right-aligned header actions (IconButtons, a ghost Button). */
    actions?: Snippet | undefined;
    flush?: boolean;
    children?: Snippet;
  }

  let {
    title,
    titleTag = 'h2',
    header,
    actions,
    flush = false,
    class: className,
    children,
    ...rest
  }: Props = $props();

  const titleId = $props.id();
  const hasHeader = $derived(Boolean(title || header || actions));
</script>

<section
  {...rest}
  class={['ui-panel', className]}
  aria-labelledby={title ? titleId : undefined}
>
  {#if hasHeader}
    <div class="ui-panel-header">
      {#if header}
        {@render header()}
      {:else if title}
        <svelte:element this={titleTag} id={titleId} class="ui-panel-title">{title}</svelte:element>
      {/if}
      {#if actions}
        <div class="ui-panel-actions">{@render actions()}</div>
      {/if}
    </div>
  {/if}
  <div class="ui-panel-body" class:is-flush={flush}>
    {@render children?.()}
  </div>
</section>

<style>
  .ui-panel {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
    background: var(--color-bg-elevated);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
  }

  .ui-panel-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 0 0 auto;
    height: 32px;
    padding: 0 var(--space-1) 0 var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .ui-panel-title {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-ui);
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .ui-panel-actions {
    display: flex;
    align-items: center;
    gap: 2px;
    margin-left: auto;
  }

  .ui-panel-body {
    flex: 1 1 auto;
    min-height: 0;
    padding: var(--panel-padding);
  }

  .ui-panel-body.is-flush {
    padding: 0;
  }
</style>
