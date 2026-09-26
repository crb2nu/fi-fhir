<script lang="ts">
  /**
   * Receipt-to-delivery lineage for one durable message, plus the recovery
   * actions the control plane exposes for its delivery attempts. Operator ›
   * Messages, the pane beside the receipts table.
   *
   * The event payload is rendered semantically: field coordinates and JSON
   * kinds only. The server never returns a stored value, so there is nothing
   * here to redact — this view shows the shape of the message, not its content.
   * Each attempt's Delivery block renders the destination provenance ledger,
   * which is clinical-content-free by construction (Slice 4.2c).
   */

  import { createEventDispatcher } from 'svelte';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import FileQuestion from '@lucide/svelte/icons/file-question';
  import MousePointerClick from '@lucide/svelte/icons/mouse-pointer-click';
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import Send from '@lucide/svelte/icons/send';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import {
    Badge,
    Button,
    EmptyState,
    KeyValue,
    Panel,
    Table,
    Td,
    Th,
    Tr,
    type IconComponent
  } from '$lib/ui/primitives';
  import DestinationDeliveries from './DestinationDeliveries.svelte';
  import {
    attemptStatusVariant,
    badgeTone,
    deadLetterStateLabel,
    deliveryActionBlockedReason,
    formatTimestamp,
    outboxStatusVariant,
    shortDigest,
    type DeliveryAction
  } from './attemptPresentation';
  import type { OperatorMessageTrace } from './operatorApi';
  import { deliveryControlBlock } from './operatorAccess';

  export let trace: OperatorMessageTrace | null = null;
  export let loading = false;
  export let error: string | null = null;
  export let receiptId: string | null = null;

  const dispatch = createEventDispatcher<{
    control: { action: DeliveryAction; attemptId: string };
    retry: void;
  }>();

  const recoveryActions: DeliveryAction[] = ['replay', 'resubmit', 'discard'];
  const actionIcons: Record<DeliveryAction, IconComponent> = {
    replay: RotateCcw,
    resubmit: Send,
    discard: Trash2
  };

  function actionLabel(action: DeliveryAction): string {
    return action.charAt(0).toUpperCase() + action.slice(1);
  }

  function revision(ref: { artifactId: string; revisionId: string; digest: string }): string {
    return `${ref.artifactId}@${ref.revisionId} · ${shortDigest(ref.digest)}`;
  }
</script>

