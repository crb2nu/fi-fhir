<!--
  StatisticsView — Verification › Statistics: what the durable tables say
  about one window (`operatorAdmissionStatistics`). Receipts by status and
  definition, canonical events by type, delivery attempts by status and
  destination, and a per-bucket series drawn as plain bars. "Delivered" is
  the attempts ledger's own word: attempts created in the window that have
  succeeded. Every figure is a server count; nothing is sampled or estimated.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import { EmptyState, IconButton, Panel, Table, Td, Th, Tr } from '$lib/ui/primitives';
  import { describeOperatorFailure } from '$lib/features/operator/operatorErrors';
  import StatisticsSeries from './StatisticsSeries.svelte';
  import WindowPicker from './WindowPicker.svelte';
  import { fetchAdmissionStatistics, type AdmissionStatistics } from './verificationApi';
  import { connectionHref, definitionHref } from './verificationLinks';
  import { deliveredRatio, describeWindow, resolveWindow, type WindowChoice } from './statisticsWindow';

  interface Props {
    choice: WindowChoice;
  }

  let { choice = $bindable() }: Props = $props();

  let stats = $state<AdmissionStatistics | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let seq = 0;

  const delivered = $derived(stats ? deliveredRatio(stats) : null);
  const empty = $derived(
    stats !== null &&
      stats.acceptedReceipts + stats.rejectedReceipts + stats.canonicalEvents === 0 &&
      stats.queuedAttempts + stats.succeededAttempts + stats.failedAttempts === 0
  );

  async function load(next: WindowChoice): Promise<void> {
    const resolved = resolveWindow(next);
    if (!resolved.ok) {
      error = resolved.message;
      return;
    }
    const current = ++seq;
    loading = true;
    error = null;
    try {
      const result = await fetchAdmissionStatistics(
        { from: resolved.window.from, to: resolved.window.to },
        resolved.window.bucket
      );
      if (current !== seq) return;
      stats = result;
    } catch (err) {
      if (current !== seq) return;
      error = describeOperatorFailure(err).message;
      stats = null;
    } finally {
      if (current === seq) loading = false;
    }
  }

  onMount(() => {
    void load(choice);
  });
</script>

