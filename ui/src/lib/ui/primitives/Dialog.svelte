<!--
  Dialog — a modal surface: a 40 px header (title, close), a padded body and
  an optional footer of md buttons, on the overlay colour with the one
  elevation dialogs are allowed. Labelled by its title, described by
  `description` when given. While open, Tab and Shift+Tab stay inside; Escape
  and a click on the backdrop close it (unless `dismissible` is false, e.g.
  while a write is in flight); closing returns focus to the element that had
  it before the dialog opened.

  Controlled: with `onclose` the owner decides what closing means; without
  it the dialog sets `open` to false (bind:open).

    <Dialog bind:open title="Close Profiles?" description="…">
      {#snippet footer()}
        <Button size="md" variant="ghost" onclick={() => (open = false)}>Cancel</Button>
        <Button size="md" variant="danger" onclick={closeTab}>Close tab</Button>
      {/snippet}
    </Dialog>

  `layout="bare"` drops the header, body padding and footer for a surface
  that lays itself out (the command palette); the title is then read by
  assistive tech only.
-->
<script lang="ts">
  import { tick, type Snippet } from 'svelte';
  import type { HTMLAttributes } from 'svelte/elements';
  import X from '@lucide/svelte/icons/x';
  import { createDialogFocusController, type DialogFocusController } from '$lib/domain/a11yDialog';
  import IconButton from './IconButton.svelte';

  interface Props extends Omit<HTMLAttributes<HTMLDivElement>, 'title' | 'role'> {
    open?: boolean;
    /** Accessible name; the header's h2 (visually hidden in `bare`). */
    title: string;
    /** One sentence under the header; becomes aria-describedby. */
    description?: string | undefined;
    /** Width: sm 400 px, md 560 px, lg 720 px. */
    size?: 'sm' | 'md' | 'lg';
    /** Centred, 64 px from the top (palette), or a full-height left drawer. */
    placement?: 'center' | 'top' | 'left';
    layout?: 'default' | 'bare';
    /** Escape, the backdrop and the close button close it. Off while a write is in flight. */
    dismissible?: boolean;
    /** CSS selector, inside the dialog, of the element to focus on open. */
    initialFocus?: string | undefined;
    /** `alertdialog` for confirmations that interrupt the user. */
    role?: 'dialog' | 'alertdialog';
    onclose?: (() => void) | undefined;
    children?: Snippet;
    footer?: Snippet;
  }

  let {
    open = $bindable(false),
    title,
    description,
    size = 'md',
    placement = 'center',
    layout = 'default',
    dismissible = true,
    initialFocus,
    role = 'dialog',
    onclose,
    children,
    footer,
    class: className,
    ...rest
  }: Props = $props();

  const uid = $props.id();
  const titleId = `${uid}-title`;
  const descriptionId = `${uid}-description`;

  let dialogEl: HTMLDivElement | undefined = $state();
  let focus: DialogFocusController | null = null;

  $effect(() => {
    if (!open) return;
    // Remember the opener now, before anything inside takes focus.
    const opener = typeof document !== 'undefined' ? (document.activeElement as HTMLElement | null) : null;
    void tick().then(() => {
      if (!dialogEl) return;
      const target = initialFocus ? dialogEl.querySelector<HTMLElement>(initialFocus) : null;
      focus = createDialogFocusController(dialogEl, { initialFocus: target });
      focus.focusInitial();
    });
    return () => {
      focus = null;
      if (opener && opener.isConnected) opener.focus();
    };
  });

  function close(): void {
    if (onclose) onclose();
    else open = false;
  }

  function onKeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      // Consumed here, so page-level Escape handlers (which check
      // defaultPrevented) leave their own state alone.
      event.preventDefault();
      event.stopPropagation();
      if (dismissible) close();
      return;
    }
    if (event.key === 'Tab') focus?.onKeydown(event);
  }

  function onBackdropClick(event: MouseEvent): void {
    if (event.target === event.currentTarget && dismissible) close();
  }
</script>

{#if open}
  <div
    class="ui-dialog-overlay"
    class:is-top={placement === 'top'}
    class:is-left={placement === 'left'}
    role="presentation"
    onclick={onBackdropClick}
    onkeydown={onKeydown}
  >
    <div
      {...rest}
      bind:this={dialogEl}
      class={['ui-dialog', `ui-dialog--${size}`, { 'ui-dialog--bare': layout === 'bare' }, className]}
      {role}
      aria-modal="true"
      aria-labelledby={titleId}
      aria-describedby={description ? descriptionId : undefined}
      tabindex="-1"
    >
      {#if layout === 'bare'}
        <h2 id={titleId} class="ui-dialog-sr-only">{title}</h2>
        {#if description}
          <p id={descriptionId} class="ui-dialog-sr-only">{description}</p>
        {/if}
        {@render children?.()}
      {:else}
        <header class="ui-dialog-head">
          <h2 id={titleId} class="ui-dialog-title">{title}</h2>
          <IconButton icon={X} label="Close dialog" onclick={close} disabled={!dismissible} tabindex={-1} />
        </header>
        <div class="ui-dialog-body">
          {#if description}
            <p id={descriptionId} class="ui-dialog-description">{description}</p>
          {/if}
          {@render children?.()}
        </div>
        {#if footer}
          <footer class="ui-dialog-actions">
            {@render footer()}
          </footer>
        {/if}
      {/if}
    </div>
  </div>
{/if}

<style>
  .ui-dialog-overlay {
    position: fixed;
    inset: 0;
    z-index: var(--z-modal);
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-4);
    background: var(--modal-backdrop);
  }

  .ui-dialog-overlay.is-top {
    align-items: flex-start;
    padding-top: 64px;
  }

  .ui-dialog-overlay.is-left {
    justify-content: flex-start;
    padding: 0;
  }

  .is-left .ui-dialog {
    width: min(320px, calc(100vw - 40px));
    height: 100dvh;
    max-height: 100dvh;
    border-radius: 0;
  }

  .ui-dialog {
    display: flex;
    flex-direction: column;
    width: 100%;
    max-height: 90vh;
    overflow: hidden;
    background: var(--color-bg-overlay);
    border: 1px solid var(--color-border-default);
    border-radius: var(--modal-radius, var(--radius-md));
    box-shadow: var(--shadow-xl);
    color: var(--color-text-primary);
    outline: none;
  }

  .ui-dialog--sm {
    max-width: var(--modal-width-sm);
  }

  .ui-dialog--md {
    max-width: var(--modal-width-md);
  }

  .ui-dialog--lg {
    max-width: var(--modal-width-lg);
  }

  .ui-dialog--bare {
    max-height: min(480px, calc(100vh - 96px));
  }

  .ui-dialog-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 0 0 auto;
    height: 40px;
    padding: 0 var(--space-2) 0 var(--space-4);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .ui-dialog-title {
    flex: 1 1 auto;
    min-width: 0;
    margin: 0;
    overflow: hidden;
    font-family: var(--font-ui);
    font-size: var(--text-lg);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .ui-dialog-body {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-4);
    overflow-y: auto;
  }

  .ui-dialog-description {
    margin: 0;
    font-size: var(--text-ui);
    line-height: var(--leading-ui);
    color: var(--color-text-secondary);
  }

  .ui-dialog-actions {
    display: flex;
    flex: 0 0 auto;
    justify-content: flex-end;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    border-top: 1px solid var(--color-border-subtle);
  }

  .ui-dialog-sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    margin: -1px;
    padding: 0;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
</style>
