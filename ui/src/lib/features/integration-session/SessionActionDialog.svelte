<!--
  The session sidebar's confirm dialog (.loom/42 E-3), in the repo's reason
  dialog idiom (ControlReasonDialog, ConnectionReasonDialog): a 40 px header,
  the failure of the last attempt inside the dialog, md buttons, and focus
  kept inside while open.

  Two uses:
  - Export: a Reason (required, recorded with the verified caller on the
    export row) and, only when the identity holds integration.phi.export, the
    raw-payload option; otherwise the sentence naming the missing role.
  - Archive: no Reason field, because archiveIntegrationSession takes none and
    records none; the description says so instead of collecting text the
    server would drop.
-->
<script lang="ts">
  import { tick } from 'svelte';
  import X from '@lucide/svelte/icons/x';
  import { Button, Field, IconButton, Textarea } from '$lib/ui/primitives';
  import { createDialogFocusController, type DialogFocusController } from '$lib/domain/a11yDialog';
  import { MAX_EXPORT_REASON_BYTES, exportReasonProblem } from './sessionReason';

  interface Props {
    open: boolean;
    title: string;
    description?: string | undefined;
    confirmText: string;
    variant?: 'primary' | 'danger' | undefined;
    /** Collect a reason (export). Archive records none, so it asks for none. */
    reasonRequired?: boolean | undefined;
    reasonHint?: string | undefined;
    /**
     * The raw-payload option: `allowed` shows the checkbox; otherwise
     * `blockedSentence` says which role is missing. Omit for no option.
     */
    rawPayload?: { allowed: boolean; blockedSentence: string; blockedReason: string } | undefined;
    loading?: boolean | undefined;
    submitError?: string | null | undefined;
    testid: string;
    onconfirm: (reason: string, includeRawPayload: boolean) => void;
    oncancel: () => void;
  }

  let {
    open,
    title,
    description,
    confirmText,
    variant = 'primary',
    reasonRequired = true,
    reasonHint = 'Recorded with your verified identity.',
    rawPayload,
    loading = false,
    submitError = null,
    testid,
    onconfirm,
    oncancel
  }: Props = $props();

  const uid = $props.id();
  const titleId = `${uid}-title`;
  const descriptionId = `${uid}-description`;

  let reason = $state('');
  let includeRaw = $state(false);
  let attempted = $state(false);
  let dialogEl: HTMLDivElement | undefined = $state();
  let focus: DialogFocusController | null = null;

  const issue = $derived(reasonRequired ? exportReasonProblem(reason) : null);

  $effect(() => {
    if (!open) return;
    reason = '';
    includeRaw = false;
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
    onconfirm(reason.trim(), Boolean(rawPayload?.allowed && includeRaw));
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
      data-testid={testid}
    >
      <header class="dialog-head">
        <h2 id={titleId} class="title">{title}</h2>
        <IconButton icon={X} label="Close dialog" onclick={oncancel} disabled={loading} tabindex={-1} />
      </header>

      <div class="dialog-body">
        {#if description}
          <p id={descriptionId} class="description">{description}</p>
        {/if}

        {#if reasonRequired}
          <Field label="Reason" required hint={reasonHint} error={attempted ? issue : undefined}>
            <Textarea
              rows={3}
              maxlength={MAX_EXPORT_REASON_BYTES}
              bind:value={reason}
              placeholder="Why is this needed?"
            />
          </Field>
        {/if}

        {#if rawPayload}
          {#if rawPayload.allowed}
            <label class="check" data-testid="session-export-raw">
              <input type="checkbox" bind:checked={includeRaw} disabled={loading} />
              Include raw sample payloads (integration.phi.export)
            </label>
          {:else}
            <p class="blocked" data-testid="session-export-phi-missing" data-reason={rawPayload.blockedReason}>
              {rawPayload.blockedSentence}
            </p>
          {/if}
        {/if}

        {#if submitError}
          <p class="submit-error" role="alert">{submitError}</p>
        {/if}
      </div>

      <footer class="actions">
        <Button variant="ghost" size="md" onclick={oncancel} disabled={loading}>
          {submitError ? 'Close' : 'Cancel'}
        </Button>
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

  .description,
  .blocked {
    margin: 0;
    font-size: var(--text-ui);
    line-height: var(--leading-ui);
    color: var(--color-text-secondary);
  }

  .blocked {
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
  }

  .check {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-ui);
    color: var(--color-text-primary);
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