<div class="statistics" data-testid="verification-statistics">
  <Panel title="Statistics" flush>
    {#snippet actions()}
      <IconButton icon={RefreshCw} label="Refresh statistics" {loading} onclick={() => void load(choice)} />
    {/snippet}
    <div class="bar">
      <WindowPicker bind:choice disabled={loading} onapply={(next) => void load(next)} />
      {#if stats}
        <span class="window" data-testid="statistics-window">{describeWindow(stats)}</span>
      {/if}
    </div>

    {#if loading && !stats}
      <EmptyState message="Counting admissions" aria-busy="true" aria-live="polite" />
    {:else if error}
      <EmptyState icon={CircleAlert} role="alert" message={error} actionLabel="Retry" onaction={() => void load(choice)} />
    {:else if stats}
      <div class="body" aria-busy={loading}>
        <dl class="tiles">
          <div class="tile" data-testid="stat-accepted" data-value={stats.acceptedReceipts}>
            <dt>Accepted receipts</dt>
            <dd>{stats.acceptedReceipts.toLocaleString()}</dd>
          </div>
          <div class="tile" data-testid="stat-rejected" data-value={stats.rejectedReceipts}>
            <dt>Rejected receipts</dt>
            <dd>{stats.rejectedReceipts.toLocaleString()}</dd>
          </div>
          <div class="tile" data-testid="stat-events" data-value={stats.canonicalEvents}>
            <dt>Canonical events</dt>
            <dd>{stats.canonicalEvents.toLocaleString()}</dd>
          </div>
          <div class="tile" data-testid="stat-attempts">
            <dt>Delivery attempts</dt>
            <dd>
              <span title="succeeded">{stats.succeededAttempts.toLocaleString()} succeeded</span>
              <span class="sub">
                {stats.failedAttempts.toLocaleString()} failed · {stats.queuedAttempts.toLocaleString()} queued
              </span>
            </dd>
          </div>
        </dl>
        <p class="delivered" data-testid="stat-delivered" data-ratio={delivered?.ratio ?? ''}>{delivered?.sentence}</p>
        {#if empty}
          <p class="muted" data-testid="statistics-empty">Nothing was admitted or delivered in this window.</p>
        {/if}

        <StatisticsSeries series={stats.series} bucket={stats.bucket} />

        <div class="groups">
          <section aria-labelledby="by-type">
            <h3 id="by-type">Events by type</h3>
            {#if stats.eventsByType.length === 0}
              <p class="muted">None in this window.</p>
            {:else}
              <Table label="Canonical events by type">
                {#snippet head()}
                  <tr><Th>Event type</Th><Th numeric>Events</Th></tr>
                {/snippet}
                {#each stats.eventsByType as group (group.key)}
                  <Tr data-testid="stat-type-row" data-key={group.key}>
                    <Td mono value={group.key} />
                    <Td numeric value={group.count} />
                  </Tr>
                {/each}
              </Table>
            {/if}
          </section>
          <section aria-labelledby="by-definition">
            <h3 id="by-definition">Receipts by definition</h3>
            {#if stats.receiptsByDefinition.length === 0}
              <p class="muted">None in this window.</p>
            {:else}
              <Table label="Receipts by definition">
                {#snippet head()}
                  <tr><Th>Definition</Th><Th numeric>Accepted</Th><Th numeric>Rejected</Th></tr>
                {/snippet}
                {#each stats.receiptsByDefinition as group (`${group.definitionId}@${group.revisionId}`)}
                  <Tr data-testid="stat-definition-row" data-definition-id={group.definitionId}>
                    <Td>
                      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- verificationLinks.ts builds this href from resolve() plus a query string -->
                      <a class="link" href={definitionHref(group.definitionId, group.revisionId)}
                        >{group.definitionId}@{group.revisionId}</a
                      >
                    </Td>
                    <Td numeric value={group.accepted} />
                    <Td numeric value={group.rejected} />
                  </Tr>
                {/each}
              </Table>
            {/if}
          </section>
          <section aria-labelledby="by-destination">
            <h3 id="by-destination">Delivery attempts by destination</h3>
            {#if stats.attemptsByDestination.length === 0}
              <p class="muted">None in this window.</p>
            {:else}
              <Table label="Delivery attempts by destination">
                {#snippet head()}
                  <tr>
                    <Th>Destination</Th><Th numeric>Succeeded</Th><Th numeric>Failed</Th><Th numeric>Queued</Th>
                  </tr>
                {/snippet}
                {#each stats.attemptsByDestination as group (group.destinationArtifactId)}
                  <Tr data-testid="stat-destination-row" data-destination-id={group.destinationArtifactId}>
                    <Td>
                      <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- verificationLinks.ts builds this href from resolve() plus a query string -->
                      <a class="link" href={connectionHref(group.destinationArtifactId)}>{group.destinationArtifactId}</a>
                    </Td>
                    <Td numeric value={group.succeeded} />
                    <Td numeric value={group.failed} />
                    <Td numeric value={group.queued} />
                  </Tr>
                {/each}
              </Table>
            {/if}
          </section>
        </div>
        {#if stats.groupsTruncated}
          <p class="muted">A grouped list reached the server's 100-group bound and is cut; the totals above are not.</p>
        {/if}
      </div>
    {/if}
  </Panel>
</div>

<style>
  .bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .window {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-tertiary);
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-3);
  }

  .tiles {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: var(--space-2);
    margin: 0;
  }

  .tile {
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    background: var(--color-bg-elevated);
  }

  .tile dt {
    font-size: var(--text-label);
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .tile dd {
    display: flex;
    flex-direction: column;
    margin: var(--space-1) 0 0;
    font-family: var(--font-mono);
    font-size: var(--text-lg);
    color: var(--color-text-primary);
  }

  .tile .sub {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .delivered {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--color-text-secondary);
  }

  .muted {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .groups {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
    gap: var(--space-3);
  }

  .groups h3 {
    margin: 0 0 var(--space-1);
    font-size: var(--text-sm);
    font-weight: 600;
    color: var(--color-text-primary);
  }

  .link {
    color: var(--color-text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    text-decoration: underline;
    text-decoration-color: var(--color-border-strong);
    text-underline-offset: 2px;
  }

  .link:hover {
    text-decoration-color: currentColor;
  }
</style>
