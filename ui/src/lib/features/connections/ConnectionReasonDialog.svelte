<!--
  Reason-required dialog for every connection catalog write (create, save,
  compile, archive), on the `Dialog` primitive like the operator's
  ControlReasonDialog: one Reason field, the failure of the last attempt
  inside the dialog, md buttons. The catalog refuses a write
  without a reason and records it with the caller's verified identity, so the
  confirm control is disabled with the reason in its title rather than allowed
  to fire and be refused.
-->
<script lang="ts">
  import { Button, Dialog, Field, Textarea } from '$lib/ui/primitives';
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

  let reason = $state('');
  let attempted = $state(false);

  const issue = $derived(reasonProblem(reason));

  // A fresh draft each time the dialog opens.
  $effect.pre(() => {
    if (!open) return;
    reason = '';
    attempted = false;
  });

  function confirm(): void {
    attempted = true;
    if (issue !== null || loading) return;
    onconfirm(reason.trim());
  }
</script>

<Dialog
  {open}
  {title}
  {description}
  dismissible={!loading}
  onclose={oncancel}
  data-testid="connection-reason-dialog"
>
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

  {#snippet footer()}
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
    line-height: var(--leading-snug);
  }
</style>
