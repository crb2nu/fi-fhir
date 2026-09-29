<!--
  The session sidebar's confirm dialog (.loom/42 E-3), on the Dialog
  primitive in the repo's reason-dialog idiom (ConnectionReasonDialog): the
  failure of the last attempt inside the dialog, md buttons, not dismissible
  while a write is in flight.

  Two uses:
  - Export: a Reason (required, recorded with the verified caller on the
    export row) and, only when the identity holds integration.phi.export, the
    raw-payload option; otherwise the sentence naming why it is not offered.
  - Archive: no Reason field, because archiveIntegrationSession takes none and
    records none; the description says so instead of collecting text the
    server would drop.
-->
<script lang="ts">
  import { Button, Dialog, Field, Textarea } from '$lib/ui/primitives';
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
     * `blockedSentence` says why it is not offered. Omit for no option.
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

  let reason = $state('');
  let includeRaw = $state(false);
  let attempted = $state(false);

  const issue = $derived(reasonRequired ? exportReasonProblem(reason) : null);

  // A fresh draft each time the dialog opens.
  $effect.pre(() => {
    if (!open) return;
    reason = '';
    includeRaw = false;
    attempted = false;
  });

  function confirm(): void {
    attempted = true;
    if (issue !== null || loading) return;
    onconfirm(reason.trim(), Boolean(rawPayload?.allowed && includeRaw));
  }
</script>

<Dialog {open} {title} {description} dismissible={!loading} onclose={oncancel} data-testid={testid}>
  {#if reasonRequired}
    <Field label="Reason" required hint={reasonHint} error={attempted ? issue : undefined}>
      <Textarea rows={3} maxlength={MAX_EXPORT_REASON_BYTES} bind:value={reason} placeholder="Why is this needed?" />
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

  {#snippet footer()}
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
  {/snippet}
</Dialog>

<style>
  .blocked {
    margin: 0;
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-secondary);
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
</style>
