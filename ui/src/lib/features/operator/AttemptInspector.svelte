<!--
  AttemptInspector — one delivery attempt in full, beside Delivery › Attempts:
  its resubmit parent and children, schedule and completion, the outbox row and
  its lease, the destination provenance ledger (the single-attempt read's 25
  rows), and its own append-only audit trail paged by the server
  (`operatorAttemptAudit`), each record with the detail document it carries.
-->
<script lang="ts">
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import ChevronLeft from '@lucide/svelte/icons/chevron-left';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import FileQuestion from '@lucide/svelte/icons/file-question';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import Send from '@lucide/svelte/icons/send';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import X from '@lucide/svelte/icons/x';
  import {
    Badge,
    Button,
    EmptyState,
    IconButton,
    KeyValue,
    Panel,
    Table,
    Td,
    Th,
    Tr,
    type IconComponent
  } from '$lib/ui/primitives';
  import AuditDetail from './AuditDetail.svelte';
  import DestinationDeliveries from './DestinationDeliveries.svelte';
  import {
    attemptStatusVariant,
    badgeTone,
    deadLetterStateLabel,
    deliveryActionBlockedReason,
    describeOutboxLease,
    formatTimestamp,
    outboxStatusVariant,
    shortDigest,
    type DeliveryAction
  } from './attemptPresentation';
  import { deliveryControlBlock } from './operatorAccess';
  import {
    fetchAttempt,
    fetchAttemptAudit,
    type OperatorAttempt,
    type OperatorAuditRecord
  } from './operatorApi';
  import { describeOperatorFailure } from './operatorErrors';
  import { connectionHref } from './operatorLinks';

  interface Props {
    attemptId: string;
    /** Open the message trace of the attempt's receipt. */
    ontrace: (receiptId: string) => void;
    /** Inspect another attempt (the resubmit parent). */
    oninspect: (attemptId: string) => void;
    oncontrol: (action: DeliveryAction, attemptId: string) => void;
    onclose: () => void;
  }

  let { attemptId, ontrace, oninspect, oncontrol, onclose }: Props = $props();

  const AUDIT_PAGE_SIZE = 5;

  let attempt = $state<OperatorAttempt | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let missing = $state(false);

  let audit = $state<OperatorAuditRecord[]>([]);
  let auditLoading = $state(false);
  let auditError = $state<string | null>(null);
  let auditCursor = $state<string | null>(null);
  let auditCursors = $state<string[]>([]);
  let auditHasNext = $state(false);
  let seq = 0;
  let auditSeq = 0;

  const recoveryActions: DeliveryAction[] = ['replay', 'resubmit', 'discard'];
  const actionIcons: Record<DeliveryAction, IconComponent> = {
    replay: RotateCcw,
    resubmit: Send,
    discard: Trash2
  };

  async function loadAttempt(id: string): Promise<void> {
    const current = ++seq;
    loading = true;
    error = null;
    missing = false;
    try {
      const answer = await fetchAttempt(id);
      if (current !== seq) return;
      attempt = answer ?? null;
      missing = !answer;
    } catch (err) {
      if (current !== seq) return;
      attempt = null;
      error = describeOperatorFailure(err).message;
    } finally {
      if (current === seq) loading = false;
    }
  }

  async function loadAudit(id: string, after: string | null): Promise<void> {
    const current = ++auditSeq;
    auditLoading = true;
    auditError = null;
    try {
      const page = await fetchAttemptAudit(id, { first: AUDIT_PAGE_SIZE, after });
      if (current !== auditSeq) return;
      audit = page.nodes;
      auditHasNext = page.pageInfo.hasNextPage;
      auditCursor = page.pageInfo.endCursor ?? null;
    } catch (err) {
      if (current !== auditSeq) return;
      audit = [];
      auditHasNext = false;
      auditError = describeOperatorFailure(err).message;
    } finally {
      if (current === auditSeq) auditLoading = false;
    }
  }

  /** Re-reads the attempt and the first audit page (after a control action). */
  export function reload(): void {
    auditCursors = [];
    void loadAttempt(attemptId);
    void loadAudit(attemptId, null);
  }

  $effect(() => {
    const id = attemptId;
    auditCursors = [];
    void loadAttempt(id);
    void loadAudit(id, null);
  });

  function nextAuditPage(): void {
    if (!auditCursor) return;
    auditCursors = [...auditCursors, auditCursor];
    void loadAudit(attemptId, auditCursor);
  }

  function previousAuditPage(): void {
    const previous = auditCursors.slice(0, -1);
    auditCursors = previous;
    void loadAudit(attemptId, previous.at(-1) ?? null);
  }

  function actionLabel(action: DeliveryAction): string {
    return action.charAt(0).toUpperCase() + action.slice(1);
  }
