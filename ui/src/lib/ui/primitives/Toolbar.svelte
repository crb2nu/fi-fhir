<!--
  Toolbar — the top of every route: title · tabs · actions (right-aligned),
  36px, one bottom border. The title is the page's heading (h1 by default) at
  15px semibold — never a hero, never a subtitle. Contextual help belongs in an
  actions-slot IconButton that opens a Popover, not in copy on the page.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';

  interface Props extends Omit<HTMLAttributes<HTMLElement>, 'title'> {
    title?: string | undefined;
    titleTag?: 'h1' | 'h2';
    /** Custom title content (replaces `title`), e.g. a breadcrumb. */
    heading?: Snippet | undefined;
    /** Underline Tabs for the route's views. */
    tabs?: Snippet | undefined;
    /** Right-aligned actions: one primary Button at most, then ghost/secondary. */
    actions?: Snippet | undefined;
    children?: Snippet;
  }

  let {
    title,
    titleTag = 'h1',
    heading,
    tabs,
    actions,
    class: className,
    children,
    ...rest
  }: Props = $props();
</script>

<header {...rest} class={['ui-toolbar', className]}>
  {#if heading}
    <div class="ui-toolbar-title">{@render heading()}</div>
  {:else if title}
    <svelte:element this={titleTag} class="ui-toolbar-title">{title}</svelte:element>
  {/if}
  {#if tabs}
    <div class="ui-toolbar-tabs">{@render tabs()}</div>
  {/if}
  {@render children?.()}
  {#if actions}
    <div class="ui-toolbar-actions">{@render actions()}</div>
  {/if}
</header>

<style>
  .ui-toolbar {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    flex: 0 0 auto;
    height: var(--toolbar-height);
    min-width: 0;
    padding: 0 var(--space-3);
    background: var(--color-bg-base);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .ui-toolbar-title {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 0 0 auto;
    min-width: 0;
    margin: 0;
    font-family: var(--font-ui);
    font-size: var(--text-title);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-normal);
    line-height: 1;
    white-space: nowrap;
    color: var(--color-text-primary);
  }

  .ui-toolbar-tabs {
    display: flex;
    align-self: stretch;
    min-width: 0;
  }

  .ui-toolbar-actions {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-left: auto;
    flex: 0 0 auto;
  }
</style>
