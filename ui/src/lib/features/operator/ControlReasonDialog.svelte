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
  import { Button, Dialog, Field, Input, Textarea } from '$lib/ui/primitives';
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
  let wasOpen = false;
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

  // A fresh draft (and a fresh idempotency nonce) each time the dialog opens.
  $: if (open !== wasOpen) {
    if (open) {
      reason = '';
      keyTouched = false;
      attempted = false;
      nonce = Math.random().toString(36).slice(2, 10);
    }
    wasOpen = open;
  }

  function handleConfirm() {
    attempted = true;
    if (!ready) return;
    dispatch('confirm', { reason: reason.trim(), idempotencyKey });
  }

  function handleCancel() {
    dispatch('cancel');
  }
</script>

<Dialog
  {open}
  {title}
  description={description || undefined}
  dismissible={!loading}
  onclose={handleCancel}
>
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

  {#snippet footer()}
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
  {/snippet}
</Dialog>

<style>
  .submit-error {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--color-danger-border);
    border-radius: var(--radius-sm);
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
    font-size: var(--text-xs);
  }
</style>
