<!--
  Reason-required dialog for every connection catalog write (create, save,
  compile, archive), modelled on the operator's ControlReasonDialog: a 40 px
  header, one Reason field, the failure of the last attempt inside the dialog,
  md buttons, and focus kept inside while open. The catalog refuses a write
  without a reason and records it with the caller's verified identity, so the
  confirm control is disabled with the reason in its title rather than allowed
  to fire and be refused.
-->
<script lang="ts">
  import { tick } from 'svelte';
  import X from '@lucide/svelte/icons/x';
  import { Button, Field, IconButton, Textarea } from '$lib/ui/primitives';
  import { createDialogFocusController, type DialogFocusController } from '$lib/domain/a11yDialog';
  import { MAX_REASON_BYTES, reasonProblem } from './editBuffer';

  interface Props {
    open: boolean;
    title: string;
    description?: string | undefined;
    confirmText: string;
    variant?: 'primary' | 'danger' | undefined;
    loading?: boolean | undefined;
    /** Failure of the last attempt, rendered inside the dialog. */
    submitError?: string | null | undefined;
    /** Offer Reload next to the failure (a version conflict or an archived connection). */
    staleView?: boolean | undefined;
    onconfirm: (reason: string) => void;
    oncancel: () => void;
    onreload?: (() => void) | undefined;
  }

  let {
    open,
    title,
    description,
    confirmText,
    variant = 'primary',
    loading = false,
    submitError = null,
    staleView = false,
    onconfirm,
    oncancel,
    onreload
  }: Props = $props();

  const uid = $props.id();
  const titleId = `${uid}-title`;
  const descriptionId = `${uid}-description`;

  let reason = $state('');
  let attempted = $state(false);
  let dialogEl: HTMLDivElement | undefined = $state();
  let focus: DialogFocusController | null = null;

  const issue = $derived(reasonProblem(reason));

  $effect(() => {
    if (!open) return;
    reason = '';
    attempted = false;
    void tick().then(() => {
      if (!dialogEl) return;
      focus = createDialogFocusController(dialogEl);
      focus.focusInitial();
    });
    return () => {
      focus?.restoreFocus();
      focus = null;
    };
  });

  function confirm(): void {
    attempted = true;
    if (issue !== null || loading) return;
    onconfirm(reason.trim());
  }

  function onKeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      event.preventDefault();
      if (!loading) oncancel();
      return;
    }
    focus?.onKeydown(event);
  }
</script>

{#if open}
  <div
    class="backdrop"
    role="presentation"
    onclick={(event) => {
      if (event.target === event.currentTarget && !loading) oncancel();
    }}
    onkeydown={onKeydown}
  >
    <div
      class="dialog"
      bind:this={dialogEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      aria-describedby={description ? descriptionId : undefined}
      data-testid="connection-reason-dialog"
    >
      <header class="dialog-head">
        <h2 id={titleId} class="title">{title}</h2>
        <IconButton icon={X} label="Close dialog" onclick={oncancel} disabled={loading} tabindex={-1} />
      </header>

      <div class="dialog-body">
        {#if description}
          <p id={descriptionId} class="description">{description}</p>
        {/if}

        <Field
          label="Reason"
          required
          hint="Recorded with your verified identity on the connection."
          error={attempted ? issue : undefined}
        >
          <Textarea
            rows={3}
            maxlength={MAX_REASON_BYTES}
            bind:value={reason}
            placeholder="Why is this change needed?"
          />
        </Field>

        {#if submitError}
          <p class="submit-error" role="alert">{submitError}</p>
        {/if}
      </div>

      <footer class="actions">
        <Button variant="ghost" size="md" onclick={oncancel} disabled={loading}>
          {submitError ? 'Close' : 'Cancel'}
        </Button>
        {#if staleView && onreload}
          <Button size="md" onclick={onreload} disabled={loading}>Reload connection</Button>
        {/if}
        <Button
          {variant}
          size="md"
          onclick={confirm}
          disabled={issue !== null || loading}
          {loading}
          title={issue ?? undefined}
        >
          {confirmText}
        </Button>
      </footer>
    </div>
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-4);
    background: var(--modal-backdrop);
    z-index: var(--z-modal);
  }

  .dialog {
    display: flex;
    flex-direction: column;
    width: min(32rem, 100%);
    max-height: 90vh;
    overflow: hidden;
    background: var(--color-bg-overlay);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-xl);
  }

  .dialog-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    height: 40px;
    padding: 0 var(--space-2) 0 var(--space-4);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .title {
    flex: 1 1 auto;
    margin: 0;
    font-family: var(--font-ui);
    font-size: var(--text-lg);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .dialog-body {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-4);
    overflow-y: auto;
  }

  .description {
    margin: 0;
    font-size: var(--text-ui);
    line-height: var(--leading-ui);
    color: var(--color-text-secondary);
  }

  .submit-error {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--color-danger-border);
    border-radius: var(--radius-sm);
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    border-top: 1px solid var(--color-border-subtle);
  }
</style>
