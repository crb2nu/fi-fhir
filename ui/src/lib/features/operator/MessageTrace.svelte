<script lang="ts">
  /**
   * Receipt-to-delivery lineage for one durable message, plus the recovery
   * actions the control plane exposes for its delivery attempts.
   *
   * The event payload is rendered semantically: field coordinates and JSON
   * kinds only. The server never returns a stored value, so there is nothing
   * here to redact — this view shows the shape of the message, not its content.
   */

  import { createEventDispatcher } from 'svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Skeleton from '$lib/ui/Skeleton.svelte';
  import {
    attemptStatusVariant,
    deadLetterStateLabel,
    deliveryActionBlockedReason,
    formatTimestamp,
    outboxStatusVariant,
    shortDigest,
    type DeliveryAction
  } from './attemptPresentation';
  import type { OperatorMessageTrace } from './operatorApi';

  export let trace: OperatorMessageTrace | null = null;
  export let loading = false;
  export let error: string | null = null;
  export let receiptId: string | null = null;

  const dispatch = createEventDispatcher<{
    control: { action: DeliveryAction; attemptId: string };
    retry: void;
  }>();

  const actions: DeliveryAction[] = ['replay', 'resubmit', 'discard'];

  function actionLabel(action: DeliveryAction): string {
    return action.charAt(0).toUpperCase() + action.slice(1);
  }
</script>

