<script lang="ts">
  /**
   * Reason-required dialog for every operator control action.
   *
   * The backend refuses any control action without a nonempty actor reason and
   * records it in the append-only audit trail, so this dialog is the only way
   * into replay/resubmit/discard and the lifecycle commands. Validation is
   * inline (toast-budget B1) and the confirm control is disabled with an
   * explanatory title rather than allowed to fire and be rejected (B2).
   */

  import { createEventDispatcher } from 'svelte';
  import { afterUpdate, tick } from 'svelte';
  import X from '@lucide/svelte/icons/x';
  import { Button, Field, IconButton, Input, Textarea } from '$lib/ui/primitives';
  import { createDialogFocusController } from '$lib/domain/a11yDialog';
  import {
    MAX_REASON_LENGTH,
    controlDraftReady,
    deriveIdempotencyKey,
    validateControlDraft
  } from './controlValidation';

  export let open = false;
  export let title = 'Confirm operator action';
  export let description = '';
  export let confirmText = 'Confirm';
  export let variant: 'primary' | 'danger' = 'primary';
  export let loading = false;
  /** Requires an idempotency key (delivery recovery) vs. not (lifecycle). */
  export let requiresIdempotencyKey = true;
  /** Identifies the intent so a repeat of the same action derives the same key. */
  export let action = '';
  export let targetId = '';
  /** Inline failure from the last submission, rendered inside the dialog. */
  export let submitError: string | null = null;

  const dispatch = createEventDispatcher<{
    confirm: { reason: string; idempotencyKey: string };
    cancel: void;
  }>();

  let reason = '';
  let idempotencyKey = '';
  let keyTouched = false;
  let attempted = false;
  let dialogEl: HTMLDivElement | null = null;
  let wasOpen = false;
  let focusCtl: ReturnType<typeof createDialogFocusController> | null = null;
  // Stable per dialog opening so retrying the identical intent reuses one key.
  let nonce = '';

  $: issues = validateControlDraft({ reason, idempotencyKey });
  $: ready = controlDraftReady({ reason, idempotencyKey });
  $: derivedKey = requiresIdempotencyKey
    ? deriveIdempotencyKey(action, targetId, reason, nonce)
    : '';
  $: if (requiresIdempotencyKey && !keyTouched) {
    idempotencyKey = derivedKey;
  }
  $: confirmBlockedReason = ready
    ? undefined
    : (issues.reason ?? issues.idempotencyKey ?? undefined);

  afterUpdate(() => {
    if (open && !wasOpen) {
      reason = '';
      keyTouched = false;
      attempted = false;
      nonce = Math.random().toString(36).slice(2, 10);
      tick().then(() => {
        if (!dialogEl) return;
        focusCtl = createDialogFocusController(dialogEl);
        focusCtl.focusInitial();
      });
    }
    if (!open && wasOpen) {
      focusCtl?.restoreFocus();
      focusCtl = null;
    }
    wasOpen = open;
  });

  function handleConfirm() {
    attempted = true;
    if (!ready) return;
    dispatch('confirm', { reason: reason.trim(), idempotencyKey });
  }

  function handleCancel() {
    dispatch('cancel');
  }

  function handleKeydown(event: KeyboardEvent) {
    if (event.key === 'Escape') {
      event.preventDefault();
      handleCancel();
    }
  }
</script>

{#if open}
  <div
    class="backdrop"
    role="presentation"
    on:click|self={handleCancel}
    on:keydown={handleKeydown}
  >
    <div
      class="dialog"
      bind:this={dialogEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby="control-dialog-title"
      aria-describedby={description ? 'control-dialog-description' : undefined}
    >
      <header class="dialog-head">
        <h2 id="control-dialog-title" class="title">{title}</h2>
        <IconButton icon={X} label="Close dialog" onclick={handleCancel} disabled={loading} tabindex={-1} />
      </header>

      <div class="dialog-body">
        {#if description}
          <p id="control-dialog-description" class="description">{description}</p>
        {/if}

        <Field
          label="Reason"
          id="control-reason"
          required
          hint="Recorded with your verified identity in the append-only audit trail."
          error={attempted ? issues.reason : undefined}
        >
          <Textarea
            rows={3}
            maxlength={MAX_REASON_LENGTH}
            bind:value={reason}
            placeholder="Why is this action necessary?"
          />
        </Field>

        {#if requiresIdempotencyKey}
          <Field
            label="Idempotency key"
            id="control-key"
            hint="Derived from this action and reason. Repeating the identical request is a no-op."
            error={issues.idempotencyKey}
          >
            <Input bind:value={idempotencyKey} mono oninput={() => (keyTouched = true)} />
          </Field>
        {/if}

        {#if submitError}
          <p class="submit-error" role="alert">{submitError}</p>
        {/if}
      </div>

      <footer class="actions">
        <Button variant="ghost" size="md" onclick={handleCancel} disabled={loading}>Cancel</Button>
        <Button
          {variant}
          size="md"
          onclick={handleConfirm}
          disabled={!ready || loading}
          {loading}
          title={confirmBlockedReason}
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
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    border-top: 1px solid var(--color-border-subtle);
  }
</style>