</script>

<Panel title="Attempt" flush class="inspector" data-testid="attempt-inspector" data-attempt-id={attemptId}>
  {#snippet actions()}
    <span class="attempt-id" title={attemptId}>{attemptId}</span>
    <IconButton icon={RefreshCw} label="Refresh attempt" loading={loading} onclick={reload} />
    <IconButton icon={X} label="Close attempt" onclick={onclose} />
  {/snippet}

  {#if loading && !attempt}
    <EmptyState align="start" message="Loading delivery attempt" aria-busy="true" aria-live="polite" />
  {:else if error}
    <EmptyState icon={CircleAlert} align="start" role="alert" message={error} actionLabel="Retry" onaction={reload} />
  {:else if missing || !attempt}
    <EmptyState
      icon={FileQuestion}
      align="start"
      message={`Delivery attempt ${attemptId} is not available in your tenant.`}
      data-testid="attempt-inspector-missing"
    />
  {:else}
    {@const current = attempt}
    <section class="section">
      <header class="head">
        <Badge tone={badgeTone(attemptStatusVariant(current.status))} dot>{current.status}</Badge>
        <Badge tone={badgeTone(outboxStatusVariant(current.outboxStatus))}>outbox {current.outboxStatus}</Badge>
        <Badge tone={current.deadLetter?.active ? 'warning' : 'neutral'}>{deadLetterStateLabel(current.deadLetter)}</Badge>
      </header>
      <dl class="links">
        <dt>Receipt</dt>
        <dd>
          <button type="button" class="link" onclick={() => ontrace(current.receiptId)}>{current.receiptId}</button>
        </dd>
        <dt>Resubmitted from</dt>
        <dd data-testid="attempt-parent">
          {#if current.parentAttemptId}
            {@const parent = current.parentAttemptId}
            <button type="button" class="link" onclick={() => oninspect(parent)}>{parent}</button>
          {:else}
            <span class="none">An original attempt, not a resubmit</span>
          {/if}
        </dd>
        <dt>Destination</dt>
        <dd>
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- operatorLinks.ts builds this href from resolve() plus a query string -->
          <a href={connectionHref(current.destination.artifactId)} class="link" title="Open this destination's connection">
            {current.destination.artifactId}@{current.destination.revisionId}
          </a>
          <span class="none">· {current.destination.class} · {shortDigest(current.destination.digest)}</span>
        </dd>
      </dl>
      <KeyValue
        columns={2}
        items={[
          { key: 'Route', value: `${current.route} → ${current.action}`, mono: true },
          { key: 'Tries', value: current.attemptCount, mono: true },
          { key: 'Recorded', value: formatTimestamp(current.recordedAt), mono: true },
          { key: 'Scheduled', value: formatTimestamp(current.scheduledAt), mono: true },
          { key: 'Completed', value: current.completedAt ? formatTimestamp(current.completedAt) : 'Not completed', mono: true },
          { key: 'Trace', value: current.traceId, mono: true, truncate: true },
          { key: 'Outbox', value: describeOutboxLease(current), mono: true, truncate: true }
        ]}
      />
      {#if current.lastErrorCode}
        <p class="failure"><code>{current.lastErrorCode}</code> <span>{current.lastErrorDetail}</span></p>
      {/if}
      <div class="actions">
        {#each recoveryActions as action (action)}
          {@const blocked = $deliveryControlBlock ?? deliveryActionBlockedReason(current, action)}
          <Button
            variant={action === 'discard' ? 'danger' : 'secondary'}
            icon={actionIcons[action]}
            disabled={blocked !== null}
            title={blocked ?? undefined}
            onclick={() => oncontrol(action, current.attemptId)}
          >
            {actionLabel(action)}
          </Button>
        {/each}
      </div>
    </section>

    <section class="section">
      <h3 class="section-title">Delivery</h3>
      <DestinationDeliveries
        deliveries={current.deliveries}
        label={`Destination deliveries for ${current.attemptId}, newest first`}
      />
    </section>
  {/if}

  <section class="section" data-testid="attempt-audit">
    <h3 class="section-title">Audit</h3>
    {#if auditLoading && audit.length === 0}
      <p class="none" aria-busy="true">Loading the audit trail</p>
    {:else if auditError}
      <p class="error" role="alert">{auditError}</p>
    {:else if audit.length === 0}
      <p class="none">No audit record for this attempt yet. The delivery worker writes one when it claims it.</p>
    {:else}
      <Table label={`Audit records for ${attemptId}`} layout="fixed" class="inner-table">
        {#snippet head()}
          <tr>
            <Th width="124px">Event</Th>
            <Th width="44px" numeric>Try</Th>
            <Th>Detail</Th>
            <Th width="152px">Recorded</Th>
          </tr>
        {/snippet}
        {#each audit as record (record.auditId)}
          <Tr data-testid="audit-row" data-event-kind={record.eventKind}>
            <Td><Badge>{record.eventKind}</Badge></Td>
            <Td numeric value={record.attemptCount} />
            <Td>
              <AuditDetail detail={record.detail} />
              {#if record.reason || record.principal.id}
                <span class="none"> · {record.principal.id || 'worker'}{record.reason ? `: “${record.reason}”` : ''}</span>
              {/if}
            </Td>
            <Td mono muted value={formatTimestamp(record.recordedAt)} />
          </Tr>
        {/each}
      </Table>
      <div class="pagination">
        <span class="page" data-testid="audit-page">Audit page {auditCursors.length + 1}</span>
        <Button
          variant="ghost"
          icon={ChevronLeft}
          onclick={previousAuditPage}
          disabled={auditCursors.length === 0 || auditLoading}
          title={auditCursors.length === 0 ? 'You are on the first page.' : undefined}
        >
          Previous
        </Button>
        <Button
          variant="ghost"
          icon={ChevronRight}
          onclick={nextAuditPage}
          disabled={!auditHasNext || auditLoading}
          title={!auditHasNext ? 'This is the oldest audit record.' : undefined}
        >
          Next
        </Button>
      </div>
    {/if}
  </section>
</Panel>

<style>
  :global(.inspector) {
    flex: 1 1 auto;
    border: 0;
    border-radius: 0;
    background: transparent;
  }

  .attempt-id {
    max-width: 240px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-tertiary);
  }

  .section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .section-title {
    margin: 0;
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .head,
  .actions {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .links {
    display: grid;
    grid-template-columns: max-content minmax(0, 1fr);
    gap: var(--space-1) var(--space-3);
    margin: 0;
    font-size: var(--text-xs);
  }

  .links dt {
    color: var(--color-text-tertiary);
  }

  .links dd {
    margin: 0;
    min-width: 0;
    overflow-wrap: anywhere;
  }

  .link {
    padding: 0;
    border: 0;
    background: none;
    color: var(--color-text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    text-decoration: underline;
    text-decoration-color: var(--color-border-strong);
    text-underline-offset: 2px;
    cursor: pointer;
    overflow-wrap: anywhere;
    text-align: left;
  }

  .link:hover {
    text-decoration-color: currentColor;
  }

  .none {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .failure,
  .error {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }

  .failure code {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }

  .section :global(.inner-table) {
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
  }

  .pagination {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-1);
  }

  .page {
    margin-right: auto;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }
</style>