<Panel title="Message trace" padding="md">
  <svelte:fragment slot="actions">
    {#if receiptId}
      <span class="receipt-id">{receiptId}</span>
    {/if}
  </svelte:fragment>

  {#if !receiptId}
    <EmptyState
      icon="search"
      title="No message selected"
      description="Choose a receipt to inspect its events, lineage, delivery attempts, and audit trail."
    />
  {:else if loading}
    <div aria-busy="true" aria-live="polite">
      <Skeleton lines={5} />
      <span class="sr-only">Loading message trace</span>
    </div>
  {:else if error}
    <div class="error-state" role="alert">
      <p class="error-message">{error}</p>
      <Button size="sm" variant="secondary" on:click={() => dispatch('retry')}>Retry</Button>
    </div>
  {:else if !trace}
    <EmptyState
      icon="folder"
      title="Message not found"
      description="This receipt is not available in your tenant. It may have been recorded elsewhere."
    />
  {:else}
    <section class="section" aria-labelledby="trace-events">
      <h3 id="trace-events" class="section-title">Canonical events</h3>
      {#if trace.events.length === 0}
        <p class="muted">This receipt produced no canonical events.</p>
      {:else}
        {#each trace.events as event (event.eventId)}
          <article class="event">
            <header class="event-header">
              <Badge variant="primary" size="sm">{event.eventType}</Badge>
              <span class="mono">MSH-10 {event.sourceMessageId}</span>
              <span class="mono">{event.correlationId}</span>
              <Badge variant="warning" size="sm">{event.classification}</Badge>
              <span class="mono muted">{formatTimestamp(event.recordedAt)}</span>
            </header>
            <p class="payload-caption">
              Payload structure only — field coordinates and JSON kinds. Stored values are never
              returned by the control plane.
            </p>
            {#if event.payloadFields.length === 0}
              <p class="muted">No payload structure recorded.</p>
            {:else}
              <ul class="fields">
                {#each event.payloadFields as field (field.path)}
                  <li>
                    <code>{field.path}</code>
                    <span class="kind">{field.kind}</span>
                    {#if field.repeated}<span class="repeated">repeated</span>{/if}
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
          <article class="lineage">
            <header class="event-header">
              <span class="mono">trace {link.traceId}</span>
              <span class="mono muted">{formatTimestamp(link.recordedAt)}</span>
            </header>
            <dl class="revisions">
              <div>
                <dt>Source</dt>
                <dd class="mono" title={link.artifactRevisions.source.digest}>
                  {link.artifactRevisions.source.artifactId}@{link.artifactRevisions.source
                    .revisionId}
                  <span class="muted">{shortDigest(link.artifactRevisions.source.digest)}</span>
                </dd>
              </div>
              <div>
                <dt>Profile</dt>
                <dd class="mono" title={link.artifactRevisions.profile.digest}>
                  {link.artifactRevisions.profile.artifactId}@{link.artifactRevisions.profile
                    .revisionId}
                  <span class="muted">{shortDigest(link.artifactRevisions.profile.digest)}</span>
                </dd>
              </div>
              <div>
                <dt>Workflow</dt>
                <dd class="mono" title={link.artifactRevisions.workflow.digest}>
                  {link.artifactRevisions.workflow.artifactId}@{link.artifactRevisions.workflow
                    .revisionId}
                  <span class="muted">{shortDigest(link.artifactRevisions.workflow.digest)}</span>
                </dd>
              </div>
            </dl>
            {#if link.routes.length > 0}
              <ul class="routes">
                {#each link.routes as route (route.route)}
                  <li>
                    <Badge variant={route.matched ? 'success' : 'default'} size="sm">
                      {route.route}
                    </Badge>
                    <span class="muted">{route.transformCount} transforms</span>
                    {#if route.skipped}
                      <span class="muted">skipped{route.skipReason ? `: ${route.skipReason}` : ''}</span>
                    {/if}
                    {#each route.plannedActions as planned (planned)}
                      <span class="chip">{planned}</span>
                    {/each}
                  </li>
                {/each}
              </ul>
            {/if}
            {#if link.diagnostics.length > 0}
              <ul class="diagnostics">
                {#each link.diagnostics as diagnostic (diagnostic.code + diagnostic.stage)}
                  <li>
                    <Badge
                      variant={diagnostic.severity === 'error' ? 'danger' : 'warning'}
                      size="sm"
                    >
                      {diagnostic.severity}
                    </Badge>
                    <code>{diagnostic.code}</code>
                    <span class="muted">{diagnostic.stage}</span>
                    {#if diagnostic.path}<span class="mono muted">{diagnostic.path}</span>{/if}
                  </li>
                {/each}
              </ul>
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
          <article class="attempt">
            <header class="event-header">
              <span class="mono">{attempt.attemptId}</span>
              <Badge variant={attemptStatusVariant(attempt.status)} size="sm">
                {attempt.status}
              </Badge>
              <Badge variant={outboxStatusVariant(attempt.outboxStatus)} size="sm">
                outbox {attempt.outboxStatus}
              </Badge>
              <span class="muted">attempt {attempt.attemptCount}</span>
              <span class="muted">{attempt.route} → {attempt.action}</span>
            </header>
            <p class="muted">{deadLetterStateLabel(attempt.deadLetter)}</p>
            {#if attempt.lastErrorCode}
              <p class="failure">
                <code>{attempt.lastErrorCode}</code>
                <span>{attempt.lastErrorDetail}</span>
              </p>
            {/if}
            <div class="attempt-actions">
              {#each actions as action (action)}
                {@const blocked = deliveryActionBlockedReason(attempt, action)}
                <Button
                  size="sm"
                  variant={action === 'discard' ? 'danger' : 'secondary'}
                  disabled={blocked !== null}
                  title={blocked ?? undefined}
                  on:click={() => dispatch('control', { action, attemptId: attempt.attemptId })}
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
        <div class="table-scroll">
          <table class="records">
            <caption class="sr-only">Append-only delivery audit records</caption>
            <thead>
              <tr>
                <th scope="col">Event</th>
                <th scope="col">Attempt</th>
                <th scope="col">Actor</th>
                <th scope="col">Reason</th>
                <th scope="col">Recorded</th>
              </tr>
            </thead>
            <tbody>
              {#each trace.audit as record (record.auditId)}
                <tr>
                  <td><Badge size="sm">{record.eventKind}</Badge></td>
                  <td class="mono">{record.attemptId}</td>
                  <td class="mono">{record.principal.id || '—'}</td>
                  <td>{record.reason || '—'}</td>
                  <td class="mono">{formatTimestamp(record.recordedAt)}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        </div>
      {/if}
    </section>
  {/if}
</Panel>

<style>
  .receipt-id {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .section {
    margin-bottom: var(--space-6);
  }

  .section-title {
    margin: 0 0 var(--space-3);
    font-family: var(--font-heading);
    font-size: var(--text-sm);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--color-text-tertiary);
  }

  .event,
  .lineage,
  .attempt {
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-md);
    padding: var(--space-3);
    margin-bottom: var(--space-3);
  }

  .event-header {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    align-items: center;
    margin-bottom: var(--space-2);
  }

  .payload-caption {
    margin: 0 0 var(--space-2);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .fields {
    list-style: none;
    margin: 0;
    padding: 0;
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(16rem, 1fr));
    gap: var(--space-1) var(--space-3);
  }

  .fields li {
    display: flex;
    gap: var(--space-2);
    align-items: baseline;
    font-size: var(--text-xs);
  }

  .fields code {
    font-family: var(--font-mono);
    color: var(--color-text-primary);
  }

  .kind {
    color: var(--color-text-tertiary);
  }

  .repeated {
    color: var(--color-info-text);
  }

  .revisions {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(14rem, 1fr));
    gap: var(--space-2);
    margin: 0 0 var(--space-2);
  }

  .revisions dt {
    font-size: var(--text-2xs);
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: var(--color-text-tertiary);
  }

  .revisions dd {
    margin: 0;
  }

  .routes,
  .diagnostics {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .routes li,
  .diagnostics li {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
    align-items: center;
    font-size: var(--text-xs);
    padding: var(--space-1) 0;
  }

  .chip {
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    padding: 0 var(--space-2);
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    color: var(--color-text-secondary);
  }

  .failure {
    margin: 0 0 var(--space-2);
    font-size: var(--text-xs);
    color: var(--color-danger-text);
    display: flex;
    gap: var(--space-2);
  }

  .attempt-actions {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .muted {
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
    margin: 0 0 var(--space-2);
  }

  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .error-state {
    background: var(--color-danger-bg);
    border: 1px solid var(--color-danger-border);
    border-radius: var(--radius-md);
    padding: var(--space-3);
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .error-message {
    margin: 0;
    color: var(--color-danger-text);
    font-size: var(--text-sm);
  }

  .table-scroll {
    overflow-x: auto;
  }

  .records {
    width: 100%;
    border-collapse: collapse;
    font-size: var(--text-sm);
  }

  .records th,
  .records td {
    text-align: left;
    padding: var(--space-2);
    border-bottom: 1px solid var(--color-border-subtle);
    white-space: nowrap;
  }

  .records thead th {
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
</style>
