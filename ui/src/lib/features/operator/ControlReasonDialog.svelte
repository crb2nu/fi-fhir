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
  import Button from '$lib/ui/Button.svelte';
  import { createDialogFocusController } from '$lib/domain/a11yDialog';
  import { afterUpdate, tick } from 'svelte';
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
    >
      <h2 id="control-dialog-title" class="title">{title}</h2>
      {#if description}
        <p class="description">{description}</p>
      {/if}

      <label class="field-label" for="control-reason">
        Reason <span class="required" aria-hidden="true">*</span>
      </label>
      <p class="field-hint" id="control-reason-hint">
        Recorded with your verified identity in the append-only audit trail.
      </p>
      <textarea
        id="control-reason"
        class="reason"
        class:invalid={attempted && issues.reason}
        rows="3"
        maxlength={MAX_REASON_LENGTH}
        aria-describedby="control-reason-hint"
        aria-invalid={attempted && !!issues.reason}
        bind:value={reason}
        placeholder="Why is this action necessary?"
      ></textarea>
      {#if attempted && issues.reason}
        <p class="field-error" role="alert">{issues.reason}</p>
      {/if}

      {#if requiresIdempotencyKey}
        <label class="field-label" for="control-key">Idempotency key</label>
        <p class="field-hint" id="control-key-hint">
          Derived from this action and reason. Repeating the identical request is a no-op.
        </p>
        <input
          id="control-key"
          class="key"
          class:invalid={!!issues.idempotencyKey}
          type="text"
          aria-describedby="control-key-hint"
          aria-invalid={!!issues.idempotencyKey}
          bind:value={idempotencyKey}
          on:input={() => (keyTouched = true)}
        />
        {#if issues.idempotencyKey}
          <p class="field-error" role="alert">{issues.idempotencyKey}</p>
        {/if}
      {/if}

      {#if submitError}
        <p class="submit-error" role="alert">{submitError}</p>
      {/if}

      <div class="actions">
        <Button variant="secondary" on:click={handleCancel} disabled={loading}>Cancel</Button>
        <Button
          {variant}
          on:click={handleConfirm}
          disabled={!ready || loading}
          {loading}
          title={confirmBlockedReason}
        >
          {confirmText}
        </Button>
      </div>
    </div>
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    background: var(--color-bg-overlay);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: var(--z-sticky);
    padding: var(--space-4);
  }

  .dialog {
    background: var(--color-bg-elevated);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-lg);
    box-shadow: var(--shadow-lg);
    padding: var(--space-6);
    width: min(34rem, 100%);
    max-height: 90vh;
    overflow-y: auto;
  }

  .title {
    margin: 0 0 var(--space-2);
    font-family: var(--font-heading);
    font-size: var(--text-lg);
    color: var(--color-text-primary);
  }

  .description {
    margin: 0 0 var(--space-4);
    color: var(--color-text-secondary);
    font-size: var(--text-sm);
  }

  .field-label {
    display: block;
    margin-top: var(--space-3);
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--color-text-primary);
  }

  .required {
    color: var(--color-danger-text);
  }

  .field-hint {
    margin: var(--space-1) 0 var(--space-2);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .reason,
  .key {
    width: 100%;
    background: var(--color-bg-input);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    color: var(--color-text-primary);
    font-family: inherit;
    font-size: var(--text-sm);
    padding: var(--space-2) var(--space-3);
  }

  .key {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .reason:focus,
  .key:focus {
    outline: none;
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .reason.invalid,
  .key.invalid {
    border-color: var(--color-danger-border);
  }

  .field-error,
  .submit-error {
    margin: var(--space-2) 0 0;
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }

  .submit-error {
    background: var(--color-danger-bg);
    border: 1px solid var(--color-danger-border);
    border-radius: var(--radius-md);
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-sm);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    margin-top: var(--space-5);
  }
</style>