<Panel title="Message trace" class="trace-panel" flush>
  {#snippet actions()}
    {#if receiptId}
      <span class="receipt-id" title={receiptId}>{receiptId}</span>
    {/if}
  {/snippet}

  {#if !receiptId && !loading && !error}
    <EmptyState
      icon={MousePointerClick}
      align="start"
      message="No message selected."
    />
  {:else if loading}
    <EmptyState align="start" message="Loading message trace" aria-busy="true" aria-live="polite" />
  {:else if error}
    <EmptyState
      icon={CircleAlert}
      align="start"
      role="alert"
      message={error}
      actionLabel="Retry"
      onaction={() => dispatch('retry')}
    />
  {:else if !trace}
    <EmptyState
      icon={FileQuestion}
      align="start"
      message="This receipt is not available in your tenant."
    />
  {:else}
    <section class="section" aria-labelledby="trace-receipt">
      <h3 id="trace-receipt" class="section-title">Receipt</h3>
      <KeyValue
        columns={2}
        items={[
          { key: 'Status', value: trace.receipt.status },
          { key: 'Recorded', value: formatTimestamp(trace.receipt.recordedAt), mono: true },
          { key: 'Correlation', value: trace.receipt.correlationId, mono: true, truncate: true },
          { key: 'Integration', value: revision(trace.receipt.integrationRevision), mono: true, truncate: true },
          { key: 'Principal', value: trace.receipt.principal.id, mono: true },
          { key: 'Retention', value: trace.receipt.rawRetentionMode },
          { key: 'Reason', value: trace.receipt.reason },
          {
            key: 'Counts',
            value: `${trace.receipt.eventCount} events · ${trace.receipt.attemptCount} attempts · ${trace.receipt.failedAttemptCount} failed · ${trace.receipt.deadLetterCount} DLQ`,
            mono: true
          }
        ]}
      />
    </section>

    <section class="section" aria-labelledby="trace-events">
      <h3 id="trace-events" class="section-title">Canonical events</h3>
      {#if trace.events.length === 0}
        <p class="muted">This receipt produced no canonical events.</p>
      {:else}
        {#each trace.events as event (event.eventId)}
          <article class="block">
            <header class="block-head">
              <span class="mono strong">{event.eventType}</span>
              <Badge tone="warning">{event.classification}</Badge>
              <span class="mono muted">{formatTimestamp(event.recordedAt)}</span>
            </header>
            <KeyValue
              items={[
                { key: 'MSH-10', value: event.sourceMessageId, mono: true },
                { key: 'Correlation', value: event.correlationId, mono: true, truncate: true },
                { key: 'Event id', value: event.eventId, mono: true, truncate: true }
              ]}
            />
            <p class="caption">
              Payload structure only: field coordinates and JSON kinds. The control plane never
              returns stored values.
            </p>
            {#if event.payloadFields.length === 0}
              <p class="muted">No payload structure recorded.</p>
            {:else}
              <ul class="fields" aria-label={`Payload fields of ${event.eventType}`}>
                {#each event.payloadFields as field (field.path)}
                  <li>
                    <code>{field.path}</code>
                    <span class="kind">{field.kind}{field.repeated ? ' []' : ''}</span>
                  </li>
                {/each}
              </ul>
            {/if}
            {#if event.payloadTruncated}
              <p class="muted">Field list truncated at the server-side bound.</p>
            {/if}
          </article>
        {/each}
      {/if}
    </section>

    <section class="section" aria-labelledby="trace-lineage">
      <h3 id="trace-lineage" class="section-title">Lineage</h3>
      {#if trace.lineage.length === 0}
        <p class="muted">No lineage was recorded for this receipt.</p>
      {:else}
        {#each trace.lineage as link (link.lineageId)}
          <article class="block">
            <KeyValue
              items={[
                { key: 'Trace', value: link.traceId, mono: true, truncate: true },
                { key: 'Recorded', value: formatTimestamp(link.recordedAt), mono: true },
                { key: 'Source', value: revision(link.artifactRevisions.source), mono: true, truncate: true },
                { key: 'Profile', value: revision(link.artifactRevisions.profile), mono: true, truncate: true },
                { key: 'Workflow', value: revision(link.artifactRevisions.workflow), mono: true, truncate: true }
              ]}
            />
            {#if link.routes.length > 0}
              <Table label={`Routes for trace ${link.traceId}`} class="inner-table">
                {#snippet head()}
                  <tr>
                    <Th>Route</Th>
                    <Th width="96px">Result</Th>
                    <Th width="80px" numeric>Transforms</Th>
                    <Th>Planned actions</Th>
                  </tr>
                {/snippet}
                {#each link.routes as route (route.route)}
                  <Tr>
                    <Td mono value={route.route} />
                    <Td>
                      <Badge tone={route.matched ? 'success' : 'neutral'}>
                        {route.matched ? 'matched' : route.skipped ? 'skipped' : 'no match'}
                      </Badge>
                    </Td>
                    <Td numeric value={route.transformCount} />
                    <Td
                      mono
                      truncate
                      muted
                      value={route.skipped && route.skipReason
                        ? route.skipReason
                        : route.plannedActions.join(', ') || '—'}
                    />
                  </Tr>
                {/each}
              </Table>
            {/if}
            {#if link.diagnostics.length > 0}
              <Table label={`Diagnostics for trace ${link.traceId}`} class="inner-table">
                {#snippet head()}
                  <tr>
                    <Th width="88px">Severity</Th>
                    <Th>Code</Th>
                    <Th width="96px">Stage</Th>
                    <Th>Path</Th>
                  </tr>
                {/snippet}
                {#each link.diagnostics as diagnostic (diagnostic.code + diagnostic.stage + (diagnostic.path ?? ''))}
                  <Tr>
                    <Td>
                      <Badge tone={diagnostic.severity === 'error' ? 'danger' : 'warning'} dot>
                        {diagnostic.severity}
                      </Badge>
                    </Td>
                    <Td mono value={diagnostic.code} />
                    <Td muted value={diagnostic.stage} />
                    <Td mono truncate muted value={diagnostic.path ?? '—'} />
                  </Tr>
                {/each}
              </Table>
            {/if}
          </article>
        {/each}
      {/if}
    </section>

    <section class="section" aria-labelledby="trace-attempts">
      <h3 id="trace-attempts" class="section-title">Delivery attempts</h3>
      {#if trace.attempts.length === 0}
        <p class="muted">No delivery attempt was created for this receipt.</p>
      {:else}
        {#each trace.attempts as attempt (attempt.attemptId)}
          <article class="block">
            <header class="block-head">
              <span class="mono strong" title={attempt.attemptId}>{attempt.attemptId}</span>
              <Badge tone={badgeTone(attemptStatusVariant(attempt.status))} dot>{attempt.status}</Badge>
              <Badge tone={badgeTone(outboxStatusVariant(attempt.outboxStatus))}>
                outbox {attempt.outboxStatus}
              </Badge>
            </header>
            <KeyValue
              items={[
                { key: 'Route', value: `${attempt.route} → ${attempt.action}`, mono: true },
                { key: 'Destination', value: revision(attempt.destination), mono: true, truncate: true },
                { key: 'Attempts', value: attempt.attemptCount, mono: true },
                { key: 'Dead letter', value: deadLetterStateLabel(attempt.deadLetter) },
                { key: 'Recorded', value: formatTimestamp(attempt.recordedAt), mono: true }
              ]}
            />
            {#if attempt.lastErrorCode}
              <p class="failure">
                <code>{attempt.lastErrorCode}</code>
                <span>{attempt.lastErrorDetail}</span>
              </p>
            {/if}
            <div class="delivery-block">
              <h4 class="block-title">Delivery</h4>
              <DestinationDeliveries
                deliveries={attempt.deliveries}
                label={`Destination deliveries for ${attempt.attemptId}, newest first`}
              />
            </div>
            <div class="attempt-actions">
              {#each recoveryActions as action (action)}
                {@const blocked =
                  $deliveryControlBlock ?? deliveryActionBlockedReason(attempt, action)}
                <Button
                  variant={action === 'discard' ? 'danger' : 'secondary'}
                  icon={actionIcons[action]}
                  disabled={blocked !== null}
                  title={blocked ?? undefined}
                  onclick={() => dispatch('control', { action, attemptId: attempt.attemptId })}
                >
                  {actionLabel(action)}
                </Button>
              {/each}
            </div>
          </article>
        {/each}
      {/if}
    </section>

    <section class="section" aria-labelledby="trace-audit">
      <h3 id="trace-audit" class="section-title">Delivery audit</h3>
      {#if trace.audit.length === 0}
        <p class="muted">No delivery audit records yet.</p>
      {:else}
        <Table label="Append-only delivery audit records" class="inner-table">
          {#snippet head()}
            <tr>
              <Th width="104px">Event</Th>
              <Th>Attempt</Th>
              <Th>Actor</Th>
              <Th>Reason</Th>
              <Th width="164px">Recorded</Th>
            </tr>
          {/snippet}
          {#each trace.audit as record (record.auditId)}
            <Tr>
              <Td><Badge>{record.eventKind}</Badge></Td>
              <Td mono truncate value={record.attemptId} />
              <Td mono truncate value={record.principal.id || '—'} />
              <Td truncate value={record.reason || '—'} />
              <Td mono muted value={formatTimestamp(record.recordedAt)} />
            </Tr>
          {/each}
        </Table>
      {/if}
    </section>
  {/if}
</Panel>

<style>
  :global(.trace-panel) {
    flex: 1 1 auto;
    border: 0;
    border-radius: 0;
    background: transparent;
  }

  .receipt-id {
    max-width: 280px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    padding-right: var(--space-2);
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

  .section-title,
  .block-title {
    margin: 0;
    font-family: var(--font-ui);
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .block {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    background: var(--color-bg-base);
  }

  .block-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .strong {
    color: var(--color-text-primary);
    font-weight: var(--font-semibold);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    min-width: 0;
  }

  .caption {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .fields {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(15rem, 1fr));
    gap: 2px var(--space-3);
  }

  .fields li {
    display: flex;
    gap: var(--space-2);
    align-items: baseline;
    min-width: 0;
    font-size: var(--text-xs);
  }

  .fields code {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-primary);
  }

  .kind {
    flex: 0 0 auto;
    color: var(--color-text-tertiary);
  }

  .block :global(.inner-table) {
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
  }

  .failure {
    display: flex;
    gap: var(--space-2);
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }

  .failure code {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }

  .delivery-block {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .attempt-actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .muted {
    margin: 0;
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
  }

  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }
</style>
