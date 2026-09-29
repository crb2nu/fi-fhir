<!--
  WorkflowConfirmDialog — the workflows feature's confirmation dialog, which
  replaces `window.confirm` (`.loom/42` E-5). A local, minimal stand-in until
  the shell's Dialog primitive (`$lib/ui/primitives/Dialog.svelte`, lane E-4)
  lands; then this file folds into it.

  Modal, labelled by its title and described by its message; focus moves to
  Cancel when it opens, Tab stays inside, Escape cancels, and focus returns to
  whatever had it before.
-->
<script lang="ts">
  import { tick, type Snippet } from 'svelte';
  import { Button } from '$lib/ui/primitives';

  interface Props {
    open: boolean;
    title: string;
    message: string;
    confirmLabel: string;
    /** `danger` for an irreversible or destructive action. */
    tone?: 'primary' | 'danger';
    busy?: boolean;
    testid?: string;
    onconfirm: () => void;
    oncancel: () => void;
    children?: Snippet;
  }

  let {
    open,
    title,
    message,
    confirmLabel,
    tone = 'primary',
    busy = false,
    testid = 'workflow-confirm-dialog',
    onconfirm,
    oncancel,
    children
  }: Props = $props();

  const uid = $props.id();
  let surface: HTMLDivElement | null = $state(null);
  let cancelButton: HTMLElement | null = null;
  let previouslyFocused: HTMLElement | null = null;

  $effect(() => {
    if (!open) return;
    previouslyFocused = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    void tick().then(() => {
      cancelButton = surface?.querySelector<HTMLElement>('[data-dialog-cancel]') ?? null;
      cancelButton?.focus();
    });
    return () => {
      previouslyFocused?.focus?.();
    };
  });

  function focusable(): HTMLElement[] {
    if (!surface) return [];
    return Array.from(
      surface.querySelectorAll<HTMLElement>(
        'button:not([disabled]), input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])'
      )
    );
  }

  function onkeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      event.preventDefault();
      event.stopPropagation();
      if (!busy) oncancel();
      return;
    }
    if (event.key !== 'Tab') return;
    const items = focusable();
    if (items.length === 0) return;
    const first = items[0]!;
    const last = items[items.length - 1]!;
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  }
</script>

{#if open}
  <div class="backdrop" role="presentation"></div>
  <div
    bind:this={surface}
    class="dialog"
    role="dialog"
    aria-modal="true"
    aria-labelledby={`${uid}-title`}
    aria-describedby={`${uid}-message`}
    data-testid={testid}
    tabindex="-1"
    {onkeydown}
  >
    <h2 id={`${uid}-title`} class="title">{title}</h2>
    <p id={`${uid}-message`} class="message">{message}</p>
    {@render children?.()}
    <div class="actions">
      <Button data-dialog-cancel onclick={oncancel} disabled={busy}>Cancel</Button>
      <Button variant={tone} size="md" onclick={onconfirm} loading={busy}>{confirmLabel}</Button>
    </div>
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: 900;
    background: var(--modal-backdrop);
  }

  .dialog {
    position: fixed;
    top: 20vh;
    left: 50%;
    z-index: 901;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    width: min(440px, calc(100vw - 32px));
    padding: var(--space-4);
    transform: translateX(-50%);
    background: var(--color-bg-overlay);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-xl);
  }

  .dialog:focus {
    outline: none;
  }

  .title {
    margin: 0;
    font-size: var(--text-lg);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .message {
    margin: 0;
    font-size: var(--text-ui);
    line-height: var(--leading-snug);
    color: var(--color-text-secondary);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
  }
</style>
